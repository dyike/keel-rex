package main

import (
	"image"
	"testing"
	"time"
)

func TestTerminalCaretBlinkTimingAndRestart(t *testing.T) {
	term := &terminal{focused: true}
	start := time.Unix(100, 0)
	pos := image.Pt(3, 2)
	for _, step := range []struct {
		elapsed time.Duration
		visible bool
		next    time.Duration
	}{
		{0, true, 500 * time.Millisecond},
		{499 * time.Millisecond, true, 500 * time.Millisecond},
		{500 * time.Millisecond, false, time.Second},
		{999 * time.Millisecond, false, time.Second},
		{time.Second, true, 1500 * time.Millisecond},
		{2750 * time.Millisecond, false, 3 * time.Second},
	} {
		visible, next := term.caretBlink(start.Add(step.elapsed), pos, true)
		if visible != step.visible || !next.Equal(start.Add(step.next)) {
			t.Fatalf("at %s: visible=%v next=%s", step.elapsed, visible, next.Sub(start))
		}
	}
	// Moving the application cursor during a hidden phase shows it immediately.
	now := start.Add(2800 * time.Millisecond)
	if visible, next := term.caretBlink(now, image.Pt(4, 2), true); !visible || !next.Equal(now.Add(500*time.Millisecond)) {
		t.Fatal("cursor movement did not restart the visible phase")
	}
	// Input restarts the phase even when the application cursor hasn't moved yet.
	term.caretEpoch = time.Time{}
	now = now.Add(600 * time.Millisecond)
	if visible, _ := term.caretBlink(now, image.Pt(4, 2), true); !visible {
		t.Fatal("input did not show the caret immediately")
	}
}

func TestTerminalCaretInactiveDoesNotAnimate(t *testing.T) {
	term := &terminal{focused: true}
	now := time.Unix(100, 0)
	pos := image.Pt(3, 2)
	term.caretBlink(now, pos, true)
	if visible, next := term.caretBlink(now.Add(time.Second), pos, false); visible || !next.IsZero() {
		t.Fatal("hidden/scrolled/exited cursor scheduled animation")
	}
	if visible, _ := term.caretBlink(now.Add(1500*time.Millisecond), pos, true); !visible {
		t.Fatal("newly visible cursor started hidden")
	}
	term.focused = false
	if visible, next := term.caretBlink(now.Add(2*time.Second), pos, true); !visible || !next.IsZero() {
		t.Fatal("unfocused cursor should be steady without scheduling frames")
	}
	term.focused = true
	if visible, _ := term.caretBlink(now.Add(2500*time.Millisecond), pos, true); !visible {
		t.Fatal("refocused cursor started hidden")
	}
}
