package ui

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"gioui.org/font"
	"github.com/dyike/keel/ui/theme"
)

// prepareAppFonts returns an installer to run on the UI thread before its first
// frame. Windows text fonts need their legacy bitmap strikes disabled: Gio
// skips BlackAndWhite bitmaps, even when the glyph also has a usable outline.
func prepareAppFonts(platform string, paths []string) (func() error, error) {
	keep := keepAppFont
	if platform != "windows" {
		set, err := theme.PrepareFontFiles(keep, paths...)
		if err != nil {
			return nil, err
		}
		return func() error { set.Use(); return nil }, nil
	}
	var files [][]byte
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := preferFontOutlines(data); err != nil {
			return nil, fmt.Errorf("font %s: %w", path, err)
		}
		files = append(files, data)
	}
	return func() error { return theme.LoadFontsWhere(keep, files...) }, nil
}

// Avoid loading every regional face of a large system TTC collection.
func keepAppFont(f font.Font) bool {
	if strings.HasPrefix(string(f.Typeface), "PingFang") {
		return f.Typeface == "PingFang SC" && (f.Weight == font.Normal || f.Weight == font.SemiBold)
	}
	switch f.Typeface {
	case "Heiti TC", ".Hiragino Sans GB Interface":
		return false
	case "Hiragino Sans GB":
		return f.Weight == font.Light || f.Weight == font.SemiBold
	case "Heiti SC":
		return f.Weight == font.Normal
	default:
		return true
	}
}

// Only the in-memory SFNT table directory is changed; installed font files
// remain untouched. Keep color emoji tables and bitmap-only fonts intact.
func preferFontOutlines(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("truncated font header")
	}
	offsets := []uint32{0}
	if string(data[:4]) == "ttcf" {
		count := uint64(binary.BigEndian.Uint32(data[8:12]))
		if count > uint64(len(data)-12)/4 {
			return fmt.Errorf("truncated font collection")
		}
		offsets = nil
		for i := uint64(0); i < count; i++ {
			offsets = append(offsets, binary.BigEndian.Uint32(data[12+4*i:16+4*i]))
		}
	}
	for _, offset := range offsets {
		start := uint64(offset)
		if start+12 > uint64(len(data)) {
			return fmt.Errorf("invalid font directory offset")
		}
		count := uint64(binary.BigEndian.Uint16(data[start+4 : start+6]))
		if start+12+16*count > uint64(len(data)) {
			return fmt.Errorf("truncated font directory")
		}
		hasOutline := false
		var bitmapTags []uint64
		for i := uint64(0); i < count; i++ {
			at := start + 12 + 16*i
			switch string(data[at : at+4]) {
			case "glyf", "CFF ", "CFF2":
				hasOutline = true
			case "EBDT", "EBLC", "EBSC", "bdat", "bloc":
				bitmapTags = append(bitmapTags, at)
			}
		}
		if hasOutline {
			for _, at := range bitmapTags {
				// Unknown tags are ignored by the OpenType parser. Their table
				// offsets and shared TTC data do not need to be rewritten.
				copy(data[at:at+4], "Rex ")
			}
		}
	}
	return nil
}
