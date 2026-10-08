// Package backend owns PTYs, terminal state, session transport and persistence.
// It has no dependency on the window, renderer or UI event loop.
package backend

import (
	"encoding/json"
	uv "github.com/charmbracelet/ultraviolet"
	"time"
)

// Session is the terminal model used by a view. Both local PTYs and attached
// server sessions satisfy the same contract; their locks and transport are private.
type Session interface {
	ID() string
	State() Frame
	Snapshot(scroll int, since uint64) Frame
	SyncRows(scroll, cols, rows int, cells *[]uv.Cell, versions *[]uint64) (Frame, bool)
	SetScroll(lines int)
	SendKey(uv.KeyEvent)
	SendText(text string, paste bool)
	SendMouse(kind string, mouse uv.Mouse)
	Resize(cols, rows int)
	SetAppearance(string)
	Clear()
	Directory() string
	Plain() string
	Find(query string, after int) (*SearchMatch, error)
	Copy(Selection) (string, error)
	Activity() (open, running bool)
	Subscribe() chan struct{}
	Unsubscribe(chan struct{})
	Done() <-chan struct{}
	Close()
	Detach()
}

// WorkspaceService is the application boundary for session creation, layout
// storage and service lifecycle. The UI can receive another implementation.
type WorkspaceService interface {
	Ensure() error
	Open(id, cwd string) (Session, error)
	LoadLayout() (json.RawMessage, error)
	SaveLayout(json.RawMessage) error
	Status([]Session, time.Time) HostStatus
	Shutdown() error
}

// Service is a connection to a persistent server, or a local session factory
// when its directory is empty. It never owns windows or UI state.
type Service struct{ dir string }

func NewService(dir string) *Service { return &Service{dir: dir} }
func (s *Service) Ensure() error {
	if s.dir == "" {
		return nil
	}
	return ensureServer(s.dir)
}
func (s *Service) Open(id, cwd string) (Session, error) {
	if s.dir == "" {
		return newSession(cwd)
	}
	return connectSession(s.dir, id, cwd)
}
func (s *Service) LoadLayout() (json.RawMessage, error) {
	if s.dir == "" {
		return nil, nil
	}
	r, err := callServer(s.dir, rpcRequest{Op: "layout"})
	return r.Layout, err
}
func (s *Service) SaveLayout(data json.RawMessage) error {
	if s.dir == "" {
		return nil
	}
	_, err := callServer(s.dir, rpcRequest{Op: "layout", Layout: data})
	return err
}
func (s *Service) Status(sessions []Session, started time.Time) HostStatus {
	return readHostStatus(s.dir, sessions, started)
}
func (s *Service) Shutdown() error { return endSessionServer(s.dir) }
func (s *Service) Run() error      { return runSessionServer(s.dir) }
func StateDirectory() string       { return stateDirectory() }

// NewSession opens a local PTY. Desktop views usually use Service.Open.
func NewSession(dir string, command ...string) (Session, error) { return newSession(dir, command...) }
func (s *session) ID() string {
	if s.remote != nil {
		return s.remote.id
	}
	return ""
}
func (s *session) State() Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remote != nil {
		return s.frame
	}
	return s.frameState(0)
}
func (s *session) Snapshot(scroll int, since uint64) Frame { return s.snapshot(scroll, since) }
func (s *session) SyncRows(scroll, cols, rows int, cells *[]uv.Cell, versions *[]uint64) (Frame, bool) {
	return s.syncRows(scroll, cols, rows, cells, versions)
}
func (s *session) SetScroll(lines int) {
	if s.remote != nil {
		s.remote.setScroll(lines)
	}
}
func (s *session) SendKey(k uv.KeyEvent)                 { s.send(k) }
func (s *session) SendText(text string, paste bool)      { s.text(text, paste) }
func (s *session) SendMouse(kind string, mouse uv.Mouse) { s.mouse(kind, mouse) }
func (s *session) Resize(cols, rows int)                 { s.resize(cols, rows) }
func (s *session) SetAppearance(value string)            { s.syncAppearance(value) }
func (s *session) Clear()                                { s.clearScreen() }
func (s *session) Directory() string                     { return s.directory() }
func (s *session) Plain() string                         { return s.plain() }
func (s *session) Find(query string, after int) (*SearchMatch, error) {
	if s.remote != nil {
		r, err := callServer(s.remote.dir, rpcRequest{Op: "find", ID: s.remote.id, Text: query, After: after})
		return r.Match, err
	}
	return s.search(query, after), nil
}
func (s *session) Copy(sel Selection) (string, error) { return s.selectionText(sel) }
func (s *session) Activity() (bool, bool)             { s.refreshLocalProgram(); return sessionActivity(s) }
func (s *session) refreshLocalProgram() {
	if s.remote == nil {
		s.refreshProgram()
	}
}
func (s *session) Subscribe() chan struct{}     { return s.subscribe() }
func (s *session) Unsubscribe(ch chan struct{}) { s.unsubscribe(ch) }
func (s *session) Done() <-chan struct{}        { return s.done }
func (s *session) Close()                       { s.close() }
func (s *session) Detach()                      { s.detach() }

var _ Session = (*session)(nil)
var _ WorkspaceService = (*Service)(nil)

// SessionOptions configures a PTY and its retained history.
type SessionOptions struct {
	Directory    string
	Command      []string
	HistoryLines int
}

func NewSessionWithOptions(options SessionOptions) (Session, error) {
	return newSessionWithOptions(options)
}

// OpenCommand creates a session with an explicit process instead of a login shell.
func (s *Service) OpenCommand(cwd string, command ...string) (Session, error) {
	if s.dir == "" {
		return newSession(cwd, command...)
	}
	r, err := callServer(s.dir, rpcRequest{Op: "create", Dir: cwd, Command: command})
	if err != nil {
		return nil, err
	}
	return connectSession(s.dir, r.ID, cwd)
}
