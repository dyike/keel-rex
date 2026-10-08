package backend

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestLegacyServerRequiresExplicitShutdown(t *testing.T) {
	dir := t.TempDir()
	listener, err := net.Listen("unix", socketPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ops := make(chan string, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var req rpcRequest
			if json.NewDecoder(conn).Decode(&req) == nil {
				ops <- req.Op
				response := rpcResponse{Version: serverProtocol - 1, PID: 12345}
				if req.Op == "shutdown" {
					response = rpcResponse{}
				}
				json.NewEncoder(conn).Encode(response)
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	err = ensureServer(dir)
	if err == nil {
		t.Fatal("connected to incompatible server")
	}
	for _, want := range []string{fmt.Sprintf("server %d, client %d", serverProtocol-1, serverProtocol), "PID 12345", "-end-sessions -state-dir " + shellQuote(dir), "clear the saved workspace", "separate -state-dir"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if op := <-ops; op != "hello" {
		t.Fatalf("startup sent %q", op)
	}
	select {
	case op := <-ops:
		t.Fatalf("startup unexpectedly sent %q", op)
	default:
	}
	if err := endSessionServer(dir); err != nil {
		t.Fatal(err)
	}
	if op := <-ops; op != "shutdown" {
		t.Fatalf("explicit shutdown sent %q", op)
	}
}

func TestEnsureServerAcceptsCurrentProtocol(t *testing.T) {
	if err := ensureServer(testServer(t)); err != nil {
		t.Fatal(err)
	}
}

func testServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	done := make(chan error, 1)
	go func() { done <- runSessionServer(dir) }()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, e := callServer(dir, rpcRequest{Op: "hello"}); e == nil {
			t.Cleanup(func() {
				callServer(dir, rpcRequest{Op: "shutdown"})
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("server did not stop")
				}
			})
			return dir
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server startup timed out")
	return ""
}
func TestSessionServiceReattachAndAlternateScreen(t *testing.T) {
	dir := testServer(t)
	cwd := t.TempDir()
	result, e := callServer(dir, rpcRequest{Op: "create", Dir: cwd, Command: []string{"/bin/zsh", "-f"}})
	if e != nil {
		t.Fatal(e)
	}
	s, e := connectSession(dir, result.ID, cwd)
	if e != nil {
		t.Fatal(e)
	}
	s.text("export REX_REATTACH=retained; printf '%s_%s\\n' SERVICE READY\r", false)
	waitScreen(t, s, "SERVICE_READY")
	first := s.snapshot(0, 0)
	s.detach()
	<-s.done
	other, e := connectSession(dir, result.ID, cwd)
	if e != nil {
		t.Fatal(e)
	}
	defer other.detach()
	if got := other.snapshot(0, 0).PID; got != first.PID {
		t.Fatal("reattach created a new shell")
	}
	other.text("printf 'ENV:%s\\n' $REX_REATTACH\r", false)
	waitScreen(t, other, "ENV:retained")
	other.text("printf '\\033[?1049h\\033[2JALT_REATTACH'; read -k 1; printf '\\033[?1049l'\r", false)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !other.snapshot(0, 0).Alt; {
		time.Sleep(20 * time.Millisecond)
	}
	if !other.snapshot(0, 0).Alt {
		t.Fatal("alternate screen was not retained")
	}
	other.detach()
	<-other.done
	third, e := connectSession(dir, result.ID, cwd)
	if e != nil {
		t.Fatal(e)
	}
	defer third.detach()
	if !third.snapshot(0, 0).Alt || !strings.Contains(third.plain(), "ALT_REATTACH") {
		t.Fatal("alternate screen lost on attach")
	}
	third.text("x", false)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if !third.snapshot(0, 0).Alt {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("alternate screen did not restore")
}
func TestDetachedViewCanStillEndItsSession(t *testing.T) {
	dir := testServer(t)
	s, e := connectSession(dir, "", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	id := s.remote.id
	s.detach()
	<-s.done
	if _, e = callServer(dir, rpcRequest{Op: "frame", ID: id}); e != nil {
		t.Fatal("detaching ended the session")
	}
	s.close()
	s.close()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if _, e = callServer(dir, rpcRequest{Op: "frame", ID: id}); e != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("explicit closure after detaching leaked its session")
}
