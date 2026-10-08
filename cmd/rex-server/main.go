// rex-server runs the session backend without linking the desktop UI.
package main

import (
	"flag"
	"github.com/dyike/keel-rex/internal/backend"
	"log"
	"runtime/debug"
)

func main() {
	dir := flag.String("state-dir", "", "Session data directory")
	end := flag.Bool("end-sessions", false, "End sessions and stop the service")
	flag.Bool("server", true, "Run session service (accepted for desktop compatibility)")
	flag.Parse()
	if *dir == "" {
		*dir = backend.StateDirectory()
	}
	service := backend.NewService(*dir)
	if *end {
		if err := service.Shutdown(); err != nil {
			log.Fatal(err)
		}
		return
	}
	debug.SetMemoryLimit(128 << 20)
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}
}
