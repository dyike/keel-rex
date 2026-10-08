package backend

import (
	"errors"
	uv "github.com/charmbracelet/ultraviolet"
	"strings"
)

// Endpoints are inclusive cells in the history and live screen, rather than
// offsets into whichever viewport happens to be visible.
type Selection struct {
	Anchor, Caret, Cols, Epoch int
	Alt                        bool
}

func (s *session) selectionText(sel Selection) (string, error) {
	if s.remote != nil {
		r := s.remote
		response, err := callServer(r.dir, rpcRequest{Op: "copy", ID: r.id, Selection: &sel})
		return response.Text, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.frameState(0)
	if sel.Cols != f.Cols || sel.Alt != f.Alt || sel.Epoch != f.HistoryEpoch {
		return "", errors.New("terminal changed; select the text again")
	}
	first, live := f.HistoryStart, f.HistoryStart+f.History
	if f.Alt {
		first, live = 0, 0
	}
	lo, hi := min(sel.Anchor, sel.Caret), max(sel.Anchor, sel.Caret)
	if lo < first*f.Cols || hi >= (live+f.Rows)*f.Cols {
		return "", errors.New("selected output is no longer available")
	}
	var out strings.Builder
	for y := lo / f.Cols; y <= hi/f.Cols; y++ {
		var row uv.Line
		if y < live {
			var retained bool
			row, retained = s.emu.Scrollback().AbsoluteLine(y)
			if !retained {
				return "", errors.New("selected output is no longer available")
			}
		}
		var line strings.Builder
		start, end := 0, f.Cols-1
		if y == lo/f.Cols {
			start = lo % f.Cols
		}
		if y == hi/f.Cols {
			end = hi % f.Cols
		}
		for x := start; x <= end; x++ {
			var c *uv.Cell
			if y >= live {
				c = s.emu.CellAt(x, y-live)
			} else if x < len(row) {
				c = &row[x]
			}
			if c == nil {
				line.WriteByte(' ')
			} else if c.Width > 0 {
				if c.Content == "" {
					line.WriteByte(' ')
				} else {
					line.WriteString(c.Content)
				}
			}
		}
		if y > lo/f.Cols {
			out.WriteByte('\n')
		}
		out.WriteString(strings.TrimRight(line.String(), " "))
	}
	return out.String(), nil
}
