// Rex desktop launcher. The UI and session backend are independent packages.
package main

import (
	"flag"
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel-rex/internal/ui"
	"log"
	"runtime/debug"
)

func main() {
	options := ui.Options{}
	flag.StringVar(&options.Directory, "dir", "", "Initial working directory")
	flag.StringVar(&options.Screenshot, "screenshot", "", "Render native screenshot and exit")
	flag.StringVar(&options.StateDirectory, "state-dir", "", "Workspace/session data directory")
	flag.BoolVar(&options.Ephemeral, "ephemeral", false, "Use local sessions without persistence")
	flag.BoolVar(&options.Snake, "snake", false, "Run bundled Snake in the current terminal")
	server := flag.Bool("server", false, "Run persistent PTY session service")
	end := flag.Bool("end-sessions", false, "End all sessions, clear the saved workspace, and stop the service")
	flag.Parse()
	if *end && (*server || options.Ephemeral || options.Snake || options.Screenshot != "") {
		log.Fatal("-end-sessions cannot be combined with -server, -ephemeral, -snake, or -screenshot")
	}
	if *end || *server {
		dir := options.StateDirectory
		if dir == "" {
			dir = backend.StateDirectory()
		}
		service := backend.NewService(dir)
		if *end {
			if err := service.Shutdown(); err != nil {
				log.Fatal(err)
			}
			log.Print("all sessions ended; session service stopped")
		} else {
			debug.SetMemoryLimit(128 << 20)
			if err := service.Run(); err != nil {
				log.Fatal(err)
			}
		}
		return
	}
	debug.SetMemoryLimit(512 << 20)
	ui.Run(options)
}
