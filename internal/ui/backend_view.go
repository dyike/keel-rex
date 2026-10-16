package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"sync/atomic"
	"time"
)

func (a *app) backendService() backend.WorkspaceService {
	if a.service == nil {
		a.service = backend.NewService(a.dataDir)
	}
	return a.service
}

// terminalView binds model notifications to the UI event loop. Coalescing and
// window invalidation belong here, rather than in the PTY or server code.
func (a *app) terminalView(s backend.Session, p *pane, focus bool) *terminal {
	changed := s.Subscribe()
	t := &terminal{session: s, owner: a, pane: p, initialFocus: focus}
	last := terminalNotice(s.State(), time.Now())
	go func() {
		defer s.Unsubscribe(changed)
		t.observe(changed, last, core.Update)
	}()
	return t
}

// Hidden terminals still update their model. Only changes to the tab's label,
// program icon, activity or attention need a frame; changing screen cells do not.
// Read visibility atomically here, without touching frame-locked app state.
type terminalNoticeState struct {
	directory, title, program string
	exited, busy              bool
	bells                     int
}

func terminalNotice(f backend.Frame, now time.Time) terminalNoticeState {
	return terminalNoticeState{f.Dir, f.Title, f.Program, f.Exited, now.Sub(f.LastOutput) < 600*time.Millisecond, f.Bells}
}

func (t *terminal) observe(changed <-chan struct{}, last terminalNoticeState, post func(func())) {
	var pending atomic.Bool
	invalidate := func() {
		next := terminalNotice(t.session.State(), time.Now())
		redraw := t.visible.Load() || next != last
		last = next
		if redraw && pending.CompareAndSwap(false, true) {
			post(func() { pending.Store(false) })
		}
	}
	for {
		select {
		case <-changed:
			invalidate()
		case <-t.session.Done():
			invalidate()
			return
		}
	}
}

// Called under the frame lock before layout. Hidden views keep their session,
// scroll and selection, but rebuild their screen and paint data when shown.
func (t *terminal) setVisible(visible bool) {
	if t.visible.Swap(visible) && !visible {
		t.releasePaint()
	}
}

func (t *terminal) releasePaint() {
	t.cells, t.rowVersions, t.rowPaints = nil, nil, nil
	t.plain = ""
	t.glyphRenderer.Release()
}

func (a *app) syncTerminalVisibility() {
	for i, tab := range a.tabs {
		tab.root.each(func(p *pane) {
			if p.term != nil {
				p.term.setVisible(!a.closed && i == a.active && (!tab.zoom || p == a.focused))
			}
		})
	}
}
