package vt

import (
	"strings"
	"testing"
)

func TestParserDataGrowsAcrossWritesWithoutTruncatingOSC(t *testing.T) {
	e := NewEmulator(20, 4)
	defer e.Close()
	var title string
	e.SetCallbacks(Callbacks{Title: func(s string) { title = s }})
	if e.parserDataSize != initialParserDataSize {
		t.Fatalf("idle parser reserves %d bytes", e.parserDataSize)
	}
	want := strings.Repeat("title-中文-", 16000)
	data := "\x1b]2;" + want + "\x1b\\OK"
	for offset := 0; offset < len(data); offset += 7919 {
		e.WriteString(data[offset:min(offset+7919, len(data))])
	}
	if title != want {
		t.Fatalf("fragmented OSC title: got %d bytes, want %d", len(title), len(want))
	}
	if e.CellAt(0, 0).Content != "O" || e.CellAt(1, 0).Content != "K" {
		t.Fatal("text after OSC was consumed by the parser")
	}
	if e.parserDataSize <= initialParserDataSize || e.parserDataSize > maxParserDataSize {
		t.Fatalf("unexpected parser buffer size %d", e.parserDataSize)
	}
}

func TestParserDataKeepsExistingLimit(t *testing.T) {
	e := NewEmulator(20, 4)
	defer e.Close()
	var title string
	e.SetCallbacks(Callbacks{Title: func(s string) { title = s }})
	e.WriteString("\x1b]2;" + strings.Repeat("x", maxParserDataSize+1000) + "\aOK")
	if len(title) != maxParserDataSize-2 || e.parserDataSize != maxParserDataSize {
		t.Fatalf("OSC limit changed: title=%d, buffer=%d", len(title), e.parserDataSize)
	}
	e.WriteString("\x1b]2;short\a")
	if title != "short" {
		t.Fatalf("parser did not recover after full buffer: %q", title)
	}
}
