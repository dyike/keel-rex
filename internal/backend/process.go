package backend

import (
	"context"
	"os/exec"
	"path/filepath"
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
	fg := foregroundPID(master)
	if fg <= 0 {
		fg = pid
	}
	out, e := exec.Command("ps", "-p", strconv.Itoa(fg), "-o", "comm=").Output()
	if e != nil {
		return
	}
	name := strings.TrimPrefix(filepath.Base(strings.TrimSpace(string(out))), "-")
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

// IsShellProgram recognizes idle shells, including login-shell process names.
func IsShellProgram(name string) bool {
	switch strings.TrimPrefix(filepath.Base(name), "-") {
	case "zsh", "bash", "fish", "sh", "dash", "ksh", "csh", "tcsh", "nu":
		return true
	}
	return false
}

func sshDestination(args []string) string {
	if len(args) == 0 {
		return ""
	}
	skip := false
	for _, arg := range args[1:] {
		if skip {
			skip = false
			continue
		}
		if arg == "--" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if len(arg) == 2 && strings.Contains("BbcDEeFIiJLlmOopRSWw", arg[1:]) {
				skip = true
			}
			continue
		}
		return arg
	}
	return ""
}

func runtimeProgramName(name string) bool { return name == "node" || name == "nodejs" || name == "bun" }

// Inspect the executable/script arguments, never prompt text or arbitrary
// arguments following the script. A regular Node process retains its identity.
func identifyCLI(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	executable := filepath.Base(args[0])
	if executable == "codex" || executable == "claude" {
		return executable
	}
	if !runtimeProgramName(name) {
		return name
	}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-e" || arg == "--eval" || arg == "-p" || arg == "--print" || strings.HasPrefix(arg, "--eval=") || strings.HasPrefix(arg, "--print=") {
			return name
		}
		if arg == "-r" || arg == "--require" || arg == "--import" || arg == "--loader" || arg == "--max-old-space-size" || arg == "--stack-size" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		script := filepath.ToSlash(arg)
		base := filepath.Base(script)
		if base == "codex" || strings.Contains(script, "/@openai/codex/") {
			return "codex"
		}
		if base == "claude" || strings.Contains(script, "/@anthropic-ai/claude-code/") {
			return "claude"
		}
		return name
	}
	return name
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
