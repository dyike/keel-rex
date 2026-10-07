package main

import (
	"bytes"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gitFile struct{ status, path string }
type gitState struct {
	dir, root, branch, diff, selected, message, commit string
	files                                              []gitFile
	busy                                               bool
	editing                                            bool
	timer                                              int
}

func gitCommand(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	return string(b), e
}
func (g *gitState) refresh() {
	if g.busy {
		return
	}
	g.busy = true
	dir := g.dir
	selected := g.selected
	go func() {
		root, e := gitCommand(dir, "rev-parse", "--show-toplevel")
		if e != nil {
			core.Update(func() { g.message = "This directory is not a Git repository."; g.files = nil; g.busy = false })
			return
		}
		root = strings.TrimSpace(root)
		branch, _ := gitCommand(root, "branch", "--show-current")
		status, _ := gitCommand(root, "status", "--porcelain=v1", "--untracked-files=all", "-z")
		items := strings.Split(status, "\x00")
		var files []gitFile
		for i := 0; i < len(items); i++ {
			v := items[i]
			if len(v) < 4 {
				continue
			}
			files = append(files, gitFile{v[:2], v[3:]})
			if v[0] == 'R' || v[0] == 'C' {
				i++
			}
		}
		found := false
		for _, f := range files {
			if f.path == selected {
				found = true
			}
		}
		if !found {
			selected = ""
			if len(files) > 0 {
				selected = files[0].path
			}
		}
		diff := "Working tree clean"
		if selected != "" {
			var err error
			diff, err = gitCommand(root, "diff", "--no-ext-diff", "--no-color", "HEAD", "--", selected)
			if err != nil {
				diff = ""
			}
			if diff == "" {
				diff, _ = gitCommand(root, "diff", "--no-ext-diff", "--no-color", "--", selected)
			}
			if diff == "" {
				diff, _ = gitCommand(root, "diff", "--cached", "--no-ext-diff", "--no-color", "--", selected)
			}
			if diff == "" {
				if b, e := readPreview(filepath.Join(root, selected)); e == nil {
					if len(b) > 256<<10 {
						b = b[:256<<10]
					}
					if bytes.IndexByte(b, 0) >= 0 {
						diff = "Binary file: " + selected
					} else {
						diff = "Untracked: " + selected + "\n" + string(b)
					}
				}
			}
		}
		core.Update(func() {
			g.root = root
			g.branch = strings.TrimSpace(branch)
			g.files = files
			g.selected = selected
			g.diff = diff
			g.busy = false
			if g.message == "This directory is not a Git repository." {
				g.message = ""
			}
		})
	}()
}
func (g *gitState) action(args ...string) {
	if g.root == "" {
		g.message = "No Git repository selected"
		return
	}
	if g.busy {
		return
	}
	g.busy = true
	dir := g.root
	go func() {
		out, e := gitCommand(dir, args...)
		core.Update(func() {
			g.busy = false
			if e != nil {
				g.message = fmt.Sprintf("%s\n%v", out, e)
			} else {
				g.message = strings.TrimSpace(out)
				if len(args) > 0 && args[0] == "commit" {
					g.editing = false
					g.commit = ""
				}
			}
			g.refresh()
		})
	}()
}
func (g *gitState) stage() {
	for _, f := range g.files {
		if f.path == g.selected {
			if f.status[0] != ' ' && f.status[0] != '?' {
				if _, e := gitCommand(g.root, "rev-parse", "--verify", "HEAD"); e == nil {
					g.action("reset", "--", f.path)
				} else {
					g.action("rm", "--cached", "--", f.path)
				}
			} else {
				g.action("add", "--", f.path)
			}
			return
		}
	}
}
func (g *gitState) Render(cx *el.Context, w, h float32) el.Element {
	cx.After(g.timer, 2*time.Second, func() { g.timer++; g.refresh() })
	left := min(float32(260), w*.34)
	bodyHeight := h - 62
	if g.editing {
		bodyHeight -= 38
	}
	if g.message != "" {
		bodyHeight -= 36
	}
	bodyHeight = max(float32(20), bodyHeight)
	files := el.Div().W(el.Dp(left)).NoShrink().H(el.Dp(bodyHeight)).ScrollY().Gap(1)
	for _, f := range g.files {
		f := f
		color := gray
		if strings.Contains(f.status, "M") || f.status == "??" {
			color = red
		}
		row := el.Div().Row().WFull().H(el.Dp(23)).NoShrink().Items(el.Center).Gap(6).Px(6).Role("button").Name("Git file "+f.path).OnClick(func() { g.selected = f.path; g.refresh() }).Child(
			el.Text(strings.TrimSpace(f.status)).Mono().TextSize(11).TextColor(color).W(el.Dp(18)).NoShrink().MaxLines(1),
			el.Text(f.path).Mono().TextSize(11).MaxLines(1).Grow().W(el.Dp(0)),
		)
		if f.path == g.selected {
			row.Bg(theme.SubtleHover).Rounded(3)
		}
		files.Child(row)
	}
	// Preserve code lines and indentation; reveal long lines by scrolling,
	// rather than wrapping/truncating the source or shaping tab control glyphs.
	var display []string
	columns := 0
	for _, raw := range strings.Split(g.diff, "\n") {
		line := gitDisplayLine(raw, strings.HasPrefix(g.diff, "diff --git "))
		display = append(display, line)
		columns = max(columns, ansi.StringWidth(line))
	}
	content := el.Div().MinW(el.Dp(float32(columns)*7 + 12)).Gap(0)
	lines := el.Div().ID(fmt.Sprintf("git-diff-%p-%s", g, g.selected)).Grow().W(el.Dp(0)).H(el.Dp(bodyHeight)).ScrollX().ScrollY().Child(content)
	for _, l := range display {
		c := theme.Text
		if strings.HasPrefix(l, "+") {
			c = green
		}
		if strings.HasPrefix(l, "-") {
			c = red
		}
		if strings.HasPrefix(l, "@@") {
			c = cyan
		}
		content.Child(el.Text(l).Mono().TextSize(11).TextColor(c).H(el.Dp(16)).NoShrink().MaxLines(1))
	}
	button := func(name string, fn func()) el.Element {
		return el.Div().Row().Items(el.Center).Gap(5).Role("button").Name(name).Disabled(g.busy).OnClick(fn).NoShrink().Px(7).Py(4).Bg(theme.SubtleHover).Rounded(4).Hover(func(s *el.Style) { s.Bg(theme.Surface) }).Child(iconElement(actionGlyph(name), 13), el.Text(name).TextSize(10).TextColor(theme.Text).MaxLines(1))
	}
	stageLabel := "Stage"
	for _, file := range g.files {
		if file.path == g.selected && file.status[0] != ' ' && file.status[0] != '?' {
			stageLabel = "Unstage"
		}
	}
	root := el.Div().W(el.Dp(w)).H(el.Dp(h)).Gap(8).Child(el.Div().Row().H(el.Dp(22)).NoShrink().Items(el.Center).Gap(8).Child(el.Text("Changes · "+g.branch).Bold().TextSize(12).MaxLines(1).Grow().W(el.Dp(0)), button("Refresh", g.refresh)), el.Div().Row().H(el.Dp(bodyHeight)).NoShrink().Gap(8).Child(files, lines), el.Div().Row().H(el.Dp(24)).NoShrink().Items(el.Center).Gap(6).Child(button(stageLabel, g.stage), button("Stage all", func() { g.action("add", "--all") }), button("Commit…", func() { g.editing = !g.editing })))
	if g.editing {
		root.Child(el.Div().Row().H(el.Dp(30)).NoShrink().Items(el.Center).Gap(6).Child(el.Input().Bind(&g.commit).Placeholder("Commit message").Name("Commit message").OnSubmit(func(string) { g.commitChanges() }).Grow().W(el.Dp(0)), button("Create commit", g.commitChanges)))
	}
	if g.message != "" {
		root.Child(el.Text(g.message).TextSize(10).TextColor(gray).MaxLines(2))
	}
	return root
}

func readPreview(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 256<<10))
}

// gitDisplayLine expands tabs on 8-column code tab stops. Unified diff's
// leading marker is outside the source column count. Other control characters
// are escaped visibly, so the shaper never receives unsupported glyphs.
func gitDisplayLine(line string, patch bool) string {
	line = strings.TrimSuffix(line, "\r")
	var out strings.Builder
	column := 0
	var segment strings.Builder
	flush := func() {
		value := segment.String()
		out.WriteString(value)
		column += ansi.StringWidth(value)
		segment.Reset()
	}
	if patch && len(line) > 0 && (line[0] == ' ' || line[0] == '+' || line[0] == '-') {
		out.WriteByte(line[0])
		line = line[1:]
	}
	for _, r := range line {
		switch {
		case r == '\t':
			flush()
			count := 8 - column%8
			out.WriteString(strings.Repeat(" ", count))
			column += count
		case r < 32 || r == 127:
			flush()
			escaped := fmt.Sprintf("\\x%02x", r)
			out.WriteString(escaped)
			column += len(escaped)
		default:
			segment.WriteRune(r)
		}
	}
	flush()
	return out.String()
}

func (g *gitState) commitChanges() {
	if g.busy {
		return
	}
	if strings.TrimSpace(g.commit) == "" {
		g.message = "Enter a commit message"
		return
	}
	g.action("commit", "-m", strings.TrimSpace(g.commit))
}
