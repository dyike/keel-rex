package backend

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

type remoteSession struct {
	dir, id                      string
	owner                        *session
	mu                           sync.Mutex
	queue                        []rpcRequest
	wake                         chan struct{}
	stop                         chan struct{}
	once                         sync.Once
	endOnce                      sync.Once
	scroll                       int
	watch                        net.Conn
	writeMu                      sync.Mutex
	identityAt                   time.Time
	identityPID                  int
	identityRaw, identityProgram string
}

// connectSession attaches to session id on the server in dir, creating one
// in cwd when id is empty. It returns once the first screen has arrived;
// updates then come as the screen changes.
func connectSession(dir, id, cwd string) (*session, error) {
	if id == "" {
		r, e := callServer(dir, rpcRequest{Op: "create", Dir: cwd})
		if e != nil {
			return nil, e
		}
		id = r.ID
	}
	conn, e := net.DialTimeout("unix", socketPath(dir), time.Second)
	if e != nil {
		return nil, e
	}
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if e = json.NewEncoder(conn).Encode(rpcRequest{Op: "watch", ID: id}); e != nil {
		conn.Close()
		return nil, e
	}
	r := bufio.NewReaderSize(conn, 64<<10)
	var buf []byte
	first, e := readUpdate(r, &buf)
	if e != nil {
		conn.Close()
		if e.Error() == "EOF" || e.Error() == "unexpected EOF" {
			return nil, errSessionUnavailable
		}
		return nil, e
	}
	conn.SetDeadline(time.Time{})
	s := &session{cwd: cwd, done: make(chan struct{})}
	remote := &remoteSession{dir: dir, id: id, owner: s, wake: make(chan struct{}, 1), stop: make(chan struct{}), watch: conn}
	s.remote = remote
	remote.apply(first)
	s.cols, s.rows = first.frame.Cols, first.frame.Rows
	go remote.read(r, buf)
	go remote.run()
	return s, nil
}

var errSessionUnavailable = sessionError("session unavailable")

type sessionError string

func (e sessionError) Error() string { return string(e) }

func (r *remoteSession) enqueue(req rpcRequest) {
	r.mu.Lock()
	select {
	case <-r.stop:
		r.mu.Unlock()
		return
	default:
	}
	r.queue = append(r.queue, req)
	r.mu.Unlock()
	r.poke()
}

func (r *remoteSession) poke() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// setScroll asks the server for the screen scrolled back lines.
func (r *remoteSession) setScroll(lines int) {
	r.mu.Lock()
	changed := r.scroll != lines
	r.scroll = lines
	r.mu.Unlock()
	if changed {
		r.poke()
	}
}

func (r *remoteSession) finish(end bool) {
	r.once.Do(func() {
		close(r.stop)
		if r.watch != nil {
			r.watch.Close()
		}
	})
	if end {
		r.endOnce.Do(func() { go callServer(r.dir, rpcRequest{Op: "close", ID: r.id}) })
	}
}

// run sends the window's input and scroll position to the server.
func (r *remoteSession) run() {
	defer close(r.owner.done)
	sent := 0
	for {
		select {
		case <-r.stop:
			return
		case <-r.wake:
		}
		r.mu.Lock()
		queue := r.queue
		r.queue = nil
		scroll := r.scroll
		r.mu.Unlock()
		for _, req := range queue {
			req.ID = r.id
			if _, e := callServer(r.dir, req); e != nil {
				r.failed(e.Error())
				return
			}
		}
		if scroll != sent && r.watch != nil {
			r.writeMu.Lock()
			r.watch.SetWriteDeadline(time.Now().Add(3 * time.Second))
			e := json.NewEncoder(r.watch).Encode(rpcRequest{Scroll: scroll})
			r.writeMu.Unlock()
			if e != nil {
				r.failed(e.Error())
				return
			}
			sent = scroll
		}
	}
}

// read applies the server's updates until the connection ends.
func (r *remoteSession) read(br *bufio.Reader, buf []byte) {
	for {
		u, e := readUpdate(br, &buf)
		if e != nil {
			select {
			case <-r.stop:
			default:
				r.failed(e.Error())
			}
			return
		}
		r.apply(u)
	}
}

// apply puts an update into the session: its state into frame, its rows into
// the grid, each changed row with a new version for the window to redraw.
func (r *remoteSession) apply(u update) {
	f := u.frame
	if runtimeProgramName(f.Program) && !f.Exited {
		if r.identityRaw != f.Program || r.identityPID != f.PID || time.Since(r.identityAt) >= 750*time.Millisecond {
			r.identityRaw, r.identityPID = f.Program, f.PID
			r.identityProgram = identifyRemoteCLI(f.PID, f.Program)
			r.identityAt = time.Now()
		}
		f.Program = r.identityProgram
	}
	s := r.owner
	s.mu.Lock()
	old := s.frame
	if f.Cols != old.Cols || f.Rows != old.Rows || len(s.grid) != f.Cols*f.Rows {
		s.grid = make([]uv.Cell, f.Cols*f.Rows)
		for i := range s.grid {
			s.grid[i] = uv.EmptyCell
		}
		s.rowVersions = make([]uint64, f.Rows)
		for y := range s.rowVersions {
			s.gridVersion++
			s.rowVersions[y] = s.gridVersion
		}
	}
	for _, row := range u.rows {
		if row.y < 0 || row.y >= f.Rows {
			continue
		}
		if e := decodeRow(row.cells, s.grid[row.y*f.Cols:(row.y+1)*f.Cols]); e != nil {
			continue
		}
		s.gridVersion++
		s.rowVersions[row.y] = s.gridVersion
	}
	changed := len(u.rows) > 0 || f.Revision != old.Revision || f.Title != old.Title || f.Program != old.Program || f.Exited != old.Exited || f.Bells != old.Bells || f.Scroll != old.Scroll || f.Cursor != old.Cursor || f.CursorVisible != old.CursorVisible || f.History != old.History || f.HistoryStart != old.HistoryStart || f.HistoryEpoch != old.HistoryEpoch || f.Alt != old.Alt
	s.frame = f
	s.cwd = f.Dir
	s.title = f.Title
	s.exited = f.Exited
	s.mouseTracking = f.Mouse
	s.lastOutput = f.LastOutput
	s.bells = f.Bells
	s.program = f.Program
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

func (r *remoteSession) failed(message string) {
	s := r.owner
	s.mu.Lock()
	s.exited = true
	s.frame.Program = ""
	s.frame.Exited = true
	s.lost = "[Session connection lost: " + message + "]"
	s.gridVersion++
	for y := range s.rowVersions {
		s.rowVersions[y] = s.gridVersion
	}
	s.mu.Unlock()
	s.notify()
	// Publish the disconnected state before Done releases UI observers.
	r.finish(false)
}
