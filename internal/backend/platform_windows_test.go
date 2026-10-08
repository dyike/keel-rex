package backend

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The test binary can also act as a detached session service, exercising the
// same launcher and RPC handshake as go run rather than an in-process stub.
func TestMain(m *testing.M) {
	server := flag.Bool("server", false, "Run Windows test session service")
	dir := flag.String("state-dir", "", "Test session data directory")
	flag.Parse()
	if *server {
		if err := runSessionServer(*dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

func waitWindowsScreen(t *testing.T, s Session, text string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.Plain(), text) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missing %q in Windows terminal:\n%s", text, s.Plain())
}

func TestWindowsConPTYInputResizeAndClose(t *testing.T) {
	s, err := newSession(t.TempDir(), "cmd.exe", "/d", "/q")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	s.text("echo WINDOWS_INPUT_OK\r", false)
	waitWindowsScreen(t, s, "WINDOWS_INPUT_OK")
	s.resize(100, 30)
	if state := s.State(); state.Cols != 100 || state.Rows != 30 {
		t.Fatalf("resize: %+v", state)
	}
	s.text("exit\r", false)
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("exited ConPTY never reached EOF")
	}
	if !s.State().Exited {
		t.Fatal("session did not record process exit")
	}
}

func TestWindowsConPTYUnicodeAndExitStatus(t *testing.T) {
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newSession(t.TempDir(), path, "-NoLogo", "-NoProfile", "-Command", "[Console]::WriteLine('WINDOWS_中文_READY'); exit 7")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	waitWindowsScreen(t, s, "WINDOWS_中文_READY")
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("PowerShell session did not exit")
	}
	if !strings.Contains(s.Plain(), "exit status 7") {
		t.Fatalf("exit status missing: %s", s.Plain())
	}
}

func TestWindowsPersistentServiceReconnects(t *testing.T) {
	dir, cwd := t.TempDir(), t.TempDir()
	t.Setenv("KEEL_REX_SERVER", "")
	if err := ensureServer(dir); err != nil {
		t.Fatal(err)
	}
	defer endSessionServer(dir)
	before, err := callServer(dir, rpcRequest{Op: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := connectSession(dir, "", cwd)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	s.text("echo WINDOWS_RECONNECT_OK\r", false)
	waitWindowsScreen(t, s, "WINDOWS_RECONNECT_OK")
	id := s.ID()
	s.detach()
	if err := ensureServer(dir); err != nil {
		t.Fatal(err)
	}
	again, err := connectSession(dir, id, cwd)
	if err != nil {
		t.Fatal(err)
	}
	defer again.close()
	waitWindowsScreen(t, again, "WINDOWS_RECONNECT_OK")
	after, err := callServer(dir, rpcRequest{Op: "hello"})
	if err != nil || before.PID != after.PID {
		t.Fatalf("restarted existing service: %+v %+v %v", before, after, err)
	}
}
