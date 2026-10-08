package backend

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/fnv"
	"image/color"
	"io"

	uv "github.com/charmbracelet/ultraviolet"
)

// A window watches a session over one connection that stays open: it sends
// the watch request, then scroll positions as they change, and the server
// sends updates whenever the screen changes. An update is the screen's state
// (Frame without cells) and the rows that changed since the last one,
// each row its cells in a compact binary form, without the empty cells at its
// end. The first update has every row.
//
// On the wire an update is a 4-byte big-endian length, then a uvarint length
// and the JSON of the state, then a uvarint count of rows and, for each, its
// uvarint index, uvarint length and cells.

// maxUpdate bounds an update a client will read.
const maxUpdate = 64 << 20

// watchState is what the server sent a watcher last.
type watchState struct {
	meta     []byte
	hashes   []uint64
	cols     int
	scroll   int
	alt      bool
	revision uint64
	started  bool
}

// cell flags of the wire form.
const (
	wireFG = 1 << iota
	wireBG
	wireLink
)

// appendCell appends the wire form of c.
func appendCell(b []byte, c *uv.Cell) []byte {
	var flags byte
	if c.Style.Fg != nil {
		flags |= wireFG
	}
	if c.Style.Bg != nil {
		flags |= wireBG
	}
	if c.Link.URL != "" {
		flags |= wireLink
	}
	b = append(b, flags, byte(min(c.Width, 255)), c.Style.Attrs, byte(c.Style.Underline))
	b = binary.AppendUvarint(b, uint64(len(c.Content)))
	b = append(b, c.Content...)
	if flags&wireFG != 0 {
		v := color.NRGBAModel.Convert(c.Style.Fg).(color.NRGBA)
		b = append(b, v.R, v.G, v.B, v.A)
	}
	if flags&wireBG != 0 {
		v := color.NRGBAModel.Convert(c.Style.Bg).(color.NRGBA)
		b = append(b, v.R, v.G, v.B, v.A)
	}
	if flags&wireLink != 0 {
		b = binary.AppendUvarint(b, uint64(len(c.Link.URL)))
		b = append(b, c.Link.URL...)
	}
	return b
}

// emptyWireCell reports whether c is what a row is padded with.
func emptyWireCell(c *uv.Cell) bool {
	return c == nil || c.Content == " " && c.Width == 1 && c.Style.Fg == nil && c.Style.Bg == nil &&
		c.Style.UnderlineColor == nil && c.Style.Attrs == 0 && c.Style.Underline == 0 && c.Link.URL == ""
}

// decodeRow fills row with the cells in b, and the rest with empty cells.
// Strings for one-byte contents come from asciiCells, so that a row of text
// does not allocate one per cell.
func decodeRow(b []byte, row []uv.Cell) error {
	x := 0
	for len(b) > 0 && x < len(row) {
		if len(b) < 4 {
			return errors.New("short cell")
		}
		flags, width, attrs, underline := b[0], b[1], b[2], b[3]
		b = b[4:]
		n, k := binary.Uvarint(b)
		if k <= 0 || uint64(len(b)-k) < n {
			return errors.New("bad cell content")
		}
		b = b[k:]
		c := uv.Cell{Width: int(width), Style: uv.Style{Attrs: attrs, Underline: uv.Underline(underline)}}
		if n == 1 && b[0] < 0x80 {
			c.Content = asciiCells[b[0]]
		} else {
			c.Content = string(b[:n])
		}
		b = b[n:]
		if flags&wireFG != 0 {
			if len(b) < 4 {
				return errors.New("short color")
			}
			c.Style.Fg = color.NRGBA{R: b[0], G: b[1], B: b[2], A: b[3]}
			b = b[4:]
		}
		if flags&wireBG != 0 {
			if len(b) < 4 {
				return errors.New("short color")
			}
			c.Style.Bg = color.NRGBA{R: b[0], G: b[1], B: b[2], A: b[3]}
			b = b[4:]
		}
		if flags&wireLink != 0 {
			n, k := binary.Uvarint(b)
			if k <= 0 || uint64(len(b)-k) < n {
				return errors.New("bad link")
			}
			c.Link.URL = string(b[k : k+int(n)])
			b = b[k+int(n):]
		}
		row[x] = c
		x++
	}
	for ; x < len(row); x++ {
		row[x] = uv.EmptyCell
	}
	return nil
}

var asciiCells = func() (t [128]string) {
	for i := range t {
		t[i] = string(rune(i))
	}
	return
}()

// rowUpdate is a row of an update.
type rowUpdate struct {
	y     int
	cells []byte
}

// update is an update as the client reads it.
type update struct {
	frame Frame
	rows  []rowUpdate
}

// screenCopy is a copy of the screen, taken under the session's lock so
// that encoding it does not hold up the emulator.
type screenCopy struct {
	frame Frame
	cells []uv.Cell
}

// copyScreen copies the screen at scroll into c, the cells only when the
// screen changed since st. It is called with s.mu held.
func (s *session) copyScreen(c *screenCopy, st *watchState, scroll int) {
	f := s.frameState(scroll)
	c.frame = f
	c.cells = c.cells[:0]
	if st.started && f.Revision == st.revision && f.Cols == st.cols && f.Rows == len(st.hashes) && f.Scroll == st.scroll && f.Alt == st.alt {
		return
	}
	for y := 0; y < f.Rows; y++ {
		for x := 0; x < f.Cols; x++ {
			if p := s.cellAt(x, y, f); p != nil {
				c.cells = append(c.cells, *p)
			} else {
				c.cells = append(c.cells, uv.EmptyCell)
			}
		}
	}
}

// encodeUpdate returns the update that brings a watcher from st to the
// screen in c, or nil when nothing it would see changed.
func encodeUpdate(c *screenCopy, st *watchState, scratch *[]byte) []byte {
	f := c.frame
	full := !st.started || f.Cols != st.cols || f.Rows != len(st.hashes) || f.Scroll != st.scroll || f.Alt != st.alt
	if full {
		st.hashes = make([]uint64, f.Rows)
		st.cols, st.scroll, st.alt, st.started = f.Cols, f.Scroll, f.Alt, true
	}
	var rows []byte
	nrows := 0
	if len(c.cells) == f.Rows*f.Cols && (full || f.Revision != st.revision) {
		for y := 0; y < f.Rows; y++ {
			line := c.cells[y*f.Cols : (y+1)*f.Cols]
			end := len(line)
			for end > 0 && emptyWireCell(&line[end-1]) {
				end--
			}
			cell := (*scratch)[:0]
			for x := 0; x < end; x++ {
				cell = appendCell(cell, &line[x])
			}
			*scratch = cell
			h := fnv.New64a()
			h.Write(cell)
			sum := h.Sum64() | 1 // never 0, which marks a row not sent
			if !full && sum == st.hashes[y] {
				continue
			}
			st.hashes[y] = sum
			rows = binary.AppendUvarint(rows, uint64(y))
			rows = binary.AppendUvarint(rows, uint64(len(cell)))
			rows = append(rows, cell...)
			nrows++
		}
	}
	st.revision = f.Revision
	meta, _ := json.Marshal(f)
	if nrows == 0 && string(meta) == string(st.meta) {
		return nil
	}
	st.meta = meta
	msg := make([]byte, 4, 4+len(meta)+len(rows)+16)
	msg = binary.AppendUvarint(msg, uint64(len(meta)))
	msg = append(msg, meta...)
	msg = binary.AppendUvarint(msg, uint64(nrows))
	msg = append(msg, rows...)
	binary.BigEndian.PutUint32(msg, uint32(len(msg)-4))
	return msg
}

// readUpdate reads an update; the rows' cells point into a buffer that the
// next call reuses.
func readUpdate(r *bufio.Reader, buf *[]byte) (update, error) {
	var u update
	var head [4]byte
	if _, e := io.ReadFull(r, head[:]); e != nil {
		return u, e
	}
	n := binary.BigEndian.Uint32(head[:])
	if n > maxUpdate {
		return u, errors.New("update too large")
	}
	if cap(*buf) < int(n) {
		*buf = make([]byte, n)
	}
	b := (*buf)[:n]
	if _, e := io.ReadFull(r, b); e != nil {
		return u, e
	}
	ml, k := binary.Uvarint(b)
	if k <= 0 || uint64(len(b)-k) < ml {
		return u, errors.New("bad update")
	}
	if e := json.Unmarshal(b[k:k+int(ml)], &u.frame); e != nil {
		return u, e
	}
	b = b[k+int(ml):]
	count, k := binary.Uvarint(b)
	if k <= 0 {
		return u, errors.New("bad update")
	}
	b = b[k:]
	for ; count > 0; count-- {
		y, k := binary.Uvarint(b)
		if k <= 0 {
			return u, errors.New("bad row")
		}
		b = b[k:]
		l, k := binary.Uvarint(b)
		if k <= 0 || uint64(len(b)-k) < l {
			return u, errors.New("bad row")
		}
		u.rows = append(u.rows, rowUpdate{y: int(y), cells: b[k : k+int(l)]})
		b = b[k+int(l):]
	}
	return u, nil
}
