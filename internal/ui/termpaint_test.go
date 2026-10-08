package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/window"
)

// cellPainter draws a terminal's cells one by one, as the terminal did
// before it drew rows: the reference the row drawing must match.
type cellPainter struct{ t *terminal }

func (c cellPainter) Layout(gtx core.C) core.D {
	t := c.t
	scale := gtx.Metric.PxPerDp
	size := t.owner.prefs.FontSize
	cw, ch := size*.60208, size*1.28
	p := painter{gtx, scale}
	lo, hi := min(t.anchor, t.caret), max(t.anchor, t.caret)
	for y := 0; y < t.rows; y++ {
		for x := 0; x < t.cols; x++ {
			i := y*t.cols + x
			cell := t.cells[i]
			fg, bg := t.owner.colors().text, color.NRGBA{}
			if cell.Style.Fg != nil {
				fg = color.NRGBAModel.Convert(cell.Style.Fg).(color.NRGBA)
			}
			if cell.Style.Bg != nil {
				bg = color.NRGBAModel.Convert(cell.Style.Bg).(color.NRGBA)
			}
			if cell.Style.Attrs&uv.AttrReverse != 0 {
				if bg.A == 0 {
					bg = t.owner.colors().panel
				}
				fg, bg = bg, fg
			}
			if (t.hasSelection || hi > lo) && i >= lo && i <= hi {
				bg = rgb(0xd6e4fa)
			}
			xx, yy := float32(x)*cw, float32(y)*ch
			if bg.A != 0 {
				p.rect(xx, yy, cw*float32(max(1, cell.Width)), ch, 0, bg)
			}
			if cell.Width > 0 && cell.Content != "" && cell.Content != " " {
				p.label(cell.Content, xx, yy+size, size, fg, true, cell.Style.Attrs&uv.AttrBold != 0)
			}
			if cell.Style.Underline != 0 {
				p.line(xx, yy+ch-2, cw, .7, fg)
			}
		}
	}
	return core.D{Size: gtx.Constraints.Max}
}

type rowPainter struct{ t *terminal }

func (r rowPainter) Layout(gtx core.C) core.D {
	t := r.t
	size := t.owner.prefs.FontSize
	t.paintRows(gtx, gtx.Metric.PxPerDp, size*.60208, size*1.28, size)
	return core.D{Size: gtx.Constraints.Max}
}

func readPNG(t *testing.T, path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// keep copies the drawings to $REX_PAINT_QA_DIR, to look at.
func keep(t *testing.T, paths ...string) {
	out := os.Getenv("REX_PAINT_QA_DIR")
	if out == "" {
		return
	}
	os.MkdirAll(out, 0755)
	for _, p := range paths {
		data, _ := os.ReadFile(p)
		os.WriteFile(filepath.Join(out, filepath.Base(p)), data, 0644)
	}
}

// Drawing rows in runs of text puts the same text in each cell as drawing
// each cell did.
func TestRowPaintMatchesCellPaint(t *testing.T) {
	s := outputSession(t, 100)
	a := &app{prefs: defaultPreferences()}
	term := &terminal{session: s, owner: a, cols: 60, rows: 8}
	s.Resize(60, 8)
	feedOutput(t, s, "\x1b[2J\x1b[Hplain text, ok: {}[]()<>=+-*/\r\n"+
		"\x1b[31mred\x1b[0m \x1b[1;32mbold green\x1b[0m \x1b[4munderlined\x1b[0m \x1b[7mreverse\x1b[0m\r\n"+
		"\x1b[44m blue background \x1b[0m 中文宽字符 混合 abc 终端\r\n"+
		"box ─┼─ █▓▒░ \x1b[38;2;200;120;40mtrue color\x1b[0m \x1b[38;5;99m256 color\x1b[0m e\u0301 combining\r\n"+
		"emoji 🚀 ✅ 🔥 after\r\n")
	s.SyncRows(0, term.cols, term.rows, &term.cells, &term.rowVersions)
	term.anchor, term.caret, term.hasSelection = 3*60+2, 4*60+12, true
	dir := t.TempDir()
	for _, scale := range []float32{1, 2} {
		a, b := filepath.Join(dir, fmt.Sprintf("cells-%g.png", scale)), filepath.Join(dir, fmt.Sprintf("rows-%g.png", scale))
		if err := window.ScreenshotAtScale(cellPainter{term}, 480, 140, scale, a); err != nil {
			t.Fatal(err)
		}
		if err := window.ScreenshotAtScale(rowPainter{term}, 480, 140, scale, b); err != nil {
			t.Fatal(err)
		}
		ia, ib := readPNG(t, a), readPNG(t, b)
		// Glyphs may sit a fraction of a pixel apart: Gio places the glyphs
		// of a run on its own grid, where a cell alone is moved by a
		// transform. So compare what each cell holds, its mean color.
		cw, ch := 12.5*.60208*float64(scale), 12.5*1.28*float64(scale)
		mean := func(img image.Image, col, row int) [3]float64 {
			var sum [3]float64
			n := 0.0
			for y := int(float64(row) * ch); y < int(float64(row+1)*ch); y++ {
				for x := int(float64(col) * cw); x < int(float64(col+1)*cw); x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					sum[0], sum[1], sum[2] = sum[0]+float64(r>>8), sum[1]+float64(g>>8), sum[2]+float64(b>>8)
					n++
				}
			}
			return [3]float64{sum[0] / n, sum[1] / n, sum[2] / n}
		}
		worst := 0.0
		for row := 0; row < term.rows; row++ {
			if row >= 4 {
				continue // the emoji, and what of them reaches below: see below
			}
			for col := 0; col < term.cols; col++ {
				ma, mb := mean(ia, col, row), mean(ib, col, row)
				for k := range 3 {
					if d := math.Abs(ma[k] - mb[k]); d > 12 {
						t.Logf("%gx: cell %d,%d %q differs by %.1f", scale, col, row, term.cells[row*term.cols+col].Content, d)
					}
					worst = max(worst, math.Abs(ma[k]-mb[k]))
				}
			}
		}
		if worst > 12 {
			keep(t, a, b)
			t.Fatalf("%gx: a cell's mean color differs by %.1f between cell and row drawing", scale, worst)
		}
		// The emoji are drawn whole, in color.
		colorful := 0
		for y := int(4 * ch); y < int(6*ch); y++ {
			for x := int(6 * cw); x < int(14*cw); x++ {
				r, g, b, _ := ib.At(x, y).RGBA()
				if max(r, g, b)-min(r, g, b) > 0x6000 {
					colorful++
				}
			}
		}
		if colorful < int(20*scale*scale) {
			keep(t, a, b)
			t.Fatalf("%gx: emoji missing from the row drawing (%d colored pixels)", scale, colorful)
		}
	}
}
