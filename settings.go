package main

import (
	"encoding/json"
	"github.com/dyike/keel/ui/theme"
	"image/color"
	"os"
	"path/filepath"
)

type preferences struct {
	WindowWidth   int     `json:"windowWidth,omitempty"`
	WindowHeight  int     `json:"windowHeight,omitempty"`
	Notifications bool    `json:"notifications"`
	Appearance    string  `json:"appearance"`
	FontSize      float32 `json:"fontSize"`
}
type paletteColors struct{ text, muted, panel, border, hover, active, track, top, bottom color.NRGBA }

func defaultPreferences() preferences {
	return preferences{FontSize: 12.5, Appearance: "light", WindowWidth: 1057, WindowHeight: 639}
}
func (a *app) colors() paletteColors {
	if a.prefs.Appearance == "dark" || a.prefs.Appearance == "system" && a.appearanceDark {
		return paletteColors{rgb(0xe0e4e8), rgb(0x929b9c), rgb(0x24272a), rgb(0x414247), rgb(0x373c42), rgb(0x44434b), rgb(0x36303d), rgb(0x382d43), rgb(0x332d27)}
	}
	return paletteColors{ink, gray, rgb(0xf4f4f1), rgb(0xffffff), rgb(0xe8ebed), rgb(0xfdfcfd), rgb(0xe8dce9), rgb(0xefdaf1), rgb(0xe8dab0)}
}
func (a *app) loadPreferences() {
	a.prefs = defaultPreferences()
	if a.dataDir == "" {
		return
	}
	if b, e := os.ReadFile(filepath.Join(a.dataDir, "settings.json")); e == nil {
		json.Unmarshal(b, &a.prefs)
	}
	if a.prefs.WindowWidth <= 0 || a.prefs.WindowWidth > 16384 {
		a.prefs.WindowWidth = 1057
	}
	if a.prefs.WindowHeight <= 0 || a.prefs.WindowHeight > 16384 {
		a.prefs.WindowHeight = 639
	}
	a.prefs.FontSize = max(8, min(32, a.prefs.FontSize))
	if a.prefs.Appearance != "dark" && a.prefs.Appearance != "system" {
		a.prefs.Appearance = "light"
	}
}
func (a *app) refreshAppearance() {
	dark := a.prefs.Appearance == "dark" || a.prefs.Appearance == "system" && systemDark()
	if dark != a.appearanceDark {
		a.applyAppearance()
	}
}
func (a *app) applyAppearance() {
	nativeAppearance(a.prefs.Appearance)
	a.appearanceDark = a.prefs.Appearance == "dark" || a.prefs.Appearance == "system" && systemDark()
	if a.prefs.Appearance == "dark" || a.prefs.Appearance == "system" && systemDark() {
		theme.Apply(theme.Dark())
	} else {
		theme.Apply(theme.Light())
	}
}
func (a *app) setAppearance(value string) {
	a.prefs.Appearance = value
	a.applyAppearance()
	a.savePreferences()
}
func (a *app) setFontSize(size float32) {
	a.prefs.FontSize = max(8, min(32, size))
	a.savePreferences()
}
func (a *app) savePreferences() {
	if a.dataDir != "" {
		b, _ := json.MarshalIndent(a.prefs, "", "  ")
		if e := atomicJSON(filepath.Join(a.dataDir, "settings.json"), b); e != nil {
			a.notice = e.Error()
		}
	}
}

func (a *app) rememberWindowSize(width, height int) {
	if width <= 0 || height <= 0 || width > 16384 || height > 16384 {
		return
	}
	if a.prefs.WindowWidth == width && a.prefs.WindowHeight == height {
		return
	}
	a.prefs.WindowWidth = width
	a.prefs.WindowHeight = height
}
