package main

import (
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/x/vt"
)

func benchVT(b *testing.B, name string, scrollback int) {
	data, _ := os.ReadFile(benchData(b, name))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := vt.NewEmulator(127, 31)
		e.Scrollback().SetMaxLines(scrollback)
		go io.Copy(io.Discard, e)
		for off := 0; off < len(data); off += 1024 {
			e.Write(data[off:min(off+1024, len(data))])
		}
		e.Close()
	}
}

func BenchmarkVTPlain(b *testing.B)       { benchVT(b, "plain", 10000) }
func BenchmarkVTPlainNoHist(b *testing.B) { benchVT(b, "plain", 0) }
func BenchmarkVTANSI(b *testing.B)        { benchVT(b, "ansi", 10000) }
func BenchmarkVTCJK(b *testing.B)         { benchVT(b, "cjk", 10000) }
func BenchmarkVTFrames(b *testing.B)      { benchVT(b, "frames", 10000) }
