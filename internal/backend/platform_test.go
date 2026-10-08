package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionServerLockIsExclusiveAndReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.lock")
	a, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err = lockSessionServer(a); err != nil {
		t.Fatal(err)
	}
	if err = lockSessionServer(b); err == nil {
		t.Fatal("second service acquired the same lock")
	}
	a.Close()
	if err = lockSessionServer(b); err != nil {
		t.Fatal("lock not released after service exited:", err)
	}
}

func TestShellNamesIncludeWindowsExecutables(t *testing.T) {
	for _, name := range []string{"pwsh.exe", "PowerShell.EXE", "cmd.exe", "bash", "-zsh", "fish"} {
		if !IsShellProgram(name) {
			t.Fatalf("%q not recognized as an idle shell", name)
		}
	}
}
