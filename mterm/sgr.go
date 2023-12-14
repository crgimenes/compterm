package mterm

import "fmt"

const (
	Color16  = 1
	Color256 = 2
	Color16M = 3
)

const (
	FlagBold uint8 = 1 << iota
	FlagDim
	FlagItalic
	FlagUnderline
	FlagBlink
	FlagInverse
	FlagInvisible
	FlagStrike
)

type Color [3]byte

type sgrState struct {
	FG        Color
	BG        Color
	UL        Color // underline color
	ColorType uint8 // 0b00uubbff (u underline, b BG, f FG color types)
	Flags     uint8
}

// set the set based on CSI parameters
func (s *sgrState) set(p ...int) error {
	if len(p) == 0 {
		*s = sgrState{}
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		sub := p[i:]
		switch {
		case c == 0:
			*s = sgrState{}
		case c == 1:
			s.Flags |= FlagBold
		case c == 21: // double underline?!
		//	s.Flags &= ^FlagBold
		case c == 2:
			s.Flags |= FlagDim
		case c == 22:
			s.Flags &= ^(FlagDim | FlagBold)
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
		case c == 28:
			s.Flags &= ^FlagInvisible
		case c == 9:
			s.Flags |= FlagStrike
		case c == 29:
			s.Flags &= ^FlagStrike
		case c >= 90 && c <= 97: // FG bright (not bold)
			s.ColorType = s.ColorType&0b11111100 | Color16
			s.FG[0] = byte(c)
		case c >= 100 && c <= 107: // BG bright (not bold)
			s.ColorType = s.ColorType&0b11110011 | (Color16 << 2)
			s.BG[0] = byte(c)
		case c >= 30 && c <= 37: // FG
			s.ColorType = s.ColorType&0b11111100 | Color16
			s.FG[0] = byte(c)
		case c == 39: // FG default foreground
			s.ColorType &= 0b11111100
		case c >= 40 && c <= 47: // BG 16 colors
			s.ColorType = s.ColorType&0b11110011 | (Color16 << 2)
			s.BG[0] = byte(c)
		case c == 49: // BG default background
			s.ColorType &= 0b11110011
		// FG 256 Colors
		case c == 38 && len(sub) >= 3 && sub[1] == 5:
			s.ColorType = s.ColorType&0b11111100 | Color256
			s.FG[0] = byte(sub[2])
			i += 2
		// BG 256 Colors
		case c == 48 && len(sub) >= 3 && sub[1] == 5:
			s.ColorType = s.ColorType&0b11110011 | (Color256 << 2)
			s.BG[0] = byte(sub[2])
			i += 2
		// FG 16M Colors
		case c == 38 && len(sub) >= 5 && sub[1] == 2:
			s.ColorType = s.ColorType&0b11111100 | Color16M
			s.FG = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4
		// BG 16M Colors
		case c == 48 && len(sub) >= 5 && sub[1] == 2:
			s.ColorType = s.ColorType&0b11110011 | (Color16M << 2)
			s.BG = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4
		// XXX: Experimental
		// underline sample
		// \x1b[58:2::173:216:230m
		case c == 58 && len(sub) >= 3 && sub[1] == 5:
			s.ColorType = s.ColorType&0b11001111 | (Color256 << 4)
			s.UL[0] = byte(sub[2])
			i += 2
		case c == 58 && len(sub) >= 5 && sub[1] == 2:
			s.ColorType = s.ColorType&0b11001111 | (Color16M << 4)
			s.UL = Color{byte(sub[2]), byte(sub[3]), byte(sub[4])}
			i += 4
		case c == 59: // Default underline color
			s.ColorType &= 0b11001111
		case c == 53:
			// TODO overline
		case c == 55:
			// TODO turn off overline
		default:
			return fmt.Errorf("unknown SGR: %v", c)
		}
	}
	return nil
}
