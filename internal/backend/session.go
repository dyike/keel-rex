package backend

import (
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type session struct {
	appearance string
	remote     *remoteSession
	frame      Frame
	// A remote session's screen: the cells of the last update, a version per
	// row that changes with the row, and why the connection ended.
	grid        []uv.Cell
	rowVersions []uint64
	gridVersion uint64
	lost        string
	// localVersion counts the row versions syncRows gives a local session.
	localVersion   uint64
	revision       uint64
	lastOutput     time.Time
	processChecked time.Time
	bells          int
	program        string

	input                         *ptyInput
	watchMu                       sync.Mutex
	watchers                      map[chan struct{}]struct{}
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
	return newSessionWithOptions(SessionOptions{Directory: dir, Command: command})
}

func newSessionWithOptions(options SessionOptions) (*session, error) {
	dir, command := options.Directory, options.Command
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	s := &session{cwd: dir, title: filepath.Base(shell), cols: 80, rows: 24, cursorVisible: true, revision: 1, done: make(chan struct{}), inputDone: make(chan struct{})}
	s.emu = vt.NewEmulator(80, 24)
	// History is kept to 10000 lines, and to 8 MB packed: richly colored
	// output takes more per line.
	history := options.HistoryLines
	if history <= 0 {
		history = 10000
	}
	s.emu.SetScrollbackSize(history)
	s.emu.Scrollback().SetMaxBytes(8 << 20)
	s.emu.SetDefaultForegroundColor(rgb(0x272d30))
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
	// The command's identity is available before the first foreground query;
	// an explicit bash session must not initially inherit SHELL's zsh label.
	s.program = strings.TrimPrefix(filepath.Base(s.cmd.Path), "-")
	s.title = s.program
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
		// One goroutine reads the PTY, this one writes to the emulator what
		// has arrived since it last looked, all at once: a PTY read on macOS
		// is at most 1 KB, and the emulator scrolls past a screenful of
		// lines in one go when it has them together.
		chunks := make(chan []byte, 64)
		free := make(chan []byte, 64)
		go func() {
			defer close(chunks)
			for {
				var b []byte
				select {
				case b = <-free:
				default:
					b = make([]byte, 16<<10)
				}
				n, e := m.Read(b[:cap(b)])
				if n > 0 {
					chunks <- b[:n]
				}
				if e != nil {
					return
				}
			}
		}()
		var batch []byte
		for c := range chunks {
			batch = append(batch[:0], c...)
			recycle(free, c)
		more:
			for len(batch) < 256<<10 {
				select {
				case c, ok := <-chunks:
					if !ok {
						break more
					}
					batch = append(batch, c...)
					recycle(free, c)
				default:
					break more
				}
			}
			s.mu.Lock()
			s.emu.Write(batch)
			s.revision++
			s.lastOutput = time.Now()
			s.mu.Unlock()
			s.notify()
			if cap(batch) > 512<<10 {
				batch = nil
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
	s.changed()
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
		var b strings.Builder
		for y := 0; y < len(s.rowVersions) && s.frame.Cols > 0; y++ {
			b.WriteString(RowText(s.grid[y*s.frame.Cols : (y+1)*s.frame.Cols]))
			b.WriteByte('\n')
		}
		if s.lost != "" {
			b.WriteString(s.lost + "\n")
		}
		return b.String()
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

// subscribe returns a channel that receives when the screen changes, for a
// window watching the session; unsubscribe releases it.
func (s *session) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	s.watchMu.Lock()
	if s.watchers == nil {
		s.watchers = map[chan struct{}]struct{}{}
	}
	s.watchers[ch] = struct{}{}
	s.watchMu.Unlock()
	return ch
}

func (s *session) unsubscribe(ch chan struct{}) {
	s.watchMu.Lock()
	delete(s.watchers, ch)
	s.watchMu.Unlock()
}

// changed tells the windows watching the session that its screen changed.
func (s *session) changed() {
	s.watchMu.Lock()
	for ch := range s.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.watchMu.Unlock()
}

func (s *session) notify() { s.changed() }

// recycle hands a read buffer back to the PTY reader, or drops it when the
// reader holds enough.
func recycle(free chan []byte, b []byte) {
	select {
	case free <- b:
	default:
	}
}

// RowText is the text of a row of cells, without trailing spaces.
func RowText(row []uv.Cell) string {
	var b strings.Builder
	for i := range row {
		if row[i].Width > 0 {
			b.WriteString(row[i].Content)
		}
	}
	return strings.TrimRight(b.String(), " ")
}
