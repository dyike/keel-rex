package ui

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceRestoresOrderNamesSplitsAndFocus(t *testing.T) {
	dir := testServer(t)
	cwd := t.TempDir()
	a := newAppAt(cwd, dir)
	a.newTabAt(cwd)
	a.tabs[a.active].customTitle = true
	a.tabs[a.active].title = "Renamed work"
	a.split(true)
	a.tabs[a.active].root.ratio = .63
	focus := a.focused.id
	sid := a.focused.term.session.ID()
	a.moveTab(a.tabs[a.active], 0)
	a.persist(true)
	a.close()
	b := newAppAt(cwd, dir)
	defer b.close()
	if b.active != 0 || b.tabs[0].title != "Renamed work" || !b.tabs[0].customTitle || b.tabs[0].root.ratio != .63 || b.focused.id != focus {
		state, _ := json.Marshal(b.workspaceState())
		t.Fatalf("workspace changed: %s", state)
	}
	if b.focused.term.session.ID() != sid {
		t.Fatal("restored layout did not reattach its session")
	}
}
func TestFuzzyCommandsAndTabNavigation(t *testing.T) {
	a := newApp(t.TempDir())
	defer a.close()
	first := a.tabs[0]
	first.focus = a.focused
	a.newTab()
	second := a.tabs[1]
	secondFocus := a.focused
	a.activate(0)
	if a.focused != first.focus {
		t.Fatal("tab lost its remembered focus")
	}
	a.activate(1)
	if a.focused != secondFocus {
		t.Fatal("second tab lost focus")
	}
	a.moveTab(second, 0)
	if a.tabs[0] != second || a.active != 0 {
		t.Fatal("reorder changed active workspace")
	}
	a.query = "spl r"
	rows := a.paletteResults()
	if len(rows) == 0 || rows[0].ID != "split-right" {
		t.Fatalf("unexpected fuzzy result: %+v", rows)
	}
	a.query = "nonexistent command"
	if len(a.paletteResults()) != 0 {
		t.Fatal("nonmatching commands shown")
	}
}
