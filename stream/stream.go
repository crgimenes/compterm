package stream

import (
	"bytes"
	"io"
	"sync"
)

// Stream is a structure that implements a byte stream.
type Stream struct {
	buffer bytes.Buffer
	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
}

// New criate a new instance of Stream.
func New() *Stream {
	s := &Stream{}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Write implements the io.Writer interface.
func (s *Stream) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, io.EOF
	}

	n, err = s.buffer.Write(p)
	s.cond.Broadcast() // Notify all readers.
	return n, err
}

// Read implements the io.Reader interface.
// Blocks until data is available or the stream is closed.
func (s *Stream) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for s.buffer.Len() == 0 {
		if s.closed {
			return 0, io.EOF
		}
		if s.cond == nil {
			return 0, io.ErrClosedPipe
		}
		s.cond.Wait() // Wait for data to be available.
	}

	return s.buffer.Read(p)
}

// Close marck the stream as closed.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.cond.Broadcast() // Notify all readers.
	return nil
}
