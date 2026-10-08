package ui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/el"
)

func TestNestedDividerQueuedMoves(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, vertical := range []bool{false, true} {
			t.Run(fmt.Sprintf("vertical=%t/scale=%g", vertical, scale), func(t *testing.T) {
				a := newApp(t.TempDir())
				defer a.close()
				h := interactionView{view: &workspaceView{app: a, root: el.Root(a)}, scale: scale}
				h.render()
				n := a.tabs[0].root
				other := n.first
				if !vertical {
					n, other = other, n
				}
				ratio, otherRatio := n.ratio, other.ratio
				r := n.bounds
				span, x, y := r.H-8, r.X+r.W/2, r.Y+(r.H-8)*ratio+4
				if vertical {
					span, x, y = r.W-8, r.X+(r.W-8)*ratio+4, r.Y+r.H/2
				}
				queue := func(kind pointer.Kind, delta float32, at time.Duration, buttons pointer.Buttons) {
					px, py := x, y
					if vertical {
						px += delta
					} else {
						py += delta
					}
					h.router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Position: f32.Pt(px*scale, py*scale), Time: at, Buttons: buttons})
				}
				queue(pointer.Press, 0, time.Second, pointer.ButtonPrimary)
				h.render()
				// Native input can deliver several movements before the next frame.
				for i, delta := range []float32{20, 40, 60} {
					queue(pointer.Move, delta, time.Second+time.Duration(i+1)*20*time.Millisecond, pointer.ButtonPrimary)
				}
				queue(pointer.Release, 60, 1200*time.Millisecond, 0)
				h.render()
				want := ratio + 60/span
				if math.Abs(float64(n.ratio-want)) > .005 {
					t.Fatalf("queued movements drifted: got %g want %g", n.ratio, want)
				}
				if other.ratio != otherRatio || n.resizing {
					t.Fatal("drag changed another divider or continued after release")
				}
				queue(pointer.Move, 100, 1400*time.Millisecond, 0)
				h.render()
				if math.Abs(float64(n.ratio-want)) > .005 {
					t.Fatal("hover changed divider ratio after release")
				}
			})
		}
	}
}

func TestDividerCancellationAndLimits(t *testing.T) {
	n := &pane{ratio: .5, vertical: true}
	n.resizeSplit(el.DragEvent{Kind: el.DragStart, X: 4}, 500, 1000)
	n.resizeSplit(el.DragEvent{Kind: el.DragMove, X: 1004}, 500, 1000)
	if n.ratio != .85 {
		t.Fatal("pane minimum size was not preserved", n.ratio)
	}
	n.resizeSplit(el.DragEvent{Kind: el.DragEnd, Canceled: true}, 850, 1000)
	n.resizeSplit(el.DragEvent{Kind: el.DragMove, X: -1000}, 850, 1000)
	if n.ratio != .85 || n.resizing {
		t.Fatal("cancelled drag still changed the divider", n.ratio)
	}
}

func TestCreatedSplitDragAndDoubleClick(t *testing.T) {
	a := newApp(t.TempDir())
	defer a.close()
	a.newTab()
	a.split(true)
	root := a.tabs[a.active].root
	a.focusPane(root.second)
	a.split(false)
	n := root.second
	h := interactionView{view: &workspaceView{app: a, root: el.Root(a)}}
	h.render()
	r := n.bounds
	x, y := r.X+r.W/2, r.Y+(r.H-8)*n.ratio+4
	h.pointer(pointer.Press, x, y, time.Second, pointer.ButtonPrimary)
	h.pointer(pointer.Move, x, y+80, 1200*time.Millisecond, pointer.ButtonPrimary)
	h.pointer(pointer.Release, x, y+80, 1400*time.Millisecond, 0)
	want := .5 + 80/(r.H-8)
	if math.Abs(float64(n.ratio-want)) > .005 || root.ratio != .5 {
		t.Fatalf("new nested split could not resize independently: nested=%g root=%g", n.ratio, root.ratio)
	}
	y = r.Y + (r.H-8)*n.ratio + 4
	for i := range 2 {
		at := 3*time.Second + time.Duration(i)*150*time.Millisecond
		h.pointer(pointer.Press, x, y, at, pointer.ButtonPrimary)
		h.pointer(pointer.Release, x, y, at+50*time.Millisecond, 0)
	}
	if n.ratio != .5 || n.resizing {
		t.Fatalf("double-click did not restore equal panes: ratio=%g resizing=%t", n.ratio, n.resizing)
	}
}
