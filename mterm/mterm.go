package mterm

import (
	"bytes"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type stateFn func(t *Terminal, r rune) (stateFn, error)

// Terminal is an in memory terminal emulator
type Terminal struct {
	mux          sync.Mutex
	screens      [2]*Grid
	screenTarget int

	cstate SGRState

	stateProc stateFn

	Title      string
	TabSize    int
	cellUpdate int

	// to handle CSIs CSIu
	saveCursor   [2]int
	scrollRegion [2]int // startRow, endRow

	// holds partial input runes until is able to fully read
	part []byte
}

// New returns a new terminal with the given rows and cols
func New(rows, cols int) *Terminal {
	return &Terminal{
		// cursorLine: 0,
		// cursorCol:  0,

		screens: [2]*Grid{
			{
				cells: make([]Cell, rows*cols),
				size: [2]int{
					rows,
					cols,
				},
				backlogSize: 1000,
			},
		},

		TabSize: 8,
		// BacklogSize: 1000,

		stateProc:    (*Terminal).normal,
		scrollRegion: [2]int{0, rows},
	}
}

// Cells returns a copy of the underlying screen cells
func (t *Terminal) Cells() []Cell {
	t.mux.Lock()
	defer t.mux.Unlock()

	return slices.Clone(t.screens[t.screenTarget].cells)
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

	for i := range p {
		t.part = append(t.part, p[i])
		if !utf8.FullRune(t.part) {
			continue
		}
		r, _ := utf8.DecodeRune(t.part)
		t.part = t.part[:0]
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

func (t *Terminal) Resize(rows, cols int) {
	t.mux.Lock()
	defer t.mux.Unlock()
	if cols == 0 || rows == 0 {
		// What to do?!
		return
	}

	switch t.screenTarget {
	case 0:
		t.screens[0].ResizeAndReflow(rows, cols)
	case 1:
		t.screens[1].Resize(rows, cols)
	}
	t.saveCursor = [2]int{0, 0}
	t.scrollRegion = [2]int{0, rows} // reset?! or resize
}

// Clear clears the terminal moving cursor to 0,0
func (t *Terminal) Clear() {
	t.mux.Lock()
	defer t.mux.Unlock()

	s := t.screens[t.screenTarget]
	s.cursor = [2]int{}
	sz := s.size
	// reset virtual scroll as well
	t.screens[0].cells = make([]Cell, sz[0]*sz[1])
}

func (t *Terminal) GetScreenAsAnsi() []byte {
	t.mux.Lock()
	defer t.mux.Unlock()

	return t.getScreenAsAnsi(false)
}

// Updates returns a sequence number that is incremented every time the screen
// cells are updated
func (t *Terminal) Updates() int {
	t.mux.Lock()
	defer t.mux.Unlock()

	return t.cellUpdate
}

// GetCursorPos returns the current cursor position in lines, cols
func (t *Terminal) CursorPos() (int, int) {
	t.mux.Lock()
	defer t.mux.Unlock()

	s := t.screens[t.screenTarget]
	return s.cursor[0], s.cursor[1]
}

func (t *Terminal) put(r rune) error {
	// Default to normal stateFn

	// TODO: Figure this out, vim/compterm client ocasionally sends this in any state!?
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

// maybe this move to grid.go
func (t *Terminal) nextLine() {
	s := t.screens[t.screenTarget]
	rows, cols := s.Size()

	s.cursor[0]++
	switch {
	case s.cursor[0] < t.scrollRegion[1]-1:
		return
	case s.cursor[0] == t.scrollRegion[1] && t.scrollRegion[0] == 0:
		s.cursor[0]--
		if t.screenTarget == 0 && len(s.cells)/cols < s.backlogSize {
			t.screens[0].cells = append(t.screens[0].cells, make([]Cell, cols)...)
			// TODO: should be:
			// - insert a new line in offseted scrollRegion[1]
			// - copy the rest
			return
		} // else copy on region?
		region := t.screenScrollRegion()
		copy(region, region[cols:])
		fill(region[len(region)-cols:], Cell{})
	case s.cursor[0] == t.scrollRegion[1]:
		s.cursor[0]--
		region := t.screenScrollRegion()
		if len(region)/cols < s.backlogSize {
			region = append(region, make([]Cell, cols)...)
			t.cellUpdate++
		}
		copy(region, region[cols:])
		fill(region[len(region)-cols:], Cell{})

	// Replicate odd xterm behaviour of when the cursor is outside of the region
	// it will not print neither scroll
	case s.cursor[0] == rows:
		s.cursor[0]--
	}
}

func (t *Terminal) normal(r rune) (stateFn, error) {
	s := t.screens[t.screenTarget]
	_, cols := s.size[0], s.size[1]
	switch {
	case r == '\033':
		return (*Terminal).esc, nil
	case r == '\n':
		t.nextLine()
		s.cursor[1] = 0

		// screen := t.screenView()
		prevLine := s.cursor[0] - 1
		// find current line ending (non space) and mark it as new line
		// clear any previous newlines marks on the line
		line := t.screenLine(prevLine) // screen[prevLine*cols : prevLine*cols+cols]
		mark := 0
		for i := range line {
			line[i].nl = false
			if line[i].Char > ' ' {
				mark = i
			}
		}
		line[mark].nl = true
	case r == '\r':
		s.cursor[1] = 0
	case r == '\b':
		s.cursor[1] = max(0, s.cursor[1]-1)
	case r == '\t':
		s.cursor[1] = (s.cursor[1] + t.TabSize) / t.TabSize * t.TabSize
		s.cursor[1] = min(s.cursor[1], cols-1)
	case r < ' ': // least printable char, we ignore it
	// case !unicode.IsPrint(r):
	default:
		if s.cursor[1] >= cols {
			t.nextLine()
			s.cursor[1] = 0
		}
		cl := Cell{
			Char:     r,
			SGRState: t.cstate,
		}
		screen := t.screenView()
		offs := s.cursor[1] + s.cursor[0]*cols
		if offs < 0 || offs >= len(screen) {
			// Rare, but to be safe..
			return nil, fmt.Errorf("offset out of bounds: %d", offs)
		}
		screen[offs] = cl
		s.cursor[1]++
		t.cellUpdate++
	}
	return nil, nil
}

func (t *Terminal) esc(r rune) (stateFn, error) {
	s := t.screens[t.screenTarget]
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
	case 'M': // Reverse Index (move cursor up, scrolling if needed)
		s.cursor[0] = max(0, s.cursor[0]-1)
	case 'k':
		return t.captureString(func(s string) stateFn {
			// ignore string
			return (*Terminal).normal
		}), nil
	case '\\':
		// TODO: {lpf} (completed by copilot: String Terminator (ST))
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

// State to capture a string until String terminator ESC\\
// once done it will call the given function with the captured string
func (t *Terminal) captureString(fn func(s string) stateFn) stateFn {
	sb := strings.Builder{}
	esc := false
	return func(_ *Terminal, r rune) (stateFn, error) {
		switch {
		case r == '\033':
			esc = true
		case r == '\\' && esc:
			return fn(sb.String()), nil
		default:
			esc = false
			sb.WriteRune(r)
		}
		return nil, nil
	}
}

// State Operating System Command
func (t *Terminal) osc() stateFn {
	attrbuf := bytes.NewBuffer(nil)
	title := &strings.Builder{}
	esc := false
	return func(_ *Terminal, r rune) (stateFn, error) {
		if r == ';' || unicode.IsNumber(r) {
			esc = false
			attrbuf.WriteRune(r)
			return nil, nil
		}
		switch {
		case r == '\a' || (r == '\\' && esc):
			t.Title = title.String()
			return (*Terminal).normal, nil
		// Handle string terminator "\033\\"
		case r == '\033': // string terminator
			esc = true
			return nil, nil
		default:
			title.WriteRune(r)
		}
		esc = false
		return nil, nil
	}
}

// to handle cases like "\033[>P;N..." (cursor keys to application mode)
func (t *Terminal) csiGT() stateFn {
	// attrbuf := bytes.NewBuffer(nil)
	return func(_ *Terminal, r rune) (stateFn, error) {
		if r == ';' || unicode.IsNumber(r) {
			// TODO: implement as params
			// attrbuf.WriteRune(r){
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
	s := t.screens[t.screenTarget]
	rows, cols := s.size[0], s.size[1]
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
			s.cursor[0] = max(0, s.cursor[0]-n)
		case 'B': // Cursor DOWN
			n := 1
			getParams(p, &n)
			s.cursor[0] = min(rows-1, s.cursor[0]+n)
		case 'C': // Cursor FORWARD
			n := 1
			getParams(p, &n)
			s.cursor[1] = min(cols-1, s.cursor[1]+n)
		case 'D': // Cursor BACK
			n := 1
			getParams(p, &n)
			s.cursor[1] = max(0, s.cursor[1]-n)
		case 'E': // (copilot) Moves cursor to beginning of the line n (default 1) lines down.
			n := 1
			getParams(p, &n)
			s.cursor[1] = 0
			s.cursor[0] = min(rows-1, s.cursor[0]+n)
		case 'F': // (copilot)  Moves cursor to beginning of the line n (default 1) lines up.
			n := 1
			getParams(p, &n)
			s.cursor[1] = 0
			s.cursor[0] = max(0, s.cursor[0]-n)
		case 'G': // (copilot) Cursor HORIZONTAL ABSOLUTE
			n := 1
			getParams(p, &n)
			s.cursor[1] = clamp(n-1, 0, cols-1)
		case 'H': // Cursor POSITION (col, line)
			line, col := 1, 1
			getParams(p, &line, &col)
			s.cursor[1] = clamp(col-1, 0, cols-1)
			s.cursor[0] = clamp(line-1, 0, rows-1)
		case 'd':
			n := 0
			getParams(p, &n)
			s.cursor[0] = clamp(n-1, 0, rows-1)
		// Display erase
		case 'J': // Erase in Display
			n := 0
			getParams(p, &n)

			screen := t.screenView()

			switch n {
			case 0: // clear from cursor to end
				off := clamp(s.cursor[1]+s.cursor[0]*cols, 0, len(screen))
				fill(screen[off:], Cell{SGRState: t.cstate})
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				off := clamp(s.cursor[1]+s.cursor[0]*cols, 0, len(screen))
				fill(screen[:off], Cell{SGRState: t.cstate})
				t.cellUpdate++
			case 2: // clear everything
				fill(screen, Cell{SGRState: t.cstate})
				t.cellUpdate++
			case 3: // clear scrollback
				if t.screenTarget == 1 {
					break
				}
				if len(t.screens[0].cells) <= rows*cols {
					break
				}
				copy(t.screens[0].cells, screen)
				t.screens[0].cells = t.screens[0].cells[:rows*cols]
				t.cellUpdate++
			}
		case 'K': // Erase in Line
			n := 0
			getParams(p, &n)

			// screen := t.screenView()

			// l := clamp(s.cursor[0], 0, rows) * cols
			line := t.screenLine(s.cursor[0]) // screen[l : l+cols]
			//line := screen[l : l+cols]
			switch n {
			case 0: // clear from cursor to end
				fill(line[s.cursor[1]:], Cell{SGRState: t.cstate})
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				fill(line[:s.cursor[1]], Cell{SGRState: t.cstate})
				t.cellUpdate++
			case 2: // clear everything
				fill(line, Cell{SGRState: t.cstate})
				t.cellUpdate++
			}
		case 'M': // Delete lines, it will move the rest of the lines up
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			lr := max(s.cursor[0], 0)
			loff := clamp(lr*cols, 0, len(region))
			eoff := clamp(loff+n*cols, 0, len(region))
			copy(region[loff:], region[eoff:])
			fill(region[len(region)-n*cols:], Cell{})
			t.cellUpdate++
		case 'P': // Delete chars in line it will move the rest of the line to the left
			n := 1
			getParams(p, &n)

			line := t.screenLine(s.cursor[0])

			copy(line[s.cursor[1]:], line[s.cursor[1]+n:])
			fill(line[len(line)-n:], Cell{})
		case 'X': // Erase chars
			n := 0
			getParams(p, &n)

			screen := t.screenView()

			off := s.cursor[1] + s.cursor[0]*cols
			end := min(off+n, len(screen))
			fill(screen[off:end], Cell{SGRState: t.cstate})
			t.cellUpdate++
		case 'L': // Insert lines, it will push lines forward
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			lr := max(s.cursor[0], 0)
			loff := clamp(lr*cols, 0, len(region))
			eoff := clamp(loff+n*cols, 0, len(region))
			dup := slices.Clone(region)
			copy(region[eoff:], dup[loff:])
			fill(region[loff:eoff], Cell{SGRState: t.cstate})
			t.cellUpdate++
		case '@':
			// TODO: {lpf} (comment by copilot: Insert blank characters (SP) (default = 1))
		// SGR
		case 'm':
			err := t.cstate.Set(p...)
			return (*Terminal).normal, err
		case 'u':
			s.cursor[0] = t.saveCursor[0]
			s.cursor[1] = t.saveCursor[1]
		case 's':
			t.saveCursor = [2]int{s.cursor[0], s.cursor[1]}
		case 'c':
			// TODO: {lpf} (comment by copilot: Send device attributes)
		case 'h':
			switch p[0] {
			// enter private mode
			case 1049:
				t.screens[1] = &Grid{
					cells:  make([]Cell, rows*cols),
					size:   [2]int{rows, cols},
					cursor: t.screens[0].cursor,
				}
				t.screenTarget = 1

			case 1004:
				// TODO: Turn focus report ON
			}
		case 'l':
			switch p[0] {
			// restore private mode
			case 1049:
				t.screens[0].ResizeAndReflow(rows, cols)
				t.screenTarget = 0
			case 25: // hide cursor if first rune is '?'
			case 1:
				// TODO: Turn cursor keys to application mode OFF
			}
		case 't':
			// TODO: {lpf} (comment by copilot: Window manipulation)
		case 'r':
			top, bottom := 1, rows
			getParams(p, &top, &bottom)

			switch {
			// Invert order if top is bigger (alacritty)
			case top > bottom:
				top, bottom = bottom, top
			// Disable scrollRegion if equal (alacritty, xterm)
			case top == bottom:
				top, bottom = 1, rows
			}

			t.scrollRegion[0] = clamp(top-1, 0, rows)
			t.scrollRegion[1] = clamp(bottom, 0, rows)

			// TODO: this needs some love, it's not working as expected
			// some cases it resets cursor, others resets the whole screen
			switch {
			case len(p) == 0:
				// fill(t.screen, Cell{})
				// Reset backScroll too
				s.cursor = [2]int{}
			case len(p) == 1:
				s.cursor = [2]int{}
			default:
				// fill(t.screen, Cell{})
				// s.cursor[0] = 0
				// s.cursor[1] = 0
			}
		case 'S': // Scrollup
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			copy(region, region[n*cols:])
			fill(region[len(region)-n*cols:], Cell{})
		case 'T': // Scrolldown
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			copy(region[n*cols:], region)
			fill(region[:n*cols], Cell{})
		default:
			return (*Terminal).normal, fmt.Errorf("unknown CSI: %d %[1]c", r)

		}
		return (*Terminal).normal, nil
	}
}

func (t *Terminal) screenLine(n int) []Cell {
	s := t.screens[t.screenTarget]
	n = clamp(n, 0, s.size[0]-1)

	return s.cells[n*s.size[1] : n*s.size[1]+s.size[1]]
}

func (t *Terminal) screenScrollRegion() []Cell {
	s := t.screens[t.screenTarget]
	cols := s.size[1]
	screen := t.screenView()
	start := clamp(t.scrollRegion[0]*cols, 0, len(screen))
	end := clamp(t.scrollRegion[1]*cols, 0, len(screen))

	return screen[start:end]
}

func (t *Terminal) screenView() []Cell {
	s := t.screens[t.screenTarget]
	rows, cols := s.size[0], s.size[1]

	start := max(len(s.cells)-rows*cols, 0)
	end := len(s.cells)

	return s.cells[start:end]
}

func (t *Terminal) getScreenAsAnsi(cursor bool) []byte {
	s := t.screens[t.screenTarget]
	cols := s.size[1]

	buf := bytes.NewBuffer(nil)
	x, y := 0, 0
	lastState := SGRState{}
	screen := t.screenView()
	for i := range screen {
		if x >= cols {
			y++
			x = 0
			buf.WriteString("\r\n")
			lastState = SGRState{}
		}
		c := screen[i]
		if c.SGRState != lastState {
			lastState = c.SGRState
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
			// underline
			switch (c.ColorType >> 4) & 0b11 {
			case Color256:
				fmt.Fprintf(buf, ";58;5;%d", c.UL[0])
			case Color16M:
				fmt.Fprintf(buf, ";58;2;%d;%d;%d", c.UL[0], c.UL[1], c.UL[2])
			}
			if c.Flags&FlagBold != 0 {
				buf.WriteString(";1")
			}
			if c.Flags&FlagDim != 0 {
				buf.WriteString(";2")
			}
			if c.Flags&FlagItalic != 0 {
				buf.WriteString(";3")
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
		if r < ' ' {
			r = ' '
		}

		if cursor && x == s.cursor[1] && y == s.cursor[0] {
			fmt.Fprintf(buf, "\033[7m%c\033[27m", r)
			x += 1
			lastState = SGRState{}
			continue
		}
		buf.WriteRune(r)
		x += 1
	}
	return buf.Bytes()
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

	return t.getScreenAsAnsi(true)
}
