package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"image/color"
	"strings"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/op"
	"gioui.org/text"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/theme"
	"golang.org/x/image/math/fixed"
)

// text is the text of the screen, for screen readers.
func (t *terminal) text() string {
	var b strings.Builder
	for y := 0; y < t.rows && (y+1)*t.cols <= len(t.cells); y++ {
		b.WriteString(backend.RowText(t.cells[y*t.cols : (y+1)*t.cols]))
		b.WriteByte('\n')
	}
	return b.String()
}

// rowPaint is the drawing of a row, kept while nothing it shows changes, so
// that a frame replays the rows that did not change instead of shaping their
// text again, and Gio reuses their outlines.
type rowPaint struct {
	key   rowKey
	ok    bool
	ops   op.Ops
	call  op.CallOp
	runes []int
	glyph []text.Glyph
	runs  []theme.GlyphRun
}

// rowKey is everything a row's drawing depends on.
type rowKey struct {
	version              uint64
	cols                 int
	cw, ch, size, scale  float32
	fg, panel, selection color.NRGBA
	selLo, selHi         int // selected columns, -1 when none
	adaptCodex           bool
	shaper               *text.Shaper
}

// paintRows draws the cells, row by row, replaying the rows that did not
// change since the last frame.
func (t *terminal) paintRows(gtx core.C, scale, cw, ch, size float32) {
	if len(t.rowPaints) != t.rows {
		t.rowPaints = make([]rowPaint, t.rows)
	}
	fg, panel := ink, color.NRGBA{}
	if t.owner != nil {
		fg, panel = t.owner.colors().text, t.owner.colors().panel
	}
	selection := rgb(0xd6e4fa)
	if t.owner != nil && (t.owner.prefs.Appearance == "dark" || t.owner.prefs.Appearance == "system" && t.owner.appearanceDark) {
		selection = rgb(0x394c65)
	}
	lo, hi := min(t.anchor, t.caret), max(t.anchor, t.caret)
	selecting := t.hasSelection || hi > lo
	// Codex caches its terminal palette at startup. Its explicit composer
	// backgrounds and footer colors can outlive a terminal theme change.
	adaptCodex := strings.EqualFold(t.viewFrame.Program, "codex")
	t.glyphRenderer.BeginFrame(theme.Material.Shaper)
	for y := 0; y < t.rows; y++ {
		key := rowKey{version: t.rowVersions[y], cols: t.cols, cw: cw, ch: ch, size: size, scale: scale, fg: fg, panel: panel, selection: selection, selLo: -1, selHi: -1}
		key.adaptCodex = adaptCodex
		key.shaper = theme.Material.Shaper
		if start, end := (t.viewStart+y)*t.cols, (t.viewStart+y+1)*t.cols-1; selecting && hi >= start && lo <= end {
			key.selLo, key.selHi = max(lo, start)-start, min(hi, end)-start
		}
		rp := &t.rowPaints[y]
		if !rp.ok || rp.key != key {
			rp.key, rp.ok = key, false
			t.prepareRow(rp, y, key)
		}
		for _, run := range rp.runs {
			t.glyphRenderer.Prepare(run)
		}
	}
	t.glyphRenderer.Commit()
	for y := 0; y < t.rows; y++ {
		rp := &t.rowPaints[y]
		if !rp.ok {
			rp.ops.Reset()
			rec := gtx
			rec.Ops = &rp.ops
			m := op.Record(rec.Ops)
			t.paintRow(painter{rec, scale}, rp, y, rp.key)
			rp.call = m.Stop()
			rp.ok = true
		}
		rp.call.Add(gtx.Ops)
	}
}

// cellColors returns the colors a cell is drawn in.
func cellColors(c *uv.Cell, key rowKey, selected bool) (fg, bg color.NRGBA) {
	fg = key.fg
	if c.Style.Fg != nil {
		fg = color.NRGBAModel.Convert(c.Style.Fg).(color.NRGBA)
	}
	if c.Style.Bg != nil {
		bg = color.NRGBAModel.Convert(c.Style.Bg).(color.NRGBA)
		if key.adaptCodex {
			bg = codexBackground(bg, key.panel)
		}
		if c.Style.Fg == nil {
			// A TUI can retain its explicit input background across theme
			// changes while its default foreground follows the terminal.
			fg = readableDefaultForeground(fg, bg)
		}
	}
	if c.Style.Attrs&uv.AttrReverse != 0 {
		if bg.A == 0 {
			bg = key.panel
		}
		fg, bg = bg, fg
	}
	if selected {
		bg = key.selection
		if c.Style.Fg == nil && c.Style.Attrs&uv.AttrReverse == 0 {
			fg = readableDefaultForeground(key.fg, bg)
		}
	}
	if key.adaptCodex {
		background := bg
		if background.A == 0 {
			background = key.panel
		}
		fg = readableCodexForeground(fg, background)
	}
	return fg, bg
}

// paintRow draws row y: its backgrounds, then its text in runs of one color
// and weight, each glyph in its cell, then its underlines.
func (t *terminal) paintRow(p painter, rp *rowPaint, y int, key rowKey) {
	row := t.cells[y*t.cols : (y+1)*t.cols]
	cw, ch := key.cw, key.ch
	yy := float32(y) * ch
	selected := func(x int) bool { return key.selLo >= 0 && x >= key.selLo && x <= key.selHi }
	// Backgrounds, a rectangle per run of one color.
	for x := 0; x < len(row); {
		_, bg := cellColors(&row[x], key, selected(x))
		end := x + max(1, row[x].Width)
		for end < len(row) {
			if _, b := cellColors(&row[end], key, selected(end)); b != bg {
				break
			}
			end += max(1, row[end].Width)
		}
		if bg.A != 0 {
			p.rect(float32(x)*cw, yy, cw*float32(min(end, len(row))-x), ch, 0, bg)
		}
		x = end
	}
	for _, run := range rp.runs {
		t.glyphRenderer.Paint(p.gtx.Ops, run)
	}
	// Underlines, a line per run of one color.
	for x := 0; x < len(row); {
		if row[x].Style.Underline == 0 {
			x++
			continue
		}
		fg, _ := cellColors(&row[x], key, selected(x))
		end := x + 1
		for end < len(row) && row[end].Style.Underline != 0 {
			if f, _ := cellColors(&row[end], key, selected(end)); f != fg {
				break
			}
			end++
		}
		p.line(float32(x)*cw, yy+ch-2, cw*float32(end-x), .7, fg)
		x = end
	}
}

// Prepare all visible runs before painting so Keel can upload changed atlas
// pages once per frame. Retain row glyphs while the row's inputs are unchanged.
func (t *terminal) prepareRow(rp *rowPaint, y int, key rowKey) {
	rp.glyph = rp.glyph[:0]
	rp.runs = rp.runs[:0]
	row := t.cells[y*t.cols : (y+1)*t.cols]
	selected := func(x int) bool { return key.selLo >= 0 && x >= key.selLo && x <= key.selHi }
	drawn := func(c *uv.Cell) bool { return c.Width > 0 && c.Content != "" && c.Content != " " }
	for x := 0; x < len(row); {
		c := &row[x]
		if !drawn(c) {
			x++
			continue
		}
		fg, _ := cellColors(c, key, selected(x))
		bold := c.Style.Attrs&uv.AttrBold != 0
		end, last := x, x
		for end < len(row) {
			d := &row[end]
			if d.Width == 0 || d.Content == "" || d.Content == " " {
				end++
				continue
			}
			if f, _ := cellColors(d, key, selected(end)); f != fg || (d.Style.Attrs&uv.AttrBold != 0) != bold {
				break
			}
			last = end
			end++
		}
		rp.prepareRun(row[x:last+1], float32(x)*key.cw, float32(y)*key.ch+key.size, key, fg, bold)
		x = last + 1
	}
}

// prepareRun shapes the text of cells, from x and the baseline y, as label does
// for one cell, with each glyph moved to its cell: CJK glyphs are narrower
// than two cells and emoji wider than their font's advance.
func (rp *rowPaint) prepareRun(cells []uv.Cell, x, y float32, key rowKey, c color.NRGBA, bold bool) {
	var b strings.Builder
	rp.runes = rp.runes[:0]
	for i := range cells {
		cell := &cells[i]
		if cell.Width == 0 || cell.Content == "" {
			continue
		}
		b.WriteString(cell.Content)
		for range utf8.RuneCountInString(cell.Content) {
			rp.runes = append(rp.runes, i)
		}
	}
	weight := font.Normal
	if bold {
		weight = font.Bold
	}
	sh := theme.Material.Shaper
	px := key.size * key.scale
	params := text.Parameters{Font: font.Font{Typeface: terminalFontFace + ", " + theme.EmojiFace, Weight: weight}, PxPerEm: fixed.Int26_6(px * 64), MaxWidth: 1 << 24}
	sh.LayoutString(params, b.String())
	offset := len(rp.glyph)
	gs := rp.glyph
	for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
		gs = append(gs, g)
	}
	rp.glyph = gs
	gs = gs[offset:]
	if len(gs) == 0 {
		return
	}
	// Each cluster starts at its cell; glyphs within a cluster keep their
	// places relative to its first.
	r, start := 0, 0
	for i := range gs {
		if gs[i].Flags&text.FlagClusterBreak == 0 {
			continue
		}
		col := 0
		if r < len(rp.runes) {
			col = rp.runes[r]
		}
		base := gs[start].X
		at := fixed.Int26_6(float32(col) * key.cw * key.scale * 64)
		for k := start; k <= i; k++ {
			gs[k].X = at + gs[k].X - base
		}
		r += int(gs[i].Runes)
		start = i + 1
	}
	rp.runs = append(rp.runs, theme.GlyphRun{Params: params, Glyphs: gs, Color: c, Position: f32.Pt(x*key.scale, y*key.scale)})
}
