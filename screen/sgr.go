package screen

import "strings"

// sgrFilter rewrites colon-separated color SGR sub-parameters (the ITU form,
// e.g. "38:2:R:G:B") into the legacy semicolon form ("38;2;R;G;B") that every
// xterm.js version renders correctly. Some programs (notably Neovim) emit the
// colon form, which older xterm.js mis-parses (rotated RGB channels).
//
// Only the color introducers 38/48/58 are touched; other colon sub-parameters
// such as "4:3" (curly underline) and all non-SGR sequences pass through
// unchanged. The filter is stateful to handle sequences split across writes.
type sgrFilter struct {
	state csiState
	csi   []byte // buffered CSI parameter bytes (between "ESC[" and the final byte)
}

type csiState int

const (
	csiNormal csiState = iota
	csiEsc             // saw ESC
	csiParams          // inside "ESC[", collecting parameter bytes
)

const csiMaxLen = 512 // safety cap for a single CSI parameter run

// filter appends the normalized form of p to dst and returns it.
func (f *sgrFilter) filter(dst, p []byte) []byte {
	for _, b := range p {
		switch f.state {
		case csiNormal:
			if b == escByte {
				f.state = csiEsc
				continue
			}
			dst = append(dst, b)

		case csiEsc:
			if b == '[' {
				f.state = csiParams
				f.csi = f.csi[:0]
				continue
			}
			dst = append(dst, escByte) // the held ESC was not a CSI introducer
			if b == escByte {
				continue // another ESC: stay in csiEsc
			}
			dst = append(dst, b)
			f.state = csiNormal

		case csiParams:
			if b == escByte {
				// a new escape interrupts the CSI: flush what we held, restart
				dst = f.flush(dst)
				f.state = csiEsc
				continue
			}
			if b >= 0x40 && b <= 0x7e { // final byte of the CSI
				dst = f.emit(dst, b)
				f.state = csiNormal
				continue
			}
			f.csi = append(f.csi, b)
			if len(f.csi) > csiMaxLen {
				dst = f.flush(dst) // malformed / too long
				f.state = csiNormal
			}
		}
	}
	return dst
}

// flush writes the raw, un-terminated CSI prefix collected so far.
func (f *sgrFilter) flush(dst []byte) []byte {
	dst = append(dst, escByte, '[')
	dst = append(dst, f.csi...)
	f.csi = f.csi[:0]
	return dst
}

// emit writes the completed CSI, normalizing it when it is an SGR sequence.
func (f *sgrFilter) emit(dst []byte, final byte) []byte {
	dst = append(dst, escByte, '[')
	if final == 'm' && isSGRParams(f.csi) {
		dst = append(dst, normalizeSGR(string(f.csi))...)
	} else {
		dst = append(dst, f.csi...)
	}
	f.csi = f.csi[:0]
	return append(dst, final)
}

// isSGRParams reports whether the bytes are plain SGR parameters (digits, ';'
// and ':'), i.e. no private markers or intermediates we should not rewrite.
func isSGRParams(b []byte) bool {
	for _, c := range b {
		if (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// normalizeSGR converts the colon color groups in an SGR parameter string to
// the semicolon form, leaving every other parameter untouched.
func normalizeSGR(params string) string {
	parts := strings.Split(params, ";")
	for i, part := range parts {
		parts[i] = normalizeColorParam(part)
	}
	return strings.Join(parts, ";")
}

func normalizeColorParam(part string) string {
	if !strings.HasPrefix(part, "38:") &&
		!strings.HasPrefix(part, "48:") &&
		!strings.HasPrefix(part, "58:") {
		return part
	}

	sub := strings.Split(part, ":")
	if len(sub) < 3 {
		return part
	}

	switch sub[1] {
	case "5": // indexed: code:5:N
		return sub[0] + ";5;" + sub[2]
	case "2": // truecolor: code:2:[colorspace:]R:G:B
		if len(sub) < 5 {
			return part
		}
		rgb := sub[len(sub)-3:]
		return sub[0] + ";2;" + rgb[0] + ";" + rgb[1] + ";" + rgb[2]
	default:
		return part
	}
}
