package backend

import (
	"strings"
	"testing"
)

func TestSearchScrollbackUnicodeAndNext(t *testing.T) {
	s, e := newSession(t.TempDir(), "/bin/zsh", "-f")
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	s.mu.Lock()
	s.emu.WriteString("\x1b[2J\x1b[H")
	s.emu.WriteString("prefix 中文 target\r\nsecond 中文 target\r\n" + strings.Repeat("padding\r\n", 60))
	s.mu.Unlock()
	first := s.search("中文 target", -1)
	if first == nil || first.Count != 2 || first.Scroll <= 0 || first.Anchor != 7 {
		t.Fatalf("wide-character scrollback match: %+v", first)
	}
	next := s.search("中文 target", first.Position)
	if next == nil || next.Position <= first.Position || next.Anchor != 7 {
		t.Fatalf("next match: %+v", next)
	}
	wrapped := s.search("中文 target", next.Position)
	if wrapped.Position != first.Position {
		t.Fatal("search did not wrap")
	}
	if s.search("no match", -1) != nil {
		t.Fatal("matched missing text")
	}
}
