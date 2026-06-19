package screen

import (
	"sync"
	"testing"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/stream"
)

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
