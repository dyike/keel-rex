package backend

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ConPTY has no Unix foreground process group. Inspect shell descendants
// instead; console host processes are implementation details, not programs.
func windowsProcesses() map[uint32]windows.ProcessEntry32 {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snapshot)
	entries := map[uint32]windows.ProcessEntry32{}
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &e); err == nil; err = windows.Process32Next(snapshot, &e) {
		entries[e.ProcessID] = e
	}
	return entries
}
func windowsForegroundPID(root int) int {
	return windowsForegroundProcess(windowsProcesses(), root)
}
func windowsForegroundProcess(entries map[uint32]windows.ProcessEntry32, root int) int {
	best, depth := uint32(root), 0
	for pid, entry := range entries {
		name := programName(windows.UTF16ToString(entry.ExeFile[:]))
		if name == "conhost" || name == "openconsole" {
			continue
		}
		parent, d := entry.ParentProcessID, 1
		for seen := map[uint32]bool{}; parent != 0 && !seen[parent]; d++ {
			if parent == uint32(root) {
				if d > depth || d == depth && pid > best {
					best, depth = pid, d
				}
				break
			}
			seen[parent] = true
			parent = entries[parent].ParentProcessID
		}
	}
	return int(best)
}
func (s *session) refreshProgram() {
	s.mu.Lock()
	if s.closed || s.exited || time.Since(s.processChecked) < 750*time.Millisecond {
		s.mu.Unlock()
		return
	}
	s.processChecked = time.Now()
	pid := s.cmd.Process.Pid
	s.mu.Unlock()
	entries := windowsProcesses()
	if e, ok := entries[uint32(windowsForegroundProcess(entries, pid))]; ok {
		s.mu.Lock()
		s.program = programName(windows.UTF16ToString(e.ExeFile[:]))
		s.mu.Unlock()
	}
}
func identifyRemoteCLI(_ int, name string) string { return name }
