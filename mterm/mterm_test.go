package mterm

import (
	"fmt"
	"strings"
	"testing"
)

// writeLines writes n numbered lines ("line 0001" ...) to the terminal.
func writeLines(t *testing.T, tr *Terminal, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		_, err := fmt.Fprintf(tr, "line %04d\r\n", i)
		if err != nil {
			t.Fatalf("writing line %d: %v", i, err)
		}
	}
}

func TestHistory(t *testing.T) {
	tr := New(10, 20)

	if h := tr.GetHistoryAsAnsi(); len(h) != 0 {
		t.Fatalf("fresh terminal has history: %q", h)
	}

	writeLines(t, tr, 30)

	hist := string(tr.GetHistoryAsAnsi())
	screen := string(tr.GetScreenAsAnsi())

	if !strings.Contains(hist, "line 0001") {
		t.Errorf("history is missing the first scrolled-off line")
	}
	if strings.Contains(hist, "line 0030") {
		t.Errorf("history contains a line still on screen")
	}
	if !strings.Contains(screen, "line 0030") {
		t.Errorf("screen is missing the last line")
	}
	if !strings.HasSuffix(hist, "\033[0m\r\n") {
		t.Errorf("history does not end with an SGR reset and newline")
	}
}

// TestHistoryCap pins the eviction order at the backlog cap: the OLDEST line
// is dropped, so history stays the most recent scrolled-off lines.
func TestHistoryCap(t *testing.T) {
	tr := New(10, 20)

	writeLines(t, tr, 1200)

	hist := string(tr.GetHistoryAsAnsi())

	if strings.Contains(hist, "line 0001") {
		t.Errorf("history kept the oldest line past the cap")
	}
	if !strings.Contains(hist, "line 1100") {
		t.Errorf("history is missing a recent scrolled-off line")
	}
	if lines := strings.Count(hist, "\r\n"); lines > 1000 {
		t.Errorf("history has %d lines, beyond the backlog cap", lines)
	}
}

// TestHistoryAltScreen: with the alt screen active, the whole primary screen
// is history — it is what was on display before the alt-screen app started.
func TestHistoryAltScreen(t *testing.T) {
	tr := New(10, 20)

	writeLines(t, tr, 5)
	_, err := tr.Write([]byte("\033[?1049h"))
	if err != nil {
		t.Fatalf("entering alt screen: %v", err)
	}
	_, err = tr.Write([]byte("alt content"))
	if err != nil {
		t.Fatalf("writing to alt screen: %v", err)
	}

	hist := string(tr.GetHistoryAsAnsi())
	if !strings.Contains(hist, "line 0005") {
		t.Errorf("alt-screen history is missing the primary screen content")
	}
	if strings.Contains(hist, "alt content") {
		t.Errorf("alt-screen history contains the alt screen itself")
	}
}

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
