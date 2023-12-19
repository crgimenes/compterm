package screen

import (
	"os"

	"github.com/crgimenes/compterm/client"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/stream"
)

type ConnectedClient struct {
	WritePermission bool
	wsStreamEnabled bool // Websocket stream enabled
	mt              *mterm.Terminal
	Client          *client.Client
}

type Screen struct {
	Width   int
	Height  int
	Clients []*ConnectedClient
	Stream  *stream.Stream
	ptmx    *os.File
}

func NewScreen(width, height int) *Screen {
	return &Screen{
		Width:  width,
		Height: height,
		Stream: stream.New(),
	}
}

// Writer interface
func (s *Screen) Write(p []byte) (n int, err error) {
	return 0, nil
}
