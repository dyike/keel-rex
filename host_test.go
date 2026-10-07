package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
)

func TestHostServerStatsIncludeUnattachedSessions(t *testing.T) {
	dir := testServer(t)
	idle, err := callServer(dir, rpcRequest{Op: "create", Dir: t.TempDir(), Command: []string{"/bin/zsh", "-f"}})
	if err != nil {
		t.Fatal(err)
	}
	running, err := callServer(dir, rpcRequest{Op: "create", Dir: t.TempDir(), Command: []string{"/bin/sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	status := readHostStatus(dir, nil, time.Now())
	if !status.Connected || status.WindowOnly || status.PID != os.Getpid() || status.Open != 2 || status.Running != 1 || status.Uptime == "" {
		t.Fatalf("server statistics: %+v", status)
	}
	if _, err := callServer(dir, rpcRequest{Op: "close", ID: running.ID}); err != nil {
		t.Fatal(err)
	}
	status = readHostStatus(dir, nil, time.Now())
	if status.Open != 1 || status.Running != 0 {
		t.Fatalf("closed session counted: %+v", status)
	}
	if _, err := callServer(dir, rpcRequest{Op: "close", ID: idle.ID}); err != nil {
		t.Fatal(err)
	}
}

func TestHostOldServerPreservesConnectionAndLabelsWindowCounts(t *testing.T) {
	dir := t.TempDir()
	listener, err := net.Listen("unix", socketPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var req rpcRequest
			json.NewDecoder(conn).Decode(&req)
			reply := rpcResponse{Error: "session unavailable"}
			if req.Op == "hello" {
				reply = rpcResponse{Version: serverProtocol, PID: os.Getpid()}
			}
			json.NewEncoder(conn).Encode(reply)
			conn.Close()
		}
	}()
	s := &session{remote: &remoteSession{}, frame: screenFrame{Program: "codex"}}
	status := readHostStatus(dir, []*session{s}, time.Now())
	if !status.Connected || !status.WindowOnly || status.Open != 1 || status.Running != 1 || status.PID != os.Getpid() || status.Uptime == "" {
		t.Fatalf("legacy server fallback: %+v", status)
	}
	listener.Close()
	status = readHostStatus(dir, []*session{s}, time.Now())
	if status.Connected {
		t.Fatal("disconnected server reported connected")
	}
}

func TestHostActivityExcludesExitedSessions(t *testing.T) {
	for _, s := range []*session{{exited: true, program: "codex"}, {closed: true, program: "codex"}, {remote: &remoteSession{}, frame: screenFrame{Exited: true, Program: "codex"}}} {
		if open, running := sessionActivity(s); open || running {
			t.Fatal("ended session counted as active")
		}
	}
	for _, program := range []string{"zsh", "bash", "sh", "fish", "-zsh", ""} {
		if open, running := sessionActivity(&session{program: program}); !open || running {
			t.Fatalf("idle shell %q counted as running", program)
		}
	}
}

func TestHostPanelVisualFixture(t *testing.T) {
	dir := os.Getenv("REX_HOST_QA_DIR")
	if dir == "" {
		t.Skip("host panel screenshots not requested")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	defer theme.Apply(theme.Light())
	for _, appearance := range []string{"light", "dark"} {
		if appearance == "dark" {
			theme.Apply(theme.Dark())
		} else {
			theme.Apply(theme.Light())
		}
		for _, state := range []string{"connected", "legacy", "local", "disconnected"} {
			a := &app{prefs: preferences{Appearance: appearance}, hostDetails: hostDetails{Model: "Mac mini", Chip: "Apple M4", Memory: "24 GB", System: "macOS 27.0.1", User: "ityike"}, hostStatus: hostStatus{Loaded: true, Connected: true, PID: 33602, Uptime: "16m", Open: 4, Running: 2}}
			switch state {
			case "legacy":
				a.hostStatus.WindowOnly = true
			case "local":
				a.hostStatus.Local = true
			case "disconnected":
				a.hostStatus.Connected = false
			}
			for _, scale := range []float32{1, 2} {
				root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().P(28).Bg(a.colors().top).Child(a.hostPanel(300)) }))
				name := fmt.Sprintf("host-%s-%s-%gx.png", appearance, state, scale)
				if err := window.ScreenshotAtScale(root, 356, 440, scale, filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
