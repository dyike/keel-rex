package vt

import "github.com/charmbracelet/x/ansi/parser"

const initialParserDataSize = 64 << 10
const maxParserDataSize = 4 << 20

// Keep the existing 4 MiB OSC/DCS limit without allocating it for every idle
// terminal. SetDataSize preserves the parser's current length and state; copy
// the collected prefix into the new buffer before it consumes the next byte.
func (e *Emulator) growParserData(b byte) {
	if e.parserDataSize >= maxParserDataSize {
		return
	}
	state := e.parser.State()
	switch state {
	case parser.OscStringState, parser.DcsStringState, parser.ApcStringState, parser.PmStringState, parser.SosStringState:
	default:
		return
	}
	data := e.parser.Data()
	if len(data) != e.parserDataSize {
		return
	}
	_, action := parser.Table.Transition(state, b)
	if action != parser.PutAction {
		return
	}
	e.parserDataSize = min(2*e.parserDataSize, maxParserDataSize)
	e.parser.SetDataSize(e.parserDataSize)
	copy(e.parser.Data(), data)
}
