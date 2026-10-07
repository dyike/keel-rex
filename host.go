package main

import (
	"fmt"
	"github.com/dyike/keel/ui/core"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type hostDetails struct{ Model, Chip, Memory, System, User string }
type serverStats struct {
	Started       time.Time
	Open, Running int
}
type hostStatus struct {
	Connected, Loaded, Local, WindowOnly bool
	PID                                  int
	Uptime                               string
	Open, Running                        int
}

func (a *app) loadHostInfo() {
	a.hostModel = runtime.GOOS
	a.hostDetails = hostDetails{Model: "Reading…", Chip: "Reading…", Memory: "Reading…", System: runtime.GOOS, User: os.Getenv("USER")}
	go func() {
		details := hostDetails{Model: runtime.GOARCH, Chip: runtime.GOARCH, Memory: "Unavailable", System: runtime.GOOS, User: os.Getenv("USER")}
		if u, err := user.Current(); err == nil {
			details.User = u.Username
		}
		model := runtime.GOARCH
		if runtime.GOOS == "darwin" {
			read := func(name string) string {
				b, _ := exec.Command("sysctl", "-n", name).Output()
				return strings.TrimSpace(string(b))
			}
			chip, hw := read("machdep.cpu.brand_string"), read("hw.model")
			memory, _ := strconv.ParseUint(read("hw.memsize"), 10, 64)
			version, _ := exec.Command("sw_vers", "-productVersion").Output()
			model = hw
			for _, family := range []string{"Macmini", "MacBookPro", "MacBookAir", "MacStudio", "iMac", "MacPro"} {
				if strings.HasPrefix(hw, family) {
					model = map[string]string{"Macmini": "Mac mini", "MacBookPro": "MacBook Pro", "MacBookAir": "MacBook Air", "MacStudio": "Mac Studio", "MacPro": "Mac Pro", "iMac": "iMac"}[family]
					break
				}
			}
			details.Model, details.Chip = model, chip
			if memory > 0 {
				details.Memory = fmt.Sprintf("%.0f GB", float64(memory)/(1<<30))
			}
			details.System = "macOS " + strings.TrimSpace(string(version))
		}
		core.Update(func() { a.hostDetails, a.hostModel = details, model })
	}()
}

func sessionActivity(s *session) (open, running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exited, closed, program := s.exited, s.closed, s.program
	if s.remote != nil {
		exited, program = s.frame.Exited, s.frame.Program
	}
	open = !exited && !closed
	running = open && program != "" && program != "zsh" && program != "bash" && program != "fish" && program != "sh" && program != "-zsh" && program != "-bash"
	return
}

func (srv *sessionServer) stats() serverStats {
	srv.mu.Lock()
	sessions := make([]*session, 0, len(srv.sessions))
	for _, s := range srv.sessions {
		sessions = append(sessions, s)
	}
	started := srv.started
	srv.mu.Unlock()
	stats := serverStats{Started: started}
	for _, s := range sessions {
		s.refreshProgram()
		open, running := sessionActivity(s)
		if open {
			stats.Open++
		}
		if running {
			stats.Running++
		}
	}
	return stats
}

func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", max(0, int(d.Seconds())))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

func readHostStatus(dir string, sessions []*session, started time.Time) hostStatus {
	status := hostStatus{Loaded: true, Local: dir == "", WindowOnly: dir != ""}
	if dir != "" {
		r, err := callServer(dir, rpcRequest{Op: "stats"})
		if err != nil {
			r, err = callServer(dir, rpcRequest{Op: "hello"})
			if err != nil {
				return status
			}
		}
		status.Connected, status.PID = true, r.PID
		if r.Stats != nil {
			status.Open, status.Running = r.Stats.Open, r.Stats.Running
			status.Uptime = formatUptime(time.Since(r.Stats.Started))
			status.WindowOnly = false
			return status
		}
		// Older servers keep running sessions; query their PID without restarting.
		if b, err := exec.Command("ps", "-p", strconv.Itoa(r.PID), "-o", "etime=").Output(); err == nil {
			status.Uptime = strings.TrimSpace(string(b))
		}
	} else {
		status.Connected = true
		status.PID = os.Getpid()
		status.Uptime = formatUptime(time.Since(started))
	}
	for _, s := range sessions {
		if s.remote == nil {
			s.refreshProgram()
		}
		open, running := sessionActivity(s)
		if open {
			status.Open++
		}
		if running {
			status.Running++
		}
	}
	return status
}

func (a *app) refreshHostStatus() {
	if a.hostStatusLoading || a.closed {
		return
	}
	a.hostStatusLoading = true
	var sessions []*session
	seen := map[*session]bool{}
	for _, tab := range a.tabs {
		tab.root.each(func(p *pane) {
			if p.term != nil && !seen[p.term.session] {
				seen[p.term.session] = true
				sessions = append(sessions, p.term.session)
			}
		})
	}
	dir, started := a.dataDir, a.started
	go func() {
		status := readHostStatus(dir, sessions, started)
		core.Update(func() {
			a.hostStatusLoading = false
			if !a.closed {
				a.hostStatus = status
			}
		})
	}()
}
func (a *app) toggleHostInfo() {
	a.hostOpen = !a.hostOpen
	if a.hostOpen {
		a.refreshHostStatus()
	}
}
