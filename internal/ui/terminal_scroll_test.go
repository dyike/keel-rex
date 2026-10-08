package ui

import (
	"fmt"
	"github.com/dyike/keel-rex/internal/backend"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

func TestTerminalWheelScrollsShellHistory(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			s := outputSession(t, 100)
			defer s.Close()
			p := &pane{}
			a := &app{focused: p, prefs: defaultPreferences()}
			term := &terminal{session: s, pane: p, owner: a}
			p.term = term
			var router input.Router
			var ops op.Ops
			render := func() {
				for range 2 {
					ops.Reset()
					term.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(320*scale), int(96*scale)))})
					router.Frame(&ops)
				}
			}
			wheel := func(lines float32) {
				router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(20*scale, 20*scale), Scroll: f32.Pt(0, lines*a.prefs.FontSize*1.28*scale)})
				render()
			}
			render()
			var output strings.Builder
			output.WriteString("\x1b[2J\x1b[H")
			for i := range 40 {
				fmt.Fprintf(&output, "history-%02d\r\n", i)
			}
			feedOutput(t, s, output.String())
			render()
			live := s.Snapshot(0, 0)
			if live.History < 10 || !strings.Contains(live.Plain, "history-39") {
				t.Fatalf("missing history fixture: %+v", live)
			}
			wheel(-3)
			if term.scroll != 3 || strings.Contains(s.Snapshot(term.scroll, 0).Plain, "history-39") {
				t.Fatalf("wheel did not show older shell output: scroll=%d", term.scroll)
			}
			wheel(3)
			if term.scroll != 0 {
				t.Fatalf("wheel did not return to live output: %d", term.scroll)
			}
			for range 4 {
				wheel(-.25)
			}
			if term.scroll != 1 {
				t.Fatalf("trackpad fractional scroll lost: %d", term.scroll)
			}
			wheel(-10000)
			if term.scroll != live.History {
				t.Fatalf("scroll did not clamp to history: %d", term.scroll)
			}
			wheel(1)
			if term.scroll != live.History-1 {
				t.Fatalf("scroll remained stuck at the top: %d", term.scroll)
			}
		})
	}
}

func TestTerminalScrollbarClickAndDrag(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			remote := &modelSession{}
			s := remote
			s.state = backend.Frame{Cols: 40, Rows: 6, History: 25}
			p := &pane{}
			a := &app{focused: p, prefs: defaultPreferences()}
			term := &terminal{session: s, pane: p, owner: a}
			p.term = term
			var router input.Router
			var ops op.Ops
			render := func() {
				for range 3 {
					ops.Reset()
					term.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(320*scale), int(96*scale)))})
					router.Frame(&ops)
				}
			}
			move := func(kind pointer.Kind, y float32, buttons pointer.Buttons) {
				router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Position: f32.Pt(315*scale, y*scale), Buttons: buttons})
				render()
			}
			render()
			move(pointer.Press, 10, pointer.ButtonPrimary)
			move(pointer.Release, 10, 0)
			if term.scroll < 20 || term.hasSelection || term.selecting {
				t.Fatalf("scrollbar click did not show history or selected terminal text: scroll=%d selection=%v", term.scroll, term.hasSelection)
			}
			move(pointer.Press, 12, pointer.ButtonPrimary)
			move(pointer.Move, 30, pointer.ButtonPrimary)
			move(pointer.Move, 90, pointer.ButtonPrimary)
			move(pointer.Release, 90, 0)
			if term.scroll != 0 || remote.scroll != 0 {
				t.Fatalf("drag did not return to live output: local=%d remote=%d", term.scroll, remote.scroll)
			}
			if dir := os.Getenv("REX_SCROLL_QA_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				root := el.Root(el.ViewFunc(func(*el.Context) el.Element {
					return el.Div().Bg(a.colors().panel).Child(el.Widget(term).Grow())
				}))
				if err := window.ScreenshotAtScale(root, 320, 96, scale, filepath.Join(dir, fmt.Sprintf("scrollbar-%gx.png", scale))); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestTerminalRemoteScrollBoundsAndMouseProtocol(t *testing.T) {
	remote := &modelSession{}
	s := remote
	s.state = backend.Frame{Cols: 40, Rows: 6, History: 25}
	p := &pane{}
	a := &app{focused: p, prefs: defaultPreferences()}
	term := &terminal{session: s, pane: p, owner: a}
	p.term = term
	var router input.Router
	var ops op.Ops
	render := func() {
		for range 2 {
			ops.Reset()
			term.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(320, 96))})
			gtx := layout.Context{Ops: &ops, Source: router.Source()}
			gtx.Execute(key.FocusCmd{Tag: term})
			router.Frame(&ops)
		}
	}
	wheel := func(lines float32) {
		router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(20, 20), Scroll: f32.Pt(0, lines*a.prefs.FontSize*1.28)})
		render()
	}
	render()
	wheel(-10000)
	if term.scroll != 25 || remote.scroll != 25 {
		t.Fatalf("remote request exceeds history: local=%d remote=%d", term.scroll, remote.scroll)
	}
	wheel(1)
	if term.scroll != 24 || remote.scroll != 24 {
		t.Fatalf("remote scroll stuck at top: local=%d remote=%d", term.scroll, remote.scroll)
	}
	// A pending frame must not restore the old scroll position after typing.
	router.Queue(key.EditEvent{Text: "x"})
	render()
	if term.scroll != 0 || remote.scroll != 0 {
		t.Fatal("typing did not return to the live screen")
	}
	s.state.Mouse = true
	s.state.Alt = true
	render()
	wheel(-1)
	if term.scroll != 0 {
		t.Fatal("mouse-enabled TUI changed shell scrollback")
	}
	for _, req := range remote.queue {
		if req.Op == "mouse" && req.Kind == "scroll" && req.Mouse.Button == uv.MouseWheelUp {
			return
		}
	}
	t.Fatal("wheel was not forwarded to the mouse-enabled TUI")
}
