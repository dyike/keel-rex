package ui

// Temporary startup trace: REX_STARTUP_TRACE=1.

import (
	"fmt"
	"os"
	"time"
)

var startupT0 = time.Now()
var startupOn = os.Getenv("REX_STARTUP_TRACE") != ""

func mark(name string) {
	if startupOn {
		fmt.Fprintf(os.Stderr, "startup %d %7.1f ms  %s\n", time.Now().UnixMicro(), float64(time.Since(startupT0).Microseconds())/1000, name)
	}
}
