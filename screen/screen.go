package screen

import (
	"fmt"
	"log"
	"os"

	"github.com/crgimenes/compterm/client"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/stream"
)

type ConnectedClient struct {
	WritePermission bool
	Client          *client.Client
}

type Screen struct {
	Title   string
	Columns int
	Rows    int
	Clients []*ConnectedClient
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
func (m *Manager) AttachClient(c *client.Client) {
	// TODO: check if client is already attached
	// TODO: check if client has write permission
	// remove client from other screens
	// add client to screen
}

// detach a client from a screen
func (m *Manager) DetachClient(c *client.Client) {
	// TODO: check if client is attached
	// remove client from screen and add to default screen
}

// create a new screen
func (m *Manager) NewScreen(rows, columns int) *Screen {
	s := New(rows, columns)
	m.Screens = append(m.Screens, s)
	return s
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

	// TODO: move clients to default screen

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
	return s
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
