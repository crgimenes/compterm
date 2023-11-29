package mterm

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	FlagFG byte = 1 << iota
	FlagBG
	FlagBold
	FlagUnderline
	FlagBlink
	FlagInverse
	FlagInvisible
	FlagStrike
)

type Color [3]byte

type cstate struct {
	FG    Color
	BG    Color
	flags byte // maybe bigger to accomodate more flags
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

	MaxCols int
	MaxRows int

	TabSize int
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
			if c.flags&FlagFG != 0 {
				codes = append(codes,
					fmt.Sprintf("38;2;%d;%d;%d", c.FG[0], c.FG[1], c.FG[2]),
				)
			}
			if c.flags&FlagBG != 0 {
				if c.BG[0] != 0 || c.BG[1] != 0 || c.BG[2] != 0 {
					codes = append(codes,
						fmt.Sprintf("48;2;%d;%d;%d", c.BG[0], c.BG[1], c.BG[2]),
					)
				}
			}
			if c.flags&FlagBold != 0 {
				codes = append(codes, "1")
			}
			if c.flags&FlagUnderline != 0 {
				codes = append(codes, "4")
			}
			if c.flags&FlagBlink != 0 {
				codes = append(codes, "5")
			}
			if c.flags&FlagInverse != 0 {
				codes = append(codes, "7")
			}
			if c.flags&FlagInvisible != 0 {
				codes = append(codes, "8")
			}
			if c.flags&FlagStrike != 0 {
				codes = append(codes, "9")
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
	switch r {
	case '\033':
		return (*Terminal).esc
	case '\n':
		t.cursorCol = 0
		// "scroll"
		// Could implement a func out of this
		// to be used both in default and here
		t.nextLine()
	case '\r':
		t.cursorCol = 0
	case '\t':
		// round to a multiple of tabSize for better alignment
		t.cursorCol = (t.cursorCol + t.TabSize) / t.TabSize * t.TabSize
		t.cursorCol = min(t.cursorCol, t.MaxCols-1)
	default:
		cl := Cell{
			Char:   r,
			cstate: t.cstate,
		}
		offs := t.cursorCol + t.cursorLine*t.MaxCols
		t.screen[offs] = cl
		t.cursorCol++
		if t.cursorCol >= t.MaxCols {
			t.cursorCol = 0
			t.nextLine()
		}
	}
	return nil
}

func (t *Terminal) nextLine() {
	if t.cursorLine == t.MaxRows-1 {
		copy(t.screen, t.screen[t.MaxCols:])
		for i := len(t.screen) - t.MaxCols; i < len(t.screen); i++ {
			t.screen[i] = Cell{}
		}
	} else {
		t.cursorLine++
	}
}

func (t *Terminal) esc(r rune) stateFn {
	switch r {
	case '[':
		return t.csi()
		// More escape sequences
	}
	return (*Terminal).normal
}

func (t *Terminal) csi() stateFn {
	attrbuf := bytes.NewBuffer(nil)

	mkparam := func() []int {
		if attrbuf.Len() == 0 {
			return nil
		}
		defer attrbuf.Reset()
		s := attrbuf.String()
		ps := strings.Split(s, ";")
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
		if r == ';' || unicode.IsNumber(r) {
			attrbuf.WriteRune(r)
			return nil
		}

		p := mkparam()
		switch r {
		// Cursor movement
		case 'A': // Cursor UP
			n := 1
			if len(p) > 0 {
				n = p[0]
			}
			t.cursorLine = max(0, t.cursorLine-n)
		case 'B': // Cursor DOWN
			n := 1
			if len(p) > 0 {
				n = p[0]
			}
			t.cursorLine = min(t.MaxRows-1, t.cursorLine+n)
		case 'C': // Cursor FORWARD
			n := 1
			if len(p) > 0 {
				n = p[0]
			}
			t.cursorCol = min(t.MaxCols-1, t.cursorCol+n)
		case 'D': // Cursor BACK
			n := 1
			if len(p) > 0 {
				n = p[0]
			}
			t.cursorCol = max(0, t.cursorCol-n)
		case 'H': // Cursor POSITION (col, line)
			col, line := 0, 0
			if len(p) > 0 {
				// careful with 0 or missing param
				line = p[0]
			}
			if len(p) > 1 {
				col = p[1]
			}
			t.cursorCol = max(min(t.MaxCols-1, col), 0)
			t.cursorLine = max(min(t.MaxRows-1, line), 0)
		case 'J': // Erase in Display
			switch {
			case len(p) == 0 || p[0] == 0:
				off := t.cursorCol + t.cursorLine*t.MaxCols
				for i := 0; i < off; i++ {
					t.screen[i] = Cell{}
				}
			case p[0] == 1:
				off := t.cursorCol + t.cursorLine*t.MaxCols
				for i := off; i < len(t.screen); i++ {
					t.screen[i] = Cell{}
				}
			case p[0] == 2:
				t.screen = make([]Cell, t.MaxCols*t.MaxRows)
			}
		case 'K': // Erase in Line
			switch {
			case len(p) == 0 || p[0] == 0:
				off := t.cursorCol + t.cursorLine*t.MaxCols
				for i := off; i < off+t.MaxCols; i++ {
					t.screen[i] = Cell{}
				}
			case p[0] == 1:
				off := t.cursorCol + t.cursorLine*t.MaxCols
				for i := off - t.MaxCols; i < off; i++ {
					t.screen[i] = Cell{}
				}
			case p[0] == 2:
				off := t.cursorLine * t.MaxCols
				for i := off; i < off+t.MaxCols; i++ {
					t.screen[i] = Cell{}
				}
			}
		// SGR
		case 'm':
			for len(p) > 0 {
				switch {
				case p[0] == 0:
					t.cstate.flags = 0
					p = p[1:]
				case p[0] == 1:
					t.cstate.flags |= FlagBold
					p = p[1:]
				case p[0] == 4:
					t.cstate.flags |= FlagUnderline
					p = p[1:]
				case p[0] == 5:
					t.cstate.flags |= FlagBlink
					p = p[1:]
				case p[0] == 7:
					t.cstate.flags |= FlagInverse
					p = p[1:]
				case p[0] == 8:
					t.cstate.flags |= FlagInvisible
					p = p[1:]
				case p[0] == 9:
					t.cstate.flags |= FlagStrike
					p = p[1:]
				case p[0] == 22:
					t.cstate.flags &= ^FlagBold
					p = p[1:]
				case p[0] >= 30 && p[0] <= 37:
					t.cstate.flags |= FlagFG
					bits := p[0] - 30
					t.cstate.FG = [3]byte{}
					if bits&0b001 != 0 {
						t.cstate.FG[0] = 155
					}
					if bits&0b010 != 0 {
						t.cstate.FG[1] = 155
					}
					if bits&0b100 != 0 {
						t.cstate.FG[2] = 155
					}
					p = p[1:]
				case p[0] >= 40 && p[0] <= 47:
					t.cstate.flags |= FlagBG
					bits := p[0] - 40
					t.cstate.BG = [3]byte{}
					if bits&0b001 != 0 {
						t.cstate.BG[0] = 155
					}
					if bits&0b010 != 0 {
						t.cstate.BG[1] = 155
					}
					if bits&0b100 != 0 {
						t.cstate.BG[2] = 155
					}
					p = p[1:]
				// 256 TODO: {lpf} implement a 256 color map
				// case p[0] == 38 && len(p) > 2 && p[1] == 5:
				// 	t.cstate.flags |= FlagFG
				// 	t.cstate.FG = Color256[p[2]]
				// 	p = p[3:]
				// case p[0] == 48 && len(p) > 2 && p[1] == 5:
				// 	t.cstate.flags |= FlagBG
				// 	t.cstate.BG = Color256[p[2]]
				// 	p = p[3:]
				// 16M
				case p[0] == 38 && len(p) > 4 && p[1] == 2:
					t.cstate.flags |= FlagFG
					t.cstate.FG = Color{byte(p[2]), byte(p[3]), byte(p[4])}
					p = p[5:]
				// 16M
				case p[0] == 48 && len(p) > 4 && p[1] == 2:
					t.cstate.flags |= FlagBG
					t.cstate.BG = Color{byte(p[2]), byte(p[3]), byte(p[4])}
					p = p[5:]
				}
			}
		}
		return (*Terminal).normal
	}
}

// DBG Similar to GetScreenAsAnsi but with cursor
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
		// draw cursor
		c := t.screen[i]

		if c.cstate != lastState {
			lastState = c.cstate
			codes := []string{"0"}
			// different state, we shall reset and set the new state
			if c.flags&FlagFG != 0 {
				codes = append(codes,
					fmt.Sprintf("38;2;%d;%d;%d", c.FG[0], c.FG[1], c.FG[2]),
				)
			}
			if c.flags&FlagBG != 0 {
				if c.BG[0] != 0 || c.BG[1] != 0 || c.BG[2] != 0 {
					codes = append(codes,
						fmt.Sprintf("48;2;%d;%d;%d", c.BG[0], c.BG[1], c.BG[2]),
					)
				}
			}
			if c.flags&FlagBold != 0 {
				codes = append(codes, "1")
			}
			if c.flags&FlagUnderline != 0 {
				codes = append(codes, "4")
			}
			if c.flags&FlagBlink != 0 {
				codes = append(codes, "5")
			}
			if c.flags&FlagInverse != 0 {
				codes = append(codes, "7")
			}
			if c.flags&FlagInvisible != 0 {
				codes = append(codes, "8")
			}
			if c.flags&FlagStrike != 0 {
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
