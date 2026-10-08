package ui

import (
	"fmt"
	"github.com/dyike/keel/native/notification"
	"github.com/dyike/keel/ui/core"
	"os"
)

func (a *app) enableNotifications() {
	if os.Getenv("KEEL_HEADLESS") == "1" {
		a.notice = "System notifications require a running app bundle"
		return
	}
	notification.RequestPermission(func(err error) {
		core.Update(func() {
			if a.closed {
				return
			}
			if err != nil {
				a.notice = "Notifications unavailable: " + err.Error()
				return
			}
			a.prefs.Notifications = true
			a.savePreferences()
		})
	})
}
func (a *app) notifyAttention(tab *workspace, p *pane, reason string) {
	if !a.prefs.Notifications || a.window == nil || os.Getenv("KEEL_HEADLESS") == "1" {
		return
	}
	notification.Post(notification.Message{ID: fmt.Sprintf("pane-%d", p.id), Title: a.tabLabel(tab), Body: reason, OnClick: func() {
		core.Update(func() {
			if a.closed {
				return
			}
			for i, t := range a.tabs {
				if t == tab && containsPane(t.root, p) {
					a.activate(i)
					a.focusPane(p)
					a.window.Raise()
					return
				}
			}
		})
	}}, nil)
}
