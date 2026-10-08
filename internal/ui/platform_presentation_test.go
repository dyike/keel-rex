package ui

import (
	"strings"
	"testing"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/window"
)

func TestDesktopPlatformMenusAndShortcuts(t *testing.T) {
	previousChrome := desktopChrome
	t.Cleanup(func() { desktopChrome = previousChrome })
	for _, platform := range []string{"darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			desktopChrome = chromeForPlatform(platform)
			a := &app{}
			commands := a.commands()
			bindings := map[string]string{}
			for _, command := range commands {
				if command.Shortcut == "" {
					continue
				}
				name, mods, err := core.ParseShortcut(command.Shortcut)
				if err != nil {
					t.Fatal(err)
				}
				binding := mods.String() + ":" + string(name)
				if previous := bindings[binding]; previous != "" {
					t.Fatalf("%s and %s share %s", previous, command.ID, binding)
				}
				bindings[binding] = command.ID
				if platform == "windows" && strings.ContainsAny(command.Hint, "⌘⌥⌃⇧") {
					t.Fatalf("Mac shortcut on Windows: %+v", command)
				}
			}
			menus := a.applicationMenu()
			if _, err := window.NewMenuBar(menus...); err != nil {
				t.Fatal(err)
			}
			if platform == "windows" {
				for i, title := range []string{"File", "Edit", "View", "Tabs", "Help"} {
					if menus[i].Title != title {
						t.Fatalf("menu %d: %q, want %q", i, menus[i].Title, title)
					}
				}
				if desktopChrome.hostLabel != "This PC" || iconName("device") != "monitor" {
					t.Fatal("Windows host identity is incorrect")
				}
				if hint := a.commandHint("zoom"); hint != "Ctrl+Shift+Enter" {
					t.Fatalf("context menu shortcut: %q", hint)
				}
			} else if menus[0].Role != window.MenuApplication || a.commandHint("new-tab") != "⌘T" {
				t.Fatal("macOS application menu or shortcut changed")
			}
		})
	}
}
