//go:build darwin

package ui

import (
	"sync"

	"github.com/dyike/keel/gio/font"
	"github.com/dyike/keel/ui/theme"
)

func prepareDarwinAppFonts(paths []string) (func() error, error) {
	// Keel preserves original TTC identities and prepares native font metadata
	// without eagerly parsing glyph/AAT tables. The keep callback runs in parallel.
	var mu sync.Mutex
	families := make(map[font.Typeface]bool)
	set, err := theme.PrepareFontFiles(func(f font.Font) bool {
		if !keepAppFont(f) {
			return false
		}
		mu.Lock()
		families[f.Typeface] = true
		mu.Unlock()
		return true
	}, paths...)
	if err != nil {
		return nil, err
	}
	primary := "PingFang SC"
	for _, family := range []font.Typeface{"PingFang SC", "Hiragino Sans GB", "Heiti SC"} {
		if families[family] {
			primary = string(family)
			break
		}
	}
	return func() error {
		set.Use()
		uiFontFace, terminalFontFace = darwinFontFamilies(primary)
		theme.Material.Face = uiFontFace
		return nil
	}, nil
}
