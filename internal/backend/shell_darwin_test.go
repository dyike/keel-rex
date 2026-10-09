package backend

import (
	"strings"
	"testing"
)

func TestAccountLoginShell(t *testing.T) {
	if shell := readAccountLoginShell(); shell == "" {
		t.Fatal("could not read macOS account login shell")
	}
}

func TestMacDefaultShell(t *testing.T) {
	for _, tt := range []struct {
		name, inherited, account, want string
	}{
		{"Finder bash", "/bin/bash", "/bin/zsh", "/bin/zsh"},
		{"Finder sh", "/bin/sh", "/bin/zsh", "/bin/zsh"},
		{"empty environment", "", "/bin/zsh", "/bin/zsh"},
		{"bash account", "/bin/bash", "/bin/bash", "/bin/bash"},
		{"fish account", "/bin/bash", "/opt/homebrew/bin/fish", "/opt/homebrew/bin/fish"},
		{"Nix environment", "/etc/profiles/per-user/test/bin/zsh", "/bin/zsh", "/etc/profiles/per-user/test/bin/zsh"},
		{"custom wrapper", "/tmp/shell.zsh", "/bin/zsh", "/tmp/shell.zsh"},
		{"lookup failed", "/bin/bash", "", "/bin/bash"},
		{"empty lookup failed", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := macDefaultShell(tt.inherited, func() string { return tt.account }); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultSessionShellEnvironment(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	s, err := newSession(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	want := shellCommand()[0]
	if s.cmd.Path != want {
		t.Fatalf("launched %q, want %q", s.cmd.Path, want)
	}
	var shellEnv string
	for _, env := range s.cmd.Env {
		if strings.HasPrefix(env, "SHELL=") {
			shellEnv = strings.TrimPrefix(env, "SHELL=")
		}
	}
	if shellEnv != want {
		t.Fatalf("initial SHELL=%q, want %q", shellEnv, want)
	}
	// Login scripts may deliberately replace SHELL (for example with Nix's
	// per-user zsh); verify startup without overriding that customization.
	s.text("printf 'REX_%s\\n' SHELL_READY\r", false)
	waitScreen(t, s, "REX_SHELL_READY")
}
