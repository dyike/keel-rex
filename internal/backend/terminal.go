package backend

import (
	"io"
	"path/filepath"
	"strings"
)

type terminalPTY interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Wait() error
	Hangup()
	Kill()
	ForegroundPID() int
}

func programName(path string) string {
	name := strings.TrimPrefix(filepath.Base(path), "-")
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		name = strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	}
	return name
}
