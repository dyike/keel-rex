package ui

import (
	"fmt"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"image"
)

func (a *app) hostPanel(width float32) *el.DivEl {
	c := a.colors()
	details, status := a.hostDetails, a.hostStatus
	background := c.panel
	if a.prefs.Appearance != "dark" && !(a.prefs.Appearance == "system" && a.appearanceDark) {
		background = rgb(0xfcfcfe)
	}
	state, dot := desktopChrome.hostLabel+" · connecting…", c.muted
	if status.Loaded {
		state = desktopChrome.hostLabel + " · disconnected"
		dot = red
		if status.Connected {
			state, dot = desktopChrome.hostLabel+" · connected", green
			if status.Local {
				state = desktopChrome.hostLabel + " · local sessions"
			}
		}
	}
	icon := el.Div().Size(el.Dp(38)).Rounded(9).Bg(c.hover).Center().Child(el.Widget(core.Func(func(gtx core.C) core.D {
		p := painter{gtx, gtx.Metric.PxPerDp}
		p.glyph("device", 0, 0, 24, c.text)
		return core.D{Size: image.Pt(gtx.Dp(24), gtx.Dp(24))}
	})).Size(el.Dp(24)))
	header := el.Div().Row().Items(el.Center).Gap(10).Child(icon,
		el.Div().Grow().W(el.Dp(0)).Gap(2).Child(
			el.Text(hostname()).Bold().TextSize(14).MaxLines(1),
			el.Div().Row().Items(el.Center).Gap(6).Child(el.Div().Size(el.Dp(7)).Rounded(4).Bg(dot).NoShrink(), el.Text(state).TextSize(12).TextColor(c.muted).MaxLines(1).Grow().W(el.Dp(0)))))
	divider := func() el.Element { return el.Div().H(el.Dp(1)).Bg(c.hover).My(1) }
	row := func(label, value string) el.Element {
		if value == "" {
			value = "Unavailable"
		}
		return el.Div().Row().Items(el.Start).Gap(4).Child(el.Text(label).W(el.Dp(86)).NoShrink().TextSize(12).TextColor(c.muted), el.Text(value).TextSize(12).Grow().W(el.Dp(0)))
	}
	hardware := el.Div().Gap(6).Child(row("Model", details.Model), row("Chip", details.Chip), row("Memory", details.Memory), row("System", details.System), row("User", details.User))
	server, sessions := "Connecting…", "Reading…"
	note := "Sessions live in the server: quit Rex Keel and they keep running, and come back as they were when it opens again."
	if status.Loaded {
		server, sessions = "Disconnected", "Unavailable"
		if status.Connected {
			server = fmt.Sprintf("pid %d", status.PID)
			if status.Uptime != "" {
				server += " · up " + status.Uptime
			}
			sessions = fmt.Sprintf("%d open, %d running a program", status.Open, status.Running)
			if status.WindowOnly {
				sessions = fmt.Sprintf("%d in this window, %d running", status.Open, status.Running)
				note += " The existing server reports counts for this window only; total counts become available after the server is next started."
			}
			if status.Local {
				server = "In this app · " + server
				note = "These sessions run inside Rex Keel. Quitting the app ends them; use the normal app launch for persistent sessions."
			}
		} else {
			note = "The session server is unreachable. Existing sessions may still be running; connection status refreshes while this panel is open."
		}
	}
	return el.Div().W(el.Dp(width)).Bg(background).TextColor(c.text).Border(1, c.hover).Rounded(14).Shadow(theme.ElevationMd).P(14).Gap(12).Child(header, divider(), hardware, divider(), el.Div().Gap(6).Child(row("Server", server), row("Sessions", sessions)), el.Text(note).TextSize(11).TextColor(c.muted))
}
