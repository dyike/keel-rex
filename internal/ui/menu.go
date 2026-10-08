package ui

import (
	"log"
	"os"

	"github.com/dyike/keel/ui/window"
)

var applicationApp *app

func installApplicationMenu(a *app) {
	if os.Getenv("KEEL_HEADLESS") == "1" {
		return
	}
	applicationApp = a
	if err := window.SetApplicationMenu(a.applicationMenu()...); err != nil {
		log.Printf("application menu: %v", err)
	}
}

func (a *app) applicationMenu() []window.MenuItem {
	commands := map[string]appCommand{}
	for _, command := range a.commands() {
		commands[command.ID] = command
	}
	item := func(id, title string) window.MenuItem {
		shortcut := commands[id].Shortcut
		switch id {
		case "quit":
			shortcut = desktopChrome.shortcut("mod+q")
		case "palette":
			shortcut = desktopChrome.shortcut("mod+shift+p")
		}
		return window.MenuItem{ID: id, Title: title, Shortcut: shortcut, OnSelect: func() {
			if id == "quit" {
				if a.window != nil {
					a.window.Close()
				}
				return
			}
			if a.overlayOpen() {
				return
			}
			if id == "palette" {
				a.openPalette()
				return
			}
			a.runCommand(id)
		}}
	}
	edit := func(id, title, shortcut string, action window.MenuAction) window.MenuItem {
		// Keep Ctrl+C/Ctrl+A available to terminal programs on Windows/Linux.
		if !desktopChrome.trafficLights {
			shortcut = "ctrl+shift+" + shortcut[len("mod+"):]
		}
		return window.MenuItem{ID: id, Title: title, Shortcut: shortcut, Action: action}
	}
	menus := []window.MenuItem{
		window.MenuItem{Title: "Rex Keel", Role: window.MenuApplication, Children: []window.MenuItem{
			item("host", "About this host"), {Separator: true}, item("quit", "Quit Rex (keep sessions)"), item("end-all", "Quit and end all sessions…"),
		}},
		window.MenuItem{Title: "Shell", Children: []window.MenuItem{
			item("new-tab", "New tab"), item("directory", "Open directory…"), item("split-right", "Split right"), item("split-down", "Split down"), item("close-pane", "Close pane"), item("close-tab", "Close tab"), item("restart", "Restart shell…"),
		}},
		window.MenuItem{Title: "Edit", Role: window.MenuEdit, Children: []window.MenuItem{
			edit("copy", "Copy", "mod+c", window.MenuCopy), edit("paste", "Paste", "mod+v", window.MenuPaste), edit("select-all", "Select all", "mod+a", window.MenuSelectAll), item("search", "Find in terminal…"), item("clear", "Clear screen and scrollback"),
		}},
		window.MenuItem{Title: "View", Children: []window.MenuItem{
			item("palette", "Command palette…"), item("zoom", "Zoom / restore pane"), item("equalize", "Equalize panes"), item("larger", "Larger text"), item("smaller", "Smaller text"), item("actual-size", "Actual text size"), item("light", "Appearance: Light"), item("dark", "Appearance: Dark"), item("system", "Appearance: System"),
		}},
		window.MenuItem{Title: "Tabs", Children: []window.MenuItem{
			item("previous-tab", "Previous tab"), item("next-tab", "Next tab"), item("rename", "Rename tab…"),
		}},
	}
	if !desktopChrome.trafficLights {
		file := menus[1]
		file.Title = "File"
		file.Children = append(file.Children, window.MenuItem{Separator: true}, item("quit", "Exit (keep sessions)"), item("end-all", "Exit and end all sessions…"))
		menus = append([]window.MenuItem{file}, menus[2:]...)
		menus = append(menus, window.MenuItem{Title: "Help", Role: window.MenuHelp, Children: []window.MenuItem{item("host", "Host and session information…")}})
	}
	return menus
}
