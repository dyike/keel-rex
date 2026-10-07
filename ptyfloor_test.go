package main

import (
	"os/exec"
	"testing"

	"github.com/creack/pty"
)

func BenchmarkPTYFloor(b *testing.B) {
	p := benchData(b, "plain")
	for i := 0; i < b.N; i++ {
		m, e := pty.StartWithSize(exec.Command("/bin/cat", p), &pty.Winsize{Cols: 127, Rows: 31})
		if e != nil {
			b.Fatal(e)
		}
		buf := make([]byte, 32768)
		reads, max := 0, 0
		for {
			n, e := m.Read(buf)
			reads++
			if n > max {
				max = n
			}
			if e != nil {
				break
			}
		}
		b.ReportMetric(float64(reads), "reads")
		b.ReportMetric(float64(max), "maxread")
	}
}
