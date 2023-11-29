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
	CSI
	OSC
	DCS
	ST
	IGNORE
)

type Color struct {
	Red   uint8
	Green uint8
	Blue  uint8
}

type ScreenCell struct {
	Char rune
	FG   Color
	BG   Color
}

type ANSIParser struct {
	screen []ScreenCell
	mux    sync.Mutex
	state  int

	cursorLine int
	cursorCol  int

	MaxCols int
	MaxRows int

	TabSize int

	params string // CSI parameters
}

func New(rows, cols int) *ANSIParser {
	r := &ANSIParser{
		MaxCols: cols,
		MaxRows: rows,

		cursorLine: 0,
		cursorCol:  0,

		screen: make([]ScreenCell, rows*cols),

		TabSize: 4,

		params: "",

		state: NORMAL,
	}
	for i := 0; i < rows*cols; i++ {
		r.screen[i].Char = ' '
	}

	return r
}

func (ap *ANSIParser) Resize(rows, cols int) {
	ap.mux.Lock()
	defer ap.mux.Unlock()

	ap.MaxCols = cols
	ap.MaxRows = rows

	ap.cursorLine = 0
	ap.cursorCol = 0

	ap.screen = make([]ScreenCell, rows*cols)
}

func (ap *ANSIParser) PutCharWithColor(r rune, fg, bg Color) {
	ap.mux.Lock()
	defer ap.mux.Unlock()

	i := ap.cursorLine*ap.MaxCols + ap.cursorCol
	ap.screen[i].Char = r
	ap.screen[i].FG = fg
	ap.screen[i].BG = bg

	ap.cursorCol++

	if ap.cursorCol >= ap.MaxCols {
		ap.cursorCol = 0
		ap.cursorLine++
	}

	if ap.cursorLine >= ap.MaxRows {
		ap.cursorLine = 0
	}
}

func (ap *ANSIParser) Put(r rune) {
	ap.mux.Lock()
	defer ap.mux.Unlock()

	i := ap.cursorLine*ap.MaxCols + ap.cursorCol
	ap.screen[i].Char = r
	ap.cursorCol++

	if ap.cursorCol >= ap.MaxCols {
		ap.cursorCol = 0
		ap.cursorLine++
	}

	if ap.cursorLine >= ap.MaxRows {
		ap.cursorLine = 0
	}
}

func (ap *ANSIParser) Clear() {
	ap.mux.Lock()
	defer ap.mux.Unlock()

	ap.cursorLine = 0
	ap.cursorCol = 0

	ap.screen = make([]ScreenCell, ap.MaxRows*ap.MaxCols)
}

// GetScreenAsAnsi returns the screen as ANSI
func (ap *ANSIParser) GetScreenAsAnsi() []byte {
	ap.mux.Lock()
	defer ap.mux.Unlock()

	buffer := make([]byte, 0, ap.MaxRows*ap.MaxCols*8) // try to guess the size of the buffer
	bs := bytes.NewBuffer(buffer)

	for i := 0; i < ap.MaxRows*ap.MaxCols; i++ {
		// TODO: implement colors and other attributes
		cell := ap.screen[i]
		bs.WriteRune(cell.Char)
	}

	return bs.Bytes()
}

func isCSIFinal(b byte) bool {
	return b >= 0x40 && b <= 0x7E
}

func (ap *ANSIParser) Write(p []byte) (int, error) {
	for _, b := range p {
		ap.Put(rune(b))
	}
	return len(p), nil
}
