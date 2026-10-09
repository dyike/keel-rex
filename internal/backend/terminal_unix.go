//go:build !windows

package backend

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type unixTerminal struct {
	*os.File
	cmd *exec.Cmd
}

func shellCommand() []string {
	shell := defaultShell()
	if shell == "" {
		shell = "/bin/zsh"
	}
	return []string{shell, "-l"}
}

func startTerminal(cmd *exec.Cmd, cols, rows int) (terminalPTY, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &unixTerminal{File: f, cmd: cmd}, nil
}
func (t *unixTerminal) Resize(cols, rows int) error {
	return pty.Setsize(t.File, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (t *unixTerminal) Wait() error        { return t.cmd.Wait() }
func (t *unixTerminal) Hangup()            { _ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGHUP) }
func (t *unixTerminal) Kill()              { _ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL) }
func (t *unixTerminal) ForegroundPID() int { return foregroundPID(t.File) }
func terminalDirectory(path string) string { return path }
