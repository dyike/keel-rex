package ui

import (
	"fmt"
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// Options contains presentation and workspace choices from the launcher.
type Options struct {
	Service                               backend.WorkspaceService
	Directory, Screenshot, StateDirectory string
	Ephemeral, Snake                      bool
}

// Run starts the native workspace. Session lifecycle is supplied by the backend service.
func Run(options Options) {
	dir, screenshot, stateDir := &options.Directory, &options.Screenshot, &options.StateDirectory
	ephemeral, snake := &options.Ephemeral, &options.Snake
	mark("flags")
	if *snake {
		runSnake()
		return
	}
	if *dir == "" {
		*dir, _ = os.Getwd()
		if *dir == "/" || *dir == "" {
			*dir, _ = os.UserHomeDir()
		}
	}
	abs, e := filepath.Abs(*dir)
	if e != nil {
		log.Fatal(e)
	}
	// macOS maps its system fonts and retains only the needed Chinese faces.
	// Windows prepares in-memory font data without legacy bitmap
	// strikes. Font preparation runs alongside session startup; installation
	// completes on the UI thread before its first frame.
	fonts := platformFontFiles()
	fontSet := make(chan func() error, 1)
	go func() {
		set, e := prepareAppFonts(runtime.GOOS, fonts)
		if e != nil {
			log.Fatal(e)
		}
		fontSet <- set
	}()
	useFonts := func() {
		if err := (<-fontSet)(); err != nil {
			log.Fatal(err)
		}
	}
	theme.Material.Face = uiFontFace
	mark("fonts")
	var a *app
	var workspace chan error
	if *ephemeral || *screenshot != "" {
		service := options.Service
		if service == nil {
			service = backend.NewService("")
		}
		a = newAppShellWithService(abs, "", service)
		a.openWorkspace()
	} else {
		if *stateDir == "" {
			*stateDir = backend.StateDirectory()
		}
		// The session server starts and the workspace opens while AppKit
		// starts and opens the window; the first frame waits for them.
		service := options.Service
		if service == nil {
			service = backend.NewService(*stateDir)
		}
		a = newAppShellWithService(abs, *stateDir, service)
		workspace = make(chan error, 1)
		go func() {
			e := a.backendService().Ensure()
			mark("server")
			if e == nil {
				a.openWorkspace()
			}
			mark("app")
			workspace <- e
		}()
	}
	a.applyAppearance()
	root := &workspaceView{app: a, root: el.Root(a)}
	if *screenshot != "" {
		useFonts()
		defer a.close()
		if e := window.ScreenshotAtScale(root, 1057, 639, 2, *screenshot); e != nil {
			log.Fatal(e)
		}
		return
	}
	a.window = window.Open(desktopChrome.windowOptions(window.Options{Title: fmt.Sprintf("Rex · %s", filepath.Base(abs)), Width: a.prefs.WindowWidth, Height: a.prefs.WindowHeight, MinWidth: 800, MinHeight: 480, OnResize: a.rememberWindowSize, Content: root, OnClose: a.close}))
	mark("window.Open")
	// The first frame waits for the fonts and the workspace, so it draws
	// with them.
	core.Update(func() {
		useFonts()
		if workspace != nil {
			if e := <-workspace; e != nil {
				log.Fatal(e)
			}
		}
	})
	installApplicationMenu(a)
	if a.prefs.Notifications {
		a.enableNotifications()
	}
	mark("main loop")
	window.Main()
}
