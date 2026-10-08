package ui

import (
	"runtime"

	"github.com/dyike/keel/ui/window"
)

type platformChrome struct {
	trafficLights bool
	hostLeft      float32
	paletteIcon   string
}

var desktopChrome = chromeForPlatform(runtime.GOOS)

func chromeForPlatform(platform string) platformChrome {
	if platform == "darwin" {
		return platformChrome{trafficLights: true, hostLeft: 89, paletteIcon: "command"}
	}
	if platform == "windows" {
		return platformChrome{hostLeft: 8, paletteIcon: "windows"}
	}
	return platformChrome{hostLeft: 8, paletteIcon: "search"}
}

func (c platformChrome) tabsLeft() float32 { return c.hostLeft + 113 }

func (c platformChrome) windowOptions(options window.Options) window.Options {
	// AppKit overlays its native buttons on the tab bar. Other desktops use
	// the system title bar for window actions, dragging and resizing.
	options.Frameless = c.trafficLights
	options.NativeTrafficLights = c.trafficLights
	if c.trafficLights {
		options.TrafficLightLayout = &window.TrafficLightLayout{Height: 44, Left: 15, Spacing: 23}
	} else {
		options.TrafficLightLayout = nil
	}
	return options
}
