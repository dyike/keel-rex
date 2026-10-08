//go:build !windows

package backend

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// A throttled query keeps process inspection off the UI thread. The server
// watches the PTY's foreground process group, not commands guessed from input.
func (s *session) refreshProgram() {
	s.mu.Lock()
	if s.closed || s.exited || time.Since(s.processChecked) < 750*time.Millisecond {
		s.mu.Unlock()
		return
	}
	s.processChecked = time.Now()
	pid := s.cmd.Process.Pid
	master := s.master
	s.mu.Unlock()
	fg := master.ForegroundPID()
	if fg <= 0 {
		fg = pid
	}
	out, e := exec.Command("ps", "-p", strconv.Itoa(fg), "-o", "comm=").Output()
	if e != nil {
		return
	}
	name := programName(strings.TrimSpace(string(out)))
	if runtimeProgramName(name) {
		if command, err := exec.Command("ps", "-p", strconv.Itoa(fg), "-o", "args=").Output(); err == nil {
			name = identifyCLI(name, strings.Fields(string(command)))
		}
	}
	title := name
	if name == "ssh" {
		command, _ := exec.Command("ps", "-p", strconv.Itoa(fg), "-o", "args=").Output()
		if host := sshDestination(strings.Fields(string(command))); host != "" {
			title = "ssh " + host
		}
	}
	dir := processDirectory(fg)
	s.mu.Lock()
	s.program = name
	if name == "ssh" {
		s.title = title
	}
	if dir != "" {
		s.cwd = dir
	}
	s.mu.Unlock()
}

func identifyRemoteCLI(shellPID int, name string) string {
	if shellPID <= 0 {
		return name
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(shellPID), "-o", "tpgid=").Output()
	if err != nil {
		return name
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		return name
	}
	out, err = exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return name
	}
	return identifyCLI(name, strings.Fields(string(out)))
}
