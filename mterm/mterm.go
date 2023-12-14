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

// Cell is a single cell in the terminal
type Cell struct {
	Char rune
	nl   bool // new: 2023-12-13 is new line
	sgrState
}

type stateFn func(t *Terminal, r rune) (stateFn, error)

// Terminal is an in memory terminal emulator
type Terminal struct {
	mux    sync.Mutex
	screen []Cell

	cstate sgrState

	stateProc stateFn

	cursorLine int
	cursorCol  int

	Title   string
	MaxCols int
	MaxRows int

	TabSize     int
	BacklogSize int

	cellUpdate int

	saveCursor   [2]int
	scrollRegion [2]int // startRow, endRow

	// holds partial input runes until is able to fully read
	part []byte
}

// New returns a new terminal with the given rows and cols
func New(rows, cols int) *Terminal {
	return &Terminal{
		MaxCols: cols,
		MaxRows: rows,

		cursorLine: 0,
		cursorCol:  0,

		screen: make([]Cell, rows*cols),

		TabSize:     8,
		BacklogSize: 1000,

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

	newScreen := make([]Cell, cols*rows)
	emptyRow := make([]Cell, cols)

	ni := 0
	addLine := func(line []Cell) {
		for i := 0; i < (len(line)-1)/cols; i++ {
			newScreen = append(newScreen, emptyRow...)
		}
		for len(newScreen)-ni < len(line) {
			newScreen = append(newScreen, emptyRow...)
		}
		copy(newScreen[ni:], line)
		ni += len(line)
		if len(line)%cols != 0 {
			ni += (cols - (ni % cols))
		}
	}

	start := 0
	for i := start; i < len(t.screen); i++ {
		c := t.screen[i]
		if !c.nl {
			continue
		}
		// add logical text line
		addLine(t.screen[start : i+1])
		start = i + t.MaxCols - (i % t.MaxCols)
	}
	end := start
	// just check if there's more to print until the end of the screen
	for i := start; i < len(t.screen); i++ {
		if t.screen[i].Char > ' ' {
			end = i
		}
	}
	addLine(t.screen[start:end])

	t.MaxRows = rows
	t.MaxCols = cols
	t.screen = newScreen
	t.cursorCol = min(t.cursorCol, cols-1)
	t.cursorLine = min(t.cursorLine, rows-1)
	t.saveCursor = [2]int{0, 0}
	t.scrollRegion = [2]int{0, rows} // reset?! or resize
}

// Clear clears the terminal moving cursor to 0,0
func (t *Terminal) Clear() {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.cursorLine = 0
	t.cursorCol = 0
	// reset virtual scroll as well
	t.screen = make([]Cell, t.MaxRows*t.MaxCols)
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

	return t.cursorLine, t.cursorCol
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

func (t *Terminal) nextLine() {
	t.cursorLine++
	switch {
	case t.cursorLine < t.scrollRegion[1]-1:
		return
	case t.cursorLine == t.scrollRegion[1] && t.scrollRegion[0] == 0:
		t.cursorLine--
		if len(t.screen)/t.MaxCols < t.BacklogSize {
			t.screen = append(t.screen, make([]Cell, t.MaxCols)...)
			// TODO: should be:
			// - insert a new line in offseted scrollRegion[1]
			// - copy the rest
			return
		} // else copy on region?
		region := t.screenScrollRegion()
		copy(region, region[t.MaxCols:])
		fill(region[len(region)-t.MaxCols:], Cell{})
	case t.cursorLine == t.scrollRegion[1]:
		t.cursorLine--
		region := t.screenScrollRegion()
		copy(region, region[t.MaxCols:])
		fill(region[len(region)-t.MaxCols:], Cell{})

	// Replicate odd xterm behaviour of when the cursor is outside of the region
	// it will not print neither scroll
	case t.cursorLine == t.MaxRows:
		t.cursorLine--
	}
}

func (t *Terminal) normal(r rune) (stateFn, error) {
	switch {
	case r == '\033':
		return (*Terminal).esc, nil
	case r == '\n':
		t.nextLine()
		t.cursorCol = 0

		screen := t.screenView()
		prevLine := t.cursorLine - 1
		// find current line ending (non space) and mark it as new line
		// clear any previous newlines marks on the line
		line := screen[prevLine*t.MaxCols : prevLine*t.MaxCols+t.MaxCols]
		mark := 0
		for i := range line {
			line[i].nl = false
			if line[i].Char > ' ' {
				mark = i
			}
		}
		line[mark].nl = true
	case r == '\r':
		t.cursorCol = 0
	case r == '\b':
		t.cursorCol = max(0, t.cursorCol-1)
	case r == '\t':
		t.cursorCol = (t.cursorCol + t.TabSize) / t.TabSize * t.TabSize
		t.cursorCol = min(t.cursorCol, t.MaxCols-1)
	case r < ' ': // least printable char, we ignore it
	// case !unicode.IsPrint(r):
	default:
		if t.cursorCol >= t.MaxCols {
			t.nextLine()
			t.cursorCol = 0
		}
		cl := Cell{
			Char:     r,
			sgrState: t.cstate,
		}
		screen := t.screenView()
		offs := t.cursorCol + t.cursorLine*t.MaxCols
		if offs >= len(screen) {
			// Rare, but to be safe..
			return nil, fmt.Errorf("offset out of bounds: %d", offs)
		}
		screen[offs] = cl
		t.cursorCol++
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
	case 'M': // Reverse Index (move cursor up, scrolling if needed)
		// TODO: scrolling
		t.cursorLine = max(0, t.cursorLine-1)
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
			t.cursorCol = clamp(n-1, 0, t.MaxCols-1)
		case 'H': // Cursor POSITION (col, line)
			line, col := 1, 1
			getParams(p, &line, &col)
			t.cursorCol = clamp(col-1, 0, t.MaxCols-1)
			t.cursorLine = clamp(line-1, 0, t.MaxRows-1)
		case 'd':
			n := 0
			getParams(p, &n)
			t.cursorLine = clamp(n-1, 0, t.MaxRows-1)
		// Display erase
		case 'J': // Erase in Display
			n := 0
			getParams(p, &n)

			screen := t.screenView()

			switch n {
			case 0: // clear from cursor to end
				off := t.cursorCol + t.cursorLine*t.MaxCols
				fill(screen[off:], Cell{sgrState: t.cstate})
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				off := t.cursorCol + t.cursorLine*t.MaxCols
				fill(screen[:off], Cell{sgrState: t.cstate})
				t.cellUpdate++
			case 2: // clear everything
				// t.screen = t.screen[:t.MaxRows*t.MaxCols]
				fill(screen, Cell{sgrState: t.cstate})
				t.cellUpdate++
			}
		case 'K': // Erase in Line
			n := 0
			getParams(p, &n)

			screen := t.screenView()

			l := clamp(t.cursorLine, 0, t.MaxRows) * t.MaxCols
			line := screen[l : l+t.MaxCols]
			switch n {
			case 0: // clear from cursor to end
				fill(line[t.cursorCol:], Cell{sgrState: t.cstate})
				t.cellUpdate++
			case 1: // clear from beginning to cursor
				fill(line[:t.cursorCol], Cell{sgrState: t.cstate})
				t.cellUpdate++
			case 2: // clear everything
				fill(line, Cell{sgrState: t.cstate})
				t.cellUpdate++
			}
		case 'M': // Delete lines, it will move the rest of the lines up
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			lr := max(t.cursorLine, 0)
			loff := clamp(lr*t.MaxCols, 0, len(region))
			eoff := clamp(loff+n*t.MaxCols, 0, len(region))
			copy(region[loff:], region[eoff:])
			fill(region[len(region)-n*t.MaxCols:], Cell{})
			t.cellUpdate++
		case 'P': // Delete chars in line it will move the rest of the line to the left
			n := 1
			getParams(p, &n)

			screen := t.screenView()

			l := t.cursorLine * t.MaxCols
			line := screen[l : l+t.MaxCols]

			copy(line[t.cursorCol:], line[t.cursorCol+n:])
			fill(line[len(line)-n:], Cell{})
		case 'X': // Erase chars
			n := 0
			getParams(p, &n)

			screen := t.screenView()

			off := t.cursorCol + t.cursorLine*t.MaxCols
			end := min(off+n, len(screen))
			fill(screen[off:end], Cell{sgrState: t.cstate})
			t.cellUpdate++
		case 'L': // Insert lines, it will push lines forward
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			lr := max(t.cursorLine, 0)
			loff := clamp(lr*t.MaxCols, 0, len(region))
			eoff := clamp(loff+n*t.MaxCols, 0, len(region))
			dup := slices.Clone(region)
			copy(region[eoff:], dup[loff:])
			fill(region[loff:eoff], Cell{sgrState: t.cstate})
			t.cellUpdate++
		case '@':
			// TODO: {lpf} (comment by copilot: Insert blank characters (SP) (default = 1))
		// SGR
		case 'm':
			err := t.cstate.set(p...)
			return (*Terminal).normal, err
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
				t.cursorLine = 0
				t.cursorCol = 0
			case len(p) == 1:
				t.cursorLine = 0
				t.cursorCol = 0
			default:
				// fill(t.screen, Cell{})
				// t.cursorLine = 0
				// t.cursorCol = 0
			}
		case 'S': // Scrollup
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			copy(region, region[n*t.MaxCols:])
			fill(region[len(region)-n*t.MaxCols:], Cell{})
		case 'T': // Scrolldown
			n := 1
			getParams(p, &n)

			region := t.screenScrollRegion()

			copy(region[n*t.MaxCols:], region)
			fill(region[:n*t.MaxCols], Cell{})
		default:
			return (*Terminal).normal, fmt.Errorf("unknown CSI: %d %[1]c", r)

		}
		return (*Terminal).normal, nil
	}
}

func (t *Terminal) screenScrollRegion() []Cell {
	screen := t.screenView()
	start := clamp(t.scrollRegion[0]*t.MaxCols, 0, len(screen))
	end := clamp(t.scrollRegion[1]*t.MaxCols, 0, len(screen))

	return screen[start:end]
}

func (t *Terminal) screenView() []Cell {
	start := max(len(t.screen)-t.MaxRows*t.MaxCols, 0)
	end := len(t.screen)

	return t.screen[start:end]
}

func (t *Terminal) getScreenAsAnsi(cursor bool) []byte {
	buf := bytes.NewBuffer(nil)
	x, y := 0, 0
	lastState := sgrState{}
	screen := t.screenView()
	for i := range screen {
		if x >= t.MaxCols {
			y++
			x = 0
			buf.WriteString("\r\n")
			lastState = sgrState{}
		}
		c := screen[i]
		if c.sgrState != lastState {
			lastState = c.sgrState
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

		if cursor && x == t.cursorCol && y == t.cursorLine {
			fmt.Fprintf(buf, "\033[7m%c\033[27m", r)
			x += 1
			lastState = sgrState{}
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
