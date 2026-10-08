package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"io"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/op"
	"github.com/dyike/keel/ui/core"
)

func (t *terminal) selectionRange() backend.Selection {
	return backend.Selection{Anchor: t.anchor, Caret: t.caret, Cols: t.cols, Epoch: t.viewFrame.HistoryEpoch, Alt: t.viewFrame.Alt}
}

type selectionCopy struct {
	text string
	err  error
}

// Read a remote selection without blocking terminal input or rendering.
func (t *terminal) copySelection() {
	if !t.hasSelection && t.anchor == t.caret {
		return
	}
	sel := t.selectionRange()
	// Each copy owns its result channel; a late response cannot overwrite a
	// newer copy. Only Layout writes terminal fields or the clipboard.
	result := make(chan selectionCopy, 1)
	t.pendingCopy = result
	go func() {
		text, err := t.session.Copy(sel)
		result <- selectionCopy{text: text, err: err}
		core.Update(func() {})
	}()
}

func (t *terminal) flushCopy(gtx core.C) {
	select {
	case result := <-t.pendingCopy:
		t.pendingCopy = nil
		if result.err != nil {
			if t.owner != nil {
				t.owner.notice = result.err.Error()
			}
			return
		}
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(result.text))})
	default:
	}
}

func (t *terminal) clearSelection() {
	t.hasSelection, t.selecting = false, false
	t.anchor = t.caret
	t.selectionScrollAt = time.Time{}
	t.extendSelection = false
}

func (t *terminal) selectAll() {
	first, last := t.viewFrame.HistoryStart, t.viewFrame.HistoryStart+t.viewFrame.History+t.rows
	if t.viewFrame.Alt {
		first, last = 0, t.rows
	}
	t.anchor, t.caret, t.hasSelection = first*t.cols, last*t.cols-1, true
}

func (t *terminal) syncSelection(frame backend.Frame) {
	if t.viewReady && (frame.Alt != t.viewFrame.Alt || frame.HistoryEpoch != t.viewFrame.HistoryEpoch || frame.Cols != t.viewFrame.Cols) {
		t.clearSelection()
	}
	t.viewFrame, t.viewReady = frame, true
	t.viewStart = frame.HistoryStart + frame.History - frame.Scroll
	first := frame.HistoryStart
	if frame.Alt {
		t.viewStart, first = 0, 0
	}
	if t.hasSelection || t.selecting {
		lo, hi := first*t.cols, (first+frame.History+t.rows)*t.cols-1
		if frame.Alt {
			hi = t.rows*t.cols - 1
		}
		if max(t.anchor, t.caret) < lo || min(t.anchor, t.caret) > hi {
			t.clearSelection()
		} else {
			t.anchor, t.caret = max(lo, min(t.anchor, hi)), max(lo, min(t.caret, hi))
		}
	}
}

func (t *terminal) selectionIndex(pos f32.Point, cw, ch float32) int {
	x := max(0, min(t.cols-1, int(pos.X/cw)))
	y := max(0, min(t.rows-1, int(pos.Y/ch)))
	return (t.viewStart+y)*t.cols + x
}

func (t *terminal) autoScrollSelection(gtx core.C, ch, h float32) {
	if !t.selecting || t.viewFrame.Alt {
		t.selectionScrollAt = time.Time{}
		return
	}
	delta := 0
	if t.selectionPointer.Y < ch/2 {
		delta = min(6, 1+int((ch/2-t.selectionPointer.Y)/ch))
	} else if t.selectionPointer.Y > h-ch/2 {
		delta = -min(6, 1+int((t.selectionPointer.Y-h+ch/2)/ch))
	}
	if delta == 0 {
		t.selectionScrollAt = time.Time{}
		return
	}
	if t.selectionScrollAt.IsZero() || !gtx.Now.Before(t.selectionScrollAt) {
		t.scroll = max(0, min(t.viewFrame.History, t.scroll+delta))
		t.selectionScrollAt = gtx.Now.Add(50 * time.Millisecond)
	}
	if delta > 0 && t.scroll < t.viewFrame.History || delta < 0 && t.scroll > 0 {
		gtx.Execute(op.InvalidateCmd{At: t.selectionScrollAt})
	}
}
