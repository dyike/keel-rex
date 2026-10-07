package main

import (
	uv "github.com/charmbracelet/ultraviolet"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitScreen(t *testing.T, s *session, needle string) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if strings.Contains(s.plain(), needle) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("missing %q, screen:\n%s", needle, s.plain())
}
func TestPersistentPTYAndInterrupt(t *testing.T) {
	dir := t.TempDir()
	s, e := newSession(dir, "/bin/zsh", "-f")
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	s.text("export REX_TEST=retained; printf 'PTY:%s\\n' $(test -t 0 && echo yes); mkdir child; cd child; printf 'STATE:%s:%s\\n' $REX_TEST ${PWD:t}\r", false)
	waitScreen(t, s, "STATE:retained:child")
	waitScreen(t, s, "PTY:yes")
	s.text("sleep 30\r", false)
	time.Sleep(80 * time.Millisecond)
	s.send(uv.KeyPressEvent{Code: 'c', Mod: uv.ModCtrl})
	s.text("printf '%s_%s\\n' INTERRUPT OK\r", false)
	waitScreen(t, s, "INTERRUPT_OK")
}
func TestPTYResizeColorsAndAlternateScreen(t *testing.T) {
	s, e := newSession(t.TempDir(), "/bin/zsh", "-f")
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	s.resize(101, 33)
	s.text("stty size; printf '\\033[31mRED_CELL\\033[0m\\n'\r", false)
	waitScreen(t, s, "33 101")
	waitScreen(t, s, "RED_CELL")
	s.mu.Lock()
	found := false
	for y := 0; y < s.rows; y++ {
		for x := 0; x < s.cols; x++ {
			c := s.emu.CellAt(x, y)
			if c != nil && c.Content == "R" && c.Style.Fg != nil {
				found = true
			}
		}
	}
	s.mu.Unlock()
	if !found {
		t.Fatal("ANSI foreground not retained")
	}
	s.text("printf '\\033[?1049h\\033[2JALT_ACTIVE'; read -k 1; printf '\\033[?1049l'\r", false)
	waitScreen(t, s, "ALT_ACTIVE")
	s.mu.Lock()
	alt := s.emu.IsAltScreen()
	s.mu.Unlock()
	if !alt {
		t.Fatal("alternate screen not enabled")
	}
	s.text("x", false)
	time.Sleep(120 * time.Millisecond)
	s.mu.Lock()
	alt = s.emu.IsAltScreen()
	s.mu.Unlock()
	if alt {
		t.Fatal("alternate screen not restored")
	}
}
func TestIndependentSessionsAndClose(t *testing.T) {
	dir := t.TempDir()
	a, e := newSession(dir, "/bin/zsh", "-f")
	if e != nil {
		t.Fatal(e)
	}
	b, e := newSession(dir, "/bin/zsh", "-f")
	if e != nil {
		t.Fatal(e)
	}
	defer a.close()
	defer b.close()
	a.text("export REX_ISOLATED=secret; printf 'A_READY\\n'\r", false)
	waitScreen(t, a, "A_READY")
	b.text("printf 'B:%s\\n' ${REX_ISOLATED-unset}\r", false)
	waitScreen(t, b, "B:unset")
	a.close()
	select {
	case <-a.done:
	case <-time.After(3 * time.Second):
		t.Fatal("closed session process still running")
	}
	b.text("printf 'B_ALIVE\\n'\r", false)
	waitScreen(t, b, "B_ALIVE")
}
func TestVimPTYRoundTrip(t *testing.T) {
	if _, e := exec.LookPath("vim"); e != nil {
		t.Skip("vim unavailable")
	}
	dir := t.TempDir()
	s, e := newSession(dir, "/bin/zsh", "-f")
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	s.text("vim -u NONE -n result.txt\r", false)
	time.Sleep(400 * time.Millisecond)
	s.text("iNative Rex 编辑测试", false)
	s.send(uv.KeyPressEvent{Code: uv.KeyEscape})
	s.text(":wq\r", false)
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		if b, e := os.ReadFile(filepath.Join(dir, "result.txt")); e == nil && strings.Contains(string(b), "Native Rex 编辑测试") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("vim edit/save failed: %s", s.plain())
}

func TestPasteDoesNotBlockOnForegroundProgram(t *testing.T) {
	s, e := newSession(t.TempDir(), "/bin/sh", "-c", "sleep 5")
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	done := make(chan struct{})
	go func() { s.text(strings.Repeat("x", 100000), true); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PTY backpressure blocked input/UI")
	}
}
