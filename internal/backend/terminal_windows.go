package backend

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

type windowsTerminal struct {
	*conpty.ConPty
	cmd        *exec.Cmd
	job        windows.Handle
	jobMu      sync.Mutex
	done       chan error
	output     *os.File
	outputOnce sync.Once
}

func shellCommand() []string {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return []string{path, "-NoLogo"}
		}
	}
	if shell := os.Getenv("COMSPEC"); shell != "" {
		return []string{shell}
	}
	return []string{"cmd.exe"}
}

func startTerminal(cmd *exec.Cmd, cols, rows int) (terminalPTY, error) {
	console, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, fmt.Errorf("create Windows ConPTY (Windows 10 1809 or newer required): %w", err)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		console.Close()
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		console.Close()
		return nil, err
	}
	pid, handle, err := console.Spawn(cmd.Path, cmd.Args, &syscall.ProcAttr{Dir: cmd.Dir, Env: cmd.Env, Sys: cmd.SysProcAttr})
	if err != nil {
		windows.CloseHandle(job)
		console.Close()
		return nil, err
	}
	defer windows.CloseHandle(windows.Handle(handle))
	if err = windows.AssignProcessToJobObject(job, windows.Handle(handle)); err != nil {
		windows.TerminateProcess(windows.Handle(handle), 1)
		windows.CloseHandle(job)
		console.Close()
		return nil, err
	}
	cmd.Process, err = os.FindProcess(pid)
	if err != nil {
		windows.CloseHandle(job)
		console.Close()
		return nil, err
	}
	var output windows.Handle
	if err = windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(console.OutPipeReadFd()), windows.CurrentProcess(), &output, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		windows.CloseHandle(job)
		go io.Copy(io.Discard, console)
		console.Close()
		cmd.Process.Wait()
		return nil, err
	}
	t := &windowsTerminal{ConPty: console, cmd: cmd, job: job, done: make(chan error, 1), output: os.NewFile(uintptr(output), "conpty-output")}
	go func() {
		state, err := cmd.Process.Wait()
		cmd.ProcessState = state
		if err == nil && state != nil && !state.Success() {
			err = &exec.ExitError{ProcessState: state}
		}
		// ConPTY's output pipe can remain open after the shell exits. Close the
		// console concurrently with the session reader. Our duplicate output
		// handle survives the library's Close, preserving final output even on
		// Windows 11 24H2 where ClosePseudoConsole returns immediately.
		t.jobMu.Lock()
		windows.CloseHandle(t.job)
		t.job = 0
		t.jobMu.Unlock()
		console.Close()
		t.done <- err
	}()
	return t, nil
}
func (t *windowsTerminal) Read(b []byte) (int, error) { return t.output.Read(b) }
func (t *windowsTerminal) Close() error {
	err := t.ConPty.Close()
	t.outputOnce.Do(func() { t.output.Close() })
	return err
}
func (t *windowsTerminal) Wait() error { return <-t.done }
func (t *windowsTerminal) Hangup()     { t.Kill() }
func (t *windowsTerminal) Kill() {
	t.jobMu.Lock()
	defer t.jobMu.Unlock()
	if t.job != 0 {
		_ = windows.TerminateJobObject(t.job, 1)
	}
}
func (t *windowsTerminal) ForegroundPID() int { return windowsForegroundPID(t.cmd.Process.Pid) }
func terminalDirectory(path string) string {
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(strings.TrimPrefix(path, "localhost/"))
}
