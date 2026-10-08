package ui

import (
	"strings"
	"testing"

	"github.com/dyike/keel-rex/internal/backend"
)

func TestShellTitlesAndIdleStateUseActualProgram(t *testing.T) {
	for _, name := range []string{"bash", "zsh", "fish", "sh", "-bash", "-zsh", "/bin/dash"} {
		session := &modelSession{state: backend.Frame{Program: name, Title: "custom prompt title", Dir: "/tmp/project"}}
		p := &pane{term: &terminal{session: session}}
		tab := &workspace{root: p, focus: p, title: "custom", customTitle: true}
		a := &app{tabs: []*workspace{tab}, focused: p}
		if title := programTitle(session); title == "Shell" || !backend.IsShellProgram(title) {
			t.Fatalf("%q displayed as %q", name, title)
		}
		if paneRunning(p) {
			t.Fatalf("idle %q requires closing confirmation", name)
		}
		if label := a.tabLabel(tab); label != "custom" {
			t.Fatalf("lost custom label: %q", label)
		}
		tab.customTitle = false
		if label := a.tabLabel(tab); !strings.Contains(label, "project") || !strings.HasPrefix(label, programTitle(session)+" ") {
			t.Fatalf("missing program or directory: %q", label)
		}
	}
}

func TestBackgroundProgramReturningToShellRequestsAttention(t *testing.T) {
	s := &modelSession{state: backend.Frame{Program: "-bash"}}
	background := &pane{lastProgram: "codex", term: &terminal{session: s}}
	focused := &pane{}
	a := &app{focused: focused, tabs: []*workspace{{root: &pane{first: background, second: focused}}}}
	if !a.checkActivity() || !background.attention {
		t.Fatal("program completion in bash did not request attention or redraw")
	}
	background.attention = false
	background.lastProgram = "zsh"
	if a.checkActivity(); background.attention {
		t.Fatal("switching idle shells requested program-completion attention")
	}
}
