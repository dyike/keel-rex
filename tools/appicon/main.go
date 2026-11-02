// Command appicon rasterizes the editable app mark for platform packaging.
// Run from the repository root: go run ./tools/appicon
package main

import (
	"image"
	"image/png"
	"log"
	"os"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func main() {
	icon, err := oksvg.ReadIcon("assets/keel-icon.svg", oksvg.StrictErrorMode)
	if err != nil {
		log.Fatal(err)
	}
	const size = 1024
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.SetTarget(0, 0, size, size)
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)
	f, err := os.Create("assets/keel-icon.png")
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
