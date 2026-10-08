//go:build !windows

package backend

import (
	"github.com/dyike/keel/native/process"
	"os"
)

func foregroundPID(f *os.File) int    { pid, _ := process.ForegroundPID(f); return pid }
func processDirectory(pid int) string { dir, _ := process.Directory(pid); return dir }
