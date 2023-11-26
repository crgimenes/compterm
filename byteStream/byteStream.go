package byteStream

import (
	"bytes"
	"io"
	"sync"
)

// ByteStream é uma struct que implementa a interface io.Reader.
type ByteStream struct {
	buffer bytes.Buffer
	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
}

// NewByteStream cria uma nova instância de ByteStream.
func NewByteStream() *ByteStream {
	bs := &ByteStream{}
	bs.cond = sync.NewCond(&bs.mu)
	return bs
}

// Write adiciona dados ao buffer do ByteStream e notifica os leitores.
func (bs *ByteStream) Write(p []byte) (n int, err error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.closed {
		return 0, io.EOF
	}

	n, err = bs.buffer.Write(p)
	bs.cond.Broadcast() // Notifica que novos dados estão disponíveis.
	return n, err
}

// Read implementa a interface io.Reader.
// Bloqueia até que dados estejam disponíveis para leitura.
func (bs *ByteStream) Read(p []byte) (n int, err error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	for bs.buffer.Len() == 0 {
		if bs.closed {
			return 0, io.EOF
		}
		bs.cond.Wait() // Espera por novos dados.
	}

	return bs.buffer.Read(p)
}

// Close marca o ByteStream como fechado.
func (bs *ByteStream) Close() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.closed = true
	bs.cond.Broadcast() // Notifica os leitores para que possam concluir.
	return nil
}
