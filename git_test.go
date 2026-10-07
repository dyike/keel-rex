package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
)

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
		{" M", "go.mod"}, {" M", "go.sum"}, {" M", "ui/window/README.zh-CN.md"},
		{" M", "ui/window/titlebar_darwin.go"}, {"??", "examples/rex/assets/very-long-filename.png"},
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
