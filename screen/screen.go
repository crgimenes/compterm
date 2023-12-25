package screen

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/crgimenes/compterm/client"
	"github.com/crgimenes/compterm/mterm"
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
	mt      *mterm.Terminal
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
	s := &Screen{
		Width:  width,
		Height: height,
		Stream: stream.New(),
		mt:     mterm.New(height, width),
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

// Resize screen
func (s *Screen) Resize(height, width int) {
	s.Height = height
	s.Width = width
	s.mt.Resize(height, width)

	s.Write([]byte(fmt.Sprintf("\033[8;%d;%dt",
		s.Height,
		s.Width,
	)))
}

// GetScreenAsANSI returns the screen as ANSI
func (s *Screen) GetScreenAsANSI() []byte {
	return s.mt.GetScreenAsAnsi()
}

// CursorPos() returns the cursor position
func (s *Screen) CursorPos() (lin, col int) {
	return s.mt.CursorPos()
}

func (s *Screen) Exec(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	scmd := strings.Split(cmd, " ")
	c := exec.Command(scmd[0], scmd[1:]...)

	ptmx, err := pty.Start(c)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	c.Stderr = stderr

	go io.Copy(ptmx, stdin)
	io.Copy(stdout, ptmx)

	return c.Wait()
}
