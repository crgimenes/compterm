package screen

import (
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/protocol"
	"github.com/crgimenes/compterm/stream"
)

// Screen broadcasts a single terminal to every attached client. It keeps an
// authoritative in-memory terminal emulator so new clients can be brought to
// the current state.
//
// mx guards Clients, Rows, and Columns. The emulator (mt) and the broadcast
// Stream are internally synchronized, so they are used without holding mx. The
// lock is never held while sending to a client.
type Screen struct {
	Columns int             `json:"columns"`
	Rows    int             `json:"rows"`
	Clients []*Client       `json:"-"`
	Stream  *stream.Stream  `json:"-"`
	mt      *mterm.Terminal `json:"-"`
	mx      sync.Mutex      `json:"-"`

	// writeMu serializes Write so the stateful clipboard filter is safe.
	writeMu sync.Mutex      `json:"-"`
	clip    clipboardFilter `json:"-"`
	clipBuf []byte          `json:"-"`
}

type Client struct {
	bs        *stream.Stream
	conn      *websocket.Conn
	SessionID string `json:"session_id"`
	outbuff   []byte
	mx        sync.Mutex
	done      chan struct{}
}

func New(rows, columns int) *Screen {
	s := &Screen{
		Columns: columns,
		Rows:    rows,
		Stream:  stream.New(),
		mt:      mterm.New(rows, columns),
	}

	go s.writeToAttachedClients()

	return s
}

// AttachClient attaches a client and brings it to the current screen state.
func (s *Screen) AttachClient(c *Client) {
	s.mx.Lock()
	if !slices.Contains(s.Clients, c) {
		s.Clients = append(s.Clients, c)
	}
	s.mx.Unlock()

	s.updateToCurrentState(c)
}

// size returns the current dimensions under the lock.
func (s *Screen) size() (rows, columns int) {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.Rows, s.Columns
}

// snapshotClients returns a copy of the attached clients so the lock is not
// held while sending.
func (s *Screen) snapshotClients() []*Client {
	s.mx.Lock()
	defer s.mx.Unlock()
	return slices.Clone(s.Clients)
}

// removeClients detaches the given clients in a single locked pass.
func (s *Screen) removeClients(dead []*Client) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.Clients = slices.DeleteFunc(s.Clients, func(c *Client) bool {
		return slices.Contains(dead, c)
	})
}

// broadcast sends a framed message to every attached client and detaches the
// ones that are closed or fail to receive it.
func (s *Screen) broadcast(prefix byte, p []byte) {
	var dead []*Client

	for _, c := range s.snapshotClients() {
		if c.IsClosed() {
			dead = append(dead, c)
			continue
		}
		if err := c.Send(prefix, p); err != nil {
			log.Printf("error writing to websocket: %s\r\n", err)
			c.Close()
			dead = append(dead, c)
		}
	}

	if len(dead) > 0 {
		s.removeClients(dead)
	}
}

func (s *Screen) writeToAttachedClients() {
	msg := make([]byte, constants.BufferSize)
	for {
		n, err := s.Read(msg)
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Printf("error reading from byte stream: %s\r\n", err)
		}

		s.broadcast(constants.MSG, msg[:n])
	}
}

// Write implements io.Writer: it strips the host's clipboard sequences (OSC 52)
// and feeds the cleaned bytes to both the emulator and the broadcast stream, so
// neither the live deltas nor the snapshot can carry the host's clipboard.
func (s *Screen) Write(p []byte) (n int, err error) {
	s.writeMu.Lock()
	s.clipBuf = s.clip.filter(s.clipBuf[:0], p)
	_, _ = s.mt.Write(s.clipBuf)
	_, _ = s.Stream.Write(s.clipBuf)
	s.writeMu.Unlock()
	return len(p), nil
}

func (s *Screen) updateToCurrentState(c *Client) {
	rows, columns := s.size()
	crows, ccolumns := s.CursorPos()
	msg := s.GetScreenAsANSI()

	_ = c.Send(constants.RESIZE,
		fmt.Appendf(nil, "%d:%d", rows, columns))

	m := fmt.Sprintf("\033[8;%d;%dt\033[0;0H%s\033[%d;%dH",
		rows, columns, msg, crows+1, ccolumns+1)

	_ = c.Send(constants.MSG, []byte(m))
}

func (s *Screen) Read(p []byte) (n int, err error) {
	return s.Stream.Read(p)
}

// Resize resizes the screen and notifies attached clients.
func (s *Screen) Resize(rows, columns int) {
	s.mx.Lock()
	if rows == s.Rows && columns == s.Columns {
		s.mx.Unlock()
		return
	}
	s.Rows = rows
	s.Columns = columns
	s.mt.Resize(rows, columns)
	s.mx.Unlock()

	_, _ = s.Write(fmt.Appendf(nil, "\033[8;%d;%dt", rows, columns))
	s.broadcast(constants.RESIZE, fmt.Appendf(nil, "%d:%d", rows, columns))
}

// GetScreenAsANSI returns the current screen content as ANSI.
func (s *Screen) GetScreenAsANSI() []byte {
	return s.mt.GetScreenAsAnsi()
}

// CursorPos returns the cursor position.
func (s *Screen) CursorPos() (rows, columns int) {
	return s.mt.CursorPos()
}

func NewClient(conn *websocket.Conn) *Client {
	c := &Client{
		bs:      stream.New(),
		conn:    conn,
		outbuff: make([]byte, constants.BufferSize),
		done:    make(chan struct{}),
	}

	go c.writeLoop()
	go c.rejectInput()

	return c
}

func (c *Client) Close() {
	select {
	case <-c.done:
		return
	default:
		close(c.done)
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
	}
}

// IsClosed reports whether the client is closed.
func (c *Client) IsClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Send frames a message and queues it to the client's stream.
func (c *Client) Send(prefix byte, p []byte) (err error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	ln, err := protocol.Encode(c.outbuff, p, prefix, 0)
	if err != nil {
		return err
	}

	n := 0
	for n < ln {
		n, err = c.bs.Write(c.outbuff[n:ln])
		if err != nil {
			return err
		}
	}

	return nil
}

// rejectInput enforces compterm's one-way contract. A viewer must never send
// anything to the host, so the connection is read only to detect disconnects
// and to drop any client that tries to send data.
func (c *Client) rejectInput() {
	for {
		select {
		case <-c.done:
			return
		default:
			_, _, err := c.conn.Read(context.Background())
			if err != nil {
				cs := websocket.CloseStatus(err)
				if cs != websocket.StatusNormalClosure &&
					cs != websocket.StatusGoingAway &&
					cs != -1 {
					log.Printf("error reading from websocket: %s\r\n", err)
				}
				c.Close()
				return
			}

			// The stream is one-way; a client that sends data is dropped.
			log.Printf("client %q sent data on a read-only connection; closing\r\n", c.SessionID)
			c.Close()
			return
		}
	}
}

// writeLoop drains the client stream to the websocket.
func (c *Client) writeLoop() {
	buff := make([]byte, constants.BufferSize)
	for {
		select {
		case <-c.done:
			return
		default:
			n, err := c.bs.Read(buff)
			if err != nil {
				log.Printf("error reading from byte stream: %s\r\n", err)
				return
			}

			err = c.conn.Write(context.Background(), websocket.MessageBinary, buff[:n])
			if err != nil {
				cs := websocket.CloseStatus(err)
				if cs != websocket.StatusNormalClosure &&
					cs != websocket.StatusGoingAway &&
					cs != -1 {
					log.Printf("error writing to websocket: %s, %v\r\n", err, cs)
				}
				c.Close()
				return
			}
		}
	}
}
