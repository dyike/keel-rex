package main

import (
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/dyike/keel/ui/core"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type session struct {
	appearance     string
	remote         *remoteSession
	frame          screenFrame
	revision       uint64
	lastOutput     time.Time
	processChecked time.Time
	bells          int
	program        string

	input                         *ptyInput
	notifyPending                 atomic.Bool
	mouseTracking                 bool
	inputDone                     chan struct{}
	mu                            sync.Mutex
	emu                           *vt.Emulator
	master                        *os.File
	cmd                           *exec.Cmd
	cwd, title                    string
	cols, rows                    int
	exited, closed, cursorVisible bool
	done                          chan struct{}
}

func newSession(dir string, command ...string) (*session, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	s := &session{cwd: dir, title: filepath.Base(shell), cols: 80, rows: 24, cursorVisible: true, revision: 1, done: make(chan struct{}), inputDone: make(chan struct{})}
	s.emu = vt.NewEmulator(80, 24)
	s.emu.SetScrollbackSize(10000)
	s.emu.SetDefaultForegroundColor(ink)
	s.emu.SetDefaultBackgroundColor(rgb(0xf4f4f4))
	s.emu.SetCallbacks(vt.Callbacks{Bell: func() { s.bells++ }, Title: func(v string) { s.title = v }, WorkingDirectory: func(v string) {
		if parsed, e := url.Parse(v); e == nil && parsed.Scheme == "file" && parsed.Path != "" {
			s.cwd = parsed.Path
		}
	}, CursorVisibility: func(v bool) { s.cursorVisible = v }, EnableMode: func(m ansi.Mode) {
		if mouseMode(m) {
			s.mouseTracking = true
		}
	}, DisableMode: func(m ansi.Mode) {
		if mouseMode(m) {
			s.mouseTracking = false
		}
	}})
	if len(command) > 0 {
		s.cmd = exec.Command(command[0], command[1:]...)
	} else {
		s.cmd = exec.Command(shell, "-l")
	}
	s.cmd.Dir = dir
	s.cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=RexKeel", "TERM_PROGRAM_VERSION=0.3.0")
	m, e := pty.StartWithSize(s.cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if e != nil {
		s.emu.Close()
		return nil, e
	}
	s.master = m
	s.input = newPTYInput(m)
	go s.input.pump()
	go func() { defer close(s.inputDone); io.Copy(s.input, s.emu) }()
	go func() {
		defer close(s.done)
		b := make([]byte, 32768)
		for {
			n, e := m.Read(b)
			if n > 0 {
				s.mu.Lock()
				s.emu.Write(b[:n])
				s.revision++
				s.lastOutput = time.Now()
				s.mu.Unlock()
				s.notify()
			}
			if e != nil {
				break
			}
		}
		e := s.cmd.Wait()
		s.mu.Lock()
		s.exited = true
		s.revision++
		if !s.closed {
			msg := "\r\n[Session ended]"
			if e != nil {
				msg = fmt.Sprintf("\r\n[Session ended: %v]", e)
			}
			s.emu.WriteString(msg)
		}
		s.emu.InputPipe().(io.Closer).Close()
		s.mu.Unlock()
		s.input.close()
		s.master.Close()
		<-s.inputDone
		s.mu.Lock()
		s.emu.Close()
		s.mu.Unlock()
		s.notify()
	}()
	return s, nil
}
func (s *session) send(k uv.KeyEvent) {
	if s.remote != nil {
		if press, ok := k.(uv.KeyPressEvent); ok {
			s.remote.enqueue(rpcRequest{Op: "key", Key: uv.Key(press)})
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && !s.exited {
		s.emu.SendKey(k)
	}
}
func (s *session) text(v string, paste bool) {
	if s.remote != nil {
		s.remote.enqueue(rpcRequest{Op: "text", Text: v, Paste: paste})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && !s.exited {
		if paste {
			s.emu.Paste(v)
		} else {
			s.emu.SendText(v)
		}
	}
}
func (s *session) resize(cols, rows int) {
	if s.remote != nil {
		s.mu.Lock()
		changed := cols != s.cols || rows != s.rows
		s.cols, s.rows = cols, rows
		s.mu.Unlock()
		if changed {
			s.remote.enqueue(rpcRequest{Op: "resize", Cols: cols, Rows: rows})
		}
		return
	}

	cols = max(2, cols)
	rows = max(2, rows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || cols == s.cols && rows == s.rows {
		return
	}
	s.cols, s.rows = cols, rows
	s.emu.Resize(cols, rows)
	s.revision++
	pty.Setsize(s.master, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (s *session) close() {
	if s.remote != nil {
		s.remote.finish(true)
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.emu.InputPipe().(io.Closer).Close()
	s.input.close()
	s.master.Close()
	pid := s.cmd.Process.Pid
	running := !s.exited
	s.mu.Unlock()
	if running {
		syscall.Kill(-pid, syscall.SIGHUP)
	}
	go func() { <-s.inputDone; s.mu.Lock(); s.emu.Close(); s.mu.Unlock() }()
	go func() {
		select {
		case <-s.done:
			return
		case <-time.After(time.Second):
			if !running {
				return
			}
			syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()
}
func (s *session) directory() string { s.mu.Lock(); defer s.mu.Unlock(); return s.cwd }
func (s *session) plain() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remote != nil {
		return s.frame.Plain
	}
	return s.emu.String()
}
func (s *session) detach() {
	if s.remote != nil {
		s.remote.finish(false)
	} else {
		s.close()
	}
}

func mouseMode(m ansi.Mode) bool {
	return m == ansi.ModeMouseX10 || m == ansi.ModeMouseNormal || m == ansi.ModeMouseHighlight || m == ansi.ModeMouseButtonEvent || m == ansi.ModeMouseAnyEvent
}

func (s *session) notify() {
	if s.notifyPending.CompareAndSwap(false, true) {
		core.Update(func() { s.notifyPending.Store(false) })
	}
}
