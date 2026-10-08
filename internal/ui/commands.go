package ui

import (
	"fmt"
	"github.com/dyike/keel/ui/el"
	"os"
	"sort"
	"strings"
	"unicode"
)

type appCommand struct {
	ID, Title, Shortcut, Hint string
	Run                       func()
}

func (a *app) commands() []appCommand {
	commands := []appCommand{
		{"new-tab", "New tab", "mod+t", "⌘T", a.newTab},
		{"split-right", "Split right", "mod+d", "⌘D", func() { a.split(true) }},
		{"split-down", "Split down", "mod+shift+d", "⇧⌘D", func() { a.split(false) }},
		{"close-pane", "Close pane", "mod+w", "⌘W", func() { a.requestClosePane(a.focused) }},
		{"close-tab", "Close tab", "mod+shift+w", "⇧⌘W", a.requestCloseTab},
		{"zoom", "Zoom / restore pane", "mod+shift+enter", "⇧⌘↩", func() { a.tabs[a.active].zoom = !a.tabs[a.active].zoom }},
		{"equalize", "Equalize panes", "mod+ctrl+=", "⌃⌘=", a.equalize},
		{"next-tab", "Next tab", "mod+shift+]", "⇧⌘]", func() { a.cycleTab(1) }},
		{"previous-tab", "Previous tab", "mod+shift+[", "⇧⌘[", func() { a.cycleTab(-1) }},
		{"rename", "Rename tab", "mod+shift+r", "⇧⌘R", a.startRename},
		{"directory", "Open directory", "mod+o", "⌘O", a.openDirectory},
		{"clear", "Clear screen and scrollback", "mod+k", "⌘K", func() {
			if a.focused != nil && a.focused.term != nil {
				a.focused.term.session.Clear()
				a.focused.term.scroll = 0
			}
		}},
		{"restart", "Restart shell", "", "", func() {
			a.confirmTitle = "End the current program and restart this shell?"
			a.confirmAction = a.restartFocused
		}},
		{"larger", "Larger terminal text", "mod+=", "⌘+", func() { a.setFontSize(a.prefs.FontSize + 1) }},
		{"smaller", "Smaller terminal text", "mod+-", "⌘−", func() { a.setFontSize(a.prefs.FontSize - 1) }},
		{"actual-size", "Reset text size", "mod+0", "⌘0", func() { a.setFontSize(12.5) }},
		{"light", "Appearance: Light", "", "", func() { a.setAppearance("light") }},
		{"dark", "Appearance: Dark", "", "", func() { a.setAppearance("dark") }},
		{"system", "Appearance: System", "", "", func() { a.setAppearance("system") }},
		{"codex", "Launch Codex in focused terminal", "", "", a.codex},
		{"git", "Open Git changes pane", "", "", a.openGit},
		{"snake", "Run Snake in focused terminal", "", "", a.launchSnake},
		{"host", "Show host and session server", "", "", a.toggleHostInfo},
		{"search", "Find in terminal", "mod+f", "⌘F", func() { a.searchOpen = true; a.editFocus = true }},
		{"enable-notifications", "Enable system notifications", "", "", a.enableNotifications},
		{"disable-notifications", "Disable system notifications", "", "", func() { a.prefs.Notifications = false; a.savePreferences() }},
		{"end-all", "Quit and end all sessions", "mod+alt+q", "⌥⌘Q", func() { a.confirmTitle = "End every running session and quit Rex?"; a.confirmAction = a.endAll }},
	}
	for _, direction := range []struct {
		name, key, hint string
		x, y            float32
	}{
		{"left", "left", "←", -1, 0}, {"right", "right", "→", 1, 0}, {"up", "up", "↑", 0, -1}, {"down", "down", "↓", 0, 1},
	} {
		d := direction
		commands = append(commands, appCommand{"focus-" + d.name, "Focus pane " + d.name, "mod+alt+" + d.key, "⌥⌘" + d.hint, func() { a.moveFocus(d.x, d.y) }}, appCommand{"divider-" + d.name, "Move divider " + d.name, "mod+ctrl+" + d.key, "⌃⌘" + d.hint, func() { a.resizeDivider(d.x, d.y) }})
	}
	return commands
}
func (a *app) shortcuts(cx *el.Context) {
	if a.confirmAction != nil || a.renameOpen || a.directoryOpen || a.hostOpen || a.searchOpen || a.menuTab != nil || a.contextPane != nil {
		return
	}
	if a.palette {
		rows := a.paletteResults()
		cx.Shortcut("down", func() {
			if len(rows) > 0 {
				a.paletteIndex = (a.paletteIndex + 1) % len(rows)
			}
		})
		cx.Shortcut("up", func() {
			if len(rows) > 0 {
				a.paletteIndex = (a.paletteIndex + len(rows) - 1) % len(rows)
			}
		})
		return
	}
	for _, command := range a.commands() {
		if command.Shortcut != "" {
			cx.Shortcut(command.Shortcut, command.Run)
		}
	}
	cx.Shortcut("mod+shift+p", a.openPalette)
	cx.Shortcut("mod+p", a.openPalette)
	cx.Shortcut("mod+enter", func() { a.tabs[a.active].zoom = !a.tabs[a.active].zoom })
	cx.Shortcut("ctrl+tab", func() { a.cycleTab(1) })
	cx.Shortcut("ctrl+shift+tab", func() { a.cycleTab(-1) })
	cx.Shortcut("mod+shift+o", a.openDirectory)
	for n := 1; n <= 9; n++ {
		n := n
		cx.Shortcut(fmt.Sprintf("mod+%d", n), func() {
			if n == 9 {
				a.activate(len(a.tabs) - 1)
			} else {
				a.activate(n - 1)
			}
		})
	}
	if a.palette {
		rows := a.paletteResults()
		cx.Shortcut("down", func() {
			if len(rows) > 0 {
				a.paletteIndex = (a.paletteIndex + 1) % len(rows)
			}
		})
		cx.Shortcut("up", func() {
			if len(rows) > 0 {
				a.paletteIndex = (a.paletteIndex + len(rows) - 1) % len(rows)
			}
		})
		cx.Shortcut("enter", a.runPaletteSelection)
	}
}
func (a *app) runCommand(id string) {
	for _, command := range a.commands() {
		if command.ID == id {
			command.Run()
			return
		}
	}
}
func (a *app) openPalette() {
	a.palette = true
	a.paletteFocus = true
	a.query = ""
	a.paletteIndex = 0
	a.paletteReveal = -1
	a.menuTab = nil
	a.contextPane = nil
}
func (a *app) overlayOpen() bool {
	return a.palette || a.renameOpen || a.directoryOpen || a.searchOpen || a.hostOpen || a.confirmAction != nil || a.menuTab != nil || a.contextPane != nil
}
func (a *app) closeOverlay() {
	a.notice = ""
	a.palette = false
	a.directoryOpen = false
	a.renameOpen = false
	a.hostOpen = false
	a.menuTab = nil
	a.contextPane = nil
	a.searchOpen = false
	a.confirmTitle = ""
	a.confirmAction = nil
	if a.focused != nil {
		a.focusPane(a.focused)
	}
}
func (a *app) openDirectory() {
	a.directoryOpen = true
	a.editFocus = true
	a.path = a.currentDir()
	a.notice = ""
}
func (a *app) startRename() {
	a.renameOpen = true
	a.editFocus = true
	a.renameText = a.tabs[a.active].title
}
func (a *app) commitRename() {
	tab := a.tabs[a.active]
	tab.title = strings.TrimSpace(a.renameText)
	tab.customTitle = tab.title != ""
	a.closeOverlay()
}
func (a *app) openGit() {
	dir := a.currentDir()
	a.split(false)
	p := a.focused
	if p.term != nil {
		p.term.session.Close()
		p.term = nil
	}
	p.git = &gitState{dir: dir}
	p.git.refresh()
	a.focusPane(p)
}
func (a *app) launchSnake() {
	if a.focused != nil && a.focused.term != nil {
		exe, e := os.Executable()
		if e != nil {
			a.notice = e.Error()
			return
		}
		a.focused.term.session.SendText(shellQuote(exe)+" -snake\r", false)
		a.tabs[a.active].zoom = true
	}
}
func (a *app) runPaletteSelection() {
	rows := a.paletteResults()
	if len(rows) == 0 {
		return
	}
	index := max(0, min(a.paletteIndex, len(rows)-1))
	run := rows[index].Run
	a.closeOverlay()
	run()
}
func (a *app) paletteResults() []appCommand {
	rows := a.commands()
	for index, tab := range a.tabs {
		index, tab := index, tab
		tab.root.each(func(p *pane) {
			label := "Git changes"
			if p.term != nil {
				label = programTitle(p.term.session) + " · " + shortPath(p.term.session.Directory())
			}
			rows = append(rows, appCommand{Title: fmt.Sprintf("Go to tab %d · %s", index+1, label), Run: func() { a.activate(index); a.focusPane(p) }})
		})
	}
	if strings.TrimSpace(a.query) == "" {
		return rows
	}
	type match struct {
		command appCommand
		score   int
	}
	var matches []match
	for _, row := range rows {
		if score, ok := fuzzyScore(a.query, row.Title); ok {
			matches = append(matches, match{row, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	rows = nil
	for _, match := range matches {
		rows = append(rows, match.command)
	}
	return rows
}
func fuzzyScore(query, text string) (int, bool) {
	needle := []rune(strings.ToLower(strings.TrimSpace(query)))
	hay := []rune(strings.ToLower(text))
	i, score, last := 0, 0, -2
	for at, r := range hay {
		if i < len(needle) && r == needle[i] {
			score += 10
			if at == last+1 {
				score += 8
			}
			if at == 0 || unicode.IsSpace(hay[at-1]) {
				score += 5
			}
			score -= at / 8
			last = at
			i++
		}
	}
	return score, i == len(needle)
}
