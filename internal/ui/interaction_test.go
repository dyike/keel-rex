package ui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"image"
	"testing"
	"time"
)

type interactionView struct {
	view   *workspaceView
	router input.Router
	ops    op.Ops
	scale  float32
}

func (h *interactionView) render() {
	scale := h.scale
	if scale == 0 {
		scale = 1
	}
	for range 3 {
		h.ops.Reset()
		h.view.Layout(layout.Context{Ops: &h.ops, Now: time.Now(), Source: h.router.Source(), Constraints: layout.Exact(image.Pt(int(1057*scale), int(639*scale))), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}})
		h.router.Frame(&h.ops)
	}
}
func (h *interactionView) pointer(kind pointer.Kind, x, y float32, at time.Duration, buttons pointer.Buttons) {
	h.router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, Position: f32.Pt(x, y), Time: at, Buttons: buttons})
	h.render()
}
func TestPointerTabReorderAndDividerDrag(t *testing.T) {
	a := newApp(t.TempDir())
	defer a.close()
	first := a.tabs[0]
	a.newTab()
	a.newTab()
	active := a.tabs[a.active]
	h := interactionView{view: &workspaceView{app: a, root: el.Root(a)}}
	h.render()
	h.pointer(pointer.Press, 270, 20, time.Second, pointer.ButtonPrimary)
	h.pointer(pointer.Move, 740, 20, 1200*time.Millisecond, pointer.ButtonPrimary)
	h.pointer(pointer.Release, 740, 20, 1300*time.Millisecond, 0)
	if a.tabs[2] != first || a.tabs[a.active] != active {
		t.Fatal("drag changed the wrong tab or active workspace")
	}
	a.activate(2)
	h.render()
	ratio := first.root.ratio
	x := float32(8) + (1057-16-8)*ratio + 4
	h.pointer(pointer.Press, x, 150, 2*time.Second, pointer.ButtonPrimary)
	h.pointer(pointer.Move, x+60, 150, 2200*time.Millisecond, pointer.ButtonPrimary)
	h.pointer(pointer.Move, x+120, 150, 2400*time.Millisecond, pointer.ButtonPrimary)
	h.pointer(pointer.Release, x+120, 150, 2600*time.Millisecond, 0)
	want := ratio + 120/float32(1057-16-8)
	if got := first.root.ratio; got < want-.005 || got > want+.005 {
		t.Fatalf("divider drifted while moving: got %f want %f", got, want)
	}
}
