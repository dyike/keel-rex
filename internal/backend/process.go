package backend

import (
	"path/filepath"
	"strings"
)

// IsShellProgram recognizes idle shells, including login-shell process names.
func IsShellProgram(name string) bool {
	switch programName(name) {
	case "zsh", "bash", "fish", "sh", "dash", "ksh", "csh", "tcsh", "nu", "powershell", "pwsh", "cmd":
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
