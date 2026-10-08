package ui

import (
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel-rex/internal/backend"
	"strings"
	"testing"
	"time"
)

// UI tests use a model double, not private transport queues or emulator locks.
type modelInput struct {
	Op, Text, Kind string
	Key            uv.Key
	Mouse          uv.Mouse
}
type modelSession struct {
	state  backend.Frame
	queue  []modelInput
	scroll int
}

func (s *modelSession) ID() string                         { return "model" }
func (s *modelSession) State() backend.Frame               { return s.state }
func (s *modelSession) Snapshot(int, uint64) backend.Frame { return s.state }
func (s *modelSession) SyncRows(_ int, cols, rows int, cells *[]uv.Cell, versions *[]uint64) (backend.Frame, bool) {
	changed := len(*cells) != cols*rows
	if changed {
		*cells = make([]uv.Cell, cols*rows)
		*versions = make([]uint64, rows)
	}
	return s.state, changed
}
func (s *modelSession) SetScroll(n int) { s.scroll = n }
func (s *modelSession) SendKey(k uv.KeyEvent) {
	if p, ok := k.(uv.KeyPressEvent); ok {
		s.queue = append(s.queue, modelInput{Op: "key", Key: uv.Key(p)})
	}
}
func (s *modelSession) SendText(v string, _ bool) {
	s.queue = append(s.queue, modelInput{Op: "text", Text: v})
}
func (s *modelSession) SendMouse(kind string, m uv.Mouse) {
	s.queue = append(s.queue, modelInput{Op: "mouse", Kind: kind, Mouse: m})
}
func (s *modelSession) Resize(int, int)                                {}
func (s *modelSession) SetAppearance(string)                           {}
func (s *modelSession) Clear()                                         {}
func (s *modelSession) Directory() string                              { return s.state.Dir }
func (s *modelSession) Plain() string                                  { return s.state.Plain }
func (s *modelSession) Find(string, int) (*backend.SearchMatch, error) { return nil, nil }
func (s *modelSession) Copy(backend.Selection) (string, error)         { return "", nil }
func (s *modelSession) Activity() (bool, bool) {
	return !s.state.Exited, !s.state.Exited && s.state.Program == "codex"
}
func (s *modelSession) Subscribe() chan struct{}  { return make(chan struct{}) }
func (s *modelSession) Unsubscribe(chan struct{}) {}
func (s *modelSession) Done() <-chan struct{}     { return nil }
func (s *modelSession) Close()                    {}
func (s *modelSession) Detach()                   {}

var _ backend.Session = (*modelSession)(nil)

func testServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	service := backend.NewService(dir)
	done := make(chan error, 1)
	go func() { done <- service.Run() }()
	waitModel(t, func() bool { return service.Status(nil, time.Now()).Connected })
	t.Cleanup(func() {
		service.Shutdown()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	return dir
}
func waitModel(t *testing.T, ready func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("terminal model update timed out")
}
func waitScreen(t *testing.T, s backend.Session, text string) {
	t.Helper()
	waitModel(t, func() bool { return strings.Contains(s.Plain(), text) })
}

// A raw echoing PTY supplies real output through the public backend contract.
const outputCommand = "stty raw -echo; printf '\033]0;fixture-ready\007'; exec cat"

func outputSession(t *testing.T, history int) backend.Session {
	t.Helper()
	s, err := backend.NewSessionWithOptions(backend.SessionOptions{Directory: t.TempDir(), Command: []string{"/bin/sh", "-c", outputCommand}, HistoryLines: history})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	waitModel(t, func() bool { return s.State().Title == "fixture-ready" })
	return s
}
func feedOutput(t *testing.T, s backend.Session, text string) {
	t.Helper()
	title := fmt.Sprintf("fixture-%d", s.State().Revision)
	s.SendText(text+"\x1b]0;"+title+"\a", false)
	waitModel(t, func() bool { return s.State().Title == title })
}
func TestProgramIconsUseModelMetadata(t *testing.T) {
	for _, program := range []string{"codex", "claude"} {
		if got := programIcon(&modelSession{state: backend.Frame{Program: program}}); got != program {
			t.Fatalf("%s icon = %s", program, got)
		}
	}
}
