package ui

import (
	"image/color"
	"math"
)

var linearChannel = func() [256]float64 {
	var channels [256]float64
	for i := range channels {
		v := float64(i) / 255
		if v <= .04045 {
			channels[i] = v / 12.92
		} else {
			channels[i] = math.Pow((v+.055)/1.055, 2.4)
		}
	}
	return channels
}()

func luminance(c color.NRGBA) float64 {
	return .2126*linearChannel[c.R] + .7152*linearChannel[c.G] + .0722*linearChannel[c.B]
}

func contrast(a, b color.NRGBA) float64 {
	x, y := luminance(a), luminance(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}

func readableDefaultForeground(fg, bg color.NRGBA) color.NRGBA {
	if bg.A != 255 || contrast(fg, bg) >= 4.5 {
		return fg
	}
	dark, light := rgb(0x272d30), rgb(0xe0e4e8)
	if contrast(light, bg) > contrast(dark, bg) {
		return light
	}
	return dark
}

// Remap stale neutral composer/message backgrounds only when they belong to
// the opposite appearance. Colored diff and syntax backgrounds stay intact.
func codexBackground(bg, panel color.NRGBA) color.NRGBA {
	if bg.A != 255 || max(bg.R, bg.G, bg.B)-min(bg.R, bg.G, bg.B) > 32 {
		return bg
	}
	if (luminance(bg) < .4) == (luminance(panel) < .4) {
		return bg
	}
	if luminance(panel) < .4 {
		return rgb(0x373c42)
	}
	return rgb(0xe8ebed)
}

// Keep hue while giving cached Codex footer colors enough contrast on the
// current surface. Other terminal programs retain their exact ANSI colors.
func readableCodexForeground(fg, bg color.NRGBA) color.NRGBA {
	if bg.A != 255 || contrast(fg, bg) >= 4.5 {
		return fg
	}
	target := rgb(0x000000)
	if contrast(rgb(0xffffff), bg) > contrast(target, bg) {
		target = rgb(0xffffff)
	}
	for step := 1; step <= 20; step++ {
		mix := func(a, b uint8) uint8 { return uint8((int(a)*(20-step) + int(b)*step) / 20) }
		candidate := color.NRGBA{R: mix(fg.R, target.R), G: mix(fg.G, target.G), B: mix(fg.B, target.B), A: fg.A}
		if contrast(candidate, bg) >= 4.5 {
			return candidate
		}
	}
	return target
}
