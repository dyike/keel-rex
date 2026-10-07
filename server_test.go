package main

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
func TestWorkspaceRestoresOrderNamesSplitsAndFocus(t *testing.T) {
	dir := testServer(t)
	cwd := t.TempDir()
	a := newAppAt(cwd, dir)
	a.newTabAt(cwd)
	a.tabs[a.active].customTitle = true
	a.tabs[a.active].title = "Renamed work"
	a.split(true)
	a.tabs[a.active].root.ratio = .63
	focus := a.focused.id
	sid := a.focused.term.session.remote.id
	a.moveTab(a.tabs[a.active], 0)
	a.persist(true)
	a.close()
	b := newAppAt(cwd, dir)
	defer b.close()
	if b.active != 0 || b.tabs[0].title != "Renamed work" || !b.tabs[0].customTitle || b.tabs[0].root.ratio != .63 || b.focused.id != focus {
		state, _ := json.Marshal(b.workspaceState())
		t.Fatalf("workspace changed: %s", state)
	}
	if b.focused.term.session.remote.id != sid {
		t.Fatal("restored layout did not reattach its session")
	}
}
func TestFuzzyCommandsAndTabNavigation(t *testing.T) {
	a := newApp(t.TempDir())
	defer a.close()
	first := a.tabs[0]
	first.focus = a.focused
	a.newTab()
	second := a.tabs[1]
	secondFocus := a.focused
	a.activate(0)
	if a.focused != first.focus {
		t.Fatal("tab lost its remembered focus")
	}
	a.activate(1)
	if a.focused != secondFocus {
		t.Fatal("second tab lost focus")
	}
	a.moveTab(second, 0)
	if a.tabs[0] != second || a.active != 0 {
		t.Fatal("reorder changed active workspace")
	}
	a.query = "spl r"
	rows := a.paletteResults()
	if len(rows) == 0 || rows[0].ID != "split-right" {
		t.Fatalf("unexpected fuzzy result: %+v", rows)
	}
	a.query = "nonexistent command"
	if len(a.paletteResults()) != 0 {
		t.Fatal("nonmatching commands shown")
	}
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
