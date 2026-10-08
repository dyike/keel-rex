package backend

import "strings"

type SearchMatch struct {
	Scroll, Anchor, Caret, Position, Count int
	Selection                              Selection
}

// Search cell contents rather than display strings so wide Unicode and combining
// characters keep their correct terminal column positions.
func (s *session) search(query string, after int) *SearchMatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	if query == "" {
		return nil
	}
	first, history, epoch := s.emu.Scrollback().State()
	alt := s.emu.IsAltScreen()
	if s.emu.IsAltScreen() {
		first, history = 0, 0
	}
	needle := strings.ToLower(query)
	var matches []SearchMatch
	for row := 0; row < history+s.rows; row++ {
		var text strings.Builder
		var columns []int
		for col := 0; col < s.cols; col++ {
			cell := s.emu.CellAt(col, row-history)
			if row < history {
				cell = s.emu.Scrollback().AbsoluteCellAt(col, first+row)
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
			matches = append(matches, SearchMatch{Scroll: scroll, Anchor: screenRow*s.cols + columns[i], Caret: screenRow*s.cols + columns[end], Position: row*s.cols + columns[i], Selection: Selection{Anchor: (first+row)*s.cols + columns[i], Caret: (first+row)*s.cols + columns[end], Cols: s.cols, Epoch: epoch, Alt: alt}})
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
