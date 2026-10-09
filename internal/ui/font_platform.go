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
		return "Helvetica Neue", "Menlo"
	case "windows":
		return "Segoe UI, Microsoft YaHei, Microsoft YaHei UI, SimSun", "Consolas, Microsoft YaHei, Microsoft YaHei UI, SimSun"
	default:
		return "Go", "Go Mono"
	}
}

func platformFontFiles() []string {
	if runtime.GOOS == "darwin" {
		fonts := []string{"/System/Library/Fonts/Menlo.ttc", "/System/Library/Fonts/HelveticaNeue.ttc"}
		cjk, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*/*.asset/AssetData/PingFang.ttc")
		if len(cjk) > 0 {
			fonts = append(fonts, cjk[0])
		}
		return fonts
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
