module github.com/dyike/keel-rex

go 1.26.1

require (
	gioui.org v0.10.3
	github.com/charmbracelet/ultraviolet v0.0.0-20260303162955-0b88c25f3fff
	github.com/charmbracelet/x/ansi v0.11.7
	github.com/charmbracelet/x/term v0.2.2
	github.com/charmbracelet/x/vt v0.0.0-20261004011457-ad85c59fdf4e
	github.com/creack/pty v1.1.24
	github.com/dyike/keel v0.1.6
	github.com/go-text/typesetting v0.3.5
	golang.org/x/image v0.46.0
)

require (
	gioui.org/shader v1.0.9 // indirect
	github.com/charmbracelet/colorprofile v0.4.2 // indirect
	github.com/charmbracelet/x/exp/ordered v0.1.0 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jezek/xgb v1.3.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-runewidth v0.0.23 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c // indirect
	github.com/srwiley/rasterx v0.0.0-20210519020934-456a8d69b780 // indirect
	github.com/tc-hib/winres v0.3.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/exp/shiny v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

// Local Keel checkout includes the native traffic-light layout API.

tool github.com/dyike/keel/cmd/keel

replace github.com/charmbracelet/x/vt => ./third_party/vt
