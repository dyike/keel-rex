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
	// Fill the first buffer just before the 0x9c continuation byte in 在.
	want := strings.Repeat("x", initialParserDataSize-3) + "在" + strings.Repeat("title-中文-", 16000)
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
	want := strings.Repeat("x", maxParserDataSize-2)
	e.WriteString("\x1b]2;" + want + "在\aOK")
	if title != want || e.parserDataSize != maxParserDataSize {
		t.Fatalf("OSC limit changed: title=%d, buffer=%d", len(title), e.parserDataSize)
	}
	if got := strings.TrimSpace(e.String()); got != "OK" {
		t.Errorf("screen = %q, want %q", got, "OK")
	}
	e.WriteString("\x1b]2;short\a")
	if title != "short" {
		t.Fatalf("parser did not recover after full buffer: %q", title)
	}
}
