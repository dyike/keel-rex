package ui

import (
	"fmt"
	"github.com/dyike/keel-rex/internal/backend"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
)

func TestHostPanelVisualFixture(t *testing.T) {
	dir := os.Getenv("REX_HOST_QA_DIR")
	if dir == "" {
		t.Skip("host panel screenshots not requested")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	defer theme.Apply(theme.Light())
	for _, appearance := range []string{"light", "dark"} {
		if appearance == "dark" {
			theme.Apply(theme.Dark())
		} else {
			theme.Apply(theme.Light())
		}
		for _, state := range []string{"connected", "legacy", "local", "disconnected"} {
			a := &app{prefs: preferences{Appearance: appearance}, hostDetails: backend.HostDetails{Model: "Mac mini", Chip: "Apple M4", Memory: "24 GB", System: "macOS 27.0.1", User: "ityike"}, hostStatus: backend.HostStatus{Loaded: true, Connected: true, PID: 33602, Uptime: "16m", Open: 4, Running: 2}}
			switch state {
			case "legacy":
				a.hostStatus.WindowOnly = true
			case "local":
				a.hostStatus.Local = true
			case "disconnected":
				a.hostStatus.Connected = false
			}
			for _, scale := range []float32{1, 2} {
				root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().P(28).Bg(a.colors().top).Child(a.hostPanel(300)) }))
				name := fmt.Sprintf("host-%s-%s-%gx.png", appearance, state, scale)
				if err := window.ScreenshotAtScale(root, 356, 440, scale, filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
