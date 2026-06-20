package screen

import (
	"sync"
	"testing"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/stream"
)

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

// bareClient builds a client without a websocket connection or background
// goroutines, exercising only the screen-level bookkeeping that the lock
// protects. It never triggers any network call.
func bareClient() *Client {
	return &Client{
		bs:      stream.New(),
		outbuff: make([]byte, constants.BufferSize),
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
