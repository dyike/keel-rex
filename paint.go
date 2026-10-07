package main

import (
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/theme"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"math"
)

var ink = rgb(0x272d30)
var gray = rgb(0x646767)
var cyan = rgb(0x51afbf)
var green = rgb(0x429266)
var red = rgb(0xd34365)
var blue = rgb(0x239dff)

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

type painter struct {
	gtx   core.C
	scale float32
}

func (p painter) rect(x, y, w, h, r float32, c color.NRGBA) {
	q := func(v float32) int { return int(math.Round(float64(v * p.scale))) }
	paint.FillShape(p.gtx.Ops, c, clip.RRect{Rect: image.Rect(q(x), q(y), q(x+w), q(y+h)), NE: q(r), NW: q(r), SE: q(r), SW: q(r)}.Op(p.gtx.Ops))
}
func (p painter) line(x, y, w, h float32, c color.NRGBA) { p.rect(x, y, w, h, 0, c) }
func (p painter) label(s string, x, y, size float32, c color.NRGBA, mono, bold bool) float32 {
	fam := font.Typeface("Helvetica Neue")
	if mono {
		fam = "Menlo"
	}
	fam += ", " + theme.EmojiFace
	weight := font.Normal
	if bold {
		weight = font.Bold
	}
	sh := theme.Material.Shaper
	px := size * p.scale
	sh.LayoutString(text.Parameters{Font: font.Font{Typeface: fam, Weight: weight}, PxPerEm: fixed.Int26_6(px * 64), MaxWidth: 10000}, s)
	var gs []text.Glyph
	for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
		gs = append(gs, g)
	}
	if len(gs) == 0 {
		return 0
	}
	tr := op.Affine(f32.AffineId().Offset(f32.Pt(x*p.scale, y*p.scale))).Push(p.gtx.Ops)
	paint.ColorOp{Color: c}.Add(p.gtx.Ops)
	shape := clip.Outline{Path: sh.Shape(gs)}.Op().Push(p.gtx.Ops)
	paint.PaintOp{}.Add(p.gtx.Ops)
	shape.Pop()
	if bitmap := sh.Bitmaps(gs); bitmap != (op.CallOp{}) {
		bitmap.Add(p.gtx.Ops)
	}
	tr.Pop()
	return float32(gs[len(gs)-1].X+gs[len(gs)-1].Advance) / 64 / p.scale
}

// UI symbols are vector paths: only their geometry is painted, so there is
// no screenshot background or bitmap fringe at any display scale.
func (p painter) stroke(c color.NRGBA, width float32, points ...f32.Point) {
	if len(points) < 2 {
		return
	}
	var path clip.Path
	path.Begin(p.gtx.Ops)
	path.MoveTo(f32.Pt(points[0].X*p.scale, points[0].Y*p.scale))
	for _, pt := range points[1:] {
		path.LineTo(f32.Pt(pt.X*p.scale, pt.Y*p.scale))
	}
	paint.FillShape(p.gtx.Ops, c, clip.Stroke{Path: path.End(), Width: width * p.scale}.Op())
	for _, v := range points {
		p.disk(v.X, v.Y, width/2, c)
	}
}
func (p painter) trafficLight(kind int, x, y float32, hover bool) {
	fills := []uint32{0xff6058, 0xffbd2e, 0x28c840}
	borders := []uint32{0xe14942, 0xdca020, 0x20a932}
	fill, border := rgb(fills[kind]), rgb(borders[kind])
	if w := core.CurrentWindow(); w != nil && !w.Focused() {
		fill, border = rgb(0xd2cbd3), rgb(0xbcb5be)
	}
	p.rect(x, y, 14, 14, 7, border)
	p.rect(x+.5, y+.5, 13, 13, 6.5, fill)
	if !hover {
		return
	}
	col := rgb(0x60432b)
	switch kind {
	case 0:
		p.stroke(col, 1.1, f32.Pt(x+4, y+4), f32.Pt(x+10, y+10))
		p.stroke(col, 1.1, f32.Pt(x+10, y+4), f32.Pt(x+4, y+10))
	case 1:
		p.stroke(col, 1.2, f32.Pt(x+3.5, y+7), f32.Pt(x+10.5, y+7))
	case 2:
		p.stroke(rgb(0x155923), 1.1, f32.Pt(x+4, y+8), f32.Pt(x+4, y+4), f32.Pt(x+8, y+4))
		p.stroke(rgb(0x155923), 1.1, f32.Pt(x+6, y+10), f32.Pt(x+10, y+10), f32.Pt(x+10, y+6))
	}
}
func (p painter) programStack(tab *workspace, x, y float32) {
	var icons []string
	tab.root.each(func(n *pane) {
		if n != tab.focus && len(icons) < 2 {
			icon := "git"
			if n.term != nil {
				icon = programIcon(n.term.session)
			}
			icons = append(icons, icon)
		}
	})
	front := "git"
	if tab.focus == nil {
		front = "fish"
	} else if tab.focus.term != nil {
		front = programIcon(tab.focus.term.session)
	}
	icons = append([]string{front}, icons...)
	for i := len(icons) - 1; i >= 0; i-- {
		xx := x + float32(i)*5
		yy, w, h := y, float32(24), float32(20)
		if i > 0 {
			yy, w, h = y+1, 22, 18
		}
		p.rect(xx, yy+.75, w, h, 5, color.NRGBA{A: 28})
		p.programTile(icons[i], xx, yy, w, h)
	}
}
