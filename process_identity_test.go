package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIIdentityUsesEntryPoint(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"node", []string{"node", "/Users/user/.npm-global/bin/codex"}, "codex"},
		{"node", []string{"node", "/opt/node_modules/@openai/codex/bin/codex.js"}, "codex"},
		{"node", []string{"node", "--require", "source-map-support", "/opt/node_modules/@anthropic-ai/claude-code/cli.js"}, "claude"},
		{"bun", []string{"bun", "/usr/local/bin/claude"}, "claude"},
		{"node", []string{"node", "/app/server.js", "codex"}, "node"},
		{"node", []string{"node", "-e", "require('@openai/codex')"}, "node"},
		{"node", []string{"node", "/app/codex.js"}, "node"},
		{"node", []string{"node", "/app/server.js", "/opt/node_modules/@openai/codex/bin/codex.js"}, "node"},
	}
	for _, c := range cases {
		got := identifyCLI(c.name, c.args)
		if got != c.want {
			t.Errorf("%v: got %q want %q", c.args, got, c.want)
		}
		s := &session{program: got}
		if got == "codex" && programIcon(s) != "codex" {
			t.Fatal("Codex mapped to wrong icon")
		}
		if got == "claude" && programIcon(s) != "claude" {
			t.Fatal("Claude mapped to wrong icon")
		}
	}
}

func TestRemoteCLIIdentificationWorksWithOldServerRuntimeName(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// The shell is the foreground process-group leader. Its CLI entry path
	// remains in ps even while a child runs, just like a Node CLI wrapper.
	s, err := newSession(dir, "/bin/sh", script)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if got := identifyRemoteCLI(s.cmd.Process.Pid, "node"); got != "codex" {
		t.Fatalf("live foreground CLI: got %q", got)
	}
}
