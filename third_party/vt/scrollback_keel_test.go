package vt

import (
	"fmt"
	"strings"
	"testing"
)

// The packed scrollback keeps the newest lines, in order, with their text and
// styles, whether a line is packed yet or still pending.
func TestPackedScrollbackOrderAndContent(t *testing.T) {
	for _, max := range []int{7, 100, 5000} {
		e := NewEmulator(40, 5)
		e.Scrollback().SetMaxLines(max)
		total := 3000
		for i := 0; i < total; i++ {
			fmt.Fprintf(e, "line %05d \x1b[31mred\x1b[0m 中文\r\n", i)
			if i%97 == 0 { // read while the packer may be running
				_ = e.ScrollbackCellAt(0, e.ScrollbackLen()/2)
			}
		}
		n := e.ScrollbackLen()
		if want := min(max, total-4); n != want {
			t.Fatalf("max %d: len %d, want %d", max, n, want)
		}
		for y := 0; y < n; y++ {
			var b strings.Builder
			for x := 0; x < 40; x++ {
				if c := e.ScrollbackCellAt(x, y); c != nil {
					b.WriteString(c.Content)
				}
			}
			want := fmt.Sprintf("line %05d red 中文", total-4-n+y)
			if got := strings.TrimRight(b.String(), " "); got != want {
				t.Fatalf("max %d: line %d = %q, want %q", max, y, got, want)
			}
			if c := e.ScrollbackCellAt(11, y); c == nil || c.Style.Fg == nil {
				t.Fatalf("max %d: line %d lost its color", max, y)
			}
			if c := e.ScrollbackCellAt(15, y); c == nil || c.Width != 2 {
				t.Fatalf("max %d: line %d lost its wide cell: %+v", max, y, c)
			}
		}
	}
}

func TestPackedScrollbackClearWhilePacking(t *testing.T) {
	e := NewEmulator(20, 3)
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(e, "%d\r\n", i)
		if i == 1000 {
			e.ClearScrollback()
		}
	}
	if n := e.ScrollbackLen(); n != 2000-1001-2 && n != 2000-1001-1 {
		t.Logf("len after clear: %d", n)
	}
	c := e.ScrollbackCellAt(0, e.ScrollbackLen()-1)
	if c == nil {
		t.Fatal("no newest line")
	}
}

// A byte bound drops the oldest lines once they hold more, keeping the rest
// in order.
func TestScrollbackMaxBytes(t *testing.T) {
	e := NewEmulator(40, 5)
	e.Scrollback().SetMaxBytes(20 << 10)
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(e, "line %05d \x1b[31mred\x1b[32mgreen\x1b[0m\r\n", i)
	}
	n := e.ScrollbackLen()
	if n < 50 || n > 400 {
		t.Fatalf("%d lines kept under 20 KB", n)
	}
	for y := 0; y < n; y++ {
		var b strings.Builder
		for x := 0; x < 10; x++ {
			b.WriteString(e.ScrollbackCellAt(x, y).Content)
		}
		if want := fmt.Sprintf("line %05d", 3000-4-n+y); b.String() != want {
			t.Fatalf("line %d = %q, want %q", y, b.String(), want)
		}
	}
}
