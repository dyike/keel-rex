package ui

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/font"
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
)

// A stable close-up for the shell labels, title weight and overlapping tiles.
func TestTitleDetailsVisualFixture(t *testing.T) {
	dir := os.Getenv("REX_DETAILS_QA_DIR")
	if dir == "" {
		t.Skip("title detail screenshots not requested")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	fonts, err := theme.PrepareFontFiles(func(font.Font) bool { return true }, "/System/Library/Fonts/HelveticaNeue.ttc", "/System/Library/Fonts/Menlo.ttc")
	if err != nil {
		t.Fatal(err)
	}
	oldFace, oldShaper := theme.Material.Face, theme.Material.Shaper
	fonts.Use()
	theme.Material.Face = "Helvetica Neue"
	defer func() { theme.Material.Face, theme.Material.Shaper = oldFace, oldShaper; theme.Apply(theme.Light()) }()
	for _, appearance := range []string{"light", "dark"} {
		a := &app{prefs: defaultPreferences(), hostModel: "Mac Studio"}
		a.prefs.Appearance = appearance
		if appearance == "dark" {
			theme.Apply(theme.Dark())
		} else {
			theme.Apply(theme.Light())
		}
		leaf := func(id int, program, path string) *pane {
			p := &pane{id: id}
			p.term = &terminal{owner: a, pane: p, session: &modelSession{state: backend.Frame{Program: program, Dir: path, Cols: 80, Rows: 4}}}
			return p
		}
		home, _ := os.UserHomeDir()
		codex := leaf(1, "codex", filepath.Join(home, "Sites/rex-snake"))
		node := leaf(2, "node", filepath.Join(home, "Sites/rex-snake"))
		git := &pane{id: 3, git: &gitState{dir: home}}
		bun := leaf(4, "bun", filepath.Join(home, "Sites/announcement"))
		fish := leaf(5, "fish", filepath.Join(home, "Sites/macos-client"))
		a.tabs = []*workspace{
			{title: "Stress-test Snake demo", customTitle: true, focus: codex, root: &pane{vertical: true, ratio: .55, first: codex, second: &pane{first: node, second: git}}},
			{focus: bun, root: &pane{first: bun, second: git}},
			{focus: fish, root: fish},
		}
		a.focused = codex
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			box := el.Div().WFull().HFull().Child(el.Widget(&chrome{a}))
			a.renderTabs(cx, box, 1057)
			box.Child(a.pane(cx, codex, 8, 46, 568, 74), a.pane(cx, node, 584, 46, 465, 74))
			return box
		}))
		if err := window.ScreenshotAtScale(root, 1057, 120, 2, filepath.Join(dir, "title-details-"+appearance+".png")); err != nil {
			t.Fatal(err)
		}
	}
}
