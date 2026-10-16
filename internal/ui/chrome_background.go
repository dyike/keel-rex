package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/dyike/keel/gio/op/clip"
)

// Clip only the corner bands with curves. A single window-sized rounded clip
// makes Gio allocate coverage for the whole window, even though almost every
// background pixel lies in its rectangular interior. Keep the same scanline
// colors and antialiased corners, with narrow masks at the top and bottom.
func paintChromeBackground(p painter, size image.Point, radius int, top, bottom color.NRGBA) {
	scale := p.scale
	w, h := float32(size.X)/scale, float32(size.Y)/scale
	radius = min(radius, size.X/2, size.Y/2)
	rows := int(math.Ceil(float64(h)))
	band := func(rect image.Rectangle, rounded clip.RRect, from, to int) {
		area := clip.Rect(rect).Push(p.gtx.Ops)
		defer area.Pop()
		if rounded != (clip.RRect{}) {
			defer rounded.Push(p.gtx.Ops).Pop()
		}
		for y := max(0, from); y < min(rows, to); y++ {
			t := float32(y) / h
			mix := func(a, b uint8) uint8 { return uint8(float32(a)*(1-t) + float32(b)*t) }
			p.rect(0, float32(y), w, 1, 0, color.NRGBA{R: mix(top.R, bottom.R), G: mix(top.G, bottom.G), B: mix(top.B, bottom.B), A: 255})
		}
	}
	if radius == 0 {
		band(image.Rectangle{Max: size}, clip.RRect{}, 0, rows)
		return
	}
	r, y := float64(radius)/float64(scale), float64(size.Y-radius)/float64(scale)
	band(image.Rect(0, 0, size.X, radius), clip.RRect{Rect: image.Rect(0, 0, size.X, 2*radius), NW: radius, NE: radius}, 0, int(math.Ceil(r)))
	band(image.Rect(0, radius, size.X, size.Y-radius), clip.RRect{}, int(math.Floor(r)), int(math.Ceil(y)))
	band(image.Rect(0, size.Y-radius, size.X, size.Y), clip.RRect{Rect: image.Rect(0, size.Y-2*radius, size.X, size.Y), SW: radius, SE: radius}, int(math.Floor(y)), rows)
}
