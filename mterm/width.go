package mterm

import "unicode"

// wideRunes holds the East Asian Wide and Fullwidth blocks plus the emoji
// presentation ranges — the practical wcwidth subset every terminal (tmux,
// xterm, iTerm2) renders as two columns. Cell arithmetic must agree with
// them: counting a wide rune as one column makes re-rendered lines overflow
// and scroll the viewer's screen (seen as a duplicated tmux status bar after
// reconnect, via CJK characters in the bar).
var wideRunes = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x1100, Hi: 0x115F, Stride: 1}, // Hangul Jamo
		{Lo: 0x2329, Hi: 0x232A, Stride: 1},
		{Lo: 0x2E80, Hi: 0x2FFB, Stride: 1}, // CJK Radicals .. Ideographic Description
		{Lo: 0x3000, Hi: 0x303E, Stride: 1}, // CJK Symbols and Punctuation
		{Lo: 0x3041, Hi: 0x33FF, Stride: 1}, // Hiragana .. CJK Compatibility
		{Lo: 0x3400, Hi: 0x4DBF, Stride: 1}, // CJK Extension A
		{Lo: 0x4E00, Hi: 0x9FFF, Stride: 1}, // CJK Unified Ideographs
		{Lo: 0xA000, Hi: 0xA4CF, Stride: 1}, // Yi
		{Lo: 0xA960, Hi: 0xA97C, Stride: 1}, // Hangul Jamo Extended-A
		{Lo: 0xAC00, Hi: 0xD7A3, Stride: 1}, // Hangul Syllables
		{Lo: 0xF900, Hi: 0xFAFF, Stride: 1}, // CJK Compatibility Ideographs
		{Lo: 0xFE10, Hi: 0xFE19, Stride: 1}, // Vertical Forms
		{Lo: 0xFE30, Hi: 0xFE6B, Stride: 1}, // CJK Compatibility Forms
		{Lo: 0xFF00, Hi: 0xFF60, Stride: 1}, // Fullwidth Forms
		{Lo: 0xFFE0, Hi: 0xFFE6, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x16FE0, Hi: 0x16FE4, Stride: 1},
		{Lo: 0x17000, Hi: 0x18CD5, Stride: 1}, // Tangut
		{Lo: 0x1B000, Hi: 0x1B2FB, Stride: 1}, // Kana Extended
		{Lo: 0x1F300, Hi: 0x1F64F, Stride: 1}, // emoji
		{Lo: 0x1F680, Hi: 0x1F6FF, Stride: 1}, // transport emoji
		{Lo: 0x1F900, Hi: 0x1F9FF, Stride: 1}, // supplemental emoji
		{Lo: 0x1FA70, Hi: 0x1FAFF, Stride: 1}, // extended emoji
		{Lo: 0x20000, Hi: 0x2FFFD, Stride: 1}, // CJK Extensions B..F
		{Lo: 0x30000, Hi: 0x3FFFD, Stride: 1}, // CJK Extension G
	},
}

// runeWidth returns the number of terminal columns a rune occupies: 0 for
// combining and other zero-width marks, 2 for wide/fullwidth, 1 otherwise.
func runeWidth(r rune) int {
	switch {
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case unicode.In(r, wideRunes):
		return 2
	}
	return 1
}
