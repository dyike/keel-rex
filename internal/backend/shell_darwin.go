package backend

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"time"
)

var accountLoginShell = sync.OnceValue(readAccountLoginShell)

func defaultShell() string {
	return macDefaultShell(os.Getenv("SHELL"), accountLoginShell)
}

func macDefaultShell(inherited string, accountShell func() string) string {
	// Finder-launched apps may inherit macOS's generic bash/sh environment.
	// Keep custom paths (including Nix shells and benchmark wrappers), but
	// resolve generic defaults from the user's account instead.
	if inherited == "" || inherited == "/bin/bash" || inherited == "/bin/sh" {
		if shell := accountShell(); shell != "" {
			return shell
		}
	}
	return inherited
}

func readAccountLoginShell() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/bin/dscl", "/Search", "-read", "/Users/"+u.Username, "UserShell").Output()
	if err != nil {
		return ""
	}
	key, value, ok := strings.Cut(strings.TrimSpace(string(output)), ":")
	if !ok || key != "UserShell" {
		return ""
	}
	shell := strings.TrimSpace(value)
	info, err := os.Stat(shell)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return ""
	}
	return shell
}
