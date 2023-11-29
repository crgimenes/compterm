package ansiParser

import (
	"strings"
	"testing"
)

func TestANSIParser_Write(t *testing.T) {
	ap := New(24, 80)

	p := []byte("test simple string")

	got, err := ap.Write(p)
	if err != nil {
		t.Errorf("ANSIParser.Write() error = %v", err)
		return
	}

	if got != len(p) {
		t.Errorf("ANSIParser.Write() = %v, want %v", got, len(p))
	}

	s := ap.GetScreenAsAnsi()
	//t.Logf("screen: %q", s)
	//t.Logf("cursorLine: %d", ap.cursorLine)
	//t.Logf("cursorCol: %d", ap.cursorCol)

	if ap.cursorLine != 0 {
		t.Errorf("ANSIParser.cursorLine = %v, want %v", ap.cursorLine, 0)
	}

	if ap.cursorCol != len(p) {
		t.Errorf("ANSIParser.cursorCol = %v, want %v", ap.cursorCol, len(p))
	}

	if strings.TrimSpace(string(s)) != string(p) {
		t.Errorf("ANSIParser.GetScreenAsAnsi() = %q, want %q", s, p)
	}
}
