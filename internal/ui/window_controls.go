package ui

import (
	"gioui.org/f32"
	"gioui.org/io/system"
	"gioui.org/op/clip"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
)

// The caption buttons share the tab bar and invoke the native window actions.
func (a *app) renderWindowControls(cx *el.Context, root *el.DivEl, width float32) {
	root.Child(el.Div().Absolute().Left(width - 140).Top(12).W(el.Dp(1)).H(el.Dp(20)).Bg(a.colors().border))
	maximized := a.window != nil && a.window.Maximized()
	maximizeName := "Maximize window"
	if maximized {
		maximizeName = "Restore window"
	}
	buttons := []struct {
		id, name string
		action   func()
	}{
		{"window-minimize", "Minimize window", func() {
			if a.window != nil {
				a.window.Minimize()
			}
		}},
		{"window-maximize", maximizeName, func() {
			if a.window != nil {
				a.window.ToggleMaximize()
			}
		}},
		{"window-close", "Close window", func() {
			if a.window != nil {
				a.window.Close()
			}
		}},
	}
	for i, button := range buttons {
		b := el.Div().ID(button.id).Absolute().Left(width - 132 + float32(i)*44).Top(0).W(el.Dp(44)).H(el.Dp(44)).Role("button").Name(button.name).OnClick(button.action).Hover(func(s *el.Style) {
			if i == 2 {
				s.Bg(rgb(0xc42b1c))
			} else {
				s.Bg(a.colors().hover)
			}
		})
		b.Child(el.Widget(core.Func(func(gtx core.C) core.D {
			p := painter{gtx, gtx.Metric.PxPerDp}
			ink := a.colors().muted
			if i == 2 && cx.Hovered(button.id) {
				ink = rgb(0xffffff)
			}
			box := func(x, y float32) {
				p.line(x, y, 10, 1, ink)
				p.line(x, y+9, 10, 1, ink)
				p.line(x, y, 1, 10, ink)
				p.line(x+9, y, 1, 10, ink)
			}
			switch i {
			case 0:
				p.line(17, 22, 10, 1, ink)
			case 1:
				if maximized {
					p.line(19, 16, 9, 1, ink)
					p.line(27, 16, 1, 9, ink)
					box(16, 19)
				} else {
					box(17, 17)
				}
			case 2:
				p.stroke(ink, 1, f32.Pt(17, 17), f32.Pt(27, 27))
				p.stroke(ink, 1, f32.Pt(27, 17), f32.Pt(17, 27))
			}
			return core.D{Size: gtx.Constraints.Max}
		})))
		root.Child(b)
	}
}

func (a *app) renderHeader(cx *el.Context, root *el.DivEl, w float32) {
	button := func(name string, x, y, ww, hh float32, fn func()) *el.DivEl {
		b := el.Div().Absolute().Left(x).Top(y).W(el.Dp(ww)).H(el.Dp(hh)).Rounded(5).Role("button").Name(name).OnClick(fn).Hover(func(s *el.Style) { s.Bg(a.colors().hover) })
		icon := ""
		if name == "New terminal tab" {
			icon = "plus"
		}
		if name == "Command palette" {
			icon = desktopChrome.paletteIcon
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
	if desktopChrome.trafficLights || desktopChrome.windowControls {
		// Keep native drag areas outside the host chip, tabs and toolbar buttons.
		rails := []paneRect{{0, 0, w, 5}, {0, 41, w, 5}, {78, 5, 11, 36}}
		if desktopChrome.windowControls {
			_, rowWidth, _ := a.tabStripLayout(w)
			blankLeft := desktopChrome.tabsLeft() + rowWidth + 6
			blankWidth := max(float32(0), desktopChrome.toolbarRight(w)-82-blankLeft)
			rails = []paneRect{{0, 0, w - 132, 5}, {0, 41, w - 132, 5}, {blankLeft, 5, blankWidth, 36}}
		}
		for _, rail := range rails {
			if rail.W <= 0 {
				continue
			}
			r := rail
			root.Child(el.Div().Absolute().Left(r.X).Top(r.Y).W(el.Dp(r.W)).H(el.Dp(r.H)).Decorate(func(gtx core.C, draw func()) {
				defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
				system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
				draw()
			}))
		}
	}

	a.renderTabs(cx, root, w)
	root.Child(el.Div().ID("host-chip").Absolute().Left(desktopChrome.hostLeft).Top(5).W(el.Dp(desktopChrome.hostWidth())).H(el.Dp(34)).Role("button").Name("Host information").OnClick(a.toggleHostInfo))
	toolbarRight := desktopChrome.toolbarRight(w)
	root.Child(button("New terminal tab", toolbarRight-37, 9, 28, 27, a.newTab), button("Command palette", toolbarRight-72, 9, 29, 27, a.openPalette))
	if desktopChrome.windowControls {
		a.renderWindowControls(cx, root, w)
	}
}
