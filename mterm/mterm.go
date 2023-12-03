package mterm

import (
	"bytes"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var color16 = []Color{
	{0, 0, 0},
	{205, 0, 0},
	{0, 205, 0},
	{205, 205, 0},
	{0, 0, 238},
	{205, 0, 205},
	{0, 205, 205},
	{229, 229, 229},
}

const (
	FlagFG uint16 = 1 << iota
	FlagBG
	FlagUnderlineColor
	FlagBold
	FlagUnderline
	FlagBlink
	FlagInverse
	FlagInvisible
	FlagStrike
	FlagItalic
)

type Color [3]byte

type cstate struct {
	FG    Color
	BG    Color
	UL    Color // underline color
	Flags uint16
}

// set the set based on CSI parameters
func (s *cstate) set(p ...int) {
	for i := 0; i < len(p); i++ {
		c := p[i]
		sub := p[i:]
		switch {
		case c == 0:
			s.Flags = 0
		case c == 1:
			s.Flags |= FlagBold
		case c == 22:
			s.Flags &= ^FlagBold
		case c == 3:
			s.Flags |= FlagItalic
		case c == 4:
			s.Flags |= FlagUnderline
		case c == 24:
			s.Flags &= ^FlagUnderline
		case c == 5:
			s.Flags |= FlagBlink
		case c == 7:
			s.Flags |= FlagInverse
		case c == 27:
			s.Flags &= ^FlagInverse
		case c == 8:
			s.Flags |= FlagInvisible
		case c == 9:
			s.Flags |= FlagStrike
		case c >= 90 && c <= 97:
			s.Flags |= FlagFG
			s.FG = color16[c-90]
			s.FG[0] = min(s.FG[0]+50, 255)
			s.FG[1] = min(s.FG[1]+50, 255)
			s.FG[2] = min(s.FG[2]+50, 255)
		case c >= 30 && c <= 37:
			s.Flags |= FlagFG
			s.FG = color16[c-30]
		case c == 39: // default foreground
			s.Flags &= ^FlagFG
		case c >= 40 && c <= 47:
			s.Flags |= FlagBG
			s.BG = color16[c-40]
		case c == 49:
			s.Flags &= ^FlagBG
		// 256 TODO: {lpf} implement a 256 color map
		case c == 38 && len(sub) >= 3 && sub[1] == 5:
			s.Flags |= FlagFG
			// s.FG = Color256[p[2]]
			s.FG = Color{255, 255, 255} // color16[7]
			i += 2
		case c == 48 && len(sub) >= 3 && sub[1] == 5:
			s.Flags |= FlagBG
			// s.BG = Color256[p[2]]
			s.BG = color16[0]
			i += 2
		// 16M
		case c == 38 && len(sub) >= 5 && sub[1] == 2:
			s.Flags |= FlagFG
			s.FG = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4
		// 16M
		case c == 48 && len(sub) >= 5 && sub[1] == 2:
			s.Flags |= FlagBG
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
			log.Printf("Unknown SGR: %v", c)
			panic("unknown sgr")
		}
	}
}

type Cell struct {
	Char rune
	cstate
}

type stateFn func(t *Terminal, r rune) stateFn

type Terminal struct {
	mux    sync.Mutex
	screen []Cell

	cstate cstate

	stateProc stateFn

	cursorLine int
	cursorCol  int

	Title   string
	MaxCols int
	MaxRows int

	TabSize int

	cellUpdate int

	saveCursor [2]int
}

func New(rows, cols int) *Terminal {
	screen := make([]Cell, rows*cols)
	for i := range screen {
		screen[i].Char = ' '
	}
	return &Terminal{
		MaxCols: cols,
		MaxRows: rows,

		cursorLine: 0,
		cursorCol:  0,

		screen: screen,

		TabSize: 8,

		stateProc: (*Terminal).normal,
	}
}

func (t *Terminal) Cells() []Cell {
	t.mux.Lock()
	defer t.mux.Unlock()

	return slices.Clone(t.screen)
}

func (t *Terminal) Write(p []byte) (int, error) {
	t.mux.Lock()
	defer t.mux.Unlock()

	l := len(p)
	for len(p) > 0 {
		r, sz := utf8.DecodeRune(p)
		p = p[sz:]
		t.put(r)
	}
	return l, nil
}

func (t *Terminal) Put(r rune) {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.put(r)
}

func (t *Terminal) Resize(rows, cols int) {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.MaxRows = rows
	t.MaxCols = cols
	t.cursorLine = 0
	t.cursorCol = 0

	t.screen = make([]Cell, cols*rows)
}

func (t *Terminal) Clear() {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.cursorLine = 0
	t.cursorCol = 0
	t.screen = make([]Cell, t.MaxRows*t.MaxCols)
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
			fmt.Fprintln(buf) // not needed?!
		}
		c := t.screen[i]
		if c.cstate != lastState {
			lastState = c.cstate
			codes := []string{"0"}
			// different state, we shall reset and set the new state
			if c.Flags&FlagFG != 0 {
				codes = append(codes,
					fmt.Sprintf("38;2;%d;%d;%d", c.FG[0], c.FG[1], c.FG[2]),
				)
			}
			if c.Flags&FlagBG != 0 {
				if c.BG[0] != 0 || c.BG[1] != 0 || c.BG[2] != 0 {
					codes = append(codes,
						fmt.Sprintf("48;2;%d;%d;%d", c.BG[0], c.BG[1], c.BG[2]),
					)
				}
			}
			if c.Flags&FlagUnderlineColor != 0 {
				codes = append(codes,
					fmt.Sprintf("58;2;%d;%d;%d", c.UL[0], c.UL[1], c.UL[2]),
				)
			}
			if c.Flags&FlagBold != 0 {
				codes = append(codes, "1")
			}
			if c.Flags&FlagUnderline != 0 {
				codes = append(codes, "4")
			}
			if c.Flags&FlagBlink != 0 {
				codes = append(codes, "5")
			}
			if c.Flags&FlagInverse != 0 {
				codes = append(codes, "7")
			}
			if c.Flags&FlagInvisible != 0 {
				codes = append(codes, "8")
			}
			if c.Flags&FlagStrike != 0 {
				codes = append(codes, "9")
			}
			if c.Flags&FlagItalic != 0 {
				codes = append(codes, "3")
			}
			fmt.Fprintf(buf, "\033[%sm", strings.Join(codes, ";"))
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

func (t *Terminal) put(r rune) {
	// Default to normal stateFn
	sfn := t.stateProc
	if sfn == nil {
		sfn = (*Terminal).normal
	}

	ns := sfn(t, r)
	if ns != nil {
		t.stateProc = ns
	}
}

func (t *Terminal) normal(r rune) stateFn {
	switch {
	case r == '\033':
		return (*Terminal).esc
	case r == '\n':
		t.cursorCol = 0
		t.nextLine()
	case r == '\r':
		t.cursorCol = 0
	case r == '\b':
		t.cursorCol = max(0, t.cursorCol-1)
	case r == '\t':
		t.cursorCol = (t.cursorCol + t.TabSize) / t.TabSize * t.TabSize
		t.cursorCol = min(t.cursorCol, t.MaxCols-1)
	case r < 32: // least printable char, we ignore it

	default:
		if t.cursorCol >= t.MaxCols {
			t.cursorCol = 0
			t.nextLine()
		}
		cl := Cell{
			Char:   r,
			cstate: t.cstate,
		}
		offs := t.cursorCol + t.cursorLine*t.MaxCols
		t.screen[offs] = cl
		// TODO: {lpf} might have issues with erasers
		t.cursorCol++
		t.cellUpdate++
	}
	return nil
}

func (t *Terminal) esc(r rune) stateFn {
	switch r {
	case '[':
		return t.csi()
	case ']':
		return t.osc()
	case '>':
		// TODO: {lpf} (completed by copilot: DEC private mode reset)
	case '=':
		// TODO: {lpf} (completed by copilot: DEC private mode set)
	case '(':
		return t.ignore(1, (*Terminal).normal) // set G0 charset (ignore next rune and go to normal state)
	default:
		panic(fmt.Errorf("unknown escape sequence: %d %[1]c", r))
	}
	return (*Terminal).normal
}

// State dummy state to accept one char
func (t *Terminal) ignore(n int, next stateFn) stateFn {
	return func(*Terminal, rune) stateFn {
		n--
		if n <= 0 {
			return next
		}
		return nil
	}
}

// State customSeq is a helper function to create a stateFn that will accept a sequence
// once the sequence is complete it will call the provided function with bool
// as true, false otherwise
func (t *Terminal) customSeq(s []rune, fn func(*Terminal, []rune, bool) stateFn) stateFn {
	seq := make([]rune, 0, len(s))
	return func(t *Terminal, r rune) stateFn {
		if r != s[0] {
			return fn(t, seq, false)
		}
		seq = append(seq, r)
		s = s[1:]
		// finished, call the func and return next state
		if len(s) == 0 {
			return fn(t, seq, true)
		}
		return nil
	}
}

// State Operating System Command
func (t *Terminal) osc() stateFn {
	attrbuf := bytes.NewBuffer(nil)
	title := &strings.Builder{}
	var fn stateFn
	fn = func(_ *Terminal, r rune) stateFn {
		if r == ';' || unicode.IsNumber(r) {
			attrbuf.WriteRune(r)
			return nil
		}
		switch r {
		case '\a': // xterm
			// TODO: Lock t.SetTitle(title.String())
			t.Title = title.String()
			return (*Terminal).normal
		// Handle string terminator "\033\\"
		case '\033': // string terminator
			return t.customSeq([]rune{'\\'}, func(t *Terminal, s []rune, ok bool) stateFn {
				if ok {
					// TODO: Lock
					t.Title = title.String()
					return (*Terminal).normal
				}

				title.WriteRune('\033')
				sfn := fn
				for _, r := range s {
					// pass unaccepted runes through this state, following the fns
					if f := sfn(t, r); f != nil {
						sfn = f
					}
				}
				return sfn
			})
		default:
			title.WriteRune(r)
		}
		return nil
	}
	return fn
}

// State Control Sequence Introducer
func (t *Terminal) csi() stateFn {
	attrbuf := bytes.NewBuffer(nil)

	mkparam := func() []int {
		if attrbuf.Len() == 0 {
			return nil
		}
		defer attrbuf.Reset()
		s := attrbuf.String()
		ps := strings.FieldsFunc(s, func(r rune) bool {
			return r == ':' || r == ';'
		})
		param := make([]int, len(ps))
		for i, p := range ps {
			v, err := strconv.Atoi(p)
			if err != nil {
				log.Printf("Error parsing param: %v param:%v", err, s)
				return nil
			}
			param[i] = v
		}
		return param
	}
	return func(t *Terminal, r rune) stateFn {
		if r == ':' || r == ';' || unicode.IsNumber(r) {
			attrbuf.WriteRune(r)
			return nil
		}

		p := mkparam()
		switch r {
		// for sequences like ESC[?25l (hide cursor)
		case '?':
			// maybe set a flag somewhere
			return nil
		// Cursor movement
		case 'A': // Cursor UP
			n := 1
			getParams(p, &n)
			t.cursorLine = max(0, t.cursorLine-n)
		case 'B': // Cursor DOWN
			n := 1
			getParams(p, &n)
			t.cursorLine = min(t.MaxRows-1, t.cursorLine+n)
		case 'C': // Cursor FORWARD
			n := 1
			getParams(p, &n)
			t.cursorCol = min(t.MaxCols-1, t.cursorCol+n)
		case 'D': // Cursor BACK
			n := 1
			getParams(p, &n)
			t.cursorCol = max(0, t.cursorCol-n)
		case 'E': // (copilot) Moves cursor to beginning of the line n (default 1) lines down.
			n := 1
			getParams(p, &n)
			t.cursorCol = 0
			t.cursorLine = min(t.MaxRows-1, t.cursorLine+n)
		case 'F': // (copilot)  Moves cursor to beginning of the line n (default 1) lines up.
			n := 1
			getParams(p, &n)
			t.cursorCol = 0
			t.cursorLine = max(0, t.cursorLine-n)
		case 'G': // (copilot) Cursor HORIZONTAL ABSOLUTE
			n := 1
			getParams(p, &n)
			t.cursorCol = max(0, min(t.MaxCols-1, n-1))
		case 'H': // Cursor POSITION (col, line)
			line, col := 1, 1
			getParams(p, &line, &col)
			t.cursorCol = max(min(t.MaxCols-1, col-1), 0)
			t.cursorLine = max(min(t.MaxRows-1, line-1), 0)
		case 'd':
			n := 0
			getParams(p, &n)
			t.cursorLine = max(min(t.MaxRows-1, n-1), 0)

		// Display erase
		case 'J': // Erase in Display
			n := 0
			getParams(p, &n)
			switch n {
			case 0: // clear from cursor to end
				off := t.cursorCol + t.cursorLine*t.MaxCols
				for i := off; i < len(t.screen); i++ {
					t.screen[i] = Cell{}
				}
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				off := t.cursorCol + t.cursorLine*t.MaxCols
				for i := 0; i < off; i++ {
					t.screen[i] = Cell{}
				}
				t.cellUpdate++
			case 2: // clear everything
				t.screen = make([]Cell, t.MaxCols*t.MaxRows)
				t.cellUpdate++
			}
		case 'K': // Erase in Line
			n := 0
			getParams(p, &n)
			l := t.cursorLine * t.MaxCols
			line := t.screen[l : l+t.MaxCols]
			switch n {
			case 0: // clear from cursor to end
				for i := t.cursorCol; i < len(line); i++ {
					line[i] = Cell{}
				}
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				for i := 0; i < t.cursorCol; i++ {
					line[i] = Cell{}
				}
				t.cellUpdate++
			case 2: // clear everything
				for i := range line {
					line[i] = Cell{}
				}
				t.cellUpdate++
			}
		case 'M': // Delete lines, it will move the rest of the lines up
			n := 1
			getParams(p, &n)
			off := t.cursorCol + t.cursorLine*t.MaxCols
			copy(t.screen[off:], t.screen[off+n*t.MaxCols:])
			for i := len(t.screen) - n*t.MaxCols; i < len(t.screen); i++ {
				t.screen[i] = Cell{}
			}
			t.cellUpdate++
		case 'P': // Delete chars in line it will move the rest of the line to the left
			n := 1
			getParams(p, &n)
			l := t.cursorLine * t.MaxCols
			line := t.screen[l : l+t.MaxCols]

			copy(line[t.cursorCol:], line[t.cursorCol+n:])
			for i := len(line) - n; i < len(line); i++ {
				line[i] = Cell{}
			}
		case 'X': // Erase chars
			n := 0
			getParams(p, &n)
			off := t.cursorCol + t.cursorLine*t.MaxCols
			for i := off; i < min(off+n, len(t.screen)); i++ {
				t.screen[i] = Cell{}
			}
			t.cellUpdate++
		case 'L': // Insert lines, it will push lines forward
			n := 1
			getParams(p, &n)
			l := max(t.cursorLine-1, 0) * t.MaxCols
			e := l + n*t.MaxCols
			dup := slices.Clone(t.screen)
			for i := l; i < e; i++ {
				t.screen[i] = Cell{}
			}
			copy(t.screen[e:], dup[l:])
			t.cellUpdate++
		case '@':
			// TODO: {lpf} (comment by copilot: Insert blank characters (SP) (default = 1))
		// SGR
		case 'm':
			t.cstate.set(p...)
		case '>':
			// TODO: {lpf} (comment by copilot: DECRST)
		case 'u':
			t.cursorLine = t.saveCursor[0]
			t.cursorCol = t.saveCursor[1]
		case 's':
			t.saveCursor = [2]int{t.cursorLine, t.cursorCol}
		case 'c':
			// TODO: {lpf} (comment by copilot: Send device attributes)
		case 'h':
			switch p[0] {
			case 1004:
				// TODO: Turn focus report ON
			}
		case 'l':
			switch p[0] {
			case 1:
				// TODO: Turn cursor keys to application mode OFF
			}
		case 't':
			// TODO: {lpf} (comment by copilot: Window manipulation)
		case 'r':
			top, bottom := 0, t.MaxRows
			getParams(p, &top, &bottom)
			// TODO: {lpf} Setup scroll area
		case 'S': // Scrollup
			// TODO: {lpf} should this have an internal scroll?
		case 'T': // Scrolldown
			// TODO: {lpf} should this have an internal scroll?
		case '\x01':
			// no clue, might have been websocket related issues
		default:
			panic(fmt.Errorf("unknown CSI: %d %[1]c", r))

		}
		return (*Terminal).normal
	}
}

// TODO: {lpf} deal with scroll area
func (t *Terminal) nextLine() {
	if t.cursorLine == t.MaxRows-1 {
		copy(t.screen, t.screen[t.MaxCols:])
		for i := len(t.screen) - t.MaxCols; i < len(t.screen); i++ {
			t.screen[i] = Cell{}
		}
		t.cellUpdate++
	} else {
		t.cursorLine++
	}
}

// Updates returns a sequence number that is incremented every time the screen
// is updated
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
			fmt.Fprintln(buf) // not needed?!
		}
		c := t.screen[i]

		if c.cstate != lastState {
			lastState = c.cstate
			// different state, we shall reset and set the new state
			codes := []string{"0"}
			if c.Flags&FlagFG != 0 {
				codes = append(codes,
					fmt.Sprintf("38;2;%d;%d;%d", c.FG[0], c.FG[1], c.FG[2]),
				)
			}
			if c.Flags&FlagBG != 0 {
				if c.BG[0] != 0 || c.BG[1] != 0 || c.BG[2] != 0 {
					codes = append(codes,
						fmt.Sprintf("48;2;%d;%d;%d", c.BG[0], c.BG[1], c.BG[2]),
					)
				}
			}
			if c.Flags&FlagUnderlineColor != 0 {
				codes = append(codes,
					fmt.Sprintf("58;2;%d;%d;%d", c.UL[0], c.UL[1], c.UL[2]),
				)
			}
			if c.Flags&FlagBold != 0 {
				codes = append(codes, "1")
			}
			if c.Flags&FlagUnderline != 0 {
				codes = append(codes, "4")
			}
			if c.Flags&FlagBlink != 0 {
				codes = append(codes, "5")
			}
			if c.Flags&FlagInverse != 0 {
				codes = append(codes, "7")
			}
			if c.Flags&FlagInvisible != 0 {
				codes = append(codes, "8")
			}
			if c.Flags&FlagStrike != 0 {
				codes = append(codes, "9")
			}
			fmt.Fprintf(buf, "\033[%sm", strings.Join(codes, ";"))
		}

		r := c.Char
		if r == 0 {
			r = ' '
		}
		if x == t.cursorCol && y == t.cursorLine {
			fmt.Fprintf(buf, "\033[7m%c\033[0m", r)
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
