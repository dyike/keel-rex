package ui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"strings"
	"time"
)

type gitFile = backend.GitFile

type gitState struct {
	repo                                                                      *backend.Repository
	dir, root, branch, diff, selected, message, commit                        string
	files                                                                     []gitFile
	busy, editing, selectedStaged, refreshPending, refreshError, messageError bool
	committing, commitFocus                                                   bool
	timer                                                                     int
}

func (g *gitState) repository() *backend.Repository {
	if g.repo == nil {
		g.repo = backend.NewRepository(g.dir)
	}
	return g.repo
}
func (g *gitState) refresh() {
	if g.busy {
		g.refreshPending = true
		return
	}
	g.busy = true
	repo, selected, staged := g.repository(), g.selected, g.selectedStaged
	go func() {
		snapshot, err := repo.Snapshot(selected, staged)
		core.Update(func() {
			g.busy = false
			pending := g.refreshPending || g.selected != selected || g.selectedStaged != staged
			g.refreshPending = false
			if err != nil {
				g.message, g.messageError, g.refreshError = err.Error(), true, true
				g.root, g.diff, g.branch = "", "", ""
				g.files = nil
			} else {
				g.root, g.branch, g.files = snapshot.Root, snapshot.Branch, snapshot.Files
				g.repo = backend.NewRepository(snapshot.Root)
				if g.selected == selected && g.selectedStaged == staged {
					g.selected, g.selectedStaged, g.diff = snapshot.Selected, snapshot.SelectedStaged, snapshot.Diff
				}
				if g.refreshError {
					g.message, g.messageError, g.refreshError = "", false, false
				}
			}
			if pending {
				g.refresh()
			}
		})
	}()
}
func (g *gitState) runAction(commit bool, operation func() (string, error)) {
	if g.root == "" {
		g.message, g.messageError = "No Git repository selected", true
		return
	}
	if g.busy {
		return
	}
	g.busy = true
	g.committing = commit
	go func() {
		out, err := operation()
		core.Update(func() {
			g.busy = false
			g.committing = false
			g.messageError, g.refreshError = err != nil, false
			if err != nil {
				g.message = strings.TrimSpace(out + "\n" + err.Error())
			} else {
				g.message = strings.TrimSpace(out)
				if commit {
					g.editing, g.commit = false, ""
				}
			}
			g.refreshPending = false
			g.refresh()
		})
	}()
}
func (g *gitState) action(args ...string) {
	repo := g.repository()
	g.runAction(false, func() (string, error) { return repo.Run(args...) })
}
func (g *gitState) selectedFile() (gitFile, bool) {
	for _, f := range g.files {
		if f.Path == g.selected && (g.selectedStaged && f.Staged() || !g.selectedStaged && f.Unstaged()) {
			return f, true
		}
	}
	return gitFile{}, false
}
func (g *gitState) counts() (staged, changed, conflicts int) {
	for _, f := range g.files {
		if f.Staged() {
			staged++
		}
		if f.Unstaged() {
			changed++
		}
		if f.Conflicted() {
			conflicts++
		}
	}
	return
}
func (g *gitState) stage() {
	f, ok := g.selectedFile()
	if !ok {
		return
	}
	repo := g.repository()
	if g.selectedStaged {
		g.runAction(false, func() (string, error) { return repo.Unstage(f) })
	} else {
		g.runAction(false, func() (string, error) { return repo.Stage(f) })
	}
}
func (g *gitState) Render(cx *el.Context, w, h float32) el.Element {
	cx.After(g.timer, 2*time.Second, func() { g.timer++; g.refresh() })
	stagedCount, changedCount, conflictCount := g.counts()
	toolbarHeight := float32(24)
	if w < 360 {
		toolbarHeight = 54
	}
	bodyHeight := h - 38 - toolbarHeight
	if g.editing {
		bodyHeight -= 38
	}
	if g.message != "" {
		bodyHeight -= 36
	}
	bodyHeight = max(float32(20), bodyHeight)
	left := min(float32(260), w*.38)
	files := el.Div().W(el.Dp(left)).NoShrink().H(el.Dp(bodyHeight)).ScrollY().Gap(1)
	section := func(title string, staged, conflict bool) {
		var entries []gitFile
		for _, f := range g.files {
			if staged && f.Staged() || !staged && f.Unstaged() && f.Conflicted() == conflict {
				entries = append(entries, f)
			}
		}
		if len(entries) == 0 {
			return
		}
		files.Child(el.Text(fmt.Sprintf("%s · %d", title, len(entries))).Bold().TextSize(10).TextColor(theme.Muted).MaxLines(1).Mt(4).Mb(3))
		for _, f := range entries {
			f := f
			color := red
			if staged {
				color = green
			}
			if conflict {
				color = theme.Warning
			}
			status := string(f.Status[1])
			if staged {
				status = string(f.Status[0])
			}
			if conflict {
				status = "!"
			}
			name := "Git file " + f.Path
			if staged {
				name = "Git staged file " + f.Path
			}
			label := f.Path
			if f.OriginalPath != "" {
				label = f.OriginalPath + " → " + f.Path
			}
			row := el.Div().Row().WFull().H(el.Dp(23)).NoShrink().Items(el.Center).Gap(6).Px(6).Role("button").Name(name).OnClick(func() {
				g.selected, g.selectedStaged, g.diff = f.Path, staged, ""
				g.refresh()
			}).Child(el.Text(status).Mono().TextSize(11).Bold().TextColor(color).W(el.Dp(12)).NoShrink().MaxLines(1), el.Text(label).Mono().TextSize(11).MaxLines(1).Grow().W(el.Dp(0)))
			if f.Path == g.selected && staged == g.selectedStaged {
				row.Bg(theme.SubtleHover).Rounded(3)
			}
			files.Child(row)
		}
	}
	section("Conflicts", false, true)
	section("Changes", false, false)
	section("Staged", true, false)
	var display []string
	columns := 0
	patch := strings.HasPrefix(g.diff, "diff --")
	for _, raw := range strings.Split(g.diff, "\n") {
		line := gitDisplayLine(raw, patch)
		display = append(display, line)
		columns = max(columns, ansi.StringWidth(line))
	}
	content := el.Div().MinW(el.Dp(float32(columns)*7 + 12)).Gap(0)
	lines := el.Div().ID(fmt.Sprintf("git-diff-%p-%s-%t", g, g.selected, g.selectedStaged)).WFull().H(el.Dp(max(0, bodyHeight-23))).ScrollX().ScrollY().Child(content)
	for _, l := range display {
		c := theme.Text
		if patch {
			if strings.HasPrefix(l, "+") {
				c = green
			}
			if strings.HasPrefix(l, "-") {
				c = red
			}
			if strings.HasPrefix(l, "@@") {
				c = cyan
			}
		}
		content.Child(el.Text(l).Mono().TextSize(11).TextColor(c).H(el.Dp(16)).NoShrink().MaxLines(1))
	}
	selected, hasSelected := g.selectedFile()
	diffTitle := "Working tree diff"
	if g.selectedStaged {
		diffTitle = "Staged diff"
	}
	if selected.Conflicted() {
		diffTitle = "Conflict diff"
	}
	if g.selected == "" {
		diffTitle = "No changes selected"
	}
	preview := el.Div().Grow().W(el.Dp(0)).H(el.Dp(bodyHeight)).Gap(5).Child(el.Text(diffTitle).Bold().TextSize(10.5).TextColor(theme.Muted).MaxLines(1), lines)
	button := func(name string, enabled bool, fn func()) el.Element {
		return el.Div().Row().Items(el.Center).Gap(5).Role("button").Name(name).Disabled(g.busy || !enabled).OnClick(fn).NoShrink().Px(5).Py(4).Bg(theme.SubtleHover).Rounded(4).Hover(func(s *el.Style) { s.Bg(theme.Surface) }).Child(iconElement(actionGlyph(name), 13), el.Text(name).TextSize(10).TextColor(theme.Text).MaxLines(1))
	}
	stageLabel := "Stage"
	if g.selectedStaged {
		stageLabel = "Unstage"
	} else if selected.Conflicted() {
		stageLabel = "Stage resolution"
	}
	branch := g.branch
	if branch == "" {
		branch = "Reading…"
	}
	toolbar := el.Div().Row().Wrap().NoShrink().Items(el.Center).Gap(6).Child(
		button(stageLabel, hasSelected, g.stage),
		button("Stage all", changedCount > 0 && conflictCount == 0, func() { g.action("add", "--all") }),
		button("Unstage all", stagedCount > 0, func() { repo := g.repository(); g.runAction(false, repo.UnstageAll) }),
		button("Commit…", stagedCount > 0 && conflictCount == 0, func() {
			g.editing = !g.editing
			g.commitFocus = g.editing
		}),
	)
	root := el.Div().W(el.Dp(w)).H(el.Dp(h)).Gap(8).Child(
		el.Div().Row().H(el.Dp(22)).NoShrink().Items(el.Center).Gap(8).Child(el.Text("Changes · "+branch).Bold().TextSize(12).MaxLines(1).Grow().W(el.Dp(0)), button("Refresh", true, g.refresh)),
		el.Div().Row().H(el.Dp(bodyHeight)).NoShrink().Gap(8).Child(files, preview), toolbar,
	)
	if g.editing {
		inputID := fmt.Sprintf("git-commit-%p", g)
		if g.commitFocus {
			cx.Focus(inputID)
			g.commitFocus = false
		}
		// Snapshot refreshes must leave the editor enabled and focused. During
		// submission, read-only retains focus without accepting unsent edits.
		root.Child(el.Div().Row().H(el.Dp(30)).NoShrink().Items(el.Center).Gap(6).Child(el.Input().ID(inputID).Bind(&g.commit).Placeholder("Commit message").Name("Commit message").ReadOnly(g.committing).OnSubmit(func(string) { g.commitChanges() }).Grow().W(el.Dp(0)), button("Create commit", strings.TrimSpace(g.commit) != "" && stagedCount > 0 && conflictCount == 0, g.commitChanges)))
	}
	if g.message != "" {
		c := gray
		if g.messageError {
			c = red
		}
		root.Child(el.Text(g.message).TextSize(10).TextColor(c).MaxLines(2))
	}
	return root
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
	staged, _, conflicts := g.counts()
	if strings.TrimSpace(g.commit) == "" {
		g.message, g.messageError = "Enter a commit message", true
		return
	}
	if conflicts > 0 {
		g.message, g.messageError = "Resolve and stage conflicted files before committing", true
		return
	}
	if staged == 0 {
		g.message, g.messageError = "Stage changes before committing", true
		return
	}
	repo, message := g.repository(), g.commit
	g.runAction(true, func() (string, error) { return repo.Commit(message) })
}
