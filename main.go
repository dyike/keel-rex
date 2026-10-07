// Rex: native Keel terminal workspace, backed by real PTY sessions.
package main

import (
	"flag"
	"fmt"
	"gioui.org/io/system"
	"gioui.org/op/clip"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"runtime"
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

	id                      int
	first, second           *pane
	vertical                bool
	ratio                   float32
	dragRatio, dragX, dragY float32
	term                    *terminal
	git                     *gitState
	err                     string
}
type workspace struct {
	focus       *pane
	customTitle bool

	root  *pane
	zoom  bool
	title string
}
type app struct {
	window                              *window.Window
	closed                              bool
	dataDir                             string
	prefs                               preferences
	saveTimer                           int
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
	hostDetails                         hostDetails
	hostStatus                          hostStatus
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
	var s *session
	var e error
	if a.dataDir != "" {
		s, e = connectSession(a.dataDir, "", dir)
	} else {
		s, e = newSession(dir)
	}
	if e != nil {
		p.err = e.Error()
	} else {
		p.term = &terminal{session: s, owner: a, pane: p, initialFocus: true}
	}
	return p
}
func newApp(dir string) *app { return newAppAt(dir, "") }
func newAppAt(dir, dataDir string) *app {
	a := &app{cwd: dir, path: dir, dataDir: dataDir, started: time.Now()}
	a.loadPreferences()
	a.appearanceDark = a.prefs.Appearance == "dark" || a.prefs.Appearance == "system" && systemDark()
	a.loadHostInfo()
	if dataDir != "" {
		a.persistQueue = make(chan persistJob, 1)
		go a.persistenceWriter(dataDir)
		if a.restoreWorkspace() {
			return a
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
	return a
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
			p.term.session.close()
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
		dir = target.term.session.directory()
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
				p.term.session.detach()
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
		callServer(a.dataDir, rpcRequest{Op: "shutdown"})
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
	a.focused.term.session.text("codex\r", false)
	a.focused.term.initialFocus = true
	a.palette = false
}
func (a *app) Render(cx *el.Context) el.Element {
	w, h := cx.ViewportSize()
	a.shortcuts(cx)
	cx.After(a.saveTimer, 700*time.Millisecond, func() { a.saveTimer++; a.checkActivity(); a.refreshAppearance(); a.persist(false) })
	root := el.Div().W(el.Dp(w)).H(el.Dp(h)).Child(el.Widget(&chrome{a}))
	button := func(name string, x, y, ww, hh float32, fn func()) *el.DivEl {
		b := el.Div().Absolute().Left(x).Top(y).W(el.Dp(ww)).H(el.Dp(hh)).Rounded(5).Role("button").Name(name).OnClick(fn).Hover(func(s *el.Style) { s.Bg(a.colors().hover) })
		icon := ""
		if name == "New terminal tab" {
			icon = "plus"
		}
		if name == "Command palette" {
			icon = "command"
		}
		if icon != "" {
			b.Child(el.Widget(core.Func(func(gtx core.C) core.D {
				p := painter{gtx, gtx.Metric.PxPerDp}
				size := float32(17)
				if icon == "plus" {
					size = 19
				}
				p.symbol(icon, (ww-size)/2, (hh-size)/2, size, size)
				return core.D{Size: gtx.Constraints.Max}
			})))
		}
		return b
	}
	wc := core.CurrentWindow()
	if wc != nil && runtime.GOOS != "darwin" {
		root.Child(button("Close window", 13, 13, 18, 18, wc.Close).ID("window-close"), button("Minimize window", 36, 13, 18, 18, wc.Minimize).ID("window-minimize"), button("Zoom window", 59, 13, 18, 18, wc.ToggleMaximize).ID("window-zoom"))
	}
	// Keep native drag areas outside the host chip, tabs and toolbar buttons.
	for _, rail := range []paneRect{{0, 0, w, 5}, {0, 41, w, 5}, {78, 5, 11, 36}} {
		r := rail
		root.Child(el.Div().Absolute().Left(r.X).Top(r.Y).W(el.Dp(r.W)).H(el.Dp(r.H)).Decorate(func(gtx core.C, draw func()) {
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
			draw()
		}))
	}

	a.renderTabs(cx, root, w)
	root.Child(el.Div().ID("host-chip").Absolute().Left(89).Top(5).W(el.Dp(110)).H(el.Dp(34)).Role("button").Name("Host information").OnClick(a.toggleHostInfo))
	root.Child(button("New terminal tab", w-37, 9, 28, 27, a.newTab), button("Command palette", w-72, 9, 29, 27, a.openPalette))
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
	if n.first == nil {
		n.bounds = paneRect{x, y, w, h}
		root.Child(a.pane(cx, n, x, y, w, h))
		return
	}
	if n.vertical {
		ww := (w - 8) * n.ratio
		a.layout(cx, root, n.first, x, y, ww, h)
		a.layout(cx, root, n.second, x+ww+8, y, w-ww-8, h)
		root.Child(el.Div().Absolute().Left(x + ww).Top(y).W(el.Dp(8)).H(el.Dp(h)).Role("separator").Name("Vertical divider").OnDoubleClick(func() { n.ratio = .5 }).OnDrag(func(e el.DragEvent) {
			if e.Kind == el.DragStart {
				n.dragRatio, n.dragX = n.ratio, e.X
			}
			if e.Kind == el.DragMove {
				n.ratio = max(.15, min(.85, (e.X-n.dragX)/max(1, w-8)+n.ratio))
			}
		}))
	} else {
		hh := (h - 8) * n.ratio
		a.layout(cx, root, n.first, x, y, w, hh)
		a.layout(cx, root, n.second, x, y+hh+8, w, h-hh-8)
		root.Child(el.Div().Absolute().Left(x).Top(y + hh).W(el.Dp(w)).H(el.Dp(8)).Role("separator").Name("Horizontal divider").OnDoubleClick(func() { n.ratio = .5 }).OnDrag(func(e el.DragEvent) {
			if e.Kind == el.DragStart {
				n.dragRatio, n.dragY = n.ratio, e.Y
			}
			if e.Kind == el.DragMove {
				n.ratio = max(.15, min(.85, (e.Y-n.dragY)/max(1, h-8)+n.ratio))
			}
		}))
	}
}
func (a *app) pane(cx *el.Context, p *pane, x, y, w, h float32) el.Element {
	p.bounds = paneRect{x, y, w, h}
	colors := a.colors()
	root := el.Div().Absolute().Left(x).Top(y).W(el.Dp(w)).H(el.Dp(h)).Bg(colors.panel).Border(1, colors.border).TextColor(colors.text).Rounded(10).P(10).Gap(7)
	name := "Shell"
	icon := "fish"
	if p.term != nil {
		name = programTitle(p.term.session) + " · " + shortPath(p.term.session.directory())
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
	})), el.Text(name).TextSize(12).Bold().MaxLines(1).Grow().W(el.Dp(0)))
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
		frame := p.term.session.snapshot(0, 0)
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
	defer clip.RRect{Rect: image.Rectangle{Max: sz}, NE: int(16 * sc), NW: int(16 * sc), SE: int(16 * sc), SW: int(16 * sc)}.Push(gtx.Ops).Pop()
	colors := c.a.colors()
	for y := float32(0); y < h; y++ {
		t := y / h
		mix := func(a, b uint8) uint8 { return uint8(float32(a)*(1-t) + float32(b)*t) }
		p.rect(0, y, w, 1, 0, color.NRGBA{R: mix(colors.top.R, colors.bottom.R), G: mix(colors.top.G, colors.bottom.G), B: mix(colors.top.B, colors.bottom.B), A: 255})
	}
	a := c.a
	if runtime.GOOS != "darwin" {
		for i := 0; i < 3; i++ {
			p.trafficLight(i, 15+float32(i*23), 15, false)
		}
	}
	hostClip := clip.Rect(image.Rect(int(89*sc), 0, int(199*sc), int(44*sc))).Push(gtx.Ops)
	p.symbol("device", 95, 13, 18, 16)
	p.label(hostname(), 122, 21, 13, a.colors().text, false, true)
	host, _ := os.Hostname()
	_ = host
	p.label(a.hostModel, 122, 33, 10, a.colors().muted, false, false)
	hostClip.Pop()
	return core.D{Size: sz}
}
func main() {
	snake := flag.Bool("snake", false, "Run bundled Snake in the current terminal")
	dir := flag.String("dir", "", "Initial working directory")
	screenshot := flag.String("screenshot", "", "Render native screenshot and exit")
	server := flag.Bool("server", false, "Run persistent PTY session service")
	stateDir := flag.String("state-dir", "", "Workspace/session data directory")
	ephemeral := flag.Bool("ephemeral", false, "Use local sessions without persistence")
	flag.Parse()
	if *server {
		if *stateDir == "" {
			*stateDir = stateDirectory()
		}
		if e := runSessionServer(*stateDir); e != nil {
			log.Fatal(e)
		}
		return
	}
	// Flags must be registered before parsing.
	if *snake {
		runSnake()
		return
	}
	if *dir == "" {
		*dir, _ = os.Getwd()
		if *dir == "/" || *dir == "" {
			*dir, _ = os.UserHomeDir()
		}
	}
	abs, e := filepath.Abs(*dir)
	if e != nil {
		log.Fatal(e)
	}
	fonts := []string{"/System/Library/Fonts/Menlo.ttc", "/System/Library/Fonts/HelveticaNeue.ttc"}
	cjk, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*/*.asset/AssetData/PingFang.ttc")
	if len(cjk) > 0 {
		fonts = append(fonts, cjk[0])
	}
	for _, path := range fonts {
		if b, e := os.ReadFile(path); e == nil {
			if e = theme.LoadFonts(b); e != nil {
				log.Fatal(e)
			}
		}
	}
	theme.Material.Face = "Helvetica Neue"
	var a *app
	if *ephemeral || *screenshot != "" {
		a = newApp(abs)
	} else {
		if *stateDir == "" {
			*stateDir = stateDirectory()
		}
		if e := ensureServer(*stateDir); e != nil {
			log.Fatal(e)
		}
		a = newAppAt(abs, *stateDir)
	}
	a.applyAppearance()
	root := &workspaceView{app: a, root: el.Root(a)}
	if *screenshot != "" {
		defer a.close()
		if e := window.ScreenshotAtScale(root, 1057, 639, 2, *screenshot); e != nil {
			log.Fatal(e)
		}
		return
	}
	a.window = window.Open(window.Options{Title: fmt.Sprintf("Rex · %s", filepath.Base(abs)), Width: a.prefs.WindowWidth, Height: a.prefs.WindowHeight, MinWidth: 800, MinHeight: 480, OnResize: a.rememberWindowSize, Frameless: true, NativeTrafficLights: true, TrafficLightLayout: &window.TrafficLightLayout{Height: 44, Left: 15, Spacing: 23}, Content: root, OnClose: a.close})
	installApplicationMenu(a)
	if a.prefs.Notifications {
		a.enableNotifications()
	}
	window.Main()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (a *app) currentDir() string {
	if a.focused != nil {
		if a.focused.term != nil {
			return a.focused.term.session.directory()
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
