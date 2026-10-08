package ui

import (
	"fmt"

	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/el"
)

func (a *app) divider(cx *el.Context, n *pane, x, y, w, h, span float32) el.Element {
	name, cursor := "Horizontal divider", pointer.CursorRowResize
	gripX, gripY, gripW, gripH := max(0, (w-36)/2), float32(5), min(36, w), float32(2)
	if n.vertical {
		name, cursor = "Vertical divider", pointer.CursorColResize
		x, w = x-2, w+4
		gripX, gripY, gripW, gripH = 5, max(0, (h-36)/2), 2, min(36, h)
	} else {
		y, h = y-2, h+4
	}
	id := fmt.Sprintf("divider-%p", n)
	// Events are relative to the handle's last rendered position. Capture that
	// position once per frame so several queued moves don't accumulate drift.
	start := span * n.ratio
	divider := el.Div().ID(id).Absolute().Left(x).Top(y).
		W(el.Dp(w)).H(el.Dp(h)).Role("separator").Name(name).Focusable(false).
		Cursor(cursor).
		OnDoubleClick(func() { n.ratio = .5 }).
		OnDrag(func(e el.DragEvent) { n.resizeSplit(e, start, span) })
	if cx.Hovered(id) || n.resizing {
		divider.Child(el.Div().Absolute().Left(gripX).Top(gripY).W(el.Dp(gripW)).H(el.Dp(gripH)).Rounded(1).Bg(rgb(0x565a60)))
	}
	return divider
}

func (n *pane) resizeSplit(e el.DragEvent, start, span float32) {
	position := e.Y
	if n.vertical {
		position = e.X
	}
	switch e.Kind {
	case el.DragStart:
		n.dividerDragOffset, n.resizing = position, true
	case el.DragMove:
		if n.resizing && !e.Canceled {
			n.ratio = max(.15, min(.85, (start+position-n.dividerDragOffset)/max(1, span)))
		}
	case el.DragEnd:
		// Release can share a frame with the double-click callback. Preserve
		// its equalization rather than restoring the last rendered ratio.
		n.resizing = false
	}
}
