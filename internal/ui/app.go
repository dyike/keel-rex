// Rex: native Keel terminal workspace, backed by real PTY sessions.
package ui

import (
	"fmt"
	"gioui.org/op/clip"
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var nextID int

type pane struct {
	bounds      paneRect
	attention   bool
	restarting  bool
	seenBells   int
	lastProgram string
	wasBusy     bool // busy when last drawn

	id                int
	first, second     *pane
	vertical          bool
	ratio             float32
	dividerDragOffset float32
	resizing          bool
	term              *terminal
	git               *gitState
	err               string
}
type workspace struct {
	focus       *pane
	customTitle bool

	root  *pane
	zoom  bool
	title string
}
type app struct {
	service                             backend.WorkspaceService
	window                              *window.Window
	closed                              bool
	dataDir                             string
	prefs                               preferences
	lastLayout                          string
	persistQueue                        chan persistJob
	query                               string
	paletteIndex                        int
	lastQuery                           string
	directoryOpen, renameOpen, hostOpen bool
	editFocus                           bool
	renameText                          string
	menuTab                             *workspace
	contextPane                         *pane
	confirmTitle                        string
	confirmAction                       func()
	draggingTab                         *workspace
	dragOrigin                          float32
	searchOpen                          bool
	searchQuery                         string
	paletteReveal                       int
	hostDetails                         backend.HostDetails
	hostStatus                          backend.HostStatus
	hostStatusLoading                   bool
	hostPoll                            int
	started                             time.Time
	hostModel                           string
	appearanceDark                      bool

	paletteFocus bool
	tabs         []*workspace
	active       int
	focused      *pane
	cwd          string
	palette      bool
	path         string
	notice       string
}

func newPane(a *app, dir string) *pane {
	nextID++
	p := &pane{id: nextID, ratio: .5}
	s, e := a.backendService().Open("", dir)
	if e != nil {
		p.err = e.Error()
	} else {
		p.term = a.terminalView(s, p, true)
	}
	return p
}
func newApp(dir string) *app { return newAppAt(dir, "") }
func newAppAt(dir, dataDir string) *app {
	a := newAppShell(dir, dataDir)
	a.openWorkspace()
	return a
}

// newAppShell makes the app without its workspace: its preferences, enough
// to open the window.
func newAppShell(dir, dataDir string) *app {
	return newAppShellWithService(dir, dataDir, backend.NewService(dataDir))
}

func newAppShellWithService(dir, dataDir string, service backend.WorkspaceService) *app {
	a := &app{cwd: dir, path: dir, dataDir: dataDir, service: service, started: time.Now()}
	a.loadPreferences()
	a.appearanceDark = a.prefs.Appearance == "dark" || a.prefs.Appearance == "system" && systemDark()
	a.loadHostInfo()
	return a
}

// openWorkspace restores the saved tabs and their sessions, or opens new
// ones. With a data directory the session server must be running.
func (a *app) openWorkspace() {
	dir, dataDir := a.cwd, a.dataDir
	if dataDir != "" {
		a.persistQueue = make(chan persistJob, 1)
		go a.persistenceWriter()
		if a.restoreWorkspace() {
			return
		}
	}
	first := newPane(a, dir)
	second := newPane(a, dir)
	if second.term != nil {
		second.term.initialFocus = false
	}
	nextID++
	gitPane := &pane{id: nextID, git: &gitState{dir: dir}}
	gitPane.git.refresh()
	a.tabs = []*workspace{{title: filepath.Base(dir), root: &pane{vertical: true, ratio: .54647, first: &pane{ratio: .43328, first: first, second: gitPane}, second: second}}}
	a.focusPane(first)
}
func (p *pane) each(fn func(*pane)) {
	if p == nil {
		return
	}
	if p.first != nil {
		p.first.each(fn)
		p.second.each(fn)
	} else {
		fn(p)
	}
}
func (p *pane) close() {
	p.each(func(p *pane) {
		if p.term != nil {
			p.term.glyphRenderer.Release()
			p.term.session.Close()
		}
	})
}
func (a *app) newTab() { a.newTabAt(a.currentDir()) }
func (a *app) newTabAt(dir string) {
	p := newPane(a, dir)
	a.tabs = append(a.tabs, &workspace{root: p, title: filepath.Base(dir)})
	a.active = len(a.tabs) - 1
	a.focusPane(p)
}
func (a *app) activate(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	a.active = i
	tab := a.tabs[i]
	target := tab.focus
	if target == nil {
		tab.root.each(func(p *pane) {
			if target == nil {
				target = p
			}
		})
	}
	a.focusPane(target)
}
func (a *app) closeTab() {
	a.tabs[a.active].root.close()
	a.tabs = append(a.tabs[:a.active], a.tabs[a.active+1:]...)
	if len(a.tabs) == 0 {
		a.newTab()
	}
	a.activate(min(a.active, len(a.tabs)-1))
}
func (a *app) split(vertical bool) {
	if a.focused == nil {
		return
	}
	target := a.focused
	dir := a.cwd
	if target.term != nil {
		dir = target.term.session.Directory()
	}
	fresh := newPane(a, dir)
	var walk func(**pane)
	walk = func(n **pane) {
		if *n == target {
			*n = &pane{first: target, second: fresh, vertical: vertical, ratio: .5}
			return
		}
		if (*n).first != nil {
			walk(&(*n).first)
			walk(&(*n).second)
		}
	}
	walk(&a.tabs[a.active].root)
	a.tabs[a.active].zoom = false
	a.focusPane(fresh)
}
func (a *app) closePane(target *pane) {
	if target == nil {
		return
	}
	var remove func(*pane) *pane
	remove = func(n *pane) *pane {
		if n == target {
			n.close()
			return nil
		}
		if n.first == nil {
			return n
		}
		n.first = remove(n.first)
		n.second = remove(n.second)
		if n.first == nil {
			return n.second
		}
		if n.second == nil {
			return n.first
		}
		return n
	}
	w := a.tabs[a.active]
	w.root = remove(w.root)
	w.zoom = false
	if w.root == nil {
		a.closeTab()
		return
	}
	var next *pane
	w.root.each(func(p *pane) {
		if next == nil || p.term != nil {
			next = p
		}
	})
	a.focusPane(next)
}

// Window closure detaches views. Explicit pane/tab closure ends that session.
func (a *app) close() {
	if a.closed {
		return
	}
	a.persist(true)
	if a.window != nil {
		width, height := a.window.Size()
		a.rememberWindowSize(width, height)
	}
	a.savePreferences()
	a.closed = true
	if a.persistQueue != nil {
		close(a.persistQueue)
	}
	for _, t := range a.tabs {
		t.root.each(func(p *pane) {
			if p.term != nil {
				p.term.glyphRenderer.Release()
				p.term.session.Detach()
			}
		})
	}
}
func (a *app) endAll() {
	a.persist(true)
	a.closed = true
	if a.persistQueue != nil {
		close(a.persistQueue)
	}
	if a.dataDir != "" {
		a.backendService().Shutdown()
		a.dataDir = ""
	}
	for _, t := range a.tabs {
		t.root.close()
	}
	if a.window != nil {
		a.window.Close()
	}
}
func (a *app) codex() {
	if a.focused == nil {
		return
	}
	if a.focused.term == nil {
		a.split(true)
	}
	if a.focused.term == nil {
		return
	}
	a.focused.term.session.SendText("codex\r", false)
	a.focused.term.initialFocus = true
	a.palette = false
}
func (a *app) Render(cx *el.Context) el.Element {
	w, h := cx.ViewportSize()
	a.shortcuts(cx)
	// Activity, appearance and the saved layout are checked between frames:
	// the window draws only when a check finds something to show.
	cx.Poll("checks", 700*time.Millisecond, func() bool {
		changed := a.checkActivity()
		changed = a.refreshAppearance() || changed
		a.persist(false)
		return changed
	})
	root := el.Div().W(el.Dp(w)).H(el.Dp(h)).Child(el.Widget(&chrome{a}))
	a.renderHeader(cx, root, w)
	space := a.tabs[a.active]
	if space.zoom && a.focused != nil {
		root.Child(a.pane(cx, a.focused, 8, 46, w-16, h-54))
	} else {
		a.layout(cx, root, space.root, 8, 46, w-16, h-54)
	}
	if a.notice != "" && !a.overlayOpen() {
		root.Child(el.Div().Absolute().Left(16).Top(h-68).W(el.Dp(max(180, min(w-32, 560)))).Bg(a.colors().panel).Border(1, a.colors().border).Rounded(8).P(10).Role("status").Child(el.Text(a.notice).TextSize(11).TextColor(a.colors().text).MaxLines(2), el.Div().Role("button").Name("Dismiss notice").OnClick(func() { a.notice = "" }).Child(el.Text("Dismiss").TextSize(10).TextColor(a.colors().muted))))
	}
	a.overlays(cx, w, h)
	return root
}
func (a *app) layout(cx *el.Context, root *el.DivEl, n *pane, x, y, w, h float32) {
	n.bounds = paneRect{x, y, w, h}
	if n.first == nil {
		n.bounds = paneRect{x, y, w, h}
		root.Child(a.pane(cx, n, x, y, w, h))
		return
	}
	if n.vertical {
		ww := (w - 8) * n.ratio
		a.layout(cx, root, n.first, x, y, ww, h)
		a.layout(cx, root, n.second, x+ww+8, y, w-ww-8, h)
		root.Child(a.divider(cx, n, x+ww, y, 8, h, w-8))
	} else {
		hh := (h - 8) * n.ratio
		a.layout(cx, root, n.first, x, y, w, hh)
		a.layout(cx, root, n.second, x, y+hh+8, w, h-hh-8)
		root.Child(a.divider(cx, n, x, y+hh, w, 8, h-8))
	}
}
func (a *app) pane(cx *el.Context, p *pane, x, y, w, h float32) el.Element {
	p.bounds = paneRect{x, y, w, h}
	colors := a.colors()
	root := el.Div().Absolute().Left(x).Top(y).W(el.Dp(w)).H(el.Dp(h)).Bg(colors.panel).Border(1, colors.border).TextColor(colors.text).Rounded(10).P(10).Gap(7)
	name := "Shell"
	icon := "fish"
	if p.term != nil {
		name = programTitle(p.term.session) + " · " + shortPath(p.term.session.Directory())
		icon = programIcon(p.term.session)
	}
	if p.git != nil {
		name = "Git Changes · " + shortPath(p.git.dir)
		icon = "git"
	}
	head := el.Div().ID(fmt.Sprintf("pane-head-%d", p.id)).OnClick(func() { a.focusPane(p) }).OnContextMenu(func() { a.contextPane = p }).Role("group").Name("Pane header "+strconv.Itoa(p.id)).Focusable(false).Row().H(el.Dp(26)).NoShrink().Items(el.Center).Gap(6).Child(el.Widget(core.Func(func(gtx core.C) core.D {
		pp := painter{gtx, gtx.Metric.PxPerDp}
		pp.glyph(icon, 1.75, 1.75, 14.5, colors.text)
		return core.D{Size: image.Pt(gtx.Dp(18), gtx.Dp(18))}
	})), el.Text(name).TextSize(13).Bold().MaxLines(1).Grow().W(el.Dp(0)))
	tool := func(n, ic string, fn func()) el.Element {
		return el.Div().Role("button").Name(n + " pane " + strconv.Itoa(p.id)).OnClick(fn).NoShrink().Rounded(10).Hover(func(s *el.Style) { s.Bg(colors.hover) }).W(el.Dp(26)).H(el.Dp(26)).Child(el.Widget(core.Func(func(gtx core.C) core.D {
			sc := gtx.Metric.PxPerDp
			pp := painter{gtx, sc}
			pp.symbol(ic, 5, 5, 16, 16)
			return core.D{Size: gtx.Constraints.Max}
		})))
	}
	expandIcon, expandName := "expand", "Expand"
	if a.tabs[a.active].zoom && a.focused == p {
		expandIcon, expandName = "restore", "Restore"
	}
	head.Child(tool("Split vertically", "split-v", func() { a.focusPane(p); a.split(true) }), tool("Split horizontally", "split-h", func() { a.focusPane(p); a.split(false) }), tool(expandName, expandIcon, func() { a.focusPane(p); a.tabs[a.active].zoom = !a.tabs[a.active].zoom }), tool("Close", "close", func() { a.requestClosePane(p) }))
	if a.focused == p {
		root.Border(1, rgb(0x92b3ad))
	}
	if p.attention {
		head.Child(el.Div().Size(el.Dp(6)).Rounded(3).Bg(rgb(0xdc9b45)))
	}
	root.Child(head)
	if p.term != nil {
		frame := p.term.session.Snapshot(0, 0)
		if frame.Exited {
			root.Child(el.Div().Row().H(el.Dp(25)).NoShrink().Items(el.Center).Gap(8).Child(el.Text("Session ended · "+shortPath(frame.Dir)).TextSize(11).TextColor(colors.muted).Grow().MaxLines(1), el.Div().Role("button").Name("Restart ended session").Disabled(p.restarting).OnClick(func() { a.focusPane(p); a.restartFocused() }).Px(8).Py(3).Bg(colors.hover).Rounded(4).Child(el.Text("Restart").TextSize(11))))
		}
		root.Child(el.Widget(p.term).Grow().H(el.Dp(0)).W(el.Dp(w - 22)))
	} else if p.git != nil {
		root.Child(p.git.Render(cx, max(float32(20), w-22), max(float32(30), h-53)))
	} else {
		root.Child(el.Text(p.err).TextSize(12).TextColor(red))
	}
	return root
}
func shortPath(s string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(s, home) {
		return "~" + strings.TrimPrefix(s, home)
	}
	return s
}
func fmtInt(i int) string { return "F" + strconv.Itoa(i) }

type chrome struct{ a *app }

func (c *chrome) Layout(gtx core.C) core.D {
	sz := gtx.Constraints.Max
	sc := gtx.Metric.PxPerDp
	if sc == 0 {
		sc = 1
	}
	w, h := float32(sz.X)/sc, float32(sz.Y)/sc
	p := painter{gtx, sc}
	radius := 0
	if desktopChrome.trafficLights {
		radius = int(16 * sc)
	}
	defer clip.RRect{Rect: image.Rectangle{Max: sz}, NE: radius, NW: radius, SE: radius, SW: radius}.Push(gtx.Ops).Pop()
	colors := c.a.colors()
	for y := float32(0); y < h; y++ {
		t := y / h
		mix := func(a, b uint8) uint8 { return uint8(float32(a)*(1-t) + float32(b)*t) }
		p.rect(0, y, w, 1, 0, color.NRGBA{R: mix(colors.top.R, colors.bottom.R), G: mix(colors.top.G, colors.bottom.G), B: mix(colors.top.B, colors.bottom.B), A: 255})
	}
	a := c.a
	left := desktopChrome.hostLeft
	hostClip := clip.Rect(image.Rect(int(left*sc), 0, int((left+desktopChrome.hostWidth())*sc), int(44*sc))).Push(gtx.Ops)
	p.symbol("device", left+6, 13, 18, 16)
	p.label(hostname(), left+33, 21, 13, a.colors().text, false, true)
	p.label(a.hostModel, left+33, 33, 10, a.colors().muted, false, false)
	hostClip.Pop()
	return core.D{Size: sz}
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (a *app) currentDir() string {
	if a.focused != nil {
		if a.focused.term != nil {
			return a.focused.term.session.Directory()
		}
		if a.focused.git != nil {
			return a.focused.git.dir
		}
	}
	return a.cwd
}

func (a *app) togglePalette() {
	a.palette = !a.palette
	if a.palette {
		a.paletteFocus = true
		a.path = a.currentDir()
	} else if a.focused != nil && a.focused.term != nil {
		a.focused.term.initialFocus = true
	}
}
