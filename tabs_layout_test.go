package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

func tabBounds(router *input.Router) []image.Rectangle {
	var bounds []image.Rectangle
	var walk func(input.SemanticNode)
	walk = func(n input.SemanticNode) {
		if strings.HasPrefix(n.Desc.Label, "Terminal tab ") {
			bounds = append(bounds, n.Desc.Bounds)
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	if nodes := router.AppendSemantics(nil); len(nodes) > 0 {
		walk(nodes[0])
	}
	return bounds
}

func TestTabsFitWithoutHorizontalScroll(t *testing.T) {
	for _, width := range []int{480, 680, 1057} {
		for _, count := range []int{2, 6, 14, 30} {
			for _, scale := range []float32{1, 2} {
				t.Run(fmt.Sprintf("%dpx-%dtabs-%gx", width, count, scale), func(t *testing.T) {
					a := &app{}
					for i := range count {
						a.tabs = append(a.tabs, &workspace{root: &pane{}, title: fmt.Sprintf("Workspace %d with a long name", i+1)})
					}
					root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
						box := el.Div().Bg(a.colors().track)
						a.renderTabs(cx, box, float32(width))
						return box
					}))
					var ops op.Ops
					var router input.Router
					for range 3 {
						ops.Reset()
						root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(float32(width)*scale), int(44*scale)))})
						router.Frame(&ops)
					}
					bounds := tabBounds(&router)
					if len(bounds) != count {
						t.Fatalf("visible tabs: %d want %d", len(bounds), count)
					}
					right := int(float32(width-82) * scale)
					previous := int(202 * scale)
					for i, b := range bounds {
						if b.Dx() <= 0 || b.Min.X < previous || b.Max.X > right {
							t.Fatalf("tab %d overlaps or leaves the strip: %v, previous=%d right=%d", i, b, previous, right)
						}
						previous = b.Max.X
					}
					if dir := os.Getenv("REX_TAB_QA_DIR"); dir != "" && scale == 2 && width == 680 {
						if e := os.MkdirAll(dir, 0755); e != nil {
							t.Fatal(e)
						}
						if e := window.ScreenshotAtScale(root, width, 44, scale, filepath.Join(dir, fmt.Sprintf("tabs-%d.png", count))); e != nil {
							t.Fatal(e)
						}
					}
				})
			}
		}
	}
}

func TestShortcutNewTabsStayVisible(t *testing.T) {
	a := newApp(t.TempDir())
	defer a.close()
	h := interactionView{view: &workspaceView{app: a, root: el.Root(a)}}
	h.render()
	for range 7 {
		h.router.Queue(key.Event{Name: "T", Modifiers: key.ModShortcut, State: key.Press})
		h.render()
		h.router.Queue(key.Event{Name: "T", Modifiers: key.ModShortcut, State: key.Release})
		h.render()
	}
	if len(a.tabs) != 8 || a.active != 7 {
		t.Fatalf("shortcut tabs=%d active=%d", len(a.tabs), a.active)
	}
	bounds := tabBounds(&h.router)
	if len(bounds) != 8 {
		t.Fatalf("visible tabs=%d", len(bounds))
	}
	for i, b := range bounds {
		if b.Max.X > 975 {
			t.Fatalf("tab %d outside strip: %v", i, b)
		}
	}
}

func TestCompactTabsClickDragAndClose(t *testing.T) {
	a := newApp(t.TempDir())
	defer a.close()
	for range 7 {
		a.newTab()
	}
	h := interactionView{view: &workspaceView{app: a, root: el.Root(a)}}
	h.render()
	bounds := tabBounds(&h.router)
	first := a.tabs[0]
	active := a.tabs[a.active]
	x := float32(bounds[0].Min.X + 10)
	lastX := float32(bounds[7].Min.X + 10)
	h.pointer(pointer.Press, x, 20, time.Second, pointer.ButtonPrimary)
	h.pointer(pointer.Release, x, 20, 1100*time.Millisecond, 0)
	if a.active != 0 {
		t.Fatal("compact first tab could not be activated")
	}
	a.activate(7)
	h.render()
	h.pointer(pointer.Press, x, 20, 2*time.Second, pointer.ButtonPrimary)
	h.pointer(pointer.Move, lastX, 20, 2200*time.Millisecond, pointer.ButtonPrimary)
	h.pointer(pointer.Release, lastX, 20, 2300*time.Millisecond, 0)
	if a.tabs[7] != first || a.tabs[a.active] != active {
		t.Fatal("compact drag did not preserve the active tab")
	}
	h.pointer(pointer.Move, lastX, 20, 3*time.Second, 0)
	var closeBounds image.Rectangle
	for _, n := range h.router.AppendSemantics(nil) {
		if n.Desc.Label == "Close tab 8" {
			closeBounds = n.Desc.Bounds
			break
		}
	}
	if closeBounds.Empty() || !closeBounds.In(bounds[7]) {
		t.Fatalf("compact close button outside tab: %v, tab %v", closeBounds, bounds[7])
	}
	closeX := float32(closeBounds.Min.X + closeBounds.Dx()/2)
	h.pointer(pointer.Press, closeX, 20, 3200*time.Millisecond, pointer.ButtonPrimary)
	h.pointer(pointer.Release, closeX, 20, 3300*time.Millisecond, 0)
	if len(a.tabs) != 7 || a.tabs[a.active] != active {
		t.Fatal("compact close removed the wrong tab or changed activation")
	}
}
