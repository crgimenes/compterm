package screen

// clipboardFilter strips OSC 52 (clipboard) escape sequences from a byte
// stream so the host's clipboard is never broadcast to viewers. It is stateful
// to handle sequences split across writes.
//
// An OSC sequence is "ESC ] <params> <terminator>", where the terminator is BEL
// (0x07) or ST (ESC \). OSC 52 is "ESC ] 52 ; ...". Every other OSC sequence,
// and all other bytes, pass through unchanged.
type clipboardFilter struct {
	state fState
	match int // matched bytes of osc52Prefix while classifying an OSC
}

type fState int

const (
	fNormal  fState = iota // ordinary bytes
	fEsc                   // saw ESC
	fOSC                   // saw "ESC ]", classifying the parameter
	fPass                  // OSC that is not 52: passing through to its terminator
	fPassEsc               // in fPass, saw ESC (maybe ST)
	fDrop                  // OSC 52: dropping to its terminator
	fDropEsc               // in fDrop, saw ESC (maybe ST)
)

const (
	escByte       = 0x1b
	belByte       = 0x07
	osc52Prefix   = "52;"
	oscIntroducer = ']'
	stFinal       = '\\'
)

// filter appends the cleaned form of p to dst and returns the result.
func (f *clipboardFilter) filter(dst, p []byte) []byte {
	for _, b := range p {
		switch f.state {
		case fNormal:
			if b == escByte {
				f.state = fEsc
				continue
			}
			dst = append(dst, b)

		case fEsc:
			if b == oscIntroducer {
				f.state = fOSC
				f.match = 0
				continue
			}
			dst = append(dst, escByte) // the held ESC was not an OSC introducer
			if b == escByte {
				continue // a new ESC: stay in fEsc
			}
			dst = append(dst, b)
			f.state = fNormal

		case fOSC:
			if f.match < len(osc52Prefix) && b == osc52Prefix[f.match] {
				f.match++
				if f.match == len(osc52Prefix) {
					f.state = fDrop // confirmed OSC 52
				}
				continue
			}
			// Not OSC 52: emit what was held, then pass through.
			dst = append(dst, escByte, oscIntroducer)
			dst = append(dst, osc52Prefix[:f.match]...)
			f.state = fPass
			dst = f.passByte(dst, b)

		case fPass:
			dst = f.passByte(dst, b)

		case fPassEsc:
			if b == stFinal {
				dst = append(dst, escByte, stFinal)
				f.state = fNormal
				continue
			}
			dst = append(dst, escByte)
			if b == escByte {
				continue // stay in fPassEsc
			}
			dst = append(dst, b)
			f.state = fPass

		case fDrop:
			if b == belByte {
				f.state = fNormal
				continue
			}
			if b == escByte {
				f.state = fDropEsc
			}

		case fDropEsc:
			if b == stFinal {
				f.state = fNormal // ST ends the dropped sequence
				continue
			}
			if b != escByte {
				f.state = fDrop // the ESC was not part of ST
			}
		}
	}
	return dst
}

// passByte handles a byte while passing a non-52 OSC through to its terminator.
func (f *clipboardFilter) passByte(dst []byte, b byte) []byte {
	if b == belByte {
		dst = append(dst, b)
		f.state = fNormal
		return dst
	}
	if b == escByte {
		f.state = fPassEsc
		return dst
	}
	dst = append(dst, b)
	return dst
}
