package mterm

import "unicode"

// runeWidth returns the number of terminal columns a rune occupies: 0 for
// combining and other zero-width marks, 2 for wideRunes (width_table.go: the
// East Asian Wide/Fullwidth blocks plus every Emoji_Presentation rune, the
// same table as filo-term's utf8.c), 1 otherwise. Cell arithmetic must agree
// with the terminal: counting a wide rune as one column makes re-rendered
// lines overflow and scroll the viewer's screen (seen as a duplicated tmux
// status bar after reconnect, via CJK characters in the bar).
func runeWidth(r rune) int {
	switch {
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case unicode.In(r, wideRunes):
		return 2
	}
	return 1
}
