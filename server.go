package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const serverProtocol = 1

type rpcRequest struct {
	Op, ID, Dir, Text, Kind string
	Cols, Rows, Scroll      int
	After                   int
	Since                   uint64
	Paste                   bool
	Key                     uv.Key
	Mouse                   uv.Mouse
	Command                 []string
	Layout                  json.RawMessage `json:",omitempty"`
}
type rpcResponse struct {
	Stats        *serverStats    `json:",omitempty"`
	Match        *searchMatch    `json:",omitempty"`
	Error        string          `json:",omitempty"`
	ID           string          `json:",omitempty"`
	Version, PID int             `json:",omitempty"`
	Frame        *screenFrame    `json:",omitempty"`
	Layout       json.RawMessage `json:",omitempty"`
}

func stateDirectory() string {
	if dir := os.Getenv("KEEL_REX_DIR"); dir != "" {
		return dir
	}
	base, e := os.UserConfigDir()
	if e != nil {
		base = os.TempDir()
	}
	name := "Rex Keel"
	exe, _ := os.Executable()
	if filepath.Ext(exe) != "" || filepath.Base(filepath.Dir(exe)) != "MacOS" {
		name = "Rex Keel Dev"
	}
	return filepath.Join(base, name)
}
func socketPath(dir string) string {
	p := filepath.Join(dir, "sessions.sock")
	if len(p) > 95 { // Darwin's Unix socket pathname is limited to 104 bytes.
		sum := uint64(14695981039346656037)
		for _, v := range []byte(dir) {
			sum = (sum ^ uint64(v)) * 1099511628211
		}
		p = filepath.Join(os.TempDir(), fmt.Sprintf("keel-rex-%d-%x.sock", os.Getuid(), sum))
	}
	return p
}
func callServer(dir string, req rpcRequest) (rpcResponse, error) {
	var response rpcResponse
	conn, e := net.DialTimeout("unix", socketPath(dir), time.Second)
	if e != nil {
		return response, e
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if e = json.NewEncoder(conn).Encode(req); e != nil {
		return response, e
	}
	if e = json.NewDecoder(conn).Decode(&response); e != nil {
		return response, e
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}
func ensureServer(dir string) error {
	if r, e := callServer(dir, rpcRequest{Op: "hello"}); e == nil {
		if r.Version != serverProtocol {
			return fmt.Errorf("session server protocol differs; end sessions explicitly before upgrading")
		}
		return nil
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	log, e := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := exec.Command(exe, "-server", "-state-dir", dir)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	go cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, e = callServer(dir, rpcRequest{Op: "hello"}); e == nil {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return fmt.Errorf("session server did not start: %w", e)
}
func runSessionServer(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(filepath.Join(dir, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("session server already running: %w", e)
	}
	path := socketPath(dir)
	os.Remove(path)
	listener, e := net.Listen("unix", path)
	if e != nil {
		return e
	}
	defer listener.Close()
	defer os.Remove(path)
	os.Chmod(path, 0600)
	srv := &sessionServer{sessions: map[string]*session{}, dir: dir, listener: listener, started: time.Now()}
	for {
		conn, e := listener.Accept()
		if e != nil {
			return nil
		}
		go srv.handle(conn)
	}
}

type sessionServer struct {
	started  time.Time
	mu       sync.Mutex
	sessions map[string]*session
	dir      string
	listener net.Listener
}

func (srv *sessionServer) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var req rpcRequest
	if e := json.NewDecoder(conn).Decode(&req); e != nil {
		return
	}
	response := srv.dispatch(req)
	json.NewEncoder(conn).Encode(response)
	if req.Op == "shutdown" && response.Error == "" {
		srv.listener.Close()
	}
}
func (srv *sessionServer) dispatch(req rpcRequest) rpcResponse {
	fail := func(e error) rpcResponse { return rpcResponse{Error: e.Error()} }
	switch req.Op {
	case "hello":
		return rpcResponse{Version: serverProtocol, PID: os.Getpid()}
	case "stats":
		stats := srv.stats()
		return rpcResponse{Version: serverProtocol, PID: os.Getpid(), Stats: &stats}
	case "layout":
		srv.mu.Lock()
		defer srv.mu.Unlock()
		file := filepath.Join(srv.dir, "layout.json")
		if len(req.Layout) > 0 && string(req.Layout) != "null" {
			if !json.Valid(req.Layout) {
				return fail(errors.New("invalid layout"))
			}
			if e := atomicJSON(file, req.Layout); e != nil {
				return fail(e)
			}
		}
		b, _ := os.ReadFile(file)
		return rpcResponse{Layout: b}
	case "create":
		s, e := newSession(req.Dir, req.Command...)
		if e != nil {
			return fail(e)
		}
		var token [12]byte
		rand.Read(token[:])
		id := hex.EncodeToString(token[:])
		srv.mu.Lock()
		srv.sessions[id] = s
		srv.mu.Unlock()
		return rpcResponse{ID: id}
	case "shutdown":
		srv.mu.Lock()
		for _, s := range srv.sessions {
			s.close()
		}
		srv.sessions = map[string]*session{}
		srv.mu.Unlock()
		atomicJSON(filepath.Join(srv.dir, "layout.json"), []byte(`{}`))
		return rpcResponse{}
	}
	srv.mu.Lock()
	s := srv.sessions[req.ID]
	if req.Op == "close" {
		delete(srv.sessions, req.ID)
	}
	srv.mu.Unlock()
	if s == nil {
		return fail(errors.New("session unavailable"))
	}
	switch req.Op {
	case "frame":
		s.refreshProgram()
		f := s.snapshot(req.Scroll, req.Since)
		return rpcResponse{Frame: &f}
	case "find":
		return rpcResponse{Match: s.search(req.Text, req.After)}
	case "text":
		s.text(req.Text, req.Paste)
	case "key":
		s.send(uv.KeyPressEvent(req.Key))
	case "mouse":
		s.mouse(req.Kind, req.Mouse)
	case "resize":
		s.resize(min(500, max(2, req.Cols)), min(300, max(2, req.Rows)))
	case "theme":
		s.syncAppearance(req.Kind)
	case "clear":
		s.clearScreen()
	case "close":
		s.close()
	default:
		return fail(errors.New("unknown session operation"))
	}
	return rpcResponse{}
}
func atomicJSON(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	temp, e := os.CreateTemp(filepath.Dir(path), ".state-*")
	if e != nil {
		return e
	}
	defer os.Remove(temp.Name())
	if e = temp.Chmod(0600); e == nil {
		_, e = temp.Write(b)
	}
	if e == nil {
		e = temp.Sync()
	}
	closeErr := temp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), path)
}
