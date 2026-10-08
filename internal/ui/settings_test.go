package ui

import "testing"

func TestLatestWindowSizeRestores(t *testing.T) {
	dir := t.TempDir()
	a := &app{dataDir: dir}
	a.loadPreferences()
	if a.prefs.WindowWidth != 1057 || a.prefs.WindowHeight != 639 {
		t.Fatal("initial window defaults", a.prefs)
	}
	a.rememberWindowSize(900, 550)
	a.savePreferences()
	restored := &app{dataDir: dir}
	restored.loadPreferences()
	if restored.prefs.WindowWidth != 900 || restored.prefs.WindowHeight != 550 {
		t.Fatal("latest size not restored", restored.prefs)
	}
	restored.rememberWindowSize(0, 0)
	if restored.prefs.WindowWidth != 900 || restored.prefs.WindowHeight != 550 {
		t.Fatal("invalid size replaced latest size")
	}
}
