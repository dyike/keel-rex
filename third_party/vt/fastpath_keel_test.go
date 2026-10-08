package vt

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// fuzzOutput makes output that mixes what the fast paths take with what
// they must leave alone.
func fuzzOutput(r *rand.Rand, lines int) string {
	var b strings.Builder
	words := []string{"alpha", "b", "  ", "中文", "é́", "─┼─", "\x1b[31m", "\x1b[0m", "\x1b[44m", "\x1b[1;38;2;10;20;30m", "\t", "x", "\x1b]8;;http://a\x1b\\link\x1b]8;;\x1b\\", "\x1b[2K", "\x1b[3A", "\x1b[5G", "\x1b[?7l", "\x1b[?7h", "🚀",
		"\x1b[m", "\x1b[38:5:99m", "\x1b[4:3m", "\x1b[;1m", "\x1b[49m", "\x1b[48;5;22m", "\x1b[7m", "\x1b[22;39m", "\x1b[?1m", "\x1b[1$m"}
	for i := 0; i < lines; i++ {
		switch r.Intn(10) {
		case 0, 1, 2, 3, 4, 5: // plain text lines, some long enough to wrap
			n := r.Intn(30)
			if r.Intn(8) == 0 {
				n = 60 + r.Intn(40)
			}
			for k := 0; k < n; k++ {
				b.WriteByte(byte('a' + r.Intn(26)))
				if r.Intn(6) == 0 {
					b.WriteByte(' ')
				}
			}
		case 6:
			b.WriteString(strings.Repeat(" ", r.Intn(5)))
		case 7, 8: // text with wide and box-drawing characters, as scrollText takes
			simple := []string{"中", "文", "，", "Ａ", "─", "█", "é", "a", " ", "b", "终端", "🚀", "✅", "✔", "→"}
			for k := r.Intn(20); k >= 0; k-- {
				b.WriteString(simple[r.Intn(len(simple))])
			}
			if r.Intn(5) == 0 { // and now and then a pen for the lines after
				b.WriteString([]string{"\x1b[32m", "\x1b[0m", "\x1b[4m"}[r.Intn(3)])
			}
		case 9: // colored words, as ls and compilers print them
			for k := r.Intn(8); k >= 0; k-- {
				b.WriteString([]string{"\x1b[31m", "\x1b[1;34m", "\x1b[38;5;208m", "\x1b[38;2;1;2;3m", "\x1b[0m", "\x1b[m", "\x1b[44m", "\x1b[49m"}[r.Intn(8)])
				b.WriteString([]string{"word", "中文", "x", "  ", "─"}[r.Intn(5)])
			}
			if r.Intn(2) == 0 {
				b.WriteString("\x1b[0m")
			}
		default:
			for k := r.Intn(6); k >= 0; k-- {
				b.WriteString(words[r.Intn(len(words))])
			}
		}
		switch r.Intn(12) {
		case 0:
			b.WriteString("\n")
		case 1:
			b.WriteString("\r")
		default:
			b.WriteString("\r\n")
		}
	}
	return b.String()
}

func writeChunks(e *Emulator, s string, r *rand.Rand) {
	for len(s) > 0 {
		n := min(len(s), 1+r.Intn(5000))
		e.WriteString(s[:n])
		s = s[n:]
	}
}

func dumpCell(c *uv.Cell) string {
	if c == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q/%d/%v/%v/%v/%d/%d/%q", c.Content, c.Width, c.Style.Fg, c.Style.Bg, c.Style.UnderlineColor, c.Style.Attrs, c.Style.Underline, c.Link.URL)
}

// The fast paths leave the screen and scrollback exactly as printing one
// character at a time does.
func TestFastPathsMatchSlowPath(t *testing.T) {
	for seed := int64(1); seed <= 400; seed++ {
		r := rand.New(rand.NewSource(seed))
		w, h := 20+r.Intn(60), 3+r.Intn(20)
		maxLines := []int{5, 50, 400, 10000}[r.Intn(4)]
		out := fuzzOutput(r, 50+r.Intn(800))
		chunkSeed := r.Int63()

		run := func(fast bool) *Emulator {
			fastPaths = fast
			defer func() { fastPaths = true }()
			e := NewEmulator(w, h)
			e.Scrollback().SetMaxLines(maxLines)
			writeChunks(e, out, rand.New(rand.NewSource(chunkSeed)))
			return e
		}
		slow, fast := run(false), run(true)
		if a, b := slow.ScrollbackLen(), fast.ScrollbackLen(); a != b {
			t.Fatalf("seed %d: scrollback %d lines slow, %d fast", seed, a, b)
		}
		for y := 0; y < slow.ScrollbackLen(); y++ {
			for x := 0; x < w; x++ {
				if a, b := dumpCell(slow.ScrollbackCellAt(x, y)), dumpCell(fast.ScrollbackCellAt(x, y)); a != b {
					t.Fatalf("seed %d: scrollback (%d,%d): slow %s fast %s", seed, x, y, a, b)
				}
			}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if a, b := dumpCell(slow.CellAt(x, y)), dumpCell(fast.CellAt(x, y)); a != b {
					t.Fatalf("seed %d: screen (%d,%d): slow %s fast %s", seed, x, y, a, b)
				}
			}
		}
		if a, b := slow.CursorPosition(), fast.CursorPosition(); a != b {
			t.Fatalf("seed %d: cursor slow %v fast %v", seed, a, b)
		}
	}
}
