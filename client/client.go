package client

import (
	"compterm/byteStream"
	"context"
	"log"

	"nhooyr.io/websocket"
)

type Client struct {
	bs          *byteStream.ByteStream
	localBuffer []byte
	conn        *websocket.Conn
	IP          string
	Nick        string
}

func New(conn *websocket.Conn) *Client {
	return &Client{
		bs:          byteStream.NewByteStream(),
		localBuffer: make([]byte, 8192),
		conn:        conn,
	}
}

func (c *Client) Write(p []byte) (n int, err error) {
	return c.bs.Write(p)
}

func (c *Client) Read(p []byte) (n int, err error) {
	return c.bs.Read(p)
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
