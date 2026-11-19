package vt

import (
	"strings"
	"testing"
)

func TestOSCTitlePreservesUTF8AcrossWrites(t *testing.T) {
	const title = "在 sandbox-tool 设计接口 Ü \U0001f71c"

	for _, end := range []struct{ name, bytes string }{
		{"BEL", "\a"},
		{"escaped ST", "\x1b\\"},
		{"single-byte ST", "\x9c"},
	} {
		t.Run(end.name, func(t *testing.T) {
			term := newTestTerminal(t, 80, 4)
			var titles []string
			term.SetCallbacks(Callbacks{Title: func(title string) { titles = append(titles, title) }})
			term.WriteString("\x1b[?1049h")
			sequence := "\x1b]0;" + title + end.bytes + "OK"
			for i := range len(sequence) {
				term.WriteString(sequence[i : i+1])
			}
			if len(titles) != 1 || titles[0] != title {
				t.Fatalf("received titles %q, want [%q]", titles, title)
			}
			if got := strings.TrimSpace(term.String()); got != "OK" {
				t.Errorf("screen = %q, want %q", got, "OK")
			}
		})
	}
}

func TestOSCIncompleteUTF8DoesNotConsumeST(t *testing.T) {
	term := newTestTerminal(t, 80, 4)
	var title string
	term.SetCallbacks(Callbacks{Title: func(value string) { title = value }})
	term.WriteString("\x1b]0;prefix\xe5\x9c")
	term.WriteString("OK")
	if title != "prefix\xe5" {
		t.Errorf("title = %q, want %q", title, "prefix\xe5")
	}
	if got := strings.TrimSpace(term.String()); got != "OK" {
		t.Errorf("screen = %q, want %q", got, "OK")
	}
}

// OSC 8 wire format per https://gist.github.com/egmontkov/eb6100b9 is
// ESC ] 8 ; <params> ; <uri> ST. handleHyperlink must store the
// <params> segment in Link.Params and the <uri> segment in Link.URL.
// Prior to the fix in this commit the two were swapped: Link.URL
// received parts[1] (params) and Link.Params received parts[2] (uri),
// which produced visible-but-unclickable links once Buffer.Render()
// emitted them back out via ansi.SetHyperlink(URL, Params).
func TestHandleHyperlink_StoresURLAndParamsInCorrectFields(t *testing.T) {
	t.Parallel()

	t.Run("empty params, populated uri (common case)", func(t *testing.T) {
		t.Parallel()
		term := newTestTerminal(t, 80, 24)
		// Feed an OSC 8 open + a single character + OSC 8 close so the
		// open's Link metadata sticks on the cell.
		_, _ = term.Write([]byte("\x1b]8;;https://example.com/\x1b\\X\x1b]8;;\x1b\\"))
		cell := term.CellAt(0, 0)
		if cell == nil {
			t.Fatal("cell at (0,0) is nil")
		}
		if got, want := cell.Link.URL, "https://example.com/"; got != want {
			t.Errorf("Link.URL = %q, want %q", got, want)
		}
		if got, want := cell.Link.Params, ""; got != want {
			t.Errorf("Link.Params = %q, want %q", got, want)
		}
	})

	t.Run("populated params and uri", func(t *testing.T) {
		t.Parallel()
		term := newTestTerminal(t, 80, 24)
		_, _ = term.Write([]byte("\x1b]8;id=42:tag=demo;https://example.com/path?x=1\x1b\\X\x1b]8;;\x1b\\"))
		cell := term.CellAt(0, 0)
		if cell == nil {
			t.Fatal("cell at (0,0) is nil")
		}
		if got, want := cell.Link.URL, "https://example.com/path?x=1"; got != want {
			t.Errorf("Link.URL = %q, want %q", got, want)
		}
		if got, want := cell.Link.Params, "id=42:tag=demo"; got != want {
			t.Errorf("Link.Params = %q, want %q", got, want)
		}
	})
}
