package ui

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/el"
)

type notificationModel struct {
	modelSession
	mu      sync.Mutex
	stopped chan struct{}
}

func (s *notificationModel) State() backend.Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}
func (s *notificationModel) Done() <-chan struct{} { return s.stopped }

func TestTerminalObserverSkipsHiddenScreenChanges(t *testing.T) {
	for _, name := range []string{"hidden cells", "visible cells", "program", "title", "directory", "bell", "exit", "activity"} {
		t.Run(name, func(t *testing.T) {
			before := backend.Frame{Program: "zsh", Dir: "/project", LastOutput: time.Now()}
			model := &notificationModel{stopped: make(chan struct{})}
			model.state = before
			term := &terminal{session: model}
			term.visible.Store(name == "visible cells")
			next := before
			next.Revision++
			switch name {
			case "program":
				next.Program = "codex"
			case "title":
				next.Title = "remote host"
			case "directory":
				next.Dir = "/other"
			case "bell":
				next.Bells++
			case "exit":
				next.Exited = true
			case "activity":
				before.LastOutput = time.Time{}
			}
			changed := make(chan struct{}, 1)
			posted := make(chan func(), 4)
			done := make(chan struct{})
			go func() {
				term.observe(changed, terminalNotice(before, time.Now()), func(fn func()) { posted <- fn })
				close(done)
			}()
			model.mu.Lock()
			model.state = next
			model.mu.Unlock()
			changed <- struct{}{}
			// Done also checks the final state. Joining the observer lets the
			// no-redraw assertion inspect every possible queued callback.
			close(model.stopped)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("observer did not stop")
			}
			want := 1
			if name == "hidden cells" {
				want = 0
			}
			if len(posted) != want {
				t.Fatalf("queued %d frames, want %d", len(posted), want)
			}
		})
	}
}

func TestHiddenTerminalReleasesPaintAndRestoresLatestScreen(t *testing.T) {
	// This fixture shapes system CJK text. Keep process-wide font/raster
	// caches out of the cell-versus-row pixel oracle used by other tests.
	if os.Getenv("REX_HIDDEN_TERMINAL_HELPER") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHiddenTerminalReleasesPaintAndRestoresLatestScreen$")
		cmd.Env = append(os.Environ(), "REX_HIDDEN_TERMINAL_HELPER=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hidden terminal regression: %v\n%s", err, output)
		}
		return
	}
	s := outputSession(t, 100)
	p := &pane{}
	other := &pane{}
	a := &app{prefs: defaultPreferences(), focused: p}
	p.term = &terminal{session: s, pane: p, owner: a}
	a.tabs = []*workspace{{root: p, focus: p}, {root: other, focus: other}}
	h := interactionView{view: &workspaceView{app: a, root: el.Root(a)}}
	feedOutput(t, s, "BEFORE_HIDE 中文\r\n")
	h.render()
	term := p.term
	term.hasSelection, term.anchor, term.caret = true, 0, 3
	if !term.visible.Load() || len(term.rowPaints) == 0 || len(term.cells) == 0 {
		t.Fatal("visible terminal did not prepare its screen")
	}
	a.activate(1)
	h.render()
	if term.visible.Load() || term.rowPaints != nil || term.cells != nil || term.rowVersions != nil || term.plain != "" {
		t.Fatal("hidden terminal retained its screen/paint buffers")
	}
	if !term.hasSelection || term.anchor != 0 || term.caret != 3 {
		t.Fatal("hiding a terminal lost its selection")
	}
	feedOutput(t, s, "AFTER_HIDE 中文\r\n")
	a.activate(0)
	h.render()
	if !term.visible.Load() || !strings.Contains(term.plain, "AFTER_HIDE 中文") {
		t.Fatalf("restored stale screen: %q", term.plain)
	}
	// Zooming a sibling in the same tab also hides this view.
	a.tabs[0].root = &pane{first: p, second: other, vertical: true, ratio: .5}
	a.tabs[0].zoom = true
	a.focused = other
	a.syncTerminalVisibility()
	if term.visible.Load() || term.rowPaints != nil {
		t.Fatal("zoomed-out sibling retained rendering resources")
	}
}

// This is the periodic metadata check used by local/ephemeral sessions, not
// a model double whose Snapshot already happens to return metadata only.
func BenchmarkActivityMetadata(b *testing.B) {
	s, err := backend.NewSession(b.TempDir(), "/bin/sh", "-c", "exec sleep 60")
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	s.Resize(127, 31)
	p := &pane{term: &terminal{session: s}}
	a := &app{focused: p, tabs: []*workspace{{root: p, focus: p}}}
	a.checkActivity()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.checkActivity()
	}
}
