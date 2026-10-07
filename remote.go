package main

import (
	"sync"
	"time"
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
	identityAt                   time.Time
	identityPID                  int
	identityRaw, identityProgram string
}

func connectSession(dir, id, cwd string) (*session, error) {
	if id == "" {
		r, e := callServer(dir, rpcRequest{Op: "create", Dir: cwd})
		if e != nil {
			return nil, e
		}
		id = r.ID
	}
	r, e := callServer(dir, rpcRequest{Op: "frame", ID: id})
	if e != nil {
		return nil, e
	}
	s := &session{cwd: cwd, title: r.Frame.Title, cols: r.Frame.Cols, rows: r.Frame.Rows, frame: *r.Frame, done: make(chan struct{})}
	remote := &remoteSession{dir: dir, id: id, owner: s, wake: make(chan struct{}, 1), stop: make(chan struct{})}
	s.remote = remote
	go remote.run()
	return s, nil
}
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
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
func (r *remoteSession) finish(end bool) {
	r.once.Do(func() { close(r.stop) })
	if end {
		r.endOnce.Do(func() { go callServer(r.dir, rpcRequest{Op: "close", ID: r.id}) })
	}
}

func (r *remoteSession) run() {
	defer close(r.owner.done)
	ticker := time.NewTicker(45 * time.Millisecond)
	defer ticker.Stop()
	since := uint64(0)
	oldScroll := -1
	for {
		select {
		case <-r.stop:
			return
		case <-r.wake:
		case <-ticker.C:
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
		if scroll != oldScroll {
			since = 0
			oldScroll = scroll
		}
		response, e := callServer(r.dir, rpcRequest{Op: "frame", ID: r.id, Scroll: scroll, Since: since})
		if e != nil {
			r.failed(e.Error())
			return
		}
		f := response.Frame
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
		if f.Cells == nil {
			f.Cells = old.Cells
			f.Plain = old.Plain
		}
		changed := f.Revision != old.Revision || f.Title != old.Title || f.Program != old.Program || f.Exited != old.Exited || f.Bells != old.Bells || f.Scroll != old.Scroll
		s.frame = *f
		s.cwd = f.Dir
		s.title = f.Title
		s.exited = f.Exited
		s.mouseTracking = f.Mouse
		s.lastOutput = f.LastOutput
		s.bells = f.Bells
		s.program = f.Program
		s.mu.Unlock()
		since = f.Revision
		if changed {
			s.notify()
		}
	}
}
func (r *remoteSession) failed(message string) {
	r.finish(false)
	s := r.owner
	s.mu.Lock()
	s.exited = true
	s.frame.Program = ""
	s.frame.Exited = true
	s.frame.Plain += "\n[Session connection lost: " + message + "]\n"
	s.mu.Unlock()
	s.notify()
}
