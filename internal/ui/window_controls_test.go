package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

func TestIntegratedWindowsHeader(t *testing.T) {
	previous := desktopChrome
	desktopChrome = chromeForPlatform("windows")
	t.Cleanup(func() { desktopChrome = previous })
	for _, width := range []int{800, 1057} {
		for _, count := range []int{1, 14} {
			for _, scale := range []float32{1, 1.5, 2} {
				for _, appearance := range []string{"light", "dark"} {
					t.Run(fmt.Sprintf("%d-%dtabs-%gx-%s", width, count, scale, appearance), func(t *testing.T) {
						a := &app{prefs: defaultPreferences(), hostModel: "SER8"}
						a.prefs.Appearance = appearance
						for i := range count {
							a.tabs = append(a.tabs, &workspace{root: &pane{}, title: fmt.Sprintf("pwsh workspace %d", i+1)})
						}
						root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
							box := el.Div().WFull().HFull().Child(el.Widget(&chrome{a}))
							a.renderHeader(cx, box, float32(width))
							return box
						}))
						var ops op.Ops
						var router input.Router
						for range 3 {
							ops.Reset()
							root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(float32(width)*scale), int(46*scale)))})
							router.Frame(&ops)
						}
						var controls []image.Rectangle
						var walk func(input.SemanticNode)
						walk = func(n input.SemanticNode) {
							if strings.Contains(n.Desc.Label, "window") || n.Desc.Label == "Host information" || n.Desc.Label == "Command palette" || n.Desc.Label == "New terminal tab" || strings.HasPrefix(n.Desc.Label, "Terminal tab ") {
								controls = append(controls, n.Desc.Bounds)
								if n.Desc.Bounds.Min.Y < 0 || n.Desc.Bounds.Max.Y > int(44*scale) {
									t.Fatalf("%s escaped the single header row: %v", n.Desc.Label, n.Desc.Bounds)
								}
							}
							for _, child := range n.Children {
								walk(child)
							}
						}
						if nodes := router.AppendSemantics(nil); len(nodes) > 0 {
							walk(nodes[0])
						}
						if len(controls) != count+6 {
							t.Fatalf("visible controls: %d, want %d", len(controls), count+6)
						}
						for i, bounds := range controls {
							center := f32.Pt(float32(bounds.Min.X+bounds.Max.X)/2, float32(bounds.Min.Y+bounds.Max.Y)/2)
							if action, ok := router.ActionAt(center); ok && action == system.ActionMove {
								t.Fatalf("header control became a window drag area: %v", bounds)
							}
							for _, other := range controls[i+1:] {
								if bounds.Overlaps(other) {
									t.Fatalf("header controls overlap: %v and %v", bounds, other)
								}
							}
						}
						if count == 1 {
							_, rowWidth, _ := a.tabStripLayout(float32(width))
							blankLeft := desktopChrome.tabsLeft() + rowWidth + 6
							blankRight := desktopChrome.toolbarRight(float32(width)) - 82
							point := f32.Pt((blankLeft+blankRight)/2*scale, 22*scale)
							if action, ok := router.ActionAt(point); !ok || action != system.ActionMove {
								t.Fatal("empty header must support native window dragging")
							}
						}
						if dir := os.Getenv("REX_HEADER_QA_DIR"); dir != "" && count == 1 && width == 1057 {
							if err := os.MkdirAll(dir, 0755); err != nil {
								t.Fatal(err)
							}
							if err := window.ScreenshotAtScale(root, width, 46, scale, filepath.Join(dir, fmt.Sprintf("header-%s-%gx.png", appearance, scale))); err != nil {
								t.Fatal(err)
							}
						}
					})
				}
			}
		}
	}
}
