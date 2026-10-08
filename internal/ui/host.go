package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"os"
)

func (a *app) loadHostInfo() {
	a.hostModel = desktopChrome.systemName
	a.hostDetails = backend.HostDetails{Model: "Reading…", Chip: "Reading…", Memory: "Reading…", System: desktopChrome.systemName, User: os.Getenv("USER")}
	go func() {
		details := (backend.HostInspector{}).Details()
		model := details.Model
		if model == "" || model == "Unavailable" {
			model = desktopChrome.systemName
		}
		core.Update(func() { a.hostDetails, a.hostModel = details, model })
	}()
}

func (a *app) refreshHostStatus() {
	if a.hostStatusLoading || a.closed {
		return
	}
	a.hostStatusLoading = true
	var sessions []backend.Session
	seen := map[backend.Session]bool{}
	for _, tab := range a.tabs {
		tab.root.each(func(p *pane) {
			if p.term != nil && !seen[p.term.session] {
				seen[p.term.session] = true
				sessions = append(sessions, p.term.session)
			}
		})
	}
	service, started := a.backendService(), a.started
	go func() {
		status := service.Status(sessions, started)
		core.Update(func() {
			a.hostStatusLoading = false
			if !a.closed {
				a.hostStatus = status
			}
		})
	}()
}
func (a *app) toggleHostInfo() {
	a.hostOpen = !a.hostOpen
	if a.hostOpen {
		a.refreshHostStatus()
	}
}
