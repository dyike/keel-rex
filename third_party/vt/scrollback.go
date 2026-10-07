package vt

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// DefaultScrollbackSize is the default maximum number of lines in the scrollback buffer.
const DefaultScrollbackSize = 10000

// Scrollback represents a scrollback buffer that stores lines scrolled off the screen.
//
// keel-rex: lines are kept packed in a ring, not as []uv.Cell. A cell is about
// 120 bytes, so 10000 lines of 127 columns took 150 MB, and pushing a line
// cloned it and shifted every line before it. A packed line is its text, the
// cell boundaries only when a cell is not one byte wide, and one entry per
// run of cells that share a style, a few hundred bytes for most lines.
type Scrollback struct {
	ring     []packedLine
	head     int // index of the oldest line in ring
	n        int // lines held
	maxLines int

	// The last line unpacked, for CellAt walking a line cell by cell.
	cached     int
	cachedLine uv.Line
}

// packedLine is a line of cells without their per-cell overhead.
type packedLine struct {
	text   string
	sizes  []uint8 // byte length of each cell's content; nil when every cell is 1 byte
	widths []uint8 // width of each cell; nil when every cell is 1 column wide
	runs   []styleRun
	cells  int
}

// styleRun starts a run of cells sharing a style and link, up to the next run.
type styleRun struct {
	start int
	style uv.Style
	link  uv.Link
}

// NewScrollback creates a new scrollback buffer with the given maximum number of lines.
func NewScrollback(maxLines int) *Scrollback {
	if maxLines <= 0 {
		maxLines = DefaultScrollbackSize
	}
	return &Scrollback{maxLines: maxLines, cached: -1}
}

// Push adds a line to the scrollback buffer.
// If the buffer is full, the oldest line is removed.
func (s *Scrollback) Push(line uv.Line) {
	if s == nil || s.maxLines <= 0 {
		return
	}
	s.cached = -1
	p := pack(line)
	if s.n < s.maxLines {
		if len(s.ring) < s.maxLines {
			// Grow the ring as lines arrive rather than all at once.
			if s.head != 0 || s.n != len(s.ring) {
				s.normalize()
			}
			s.ring = append(s.ring, p)
			s.n++
			return
		}
		s.ring[(s.head+s.n)%len(s.ring)] = p
		s.n++
		return
	}
	s.ring[s.head] = p
	s.head = (s.head + 1) % len(s.ring)
}

// normalize rotates the ring so that the oldest line is first.
func (s *Scrollback) normalize() {
	lines := make([]packedLine, 0, max(s.n, min(s.maxLines, 1024)))
	for i := 0; i < s.n; i++ {
		lines = append(lines, s.ring[(s.head+i)%len(s.ring)])
	}
	s.ring, s.head = lines, 0
}

func pack(line uv.Line) packedLine {
	// Trim trailing empty cells, as wrapping and resizing expect.
	last := -1
	for i := len(line) - 1; i >= 0; i-- {
		c := &line[i]
		if !c.IsZero() && !c.Equal(&uv.EmptyCell) {
			last = i
			break
		}
	}
	line = line[:last+1]
	p := packedLine{cells: len(line)}
	if len(line) == 0 {
		return p
	}
	size := 0
	simpleSizes, simpleWidths := true, true
	for i := range line {
		c := &line[i]
		size += len(c.Content)
		if len(c.Content) != 1 {
			simpleSizes = false
		}
		if c.Width != 1 {
			simpleWidths = false
		}
	}
	var b strings.Builder
	b.Grow(size)
	if !simpleSizes {
		p.sizes = make([]uint8, len(line))
	}
	if !simpleWidths {
		p.widths = make([]uint8, len(line))
	}
	for i := range line {
		c := &line[i]
		content := c.Content
		if len(content) > 255 {
			content = content[:255] // a cluster this long is not a real one
		}
		b.WriteString(content)
		if p.sizes != nil {
			p.sizes[i] = uint8(len(content))
		}
		if p.widths != nil {
			p.widths[i] = uint8(min(c.Width, 255))
		}
		if i == 0 || !c.Style.Equal(&line[i-1].Style) || c.Link != line[i-1].Link {
			p.runs = append(p.runs, styleRun{start: i, style: c.Style, link: c.Link})
		}
	}
	p.text = b.String()
	if len(p.runs) == 1 && plainStyle(&p.runs[0].style) && p.runs[0].link == (uv.Link{}) {
		p.runs = nil
	}
	return p
}

// unpack appends the cells of p to dst.
func (p *packedLine) unpack(dst uv.Line) uv.Line {
	off, run := 0, 0
	var style uv.Style
	var link uv.Link
	for i := 0; i < p.cells; i++ {
		if run < len(p.runs) && p.runs[run].start == i {
			style, link = p.runs[run].style, p.runs[run].link
			run++
		}
		n, w := 1, 1
		if p.sizes != nil {
			n = int(p.sizes[i])
		}
		if p.widths != nil {
			w = int(p.widths[i])
		}
		dst = append(dst, uv.Cell{Content: p.text[off : off+n], Width: w, Style: style, Link: link})
		off += n
	}
	return dst
}

func (s *Scrollback) at(index int) *packedLine {
	return &s.ring[(s.head+index)%len(s.ring)]
}

// PushN adds n lines from the buffer starting at line y to the scrollback.
func (s *Scrollback) PushN(buf *uv.RenderBuffer, y, n int) {
	if s == nil || buf == nil || n <= 0 {
		return
	}

	for i := range min(n, buf.Height()-y) {
		if line := buf.Line(y + i); line != nil {
			s.Push(line)
		}
	}
}

// Len returns the number of lines in the scrollback buffer.
func (s *Scrollback) Len() int {
	if s == nil {
		return 0
	}
	return s.n
}

// MaxLines returns the maximum number of lines the scrollback buffer can hold.
func (s *Scrollback) MaxLines() int {
	if s == nil {
		return 0
	}
	return s.maxLines
}

// SetMaxLines sets the maximum number of lines for the scrollback buffer.
// If the new limit is smaller than the current number of lines,
// the oldest lines are removed.
func (s *Scrollback) SetMaxLines(maxLines int) {
	if s == nil || maxLines <= 0 {
		return
	}
	s.normalize()
	if s.n > maxLines {
		s.ring = append([]packedLine(nil), s.ring[s.n-maxLines:]...)
		s.n = maxLines
	}
	s.maxLines = maxLines
	s.cached = -1
}

// Line returns the line at the given index.
// Index 0 is the oldest line, Len()-1 is the most recent.
// Returns nil if index is out of bounds.
func (s *Scrollback) Line(index int) uv.Line {
	if s == nil || index < 0 || index >= s.n {
		return nil
	}
	p := s.at(index)
	if p.cells == 0 {
		return uv.Line{}
	}
	return p.unpack(make(uv.Line, 0, p.cells))
}

// Lines returns all lines in the scrollback buffer.
// Index 0 is the oldest line.
func (s *Scrollback) Lines() []uv.Line {
	if s == nil {
		return nil
	}
	lines := make([]uv.Line, s.n)
	for i := range lines {
		lines[i] = s.Line(i)
	}
	return lines
}

// Clear removes all lines from the scrollback buffer.
func (s *Scrollback) Clear() {
	if s == nil {
		return
	}
	s.ring, s.head, s.n, s.cached = nil, 0, 0, -1
}

// CellAt returns the cell at the given position in the scrollback buffer.
// x is the column, y is the line index (0 = oldest).
// Returns nil if position is out of bounds. The cell is valid until the next
// call that changes the scrollback or reads another line.
func (s *Scrollback) CellAt(x, y int) *uv.Cell {
	if s == nil || y < 0 || y >= s.n {
		return nil
	}
	if s.cached != y {
		s.cachedLine = s.at(y).unpack(s.cachedLine[:0])
		s.cached = y
	}
	if x < 0 || x >= len(s.cachedLine) {
		return nil
	}
	return &s.cachedLine[x]
}

// plainStyle reports whether st is the zero style. Styles hold colors as
// interfaces, which == may not compare.
func plainStyle(st *uv.Style) bool {
	return st.Fg == nil && st.Bg == nil && st.UnderlineColor == nil && st.Underline == 0 && st.Attrs == 0
}
