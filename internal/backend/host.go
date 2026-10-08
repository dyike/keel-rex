package backend

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ServerStats struct {
	Started       time.Time
	Open, Running int
}
type HostStatus struct {
	Connected, Loaded, Local, WindowOnly bool
	PID                                  int
	Uptime                               string
	Open, Running                        int
}

func sessionActivity(s *session) (open, running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exited, closed, program := s.exited, s.closed, s.program
	if s.remote != nil {
		exited, program = s.frame.Exited, s.frame.Program
	}
	open = !exited && !closed
	running = open && program != "" && !IsShellProgram(program)
	return
}

func (srv *sessionServer) stats() ServerStats {
	srv.mu.Lock()
	sessions := make([]*session, 0, len(srv.sessions))
	for _, s := range srv.sessions {
		sessions = append(sessions, s)
	}
	started := srv.started
	srv.mu.Unlock()
	stats := ServerStats{Started: started}
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

func readHostStatus(dir string, sessions []Session, started time.Time) HostStatus {
	status := HostStatus{Loaded: true, Local: dir == "", WindowOnly: dir != ""}
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
		open, running := s.Activity()
		if open {
			status.Open++
		}
		if running {
			status.Running++
		}
	}
	return status
}
