package vt

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

func TestPackedLinksBelongToRetainedLines(t *testing.T) {
	s := NewScrollback(100)
	s.SetMaxBytes(4096)
	done := s.direct()
	for i := 0; i < 10000; i++ {
		link := uv.Link{URL: fmt.Sprintf("https://example.test/%d", i), Params: fmt.Sprintf("id=%d", i)}
		s.appendCells(uv.Line{
			{Content: "x", Width: 1, Link: link},
			{Content: "y", Width: 1, Link: link, Style: uv.Style{Fg: color.NRGBA{R: 100, A: 255}}},
		})
	}
	done()
	if s.bytes > 4096 || s.Len() >= 100 {
		t.Fatalf("history exceeded byte budget: lines=%d bytes=%d", s.Len(), s.bytes)
	}
	for i := 0; i < s.n; i++ {
		p := s.at(i)
		if len(p.links) != 1 {
			t.Fatalf("line has %d link entries; equal links in different style runs should share one", len(p.links))
		}
		want := 10000 - s.n + i
		for _, c := range s.Line(i) {
			if c.Link.URL != fmt.Sprintf("https://example.test/%d", want) || c.Link.Params != fmt.Sprintf("id=%d", want) {
				t.Fatalf("line %d lost its link: %+v", i, c.Link)
			}
		}
	}
}

func TestPackedLinkBytesCountTowardsBudget(t *testing.T) {
	s := NewScrollback(100)
	s.SetMaxBytes(4096)
	done := s.direct()
	for i := 0; i < 10; i++ {
		s.appendText([]byte("x"), uv.Style{}, uv.Link{URL: strings.Repeat("u", 8192), Params: fmt.Sprint(i)})
	}
	done()
	// One oversized newest line is allowed, as with an oversized text line.
	if s.Len() != 1 || s.bytes < 8192 {
		t.Fatalf("link strings escaped the budget: lines=%d bytes=%d", s.Len(), s.bytes)
	}
}

func TestPackedLinksKeepParametersAndFallbackStyles(t *testing.T) {
	s := NewScrollback(10)
	line := uv.Line{
		{Content: "a", Width: 1, Link: uv.Link{URL: "https://example.test", Params: "id=first"}},
		{Content: "b", Width: 1, Link: uv.Link{URL: "https://example.test", Params: "id=second"}},
		// Gray16 is intentionally outside the compact color representation.
		{Content: "c", Width: 1, Link: uv.Link{URL: "https://example.test/custom"}, Style: uv.Style{Fg: color.Gray16{Y: 12345}}},
		{Content: "d", Width: 1},
	}
	s.Push(line)
	s.direct()()
	got := s.Line(0)
	for i := range line {
		if !got[i].Equal(&line[i]) {
			t.Fatalf("cell %d changed after packing: got %+v want %+v", i, got[i], line[i])
		}
	}
}

func TestPackedLinksSurviveAsyncPackingAndResize(t *testing.T) {
	s := NewScrollback(300)
	for i := 0; i < 3000; i++ {
		s.Push(uv.Line{{Content: fmt.Sprint(i), Width: 1, Link: uv.Link{URL: fmt.Sprintf("https://example.test/%d", i)}}})
	}
	s.SetMaxLines(17)
	s.direct()()
	if s.Len() != 17 {
		t.Fatalf("retained %d lines after shrink", s.Len())
	}
	for i, line := range s.Lines() {
		want := 3000 - 17 + i
		if len(line) != 1 || line[0].Content != fmt.Sprint(want) || line[0].Link.URL != fmt.Sprintf("https://example.test/%d", want) {
			t.Fatalf("line %d corrupted after packing/shrink: %+v", i, line)
		}
	}
	s.Clear()
	if s.Len() != 0 || s.free != nil || s.cachedLine != nil {
		t.Fatal("clear retained raw or cached lines")
	}
	s.Push(uv.Line{{Content: "new", Width: 1, Link: uv.Link{URL: "https://example.test/new"}}})
	s.direct()()
	if got := s.Line(0)[0].Link.URL; got != "https://example.test/new" {
		t.Fatalf("clear/reuse returned old link %q", got)
	}
}

func BenchmarkPackedScrollbackUniqueLinks(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s := NewScrollback(100)
		s.SetMaxBytes(4096)
		done := s.direct()
		for j := 0; j < 10000; j++ {
			s.appendText([]byte("linked output"), uv.Style{}, uv.Link{URL: fmt.Sprintf("https://example.test/%d", j)})
		}
		done()
	}
}
