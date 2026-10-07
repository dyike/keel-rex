package main

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestTerminalIMEPreeditCommitAndCandidateGeometry(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, alt := range []bool{false, true} {
			remote := &remoteSession{wake: make(chan struct{}, 1), stop: make(chan struct{})}
			s := &session{remote: remote, frame: screenFrame{Cols: 40, Rows: 10, Alt: alt, Cursor: image.Pt(3, 2)}}
			p := &pane{}
			a := &app{focused: p, prefs: defaultPreferences()}
			term := &terminal{session: s, pane: p, owner: a}
			p.term = term
			var router input.Router
			var ops op.Ops
			render := func() {
				ops.Reset()
				g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(320*scale), int(180*scale)))}
				term.Layout(g)
				g.Execute(key.FocusCmd{Tag: term})
				router.Frame(&ops)
			}
			textSent := func() string {
				remote.mu.Lock()
				defer remote.mu.Unlock()
				var b strings.Builder
				for _, req := range remote.queue {
					if req.Op == "text" {
						b.WriteString(req.Text)
					}
					if req.Op == "key" {
						t.Errorf("key escaped IME: %+v", req.Key)
					}
				}
				return b.String()
			}
			render()
			render()
			router.Queue(key.EditEvent{Text: "ni", Range: key.Range{}}, key.CompositionEvent{Start: 0, End: 2}, key.SelectionEvent{Start: 2, End: 2})
			render()
			render()
			if got := textSent(); got != "" {
				t.Fatalf("preedit reached PTY: %q", got)
			}
			state := router.EditorState()
			if state.Snippet.Text != "ni" || state.Selection.CompositionBounds.Empty() || state.Selection.Caret.Ascent <= 0 {
				t.Fatalf("missing IME state: %+v", state)
			}
			router.Queue(key.Event{Name: key.NameSpace, State: key.Press}, key.EditEvent{Text: "你好", Range: key.Range{Start: 0, End: 2}}, key.CompositionEvent{Start: 0, End: 2}, key.SelectionEvent{Start: 2, End: 2})
			render()
			render()
			if got := textSent(); got != "" {
				t.Fatalf("candidate reached PTY before commit: %q", got)
			}
			router.Queue(key.CompositionEvent{Start: -1, End: -1})
			render()
			render()
			if got := textSent(); got != "你好" {
				t.Fatalf("committed text=%q", got)
			}
			if state := router.EditorState(); state.Snippet.Text != "" || !state.Selection.CompositionBounds.Empty() {
				t.Fatal("commit retained editor state")
			}
			router.Queue(key.EditEvent{Text: "中文😀", Range: key.Range{}}, key.CompositionEvent{Start: -1, End: -1})
			render()
			render()
			if got := textSent(); got != "你好中文😀" {
				t.Fatalf("repeated Unicode commit=%q", got)
			}
			router.Queue(key.EditEvent{Text: "zhong", Range: key.Range{}}, key.CompositionEvent{Start: 0, End: 5})
			render()
			router.Queue(key.EditEvent{Text: "", Range: key.Range{Start: 0, End: 5}}, key.CompositionEvent{Start: -1, End: -1})
			render()
			render()
			if got := textSent(); got != "你好中文😀" {
				t.Fatalf("cancel sent preedit=%q", got)
			}
		}
	}
}
