package ui

import (
	"os"
	"path/filepath"
	"runtime"

	"gioui.org/font"
)

var uiFontFace, terminalFontFace = platformFontFamilies(runtime.GOOS)

func platformFontFamilies(platform string) (font.Typeface, font.Typeface) {
	switch platform {
	case "darwin":
		// An explicit CJK family keeps normal and bold text out of incomplete
		// generic fallback faces. All of these families ship with macOS.
		return "Helvetica Neue, PingFang SC, Hiragino Sans GB, Heiti SC", "Menlo, PingFang SC, Hiragino Sans GB, Heiti SC"
	case "windows":
		return "Segoe UI, Microsoft YaHei, Microsoft YaHei UI, SimSun", "Consolas, Microsoft YaHei, Microsoft YaHei UI, SimSun"
	default:
		return "Go", "Go Mono"
	}
}

func platformFontFiles() []string {
	if runtime.GOOS == "darwin" {
		return darwinFontFiles("/System/Library")
	}
	if runtime.GOOS == "windows" {
		root := os.Getenv("WINDIR")
		if root == "" {
			root = os.Getenv("SystemRoot")
		}
		var fonts []string
		for _, name := range []string{"consola.ttf", "consolab.ttf", "segoeui.ttf", "segoeuib.ttf", "msyh.ttc", "msyhbd.ttc", "simsun.ttc", "seguiemj.ttf"} {
			path := filepath.Join(root, "Fonts", name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fonts = append(fonts, path)
			}
		}
		return fonts
	}
	return nil // Keel supplies Go font fallbacks when system fonts are unavailable.
}

// PingFang's location varies across macOS releases and installed font assets.
// Load just one CJK family; the other named families remain system fallbacks.
func darwinFontFiles(library string) []string {
	var fonts []string
	add := func(path string) bool {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fonts = append(fonts, path)
			return true
		}
		return false
	}
	for _, name := range []string{"Menlo.ttc", "HelveticaNeue.ttc"} {
		add(filepath.Join(library, "Fonts", name))
	}
	for _, name := range []string{"PingFang.ttc", "Supplemental/PingFang.ttc"} {
		if add(filepath.Join(library, "Fonts", name)) {
			return fonts
		}
	}
	for _, dir := range []string{"AssetsV2", "Assets"} {
		matches, _ := filepath.Glob(filepath.Join(library, dir, "com_apple_MobileAsset_Font*", "*.asset", "AssetData", "PingFang.ttc"))
		for _, path := range matches {
			if add(path) {
				return fonts
			}
		}
	}
	for _, name := range []string{"Hiragino Sans GB.ttc", "STHeiti Medium.ttc"} {
		if add(filepath.Join(library, "Fonts", name)) {
			return fonts
		}
	}
	return fonts
}
