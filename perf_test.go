package main

import (
	"os"
	"path/filepath"
	"testing"
)

// benchData returns a workload file from tools/bench/data (python3 tools/bench/gen_data.py).
func benchData(b *testing.B, name string) string {
	p, _ := filepath.Abs(filepath.Join("tools", "bench", "data", name+".txt"))
	st, e := os.Stat(p)
	if e != nil {
		b.Skip("run python3 tools/bench/gen_data.py first")
	}
	b.SetBytes(st.Size())
	return p
}

func benchSessionCat(b *testing.B, name string) {
	p := benchData(b, name)
	for i := 0; i < b.N; i++ {
		s, e := newSession(b.TempDir(), "/bin/cat", p)
		if e != nil {
			b.Fatal(e)
		}
		s.resize(127, 31)
		<-s.done
	}
}

func BenchmarkSessionPlain(b *testing.B)  { benchSessionCat(b, "plain") }
func BenchmarkSessionANSI(b *testing.B)   { benchSessionCat(b, "ansi") }
func BenchmarkSessionCJK(b *testing.B)    { benchSessionCat(b, "cjk") }
func BenchmarkSessionFrames(b *testing.B) { benchSessionCat(b, "frames") }
