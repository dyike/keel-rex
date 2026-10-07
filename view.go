package main

import (
	"gioui.org/io/key"
	"github.com/dyike/keel/ui/core"
)

// Custom terminal widgets use their own Gio focus tags. Apply requests after
// el finishes closing overlays and restoring its built-in editor focus.
type workspaceView struct {
	app  *app
	root core.Widget
}

func (v *workspaceView) FillsWindow() bool { return true }

func (v *workspaceView) Layout(gtx core.C) core.D {
	dims := v.root.Layout(gtx)
	a := v.app
	if !a.overlayOpen() && a.focused != nil && a.focused.term != nil {
		t := a.focused.term
		if t.initialFocus && t.inputEnabled && t.layoutTime == gtx.Now {
			gtx.Execute(key.FocusCmd{Tag: t})
			t.initialFocus = false
		}
	}
	return dims
}
