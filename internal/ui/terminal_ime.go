package ui

import (
	"gioui.org/f32"
	"gioui.org/io/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/dyike/keel/ui/core"
	"image"
)

// Only composing text is editable locally. Confirmed text belongs to the
// terminal application and must never be replaced by editor-range offsets.
type terminalIME struct {
	text      []rune
	selection key.Range
	composing bool
}

func (i *terminalIME) edit(e key.EditEvent) {
	start, end := min(e.Range.Start, e.Range.End), max(e.Range.Start, e.Range.End)
	start, end = max(0, min(start, len(i.text))), max(0, min(end, len(i.text)))
	text := append([]rune(nil), i.text[:start]...)
	text = append(text, []rune(e.Text)...)
	text = append(text, i.text[end:]...)
	i.text = text
	pos := start + len([]rune(e.Text))
	i.selection = key.Range{Start: pos, End: pos}
}

func (t *terminal) flushIME() {
	if t.ime.composing {
		return
	}
	if len(t.ime.text) > 0 {
		t.session.SendText(string(t.ime.text), false)
	}
	t.ime = terminalIME{}
}

func (t *terminal) layoutIME(gtx core.C, p painter, cursor image.Point, cw, ch float32, size image.Point, showCaret bool) {
	i := &t.ime
	start, end := max(0, min(i.selection.Start, len(i.text))), max(0, min(i.selection.End, len(i.text)))
	x, y := float32(cursor.X)*cw, float32(cursor.Y)*ch
	x = max(0, min(x, float32(size.X)/p.scale-cw))
	y = max(0, min(y, float32(size.Y)/p.scale-ch))
	caretX := x
	var bounds image.Rectangle
	if i.composing && len(i.text) > 0 {
		text := ansi.Truncate(string(i.text), max(1, int((float32(size.X)/p.scale-x)/cw)), "")
		width := max(cw, float32(ansi.StringWidth(text))*cw)
		fg, bg := ink, rgb(0xf4f4f1)
		if t.owner != nil {
			fg, bg = t.owner.colors().text, t.owner.colors().panel
		}
		p.rect(x, y, width, ch, 0, bg)
		p.label(text, x, y+ch*.8, ch/1.28, fg, true, false)
		p.line(x, y+ch-1, width, 1, fg)
		caretX = min(x+float32(ansi.StringWidth(string(i.text[:end])))*cw, float32(size.X)/p.scale-terminalCaretWidth)
		if showCaret {
			t.paintCaret(p, caretX, y, ch)
		}
		bounds = image.Rect(int(x*p.scale), int(y*p.scale), int((x+width)*p.scale), int((y+ch)*p.scale))
	}
	gtx.Execute(key.SnippetCmd{Tag: t, Snippet: key.Snippet{Range: key.Range{Start: 0, End: len(i.text)}, Text: string(i.text)}})
	gtx.Execute(key.SelectionCmd{Tag: t, Range: key.Range{Start: start, End: end}, Caret: key.Caret{Pos: f32.Pt(caretX*p.scale, (y+ch*.8)*p.scale), Ascent: ch * .8 * p.scale, Descent: ch * .2 * p.scale}, CompositionBounds: bounds})
}
