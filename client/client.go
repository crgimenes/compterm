package client

import (
	"context"
	"log"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"
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
	SessionID   string
}

func New(conn *websocket.Conn) *Client {
	return &Client{
		bs:          stream.New(),
		conn:        conn,
		localBuffer: make([]byte, constants.BufferSize),
		sbuff:       make([]byte, constants.BufferSize),
	}
}

// DirectSend sends a message to the client without using the stream
func (c *Client) DirectSend(prefix byte, p []byte) (n int, err error) {
	buff := make([]byte, constants.BufferSize)
	//buff[0] = prefix
	//n = copy(buff[1:], p)
	n, err = protocol.Encode(buff, p, prefix, 0)
	if err != nil {
		return 0, err
	}

	err = c.conn.Write(context.Background(), websocket.MessageBinary, buff[:n])
	if err != nil {
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			log.Printf("error writing to websocket: %s, %v\r\n",
				err, websocket.CloseStatus(err)) // TODO: send to file, not the screen
		}
		// removeConnection(c)
		return 0, err
	}
	return n, nil
}

// Send sends a message to the client using the stream
func (c *Client) Send(prefix byte, p []byte) (n int, err error) {
	//c.sbuff[0] = prefix
	//n = copy(c.sbuff[1:], p)

	buff := make([]byte, constants.BufferSize)
	n, err = protocol.Encode(buff, p, prefix, 0)
	if err != nil {
		return 0, err
	}

	return c.bs.Write(buff[:n])

	//return c.bs.Write(c.sbuff[:n+1])
}

// Write writes to the stream
func (c *Client) Write(p []byte) (n int, err error) {
	//return c.bs.Write(p)
	return c.Send(constants.MSG, p)
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
