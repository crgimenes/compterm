package client

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"
	"github.com/crgimenes/compterm/stream"
	"nhooyr.io/websocket"
)

type Client struct {
	CurrentScreenID int
	bs              *stream.Stream
	conn            *websocket.Conn
	IP              string
	Nick            string
	SessionID       string
	outbuff         []byte // used to avoid memory allocation on each write
	mx              sync.Mutex
}

func New(conn *websocket.Conn) *Client {
	return &Client{
		bs:      stream.New(),
		conn:    conn,
		outbuff: make([]byte, constants.BufferSize),
		mx:      sync.Mutex{},
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

// Close closes the websocket connection
func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

// WriteLoop writes to the websocket
func (c *Client) WriteLoop() {
	buff := make([]byte, constants.BufferSize)
	for {
		n, err := c.bs.Read(buff)
		if err != nil {
			log.Printf("error reading from byte stream: %s\r\n", err)
			return
		}

		err = c.conn.Write(context.Background(), websocket.MessageBinary, buff[:n])
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				log.Printf("error writing to websocket: %s, %v\r\n",
					err, websocket.CloseStatus(err)) // TODO: send to file, not the screen
			}
			// removeConnection(c)
			return
		}
	}
}

func (c *Client) ReadLoop(ptmx *os.File) {
	buff := make([]byte, constants.BufferSize)
	for {
		n, err := c.ReadFromWS(buff)
		if err != nil {
			log.Printf("error reading from websocket: %s\r\n", err)
			//removeConnection(client)
			return
		}

		// write to pty
		_, err = io.Copy(ptmx, strings.NewReader(string(buff[:n])))
		if err != nil {
			log.Printf("error writing to pty: %s\r\n", err)
			//removeConnection(client)
			return
		}
	}
}
