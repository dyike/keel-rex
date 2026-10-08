package backend

import (
	uv "github.com/charmbracelet/ultraviolet"
	"image"
	"image/color"
	"strings"
	"time"
)

type WireCell struct {
	Text      string       `json:"t,omitempty"`
	Width     int          `json:"w,omitempty"`
	FG, BG    *color.NRGBA `json:",omitempty"`
	Attrs     uint8        `json:"a,omitempty"`
	Underline uv.Underline `json:"u,omitempty"`
	Link      string       `json:"l,omitempty"`
}
type Frame struct {
	Cells                             []WireCell `json:"cells,omitempty"`
	Cols, Rows, History, Scroll       int
	HistoryStart, HistoryEpoch        int
	Cursor                            image.Point
	CursorVisible, Mouse, Alt, Exited bool
	Dir, Title, Program               string
	PID, Bells                        int
	Revision                          uint64
	LastOutput                        time.Time
	Plain                             string
}

func encodeCell(c *uv.Cell) WireCell {
	if c == nil {
		return WireCell{Text: " ", Width: 1}
	}
	w := WireCell{Text: c.Content, Width: c.Width, Attrs: c.Style.Attrs, Underline: c.Style.Underline, Link: c.Link.URL}
	if c.Style.Fg != nil {
		v := color.NRGBAModel.Convert(c.Style.Fg).(color.NRGBA)
		w.FG = &v
	}
	if c.Style.Bg != nil {
		v := color.NRGBAModel.Convert(c.Style.Bg).(color.NRGBA)
		w.BG = &v
	}
	return w
}
func (w WireCell) cell() uv.Cell {
	c := uv.Cell{Content: w.Text, Width: w.Width, Style: uv.Style{Attrs: w.Attrs, Underline: w.Underline}, Link: uv.Link{URL: w.Link}}
	if w.FG != nil {
		c.Style.Fg = *w.FG
	}
	if w.BG != nil {
		c.Style.Bg = *w.BG
	}
	return c
}
func (s *session) snapshot(scroll int, since uint64) Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remote != nil {
		return s.frame
	}
	f := s.frameState(scroll)
	if since == s.revision {
		return f
	}
	f.Cells = make([]WireCell, s.cols*s.rows)
	var plain strings.Builder
	for y := 0; y < s.rows; y++ {
		var line strings.Builder
		for x := 0; x < s.cols; x++ {
			c := s.cellAt(x, y, f)
			f.Cells[y*s.cols+x] = encodeCell(c)
			if c != nil && c.Width > 0 {
				line.WriteString(c.Content)
			}
		}
		plain.WriteString(strings.TrimRight(line.String(), " "))
		plain.WriteByte('\n')
	}
	f.Plain = plain.String()
	return f
}

// frameState is the state of the screen at scroll lines back, without its
// cells. It is called with s.mu held.
func (s *session) frameState(scroll int) Frame {
	first, history, epoch := s.emu.Scrollback().State()
	scroll = min(max(0, scroll), history)
	if s.emu.IsAltScreen() {
		scroll = 0
	}
	return Frame{Cols: s.cols, Rows: s.rows, History: history, Scroll: scroll, HistoryStart: first, HistoryEpoch: epoch, Cursor: s.emu.CursorPosition(), CursorVisible: s.cursorVisible, Mouse: s.mouseTracking, Alt: s.emu.IsAltScreen(), Exited: s.exited, Dir: s.cwd, Title: s.title, Program: s.program, Revision: s.revision, LastOutput: s.lastOutput, Bells: s.bells, PID: s.cmd.Process.Pid}
}

// cellAt returns the cell at x, y of the screen f shows, from the history
// when f is scrolled back. It is called with s.mu held.
func (s *session) cellAt(x, y int, f Frame) *uv.Cell {
	if row := y - f.Scroll; row < 0 {
		return s.emu.Scrollback().AbsoluteCellAt(x, f.HistoryStart+f.History+row)
	} else {
		return s.emu.CellAt(x, row)
	}
}
func (s *session) mouse(kind string, m uv.Mouse) {
	if s.remote != nil {
		s.remote.enqueue(rpcRequest{Op: "mouse", Mouse: m, Kind: kind})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.exited {
		return
	}
	switch kind {
	case "press":
		s.emu.SendMouse(uv.MouseClickEvent(m))
	case "release":
		s.emu.SendMouse(uv.MouseReleaseEvent(m))
	case "drag":
		s.emu.SendMouse(uv.MouseMotionEvent(m))
	case "scroll":
		s.emu.SendMouse(uv.MouseWheelEvent(m))
	}
}
func (s *session) clearScreen() {
	if s.remote != nil {
		s.remote.enqueue(rpcRequest{Op: "clear"})
		return
	}
	s.mu.Lock()
	s.emu.WriteString("\x1b[3J\x1b[2J\x1b[H")
	s.revision++
	s.mu.Unlock()
	s.notify()
}

func (s *session) syncAppearance(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.appearance == value {
		return
	}
	s.appearance = value
	if s.remote != nil {
		s.remote.enqueue(rpcRequest{Op: "theme", Kind: value})
		return
	}
	if s.closed || s.exited {
		return
	}
	fg, bg := rgb(0x272d30), rgb(0xf4f4f1)
	if value == "dark" {
		fg, bg = rgb(0xe0e4e8), rgb(0x24272a)
	}
	s.emu.SetDefaultForegroundColor(fg)
	s.emu.SetDefaultBackgroundColor(bg)
}
