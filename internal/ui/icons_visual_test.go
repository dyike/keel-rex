package ui

import (
	"fmt"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"image"
	"os"
	"path/filepath"
	"testing"
)

// Optional artifact renderer, not a native window or a UI acceptance fixture.
func TestIconVisualFixture(t *testing.T) {
	dir := os.Getenv("REX_ICON_QA_DIR")
	if dir == "" {
		t.Skip("icon inspection artifacts not requested")
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"light", "dark"} {
		if mode == "dark" {
			theme.Apply(theme.Dark())
		} else {
			theme.Apply(theme.Light())
		}
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			row := el.Div().Row().Gap(10)
			actions := el.Div().Row().Gap(10)
			for _, name := range []string{"search", "folder-open", "rotate-ccw", "check", "diff", "circle-dot", "pencil"} {
				actions.Child(el.Div().W(el.Dp(72)).Items(el.Center).Gap(8).Child(iconElement(name, 16), el.Text(name).TextSize(9).MaxLines(1)))
			}
			programs := el.Div().Row().Gap(10)
			for _, name := range []string{"codex", "claude", "node", "python", "bun", "git", "fish"} {
				name := name
				programs.Child(el.Div().W(el.Dp(72)).Items(el.Center).Gap(8).Child(el.Widget(core.Func(func(gtx core.C) core.D {
					p := painter{gtx, gtx.Metric.PxPerDp}
					p.programTile(name, 24, 0, 24, 20)
					return core.D{Size: image.Pt(gtx.Dp(72), gtx.Dp(22))}
				})), el.Text(name).TextSize(9)))
			}
			for _, name := range []string{"split-v", "split-h", "expand", "restore", "close", "plus", "command"} {
				name := name
				row.Child(el.Div().W(el.Dp(72)).Gap(12).Items(el.Center).Child(el.Widget(core.Func(func(gtx core.C) core.D {
					p := painter{gtx, gtx.Metric.PxPerDp}
					p.symbol(name, 28, 0, 16, 16)
					p.rect(24, 30, 24, 24, 5, theme.SubtleHover)
					p.symbol(name, 28, 34, 16, 16)
					return core.D{Size: image.Pt(gtx.Dp(72), gtx.Dp(60))}
				})), el.Text(name).TextSize(10).TextColor(theme.Muted)))
			}
			return el.Div().P(20).Gap(18).Bg(theme.Surface).Child(el.Text("Button icons · 16dp · idle / hover").Bold().TextSize(14), row, actions, programs)
		}))
		for _, scale := range []float32{1, 2} {
			if e := window.ScreenshotAtScale(root, 612, 265, scale, filepath.Join(dir, fmt.Sprintf("buttons-%s-%gx.png", mode, scale))); e != nil {
				t.Fatal(e)
			}
		}
	}
	theme.Apply(theme.Light())
}
