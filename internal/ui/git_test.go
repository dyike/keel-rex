package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

func TestGitCommitInputKeepsFocusDuringRefresh(t *testing.T) {
	g := &gitState{root: t.TempDir(), branch: "main", editing: true, commitFocus: true,
		files: []gitFile{{Status: "A ", Path: "new.txt"}}}
	id := fmt.Sprintf("git-commit-%p", g)
	var cx *el.Context
	focusOther := false
	root := el.Root(el.ViewFunc(func(context *el.Context) el.Element {
		cx = context
		if focusOther {
			cx.Focus("other")
			focusOther = false
		}
		return el.Div().Child(g.Render(cx, 740, 360), el.Input().ID("other"))
	}))
	var router input.Router
	var ops op.Ops
	now := time.Now()
	render := func() {
		for range 3 {
			ops.Reset()
			root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(740, 420))})
			router.Frame(&ops)
		}
	}
	render()
	if !cx.Focused(id) {
		t.Fatal("opening commit editor did not focus the input")
	}
	for _, text := range []string{"fix", " 中文", " refresh"} {
		g.busy = true // The same state entered by the periodic snapshot refresh.
		render()
		if !cx.Focused(id) {
			t.Fatal("Git refresh blurred the commit input")
		}
		pos := len([]rune(g.commit))
		router.Queue(key.EditEvent{Range: key.Range{Start: pos, End: pos}, Text: text})
		render()
		g.busy = false
		render()
	}
	if g.commit != "fix 中文 refresh" {
		t.Fatalf("lost input during refresh: %q", g.commit)
	}
	g.busy, g.committing = true, true
	render()
	if !cx.Focused(id) {
		t.Fatal("submitting commit blurred the read-only input")
	}
	router.Queue(key.EditEvent{Text: "unexpected"})
	render()
	if g.commit != "fix 中文 refresh" {
		t.Fatalf("accepted unsent edits during commit: %q", g.commit)
	}
	g.busy, g.committing = false, false
	g.message = "hook-blocked"
	render()
	if !cx.Focused(id) {
		t.Fatal("failed commit did not retain input focus")
	}
	pos := len([]rune(g.commit))
	router.Queue(key.EditEvent{Range: key.Range{Start: pos, End: pos}, Text: " retry"})
	render()
	if g.commit != "fix 中文 refresh retry" {
		t.Fatalf("could not resume editing after failed commit: %q", g.commit)
	}
	focusOther = true
	render()
	g.busy = true
	render()
	if !cx.Focused("other") {
		t.Fatal("refresh stole focus back from another input")
	}
}

func TestGitDisplayLine(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{" \tgithub.com/godbus/dbus/v5 v5.2.2", "         github.com/godbus/dbus/v5 v5.2.2"},
		{"+\t\treturn value", "+                return value"},
		{"-abc\tdef", "-abc     def"},
		{"+中文\ttext", "+中文    text"},
		{"+e\u0301\ttext", "+e\u0301       text"},
		{"+👩‍💻\ttext", "+👩‍💻      text"},
		{"+foo\r", "+foo"},
		{"+\x1b[31m", "+\\x1b[31m"},
	} {
		if got := gitDisplayLine(tc.input, true); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.input, got, tc.want)
		}
	}
	if got := gitDisplayLine(" \tvalue", false); got != "        value" {
		t.Errorf("plain preview tab: %q", got)
	}
}

// Render the same long paths and Go module indentation reported by the user.
// Set REX_GIT_QA_DIR to retain native screenshots for visual inspection.
func TestGitPanelVisualFixture(t *testing.T) {
	dir := os.Getenv("REX_GIT_QA_DIR")
	if dir == "" {
		t.Skip("visual QA artifact requested with REX_GIT_QA_DIR")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := &gitState{branch: "main", selected: "go.mod", files: []gitFile{
		{Status: " M", Path: "go.mod"}, {Status: " M", Path: "go.sum"}, {Status: " M", Path: "ui/window/README.zh-CN.md"},
		{Status: " M", Path: "ui/window/titlebar_darwin.go"}, {Status: "??", Path: "examples/rex/assets/very-long-filename.png"},
	}, diff: "diff --git a/go.mod b/go.mod\n@@ -25,11 +30,22 @@ require (\n \trequire (\n \tgithub.com/godbus/dbus/v5 v5.2.2\n+\tgithub.com/charmbracelet/colorprofile v0.4.2 // indirect\n-\tgithub.com/charmbracelet/colorprofile v0.4.1 // indirect\n \t" + strings.Repeat("long_source_path/", 10) + "module\n\n )"}
	for _, width := range []int{740, 360} {
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().P(12).Bg(rgb(0xf4f4f1)).Child(g.Render(cx, float32(width-24), 360))
		}))
		name := "git-wide.png"
		if width == 360 {
			name = "git-narrow.png"
		}
		if err := window.Screenshot(root, width, 384, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}
