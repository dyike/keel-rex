package ui

import (
	"image/color"

	"gioui.org/f32"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Keyboard symbols share an optical grid and a 1.3dp stroke at 12dp.
// Draw paths directly so stroke weight scales with display density.
func (p painter) shortcutSymbol(name string, x, y, size float32, c color.NRGBA) {
	pt := func(xx, yy float32) f32.Point { return f32.Pt(x+xx*size/24, y+yy*size/24) }
	stroke := func(coords ...float32) {
		points := make([]f32.Point, 0, len(coords)/2)
		for i := 0; i < len(coords); i += 2 {
			points = append(points, pt(coords[i], coords[i+1]))
		}
		p.stroke(c, 2.6*size/24, points...)
	}
	switch name {
	case "shift":
		stroke(12, 3, 3, 12, 8, 12, 8, 21, 16, 21, 16, 12, 21, 12, 12, 3)
	case "control":
		stroke(3, 16, 12, 7, 21, 16)
	case "option":
		stroke(3, 5, 9, 5, 15, 19, 21, 19)
		stroke(15, 5, 21, 5)
	case "return":
		stroke(21, 4, 21, 15, 3, 15)
		stroke(9, 9, 3, 15, 9, 21)
	case "arrow-left":
		stroke(21, 12, 3, 12)
		stroke(12, 3, 3, 12, 12, 21)
	case "arrow-right":
		stroke(3, 12, 21, 12)
		stroke(12, 3, 21, 12, 12, 21)
	case "arrow-up":
		stroke(12, 21, 12, 3)
		stroke(3, 12, 12, 3, 21, 12)
	case "arrow-down":
		stroke(12, 3, 12, 21)
		stroke(3, 12, 12, 21, 21, 12)
	case "command":
		px := func(xx, yy float32) f32.Point {
			v := pt(xx, yy)
			return f32.Pt(v.X*p.scale, v.Y*p.scale)
		}
		var path clip.Path
		path.Begin(p.gtx.Ops)
		path.MoveTo(px(15, 6))
		path.LineTo(px(15, 18))
		path.CubeTo(px(15, 19.657), px(16.343, 21), px(18, 21))
		path.CubeTo(px(19.657, 21), px(21, 19.657), px(21, 18))
		path.CubeTo(px(21, 16.343), px(19.657, 15), px(18, 15))
		path.LineTo(px(6, 15))
		path.CubeTo(px(4.343, 15), px(3, 16.343), px(3, 18))
		path.CubeTo(px(3, 19.657), px(4.343, 21), px(6, 21))
		path.CubeTo(px(7.657, 21), px(9, 19.657), px(9, 18))
		path.LineTo(px(9, 6))
		path.CubeTo(px(9, 4.343), px(7.657, 3), px(6, 3))
		path.CubeTo(px(4.343, 3), px(3, 4.343), px(3, 6))
		path.CubeTo(px(3, 7.657), px(4.343, 9), px(6, 9))
		path.LineTo(px(18, 9))
		path.CubeTo(px(19.657, 9), px(21, 7.657), px(21, 6))
		path.CubeTo(px(21, 4.343), px(19.657, 3), px(18, 3))
		path.CubeTo(px(16.343, 3), px(15, 4.343), px(15, 6))
		path.Close()
		paint.FillShape(p.gtx.Ops, c, clip.Stroke{Path: path.End(), Width: 2.6 * size / 24 * p.scale}.Op())
	}
}
