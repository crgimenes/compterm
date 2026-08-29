package screen

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/crgimenes/compterm/protocol"
	"github.com/crgimenes/compterm/stream"
)

// drainPayloads reads frames from the client stream until want payload bytes
// of MSG data have been reassembled, returning them.
func drainPayloads(t *testing.T, c *Client, want int) []byte {
	t.Helper()

	var acc, out []byte
	buf := make([]byte, protocol.MaxPackageSize)
	payload := make([]byte, protocol.BufferSize)
	for len(out) < want {
		n, err := c.bs.Read(buf)
		if err != nil {
			t.Fatalf("reading client stream: %v", err)
		}
		acc = append(acc, buf[:n]...)

		for len(acc) >= protocol.Overhead {
			cmd, ln, err := protocol.Decode(payload, acc)
			if err != nil {
				// an incomplete trailing frame: read more
				break
			}
			if cmd == protocol.MSG {
				if !utf8.Valid(payload[:ln]) {
					t.Errorf("frame payload is not valid UTF-8: a rune was split across frames")
				}
				out = append(out, payload[:ln]...)
			}
			acc = acc[ln+protocol.Overhead:]
		}
	}
	return out
}

func TestCompleteRunePrefix(t *testing.T) {
	block := "▀" // ▀ = E2 96 80

	tests := []struct {
		name string
		in   []byte
		want int
	}{
		{"empty", []byte{}, 0},
		{"ascii", []byte("abc"), 3},
		{"complete block", []byte(block), 3},
		{"ascii then complete block", []byte("a" + block), 4},
		{"trailing two of three bytes held", []byte("a\xe2\x96"), 1},
		{"trailing lead byte held", []byte("a\xe2"), 1},
		{"only incomplete rune", []byte("\xe2\x96"), 0},
		{"stray continuation passes through", []byte{0x96, 0x96}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := completeRunePrefix(tt.in); got != tt.want {
				t.Errorf("completeRunePrefix(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestSnapshotIncludesHistory verifies that a newly attached client receives
// the scrolled-off lines ahead of the visible screen, as one continuous
// stream, ending with the cursor position.
func TestSnapshotIncludesHistory(t *testing.T) {
	s := New(5, 20)
	for i := 1; i <= 20; i++ {
		_, err := fmt.Fprintf(s, "line %02d\r\n", i)
		if err != nil {
			t.Fatalf("writing line %d: %v", i, err)
		}
	}

	rows, columns := s.size()
	crows, ccolumns := s.CursorPos()
	want := fmt.Appendf(nil, "\033[8;%d;%dt\033[0;0H", rows, columns)
	want = append(want, s.GetHistoryAsANSI()...)
	want = append(want, s.GetScreenAsANSI()...)
	want = fmt.Appendf(want, "\033[%d;%dH", crows+1, ccolumns+1)

	c := bareClient()
	s.updateToCurrentState(c)

	got := drainPayloads(t, c, len(want))
	if !bytes.Equal(got, want) {
		t.Fatalf("snapshot differs from history+screen composition:\ngot  %q\nwant %q", got, want)
	}
	if !strings.Contains(string(got), "line 01") {
		t.Errorf("snapshot is missing scrolled-off line 01")
	}
	if !strings.Contains(string(got), "line 20") {
		t.Errorf("snapshot is missing on-screen line 20")
	}
}

// TestSnapshotRestoresTmuxModes pins the reconnect contract for a tmux-style
// host: the snapshot must re-enter the alt screen and re-establish the scroll
// region (after the content, before the cursor position) — without them the
// viewer's next region scroll moves the whole screen and duplicates the
// protected status line.
func TestSnapshotRestoresTmuxModes(t *testing.T) {
	s := New(10, 20)
	_, err := s.Write([]byte("before\r\n\033[?1049h\033[10;1HSTATUS\033[1;9r\033[9;1Hcontent"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	c := bareClient()
	s.updateToCurrentState(c)

	got := string(drainPayloads(t, c, 1))
	alt := strings.Index(got, "\033[?1049h")
	status := strings.Index(got, "STATUS")
	region := strings.Index(got, "\033[1;9r")
	if alt < 0 || status < 0 || region < 0 {
		t.Fatalf("snapshot is missing alt screen, content, or region: %q", got)
	}
	if alt >= status || status >= region {
		t.Errorf("wrong order: alt=%d content=%d region=%d in %q", alt, status, region, got)
	}
	if !strings.Contains(got[region:], "H") {
		t.Errorf("no cursor position after the region: %q", got[region:])
	}
}

// TestLeavingAltScreenRepaintsPrimary verifies that when the host leaves the
// alternate screen the broadcast stream carries a repaint of the primary
// screen. The host terminal restores that content from local memory, so
// without the repaint a viewer that cannot nest the buffer switch is left
// with a blank screen.
func TestLeavingAltScreenRepaintsPrimary(t *testing.T) {
	s := New(5, 20)
	c := bareClient()
	s.AttachClient(c)

	// discard the attach snapshot (same composition updateToCurrentState uses)
	rows, columns := s.size()
	crows, ccolumns := s.CursorPos()
	pre := fmt.Appendf(nil, "\033[8;%d;%dt\033[0;0H", rows, columns)
	pre = append(pre, s.GetHistoryAsANSI()...)
	pre = append(pre, s.GetScreenAsANSI()...)
	pre = append(pre, s.mt.GetModesAsAnsi()...)
	pre = fmt.Appendf(pre, "\033[%d;%dH", crows+1, ccolumns+1)
	_ = drainPayloads(t, c, len(pre))

	writes := []string{
		"hello-main\r\n",
		"\033[?1049hvi-screen",
		"\033[?1049l",
	}
	var raw []byte
	for _, w := range writes {
		if _, err := s.Write([]byte(w)); err != nil {
			t.Fatalf("write: %v", err)
		}
		raw = append(raw, w...)
	}

	crows, ccolumns = s.CursorPos()
	want := raw
	want = append(want, "\033[0;0H"...)
	want = append(want, s.GetScreenAsANSI()...)
	want = append(want, s.mt.GetModesAsAnsi()...)
	want = fmt.Appendf(want, "\033[%d;%dH", crows+1, ccolumns+1)

	got := drainPayloads(t, c, len(want))
	if !bytes.Equal(got, want) {
		t.Fatalf("stream after leaving alt screen:\ngot  %q\nwant %q", got, want)
	}
	leave := bytes.Index(got, []byte("\033[?1049l"))
	if !bytes.Contains(got[leave:], []byte("hello-main")) {
		t.Errorf("no primary-screen repaint after the alt screen exit: %q", got[leave:])
	}
}

// TestSendSnapshotChunks verifies that a snapshot larger than one frame is
// split across frames without ever cutting a multibyte rune.
func TestSendSnapshotChunks(t *testing.T) {
	s := New(5, 20)
	c := bareClient()

	payload := bytes.Repeat([]byte("▀"), 200_000) // 600KB, 3-byte runes
	s.sendSnapshot(c, payload)

	got := drainPayloads(t, c, len(payload))
	if !bytes.Equal(got, payload) {
		t.Fatalf("reassembled snapshot differs from original (%d vs %d bytes)", len(got), len(payload))
	}
}

// bareClient builds a client without a websocket connection or background
// goroutines, exercising only the screen-level bookkeeping that the lock
// protects. It never triggers any network call.
func bareClient() *Client {
	return &Client{
		bs:      stream.New(),
		outbuff: make([]byte, protocol.BufferSize),
		done:    make(chan struct{}),
	}
}

// TestScreenConcurrency drives attach, broadcast (via the stream pump), resize,
// and closed-client eviction concurrently. Run with -race to verify the lock
// discipline.
func TestScreenConcurrency(t *testing.T) {
	s := New(25, 80)

	var wg sync.WaitGroup
	for i := range 60 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			c := bareClient()
			s.AttachClient(c)

			// some clients "disconnect"; broadcast must evict them safely
			if i%3 == 0 {
				close(c.done)
			}

			_, _ = s.Write([]byte("data\r\n"))

			if i%5 == 0 {
				s.Resize(20+(i%4), 80)
			}
		}(i)
	}

	wg.Wait()
}
