package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel/ui/el"
)

func TestTerminalOwnsSystemTabAndKeepsButtonNavigation(t *testing.T) {
	remote := &modelSession{}
	s := remote
	s.state = backend.Frame{Cols: 40, Rows: 6}
	p := &pane{}
	a := &app{focused: p, prefs: defaultPreferences()}
	term := &terminal{session: s, pane: p, owner: a}
	p.term = term
	focusFirst := false
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		if focusFirst {
			cx.Focus("first")
			focusFirst = false
		}
		return el.Div().Child(el.Div().ID("first").Focusable(true).OnClick(func() {}).Child(el.Text("First")), el.Div().ID("second").Focusable(true).OnClick(func() {}).Child(el.Text("Second")), el.Widget(term).Grow())
	}))
	var router input.Router
	var ops op.Ops
	render := func() {
		for range 3 {
			ops.Reset()
			root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(320, 180))})
			router.Frame(&ops)
		}
	}
	render()
	router.Source().Execute(key.FocusCmd{Tag: term})
	render()
	for _, mods := range []key.Modifiers{0, 0, 0, key.ModShift} {
		router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, Modifiers: mods, State: key.Press}})
		if _, handled := router.WakeupTime(); !handled {
			t.Fatal("terminal Tab was unhandled and would move platform focus to a button")
		}
		render()
		if !router.Source().Focused(term) {
			t.Fatal("Tab stole terminal focus")
		}
	}
	tabs := 0
	for _, req := range remote.queue {
		if req.Op == "key" && req.Key.Code == uv.KeyTab {
			tabs++
			if tabs == 4 && req.Key.Mod != uv.ModShift {
				t.Fatal("Shift+Tab lost its modifier")
			}
		}
	}
	if tabs != 4 {
		t.Fatalf("terminal received %d tabs, want 4", tabs)
	}
	focusFirst = true
	render()
	if router.Source().Focused(term) {
		t.Fatal("could not focus a regular button")
	}
	router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, State: key.Press}})
	if _, handled := router.WakeupTime(); handled {
		t.Fatal("terminal intercepted Tab while a regular button had focus")
	}
}

func TestTerminalTabCompletesShellFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "completion-target"), []byte("COMPLETION_OK\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := backend.NewSession(dir, "/bin/zsh", "-f")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SendText("bindkey '^I' expand-or-complete; printf 'READY_COMPLETION\\n'\r", false)
	waitScreen(t, s, "READY_COMPLETION")
	p := &pane{}
	a := &app{focused: p, prefs: defaultPreferences()}
	term := &terminal{session: s, pane: p, owner: a}
	p.term = term
	var router input.Router
	var ops op.Ops
	render := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(640, 320))}
		term.Layout(gtx)
		gtx.Execute(key.FocusCmd{Tag: term})
		router.Frame(&ops)
	}
	render()
	render()
	s.SendText("cat comple", false)
	waitScreen(t, s, "cat comple")
	router.Queue(input.SystemEvent{Event: key.Event{Name: key.NameTab, State: key.Press}})
	render()
	waitScreen(t, s, "cat completion-target")
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	render()
	waitScreen(t, s, "COMPLETION_OK")
}
