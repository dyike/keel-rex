package main

import (
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/io/transfer"
	"gioui.org/op"
	"gioui.org/op/clip"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel/ui/core"
	"image"
	"image/color"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type terminal struct {
	ime                    terminalIME
	session                *session
	owner                  *app
	pane                   *pane
	scroll                 int
	focused, selecting     bool
	anchor, caret          int
	cols, rows             int
	cells                  []uv.Cell
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

func (t *terminal) selected() string {
	if !t.hasSelection && t.anchor == t.caret {
		return ""
	}
	lo, hi := min(t.anchor, t.caret), max(t.anchor, t.caret)
	var out strings.Builder
	for i := lo; i <= hi && i < len(t.cells); i++ {
		if i < 0 {
			continue
		}
		if i > lo && i%t.cols == 0 {
			out.WriteByte('\n')
		}
		c := t.cells[i]
		if c.Width == 0 {
			continue
		}
		if c.Content == "" {
			out.WriteByte(' ')
		} else {
			out.WriteString(c.Content)
		}
	}
	return out.String()
}
func (t *terminal) Layout(gtx core.C) core.D {
	size := gtx.Constraints.Max
	scale := gtx.Metric.PxPerDp
	if scale == 0 {
		scale = 1
	}
	w, h := float32(size.X)/scale, float32(size.Y)/scale
	fontSize := terminalSize
	if t.owner != nil && t.owner.prefs.FontSize > 0 {
		fontSize = t.owner.prefs.FontSize
	}
	cw, ch := fontSize*.60208, fontSize*1.28
	t.cols = max(2, int(w/cw))
	t.rows = max(2, int(h/ch))
	t.session.resize(t.cols, t.rows)
	if t.owner != nil {
		appearance := "light"
		if t.owner.appearanceDark {
			appearance = "dark"
		}
		t.session.syncAppearance(appearance)
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
			t.anchor = 0
			t.caret = len(t.cells) - 1
			t.hasSelection = true
		case core.EditPaste:
			gtx.Execute(clipboard.ReadCmd{Tag: t})
		case core.EditCopy:
			if value := t.selected(); value != "" {
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(value))})
			}
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: t, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll}, key.FocusFilter{Target: t}, key.Filter{Focus: t, Name: "", Optional: key.ModCtrl | key.ModCommand | key.ModShift | key.ModAlt | key.ModSuper}, transfer.TargetFilter{Target: t, Type: "application/text"})
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
			}
			if e.Focus {
				if t.owner.focused == t.pane {
					t.pane.attention = false
				}
			}
		case key.EditEvent:
			t.scroll = 0
			t.anchor = t.caret
			t.hasSelection = false
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
				t.anchor = 0
				t.caret = len(t.cells) - 1
				t.hasSelection = true
				continue
			}
			if editModifier && e.Name == "V" {
				gtx.Execute(clipboard.ReadCmd{Tag: t})
				continue
			}
			if editModifier && e.Name == "C" {
				v := t.selected()
				if v != "" {
					gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v))})
				}
				continue
			}
			if e.Modifiers.Contain(key.ModCommand) {
				continue
			}
			k := terminalKey(e)
			if k.Code != 0 {
				t.scroll = 0
				t.session.send(k)
			}
		case transfer.DataEvent:
			r := e.Open()
			b, _ := io.ReadAll(io.LimitReader(r, 16<<20))
			r.Close()
			t.scroll = 0
			t.session.text(string(b), true)
		case pointer.Event:
			s := t.session
			s.mu.Lock()
			mouse := s.mouseTracking
			s.mu.Unlock()
			if mouse && !e.Modifiers.Contain(key.ModShift) {
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
				s.mouse(kind, m)
				continue
			}
			idx := min(t.cols*t.rows-1, max(0, int(e.Position.Y/(ch*scale))*t.cols+int(e.Position.X/(cw*scale))))
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
						t.caret = min(len(t.cells)-1, t.anchor+t.cols-1)
						t.hasSelection = true
						t.clickCount = 0
					}
				}
				t.selecting = true
			case pointer.Drag:
				if t.selecting {
					t.caret = idx
					t.hasSelection = true
				}
			case pointer.Release:
				t.selecting = false
			case pointer.Scroll:
				t.scrollRemainder += e.Scroll.Y / (ch * scale)
				delta := int(t.scrollRemainder)
				t.scrollRemainder -= float32(delta)
				t.scroll = max(0, t.scroll-delta)
				t.hasSelection = false
				t.anchor = t.caret
			}
		}
	}
	t.flushIME()
	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	defer area.Pop()
	event.Op(gtx.Ops, t)
	key.InputHintOp{Tag: t, Hint: key.HintAny}.Add(gtx.Ops)
	pointer.CursorText.Add(gtx.Ops)
	s := t.session
	if s.remote != nil {
		s.remote.mu.Lock()
		s.remote.scroll = t.scroll
		s.remote.mu.Unlock()
	}
	frame := s.snapshot(t.scroll, 0)
	if s.remote == nil || frame.Scroll == t.scroll {
		t.scroll = frame.Scroll
	}
	if frame.Alt {
		t.scroll = 0
	}
	t.cells = make([]uv.Cell, t.cols*t.rows)
	for y := 0; y < min(t.rows, frame.Rows); y++ {
		for x := 0; x < min(t.cols, frame.Cols); x++ {
			index := y*frame.Cols + x
			if index < len(frame.Cells) {
				t.cells[y*t.cols+x] = frame.Cells[index].cell()
			}
		}
	}
	pos := frame.Cursor
	visible := frame.CursorVisible && !frame.Exited
	p := painter{gtx, scale}
	lo, hi := min(t.anchor, t.caret), max(t.anchor, t.caret)
	for y := 0; y < t.rows; y++ {
		for x := 0; x < t.cols; x++ {
			i := y*t.cols + x
			c := t.cells[i]
			fg, bg := ink, color.NRGBA{}
			if t.owner != nil {
				fg = t.owner.colors().text
			}
			if c.Style.Fg != nil {
				fg = color.NRGBAModel.Convert(c.Style.Fg).(color.NRGBA)
			}
			if c.Style.Bg != nil {
				bg = color.NRGBAModel.Convert(c.Style.Bg).(color.NRGBA)
			}
			if c.Style.Attrs&uv.AttrReverse != 0 {
				if bg.A == 0 {
					bg = t.owner.colors().panel
				}
				fg, bg = bg, fg
			}
			if (t.hasSelection || hi > lo) && i >= lo && i <= hi {
				bg = rgb(0xd6e4fa)
				if t.owner.prefs.Appearance == "dark" || t.owner.prefs.Appearance == "system" && t.owner.appearanceDark {
					bg = rgb(0x394c65)
				}
			}
			xx, yy := float32(x)*cw, float32(y)*ch
			if bg.A != 0 {
				p.rect(xx, yy, cw*float32(max(1, c.Width)), ch, 0, bg)
			}
			if c.Width > 0 && c.Content != "" && c.Content != " " {
				p.label(c.Content, xx, yy+fontSize, fontSize, fg, true, c.Style.Attrs&uv.AttrBold != 0)
			}
			if c.Style.Underline != 0 {
				p.line(xx, yy+ch-2, cw, .7, fg)
			}
		}
	}
	showCaret, nextBlink := t.caretBlink(gtx.Now, pos, visible && t.scroll == 0)
	if !nextBlink.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: nextBlink})
	}
	if showCaret && !(t.ime.composing && len(t.ime.text) > 0) {
		t.paintCaret(p, float32(pos.X)*cw, float32(pos.Y)*ch, ch)
	}
	t.layoutIME(gtx, p, frame.Cursor, cw, ch, size, showCaret)
	semantic.LabelOp(frame.Plain).Add(gtx.Ops)
	core.Role("text").Add(gtx.Ops)
	return core.D{Size: size}
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
	t.anchor, t.caret, t.hasSelection = lo, hi, true
}
