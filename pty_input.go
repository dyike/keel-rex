package main

import (
	"io"
	"os"
	"sync"
)

// Drain the emulator's input immediately, even when the foreground process is
// not reading its TTY. PTY backpressure must never hold the UI's emulator lock.
type ptyInput struct {
	mu     sync.Mutex
	ready  *sync.Cond
	queue  [][]byte
	closed bool
	file   *os.File
}

func newPTYInput(f *os.File) *ptyInput {
	p := &ptyInput{file: f}
	p.ready = sync.NewCond(&p.mu)
	return p
}
func (p *ptyInput) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	p.queue = append(p.queue, append([]byte(nil), b...))
	p.ready.Signal()
	return len(b), nil
}
func (p *ptyInput) close() {
	p.mu.Lock()
	p.closed = true
	p.queue = nil
	p.ready.Broadcast()
	p.mu.Unlock()
}
func (p *ptyInput) pump() {
	defer p.close()
	for {
		p.mu.Lock()
		for len(p.queue) == 0 && !p.closed {
			p.ready.Wait()
		}
		if p.closed {
			p.mu.Unlock()
			return
		}
		b := p.queue[0]
		p.queue[0] = nil
		p.queue = p.queue[1:]
		p.mu.Unlock()
		for len(b) > 0 {
			n, e := p.file.Write(b)
			if e != nil {
				return
			}
			b = b[n:]
		}
	}
}
