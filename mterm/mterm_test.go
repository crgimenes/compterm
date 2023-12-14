package mterm

import (
	"strings"
	"testing"
)

func TestTerminal_Write(t *testing.T) {
	tr := New(24, 80)

	p := []byte("test simple string")

	got, err := tr.Write(p)
	if err != nil {
		t.Errorf("Terminal.Write() error = %v", err)
		return
	}

	if got != len(p) {
		t.Errorf("Terminal.Write() = %v, want %v", got, len(p))
	}

	s := tr.GetScreenAsAnsi()
	// t.Logf("screen:\n%q", s)
	// t.Logf("cursorLine: %d", tr.cursorLine)
	// t.Logf("cursorCol: %d", tr.cursorCol)

	l, c := tr.CursorPos()
	if l != 0 {
		t.Errorf("Terminal.cursorLine = %v, want %v", l, 0)
	}

	if c != len(p) {
		t.Errorf("Terminal.cursorCol = %v, want %v", c, len(p))
	}

	if strings.TrimSpace(string(s)) != string(p) {
		t.Errorf("Terminal.GetScreenAsAnsi() = %q, want %q", s, p)
	}
}
