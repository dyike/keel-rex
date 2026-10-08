package backend

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func startSessionServer(cmd *exec.Cmd) error {
	// Windows does not support exec.Cmd.ExtraFiles. Startup is confirmed by
	// the protocol handshake in ensureServer; the detached service survives
	// closing the desktop and its launching PowerShell window.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
func lockSessionServer(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
func notifySessionServerReady() { os.Unsetenv(readyEnv) }
