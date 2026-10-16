package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/gio/op/clip"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/window"
)

func TestChromeBackgroundMatchesRoundedGradient(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, scale := range []float32{1, 1.25, 2} {
			for _, rounded := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%gx/rounded=%t", appearance, scale, rounded), func(t *testing.T) {
					a := &app{prefs: defaultPreferences()}
					a.prefs.Appearance = appearance
					colors := a.colors()
					draw := func(optimized bool) core.Widget {
						return core.Func(func(gtx core.C) core.D {
							size := gtx.Constraints.Max
							p := painter{gtx, gtx.Metric.PxPerDp}
							radius := 0
							if rounded {
								radius = int(16 * p.scale)
							}
							if optimized {
								paintChromeBackground(p, size, radius, colors.top, colors.bottom)
							} else {
								defer clip.UniformRRect(image.Rectangle{Max: size}, radius).Push(gtx.Ops).Pop()
								w, h := float32(size.X)/p.scale, float32(size.Y)/p.scale
								for y := float32(0); y < h; y++ {
									t := y / h
									mix := func(a, b uint8) uint8 { return uint8(float32(a)*(1-t) + float32(b)*t) }
									p.rect(0, y, w, 1, 0, color.NRGBA{R: mix(colors.top.R, colors.bottom.R), G: mix(colors.top.G, colors.bottom.G), B: mix(colors.top.B, colors.bottom.B), A: 255})
								}
							}
							return core.D{Size: size}
						})
					}
					before, after := filepath.Join(t.TempDir(), "before.png"), filepath.Join(t.TempDir(), "after.png")
					for path, widget := range map[string]core.Widget{before: draw(false), after: draw(true)} {
						if err := window.ScreenshotAtScale(widget, 373, 213, scale, path); err != nil {
							t.Fatal(err)
						}
					}
					aImg, bImg := readPNG(t, before), readPNG(t, after)
					for y := range aImg.Bounds().Dy() {
						for x := range aImg.Bounds().Dx() {
							ar, ag, ab, aa := aImg.At(x, y).RGBA()
							br, bg, bb, ba := bImg.At(x, y).RGBA()
							for _, pair := range [][2]uint32{{ar, br}, {ag, bg}, {ab, bb}, {aa, ba}} {
								if max(pair[0], pair[1])-min(pair[0], pair[1]) > 2*257 {
									keep(t, before, after)
									t.Fatalf("pixel (%d,%d) changed: %v vs %v", x, y, aImg.At(x, y), bImg.At(x, y))
								}
							}
						}
					}
				})
			}
		}
	}
}
