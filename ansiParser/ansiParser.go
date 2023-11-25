package ansiParser

import (
	"bytes"
	"strconv"
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

type ANSIParser struct {
	buffer bytes.Buffer
	mux    sync.Mutex

	cursorLine int
	cursorCol  int

	MaxCols int

	state int

	CSIParams string
}

func New() *ANSIParser {
	return &ANSIParser{
		state: NORMAL,
	}
}

func (ap *ANSIParser) Read(p []byte) (int, error) {
	return ap.buffer.Read(p)
}

func isCSIFinal(b byte) bool {
	return b >= 0x40 && b <= 0x7E
}

func (ap *ANSIParser) Write(p []byte) (int, error) {
	var err error
	ap.mux.Lock()
	defer ap.mux.Unlock()

	for _, b := range p {
		switch ap.state {
		case NORMAL:
			switch b {
			case '\t':
				ap.cursorCol += 4 // TODO: get tab size from terminal
				ap.buffer.WriteByte(b)
			case '\b':
				if ap.cursorCol == 0 {
					if ap.cursorLine > 0 {
						ap.cursorLine--
					}
				} else {
					ap.cursorCol--
				}
				ap.buffer.WriteByte(b)
			case '\r':
				ap.cursorCol = 0
				ap.buffer.WriteByte(b)
			case '\n':
				ap.cursorLine++
				ap.buffer.WriteByte(b)
			case '\x1b':
				ap.state = ESCAPE
				ap.buffer.WriteByte(b)
			case '\x7f': // DEL
				ap.buffer.WriteByte(b)
			case '\x00': // NUL
				ap.buffer.WriteByte(b)
			default:
				if ap.cursorCol >= ap.MaxCols {
					ap.cursorCol = 0
					ap.buffer.Write([]byte{'\r', '\n'})
				}
				ap.cursorCol++
			}
		case ESCAPE:
			switch b {
			case '[':
				ap.state = CSI
				ap.buffer.WriteByte(b)
			case ']':
				ap.state = OSC
				ap.buffer.WriteByte(b)
			case 'P':
				ap.state = DCS
				ap.buffer.WriteByte(b)
			case 'X':
				ap.state = IGNORE
				ap.buffer.WriteByte(b)
			case CSI:
				// need to acumulate the parameters and execute the command when we get the final byte
				// example CSI n E where n is the parameter and E is the command (cursor down n lines)
				switch {
				case b >= 0x40 && b <= 0x7E: // CSI final byte (execute command)
					switch b {
					case 'A': // cursor up
						ap.state = NORMAL
						ap.buffer.WriteByte(b)
						// parse the parameter
						n := 1
						if ap.CSIParams != "" {
							n, err = strconv.Atoi(ap.CSIParams)
							if err != nil {
								n = 1
							}
						}
						ap.cursorLine -= n
						if ap.cursorLine < 0 {
							ap.cursorLine = 0
						}

						// clear the parameter
						ap.CSIParams = ""
					}
				default:
					//ap.state = CSI
					ap.buffer.WriteByte(b)
					ap.CSIParams += string(b)
				}
			case OSC:
			case DCS:
			case ST:
				ap.state = NORMAL
			case IGNORE:
			}
		default:
			ap.state = NORMAL
		}

	}
	return len(p), nil
}
