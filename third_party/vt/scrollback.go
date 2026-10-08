package vt

import (
	"image/color"
	"sync"
	"unsafe"

	"github.com/charmbracelet/x/ansi"

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
//
// Packing runs beside the emulator: a line scrolled off the screen is handed
// over as it is (Adopt), waits in pending, and a goroutine packs it and
// returns its cells for the screen to reuse (Blank). Lines read the same
// whether packed yet or not. A Scrollback is safe for one writer and readers
// on other goroutines.
type Scrollback struct {
	mu       sync.Mutex
	settled  sync.Cond // signalled as pending shrinks
	ring     []packedLine
	head     int // index of the oldest line in ring
	n        int // packed lines held
	maxLines int
	bytes    int // about how much the packed lines hold
	maxBytes int // the most they may hold; 0 for no bound
	evicted  int // lines dropped from the front since the start, for the cache

	pending  []uv.Line // raw lines after the ring, oldest first
	free     []uv.Line // packed raw lines, for Blank
	packing  bool      // a goroutine is packing
	inflight int       // lines at the front of pending being packed
	skipped  int       // of those, how many were dropped meanwhile
	epoch    int       // bumped by Clear, so a pack in flight is dropped

	// styles holds the links of the lines' runs.
	styles styleTable

	// chunk is the block that the text of the next lines goes into; only the
	// packing goroutine touches it.
	chunk []byte

	// The last line unpacked, by absolute index, for CellAt walking a line
	// cell by cell.
	cached     int
	cachedLine uv.Line
}

// maxPending bounds the raw lines waiting to be packed; past it the writer
// waits, so a flood of output cannot hold its cells in memory.
const maxPending = 1024

// packBatch is how many lines wait before a goroutine starts packing them.
const packBatch = 64

// packedLine is a line of cells without their per-cell overhead.
type packedLine struct {
	text   string
	sizes  []uint8 // byte length of each cell's content; nil when every cell is 1 byte
	widths []uint8 // width of each cell; nil when every cell is 1 column wide
	runs   []styleRun
	extra  []styleEntry // styles not in the table
	cells  int
}

// styleRun starts a run of cells sharing a style and link, up to the next
// run. A style is its attributes and its colors, each a kind and a value,
// some 28 bytes where uv.Style and uv.Link take 90: colored output keeps a
// run per word. A style with a color of another type is kept whole in the
// line's extra styles, run.extra its index plus one.
type styleRun struct {
	start            uint32
	fg, bg, ul       uint32
	fgKind, bgKind   uint8
	ulKind           uint8
	attrs, underline uint8
	link             uint32 // index+1 in the scrollback's links; 0 for none
	extra            uint32
}

// styleEntry is a style and link, whole.
type styleEntry struct {
	style uv.Style
	link  uv.Link
}

// Color kinds of a styleRun.
const (
	colorNone = iota
	colorBasic
	colorIndexed
	colorTrue
	colorRGBA
	colorNRGBA
	colorAlpha16
)

// packColor returns the kind and value of c, or false for a color of
// another type.
func packColor(c color.Color) (uint8, uint32, bool) {
	switch c := c.(type) {
	case nil:
		return colorNone, 0, true
	case ansi.BasicColor:
		return colorBasic, uint32(c), true
	case ansi.IndexedColor:
		return colorIndexed, uint32(c), true
	case ansi.TrueColor:
		return colorTrue, uint32(c), true
	case color.RGBA:
		return colorRGBA, uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A), true
	case color.NRGBA:
		return colorNRGBA, uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A), true
	case color.Alpha16:
		return colorAlpha16, uint32(c.A), true
	}
	return 0, 0, false
}

func unpackColor(kind uint8, v uint32) color.Color {
	switch kind {
	case colorBasic:
		return ansi.BasicColor(v)
	case colorIndexed:
		return ansi.IndexedColor(v)
	case colorTrue:
		return ansi.TrueColor(v)
	case colorRGBA:
		return color.RGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
	case colorNRGBA:
		return color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
	case colorAlpha16:
		return color.Alpha16{uint16(v)}
	}
	return nil
}

// styleTable holds the links of a scrollback's runs, once each.
type styleTable struct {
	mu    sync.RWMutex
	links []uv.Link
	ids   map[uv.Link]uint32
}

func (t *styleTable) linkID(l uv.Link) uint32 {
	if l == (uv.Link{}) {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	id, ok := t.ids[l]
	if !ok {
		if t.ids == nil {
			t.ids = map[uv.Link]uint32{}
		}
		t.links = append(t.links, l)
		id = uint32(len(t.links))
		t.ids[l] = id
	}
	return id
}

func (t *styleTable) link(id uint32) uv.Link {
	if id == 0 {
		return uv.Link{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.links[id-1]
}

// addRun adds to p a run from cell start in e.
func (t *styleTable) addRun(p *packedLine, start int, e styleEntry) {
	r := styleRun{start: uint32(start), attrs: e.style.Attrs, underline: uint8(e.style.Underline)}
	var ok1, ok2, ok3 bool
	r.fgKind, r.fg, ok1 = packColor(e.style.Fg)
	r.bgKind, r.bg, ok2 = packColor(e.style.Bg)
	r.ulKind, r.ul, ok3 = packColor(e.style.UnderlineColor)
	if ok1 && ok2 && ok3 {
		r.link = t.linkID(e.link)
	} else {
		p.extra = append(p.extra, e)
		r.extra = uint32(len(p.extra))
	}
	p.runs = append(p.runs, r)
}

// style returns the style of run r of p.
func (t *styleTable) style(p *packedLine, r styleRun) styleEntry {
	if r.extra != 0 {
		return p.extra[r.extra-1]
	}
	return styleEntry{
		style: uv.Style{Fg: unpackColor(r.fgKind, r.fg), Bg: unpackColor(r.bgKind, r.bg), UnderlineColor: unpackColor(r.ulKind, r.ul), Attrs: r.attrs, Underline: uv.Underline(r.underline)},
		link:  t.link(r.link),
	}
}

// NewScrollback creates a new scrollback buffer with the given maximum number of lines.
func NewScrollback(maxLines int) *Scrollback {
	if maxLines <= 0 {
		maxLines = DefaultScrollbackSize
	}
	s := &Scrollback{maxLines: maxLines, cached: -1}
	s.settled.L = &s.mu
	return s
}

// Push adds a copy of line to the scrollback buffer.
// If the buffer is full, the oldest line is removed.
func (s *Scrollback) Push(line uv.Line) {
	if s == nil || s.maxLines <= 0 {
		return
	}
	cp := s.Blank(len(line))
	copy(cp, line)
	s.Adopt(cp)
}

// Adopt adds line itself to the scrollback buffer: the caller must not use it
// again. If the buffer is full, the oldest line is removed.
func (s *Scrollback) Adopt(line uv.Line) {
	if s == nil || s.maxLines <= 0 {
		return
	}
	s.mu.Lock()
	for len(s.pending) >= maxPending && s.packing {
		s.settled.Wait()
	}
	if s.n+len(s.pending) >= s.maxLines {
		if s.n > 0 {
			s.dropOldest()
		} else if len(s.pending) > 0 {
			s.pending = s.pending[1:]
			s.evicted++
			if s.skipped < s.inflight {
				s.skipped++ // it was in the batch being packed
			}
		}
	}
	s.pending = append(s.pending, line)
	// Start packing once a batch is waiting: a goroutine per line costs more
	// than packing it. Lines short of a batch read the same raw.
	if !s.packing && len(s.pending) >= packBatch {
		s.packing = true
		go s.work()
	}
	s.mu.Unlock()
}

// Blank returns a line of width cells for the screen to fill, reusing the
// cells of a packed line when one of that width is free. Its contents are
// whatever they were.
func (s *Scrollback) Blank(width int) uv.Line {
	if s != nil {
		s.mu.Lock()
		for len(s.free) > 0 {
			l := s.free[len(s.free)-1]
			s.free = s.free[:len(s.free)-1]
			if len(l) == width {
				s.mu.Unlock()
				return l
			}
		}
		s.mu.Unlock()
	}
	return make(uv.Line, width)
}

// work packs pending lines into the ring until none are left.
func (s *Scrollback) work() {
	var packed []packedLine
	for {
		s.mu.Lock()
		if len(s.pending) == 0 {
			s.packing = false
			s.settled.Broadcast()
			s.mu.Unlock()
			return
		}
		batch := append([]uv.Line(nil), s.pending[:min(len(s.pending), 64)]...)
		epoch := s.epoch
		s.inflight, s.skipped = len(batch), 0
		s.mu.Unlock()

		packed = packed[:0]
		for _, l := range batch {
			packed = append(packed, s.pack(l))
		}

		s.mu.Lock()
		if s.epoch == epoch {
			skip := s.skipped
			s.pending = append(s.pending[:0], s.pending[len(batch)-skip:]...)
			for _, p := range packed[skip:] {
				s.appendRing(p)
			}
			if len(s.free) < 64 {
				s.free = append(s.free, batch[:min(len(batch), 64-len(s.free))]...)
			}
		}
		s.inflight, s.skipped = 0, 0
		s.settled.Broadcast()
		s.mu.Unlock()
	}
}

// appendRing adds p after the newest packed line, dropping the oldest when the
// buffer is full.
func (s *Scrollback) appendRing(p packedLine) {
	if s.n+len(s.pending) >= s.maxLines && s.n > 0 {
		s.dropOldest()
	}
	s.bytes += p.size()
	defer func() {
		for s.maxBytes > 0 && s.bytes > s.maxBytes && s.n > 1 {
			s.dropOldest()
		}
	}()
	if s.n < s.maxLines {
		if len(s.ring) < s.maxLines {
			// Grow the ring as lines arrive rather than all at once. Until
			// it is full the lines run from head to the end without wrapping.
			if s.head+s.n != len(s.ring) {
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
	s.bytes -= s.ring[s.head].size()
	s.ring[s.head] = p
	s.head = (s.head + 1) % len(s.ring)
	s.evicted++
}

// dropOldest drops the oldest packed line.
func (s *Scrollback) dropOldest() {
	s.bytes -= s.ring[s.head].size()
	s.ring[s.head] = packedLine{}
	s.head = (s.head + 1) % len(s.ring)
	s.n--
	s.evicted++
}

// size is about how many bytes p holds.
func (p *packedLine) size() int {
	return 96 + len(p.text) + len(p.sizes) + len(p.widths) + 28*len(p.runs) + 96*len(p.extra)
}

// SetMaxBytes bounds the memory of the packed lines too, besides their
// number: the oldest go while they hold more than n bytes. 0, the default,
// sets no bound.
func (s *Scrollback) SetMaxBytes(n int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxBytes = n
	for s.maxBytes > 0 && s.bytes > s.maxBytes && s.n > 1 {
		s.dropOldest()
	}
}

// normalize rotates the ring so that the oldest line is first.
func (s *Scrollback) normalize() {
	lines := make([]packedLine, 0, max(s.n, min(s.maxLines, 1024)))
	for i := 0; i < s.n; i++ {
		lines = append(lines, s.ring[(s.head+i)%len(s.ring)])
	}
	s.ring, s.head = lines, 0
}

func (s *Scrollback) pack(line uv.Line) packedLine {
	// Trim trailing empty cells, as wrapping and resizing expect.
	last := len(line) - 1
	for last >= 0 && blankCell(&line[last]) {
		last--
	}
	line = line[:last+1]
	p := packedLine{cells: len(line)}
	if len(line) == 0 {
		return p
	}
	// Text goes into a shared chunk; a chunk is freed once every line in it
	// has left the ring, which happens in order.
	if cap(s.chunk)-len(s.chunk) < len(line)*4 {
		s.chunk = make([]byte, 0, max(chunkSize, len(line)*8))
	}
	start := len(s.chunk)
	buf := s.chunk
	prev := &line[0]
	if !plainStyle(&prev.Style) || prev.Link != (uv.Link{}) {
		s.styles.addRun(&p, 0, styleEntry{prev.Style, prev.Link})
	}
	for i := range line {
		c := &line[i]
		if len(c.Content) == 1 && c.Width == 1 && p.sizes == nil {
			buf = append(buf, c.Content[0])
		} else {
			if p.sizes == nil {
				// The first cell that is not one byte wide: from here on
				// every cell keeps its size and width.
				p.sizes = make([]uint8, len(line))
				p.widths = make([]uint8, len(line))
				for k := 0; k < i; k++ {
					p.sizes[k], p.widths[k] = 1, 1
				}
			}
			content := c.Content
			if len(content) > 255 {
				content = content[:255] // a cluster this long is not a real one
			}
			p.sizes[i] = uint8(len(content))
			p.widths[i] = uint8(min(c.Width, 255))
			buf = append(buf, content...)
		}
		if i > 0 && (!sameStyle(&c.Style, &prev.Style) || !sameLink(&c.Link, &prev.Link)) {
			s.styles.addRun(&p, i, styleEntry{c.Style, c.Link})
		}
		prev = c
	}
	if cap(buf) != cap(s.chunk) { // outgrew the chunk: the line has a copy of its own
		s.chunk = s.chunk[:0:0]
		p.text = unsafe.String(unsafe.SliceData(buf[start:]), len(buf)-start)
		return p
	}
	s.chunk = buf
	p.text = unsafe.String(unsafe.SliceData(buf[start:]), len(buf)-start)
	return p
}

// trimmed returns line without its trailing empty cells, as pack keeps it.
func trimmed(line uv.Line) uv.Line {
	last := len(line) - 1
	for last >= 0 && blankCell(&line[last]) {
		last--
	}
	return line[:last+1]
}

// chunkSize is the size of the blocks that hold the text of kept lines.
const chunkSize = 64 << 10

// blankCell reports whether c is a zero cell or a plain space, as trailing
// cells are trimmed when a line is kept.
func blankCell(c *uv.Cell) bool {
	if c.Content == " " {
		if c.Width != 1 {
			return false
		}
	} else if c.Content != "" || c.Width != 0 {
		return false
	}
	return plainStyle(&c.Style) && len(c.Link.URL) == 0 && len(c.Link.Params) == 0
}

// sameLink is a == b, without comparing strings when both are empty.
func sameLink(a, b *uv.Link) bool {
	if len(a.URL)|len(a.Params)|len(b.URL)|len(b.Params) == 0 {
		return true
	}
	return *a == *b
}

// unpack appends the cells of p to dst.
func (p *packedLine) unpack(dst uv.Line, styles *styleTable) uv.Line {
	off, run := 0, 0 // cells before the first run have no style
	var style uv.Style
	var link uv.Link
	for i := 0; i < p.cells; i++ {
		if run < len(p.runs) && int(p.runs[run].start) == i {
			e := styles.style(p, p.runs[run])
			style, link = e.style, e.link
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

// direct locks the scrollback for lines that go into it straight from text,
// after packing the lines handed over before, and returns the function that
// unlocks it. Between the two, appendText and appendCells add lines.
func (s *Scrollback) direct() (done func()) {
	s.mu.Lock()
	for s.packing {
		s.settled.Wait()
	}
	for len(s.pending) > 0 {
		l := s.pending[0]
		s.pending = s.pending[1:]
		s.appendRing(s.pack(l))
		if len(s.free) < 64 {
			s.free = append(s.free, l)
		}
	}
	s.pending = s.pending[:0]
	return s.mu.Unlock
}

// appendText adds a line of printable ASCII, a cell per byte in style and
// link, packed straight from its bytes, which are copied. Called between
// direct and its done.
func (s *Scrollback) appendText(text []byte, style uv.Style, link uv.Link) {
	if plainStyle(&style) && len(link.URL)|len(link.Params) == 0 {
		for len(text) > 0 && text[len(text)-1] == ' ' { // a plain space is an empty cell
			text = text[:len(text)-1]
		}
	}
	p := packedLine{cells: len(text)}
	if len(text) > 0 {
		if cap(s.chunk)-len(s.chunk) < len(text) {
			s.chunk = make([]byte, 0, max(chunkSize, len(text)))
		}
		start := len(s.chunk)
		s.chunk = append(s.chunk, text...)
		p.text = unsafe.String(&s.chunk[start], len(text))
		if !plainStyle(&style) || len(link.URL)|len(link.Params) != 0 {
			s.styles.addRun(&p, 0, styleEntry{style, link})
		}
	}
	s.appendRing(p)
}

// appendCells adds a line of cells, packed. Called between direct and its
// done.
func (s *Scrollback) appendCells(line uv.Line) {
	s.appendRing(s.pack(line))
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n + len(s.pending)
}

// State identifies retained lines independently of eviction and clearing.
func (s *Scrollback) State() (first, length, epoch int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evicted, s.n + len(s.pending), s.epoch
}

// AbsoluteLine returns a copy of a retained line by its stable row number.
func (s *Scrollback) AbsoluteLine(row int) (uv.Line, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := row - s.evicted
	if index < 0 || index >= s.n+len(s.pending) {
		return nil, false
	}
	return s.line(index), true
}

// MaxLines returns the maximum number of lines the scrollback buffer can hold.
func (s *Scrollback) MaxLines() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxLines
}

// SetMaxLines sets the maximum number of lines for the scrollback buffer.
// If the new limit is smaller than the current number of lines,
// the oldest lines are removed.
func (s *Scrollback) SetMaxLines(maxLines int) {
	if s == nil || maxLines <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.packing {
		s.settled.Wait()
	}
	s.normalize()
	if s.n > maxLines {
		for _, p := range s.ring[:s.n-maxLines] {
			s.bytes -= p.size()
		}
		s.ring = append([]packedLine(nil), s.ring[s.n-maxLines:]...)
		s.evicted += s.n - maxLines
		s.n = maxLines
	}
	s.maxLines = maxLines
}

// Line returns the line at the given index.
// Index 0 is the oldest line, Len()-1 is the most recent.
// Returns nil if index is out of bounds.
func (s *Scrollback) Line(index int) uv.Line {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.line(index)
}

func (s *Scrollback) line(index int) uv.Line {
	switch {
	case index < 0 || index >= s.n+len(s.pending):
		return nil
	case index >= s.n:
		return append(uv.Line(nil), trimmed(s.pending[index-s.n])...)
	}
	p := s.at(index)
	return p.unpack(make(uv.Line, 0, p.cells), &s.styles)
}

// Lines returns all lines in the scrollback buffer.
// Index 0 is the oldest line.
func (s *Scrollback) Lines() []uv.Line {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := make([]uv.Line, s.n+len(s.pending))
	for i := range lines {
		lines[i] = s.line(i)
	}
	return lines
}

// Clear removes all lines from the scrollback buffer.
func (s *Scrollback) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evicted += s.n + len(s.pending)
	s.ring, s.head, s.n, s.pending, s.bytes = nil, 0, 0, nil, 0
	s.styles.mu.Lock()
	s.styles.links, s.styles.ids = nil, nil
	s.styles.mu.Unlock()
	s.epoch++
	s.cached = -1
}

// CellAt returns the cell at the given position in the scrollback buffer.
// x is the column, y is the line index (0 = oldest).
// Returns nil if position is out of bounds. The cell is valid until the next
// call that reads the scrollback.
func (s *Scrollback) CellAt(x, y int) *uv.Cell {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cellAt(x, y)
}

// AbsoluteCellAt reads a stable row even if packing evicts older lines.
// The cell is valid until the next call that reads the scrollback.
func (s *Scrollback) AbsoluteCellAt(x, row int) *uv.Cell {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cellAt(x, row-s.evicted)
}

func (s *Scrollback) cellAt(x, y int) *uv.Cell {
	if y < 0 || y >= s.n+len(s.pending) || x < 0 {
		return nil
	}
	if abs := s.evicted + y; s.cached != abs {
		if y >= s.n { // not packed yet: read as if it were
			s.cachedLine = append(s.cachedLine[:0], trimmed(s.pending[y-s.n])...)
		} else {
			s.cachedLine = s.at(y).unpack(s.cachedLine[:0], &s.styles)
		}
		s.cached = abs
	}
	if x >= len(s.cachedLine) {
		return nil
	}
	return &s.cachedLine[x]
}

// plainStyle reports whether st is the zero style. Styles hold colors as
// interfaces, which == may not compare.
func plainStyle(st *uv.Style) bool {
	return st.Fg == nil && st.Bg == nil && st.UnderlineColor == nil && st.Underline == 0 && st.Attrs == 0
}

// sameStyle is uv.Style.Equal, without converting colors to compare them
// when neither cell has one, as for most text.
func sameStyle(a, b *uv.Style) bool {
	if a.Attrs != b.Attrs || a.Underline != b.Underline {
		return false
	}
	if a.Fg == nil && b.Fg == nil && a.Bg == nil && b.Bg == nil && a.UnderlineColor == nil && b.UnderlineColor == nil {
		return true
	}
	return a.Equal(b)
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}
