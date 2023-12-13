package client

import (
	"context"
	"log"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/stream"
	"nhooyr.io/websocket"
)

type Client struct {
	bs          *stream.Stream
	conn        *websocket.Conn
	localBuffer []byte
	sbuff       []byte
	IP          string
	Nick        string
}

func New(conn *websocket.Conn) *Client {
	return &Client{
		bs:          stream.New(),
		conn:        conn,
		localBuffer: make([]byte, constants.BufferSize),
		sbuff:       make([]byte, constants.BufferSize),
	}
}

func (c *Client) Send(prefix byte, p []byte) (n int, err error) {
	c.sbuff[0] = prefix
	n = copy(c.sbuff[1:], p)
	return c.bs.Write(c.sbuff[:n+1])
}

func (c *Client) Write(p []byte) (n int, err error) {
	return c.bs.Write(p)
}

func (c *Client) Read(p []byte) (n int, err error) {
	return c.bs.Read(p)
}

func (c *Client) ReadFromWS(p []byte) (n int, err error) {
	_, r, err := c.conn.Read(context.Background())
	if err != nil {
		return 0, err
	}

	n = copy(p, r)
	return n, nil
}

func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

func (c *Client) WriteLoop() {
	for {
		n, err := c.bs.Read(c.localBuffer)
		if err != nil {
			log.Printf("error reading from byte stream: %s\r\n", err)
			return
		}

		err = c.conn.Write(context.Background(), websocket.MessageBinary, c.localBuffer[:n])
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
