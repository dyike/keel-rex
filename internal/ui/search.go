package ui

import (
	"github.com/dyike/keel/ui/core"
)

func (t *terminal) find(query string) {
	if query == "" || t.searching {
		return
	}
	if query != t.lastSearch {
		t.searchAfter = -1
		t.lastSearch = query
	}
	t.searching = true
	after := t.searchAfter
	go func() {
		match, err := t.session.Find(query, after)
		core.Update(func() {
			t.searching = false
			if err != nil {
				t.owner.notice = err.Error()
				return
			}
			if match == nil {
				t.owner.notice = "No matches in terminal or scrollback"
				return
			}
			t.scroll, t.anchor, t.caret = match.Scroll, match.Selection.Anchor, match.Selection.Caret
			t.hasSelection = true
			t.searchAfter = match.Position
			t.owner.notice = ""
			t.owner.closeOverlay()
		})
	}()
}
