package backend

import (
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"
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
	s := &session{remote: &remoteSession{}, frame: Frame{Program: "codex"}}
	status := readHostStatus(dir, []Session{s}, time.Now())
	if !status.Connected || !status.WindowOnly || status.Open != 1 || status.Running != 1 || status.PID != os.Getpid() || status.Uptime == "" {
		t.Fatalf("legacy server fallback: %+v", status)
	}
	listener.Close()
	status = readHostStatus(dir, []Session{s}, time.Now())
	if status.Connected {
		t.Fatal("disconnected server reported connected")
	}
}

func TestHostActivityExcludesExitedSessions(t *testing.T) {
	for _, s := range []*session{{exited: true, program: "codex"}, {closed: true, program: "codex"}, {remote: &remoteSession{}, frame: Frame{Exited: true, Program: "codex"}}} {
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
