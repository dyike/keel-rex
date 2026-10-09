package ui

import (
	"fmt"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"image"
	"image/color"
	"math"
)

func (a *app) tabStripLayout(w float32) (gap, rowWidth, tabWidth float32) {
	width := max(float32(0), desktopChrome.toolbarRight(w)-desktopChrome.tabsLeft()-82)
	count := len(a.tabs)
	if count == 0 {
		return
	}
	gap = min(float32(2), width/float32(count)*.1)
	rowWidth = min(max(float32(0), width-6), float32(count)*240+float32(count-1)*gap)
	tabWidth = max(float32(0), (rowWidth-float32(count-1)*gap)/float32(count))
	return
}

func (a *app) renderTabs(cx *el.Context, root *el.DivEl, w float32) {
	if len(a.tabs) == 0 {
		return
	}
	c := a.colors()
	left := desktopChrome.tabsLeft()
	gap, rowWidth, tabWidth := a.tabStripLayout(w)
	padding := min(float32(8), max(float32(0), (tabWidth-16)/2))
	contentGap := min(float32(8), max(float32(0), (tabWidth-40)/4))
	frameColor, edgeColor, hoverColor := c.track, c.border, c.hover
	if a.prefs.Appearance != "dark" && !(a.prefs.Appearance == "system" && a.appearanceDark) {
		frameColor = rgb(0xf3eaf3)
		edgeColor = color.NRGBA{R: 255, G: 255, B: 255, A: 220}
		hoverColor = rgb(0xe9e2e9)
	}
	track := el.Div().ID("tab-strip").Absolute().Left(left).Top(5).W(el.Dp(rowWidth+6)).H(el.Dp(36)).Rounded(18).Border(1, edgeColor).P(2).Bg(frameColor)
	row := el.Div().Row().Gap(gap).W(el.Dp(rowWidth)).H(el.Dp(30)).NoShrink()
	for index, tab := range a.tabs {
		index, tab := index, tab
		id := fmt.Sprintf("tab-%p", tab)
		item := el.Div().ID(id).W(el.Dp(0)).Grow().H(el.Dp(30)).Row().Items(el.Center).Px(padding).Gap(contentGap).Rounded(16).Role("button").Name("Terminal tab " + fmt.Sprint(index+1) + " · " + a.tabLabel(tab)).OnClick(func() { a.activate(index) }).OnDoubleClick(func() { a.activate(index); a.startRename() }).OnContextMenu(func() { a.menuTab = tab }).Hover(func(s *el.Style) { s.Bg(hoverColor) })
		if index == a.active {
			item.Bg(c.active)
		}
		item.OnDrag(func(e el.DragEvent) {
			switch e.Kind {
			case el.DragStart:
				a.draggingTab = tab
				a.dragOrigin = e.X
			case el.DragEnd:
				if !e.Canceled && a.draggingTab == tab && tabWidth+gap > 0 && math.Abs(float64(e.X-a.dragOrigin)) > 8 {
					to := index + int(math.Round(float64((e.X-a.dragOrigin)/(tabWidth+gap))))
					a.moveTab(tab, to)
				}
				a.draggingTab = nil
			}
		})
		if tabWidth >= 64 {
			iconWidth := float32(26)
			if tabWidth >= 96 {
				iconWidth = 38
			}
			item.Child(el.Widget(core.Func(func(gtx core.C) core.D {
				p := painter{gtx, gtx.Metric.PxPerDp}
				if tabWidth >= 96 {
					p.programStack(tab, 0, 4)
				} else {
					icon := "fish"
					if tab.focus != nil {
						icon = "git"
						if tab.focus.term != nil {
							icon = programIcon(tab.focus.term.session)
						}
					}
					p.tiltedProgramTile(icon, 1, 4, 24, 20, -.055)
				}
				return core.D{Size: image.Pt(gtx.Dp(unit.Dp(iconWidth)), gtx.Dp(28))}
			})).NoShrink())
		}
		label := el.Text(a.tabLabel(tab)).TextSize(12.5).Bold().TextColor(c.muted).MaxLines(1).Grow().W(el.Dp(0))
		if index == a.active {
			label.TextColor(c.text)
		}
		item.Child(label)
		attention, busy := false, false
		tab.root.each(func(p *pane) { attention = attention || p.attention; busy = busy || a.busy(p) })
		if cx.Hovered(id) && tabWidth >= 32 {
			item.Child(el.Div().W(el.Dp(16)).H(el.Dp(20)).NoShrink().Role("button").Name("Close tab " + fmt.Sprint(index+1)).OnClick(func() { a.closeSpecificTab(tab) }).Child(el.Widget(core.Func(func(gtx core.C) core.D {
				p := painter{gtx, gtx.Metric.PxPerDp}
				p.symbol("close", 0, 2, 16, 16)
				return core.D{Size: gtx.Constraints.Max}
			}))))
		} else if tabWidth >= 40 && (attention || busy) {
			color := green
			if attention {
				color = rgb(0xdc9b45)
			}
			item.Child(el.Div().Size(el.Dp(6)).NoShrink().Rounded(3).Bg(color))
		}
		row.Child(item)
	}
	track.Child(row)
	root.Child(track)
}
