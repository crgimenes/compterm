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
type Screen struct {
	Columns int             `json:"columns"`
	Rows    int             `json:"rows"`
	Clients []*Client       `json:"-"`
	Stream  *stream.Stream  `json:"-"`
	mt      *mterm.Terminal `json:"-"`
	mx      sync.Mutex      `json:"-"`
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
	defer s.mx.Unlock()

	if slices.Contains(s.Clients, c) {
		s.updateToCurrentState(c)
		return
	}

	s.Clients = append(s.Clients, c)
	s.updateToCurrentState(c)
}

func (s *Screen) removeAttachedClient(c *Client) {
	for i, ac := range s.Clients {
		if ac == c {
			s.Clients = append(s.Clients[:i], s.Clients[i+1:]...)
			return
		}
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

		func() {
			s.mx.Lock()
			defer s.mx.Unlock()
			for _, c := range s.Clients {
				select {
				case <-c.done:
					s.removeAttachedClient(c)
					continue
				default:
					err = c.Send(constants.MSG, msg[:n])
					if err != nil {
						log.Printf("error writing to websocket: %s\r\n", err)
						c.Close()
						s.removeAttachedClient(c)
						continue
					}
				}
			}
		}()
	}
}

// Write implements io.Writer: it feeds the emulator and the broadcast stream.
func (s *Screen) Write(p []byte) (n int, err error) {
	_, _ = s.mt.Write(p)
	_, _ = s.Stream.Write(p)
	return len(p), nil
}

func (s *Screen) updateToCurrentState(c *Client) {
	crows, ccolumns := s.CursorPos()
	msg := s.GetScreenAsANSI()

	_ = c.Send(constants.RESIZE,
		fmt.Appendf(nil, "%d:%d", s.Rows, s.Columns))

	m := fmt.Sprintf("\033[8;%d;%dt\033[0;0H%s\033[%d;%dH",
		s.Rows, s.Columns, msg, crows+1, ccolumns+1)

	_ = c.Send(constants.MSG, []byte(m))
}

func (s *Screen) Read(p []byte) (n int, err error) {
	return s.Stream.Read(p)
}

func (s *Screen) Send(prefix byte, p []byte) (err error) {
	for _, c := range s.Clients {
		err = c.Send(prefix, p)
		if err != nil {
			log.Printf("error writing to websocket: %s\r\n", err)
			c.Close()
			s.removeAttachedClient(c)
			continue
		}
	}

	return nil
}

// Resize resizes the screen and notifies attached clients.
func (s *Screen) Resize(rows, columns int) {
	s.mx.Lock()
	defer s.mx.Unlock()
	if rows == s.Rows && columns == s.Columns {
		return
	}

	s.Rows = rows
	s.Columns = columns
	s.mt.Resize(rows, columns)

	_, _ = s.Write(fmt.Appendf(nil, "\033[8;%d;%dt", s.Rows, s.Columns))
	_ = s.Send(constants.RESIZE, fmt.Appendf(nil, "%d:%d", rows, columns))
}

func (s *Screen) Size() (rows, columns int) {
	return s.Rows, s.Columns
}

// GetScreenAsANSI returns the current screen content as ANSI.
func (s *Screen) GetScreenAsANSI() []byte {
	return s.mt.GetScreenAsAnsi()
}

// CursorPos returns the cursor position.
func (s *Screen) CursorPos() (rows, columns int) {
	return s.mt.CursorPos()
}

// ListConnectedClients returns the attached clients.
func (s *Screen) ListConnectedClients() []*Client {
	s.mx.Lock()
	defer s.mx.Unlock()

	return s.Clients
}

func NewClient(conn *websocket.Conn) *Client {
	c := &Client{
		bs:      stream.New(),
		conn:    conn,
		outbuff: make([]byte, constants.BufferSize),
		done:    make(chan struct{}),
	}

	go c.writeLoop()
	go c.drainInput()

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

// drainInput reads and discards client messages; compterm is read-only, so the
// only purpose is to detect disconnects.
func (c *Client) drainInput() {
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
