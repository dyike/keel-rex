package vt

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

func (e *Emulator) advanceParser(b byte) {
	if e.oscUTF8Len == 0 && (b < 0xc2 || b > 0xf4 || e.parser.State() != parser.OscStringState) {
		e.advanceParserByte(b)
		return
	}

	prevLen := e.oscUTF8Len
	e.oscUTF8[prevLen] = b
	runeBytes := e.oscUTF8[:prevLen+1]
	if !utf8.FullRune(runeBytes) {
		e.oscUTF8Len++
		return
	}
	e.oscUTF8Len = 0

	if _, size := utf8.DecodeRune(runeBytes); size == 1 {
		// Replay the invalid prefix with the original ANSI semantics. The
		// byte that broke it may start a new UTF-8 character or end OSC.
		for _, prefixByte := range runeBytes[:prevLen] {
			e.advanceParserByte(prefixByte)
		}
		e.advanceParser(b)
		return
	}

	for _, runeByte := range runeBytes {
		if runeByte != ansi.ST {
			e.advanceParserByte(runeByte)
			continue
		}

		// The ANSI parser treats 0x9c as ST in OSC. Append a space via
		// PutAction and restore the original byte in the parser's buffer.
		before := len(e.parser.Data())
		e.advanceParserByte(' ')
		if data := e.parser.Data(); len(data) > before {
			data[before] = runeByte
		}
	}
}

func (e *Emulator) advanceParserByte(b byte) {
	e.growParserData(b)
	e.parser.Advance(b)
}
