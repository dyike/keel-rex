package ui

import (
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/dyike/keel-rex/assets"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"image"
	"image/color"
	"strings"
)

// All glyphs share a 24-unit grid, rounded ends and optical padding.
// Program tiles add a separate frame; glyphs never contain bitmap backplates.
func (p painter) disk(x, y, r float32, c color.NRGBA) {
	const k = float32(.55228475)
	var path clip.Path
	path.Begin(p.gtx.Ops)
	pt := func(a, b float32) f32.Point { return f32.Pt(a*p.scale, b*p.scale) }
	path.MoveTo(pt(x+r, y))
	path.CubeTo(pt(x+r, y+k*r), pt(x+k*r, y+r), pt(x, y+r))
	path.CubeTo(pt(x-k*r, y+r), pt(x-r, y+k*r), pt(x-r, y))
	path.CubeTo(pt(x-r, y-k*r), pt(x-k*r, y-r), pt(x, y-r))
	path.CubeTo(pt(x+k*r, y-r), pt(x+r, y-k*r), pt(x+r, y))
	path.Close()
	paint.FillShape(p.gtx.Ops, c, clip.Outline{Path: path.End()}.Op())
}

type iconKey struct {
	Name  string
	Size  float32
	Color color.NRGBA
}

var iconViews = map[iconKey]*el.RootWidget{}
var iconRevision uint64

func iconName(name string) string {
	aliases := map[string]string{
		"split-v": "columns-2", "split-h": "rows-2", "expand": "maximize-2",
		"restore": "minimize-2", "close": "x", "device": "mac-studio",
		"fish": "square-terminal", "shell": "square-terminal", "git": "plus-minus-circle",
		"codex": "brand:openai", "claude": "brand:claude", "node": "node-hex",
		"bun": "brand:bun", "python": "brand:python", "editor": "brand:neovim",
		"remote": "globe", "code": "file-code", "agent": "bot",
	}
	if v := aliases[name]; v != "" {
		return v
	}
	return name
}
func (p painter) glyph(name string, x, y, size float32, c color.NRGBA) {
	if rev := theme.Revision(); iconRevision != rev {
		clear(iconViews)
		iconRevision = rev
	}
	name = iconName(name)
	key := iconKey{name, size, c}
	view := iconViews[key]
	if view == nil {
		file := "assets/icons/" + name + ".svg"
		if brand, ok := strings.CutPrefix(name, "brand:"); ok {
			file = "assets/brands/" + brand + ".svg"
		}
		data, err := assets.Icons.ReadFile(strings.TrimPrefix(file, "assets/"))
		if err != nil {
			panic(err)
		}
		icon, err := kit.SVGIcon(data)
		if err != nil {
			panic(err)
		}
		view = el.Embed(icon.Size(size).Color(c))
		iconViews[key] = view
	}
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(x*p.scale, y*p.scale))).Push(p.gtx.Ops).Pop()
	gtx := p.gtx
	px := gtx.Dp(unit.Dp(size))
	gtx.Constraints = layout.Exact(image.Pt(px, px))
	view.Layout(gtx)
}

func iconElement(name string, size float32) el.Element {
	return el.Widget(core.Func(func(gtx core.C) core.D {
		p := painter{gtx, gtx.Metric.PxPerDp}
		p.glyph(name, 0, 0, size, theme.Muted)
		return core.D{Size: image.Pt(gtx.Dp(unit.Dp(size)), gtx.Dp(unit.Dp(size)))}
	})).Size(el.Dp(size)).NoShrink()
}

// Labels stay on their controls; glyph names are shared by toolbar and menus.
func actionGlyph(label string) string {
	label = strings.ToLower(label)
	switch {
	case strings.Contains(label, "split right"), strings.Contains(label, "split vertically"):
		return "split-v"
	case strings.Contains(label, "split down"), strings.Contains(label, "split horizontally"):
		return "split-h"
	case strings.Contains(label, "close"), strings.Contains(label, "cancel"), strings.Contains(label, "end session"), strings.Contains(label, "quit"):
		return "close"
	case strings.Contains(label, "zoom"), strings.Contains(label, "restore"):
		return "expand"
	case strings.Contains(label, "rename"), strings.Contains(label, "save name"):
		return "pencil"
	case strings.Contains(label, "move tab"), strings.Contains(label, "next tab"), strings.Contains(label, "previous tab"):
		return "layout-grid"
	case strings.Contains(label, "directory"):
		return "folder-open"
	case strings.Contains(label, "find"):
		return "search"
	case strings.Contains(label, "refresh"), strings.Contains(label, "restart"):
		return "rotate-ccw"
	case strings.Contains(label, "commit"):
		return "circle-dot"
	case strings.Contains(label, "unstage"):
		return "rotate-ccw"
	case strings.Contains(label, "stage all"):
		return "diff"
	case strings.Contains(label, "stage"):
		return "check"
	case strings.Contains(label, "new tab"):
		return "plus"
	case strings.Contains(label, "git"):
		return "git-branch"
	case strings.Contains(label, "codex"):
		return "codex"
	case strings.Contains(label, "host"):
		return "device"
	case strings.Contains(label, "clear"):
		return "trash-2"
	case strings.Contains(label, "text size"), strings.Contains(label, "terminal text"):
		return "keyboard"
	case strings.Contains(label, "appearance"), strings.Contains(label, "notifications"):
		return "settings-2"
	default:
		return "square-terminal"
	}
}

func (p painter) programTile(name string, x, y, w, h float32) {
	bg, fg := rgb(0x1d2420), rgb(0x5fd38d)
	glyph := name
	if name == "fish" || name == "shell" {
		glyph = "terminal"
	}
	switch name {
	case "codex":
		bg, fg = rgb(0xf7f7f7), rgb(0x1a1a1a)
	case "claude":
		bg, fg = rgb(0xd97757), rgb(0xffffff)
	case "node":
		bg, fg, glyph = rgb(0x5fa04e), rgb(0xffffff), "brand:nodedotjs"
	case "bun":
		bg, fg = rgb(0xfbf0df), rgb(0x3b2a20)
	case "python":
		bg, fg = rgb(0x3776ab), rgb(0xffd43b)
	case "git":
		bg, fg, glyph = rgb(0x3f8f4f), rgb(0xffffff), "diff"
	case "editor":
		bg, fg = rgb(0x2b7a3d), rgb(0xffffff)
	case "remote":
		bg, fg = rgb(0x5b4bd6), rgb(0xffffff)
	}
	p.rect(x-1.1, y-1.1, w+2.2, h+2.2, 5.6, theme.Border)
	p.rect(x-.65, y-.65, w+1.3, h+1.3, 5.1, color.NRGBA{R: 255, G: 255, B: 255, A: 225})
	p.rect(x, y, w, h, 4.5, bg)
	p.rect(x+3, y+1.1, w-6, .6, .3, color.NRGBA{R: 255, G: 255, B: 255, A: 46})
	size := min(float32(11), min(w, h)*.72)
	p.glyph(glyph, x+(w-size)/2, y+(h-size)/2, size, fg)
}

func (p painter) tiltedProgramTile(name string, x, y, w, h, angle float32) {
	center := f32.Pt((x+w/2)*p.scale, (y+h/2)*p.scale)
	defer op.Affine(f32.Affine2D{}.Rotate(center, angle)).Push(p.gtx.Ops).Pop()
	p.rect(x-.5, y+1, w+1, h+1, 5.5, color.NRGBA{A: 30})
	p.programTile(name, x, y, w, h)
}
func (p painter) symbol(name string, x, y, w, h float32) {
	switch name {
	case "fish", "shell", "git", "codex", "claude", "agent", "editor", "node", "bun", "python", "code", "remote":
		p.programTile(name, x, y, w, h)
	default:
		size := min(w, h)
		p.glyph(name, x+(w-size)/2, y+(h-size)/2, size, theme.Muted)
	}
}
