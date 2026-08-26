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

// TestRenderColorReset pins the SGR contract of snapshot rendering: a colored
// line followed by a default-state line must emit a reset, or the color leaks
// into the next line when the snapshot is replayed (seen as wrong colors on
// browser reconnect after trailing-blank trimming removed the accidental
// reset that padding cells used to provide).
func TestRenderColorReset(t *testing.T) {
	tr := New(10, 20)

	_, err := tr.Write([]byte("\033[31mred\033[0m\r\nplain"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	s := string(tr.GetScreenAsAnsi())
	i := strings.Index(s, "red")
	j := strings.Index(s, "plain")
	if i < 0 || j < 0 {
		t.Fatalf("render is missing content: %q", s)
	}
	if !strings.Contains(s[i:j], "\033[0m") {
		t.Errorf("no SGR reset between a colored line and a default one: %q", s)
	}
}

// TestRenderFinalStateSync: the snapshot must leave the terminal in the
// host's CURRENT SGR state, not in the last rendered cell's state, so live
// output appended right after the snapshot keeps its colors.
func TestRenderFinalStateSync(t *testing.T) {
	tr := New(10, 20)

	// last cell is red, but the live state was reset after it
	_, err := tr.Write([]byte("\033[31mred\033[0m"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	s := string(tr.GetScreenAsAnsi())
	tail := s[strings.Index(s, "red")+3:]
	if !strings.Contains(tail, "\033[0m") {
		t.Errorf("snapshot leaves the terminal colored while the host state is reset: %q", s)
	}
}

// TestScrollRegionKeepsStatusLine emulates the tmux pattern: a scroll region
// protecting a status line at the bottom. Scrolling inside the region must
// not duplicate the protected line and must not push region lines into the
// backlog — a real terminal discards them (seen as a duplicated tmux bar on
// browser reconnect).
func TestScrollRegionKeepsStatusLine(t *testing.T) {
	tr := New(10, 20)

	_, err := tr.Write([]byte("\033[10;1HSTATUS\033[1;9r\033[9;1Hone\r\ntwo\r\nthree"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	screen := string(tr.GetScreenAsAnsi())
	if got := strings.Count(screen, "STATUS"); got != 1 {
		t.Errorf("STATUS appears %d times on screen, want 1:\n%q", got, screen)
	}
	if !strings.Contains(screen, "three") {
		t.Errorf("screen is missing the last region line: %q", screen)
	}

	hist := string(tr.GetHistoryAsAnsi())
	if hist != "" {
		t.Errorf("region scroll leaked lines into the backlog: %q", hist)
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
