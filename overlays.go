package main

import (
	"fmt"
	"github.com/dyike/keel/ui/el"
	"os"
	"strings"
	"time"
)

func (a *app) menuButton(name, hint string, run func()) el.Element {
	c := a.colors()
	glyph := actionGlyph(name)
	if glyph == "expand" && len(a.tabs) > 0 && a.tabs[a.active].zoom {
		glyph = "restore"
	}
	return el.Div().Row().H(el.Dp(32)).NoShrink().Px(10).Items(el.Center).Gap(9).Rounded(5).Role("button").Name(name).OnClick(run).Hover(func(s *el.Style) { s.Bg(c.hover) }).Child(iconElement(glyph, 15), el.Text(name).TextSize(12).Bold().TextColor(c.text).MaxLines(1).Grow().W(el.Dp(0)), el.Text(hint).TextSize(10).TextColor(c.muted).NoShrink())
}
func (a *app) overlays(cx *el.Context, w, h float32) {
	c := a.colors()
	panel := func(width float32) *el.DivEl {
		return el.Div().W(el.Dp(min(width, max(float32(100), w-32)))).Bg(c.panel).TextColor(c.text).Border(1, c.border).Rounded(12).P(14).Gap(8)
	}
	modal := func(id string, content el.Element) { cx.Overlay(id, el.Modal(content).OnDismiss(a.closeOverlay)) }
	if a.palette {
		if a.paletteFocus {
			cx.Focus("command-query")
			a.paletteFocus = false
		}
		if a.lastQuery != a.query {
			a.paletteIndex = 0
			a.lastQuery = a.query
			cx.ScrollTo("command-results", 0)
		}
		rows := a.paletteResults()
		a.paletteIndex = max(0, min(a.paletteIndex, len(rows)-1))
		listHeight := min(float32(320), max(float32(64), h-200))
		// Keep the 10dp scrollbar track and 2dp clearance outside the buttons.
		list := el.Div().ID("command-results").H(el.Dp(listHeight)).ScrollY().Pr(12).Gap(2)
		for i, row := range rows {
			i, row := i, row
			button := a.menuButton(row.Title, row.Hint, func() { a.closeOverlay(); row.Run() }).(*el.DivEl)
			if i == a.paletteIndex {
				button.Bg(c.hover)
			}
			list.Child(button)
		}
		if len(rows) == 0 {
			list.Child(el.Text("No matching commands or panes").TextSize(12).TextColor(c.muted))
		}
		if a.paletteReveal != a.paletteIndex {
			cx.ScrollIntoView("command-results", float32(a.paletteIndex)*34, float32(a.paletteIndex+1)*34)
			a.paletteReveal = a.paletteIndex
		}
		content := panel(540).Child(el.Div().Row().Items(el.Center).Gap(9).Child(iconElement("search", 17), el.Input().ID("command-query").Bind(&a.query).Name("Command search").Placeholder("Search commands or panes…").OnSubmit(func(string) { a.runPaletteSelection() }).Grow().W(el.Dp(0))), list, el.Text("↑ ↓ select · Enter run · Esc close").TextSize(10).TextColor(c.muted))
		modal("command-palette", content)
	}
	if a.directoryOpen {
		if a.editFocus {
			cx.Focus("working-directory")
			a.editFocus = false
		}
		open := func() {
			dir, e := a.validateDirectory(a.path)
			if e != nil {
				a.notice = e.Error()
				return
			}
			a.closeOverlay()
			a.cwd = dir
			a.newTabAt(dir)
		}
		content := panel(460).Child(el.Div().Row().Items(el.Center).Gap(9).Child(iconElement("folder-open", 18), el.Text("Open directory").Bold().TextSize(17)), el.Input().ID("working-directory").Bind(&a.path).SelectOnFocus(true).Name("Working directory").OnSubmit(func(string) { open() }), a.menuButton("Open in new tab", "↩", open))
		if a.notice != "" {
			content.Child(el.Text(a.notice).TextSize(11).TextColor(red).MaxLines(2))
		}
		modal("open-directory", content)
	}
	if a.renameOpen {
		if a.editFocus {
			cx.Focus("rename-tab")
			a.editFocus = false
		}
		modal("rename-tab", panel(400).Child(el.Text("Rename tab").Bold().TextSize(17), el.Input().ID("rename-tab").Bind(&a.renameText).SelectOnFocus(true).Name("Tab name").Placeholder("Leave empty to follow the active program").OnSubmit(func(string) { a.commitRename() }), a.menuButton("Save name", "↩", a.commitRename)))
	}
	if a.confirmAction != nil {
		modal("confirm-action", panel(430).Child(el.Text(a.confirmTitle).TextSize(15).Bold(), el.Text("Closing a pane ends its session. Closing the window keeps sessions running.").TextSize(12).TextColor(c.muted), a.menuButton("Cancel", "Esc", a.closeOverlay), a.menuButton("End session", "", func() { run := a.confirmAction; a.closeOverlay(); run() })))
	}
	if a.menuTab != nil {
		tab := a.menuTab
		menu := panel(240).P(6).Child(a.menuButton("Rename tab", "⇧⌘R", func() {
			a.closeOverlay()
			for i, t := range a.tabs {
				if t == tab {
					a.activate(i)
				}
			}
			a.startRename()
		}), a.menuButton("Move tab left", "", func() {
			a.closeOverlay()
			for i, t := range a.tabs {
				if t == tab {
					a.moveTab(tab, i-1)
					break
				}
			}
		}), a.menuButton("Move tab right", "", func() {
			a.closeOverlay()
			for i, t := range a.tabs {
				if t == tab {
					a.moveTab(tab, i+1)
					break
				}
			}
		}), a.menuButton("Close tab", "⇧⌘W", func() { a.closeOverlay(); a.closeSpecificTab(tab) }))
		cx.Overlay("tab-context", el.Anchored(fmt.Sprintf("tab-%p", tab), menu).OnDismiss(a.closeOverlay))
	}
	if a.contextPane != nil {
		p := a.contextPane
		menu := panel(240).P(6).Child(a.menuButton("Split right", "⌘D", func() { a.closeOverlay(); a.focusPane(p); a.split(true) }), a.menuButton("Split down", "⇧⌘D", func() { a.closeOverlay(); a.focusPane(p); a.split(false) }), a.menuButton("Zoom / restore", "⇧⌘↩", func() { a.closeOverlay(); a.focusPane(p); a.tabs[a.active].zoom = !a.tabs[a.active].zoom }), a.menuButton("Close pane", "⌘W", func() { a.closeOverlay(); a.requestClosePane(p) }))
		cx.Overlay("pane-context", el.Anchored(fmt.Sprintf("pane-head-%d", p.id), menu).OnDismiss(a.closeOverlay))
	}
	if a.hostOpen {
		cx.After(struct{ HostPoll int }{a.hostPoll}, time.Second, func() {
			a.hostPoll++
			a.refreshHostStatus()
		})
		content := a.hostPanel(min(float32(300), max(float32(100), w-32)))
		cx.Overlay("host-information", el.Anchored("host-chip", content).Placement(el.Bottom, el.Start).OnDismiss(a.closeOverlay))
	}
	if a.searchOpen {
		if a.editFocus {
			cx.Focus("terminal-search")
			a.editFocus = false
		}
		search := func() {
			if a.focused != nil && a.focused.term != nil {
				a.focused.term.find(a.searchQuery)
			}
		}
		modal("terminal-search", panel(400).Child(el.Div().Row().Items(el.Center).Gap(9).Child(iconElement("search", 18), el.Text("Find in terminal").Bold().TextSize(17)), el.Input().ID("terminal-search").Bind(&a.searchQuery).Name("Find text").OnSubmit(func(string) { search() }), a.menuButton("Find next", "↩", search), el.Text(a.notice).TextSize(11).TextColor(red).MaxLines(2)))
	}
}
func hostname() string { host, _ := os.Hostname(); return strings.TrimSuffix(host, ".local") }
