package mterm

import (
	"bytes"
	"cmp"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Based on: https://www.ditig.com/256-colors-cheat-sheet#list-of-colors
var color16 = []Color{
	{0, 0, 0},
	{128, 0, 0},
	{0, 128, 0},
	{128, 128, 0},
	{0, 0, 128},
	{128, 0, 128},
	{0, 128, 128},
	{192, 192, 192},
	// Bright
	{128, 128, 128},
	{255, 0, 0},
	{0, 255, 0},
	{255, 255, 0},
	{0, 0, 255},
	{255, 0, 255},
	{0, 255, 255},
	{255, 255, 255},
}

func color256(n byte) Color {
	cm := []byte{0, 95, 135, 175, 215, 255}
	switch {
	case n > 231:
		n -= 231
		return Color{
			8 + 10*n,
			8 + 10*n,
			8 + 10*n,
		}
	case n > 15:
		n -= 16
		return Color{
			cm[n/36%6],
			cm[n/6%6],
			cm[n%6],
		}
	default:
		return color16[n]
	}
}

const (
	Color16  = 1
	Color256 = 2
	Color16M = 3
)

const (
	FlagBold uint8 = 1 << iota
	FlagUnderline
	FlagBlink
	FlagInverse
	FlagInvisible
	FlagStrike
	FlagItalic
	FlagUnderlineColor
)

type Color [3]byte

type cstate struct {
	FG        Color
	BG        Color
	UL        Color // underline color
	ColorType uint8
	Flags     uint8
}

// set the set based on CSI parameters
func (s *cstate) set(p ...int) error {
	if len(p) == 0 {
		*s = cstate{}
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		sub := p[i:]
		switch {
		case c == 0:
			*s = cstate{}
		case c == 1:
			s.Flags |= FlagBold
		case c == 22:
			s.Flags &= ^FlagBold
		case c == 3:
			s.Flags |= FlagItalic
		case c == 23:
			s.Flags &= ^FlagItalic
		case c == 4:
			s.Flags |= FlagUnderline
		case c == 24:
			s.Flags &= ^FlagUnderline
		case c == 5:
			s.Flags |= FlagBlink
		case c == 25:
			s.Flags &= ^FlagBlink
		case c == 7:
			s.Flags |= FlagInverse
		case c == 27:
			s.Flags &= ^FlagInverse
		case c == 8:
			s.Flags |= FlagInvisible
		case c == 9:
			s.Flags |= FlagStrike
		case c >= 90 && c <= 97: // FG bright (not bold)
			s.ColorType = s.ColorType&0b1100 | Color16
			s.FG[0] = byte(c)
		case c >= 100 && c <= 107: // BG bright (not bold)
			s.ColorType = s.ColorType&0b0011 | (Color16 << 2)
			s.BG[0] = byte(c)
		case c >= 30 && c <= 37: // FG
			s.ColorType = s.ColorType&0b1100 | Color16
			s.FG[0] = byte(c)
		case c == 39: // FG default foreground
			s.ColorType &= 0b1100
		case c >= 40 && c <= 47: // BG 16 colors
			s.ColorType = s.ColorType&0b0011 | (Color16 << 2)
			s.BG[0] = byte(c)
		case c == 49: // BG default background
			s.ColorType &= 0b0011
		// FG 256 Colors
		case c == 38 && len(sub) >= 3 && sub[1] == 5:
			s.ColorType = s.ColorType&0b1100 | Color256
			s.FG[0] = byte(sub[2])
			i += 2
		// BG 256 Colors
		case c == 48 && len(sub) >= 3 && sub[1] == 5:
			s.ColorType = s.ColorType&0b0011 | (Color256 << 2)
			s.BG[0] = byte(sub[2])
			i += 2
		// FG 16M Colors
		case c == 38 && len(sub) >= 5 && sub[1] == 2:
			s.ColorType = s.ColorType&0b1100 | Color16M
			s.FG = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4
		// BG 16M Colors
		case c == 48 && len(sub) >= 5 && sub[1] == 2:
			s.ColorType = s.ColorType&0b0011 | (Color16M << 2)
			s.BG = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4
		// XXX: Experimental
		// underline sample
		// \x1b[58:2::173:216:230m
		case c == 58 && len(sub) >= 5 && sub[1] == 2:
			s.Flags |= FlagUnderlineColor
			s.UL = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4

		default:
			return fmt.Errorf("unknown SGR: %v", c)
		}
	}
	return nil
}

// Cell is a single cell in the terminal
type Cell struct {
	Char rune
	cstate
}

type stateFn func(t *Terminal, r rune) (stateFn, error)

// Terminal is an in memory terminal emulator
type Terminal struct {
	mux    sync.Mutex
	screen []Cell

	cstate cstate

	stateProc stateFn

	CursorLine int
	CursorCol  int

	Title   string
	MaxCols int
	MaxRows int

	TabSize int

	cellUpdate int

	saveCursor   [2]int
	scrollRegion [2]int // startRow, endRow
}

// New returns a new terminal with the given rows and cols
func New(rows, cols int) *Terminal {
	screen := make([]Cell, rows*cols)
	for i := range screen {
		screen[i].Char = ' '
	}
	return &Terminal{
		MaxCols: cols,
		MaxRows: rows,

		CursorLine: 0,
		CursorCol:  0,

		screen: screen,

		TabSize: 8,

		stateProc:    (*Terminal).normal,
		scrollRegion: [2]int{0, rows},
	}
}

// Cells returns a copy of the underlying screen cells
func (t *Terminal) Cells() []Cell {
	t.mux.Lock()
	defer t.mux.Unlock()

	return slices.Clone(t.screen)
}

type EscapeError struct {
	Err    error
	Offset int
}

func (e EscapeError) Error() string {
	return fmt.Sprintf("error parsing escape sequence at %d: %v", e.Offset, e.Err)
}

// Write implements io.Writer and writes the given bytes to the terminal
func (t *Terminal) Write(p []byte) (int, error) {
	t.mux.Lock()
	defer t.mux.Unlock()

	for i := 0; i < len(p); {
		r, sz := utf8.DecodeRune(p[i:])
		i += sz
		if err := t.put(r); err != nil {
			return i, EscapeError{
				Err:    err,
				Offset: i,
			}
		}
	}
	return len(p), nil
}

// Put processes a single rune in the terminal
func (t *Terminal) Put(r rune) error {
	t.mux.Lock()
	defer t.mux.Unlock()

	return t.put(r)
}

// Reset resizes and resets the terminal to the default state
func (t *Terminal) Resize(rows, cols int) {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.MaxRows = rows
	t.MaxCols = cols
	t.CursorLine = 0
	t.CursorCol = 0

	t.saveCursor = [2]int{0, 0}
	t.scrollRegion = [2]int{0, rows}

	t.screen = make([]Cell, cols*rows)
}

// Clear clears the terminal moving cursor to 0,0
func (t *Terminal) Clear() {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.CursorLine = 0
	t.CursorCol = 0
	fill(t.screen, Cell{})
}

func (t *Terminal) GetScreenAsAnsi() []byte {
	t.mux.Lock()
	defer t.mux.Unlock()

	buf := bytes.NewBuffer(nil)
	x, y := 0, 0
	lastState := cstate{}
	for i := range t.screen {
		if x >= t.MaxCols {
			y++
			x = 0
			// return to beginning of line
			buf.WriteString("\r\n")
		}
		c := t.screen[i]
		if c.cstate != lastState {
			lastState = c.cstate
			// different state, we shall reset and set the new state
			buf.WriteString("\033[0")

			switch c.ColorType & 0b11 {
			case Color16:
				fmt.Fprintf(buf, ";%d", c.FG[0])
			case Color256:
				fmt.Fprintf(buf, ";38;5;%d", c.FG[0])
			case Color16M:
				fmt.Fprintf(buf, ";38;2;%d;%d;%d", c.FG[0], c.FG[1], c.FG[2])
			}
			switch (c.ColorType >> 2) & 0b11 {
			case Color16:
				fmt.Fprintf(buf, ";%d", c.BG[0])
			case Color256:
				fmt.Fprintf(buf, ";48;5;%d", c.BG[0])
			case Color16M:
				fmt.Fprintf(buf, ";48;2;%d;%d;%d", c.BG[0], c.BG[1], c.BG[2])

			}
			if c.Flags&FlagUnderlineColor != 0 {
				fmt.Fprintf(buf, ";58;2;%d;%d;%d", c.UL[0], c.UL[1], c.UL[2])
			}
			if c.Flags&FlagBold != 0 {
				buf.WriteString(";1")
			}
			if c.Flags&FlagUnderline != 0 {
				buf.WriteString(";4")
			}
			if c.Flags&FlagBlink != 0 {
				buf.WriteString(";5")
			}
			if c.Flags&FlagInverse != 0 {
				buf.WriteString(";7")
			}
			if c.Flags&FlagInvisible != 0 {
				buf.WriteString(";8")
			}
			if c.Flags&FlagStrike != 0 {
				buf.WriteString(";9")
			}
			fmt.Fprintf(buf, "m")
		}
		r := c.Char
		if r == 0 {
			r = ' '
		}
		buf.WriteRune(r)
		x += 1
	}
	return buf.Bytes()
}

func (t *Terminal) put(r rune) error {
	// Default to normal stateFn

	// TODO: Figure this out, vim ocasionally sends this in any state!?
	// probably querying something
	if r == '\x01' {
		return nil
	}
	sfn := t.stateProc
	if sfn == nil {
		sfn = (*Terminal).normal
	}

	ns, err := sfn(t, r)
	if ns != nil {
		t.stateProc = ns
	}
	return err
}

func (t *Terminal) normal(r rune) (stateFn, error) {
	switch {
	case r == '\033':
		return (*Terminal).esc, nil
	case r == '\n':
		t.CursorCol = 0
		t.nextLine()
	case r == '\r':
		t.CursorCol = 0
	case r == '\b':
		t.CursorCol = max(0, t.CursorCol-1)
	case r == '\t':
		t.CursorCol = (t.CursorCol + t.TabSize) / t.TabSize * t.TabSize
		t.CursorCol = min(t.CursorCol, t.MaxCols-1)
	case r < 32: // least printable char, we ignore it

	default:
		if t.CursorCol >= t.MaxCols {
			t.CursorCol = 0
			t.nextLine()
		}
		cl := Cell{
			Char:   r,
			cstate: t.cstate,
		}
		offs := t.CursorCol + t.CursorLine*t.MaxCols
		if offs >= len(t.screen) {
			// Rare, but to be safe..
			return nil, fmt.Errorf("offset out of bounds: %d", offs)
		}
		t.screen[offs] = cl
		// TODO: {lpf} might have issues with erasers
		t.CursorCol++
		t.cellUpdate++
	}
	return nil, nil
}

func (t *Terminal) esc(r rune) (stateFn, error) {
	switch r {
	case '[':
		return t.csi(), nil
	case ']':
		return t.osc(), nil
	case '>':
		// TODO: {lpf} (completed by copilot: DEC private mode reset)
	case '=':
		// TODO: {lpf} (completed by copilot: DEC private mode set)
	case '(':
		return t.ignore(1, (*Terminal).normal), nil // set G0 charset (ignore next rune and go to normal state)
	case 'c':
		// TODO: should be t.Reset() and reset state
		t.Clear()
	default:
		return (*Terminal).normal, fmt.Errorf("unknown escape sequence: %d %[1]c", r)
	}
	return (*Terminal).normal, nil
}

// State dummy state to accept any n runes
func (t *Terminal) ignore(n int, next stateFn) stateFn {
	return func(*Terminal, rune) (stateFn, error) {
		n--
		if n <= 0 {
			return next, nil
		}
		return nil, nil
	}
}

type customSeqFunc func(*Terminal, []rune, bool) (stateFn, error)

// State customSeq is a helper function to create a stateFn that will accept a sequence
// once the sequence is complete it will call the provided function with bool
// as true, false otherwise
func (t *Terminal) customSeq(s []rune, fn customSeqFunc) stateFn {
	seq := make([]rune, 0, len(s))
	return func(t *Terminal, r rune) (stateFn, error) {
		seq = append(seq, r)
		if r != s[0] {
			return fn(t, seq, false)
		}
		s = s[1:]
		// finished, call the func and return next state
		if len(s) == 0 {
			return fn(t, seq, true)
		}
		return nil, nil
	}
}

// State Operating System Command
func (t *Terminal) osc() stateFn {
	attrbuf := bytes.NewBuffer(nil)
	title := &strings.Builder{}
	var fn stateFn
	fn = func(_ *Terminal, r rune) (stateFn, error) {
		if r == ';' || unicode.IsNumber(r) {
			attrbuf.WriteRune(r)
			return nil, nil
		}
		switch r {
		case '\a': // xterm
			// TODO: Lock t.SetTitle(title.String())
			t.Title = title.String()
			return (*Terminal).normal, nil
		// Handle string terminator "\033\\"
		case '\033': // string terminator
			return t.customSeq([]rune{'\\'}, func(t *Terminal, s []rune, ok bool) (stateFn, error) {
				if ok {
					// TODO: Lock
					t.Title = title.String()
					return (*Terminal).normal, nil
				}

				title.WriteRune('\033')
				sfn := fn
				for _, r := range s {
					// pass unaccepted runes through this state, following the fns
					f, err := sfn(t, r)
					if err != nil {
						return nil, err
					}
					if f != nil {
						sfn = f
					}
				}
				return sfn, nil
			}), nil
		default:
			title.WriteRune(r)
		}
		return nil, nil
	}
	return fn
}

// to handle cases like "\033[>P;N..." (cursor keys to application mode)
func (t *Terminal) csiGT() stateFn {
	// attrbuf := bytes.NewBuffer(nil)
	return func(_ *Terminal, r rune) (stateFn, error) {
		if r == ';' || unicode.IsNumber(r) {
			// TODO: implement as params
			// attrbuf.WriteRune(r)
			return nil, nil
		}
		// TODO: no clue, implement if needed (tmux)
		switch r {
		case 'm':
			return (*Terminal).normal, nil
		case 'c':
			return (*Terminal).normal, nil
		case 'q':
			return (*Terminal).normal, nil
		default:
			return (*Terminal).normal, fmt.Errorf("unknown CSI>: %d %[1]c", r)
		}
	}
}

// State Control Sequence Introducer
func (t *Terminal) csi() stateFn {
	var p []int
	nextParam := true
	return func(t *Terminal, r rune) (stateFn, error) {
		switch {
		case r == ':' || r == ';':
			nextParam = true
			return nil, nil
		case unicode.IsNumber(r):
			if nextParam {
				nextParam = false
				p = append(p, 0)
			}
			last := len(p) - 1
			p[last] = p[last]*10 + int(r-'0')
			return nil, nil
		}

		switch r {
		// for sequences like ESC[?25l (hide cursor)
		case '?':
			// maybe set a flag somewhere or use a new state
			return nil, nil
		case '>':
			return t.csiGT(), nil
		// Cursor movement
		case 'A': // Cursor UP
			n := 1
			getParams(p, &n)
			t.CursorLine = max(0, t.CursorLine-n)
		case 'B': // Cursor DOWN
			n := 1
			getParams(p, &n)
			t.CursorLine = min(t.MaxRows-1, t.CursorLine+n)
		case 'C': // Cursor FORWARD
			n := 1
			getParams(p, &n)
			t.CursorCol = min(t.MaxCols-1, t.CursorCol+n)
		case 'D': // Cursor BACK
			n := 1
			getParams(p, &n)
			t.CursorCol = max(0, t.CursorCol-n)
		case 'E': // (copilot) Moves cursor to beginning of the line n (default 1) lines down.
			n := 1
			getParams(p, &n)
			t.CursorCol = 0
			t.CursorLine = min(t.MaxRows-1, t.CursorLine+n)
		case 'F': // (copilot)  Moves cursor to beginning of the line n (default 1) lines up.
			n := 1
			getParams(p, &n)
			t.CursorCol = 0
			t.CursorLine = max(0, t.CursorLine-n)
		case 'G': // (copilot) Cursor HORIZONTAL ABSOLUTE
			n := 1
			getParams(p, &n)
			t.CursorCol = clamp(n-1, 0, t.MaxCols-1)
		case 'H': // Cursor POSITION (col, line)
			line, col := 1, 1
			getParams(p, &line, &col)
			t.CursorCol = clamp(col-1, 0, t.MaxCols-1)
			t.CursorLine = clamp(line-1, 0, t.MaxRows-1)
		case 'd':
			n := 0
			getParams(p, &n)
			t.CursorLine = clamp(n-1, 0, t.MaxRows-1)
		// Display erase
		case 'J': // Erase in Display
			n := 0
			getParams(p, &n)
			switch n {
			case 0: // clear from cursor to end
				off := t.CursorCol + t.CursorLine*t.MaxCols
				fill(t.screen[off:], Cell{cstate: t.cstate})
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				off := t.CursorCol + t.CursorLine*t.MaxCols
				fill(t.screen[:off], Cell{cstate: t.cstate})
				t.cellUpdate++
			case 2: // clear everything
				fill(t.screen, Cell{cstate: t.cstate})
				t.cellUpdate++
			}
		case 'K': // Erase in Line
			n := 0
			getParams(p, &n)
			l := clamp(t.CursorLine, 0, t.MaxRows) * t.MaxCols
			line := t.screen[l : l+t.MaxCols]
			switch n {
			case 0: // clear from cursor to end
				fill(line[t.CursorCol:], Cell{cstate: t.cstate})
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				fill(line[:t.CursorCol], Cell{cstate: t.cstate})
				t.cellUpdate++
			case 2: // clear everything
				fill(line, Cell{cstate: t.cstate})
				t.cellUpdate++
			}
		case 'M': // Delete lines, it will move the rest of the lines up
			n := 1
			getParams(p, &n)
			off := t.CursorCol + t.CursorLine*t.MaxCols
			copy(t.screen[off:], t.screen[off+n*t.MaxCols:])
			fill(t.screen[len(t.screen)-n*t.MaxCols:], Cell{})
			t.cellUpdate++
		case 'P': // Delete chars in line it will move the rest of the line to the left
			n := 1
			getParams(p, &n)
			l := t.CursorLine * t.MaxCols
			line := t.screen[l : l+t.MaxCols]

			copy(line[t.CursorCol:], line[t.CursorCol+n:])
			fill(line[len(line)-n:], Cell{})
		case 'X': // Erase chars
			n := 0
			getParams(p, &n)
			off := t.CursorCol + t.CursorLine*t.MaxCols
			end := min(off+n, len(t.screen))
			fill(t.screen[off:end], Cell{cstate: t.cstate})
			t.cellUpdate++
		case 'L': // Insert lines, it will push lines forward
			n := 1
			getParams(p, &n)

			start := t.scrollRegion[0] * t.MaxCols
			end := t.scrollRegion[1] * t.MaxCols
			region := t.screen[start:end]

			lr := max(t.CursorLine, 0)

			loff := clamp(lr*t.MaxCols, 0, len(region))
			eoff := clamp(loff+n*t.MaxCols, 0, len(region))
			dup := slices.Clone(region)
			copy(region[eoff:], dup[loff:])
			fill(region[loff:eoff], Cell{cstate: t.cstate})
			t.cellUpdate++
		case '@':
			// TODO: {lpf} (comment by copilot: Insert blank characters (SP) (default = 1))
		// SGR
		case 'm':
			err := t.cstate.set(p...)
			return (*Terminal).normal, err
		case 'u':
			t.CursorLine = t.saveCursor[0]
			t.CursorCol = t.saveCursor[1]
		case 's':
			t.saveCursor = [2]int{t.CursorLine, t.CursorCol}
		case 'c':
			// TODO: {lpf} (comment by copilot: Send device attributes)
		case 'h':
			switch p[0] {
			case 1004:
				// TODO: Turn focus report ON
			}
		case 'l':
			switch p[0] {
			case 25: // hide cursor if first rune is '?'
			case 1:
				// TODO: Turn cursor keys to application mode OFF
			}
		case 't':
			// TODO: {lpf} (comment by copilot: Window manipulation)
		case 'r':
			top, bottom := 1, t.MaxRows
			getParams(p, &top, &bottom)
			t.scrollRegion[0] = clamp(top-1, 0, t.MaxRows)
			t.scrollRegion[1] = clamp(bottom, 0, t.MaxRows)
			// TODO: this needs some love, it's not working as expected
			//
			switch {
			case len(p) == 0:
				// fill(t.screen, Cell{})
				t.CursorLine = 0
				t.CursorCol = 0
			case len(p) == 1:
				t.CursorLine = 0
				t.CursorCol = 0
			default:
				// fill(t.screen, Cell{})
				// t.cursorLine = 0
				// t.cursorCol = 0
			}

		case 'S': // Scrollup
			n := 1
			getParams(p, &n)
			start := t.scrollRegion[0] * t.MaxCols
			end := t.scrollRegion[1] * t.MaxCols
			region := t.screen[start:end]
			copy(region, region[n*t.MaxCols:])
			fill(region[len(region)-n*t.MaxCols:], Cell{})
		case 'T': // Scrolldown
			n := 1
			getParams(p, &n)
			start := t.scrollRegion[0] * t.MaxCols
			end := t.scrollRegion[1] * t.MaxCols
			region := t.screen[start:end]
			copy(region[n*t.MaxCols:], region)
			fill(region[:n*t.MaxCols], Cell{})
		default:
			return (*Terminal).normal, fmt.Errorf("unknown CSI: %d %[1]c", r)

		}
		return (*Terminal).normal, nil
	}
}

func (t *Terminal) nextLine() {
	t.CursorLine++
	if t.CursorLine < t.scrollRegion[1]-1 {
		return
	}
	if t.CursorLine == t.scrollRegion[1] {
		start := (t.scrollRegion[0]) * t.MaxCols
		end := t.scrollRegion[1] * t.MaxCols
		region := t.screen[start:end]
		// move buffer up 1 line
		copy(region, region[t.MaxCols:])
		fill(region[len(region)-t.MaxCols:], Cell{})
		t.CursorLine--
		t.cellUpdate++
	}
	// Replicate odd xterm behaviour of when the cursor is outside of the region
	// it will not print neither scroll
	if t.CursorLine == t.MaxRows {
		t.CursorLine--
	}
}

// Updates returns a sequence number that is incremented every time the screen
// cells are updated
func (t *Terminal) Updates() int {
	t.mux.Lock()
	defer t.mux.Unlock()

	return t.cellUpdate
}

// DBGStateFn returns the state func name
func (t *Terminal) DBGStateFn() string {
	fi := runtime.FuncForPC(reflect.ValueOf(t.stateProc).Pointer())
	return fi.Name()
}

// DBG Similar to GetScreenAsAnsi but with a cursor
func (t *Terminal) DBG() []byte {
	t.mux.Lock()
	defer t.mux.Unlock()

	buf := bytes.NewBuffer(nil)
	x, y := 0, 0
	lastState := cstate{}
	for i := range t.screen {
		if x >= t.MaxCols {
			y++
			x = 0
			buf.WriteString("\r\n")
			lastState = cstate{}
		}
		c := t.screen[i]

		if c.cstate != lastState {
			lastState = c.cstate
			// different state, we shall reset and set the new state
			buf.WriteString("\033[0")

			switch c.ColorType & 0b11 {
			case Color16:
				fmt.Fprintf(buf, ";%d", c.FG[0])
			case Color256:
				fmt.Fprintf(buf, ";38;5;%d", c.FG[0])
			case Color16M:
				fmt.Fprintf(buf, ";38;2;%d;%d;%d", c.FG[0], c.FG[1], c.FG[2])
			}
			switch (c.ColorType >> 2) & 0b11 {
			case Color16:
				fmt.Fprintf(buf, ";%d", c.BG[0])
			case Color256:
				fmt.Fprintf(buf, ";48;5;%d", c.BG[0])
			case Color16M:
				fmt.Fprintf(buf, ";48;2;%d;%d;%d", c.BG[0], c.BG[1], c.BG[2])
			}
			if c.Flags&FlagUnderlineColor != 0 {
				fmt.Fprintf(buf, ";58;2;%d;%d;%d", c.UL[0], c.UL[1], c.UL[2])
			}
			if c.Flags&FlagBold != 0 {
				buf.WriteString(";1")
			}
			if c.Flags&FlagUnderline != 0 {
				buf.WriteString(";4")
			}
			if c.Flags&FlagBlink != 0 {
				buf.WriteString(";5")
			}
			if c.Flags&FlagInverse != 0 {
				buf.WriteString(";7")
			}
			if c.Flags&FlagInvisible != 0 {
				buf.WriteString(";8")
			}
			if c.Flags&FlagStrike != 0 {
				buf.WriteString(";9")
			}
			fmt.Fprintf(buf, "m")
		}

		r := c.Char
		if r == 0 {
			r = ' '
		}
		if x == t.CursorCol && y == t.CursorLine {
			fmt.Fprintf(buf, "\033[7m%c\033[27m", r)
			x += 1
			lastState = cstate{}
			continue
		}
		buf.WriteRune(r)
		x += 1
	}
	return buf.Bytes()
}

// getParams helper function to get params from a slice
// if the slice is smaller than the number of params, it will leave the rest as
// is
func getParams(param []int, out ...*int) {
	for i, p := range param {
		if i >= len(out) {
			break
		}
		*out[i] = p
	}
}

// fill fills a slice with a value
func fill[S ~[]T, T any](s S, v T) {
	for i := range s {
		s[i] = v
	}
}

// clamp returns the value clamped between s and b
// similar to min(max(value, smallest),biggest)
func clamp[T cmp.Ordered](v T, s, b T) T {
	if v < s {
		return s
	}
	if v > b {
		return b
	}
	return v
}
