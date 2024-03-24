package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"

	"nhooyr.io/websocket"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, _, err := websocket.Dial(ctx, "ws://localhost:2200/ws", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.CloseNow()

	data := make([]byte, constants.BufferSize)

	for {
		_, msg, err := c.Read(ctx)
		if err != nil {
			log.Fatal(err)
		}

		cmd, n, _, err := protocol.Decode(data, msg)
		if err != nil {
			log.Fatal(err)
		}

		if cmd == constants.MSG {
			_, _ = os.Stdout.Write(data[:n])
		}
	}
}
