package client

import (
	"context"
	"fmt"
	"log"

	"compterm/constants"
	"compterm/stream"

	"nhooyr.io/websocket"
)

type Client struct {
	bs          *stream.Stream
	localBuffer []byte
	conn        *websocket.Conn
	IP          string
	Nick        string
}

func New(conn *websocket.Conn) *Client {
	return &Client{
		bs:          stream.New(),
		localBuffer: make([]byte, constants.BufferSize),
		conn:        conn,
	}
}

func (c Client) SendMessage(p []byte) (n int, err error) {
	p = append([]byte{constants.MSG}, p...)
	return c.Write(p)
}

func (c Client) ResizeTerminal(rows, cols int) (n int, err error) {
	return c.SendCommand(0x2, []byte(fmt.Sprintf("%d:%d", rows, cols)))
}

func (c *Client) SendCommand(prefix byte, p []byte) (n int, err error) {
	return c.bs.Write(append([]byte{prefix}, p...))
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
			//removeConnection(c)
			return
		}
	}
}
