package main

import (
	"github.com/dyike/keel/ui/core"
	"strings"
)

type searchMatch struct{ Scroll, Anchor, Caret, Position, Count int }

// Search cell contents rather than display strings so wide Unicode and combining
// characters keep their correct terminal column positions.
func (s *session) search(query string, after int) *searchMatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	if query == "" {
		return nil
	}
	history := s.emu.ScrollbackLen()
	if s.emu.IsAltScreen() {
		history = 0
	}
	needle := strings.ToLower(query)
	var matches []searchMatch
	for row := 0; row < history+s.rows; row++ {
		var text strings.Builder
		var columns []int
		for col := 0; col < s.cols; col++ {
			cell := s.emu.CellAt(col, row-history)
			if row < history {
				cell = s.emu.ScrollbackCellAt(col, row)
			}
			if cell == nil || cell.Width == 0 {
				continue
			}
			value := strings.ToLower(cell.Content)
			if value == "" {
				value = " "
			}
			text.WriteString(value)
			for range []byte(value) {
				columns = append(columns, col)
			}
		}
		line := text.String()
		for start := 0; start < len(line); {
			i := strings.Index(line[start:], needle)
			if i < 0 {
				break
			}
			i += start
			end := i + len(needle) - 1
			if end >= len(columns) {
				break
			}
			scroll := max(0, history-row)
			screenRow := row - history + scroll
			matches = append(matches, searchMatch{Scroll: scroll, Anchor: screenRow*s.cols + columns[i], Caret: screenRow*s.cols + columns[end], Position: row*s.cols + columns[i]})
			start = end + 1
		}
	}
	if len(matches) == 0 {
		return nil
	}
	match := matches[0]
	for _, m := range matches {
		if m.Position > after {
			match = m
			break
		}
	}
	match.Count = len(matches)
	return &match
}
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
		var match *searchMatch
		var err error
		if t.session.remote != nil {
			r := t.session.remote
			response, e := callServer(r.dir, rpcRequest{Op: "find", ID: r.id, Text: query, After: after})
			match, err = response.Match, e
		} else {
			match = t.session.search(query, after)
		}
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
			t.scroll, t.anchor, t.caret = match.Scroll, match.Anchor, match.Caret
			t.hasSelection = true
			t.searchAfter = match.Position
			t.owner.notice = ""
			t.owner.closeOverlay()
		})
	}()
}
