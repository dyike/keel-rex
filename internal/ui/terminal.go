package ui

import (
	"gioui.org/f32"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel-rex/internal/backend"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/theme"
	"image"
	"io"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type terminal struct {
	ime                    terminalIME
	session                backend.Session
	owner                  *app
	pane                   *pane
	scroll                 int
	scrollbar              widget.Scrollbar
	focused, selecting     bool
	anchor, caret          int
	viewStart              int
	viewFrame              backend.Frame
	viewReady              bool
	selectionPointer       f32.Point
	selectionScrollAt      time.Time
	extendSelection        bool
	pendingCopy            chan selectionCopy
	cols, rows             int
	cells                  []uv.Cell
	rowVersions            []uint64 // of the rows in cells, see session.syncRows
	rowPaints              []rowPaint
	plain                  string
	initialFocus           bool
	inputEnabled           bool
	layoutTime             time.Time
	caretEpoch             time.Time
	caretPosition          image.Point
	hasSelection           bool
	scrollRemainder        float32
	lastClick              time.Duration
	clickIndex, clickCount int
	searching              bool
	lastSearch             string
	searchAfter            int
}

const terminalSize float32 = 12.5
const cellWidth float32 = 7.526
const cellHeight float32 = 16
const terminalScrollbarWidth = 10

func (t *terminal) Layout(gtx core.C) core.D {
	t.flushCopy(gtx)
	size := gtx.Constraints.Max
	scale := gtx.Metric.PxPerDp
	if scale == 0 {
		scale = 1
	}
	w, h := float32(size.X)/scale, float32(size.Y)/scale
	w = max(0, w-terminalScrollbarWidth)
	fontSize := terminalSize
	if t.owner != nil && t.owner.prefs.FontSize > 0 {
		fontSize = t.owner.prefs.FontSize
	}
	cw, ch := fontSize*.60208, fontSize*1.28
	cols := max(2, int(w/cw))
	if t.cols != 0 && t.cols != cols {
		t.clearSelection()
	}
	t.cols = cols
	t.rows = max(2, int(h/ch))
	t.session.Resize(t.cols, t.rows)
	if t.owner != nil {
		appearance := "light"
		if t.owner.appearanceDark {
			appearance = "dark"
		}
		t.session.SetAppearance(appearance)
	}
	t.inputEnabled = gtx.Enabled()
	t.layoutTime = gtx.Now
	for {
		action, ok := core.NextEditAction(gtx, t)
		if !ok {
			break
		}
		switch action {
		case core.EditSelectAll:
			t.selectAll()
		case core.EditPaste:
			gtx.Execute(clipboard.ReadCmd{Tag: t})
		case core.EditCopy:
			t.copySelection()
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: t, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6}}, key.FocusFilter{Target: t}, key.Filter{Focus: t, Name: "", Optional: key.ModCtrl | key.ModCommand | key.ModShift | key.ModAlt | key.ModSuper}, key.Filter{Focus: t, Name: key.NameTab, Optional: key.ModShift}, transfer.TargetFilter{Target: t, Type: "application/text"})
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.FocusEvent, key.EditEvent, key.CompositionEvent, key.SelectionEvent, transfer.DataEvent:
			t.caretEpoch = time.Time{}
		case key.Event:
			if e.State == key.Press {
				t.caretEpoch = time.Time{}
			}
		case pointer.Event:
			if e.Kind == pointer.Press {
				t.caretEpoch = time.Time{}
			}
		}
		switch e := ev.(type) {
		case key.FocusEvent:
			t.focused = e.Focus
			if !e.Focus {
				t.ime = terminalIME{}
				// el briefly blurs its background before this widget requests
				// focus on a press. Pointer selection ends on Release/Cancel,
				// independently of that keyboard focus handoff.
			}
			if e.Focus {
				if t.owner.focused == t.pane {
					t.pane.attention = false
				}
			}
		case key.EditEvent:
			t.scroll = 0
			t.clearSelection()
			t.ime.edit(e)
		case key.CompositionEvent:
			t.ime.composing = e.Start >= 0
		case key.SelectionEvent:
			t.ime.selection = key.Range(e)
		case key.Event:
			if e.State != key.Press {
				continue
			}
			if t.ime.composing && !e.Modifiers.Contain(key.ModCommand) {
				continue
			}
			t.flushIME()
			// Printable spaces arrive through EditEvent, including IME commits.
			if e.Name == key.NameSpace && e.Modifiers&(key.ModCtrl|key.ModAlt|key.ModCommand) == 0 {
				continue
			}
			if e.Modifiers.Contain(key.ModCommand) && e.Name == "+" {
				t.owner.setFontSize(t.owner.prefs.FontSize + 1)
				continue
			}
			editModifier := e.Modifiers.Contain(key.ModCommand) || e.Modifiers.Contain(key.ModCtrl|key.ModShift)
			if editModifier && e.Name == "A" {
				t.selectAll()
				continue
			}
			if editModifier && e.Name == "V" {
				gtx.Execute(clipboard.ReadCmd{Tag: t})
				continue
			}
			if editModifier && e.Name == "C" {
				t.copySelection()
				continue
			}
			if e.Modifiers.Contain(key.ModCommand) {
				continue
			}
			k := terminalKey(e)
			if k.Code != 0 {
				t.scroll = 0
				t.clearSelection()
				t.session.SendKey(k)
			}
		case transfer.DataEvent:
			r := e.Open()
			b, _ := io.ReadAll(io.LimitReader(r, 16<<20))
			r.Close()
			t.scroll = 0
			t.clearSelection()
			t.session.SendText(string(b), true)
		case pointer.Event:
			s := t.session
			mouse := s.State().Mouse
			if mouse && !e.Modifiers.Contain(key.ModShift) && !t.selecting {
				gtx.Execute(key.FocusCmd{Tag: t})
				t.owner.focusPane(t.pane)
				m := uv.Mouse{X: max(0, int(e.Position.X/(cw*scale))), Y: max(0, int(e.Position.Y/(ch*scale))), Button: uv.MouseLeft}
				if e.Buttons.Contain(pointer.ButtonSecondary) {
					m.Button = uv.MouseRight
				}
				if e.Buttons.Contain(pointer.ButtonTertiary) {
					m.Button = uv.MouseMiddle
				}
				if e.Modifiers.Contain(key.ModCtrl) {
					m.Mod |= uv.ModCtrl
				}
				if e.Modifiers.Contain(key.ModAlt) {
					m.Mod |= uv.ModAlt
				}
				if e.Modifiers.Contain(key.ModShift) {
					m.Mod |= uv.ModShift
				}
				kind := ""
				switch e.Kind {
				case pointer.Press:
					kind = "press"
				case pointer.Release:
					kind = "release"
				case pointer.Drag:
					kind = "drag"
				case pointer.Scroll:
					kind = "scroll"
					if e.Scroll.Y < 0 {
						m.Button = uv.MouseWheelUp
					} else {
						m.Button = uv.MouseWheelDown
					}
				}
				s.SendMouse(kind, m)
				continue
			}
			pos := f32.Pt(e.Position.X/scale, e.Position.Y/scale)
			idx := t.selectionIndex(pos, cw, ch)
			switch e.Kind {
			case pointer.Press:
				gtx.Execute(key.FocusCmd{Tag: t})
				t.owner.focusPane(t.pane)
				if e.Buttons.Contain(pointer.ButtonSecondary) {
					t.owner.contextPane = t.pane
					continue
				}
				if e.Modifiers.Contain(key.ModShift) {
					t.caret = idx
					t.hasSelection = true
				} else {
					t.anchor, t.caret = idx, idx
					t.hasSelection = false
					if idx == t.clickIndex && e.Time-t.lastClick < 400*time.Millisecond {
						t.clickCount++
					} else {
						t.clickCount = 1
					}
					t.lastClick, t.clickIndex = e.Time, idx
					if t.clickCount == 2 {
						t.selectWord(idx)
					}
					if t.clickCount >= 3 {
						t.anchor = idx / t.cols * t.cols
						t.caret = t.anchor + t.cols - 1
						t.hasSelection = true
						t.clickCount = 0
					}
				}
				t.selecting = true
				t.selectionPointer = pos
				gtx.Execute(pointer.GrabCmd{Tag: t, ID: e.PointerID})
			case pointer.Drag:
				if t.selecting {
					t.selectionPointer = pos
					t.extendSelection = true
					t.caret = idx
					t.hasSelection = true
				}
			case pointer.Release:
				t.selecting = false
			case pointer.Cancel:
				t.selecting = false
			case pointer.Scroll:
				t.scrollRemainder += e.Scroll.Y / (ch * scale)
				delta := int(t.scrollRemainder)
				t.scrollRemainder -= float32(delta)
				t.scroll = max(0, t.scroll-delta)
				if t.selecting {
					t.selectionPointer = pos
				}
			}
		}
	}
	t.flushIME()
	t.autoScrollSelection(gtx, ch, h)
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	defer area.Pop()
	textSize := image.Pt(max(0, size.X-gtx.Dp(terminalScrollbarWidth)), size.Y)
	inputArea := clip.Rect(image.Rectangle{Max: textSize}).Push(gtx.Ops)
	event.Op(gtx.Ops, t)
	key.InputHintOp{Tag: t, Hint: key.HintAny}.Add(gtx.Ops)
	pointer.CursorText.Add(gtx.Ops)
	inputArea.Pop()
	s := t.session
	frame, changed := s.SyncRows(t.scroll, t.cols, t.rows, &t.cells, &t.rowVersions)
	if frame.Scroll == t.scroll {
		t.scroll = frame.Scroll
	} else {
		// The server frame can lag behind a wheel event. Keep the requested
		// position, but never request beyond the available history.
		t.scroll = min(t.scroll, frame.History)
	}
	if frame.Alt {
		t.scroll = 0
	}
	s.SetScroll(t.scroll)
	oldStart := t.viewStart
	t.syncSelection(frame)
	if t.selecting && (t.extendSelection || t.viewStart != oldStart) {
		t.caret = t.selectionIndex(t.selectionPointer, cw, ch)
		t.hasSelection = true
	}
	t.extendSelection = false
	if changed || t.plain == "" {
		t.plain = t.text()
	}
	pos := frame.Cursor
	visible := frame.CursorVisible && !frame.Exited
	p := painter{gtx, scale}
	t.paintRows(gtx, scale, cw, ch, fontSize)
	showCaret, nextBlink := t.caretBlink(gtx.Now, pos, visible && t.scroll == 0)
	if !nextBlink.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: nextBlink})
	}
	if showCaret && !(t.ime.composing && len(t.ime.text) > 0) {
		t.paintCaret(p, float32(pos.X)*cw, float32(pos.Y)*ch, ch)
	}
	t.layoutIME(gtx, p, frame.Cursor, cw, ch, textSize, showCaret)
	t.layoutScrollbar(gtx, frame)
	semantic.LabelOp(t.plain).Add(gtx.Ops)
	core.Role("text").Add(gtx.Ops)
	return core.D{Size: size}
}

func (t *terminal) layoutScrollbar(gtx core.C, frame backend.Frame) {
	if frame.Alt || frame.History <= 0 {
		return
	}
	total := float32(frame.History + t.rows)
	start := float32(frame.History-t.scroll) / total
	end := start + float32(t.rows)/total
	bar := material.Scrollbar(theme.Material, &t.scrollbar)
	bar.Indicator.MajorMinLen = 24
	color := gray
	if t.owner != nil {
		color = t.owner.colors().muted
	}
	color.A = 150
	bar.Indicator.Color = color
	color.A = 220
	bar.Indicator.HoverColor = color
	width := gtx.Dp(terminalScrollbarWidth)
	defer op.Offset(image.Pt(max(0, gtx.Constraints.Max.X-width), 0)).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(image.Pt(width, gtx.Constraints.Max.Y))
	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	pointer.CursorDefault.Add(gtx.Ops)
	bar.Layout(gtx, layout.Vertical, start, end)
	if delta := t.scrollbar.ScrollDistance(); delta != 0 {
		t.scroll = max(0, min(frame.History, t.scroll-int(math.Round(float64(delta*total)))))
		t.scrollRemainder = 0
		gtx.Execute(op.InvalidateCmd{})
	}
}

const terminalCaretWidth float32 = 2.5
const terminalCaretBlinkInterval = 500 * time.Millisecond

// Schedule only the next blink boundary; input and cursor motion restart the
// visible phase so typing never waits for a hidden caret to reappear.
func (t *terminal) caretBlink(now time.Time, position image.Point, visible bool) (bool, time.Time) {
	if !visible || !t.focused {
		t.caretEpoch = time.Time{}
		return visible, time.Time{}
	}
	if t.caretEpoch.IsZero() || position != t.caretPosition || now.Before(t.caretEpoch) {
		t.caretEpoch = now
		t.caretPosition = position
	}
	phase := now.Sub(t.caretEpoch) / terminalCaretBlinkInterval
	return phase%2 == 0, t.caretEpoch.Add((phase + 1) * terminalCaretBlinkInterval)
}

func (t *terminal) paintCaret(p painter, x, y, height float32) {
	c := rgb(0x5c6e78)
	if !t.focused {
		c.A = 90
	}
	p.rect(x, y, terminalCaretWidth, height, 0, c)
}

func terminalKey(e key.Event) uv.KeyPressEvent {
	k := uv.KeyPressEvent{}
	if e.Modifiers.Contain(key.ModCtrl) {
		k.Mod |= uv.ModCtrl
	}
	if e.Modifiers.Contain(key.ModAlt) {
		k.Mod |= uv.ModAlt
	}
	if e.Modifiers.Contain(key.ModShift) {
		k.Mod |= uv.ModShift
	}
	codes := map[key.Name]rune{key.NameReturn: uv.KeyEnter, key.NameEnter: uv.KeyEnter, key.NameEscape: uv.KeyEscape, key.NameTab: uv.KeyTab, key.NameSpace: uv.KeySpace, key.NameDeleteBackward: uv.KeyBackspace, key.NameDeleteForward: uv.KeyDelete, key.NameLeftArrow: uv.KeyLeft, key.NameRightArrow: uv.KeyRight, key.NameUpArrow: uv.KeyUp, key.NameDownArrow: uv.KeyDown, key.NameHome: uv.KeyHome, key.NameEnd: uv.KeyEnd, key.NamePageUp: uv.KeyPgUp, key.NamePageDown: uv.KeyPgDown}
	if c, ok := codes[e.Name]; ok {
		k.Code = c
		return k
	}
	name := string(e.Name)
	if strings.HasPrefix(name, "F") {
		for i := 1; i <= 12; i++ {
			if name == fmtInt(i) {
				k.Code = uv.KeyF1 + rune(i-1)
				return k
			}
		}
	}
	if k.Mod&(uv.ModCtrl|uv.ModAlt) != 0 && utf8.RuneCountInString(name) == 1 {
		k.Code = []rune(strings.ToLower(name))[0]
	}
	return k
}

func (t *terminal) selectWord(index int) {
	index -= t.viewStart * t.cols
	if index < 0 || index >= len(t.cells) {
		return
	}
	word := func(i int) bool {
		for _, r := range t.cells[i].Content {
			if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
				return true
			}
		}
		return false
	}
	lo, hi := index, index
	if word(index) {
		for lo > index/t.cols*t.cols && word(lo-1) {
			lo--
		}
		for hi+1 < min(len(t.cells), (index/t.cols+1)*t.cols) && word(hi+1) {
			hi++
		}
	}
	t.anchor, t.caret, t.hasSelection = t.viewStart*t.cols+lo, t.viewStart*t.cols+hi, true
}
