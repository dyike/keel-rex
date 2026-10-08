//go:build !windows

package backend

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func startSessionServer(cmd *exec.Cmd) error {
	ready, readyW, err := os.Pipe()
	if err != nil {
		return err
	}
	defer ready.Close()
	cmd.ExtraFiles = []*os.File{readyW}
	cmd.Env = append(os.Environ(), readyEnv+"=3")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err = cmd.Start()
	readyW.Close()
	if err != nil {
		return err
	}
	go cmd.Wait()
	ready.SetReadDeadline(time.Now().Add(5 * time.Second))
	var b [1]byte
	_, _ = ready.Read(b[:])
	return nil
}
func lockSessionServer(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
func notifySessionServerReady() {
	if os.Getenv(readyEnv) != "3" {
		return
	}
	os.Unsetenv(readyEnv)
	if f := os.NewFile(3, "ready"); f != nil {
		f.Write([]byte{1})
		f.Close()
	}
}
