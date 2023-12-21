package screen

import (
	"log"
	"os"

	"github.com/crgimenes/compterm/client"
	"github.com/crgimenes/compterm/stream"
)

type ConnectedClient struct {
	WritePermission bool
	wsStreamEnabled bool // Websocket stream enabled
	Client          *client.Client
}

type Screen struct {
	Title   string
	Width   int
	Height  int
	Clients []*ConnectedClient
	Stream  *stream.Stream
	ptmx    *os.File
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
}

// detach a client from a screen
func (m *Manager) DetachClient(c *client.Client) {
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

func New(height, width int) *Screen {
	return &Screen{
		Width:  width,
		Height: height,
		Stream: stream.New(),
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

	// write to websocket
	s.Stream.Write(p)
	return
}
