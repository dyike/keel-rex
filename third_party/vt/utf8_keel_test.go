package vt

import (
	"math/rand"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Two simple characters, or one beside ASCII, are two clusters, each of the
// width classify gives it.
func TestSimpleRunesSegmentAlone(t *testing.T) {
	var simple []rune
	for r := rune(0xA0); r < 0x20000; r++ {
		if utf8.ValidRune(r) && classify(r).simple {
			simple = append(simple, r)
		}
	}
	if len(simple) < 1000 {
		t.Fatalf("only %d simple characters", len(simple))
	}
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 300000; i++ {
		a, b := simple[rnd.Intn(len(simple))], simple[rnd.Intn(len(simple))]
		s := string(a) + string(b)
		cluster, w := ansi.FirstGraphemeCluster([]byte(s), ansi.GraphemeWidth)
		if string(cluster) != string(a) || w != classify(a).w {
			t.Fatalf("U+%04X U+%04X: first cluster %q width %d, want %q width %d", a, b, cluster, w, string(a), classify(a).w)
		}
	}
	for _, r := range []rune{'中', '─', '🚀', '✅', 'é'} {
		if !classify(r).simple {
			t.Errorf("%q should be simple", r)
		}
	}
	for _, r := range []rune{0x0301, 0x200D, 0xFE0F, '한', 0x1F1E8} {
		if classify(r).simple {
			t.Errorf("U+%04X should not be simple", r)
		}
	}
}

// A character followed by a mark still forms one cluster.
func TestSimpleRunesKeepCombiningMarks(t *testing.T) {
	e := NewEmulator(10, 2)
	e.WriteString("é́中️")
	if c := e.CellAt(0, 0); c.Content != "é́" {
		t.Fatalf("cell 0 = %q", c.Content)
	}
	if c := e.CellAt(1, 0); c.Content != "中️" {
		t.Fatalf("cell 1 = %q", c.Content)
	}
}
