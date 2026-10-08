package backend

import (
	"bufio"
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
	"time"
)

// readyEnv names the fd a server started by ensureServer reports listening on.
const readyEnv = "KEEL_REX_READY_FD"

const serverProtocol = 3

type rpcRequest struct {
	Op, ID, Dir, Text, Kind string
	Cols, Rows, Scroll      int
	After                   int
	Since                   uint64
	Paste                   bool
	Key                     uv.Key
	Mouse                   uv.Mouse
	Command                 []string
	Selection               *Selection      `json:",omitempty"`
	Layout                  json.RawMessage `json:",omitempty"`
}
type rpcResponse struct {
	Stats        *ServerStats    `json:",omitempty"`
	Match        *SearchMatch    `json:",omitempty"`
	Error        string          `json:",omitempty"`
	ID           string          `json:",omitempty"`
	Version, PID int             `json:",omitempty"`
	Frame        *Frame          `json:",omitempty"`
	Layout       json.RawMessage `json:",omitempty"`
	Text         string          `json:",omitempty"`
}

func stateDirectory() string {
	if dir := os.Getenv("KEEL_REX_DIR"); dir != "" {
		return dir
	}
	base, e := os.UserConfigDir()
	if e != nil {
		base = os.TempDir()
	}
	exe, _ := os.Executable()
	return filepath.Join(base, stateDirectoryName(exe, os.Getenv("KEEL_RUN_WATCH") != ""))
}

func stateDirectoryName(exe string, development bool) string {
	// keel run uses a temporary .app for its Dock icon. It is still a
	// development process and must reconnect to the development sessions.
	if development || filepath.Ext(exe) != "" || filepath.Base(filepath.Dir(exe)) != "MacOS" {
		return "Rex Keel Dev"
	}
	return "Rex Keel"
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
		return checkServerProtocol(dir, r)
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
	if configured := os.Getenv("KEEL_REX_SERVER"); configured != "" {
		exe = configured
	}
	cmd := exec.Command(exe, "-server", "-state-dir", dir)
	cmd.Stdout, cmd.Stderr = log, log
	if e = startSessionServer(cmd); e != nil {
		return e
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var r rpcResponse
		if r, e = callServer(dir, rpcRequest{Op: "hello"}); e == nil {
			return checkServerProtocol(dir, r)
		}
		time.Sleep(30 * time.Millisecond)
	}
	return fmt.Errorf("session server did not start: %w", e)
}

func checkServerProtocol(dir string, r rpcResponse) error {
	if r.Version == serverProtocol {
		return nil
	}
	return fmt.Errorf("session server protocol differs (server %d, client %d; PID %d); to end all sessions and clear the saved workspace, run:\n  go run . -end-sessions -state-dir %s\nthen restart the application; to keep existing sessions, use a separate -state-dir", r.Version, serverProtocol, r.PID, shellQuote(dir))
}

// Shutdown deliberately bypasses the protocol check so an upgraded client
// can end sessions owned by an older server without opening a window.
func endSessionServer(dir string) error {
	_, e := callServer(dir, rpcRequest{Op: "shutdown"})
	return e
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
	if e = lockSessionServer(lock); e != nil {
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
	notifySessionServerReady()
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
	dec := json.NewDecoder(conn)
	if e := dec.Decode(&req); e != nil {
		return
	}
	if req.Op == "watch" {
		srv.mu.Lock()
		s := srv.sessions[req.ID]
		srv.mu.Unlock()
		if s == nil {
			json.NewEncoder(conn).Encode(rpcResponse{Error: "session unavailable"})
			return
		}
		srv.watch(conn, dec, s, req.Scroll)
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
	case "copy":
		if req.Selection == nil {
			return fail(errors.New("missing terminal selection"))
		}
		text, err := s.selectionText(*req.Selection)
		if err != nil {
			return fail(err)
		}
		return rpcResponse{Text: text}
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

// watch sends a window the session's screen, and an update whenever it
// changes, until the window goes. The window sends the scroll position it
// shows over the same connection.
func (srv *sessionServer) watch(conn net.Conn, dec *json.Decoder, s *session, scroll int) {
	conn.SetDeadline(time.Time{})
	changed := s.subscribe()
	defer s.unsubscribe(changed)
	scrolls := make(chan int, 1)
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			var req rpcRequest
			if dec.Decode(&req) != nil {
				return
			}
			select {
			case <-scrolls:
			default:
			}
			scrolls <- req.Scroll
		}
	}()
	w := bufio.NewWriterSize(conn, 64<<10)
	tick := time.NewTicker(500 * time.Millisecond) // the program and idle state change without output
	defer tick.Stop()
	var st watchState
	var screen screenCopy
	var scratch []byte
	var last time.Time
	for {
		s.refreshProgram()
		s.mu.Lock()
		s.copyScreen(&screen, &st, scroll)
		s.mu.Unlock()
		msg := encodeUpdate(&screen, &st, &scratch)
		if msg != nil {
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, e := w.Write(msg); e != nil || w.Flush() != nil {
				return
			}
			last = time.Now()
		}
		select {
		case <-gone:
			return
		case scroll = <-scrolls:
		case <-changed:
			// Output that keeps coming goes out at most every 16 ms, as a
			// 60 Hz display shows it; the first change after a pause at once.
			if wait := 16*time.Millisecond - time.Since(last); wait > 0 {
				time.Sleep(wait)
			}
		case <-tick.C:
		}
	}
}
