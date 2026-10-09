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
	"github.com/dyike/keel-rex/internal/backend"
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
				if cell.Style.Fg == nil {
					fg = readableDefaultForeground(fg, bg)
				}
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

func TestTerminalThemeSwitchKeepsInputReadable(t *testing.T) {
	a := &app{prefs: defaultPreferences()}
	term := &terminal{owner: a, cols: 24, rows: 2, rowVersions: []uint64{1, 1}, cells: make([]uv.Cell, 48)}
	backgrounds := []color.NRGBA{rgb(0x373c42), rgb(0xe3e6e8)}
	for row, background := range backgrounds {
		for x := range term.cols {
			cell := uv.Cell{Content: " ", Width: 1, Style: uv.Style{Bg: background}}
			if x < len("Ask Codex anything") {
				cell.Content = string("Ask Codex anything"[x])
			}
			term.cells[row*term.cols+x] = cell
		}
	}
	for i, appearance := range []string{"dark", "light", "dark", "light"} {
		a.prefs.Appearance = appearance
		path := filepath.Join(t.TempDir(), fmt.Sprintf("input-%d-%s.png", i, appearance))
		if err := window.ScreenshotAtScale(fullRowPainter{rowPainter{term}}, 181, 32, 2, path); err != nil {
			t.Fatal(err)
		}
		img := readPNG(t, path)
		for row, background := range backgrounds {
			if pixel := color.NRGBAModel.Convert(img.At(350, row*32+16)).(color.NRGBA); pixel != background {
				t.Fatalf("%s row %d: background %v, want %v", appearance, row, pixel, background)
			}
			best := 0.0
			for y := row * 32; y < (row+1)*32; y++ {
				for x := 0; x < 300; x++ {
					pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					best = max(best, contrast(pixel, background))
				}
			}
			if best < 4.5 {
				t.Fatalf("%s row %d: input text disappeared, contrast %.2f", appearance, row, best)
			}
		}
		keep(t, path)
	}
	// Programs retain control over explicitly colored text, including dim text.
	cell := uv.Cell{Style: uv.Style{Fg: rgb(0x444444), Bg: rgb(0x373c42)}}
	fg, bg := cellColors(&cell, rowKey{fg: ink}, false)
	if fg != rgb(0x444444) || bg != rgb(0x373c42) {
		t.Fatal("overrode explicit ANSI colors")
	}
}

func TestCodexCachedPaletteFollowsAppearance(t *testing.T) {
	for _, startedDark := range []bool{false, true} {
		a := &app{prefs: defaultPreferences()}
		term := &terminal{owner: a, cols: 32, rows: 3, rowVersions: []uint64{1, 1, 1}, cells: make([]uv.Cell, 96), viewFrame: backend.Frame{Program: "codex"}}
		input, gold, green := rgb(0xe3e6e8), rgb(0x85611f), rgb(0x247434)
		if startedDark {
			input, gold, green = rgb(0x373c42), rgb(0xf6dfa6), rgb(0xa6e3a1)
		}
		for row, text := range []string{"Ask Codex to do anything", "GPT-6.1-Sol high", "~/Code/keel-rex"} {
			for col := range term.cols {
				cell := uv.Cell{Content: " ", Width: 1}
				if col < len(text) {
					cell.Content = string(text[col])
				}
				if row == 0 {
					cell.Style.Bg = input
				} else if row == 1 {
					cell.Style.Fg = gold
				} else {
					cell.Style.Fg = green
				}
				term.cells[row*term.cols+col] = cell
			}
		}
		for i, appearance := range []string{"dark", "light", "dark", "light"} {
			a.prefs.Appearance = appearance
			path := filepath.Join(t.TempDir(), fmt.Sprintf("codex-start-dark-%v-switch-%d-%s.png", startedDark, i, appearance))
			if err := window.ScreenshotAtScale(codexRows{fullRowPainter{rowPainter{term}}}, 241, 48, 2, path); err != nil {
				t.Fatal(err)
			}
			img := readPNG(t, path)
			background := color.NRGBAModel.Convert(img.At(470, 16)).(color.NRGBA)
			if (luminance(background) < .4) != (appearance == "dark") {
				t.Fatalf("%s kept the opposite composer background: %v", appearance, background)
			}
			for row := range term.rows {
				bg := a.colors().panel
				if row == 0 {
					bg = background
				}
				best := 0.0
				for y := row*32 + 2; y < (row+1)*32-2; y++ {
					for x := 0; x < 200; x++ {
						pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
						best = max(best, contrast(pixel, bg))
					}
				}
				fg, _ := cellColors(&term.cells[row*term.cols], rowKey{fg: a.colors().text, panel: a.colors().panel, adaptCodex: true}, false)
				if contrast(fg, bg) < 4.5 || best < 4.5 {
					t.Fatalf("%s row %d text lost contrast: color %.2f pixels %.2f", appearance, row, contrast(fg, bg), best)
				}
			}
			keep(t, path)
		}
		before := append([]uv.Cell(nil), term.cells...)
		term.viewFrame.Program = "zsh"
		path := filepath.Join(t.TempDir(), "shell-original-colors.png")
		if err := window.ScreenshotAtScale(fullRowPainter{rowPainter{term}}, 241, 48, 2, path); err != nil {
			t.Fatal(err)
		}
		if term.rowPaints[0].key.adaptCodex {
			t.Fatal("Codex compatibility persisted after returning to the shell")
		}
		for i := range term.cells {
			if term.cells[i].Style != before[i].Style {
				t.Fatal("theme compatibility changed the terminal's stored ANSI colors")
			}
		}
	}
}

type rowPainter struct{ t *terminal }

type codexRows struct{ fullRowPainter }

func (c codexRows) Layout(gtx core.C) core.D {
	scale := gtx.Metric.PxPerDp
	size := gtx.Constraints.Max
	painter{gtx, scale}.rect(0, 0, float32(size.X)/scale, float32(size.Y)/scale, 0, c.t.owner.colors().panel)
	return c.fullRowPainter.Layout(gtx)
}

type fullRowPainter struct{ rowPainter }

func (fullRowPainter) FillsWindow() bool { return true }

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
