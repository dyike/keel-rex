package ui

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

func TestTerminalBlueRemainsReadable(t *testing.T) {
	for _, appearance := range []string{"dark", "light"} {
		a := &app{prefs: defaultPreferences()}
		a.prefs.Appearance = appearance
		key := rowKey{fg: a.colors().text, panel: a.colors().panel, selection: rgb(0x394c65)}
		for _, blue := range []uint32{0x000080, 0x0000ff, 0x2472c8} {
			for _, selected := range []bool{false, true} {
				for _, style := range []uv.Style{
					{Fg: rgb(blue)},
					{Fg: rgb(blue), Bg: rgb(0x000000)},
					{Fg: rgb(0x000000), Bg: rgb(blue), Attrs: uv.AttrReverse},
				} {
					cell := uv.Cell{Content: "directory", Width: 1, Style: style}
					fg, bg := cellColors(&cell, key, selected)
					if bg.A == 0 {
						bg = key.panel
					}
					if contrast(fg, bg) < 4.5 || fg.B <= fg.R || fg.B <= fg.G {
						t.Fatalf("%s blue %06x selected=%v: lost readability or blue hue: %v on %v", appearance, blue, selected, fg, bg)
					}
					if cell.Style != style {
						t.Fatal("rendering modified stored terminal colors")
					}
				}
			}
		}
	}
}

func TestTerminalPreservesIntentionalColors(t *testing.T) {
	key := rowKey{fg: rgb(0xffffff), panel: rgb(0x000000)}
	for _, style := range []uv.Style{
		{Fg: rgb(0x80bfff)}, // Already readable.
		{Fg: rgb(0x000080), Attrs: uv.AttrFaint},
		{Fg: rgb(0x000080), Attrs: uv.AttrConceal},
		{Fg: rgb(0x24272a), Bg: rgb(0x24272a)},
	} {
		cell := uv.Cell{Style: style}
		fg, _ := cellColors(&cell, key, false)
		if fg != style.Fg {
			t.Fatalf("changed intentional color: %v to %v", style, fg)
		}
	}
}
