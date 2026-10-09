package ui

import (
	"runtime"

	"github.com/dyike/keel/ui/window"
)

type platformChrome struct {
	trafficLights  bool
	windowControls bool
	hostLeft       float32
	paletteIcon    string
	deviceIcon     string
	hostLabel      string
	systemName     string
	toolStroke     string
}

var desktopChrome = chromeForPlatform(runtime.GOOS)

func chromeForPlatform(platform string) platformChrome {
	if platform == "darwin" {
		return platformChrome{trafficLights: true, hostLeft: 89, paletteIcon: "command", deviceIcon: "mac-studio", hostLabel: "This Mac", systemName: "macOS"}
	}
	if platform == "windows" {
		return platformChrome{windowControls: true, hostLeft: 8, paletteIcon: "windows", deviceIcon: "monitor", hostLabel: "This PC", systemName: "Windows", toolStroke: "1.8"}
	}
	return platformChrome{hostLeft: 8, paletteIcon: "search", deviceIcon: "monitor", hostLabel: "This computer", systemName: "Linux"}
}

func (c platformChrome) hostWidth() float32 {
	if c.windowControls {
		return 160
	}
	return 110
}

func (c platformChrome) tabsLeft() float32 { return c.hostLeft + c.hostWidth() + 3 }

func (c platformChrome) toolbarRight(width float32) float32 {
	if c.windowControls {
		return width - 140
	}
	return width
}

func (c platformChrome) windowOptions(options window.Options) window.Options {
	// Both macOS and Windows integrate their window controls into the tab bar.
	options.Frameless = c.trafficLights || c.windowControls
	options.NativeTrafficLights = c.trafficLights
	if c.windowControls {
		options.MenuDisplay = window.MenuDisplayHidden
	}
	if c.trafficLights {
		options.TrafficLightLayout = &window.TrafficLightLayout{Height: 44, Left: 15, Spacing: 23}
	} else {
		options.TrafficLightLayout = nil
	}
	return options
}
