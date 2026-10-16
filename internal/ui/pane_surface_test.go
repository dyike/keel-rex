package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

type paneSurfaceScene struct {
	optimized  bool
	bg, border color.NRGBA
}

func (s paneSurfaceScene) Render(cx *el.Context) el.Element {
	w, h := cx.ViewportSize()
	content := el.Div().P(10).Gap(7)
	var card *el.DivEl
	if s.optimized {
		content.Grow().H(el.Dp(0))
		card = el.Div().W(el.Dp(w)).H(el.Dp(h)).P(1).Rounded(10).Decorate(func(gtx core.C, draw func()) {
			paintPaneSurface(gtx, s.bg, s.border)
			draw()
		}).Child(content)
	} else {
		card = content.W(el.Dp(w)).H(el.Dp(h)).Bg(s.bg).Border(1, s.border).Rounded(10)
	}
	// Colored children make even a one-pixel change in padding, gap or the
	// available terminal viewport visible to the pixel comparison.
	content.Child(el.Div().H(el.Dp(26)).NoShrink().Bg(rgb(0x6283a4)), el.Div().Grow().H(el.Dp(0)).Bg(rgb(0x6a926d)))
	// A real card is below the workspace root. Avoid Root's automatic theme
	// background on the card itself, which would cover its custom decorator.
	return el.Div().W(el.Dp(w)).H(el.Dp(h)).Bg(color.NRGBA{}).Child(card)
}

func TestPaneSurfacePreservesPixelsAndInsets(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.75, 2} {
		for _, dark := range []bool{false, true} {
			for _, focused := range []bool{false, true} {
				t.Run(fmt.Sprintf("%gx/dark=%t/focused=%t", scale, dark, focused), func(t *testing.T) {
					a := &app{prefs: defaultPreferences()}
					if dark {
						a.prefs.Appearance = "dark"
					}
					colors := a.colors()
					border := colors.border
					if focused {
						border = rgb(0x92b3ad)
					}
					s := paneSurfaceScene{bg: colors.panel, border: border}
					before, after := filepath.Join(t.TempDir(), "pane-before.png"), filepath.Join(t.TempDir(), "pane-after.png")
					if err := window.ScreenshotAtScale(el.Root(s), 373, 213, scale, before); err != nil {
						t.Fatal(err)
					}
					s.optimized = true
					if err := window.ScreenshotAtScale(el.Root(s), 373, 213, scale, after); err != nil {
						t.Fatal(err)
					}
					aImg, bImg := readPNG(t, before), readPNG(t, after)
					// Re-clipping the same curve changes GPU coverage rounding by at
					// most 3/255 at antialiased edges. Content and insets stay exact.
					edge := int(14*scale) + 1
					var worst uint32
					var worstX, worstY int
					for y := range aImg.Bounds().Dy() {
						for x := range aImg.Bounds().Dx() {
							if x >= edge && x < aImg.Bounds().Dx()-edge && y >= edge && y < aImg.Bounds().Dy()-edge && aImg.At(x, y) != bImg.At(x, y) {
								t.Fatalf("interior pixel (%d,%d) changed: %v vs %v", x, y, aImg.At(x, y), bImg.At(x, y))
							}
							ar, ag, ab, aa := aImg.At(x, y).RGBA()
							br, bg, bb, ba := bImg.At(x, y).RGBA()
							for _, pair := range [][2]uint32{{ar, br}, {ag, bg}, {ab, bb}, {aa, ba}} {
								if diff := max(pair[0], pair[1]) - min(pair[0], pair[1]); diff > worst {
									worst, worstX, worstY = diff, x, y
								}
							}
						}
					}
					if worst > 3*257 {
						keep(t, before, after)
						t.Fatalf("worst pixel (%d,%d), difference %.2f: %v vs %v", worstX, worstY, float64(worst)/257, aImg.At(worstX, worstY), bImg.At(worstX, worstY))
					}
				})
			}
		}
	}
}
