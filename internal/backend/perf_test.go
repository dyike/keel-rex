package backend

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// benchData returns a workload file from tools/bench/data (python3 tools/bench/gen_data.py).
func benchData(b *testing.B, name string) string {
	p, _ := filepath.Abs(filepath.Join("..", "..", "tools", "bench", "data", name+".txt"))
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

// benchSessionWatched is benchSessionCat with a window watching, as the
// server's watch loop does, its updates thrown away.
func benchSessionWatched(b *testing.B, name string) {
	p := benchData(b, name)
	for i := 0; i < b.N; i++ {
		s, e := newSession(b.TempDir(), "/bin/cat", p)
		if e != nil {
			b.Fatal(e)
		}
		s.resize(127, 31)
		changed := s.subscribe()
		stop := make(chan struct{})
		go func() {
			var st watchState
			var screen screenCopy
			var scratch []byte
			var last time.Time
			for {
				s.mu.Lock()
				s.copyScreen(&screen, &st, 0)
				s.mu.Unlock()
				if encodeUpdate(&screen, &st, &scratch) != nil {
					last = time.Now()
				}
				select {
				case <-stop:
					return
				case <-changed:
					if wait := 8*time.Millisecond - time.Since(last); wait > 0 {
						time.Sleep(wait)
					}
				}
			}
		}()
		<-s.done
		close(stop)
		s.unsubscribe(changed)
	}
}

func BenchmarkWatchedPlain(b *testing.B)  { benchSessionWatched(b, "plain") }
func BenchmarkWatchedANSI(b *testing.B)   { benchSessionWatched(b, "ansi") }
func BenchmarkWatchedCJK(b *testing.B)    { benchSessionWatched(b, "cjk") }
func BenchmarkWatchedFrames(b *testing.B) { benchSessionWatched(b, "frames") }
