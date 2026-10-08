package ui

import "github.com/dyike/keel/ui/window"

func systemDark() bool              { return window.SystemAppearance() == window.AppearanceDark }
func nativeAppearance(value string) { _ = window.SetNativeAppearance(window.Appearance(value)) }
