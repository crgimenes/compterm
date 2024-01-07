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
	Columns           int
	Rows              int
	Clients           []*Client // Attached clients
	ClientsProperties map[*Client]*ScreenClientProperties
	Stream            *stream.Stream
	mt                *mterm.Terminal // terminal emulator
	Input             io.Writer       // receive input (stdin) from attached clients and other sources
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
func (m *Manager) AttachClient(c *Client, screen *Screen, writePermission bool) error {
	// check if client is already attached
	for _, ac := range screen.Clients {
		if ac == c {
			ac.CurrentScreen = screen
			screen.ClientsProperties[c].WritePermission = writePermission
			// TODO: send resize, clear screen, send cursor position and screen to client
			return nil
		}
	}

	// attach client to screen
	screen.Clients = append(screen.Clients, c)
	screen.ClientsProperties[c] = &ScreenClientProperties{
		WritePermission: writePermission,
	}
	c.CurrentScreen = screen

	// TODO: send resize, clear screen, send cursor position and screen to client

	return nil
}

// detach a client from a screen
func (m *Manager) DetachClient(c *Client, screen *Screen) error {
	// check if client is attached
	for i, ac := range screen.Clients {
		if ac == c {
			screen.Clients = append(
				screen.Clients[:i],
				screen.Clients[i+1:]...)
			screen.ClientsProperties[c] = nil
			return nil
		}
	}

	return fmt.Errorf("client not attached")
}

// detach a client from all screens
func (m *Manager) DetachClientFromAllScreens(c *Client) {
	for _, s := range m.Screens {
		for i, ac := range s.Clients {
			if ac == c {
				s.Clients = append(
					s.Clients[:i],
					s.Clients[i+1:]...)
				s.ClientsProperties[c] = nil
			}
		}
	}
}

// change current screen
func (m *Manager) ChangeScreen(c *Client, screen *Screen) error {
	// check if client is attached to screen return error if not
	for _, ac := range screen.Clients {
		if ac == c {
			ac.CurrentScreen = screen
			return nil
		}
	}

	return fmt.Errorf("client not attached to screen")
}

// handle client input
func (m *Manager) HandleInput(c *Client) {
	buff := make([]byte, constants.BufferSize)
	for {
		n, err := c.ReadFromWS(buff) // Read from websocket
		if err != nil {
			sc := websocket.CloseStatus(err)
			if sc != websocket.StatusNormalClosure && sc != -1 {
				log.Printf("error reading from websocket: %s\r\n", err)
			}
			c.Close()
			//removeConnection(client)
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
				//removeConnection(client)
				return
			}
		}
	}
}

// remove a screen
func (m *Manager) RemoveScreen(id int) error {
	// screen 0 is the default screen and can't be removed
	if id == 0 {
		return fmt.Errorf("can't remove default screen")
	}

	if id < 0 || id > len(m.Screens) {
		return fmt.Errorf("invalid screen id")
	}

	// move clients to default screen
	// TODO: check if client is attached to screen
	// TODO: check client write permission
	// TODO: send resize, clear screen, send cursor position and screen to client
	m.Screens[0].Clients = append(
		m.Screens[0].Clients,
		m.Screens[id].Clients...)

	// remove screen
	m.Screens = append(m.Screens[:id], m.Screens[id+1:]...)

	return nil
}

// remove screen and kill clients
func (m *Manager) KillScreen(id int) error {
	// screen 0 is the default screen and can't be removed
	if id == 0 {
		return fmt.Errorf("can't remove default screen")
	}

	if id < 0 || id > len(m.Screens) {
		return fmt.Errorf("invalid screen id")
	}

	// TODO: kill clients

	// remove screen
	m.Screens = append(m.Screens[:id], m.Screens[id+1:]...)

	return nil
}

// get default screen
func (m *Manager) GetDefaultScreen() *Screen {
	return m.Screens[0]
}

// get screen by id
func (m *Manager) GetScreenByID(id int) (bool, *Screen) {
	if id < 0 || id > len(m.Screens) {
		return false, nil
	}
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
		Columns:           columns,
		Rows:              rows,
		Stream:            stream.New(),
		mt:                mterm.New(rows, columns),
		ClientsProperties: make(map[*Client]*ScreenClientProperties),
	}

	go s.writeToAttachedClients()

	return s
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

			//removeAllConnections()
			os.Exit(1)
		}

		//connMutex.Lock()
		for _, c := range s.Clients {
			if c.CurrentScreen != s { // client is attached to this screen but is not the current screen
				continue
			}
			err = c.Send(constants.MSG, msg[:n])
			if err != nil {
				log.Printf("error writing to websocket: %s\r\n", err)
				//removeConnection(c)
				continue
			}
		}
		//connMutex.Unlock()
	}
}

// Writer interface
func (s *Screen) Write(p []byte) (n int, err error) {
	// write to stdout
	n, err = os.Stdout.Write(p)
	if err != nil {
		log.Printf("error writing to stdout: %s\r\n", err)
		return
	}

	s.mt.Write(p) // write to mterm buffer

	// write to websocket
	s.Stream.Write(p)
	return
}

func (s *Screen) Read(p []byte) (n int, err error) {
	return s.Stream.Read(p)
}

// Resize screen
func (s *Screen) Resize(rows, columns int) {
	s.Rows = rows
	s.Columns = columns
	s.mt.Resize(rows, columns)

	s.Write([]byte(fmt.Sprintf("\033[8;%d;%dt",
		s.Rows,
		s.Columns,
	)))

	// TODO: send resize to clients
}

// Get screen size
func (s *Screen) Size() (rows, columns int) {
	return s.Rows, s.Columns
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
				if cs != websocket.StatusNormalClosure && cs != -1 {
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
