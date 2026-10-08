package ui

import (
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"sync/atomic"
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
	go func() {
		defer s.Unsubscribe(changed)
		var pending atomic.Bool
		invalidate := func() {
			if pending.CompareAndSwap(false, true) {
				core.Update(func() { pending.Store(false) })
			}
		}
		for {
			select {
			case <-changed:
				invalidate()
			case <-s.Done():
				invalidate()
				return
			}
		}
	}()
	return &terminal{session: s, owner: a, pane: p, initialFocus: focus}
}
