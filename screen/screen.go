package screen

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/crgimenes/compterm/client"
	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/stream"
)

type AttachedClient struct {
	WritePermission bool
	CurrentScreen   *Screen
	Client          *client.Client
}

type Screen struct {
	Title   string
	Columns int
	Rows    int
	Clients []*AttachedClient
	Stream  *stream.Stream
	mt      *mterm.Terminal
}

type Manager struct {
	Screens []*Screen
}

func NewManager() *Manager {
	s := New(80, 25)
	s.Title = "default"

	return &Manager{
		Screens: []*Screen{s},
	}
}

// attach a client to a screen
func (m *Manager) AttachClient(c *client.Client, screen *Screen, writePermission bool) error {
	// check if client is already attached
	for _, ac := range screen.Clients {
		if ac.Client == c {
			ac.WritePermission = writePermission
			ac.CurrentScreen = screen
			// TODO: send resize, clear screen, send cursor position and screen to client
			return nil
		}
	}

	// attach client to screen
	screen.Clients = append(screen.Clients, &AttachedClient{
		WritePermission: writePermission,
		CurrentScreen:   screen,
		Client:          c,
	})
	// TODO: send resize, clear screen, send cursor position and screen to client

	return nil
}

// detach a client from a screen
func (m *Manager) DetachClient(c *client.Client, screen *Screen) error {
	// check if client is attached
	for i, ac := range screen.Clients {
		if ac.Client == c {
			screen.Clients = append(
				screen.Clients[:i],
				screen.Clients[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("client not attached")
}

// detach a client from all screens
func (m *Manager) DetachClientFromAllScreens(c *client.Client) {
	for _, s := range m.Screens {
		for i, ac := range s.Clients {
			if ac.Client == c {
				s.Clients = append(
					s.Clients[:i],
					s.Clients[i+1:]...)
			}
		}
	}
}

// change current screen
func (m *Manager) ChangeScreen(c *client.Client, screen *Screen) error {
	// check if client is attached to screen return error if not
	for _, ac := range screen.Clients {
		if ac.Client == c {
			ac.CurrentScreen = screen
			return nil
		}
	}

	return fmt.Errorf("client not attached to screen")
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
	for _, ac := range m.Screens[id].Clients {
		m.Screens[0].Clients = append(m.Screens[0].Clients, ac)
	}

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
		Columns: columns,
		Rows:    rows,
		Stream:  stream.New(),
		mt:      mterm.New(rows, columns),
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
			err = c.Client.Send(constants.MSG, msg[:n])
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
