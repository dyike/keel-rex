package backend

import (
	"bufio"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

func TestSessionAppearanceUpdatesQueriesAndNotifies(t *testing.T) {
	s := &session{emu: vt.NewEmulator(24, 2), revision: 1}
	t.Cleanup(func() { s.emu.Close() })
	notifications := s.subscribe()
	t.Cleanup(func() { s.unsubscribe(notifications) })
	for _, appearance := range []string{"dark", "light", "dark"} {
		previous := s.revision
		s.syncAppearance(appearance)
		select {
		case <-notifications:
		case <-time.After(time.Second):
			t.Fatal("appearance change did not wake attached windows")
		}
		if s.revision <= previous {
			t.Fatal("appearance change did not advance the remote snapshot")
		}
		expected := rgb(0xf4f4f1)
		queryValue := "f4f4/f4f4/f1f1"
		if appearance == "dark" {
			expected = rgb(0x24272a)
			queryValue = "2424/2727/2a2a"
		}
		if actual := color.NRGBAModel.Convert(s.emu.BackgroundColor()).(color.NRGBA); actual != expected {
			t.Fatalf("%s background: %v", appearance, actual)
		}
		response := make(chan string, 1)
		go func() {
			data, _ := bufio.NewReader(s.emu).ReadString('\a')
			response <- data
		}()
		s.emu.WriteString("\x1b]11;?\x1b\\")
		select {
		case data := <-response:
			if !strings.Contains(data, queryValue) {
				t.Fatalf("stale OSC 11 background response: %q", data)
			}
		case <-time.After(time.Second):
			t.Fatal("no terminal background response")
		}
		s.syncAppearance(appearance)
		if s.revision != previous+1 {
			t.Fatal("unchanged appearance generated another update")
		}
	}
}
