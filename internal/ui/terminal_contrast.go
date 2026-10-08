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
