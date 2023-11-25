package ansiParser

import (
	"bytes"
	"sync"
)

// state machine
// https://en.wikipedia.org/wiki/ANSI_escape_code#CSI_codes

const (
	ESC = `\x1b`

	// ANSI parser state machine states
	NORMAL = iota
	ESCAPE
)

type ANSIParser struct {
	buffer bytes.Buffer
	mux    sync.Mutex

	cursorLine int
	cursorCol  int

	state int
}

func New() *ANSIParser {
	return &ANSIParser{
		state: NORMAL,
	}
}

func (ap *ANSIParser) Read(p []byte) (int, error) {
	return ap.buffer.Read(p)
}

func (ap *ANSIParser) Write(p []byte) (int, error) {
	ap.mux.Lock()
	defer ap.mux.Unlock()

	for _, b := range p {
		switch b {
		case '\r':
			ap.cursorCol = 0
		case '\n':
			ap.cursorLine++
		case '\x1b':

		default:
			ap.buffer.WriteByte(b)
		}
	}
	return len(p), nil
}
