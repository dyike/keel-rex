package vt

import (
	"bytes"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi/parser"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// printableASCII reports whether r is a character that can never join the
// grapheme cluster beside it. No ASCII character is a combining mark, a ZWJ, a
// spacing mark or a regional indicator, and CR and LF are not printable, so a
// cluster boundary between two of these is certain without asking the
// segmenter.
func printableASCII(r rune) bool {
	return r >= ansi.SP && r < ansi.DEL
}

// handlePrint handles printable characters.
//
// A character is held back rather than printed straight away, because the one
// after it may be a combining mark that belongs to the same cluster. Printing
// "e" from "e\u0301" before the mark arrives puts the base in one cell and
// leaves the mark to become a zero-width cell of its own, which the next write
// or erase then destroys.
func (e *Emulator) handlePrint(r rune) {
	// No ASCII character can extend the cluster before it: none of them is
	// Extend, ZWJ, SpacingMark, Prepend, a regional indicator, or part of an
	// Indic conjunct. So an ASCII character arriving is proof that whatever is
	// buffered is finished, and the buffer can go out.
	//
	// This is also what keeps the buffer from being re-segmented as it grows.
	// Asking where the clusters are on every character costs the length of the
	// buffer each time, which is quadratic over a long run of combining marks
	// that never resolves into more than one cluster.
	if printableASCII(r) {
		// Two printable ASCII characters in a row is the common case by a wide
		// margin, and needs no segmenting at all.
		if len(e.grapheme) == 1 && printableASCII(rune(e.grapheme[0])) {
			e.handleGrapheme(asciiString[e.grapheme[0]], 1)
			e.grapheme = utf8.AppendRune(e.grapheme[:0], r)
			return
		}
		e.flushGrapheme()
	}

	e.grapheme = utf8.AppendRune(e.grapheme, r)
}

// flushGrapheme flushes the current grapheme buffer, if any, and handles the
// grapheme as a single unit.
func (e *Emulator) flushGrapheme() {
	if len(e.grapheme) == 0 {
		return
	}

	// XXX: We always use [ansi.GraphemeWidth] here to report accurate widths
	// and it's up to the caller to decide how to handle Unicode vs non-Unicode
	// modes.
	method := ansi.GraphemeWidth
	graphemes := e.grapheme
	for len(graphemes) > 0 {
		// keel-rex: a character whose cluster cannot extend past it, before
		// another such character or nothing, is a cluster of its own; skip
		// the segmenter and its allocation for CJK text and box drawing.
		if r, size := utf8.DecodeRune(graphemes); r >= utf8.RuneSelf {
			if info := e.runeInfo(r); info.simple && (len(graphemes) == size || e.simpleNext(graphemes[size:])) {
				e.handleGrapheme(info.s, info.w)
				graphemes = graphemes[size:]
				continue
			}
		}
		cluster, width := ansi.FirstGraphemeCluster(graphemes, method)
		if len(cluster) == 0 {
			break
		}
		if len(cluster) == 1 && cluster[0] < utf8.RuneSelf {
			e.handleGrapheme(asciiString[cluster[0]], width)
		} else {
			e.handleGrapheme(string(cluster), width)
		}
		graphemes = graphemes[len(cluster):]
	}
	e.grapheme = e.grapheme[:0] // Reset the grapheme buffer.
}

// handleGrapheme handles UTF-8 graphemes.
func (e *Emulator) handleGrapheme(content string, width int) {
	awm := e.autoWrap
	cell := uv.Cell{
		Content: content,
		Width:   width,
		Style:   e.scr.cursorPen(),
		Link:    e.scr.cursorLink(),
	}

	x, y := e.scr.CursorPosition()
	if e.atPhantom && awm {
		// moves cursor down similar to [Terminal.linefeed] except it doesn't
		// respects [ansi.LNM] mode.
		// This will reset the phantom state i.e. pending wrap state.
		e.index()
		_, y = e.scr.CursorPosition()
		x = 0
	}

	// A single shift applies to the next character, whatever that character
	// turns out to be. Take it now, so it cannot leak onto the one after this
	// when this is a cluster no charset has a mapping for.
	single := e.gsingle
	e.gsingle = 0

	// Handle character set mappings
	if len(content) == 1 { //nolint:nestif
		var charset CharSet
		c := content[0]
		if single > 1 && single < 4 {
			charset = e.charsets[single]
		} else if c < 128 {
			charset = e.charsets[e.gl]
		} else {
			charset = e.charsets[e.gr]
		}

		if charset != nil {
			if r, ok := charset[c]; ok {
				cell.Content = r
				cell.Width = 1
			}
		}
	}

	if cell.Width == 1 && len(content) == 1 {
		e.lastChar, _ = utf8.DecodeRuneInString(content)
	}

	e.scr.SetCell(x, y, &cell)

	// Handle phantom state at the end of the line
	e.atPhantom = awm && x >= e.scr.Width()-1
	if !e.atPhantom {
		x += cell.Width
	}

	// NOTE: We don't reset the phantom state here, we handle it up above.
	e.scr.setCursor(x, y, false)
}

// asciiString holds the one-byte strings, so that printing ASCII does not
// allocate one per character.
var asciiString = func() (t [utf8.RuneSelf]string) {
	for i := range t {
		t[i] = string(rune(i))
	}
	return
}()

// fastPaths turns on printText and scrollText; tests turn it off to compare
// with printing a character at a time.
var fastPaths = true

// fastScroll turns on scrollText alone, for tests.
var fastScroll = true

// textRun returns how many bytes from the start of p are printable ASCII
// or simple characters, whose cluster boundaries are certain among them.
func (e *Emulator) textRun(p []byte) int {
	k := 0
	for k < len(p) {
		if c := p[k]; c < utf8.RuneSelf {
			if !printableASCII(rune(c)) {
				break
			}
			k++
			continue
		}
		r, size := utf8.DecodeRune(p[k:])
		if !e.runeInfo(r).simple {
			break
		}
		k += size
	}
	return k
}

// printText prints a run of text from textRun as handlePrint would one
// character at a time, writing the cells of each line in one go. The last
// character is held back as handlePrint holds it, since a combining mark may
// follow. It reports false, having done nothing, when a character set or
// disabled autowrap makes characters other than themselves.
func (e *Emulator) printText(run []byte) bool {
	if e.gsingle != 0 || e.charsets[e.gl] != nil || !e.autoWrap {
		return false
	}
	e.flushGrapheme()
	_, last := utf8.DecodeLastRune(run)
	hold := run[len(run)-last:]
	run = run[:len(run)-last]
	scr := e.scr
	width := scr.Width()
	for len(run) > 0 {
		x, y := scr.CursorPosition()
		if e.atPhantom {
			e.index()
			_, y = scr.CursorPosition()
			x = 0
			scr.setCursor(x, y, false)
		}
		if y < 0 || y >= len(scr.buf.Lines) || x >= width {
			break
		}
		line := scr.buf.Lines[y]
		pen, link := scr.cur.Pen, scr.cur.Link
		col, k, at := x, 0, x // at: where the last character went
		for k < len(run) {
			s, w, size := e.textChar(run[k:])
			// A wide character that does not fit, or cells of wide ones in
			// the way, need handleGrapheme and uv's fixups.
			if col+w > width || line[col].Width != 1 || w == 2 && line[col+1].Width != 1 {
				break
			}
			line[col] = uv.Cell{Content: s, Width: w, Style: pen, Link: link}
			if w == 2 {
				line[col+1] = uv.Cell{}
			}
			if size == 1 {
				e.lastChar = rune(run[k])
			}
			at = col
			col += w
			k += size
		}
		if col == x { // the next character goes the slow way
			s, w, size := e.textChar(run)
			e.handleGrapheme(s, w)
			run = run[size:]
			continue
		}
		scr.buf.TouchLine(x, y, col-x)
		run = run[k:]
		// As handleGrapheme has it: the wrap is pending after a character
		// in the last column; one ending there leaves the cursor on it.
		e.atPhantom = at >= width-1
		scr.setCursor(min(col, width-1), y, false)
	}
	for len(run) > 0 { // the cursor left the screen: let the slow path decide
		s, w, size := e.textChar(run)
		e.handleGrapheme(s, w)
		run = run[size:]
	}
	e.grapheme = append(e.grapheme[:0], hold...)
	return true
}

// textChar returns the string and width of the character text starts with,
// one textRun takes, and its length in bytes.
func (e *Emulator) textChar(text []byte) (string, int, int) {
	if c := text[0]; c < utf8.RuneSelf {
		return asciiString[c], 1, 1
	}
	r, size := utf8.DecodeRune(text)
	info := e.runeInfo(r)
	return info.s, info.w, size
}

// runeInfo is what printing needs to know of a character: its string, and
// whether it is a cluster of its own between printable ASCII and other such
// characters, and then its width.
type runeInfo struct {
	s      string
	w      int
	simple bool
}

// classify finds the runeInfo of r. A character is simple when the
// segmenter splits it from ASCII on either side and it is not one of the
// characters that join by pairs (Hangul jamo and syllables, regional
// indicators): then UAX #29 breaks between it and anything simple, and its
// cluster is the character alone, of the width the segmenter gives it.
func classify(r rune) runeInfo {
	s := string(r)
	info := runeInfo{s: s}
	if r < 0xA0 || r == utf8.RuneError || hangul(r) || r >= 0x1F1E6 && r <= 0x1F1FF {
		return info
	}
	before, w := ansi.FirstGraphemeCluster([]byte(s+"a"), ansi.GraphemeWidth)
	after, _ := ansi.FirstGraphemeCluster([]byte("a"+s), ansi.GraphemeWidth)
	if string(before) == s && string(after) == "a" && w >= 1 && w <= 2 {
		info.w, info.simple = w, true
	}
	return info
}

func hangul(r rune) bool {
	return r >= 0x1100 && r <= 0x11FF || r >= 0x3130 && r <= 0x318F || r >= 0xA960 && r <= 0xA97F ||
		r >= 0xAC00 && r <= 0xD7FF
}

// runeInfo returns classify(r), from a cache so that printing text neither
// segments nor allocates a string per character.
func (e *Emulator) runeInfo(r rune) runeInfo {
	if info, ok := e.runes[r]; ok {
		return info
	}
	info := classify(r)
	if e.runes == nil {
		e.runes = make(map[rune]runeInfo)
	}
	if len(e.runes) < 1<<14 {
		e.runes[r] = info
	}
	return info
}

// simpleNext reports whether the cluster boundary before b is certain: b
// starts with printable ASCII or a simple character.
func (e *Emulator) simpleNext(b []byte) bool {
	if b[0] < utf8.RuneSelf {
		return printableASCII(rune(b[0]))
	}
	r, _ := utf8.DecodeRune(b)
	return e.runeInfo(r).simple
}

// scrollText takes, from the start of p, lines of text each ended by CR LF,
// when they fill more rows than the screen has and the cursor waits at the
// start of a blank bottom line: output scrolling past faster than it can be
// seen. A line is printable ASCII and characters whose clusters stand alone,
// with SGR sequences between them; a line wider than the screen wraps onto
// the rows below, as printing wraps it. Printing them one by one would make
// cells of every row only to pack it into the scrollback a screen later;
// here the rows that leave go to the scrollback as text, and only the last
// screenful becomes cells. The result is what printing them would leave.
//
// It returns how many bytes it took, or 0 and how far into p not to look
// again: past a line it cannot take, or all of p when too few rows follow.
func (e *Emulator) scrollText(p []byte) (n, resume int) {
	scr := e.scr
	w, h := scr.Width(), scr.Height()
	if scr.scrollback == nil || len(p) < 2*h || h < 2 || w < 2 ||
		e.parser.State() != parser.GroundState || len(e.grapheme) > 0 ||
		e.gsingle != 0 || e.charsets[e.gl] != nil || !e.autoWrap || e.atPhantom ||
		scr.cur.X != 0 || scr.cur.Y != h-1 || scr.cur.Pen.Bg != nil ||
		scr.scroll != scr.buf.Bounds() {
		return 0, 1
	}
	if bytes.Count(p, newline) < h { // too few lines to look at, at a glance
		return 0, len(p)
	}
	rows := e.textLines[:0] // the text of each row the lines fill
	pen := scr.cur.Pen      // as the lines leave it
	var last byte           // the last one-byte character printed
	j := 0
lines:
	for j < len(p) {
		linePen, lineLast := pen, last
		lineRows := len(rows) // rows of the line so far go if it fails
		k, start, cols := j, j, 0
		edgeWide := false
		for {
			if k+1 < len(p) && p[k] == '\r' && p[k+1] == '\n' {
				// The line feed scrolls a blank row in, in the pen's
				// background: it must have none.
				if linePen.Bg != nil {
					rows = rows[:lineRows]
					break lines
				}
				rows = append(rows, p[start:k])
				pen, last = linePen, lineLast
				j = k + 2
				continue lines
			}
			if k >= len(p) {
				rows = rows[:lineRows]
				break lines
			}
			c := p[k]
			if c == ansi.ESC {
				n, params, ok := sgrAt(p[k:], &e.sgrParams)
				if !ok {
					rows = rows[:lineRows]
					break lines
				}
				uv.ReadStyle(params, &linePen)
				k += n
				continue
			}
			size, cw := 1, 1
			if c < utf8.RuneSelf {
				if !printableASCII(rune(c)) {
					rows = rows[:lineRows]
					break lines
				}
			} else {
				// A character of a simple range is a cluster of its own
				// when what follows is ASCII or another such character,
				// which the next round checks.
				r, s := utf8.DecodeRune(p[k:])
				info := e.runeInfo(r)
				if !info.simple {
					rows = rows[:lineRows]
					break lines
				}
				size, cw = s, info.w
			}
			if cols == w && edgeWide {
				// A wide character that ended at the last column left the
				// cursor on it, not waiting to wrap: the next character
				// takes its second cell. handleGrapheme's to place.
				rows = rows[:lineRows]
				break lines
			}
			if cols+cw > w {
				// A full row wraps when the next character comes, and
				// scrolls a blank row in as a line feed does. A wide
				// character that does not fit a row that is not full is
				// handleGrapheme's to place.
				if cols < w || linePen.Bg != nil {
					rows = rows[:lineRows]
					break lines
				}
				rows = append(rows, p[start:k])
				start, cols = k, 0
			}
			if c < utf8.RuneSelf {
				lineLast = c
			}
			cols += cw
			edgeWide = cw > 1 && cols == w
			k += size
		}
	}
	e.textLines = rows[:0]
	if len(rows) < h {
		if j == 0 || len(rows) == 0 {
			// The first line will not do: look again past its end.
			if i := bytes.IndexByte(p[j:], '\n'); i >= 0 {
				return 0, j + i + 1
			}
			return 0, len(p)
		}
		return 0, j
	}
	if !blankLine(scr.buf.Lines[h-1]) {
		return 0, 1
	}
	// The screen's rows above the bottom one leave first, then the rows that
	// never reach the screen; the last h-1 rows and a blank row stay.
	screen := scr.buf.Lines
	bottom := screen[h-1]
	for y := 0; y < h-1; y++ {
		scr.scrollback.Adopt(screen[y])
		screen[y] = scr.scrollback.Blank(w)
	}
	keep := rows[len(rows)-(h-1):]
	done := scr.scrollback.direct()
	for _, text := range rows[:len(rows)-(h-1)] {
		if isPlainText(text) {
			scr.scrollback.appendText(text, scr.cur.Pen, scr.cur.Link)
		} else {
			e.cellLine = e.textCells(text, e.cellLine[:0])
			scr.scrollback.appendCells(e.cellLine)
		}
	}
	done()
	for y, text := range keep {
		row := screen[y]
		fillLine(row, &uv.EmptyCell)
		e.textCells(text, row[:0])
	}
	screen[h-1] = bottom
	for y := 0; y < h; y++ {
		scr.buf.TouchLine(0, y, w)
	}
	if last != 0 {
		e.lastChar = rune(last)
	}
	return j, 0
}

// isPlainText reports whether text is printable ASCII and nothing else.
func isPlainText(text []byte) bool {
	for _, c := range text {
		if c >= utf8.RuneSelf || c == ansi.ESC {
			return false
		}
	}
	return true
}

// sgrAt reads the SGR sequence p starts with, ESC [ parameters m, its
// parameters as the parser collects them, into buf. It reports false for
// anything else, a sequence with a private marker or intermediate bytes
// included.
func sgrAt(p []byte, buf *ansi.Params) (n int, params ansi.Params, ok bool) {
	if len(p) < 3 || p[0] != ansi.ESC || p[1] != '[' {
		return 0, nil, false
	}
	var raw [parser.MaxParamsSize]int
	raw[0] = parser.MissingParam
	count := 0
	for i := 2; i < len(p) && i < 64; i++ {
		switch b := p[i]; {
		case b >= '0' && b <= '9':
			if count < len(raw) {
				if raw[count] == parser.MissingParam {
					raw[count] = 0
				}
				raw[count] = raw[count]*10 + int(b-'0')
			}
		case b == ';' || b == ':':
			if count < len(raw) {
				if b == ':' {
					raw[count] |= parser.HasMoreFlag
				}
				count++
				if count < len(raw) {
					raw[count] = parser.MissingParam
				}
			}
		case b == 'm':
			if count > 0 && count < len(raw)-1 || count == 0 && raw[0] != parser.MissingParam {
				count++
			}
			params = (*buf)[:0]
			for k := 0; k < count; k++ {
				params = append(params, ansi.Param(raw[k]))
			}
			*buf = params
			return i + 1, params, true
		default:
			return 0, nil, false
		}
	}
	return 0, nil, false
}

// blankLine reports whether every cell of line is a plain empty cell.
func blankLine(line uv.Line) bool {
	for i := range line {
		c := &line[i]
		if c.Content != " " || c.Width != 1 || !plainStyle(&c.Style) || len(c.Link.URL)|len(c.Link.Params) != 0 {
			return false
		}
	}
	return true
}

// textCells appends to dst the cells that printing text, of the lines
// scrollText takes, leaves at the start of a blank line: a cell per
// character in the cursor's pen, and an empty cell after a wide one. Its SGR
// sequences change the pen, as they do printed.
func (e *Emulator) textCells(text []byte, dst uv.Line) uv.Line {
	for len(text) > 0 {
		c := text[0]
		if c == ansi.ESC {
			n, params, _ := sgrAt(text, &e.sgrParams)
			uv.ReadStyle(params, &e.scr.cur.Pen)
			text = text[n:]
			continue
		}
		pen, link := e.scr.cur.Pen, e.scr.cur.Link
		if c < utf8.RuneSelf {
			dst = append(dst, uv.Cell{Content: asciiString[c], Width: 1, Style: pen, Link: link})
			text = text[1:]
			continue
		}
		r, size := utf8.DecodeRune(text)
		info := e.runeInfo(r)
		dst = append(dst, uv.Cell{Content: info.s, Width: info.w, Style: pen, Link: link})
		for j := 1; j < info.w; j++ {
			dst = append(dst, uv.Cell{})
		}
		text = text[size:]
	}
	return dst
}

var newline = []byte{'\n'}
