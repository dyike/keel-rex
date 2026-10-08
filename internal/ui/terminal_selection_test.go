package ui

import (
	"fmt"
	"github.com/dyike/keel-rex/internal/backend"
	"image"
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
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
)

type selectionHarness struct {
	test    *testing.T
	term    *terminal
	backend backend.Session
	router  input.Router
	ops     op.Ops
	scale   float32
	now     time.Time
	view    core.Widget
	size    image.Point
	origin  f32.Point
}

func selectionLine(row int) string { return fmt.Sprintf("row-%02d 中文 e\u0301 🚀", row) }

func newSelectionHarness(t *testing.T, scale float32, remote bool, historyLimit int) *selectionHarness {
	t.Helper()
	var s backend.Session
	if remote {
		dir := testServer(t)
		var err error
		s, err = backend.NewService(dir).OpenCommand(t.TempDir(), "/bin/sh", "-c", outputCommand)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Detach)
		waitModel(t, func() bool { return s.State().Title == "fixture-ready" })
	} else {
		s = outputSession(t, historyLimit)
	}
	p := &pane{}
	a := &app{focused: p, prefs: defaultPreferences()}
	term := &terminal{session: s, pane: p, owner: a}
	p.term = term
	h := &selectionHarness{test: t, term: term, backend: s, scale: scale, now: time.Unix(100, 0)}
	h.render()
	h.await(func() bool { state := s.State(); return state.Cols == h.term.cols && state.Rows == h.term.rows })
	var output strings.Builder
	output.WriteString("\x1b[3J\x1b[2J\x1b[H")
	for i := range 40 {
		output.WriteString(selectionLine(i) + "\r\n")
	}
	feedOutput(t, s, output.String())
	h.render()
	return h
}

func (h *selectionHarness) render() {
	for range 3 {
		h.ops.Reset()
		view := h.view
		if view == nil {
			view = h.term
		}
		size := h.size
		if size == (image.Point{}) {
			size = image.Pt(320, 96)
		}
		view.Layout(layout.Context{Ops: &h.ops, Source: h.router.Source(), Now: h.now, Metric: unit.Metric{PxPerDp: h.scale, PxPerSp: h.scale}, Constraints: layout.Exact(image.Pt(int(float32(size.X)*h.scale), int(float32(size.Y)*h.scale)))})
		h.router.Frame(&h.ops)
	}
}

func TestTerminalSelectionWithinWorkspace(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			h := newSelectionHarness(t, scale, false, 100)
			a, p := h.term.owner, h.term.pane
			a.tabs = []*workspace{{root: p, focus: p}}
			h.view = &workspaceView{app: a, root: el.Root(el.ViewFunc(func(*el.Context) el.Element {
				return el.Div().Child(el.Widget(h.term).Grow())
			}))}
			h.render()
			h.pointer(pointer.Press, 1, 3, pointer.ButtonPrimary, 0)
			h.pointer(pointer.Release, 1, 3, 0, 0)
			h.wheel(-10)
			h.pointer(pointer.Press, 0, 1, pointer.ButtonPrimary, 0)
			anchor := h.term.anchor
			h.pointer(pointer.Move, 8, 3, pointer.ButtonPrimary, 0)
			if !h.term.selecting || !h.term.hasSelection || h.term.caret <= anchor {
				t.Fatalf("workspace blocked drag: selecting=%v selected=%v anchor=%d caret=%d focused=%v", h.term.selecting, h.term.hasSelection, anchor, h.term.caret, h.term.focused)
			}
			h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(9*cellWidth*scale, 3.5*cellHeight*scale), Scroll: f32.Pt(0, -8*cellHeight*scale)})
			h.render()
			if !h.term.selecting || h.term.anchor != anchor || h.term.caret >= anchor {
				t.Fatalf("workspace blocked held-button wheel: selecting=%v anchor=%d caret=%d scroll=%d", h.term.selecting, h.term.anchor, h.term.caret, h.term.scroll)
			}
			h.pointer(pointer.Release, 8, 3, 0, 0)
			if got := h.copy(); strings.Count(got, "\n") < h.term.rows {
				t.Fatalf("workspace clipboard did not cross a viewport: %q", got)
			}
		})
	}
}

func TestTerminalSelectionFullWindow(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			h := newSelectionHarness(t, scale, false, 100)
			s := outputSession(t, 10000)
			h.term.session, h.backend = s, s
			a, p := h.term.owner, h.term.pane
			a.cwd = t.TempDir()
			a.tabs = []*workspace{{root: p, focus: p, title: "Selection regression"}}
			h.view = &workspaceView{app: a, root: el.Root(a)}
			h.size = image.Pt(1057, 639)
			h.render()
			var walk func(input.SemanticNode)
			walk = func(n input.SemanticNode) {
				if n.Desc.Label == h.term.plain {
					h.origin = f32.Pt(float32(n.Desc.Bounds.Min.X)/scale, float32(n.Desc.Bounds.Min.Y)/scale)
				}
				for _, child := range n.Children {
					walk(child)
				}
			}
			for _, node := range h.router.AppendSemantics(nil) {
				walk(node)
			}
			if h.origin.Y < 46 {
				t.Fatal("terminal not found inside the actual pane layout")
			}
			var output strings.Builder
			output.WriteString("\x1b[3J\x1b[2J\x1b[H")
			for i := range 200 {
				output.WriteString(selectionLine(i) + "\r\n")
			}
			feedOutput(t, s, output.String())
			h.render()
			// Establish keyboard focus, then begin a new ordinary mouse drag.
			h.pointer(pointer.Press, 1, 3, pointer.ButtonPrimary, 0)
			h.pointer(pointer.Release, 1, 3, 0, 0)
			h.pointer(pointer.Press, 0, 3, pointer.ButtonPrimary, 0)
			anchor := h.term.anchor
			h.pointer(pointer.Move, 8, 1, pointer.ButtonPrimary, 0)
			if !h.term.selecting || !h.term.hasSelection {
				t.Fatal("full window prevented ordinary shell drag selection")
			}
			h.wheel(-float32(h.term.rows + 10))
			if !h.term.selecting || h.term.anchor != anchor || h.term.caret >= anchor {
				t.Fatal("full window lost held-button selection during wheel scrolling")
			}
			h.pointer(pointer.Release, 8, 1, 0, 0)
			want := h.text()
			if got := h.copy(); got != want || strings.Count(got, "\n") <= h.term.rows {
				t.Fatalf("full window clipboard did not include the cross-screen range: %q", got)
			}
		})
	}
}

func (h *selectionHarness) await(done func() bool) {
	h.test.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			h.test.Fatal("terminal update timed out")
		}
		h.render()
		time.Sleep(time.Millisecond)
	}
}

func (h *selectionHarness) pointer(kind pointer.Kind, col, row float32, buttons pointer.Buttons, mods key.Modifiers) {
	h.now = h.now.Add(10 * time.Millisecond)
	h.router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Position: f32.Pt((h.origin.X+(col+.5)*cellWidth)*h.scale, (h.origin.Y+(row+.5)*cellHeight)*h.scale), Buttons: buttons, Modifiers: mods, Time: h.now.Sub(time.Unix(100, 0))})
	h.render()
}

func (h *selectionHarness) wheel(lines float32) {
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt((h.origin.X+20)*h.scale, (h.origin.Y+40)*h.scale), Scroll: f32.Pt(0, lines*cellHeight*h.scale)})
	h.render()
	h.await(func() bool { return h.term.viewFrame.Scroll == h.term.scroll })
}

func (h *selectionHarness) selectRows(first, last int) {
	h.pointer(pointer.Press, 0, float32(first), pointer.ButtonPrimary, 0)
	h.pointer(pointer.Move, float32(h.term.cols-1), float32(last), pointer.ButtonPrimary, 0)
	h.pointer(pointer.Release, float32(h.term.cols-1), float32(last), 0, 0)
}

func (h *selectionHarness) text() string {
	h.test.Helper()
	text, err := h.term.session.Copy(h.term.selectionRange())
	if err != nil {
		h.test.Fatal(err)
	}
	return text
}

func (h *selectionHarness) copy() string {
	h.test.Helper()
	h.router.WriteClipboard()
	h.router.Queue(key.Event{Name: "C", Modifiers: key.ModCommand, State: key.Press})
	h.render()
	var text string
	h.await(func() bool {
		_, value, ok := h.router.WriteClipboard()
		if ok {
			text = string(value)
		}
		return ok
	})
	h.router.Queue(key.Event{Name: "C", Modifiers: key.ModCommand, State: key.Release})
	h.render()
	return text
}

func selectedLines(first, last int) string {
	var lines []string
	for i := first; i <= last; i++ {
		lines = append(lines, selectionLine(i))
	}
	return strings.Join(lines, "\n")
}

func TestTerminalSelectionCopyScrollAndExtend(t *testing.T) {
	for _, remote := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("remote=%v-%gx", remote, scale), func(t *testing.T) {
				h := newSelectionHarness(t, scale, remote, 100)
				h.wheel(-15)
				first := h.term.viewStart + 1
				h.selectRows(1, 3)
				original := selectedLines(first, first+2)
				if got := h.copy(); got != original {
					t.Fatalf("initial clipboard=%q want %q", got, original)
				}
				anchor := h.term.anchor
				h.wheel(-10)
				if !h.term.hasSelection || h.term.anchor != anchor || h.text() != original {
					t.Fatal("scroll lost or moved the selection")
				}
				for y := range h.term.rowPaints {
					if h.term.rowPaints[y].key.selLo >= 0 {
						t.Fatal("offscreen selection highlighted unrelated output")
					}
				}
				if got := h.copy(); got != original {
					t.Fatalf("copy after scroll=%q", got)
				}
				start := h.term.viewStart + 1
				h.pointer(pointer.Press, 0, 1, pointer.ButtonPrimary, key.ModShift)
				h.pointer(pointer.Release, 0, 1, 0, key.ModShift)
				want := selectedLines(start, first-1) + "\nr"
				if got := h.copy(); got != want {
					t.Fatalf("cross-screen clipboard=%q want %q", got, want)
				}
				if strings.Count(want, "\n") <= h.term.rows {
					t.Fatal("fixture did not cross a viewport")
				}
			})
		}
	}
}

func TestTerminalSelectionDragScrollAndAutoScroll(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("%gx", scale), func(t *testing.T) {
			h := newSelectionHarness(t, scale, false, 100)
			h.wheel(-10)
			h.pointer(pointer.Press, 0, 3, pointer.ButtonPrimary, 0)
			anchor := h.term.anchor
			h.pointer(pointer.Move, 5, 1, pointer.ButtonPrimary, 0)
			h.wheel(-8)
			if h.term.anchor != anchor || h.term.caret >= anchor-h.term.rows*h.term.cols {
				t.Fatal("held selection did not extend through wheel scrolling")
			}
			h.pointer(pointer.Move, 0, -2, pointer.ButtonPrimary, 0)
			for i := 0; i < 40 && h.term.scroll < h.term.viewFrame.History; i++ {
				h.now = h.now.Add(60 * time.Millisecond)
				h.render()
			}
			if h.term.scroll != h.term.viewFrame.History || h.term.caret != 0 || h.term.anchor != anchor {
				t.Fatalf("edge drag did not reach oldest output: scroll=%d caret=%d", h.term.scroll, h.term.caret)
			}
			h.pointer(pointer.Release, 0, -2, 0, 0)
			want := selectedLines(0, anchor/h.term.cols-1) + "\nr"
			if got := h.copy(); got != want {
				t.Fatalf("auto-scroll clipboard=%q want %q", got, want)
			}
			stopped := h.term.scroll
			h.now = h.now.Add(time.Second)
			h.render()
			if h.term.selecting || h.term.scroll != stopped {
				t.Fatal("release did not stop auto-scroll")
			}
		})
	}
}

func TestTerminalSelectionScrollbarAndOutput(t *testing.T) {
	h := newSelectionHarness(t, 1, false, 100)
	h.wheel(-10)
	h.selectRows(1, 2)
	want, anchor := h.text(), h.term.anchor
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: f32.Pt(315, 12), Buttons: pointer.ButtonPrimary})
	h.render()
	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(315, 12)})
	h.render()
	if !h.term.hasSelection || h.term.anchor != anchor || h.text() != want {
		t.Fatalf("scrollbar cleared or moved selection: selected=%v selecting=%v anchor=%d want=%d caret=%d scroll=%d", h.term.hasSelection, h.term.selecting, h.term.anchor, anchor, h.term.caret, h.term.scroll)
	}
	feedOutput(t, h.backend, "more output\r\nmore output\r\n")
	h.render()
	if h.term.anchor != anchor || h.text() != want {
		t.Fatal("new output moved selected text")
	}
}

func TestTerminalSelectionEvictionAndClear(t *testing.T) {
	h := newSelectionHarness(t, 1, false, 10)
	h.wheel(-6)
	h.selectRows(1, 2)
	want, anchor, original := h.text(), h.term.anchor, h.term.selectionRange()
	first := h.term.viewFrame.HistoryStart
	feedOutput(t, h.backend, "more output\r\nmore output\r\n")
	h.render()
	if h.term.viewFrame.HistoryStart <= first || h.term.anchor != anchor || h.text() != want {
		t.Fatal("history eviction changed retained selection")
	}
	feedOutput(t, h.backend, strings.Repeat("new\r\n", 20))
	h.render()
	if h.term.hasSelection {
		t.Fatal("evicted selection still active")
	}
	if _, err := h.backend.Copy(original); err == nil {
		t.Fatal("copied unrelated text after history eviction")
	}
	h.selectRows(1, 2)
	beforeClear := h.term.selectionRange()
	feedOutput(t, h.backend, "\x1b[3J\x1b[2J\x1b[Hreplacement")
	h.render()
	if h.term.hasSelection {
		t.Fatal("clearing history retained old selection")
	}
	if _, err := h.backend.Copy(beforeClear); err == nil {
		t.Fatal("copy accepted selection from cleared history")
	}
}

func TestTerminalSelectAllAndWordSelection(t *testing.T) {
	h := newSelectionHarness(t, 1, false, 100)
	h.wheel(-10)
	h.term.selectAll()
	if got := h.text(); !strings.HasPrefix(got, selectionLine(0)) || !strings.Contains(got, selectionLine(39)) {
		t.Fatal("select all missed history or live output")
	}
	h.term.clearSelection()
	h.pointer(pointer.Press, 1, 1, pointer.ButtonPrimary, 0)
	h.pointer(pointer.Release, 1, 1, 0, 0)
	h.pointer(pointer.Press, 1, 1, pointer.ButtonPrimary, 0)
	h.pointer(pointer.Release, 1, 1, 0, 0)
	if got := h.text(); got != "row" {
		t.Fatalf("double-click selected %q", got)
	}
	h.pointer(pointer.Press, 1, 1, pointer.ButtonPrimary, 0)
	h.pointer(pointer.Release, 1, 1, 0, 0)
	if got := h.text(); got != selectionLine(h.term.viewStart+1) {
		t.Fatalf("triple-click selected %q", got)
	}
}

func TestTerminalSelectionEmptyHistoryLinesAndAltScreen(t *testing.T) {
	h := newSelectionHarness(t, 1, false, 100)
	feedOutput(t, h.backend, "\x1b[3J\x1b[2J\x1b[Hstart\r\n\r\nend"+strings.Repeat("\r\n", 8))
	h.render()
	h.wheel(-100)
	h.selectRows(0, 2)
	if got := h.text(); got != "start\n\nend" {
		t.Fatalf("blank history line copy=%q", got)
	}
	feedOutput(t, h.backend, "\x1b[?1049h\x1b[2JALT screen")
	h.render()
	if h.term.hasSelection || h.term.viewStart != 0 {
		t.Fatal("main-screen selection leaked into alternate screen")
	}
	h.selectRows(0, 0)
	if got := h.text(); !strings.Contains(got, "ALT screen") {
		t.Fatalf("alternate screen selection=%q", got)
	}
}
