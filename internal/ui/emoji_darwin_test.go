package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

func TestTerminalLabelRendersColorEmoji(t *testing.T) {
	dir := t.TempDir()
	if artifactDir := os.Getenv("REX_EMOJI_QA_DIR"); artifactDir != "" {
		dir = artifactDir
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, scale := range []float32{1, 2} {
		for _, mono := range []bool{false, true} {
			root := el.Root(el.ViewFunc(func(*el.Context) el.Element {
				return el.Widget(core.Func(func(gtx core.C) core.D {
					p := painter{gtx, gtx.Metric.PxPerDp}
					p.rect(0, 0, 300, 60, 0, rgb(0xffffff))
					p.label("👋 👋🏽 👨‍👩‍👧‍👦", 8, 38, 24, ink, mono, false)
					return core.D{Size: image.Pt(gtx.Dp(300), gtx.Dp(60))}
				}))
			}))
			file := filepath.Join(dir, fmt.Sprintf("emoji-mono-%t-%gx.png", mono, scale))
			if err := window.ScreenshotAtScale(root, 300, 60, scale, file); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			colored := 0
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r > 30000 && g > b+7000 && r > b+7000 {
						colored++
					}
				}
			}
			if colored < 50 {
				t.Fatalf("mono=%v scale=%g: missing color emoji (%d pixels)", mono, scale, colored)
			}
		}
	}
}
