package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"math"
	"path/filepath"
	"strings"
	"time"
)

type paneRect struct{ X, Y, W, H float32 }

func (a *app) focusPane(p *pane) {
	if p == nil || len(a.tabs) == 0 {
		return
	}
	a.tabs[a.active].root.each(func(n *pane) {
		if n.term != nil {
			n.term.initialFocus = false
		}
	})
	a.focused = p
	a.tabs[a.active].focus = p
	p.attention = false
	if p.term != nil {
		p.term.initialFocus = true
		p.seenBells = p.term.session.Snapshot(0, 0).Bells
	}
}
func (a *app) cycleTab(delta int) { a.activate((a.active + delta + len(a.tabs)) % len(a.tabs)) }
func (a *app) moveTab(tab *workspace, to int) {
	from := -1
	for i, t := range a.tabs {
		if t == tab {
			from = i
		}
	}
	if from < 0 {
		return
	}
	current := a.tabs[a.active]
	to = max(0, min(to, len(a.tabs)-1))
	a.tabs = append(a.tabs[:from], a.tabs[from+1:]...)
	a.tabs = append(a.tabs, nil)
	copy(a.tabs[to+1:], a.tabs[to:])
	a.tabs[to] = tab
	for i, t := range a.tabs {
		if t == current {
			a.active = i
		}
	}
}
func (a *app) closeSpecificTab(tab *workspace) {
	for i, t := range a.tabs {
		if t == tab {
			a.activate(i)
			a.requestCloseTab()
			return
		}
	}
}
func (a *app) moveFocus(dx, dy float32) {
	if a.focused == nil {
		return
	}
	r := a.focused.bounds
	x, y := r.X+r.W/2, r.Y+r.H/2
	best := float64(math.MaxFloat64)
	var target *pane
	a.tabs[a.active].root.each(func(p *pane) {
		if p == a.focused {
			return
		}
		q := p.bounds
		xx, yy := q.X+q.W/2-x, q.Y+q.H/2-y
		forward := xx*dx + yy*dy
		if forward <= 1 {
			return
		}
		side := float64(math.Abs(float64(xx*dy - yy*dx)))
		score := float64(forward) + side*3
		if score < best {
			best = score
			target = p
		}
	})
	if target != nil {
		a.focusPane(target)
	}
}
func (a *app) resizeDivider(dx, dy float32) {
	var nearest *pane
	var find func(*pane)
	find = func(n *pane) {
		if n == nil || n.first == nil {
			return
		}
		if containsPane(n, a.focused) && ((dx != 0 && n.vertical) || (dy != 0 && !n.vertical)) {
			nearest = n
		}
		find(n.first)
		find(n.second)
	}
	find(a.tabs[a.active].root)
	if nearest != nil {
		nearest.ratio = max(.15, min(.85, nearest.ratio+(dx+dy)*.04))
	}
}
func containsPane(root, target *pane) bool {
	if root == nil {
		return false
	}
	return root == target || containsPane(root.first, target) || containsPane(root.second, target)
}
func (a *app) equalize() {
	var walk func(*pane)
	walk = func(p *pane) {
		if p == nil {
			return
		}
		p.ratio = .5
		walk(p.first)
		walk(p.second)
	}
	walk(a.tabs[a.active].root)
}

func (a *app) restartFocused() {
	if a.focused == nil || a.focused.term == nil || a.focused.restarting {
		return
	}
	p := a.focused
	dir := p.term.session.Directory()
	p.restarting = true
	rebuild := func(err error) {
		p.restarting = false
		if a.closed {
			return
		}
		exists := false
		for _, tab := range a.tabs {
			exists = exists || containsPane(tab.root, p)
		}
		if !exists || p.git != nil {
			return
		}
		if err != nil {
			a.notice = "Could not restart session: " + err.Error()
			return
		}
		p.term.session.Close()
		fresh := newPane(a, dir)
		p.term, p.err = fresh.term, fresh.err
		if p.term != nil {
			p.term.pane = p
		}
		if a.focused == p {
			a.focusPane(p)
		}
	}
	if a.dataDir == "" {
		rebuild(nil)
		return
	}
	service := a.backendService()
	go func() { err := service.Ensure(); core.Update(func() { rebuild(err) }) }()
}

func programTitle(s backend.Session) string {
	state := s.State()
	program, title, exited := strings.TrimPrefix(filepath.Base(state.Program), "-"), state.Title, state.Exited
	if exited {
		return "Session ended"
	}
	if state.Program == "" {
		program = "Shell"
		if title != "" {
			program = strings.TrimPrefix(filepath.Base(title), "-")
		}
		if strings.Contains(program, ":") {
			program = "Shell"
		}
	}
	if backend.IsShellProgram(program) {
		return program
	}
	names := map[string]string{"codex": "Codex", "claude": "Claude Code", "node": "Node", "python": "Python", "python3": "Python", "bun": "Bun", "lazygit": "Git Changes", "vim": "Vim", "nvim": "Neovim", "ssh": "SSH", "htop": "htop"}
	if program == "ssh" && strings.HasPrefix(title, "ssh ") {
		return "SSH " + strings.TrimPrefix(title, "ssh ")
	}
	if name := names[program]; name != "" {
		return name
	}
	return program
}
func programIcon(s backend.Session) string {
	title := programTitle(s)
	if title == "Git Changes" {
		return "git"
	}
	if strings.HasPrefix(title, "SSH ") {
		return "remote"
	}
	switch title {
	case "Codex":
		return "codex"
	case "Claude Code":
		return "claude"
	case "Vim", "Neovim":
		return "editor"
	case "SSH":
		return "remote"
	case "Node":
		return "node"
	case "Python":
		return "python"
	case "Bun":
		return "bun"
	}
	return "fish"
}

// checkActivity marks background panes that need attention, and reports
// whether a pane's dots changed: attention, or output that stopped.
func (a *app) checkActivity() bool {
	changed := false
	for _, tab := range a.tabs {
		tab.root.each(func(p *pane) {
			if p.term == nil {
				return
			}
			f := p.term.session.Snapshot(0, 0)
			visible := p == a.focused && (a.window == nil || a.window.Focused())
			attention := p.attention
			if !visible && (f.Bells > p.seenBells || p.lastProgram != "" && !backend.IsShellProgram(p.lastProgram) && p.lastProgram != f.Program && backend.IsShellProgram(f.Program)) {
				if !p.attention {
					a.notifyAttention(tab, p, "A background session needs attention")
				}
				p.attention = true
			}
			if visible {
				p.seenBells = f.Bells
				p.attention = false
			}
			changed = changed || p.lastProgram != f.Program
			p.lastProgram = f.Program
			if busy := a.busy(p); busy != p.wasBusy || attention != p.attention {
				p.wasBusy = busy
				changed = true
			}
		})
	}
	return changed
}
func (a *app) tabLabel(t *workspace) string {
	if t.customTitle {
		return t.title
	}
	p := t.focus
	if p == nil {
		return t.title
	}
	if p.term != nil {
		return programTitle(p.term.session) + " " + shortPath(p.term.session.Directory())
	}
	return t.title
}
func paneRunning(p *pane) bool {
	if p == nil || p.term == nil {
		return false
	}
	state := p.term.session.State()
	return !state.Exited && state.Program != "" && !backend.IsShellProgram(state.Program)
}
func (a *app) requestClosePane(p *pane) {
	if paneRunning(p) {
		a.confirmTitle = "End the running program and close this pane?"
		a.confirmAction = func() { a.closePane(p) }
	} else {
		a.closePane(p)
	}
}
func (a *app) requestCloseTab() {
	tab := a.tabs[a.active]
	running := false
	tab.root.each(func(p *pane) { running = running || paneRunning(p) })
	if running {
		a.confirmTitle = "End programs running in this tab?"
		a.confirmAction = func() {
			for i, t := range a.tabs {
				if t == tab {
					a.activate(i)
					a.closeTab()
					return
				}
			}
		}
	} else {
		a.closeTab()
	}
}
func (a *app) busy(p *pane) bool {
	if p == nil || p.term == nil {
		return false
	}
	return time.Since(p.term.session.State().LastOutput) < 600*time.Millisecond
}
