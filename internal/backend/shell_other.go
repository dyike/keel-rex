//go:build !darwin && !windows

package backend

import "os"

func defaultShell() string { return os.Getenv("SHELL") }
