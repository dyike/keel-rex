package backend

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/x/vt"
)

func benchVT(b *testing.B, name string, scrollback int) { benchVTChunk(b, name, scrollback, 1024) }

func benchVTChunk(b *testing.B, name string, scrollback, chunk int) {
	data, _ := os.ReadFile(benchData(b, name))
	// What the program writes reaches the terminal through the PTY, whose
	// line discipline turns LF into CR LF.
	data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer() // a session makes its emulator once
		e := vt.NewEmulator(127, 31)
		e.Scrollback().SetMaxLines(scrollback)
		go io.Copy(io.Discard, e)
		b.StartTimer()
		for off := 0; off < len(data); off += chunk {
			e.Write(data[off:min(off+chunk, len(data))])
		}
		e.Close()
	}
}

func BenchmarkVTPlain(b *testing.B)       { benchVT(b, "plain", 10000) }
func BenchmarkVTPlainNoHist(b *testing.B) { benchVT(b, "plain", 0) }
func BenchmarkVTANSI(b *testing.B)        { benchVT(b, "ansi", 10000) }
func BenchmarkVTCJK(b *testing.B)         { benchVT(b, "cjk", 10000) }
func BenchmarkVTFrames(b *testing.B)      { benchVT(b, "frames", 10000) }

func BenchmarkVTPlain32K(b *testing.B) { benchVTChunk(b, "plain", 10000, 32<<10) }
func BenchmarkVTCJK32K(b *testing.B)   { benchVTChunk(b, "cjk", 10000, 32<<10) }
