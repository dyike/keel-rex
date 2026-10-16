package ui

import (
	"image"
	"image/color"

	"github.com/dyike/keel/gio/op/clip"
	"github.com/dyike/keel/gio/op/paint"
	"github.com/dyike/keel/ui/core"
)

// Large cards need coverage only along their curved edges and border. A full
// rounded fill/stroke makes Gio keep window-sized coverage textures. The four
// disjoint bands retain the same paths, antialiasing and border placement.
func paintPaneSurface(gtx core.C, background, border color.NRGBA) {
	rect := image.Rectangle{Max: gtx.Constraints.Max}
	radius := min(gtx.Dp(10), rect.Dx()/2, rect.Dy()/2)
	bw := gtx.Dp(1)
	band := func(area image.Rectangle, draw func()) {
		defer clip.Rect(area).Push(gtx.Ops).Pop()
		draw()
	}
	rounded := clip.UniformRRect(rect, radius)
	// Include the antialias/stroke fringe past a corner's tangent. A cut
	// exactly at radius can discard a few covered pixels of the stroke.
	corner := min(radius+bw+2, rect.Dy()/2)
	fill := func() { paint.FillShape(gtx.Ops, background, rounded.Op(gtx.Ops)) }
	band(image.Rect(0, 0, rect.Max.X, corner), fill)
	paint.FillShape(gtx.Ops, background, clip.Rect(image.Rect(0, corner, rect.Max.X, rect.Max.Y-corner)).Op())
	band(image.Rect(0, rect.Max.Y-corner, rect.Max.X, rect.Max.Y), fill)
	if bw == 0 {
		return
	}
	half := bw / 2
	inner := image.Rectangle{Min: image.Pt(half, half), Max: rect.Max.Sub(image.Pt(bw-half, bw-half))}
	path := clip.UniformRRect(inner, max(0, radius-half)).Path(gtx.Ops)
	stroke := func() { paint.FillShape(gtx.Ops, border, clip.Stroke{Path: path, Width: float32(bw)}.Op()) }
	band(image.Rect(0, 0, rect.Max.X, corner), stroke)
	band(image.Rect(0, rect.Max.Y-corner, rect.Max.X, rect.Max.Y), stroke)
	band(image.Rect(0, corner, bw+2, rect.Max.Y-corner), stroke)
	band(image.Rect(rect.Max.X-bw-2, corner, rect.Max.X, rect.Max.Y-corner), stroke)
}
