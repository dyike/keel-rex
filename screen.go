package main

import (
	uv "github.com/charmbracelet/ultraviolet"
	"image"
	"image/color"
	"strings"
	"time"
)

type wireCell struct {
	Text      string       `json:"t,omitempty"`
	Width     int          `json:"w,omitempty"`
	FG, BG    *color.NRGBA `json:",omitempty"`
	Attrs     uint8        `json:"a,omitempty"`
	Underline uv.Underline `json:"u,omitempty"`
	Link      string       `json:"l,omitempty"`
}
type screenFrame struct {
	Cells                             []wireCell `json:"cells,omitempty"`
	Cols, Rows, History, Scroll       int
	Cursor                            image.Point
	CursorVisible, Mouse, Alt, Exited bool
	Dir, Title, Program               string
	PID, Bells                        int
	Revision                          uint64
	LastOutput                        time.Time
	Plain                             string
}

func encodeCell(c *uv.Cell) wireCell {
	if c == nil {
		return wireCell{Text: " ", Width: 1}
	}
	w := wireCell{Text: c.Content, Width: c.Width, Attrs: c.Style.Attrs, Underline: c.Style.Underline, Link: c.Link.URL}
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
func (w wireCell) cell() uv.Cell {
	c := uv.Cell{Content: w.Text, Width: w.Width, Style: uv.Style{Attrs: w.Attrs, Underline: w.Underline}, Link: uv.Link{URL: w.Link}}
	if w.FG != nil {
		c.Style.Fg = *w.FG
	}
	if w.BG != nil {
		c.Style.Bg = *w.BG
	}
	return c
}
func (s *session) snapshot(scroll int, since uint64) screenFrame {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remote != nil {
		return s.frame
	}
	history := s.emu.ScrollbackLen()
	scroll = min(max(0, scroll), history)
	if s.emu.IsAltScreen() {
		scroll = 0
	}
	f := screenFrame{Cols: s.cols, Rows: s.rows, History: history, Scroll: scroll, Cursor: s.emu.CursorPosition(), CursorVisible: s.cursorVisible, Mouse: s.mouseTracking, Alt: s.emu.IsAltScreen(), Exited: s.exited, Dir: s.cwd, Title: s.title, Program: s.program, Revision: s.revision, LastOutput: s.lastOutput, Bells: s.bells, PID: s.cmd.Process.Pid}
	if since == s.revision {
		return f
	}
	f.Cells = make([]wireCell, s.cols*s.rows)
	var plain strings.Builder
	for y := 0; y < s.rows; y++ {
		var line strings.Builder
		for x := 0; x < s.cols; x++ {
			row := y - scroll
			var c *uv.Cell
			if row < 0 {
				c = s.emu.ScrollbackCellAt(x, history+row)
			} else {
				c = s.emu.CellAt(x, row)
			}
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
	fg, bg := ink, rgb(0xf4f4f1)
	if value == "dark" {
		fg, bg = rgb(0xe0e4e8), rgb(0x24272a)
	}
	s.emu.SetDefaultForegroundColor(fg)
	s.emu.SetDefaultBackgroundColor(bg)
}
