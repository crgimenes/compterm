package screen

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/protocol"
	"github.com/crgimenes/compterm/stream"
	"nhooyr.io/websocket"
)

type ScreenClientProperties struct {
	WritePermission bool
}

type Screen struct {
	Title             string
	columns           int
	rows              int
	Clients           []*Client // Attached clients
	ClientsProperties map[*Client]*ScreenClientProperties
	Stream            *stream.Stream
	mt                *mterm.Terminal // terminal emulator
	Input             io.Writer       // receive input (stdin) from attached clients and other sources
	mx                sync.Mutex
}

type Manager struct {
	Screens []*Screen
}

type Client struct {
	bs            *stream.Stream
	conn          *websocket.Conn
	IP            string
	Nick          string
	SessionID     string
	outbuff       []byte // used to avoid memory allocation on each write
	mx            sync.Mutex
	CurrentScreen *Screen
	done          chan struct{} // used to close the writeLoop goroutine
}

func NewManager() *Manager {
	s := New(80, 25)
	s.Title = "default"

	return &Manager{
		Screens: []*Screen{s},
	}
}

// attach a client to a screen
func (s *Screen) AttachClient(c *Client, writePermission bool) error {
	s.mx.Lock()
	defer s.mx.Unlock()
	// check if client is already attached
	for _, ac := range s.Clients {
		if ac == c {
			ac.CurrentScreen = s
			s.ClientsProperties[c].WritePermission = writePermission
			s.updateToCurrentState(c)
			return nil
		}
	}

	// attach client to screen
	s.Clients = append(s.Clients, c)
	s.ClientsProperties[c] = &ScreenClientProperties{
		WritePermission: writePermission,
	}
	c.CurrentScreen = s
	s.updateToCurrentState(c)

	return nil
}

// detach a client from a screen
func (s *Screen) DetachClient(c *Client) error {
	// check if client is attached
	for i, ac := range s.Clients {
		if ac == c {
			s.Clients = append(
				s.Clients[:i],
				s.Clients[i+1:]...)
			s.ClientsProperties[c] = nil
			return nil
		}
	}

	return fmt.Errorf("client not attached")
}

// detach a client from all screens
func (m *Manager) DetachClientFromAllScreens(c *Client) {
	for _, s := range m.Screens {
		s.DetachClient(c)
	}
}

// change current screen
func (m *Manager) ChangeScreen(c *Client, s *Screen) error {
	// check if client is attached to screen return error if not
	for _, ac := range s.Clients {
		if ac == c {
			ac.CurrentScreen = s
			s.updateToCurrentState(c)
			return nil
		}
	}

	return fmt.Errorf("client not attached to screen")
}

// handle client input
func (c *Client) HandleInput() {
	buff := make([]byte, constants.BufferSize)
	for {
		select {
		case <-c.done:
			return
		default:
			n, err := c.ReadFromWS(buff) // Read from websocket
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

			s := string(buff[:n])
			// prevent \x1b[>0;276;0c
			if s == "\x1b[>0;276;0c" {
				continue
			}

			// TODO: parse input and send to lua

			ac := c.CurrentScreen.ClientsProperties[c]

			if ac.WritePermission {
				_, err = c.CurrentScreen.Input.Write(buff[:n]) // Write to pty
				if err != nil {
					log.Printf("error writing to pty: %s\r\n", err)
					c.Close()
					return
				}
			}
		}
	}
}

// remove a screen
func (m *Manager) RemoveScreen(s *Screen) error {

	// move clients to default screen
	// TODO: check if client is attached to screen
	// TODO: check client write permission
	// TODO: send resize, clear screen, send cursor position and screen to client
	// TODO: kill clients

	// remove screen
	for i, sc := range m.Screens {
		if sc == s {
			// TODO: kill clients
			m.Screens = append(
				m.Screens[:i],
				m.Screens[i+1:]...)
		}
	}

	return nil
}

// get default screen
func (m *Manager) GetDefaultScreen() *Screen {
	// todo check if default screen exists
	return m.Screens[0]
}

// get screen by id
func (m *Manager) GetScreenByID(id int) (bool, *Screen) {
	// todo check if screen exists
	return true, m.Screens[id]
}

// get screen by Title
func (m *Manager) GetScreenByTitle(title string) (bool, *Screen) {
	for _, s := range m.Screens {
		if s.Title == title {
			return true, s
		}
	}
	return false, nil
}

func New(rows, columns int) *Screen {
	s := &Screen{
		columns:           columns,
		rows:              rows,
		Stream:            stream.New(),
		mt:                mterm.New(rows, columns),
		ClientsProperties: make(map[*Client]*ScreenClientProperties),
		//mx:                sync.Mutex{},
	}

	go s.writeToAttachedClients()

	return s
}

func (s *Screen) RemoveAttachedClient(c *Client) {
	for i, ac := range s.Clients {
		if ac == c {
			s.Clients = append(
				s.Clients[:i],
				s.Clients[i+1:]...)
			s.ClientsProperties[c] = nil
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
			os.Exit(1)
		}

		func() {
			s.mx.Lock()
			defer s.mx.Unlock()
			for _, c := range s.Clients {
				select {
				case <-c.done:
					s.RemoveAttachedClient(c)
					continue
				default:
					if c.CurrentScreen != s { // client is attached to this screen but is not the current screen
						continue
					}
					err = c.Send(constants.MSG, msg[:n])
					if err != nil {
						log.Printf("error writing to websocket: %s\r\n", err)
						c.Close()
						s.RemoveAttachedClient(c)
						continue
					}
				}
			}
		}()
	}
}

// Writer interface
func (s *Screen) Write(p []byte) (n int, err error) {
	// write to stdout
	//n, err = os.Stdout.Write(p)
	//if err != nil {
	//	log.Printf("error writing to stdout: %s\r\n", err)
	//	return
	//}

	s.mt.Write(p) // write to mterm buffer

	// write to websocket
	s.Stream.Write(p)

	n = len(p)
	return
}

func (s *Screen) updateToCurrentState(c *Client) {
	crows, ccolumns := s.CursorPos()
	msg := s.GetScreenAsANSI()

	c.Send(constants.RESIZE,
		[]byte(fmt.Sprintf("%d:%d", s.rows, s.columns)))

	m := fmt.Sprintf("\033[8;%d;%dt\033[0;0H%s\033[%d;%dH",
		s.rows, s.columns, msg, crows+1, ccolumns+1)

	c.Send(constants.MSG, []byte(m))

}

func (s *Screen) Read(p []byte) (n int, err error) {
	return s.Stream.Read(p)
}

func (s *Screen) Send(prefix byte, p []byte) (err error) {
	//s.mx.Lock()
	//defer s.mx.Unlock()
	for i, c := range s.Clients {
		err = c.Send(prefix, p)
		if err != nil {
			log.Printf("error writing to websocket: %s\r\n", err)
			c.Close()
			// remove client
			s.Clients = append(
				s.Clients[:i],
				s.Clients[i+1:]...)
			// remove client properties
			s.ClientsProperties[c] = nil
			continue
		}
	}

	return nil
}

// Resize screen
func (s *Screen) Resize(rows, columns int) {
	s.mx.Lock()
	defer s.mx.Unlock()
	if rows == s.rows && columns == s.columns {
		return
	}

	s.rows = rows
	s.columns = columns
	s.mt.Resize(rows, columns)

	s.Write([]byte(fmt.Sprintf("\033[8;%d;%dt",
		s.rows,
		s.columns,
	)))

	s.Send(constants.RESIZE,
		[]byte(fmt.Sprintf("%d:%d", rows, columns)))
}

// Get screen size
func (s *Screen) Size() (rows, columns int) {
	return s.rows, s.columns
}

// GetScreenAsANSI returns the screen as ANSI
func (s *Screen) GetScreenAsANSI() []byte {
	return s.mt.GetScreenAsAnsi()
}

// CursorPos() returns the cursor position
func (s *Screen) CursorPos() (rows, columns int) {
	return s.mt.CursorPos()
}

func NewClient(conn *websocket.Conn) *Client {
	c := &Client{
		bs:      stream.New(),
		conn:    conn,
		outbuff: make([]byte, constants.BufferSize),
		mx:      sync.Mutex{},
		done:    make(chan struct{}),
	}

	go c.writeLoop()
	go c.HandleInput()

	return c
}

func (c *Client) Close() {
	select {
	case <-c.done:
		return
	default:
		close(c.done)
		c.conn.Close(websocket.StatusNormalClosure, "")
	}
}

// Send sends a message to the client using the stream
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

// Write writes to the stream
func (c *Client) Write(p []byte) (n int, err error) {
	err = c.Send(constants.MSG, p)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Read reads from the stream
func (c *Client) Read(p []byte) (n int, err error) {
	return c.bs.Read(p)
}

// ReadFromWS reads from the websocket
func (c *Client) ReadFromWS(p []byte) (n int, err error) {
	_, r, err := c.conn.Read(context.Background())
	if err != nil {
		return 0, err
	}

	n = copy(p, r)
	return n, nil
}

// writeLoop writes to the websocket
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
					log.Printf("error writing to websocket: %s, %v\r\n",
						err, cs)
				}
				// removeConnection(c)
				c.Close()
				return
			}
		}
	}
}
