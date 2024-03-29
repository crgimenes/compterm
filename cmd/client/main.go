package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/protocol"
	"nhooyr.io/websocket"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	c, _, err := websocket.Dial(context.Background(), "ws://localhost:2200/ws", nil)
	if err != nil {
		log.Println(err)
	}
	defer c.CloseNow()
	c.SetReadLimit(-1)

	fmt.Println("\033[2J\033[H")

	t := mterm.New(24, 80)
	for {
		mt, pdata, err := c.Read(context.Background())
		if err != nil {
			if err == io.EOF {
				log.Println(">>> EOF")
				break
			}
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				log.Println(">>> Normal closure")
				break
			}
			log.Printf("Read error: %v\n", err)
			break
		}
		log.Printf("Message Type = %v, data[0] = %d, len(data) = %d\n",
			mt,
			pdata[0],
			len(pdata),
		)

		data := make([]byte, len(pdata))
		command, _, _, err := protocol.Decode(data, pdata)
		if err != nil {
			log.Printf("Decode error: %v\n", err)
		}

		// command is the first byte of data
		// command := data[0]
		switch command {
		case constants.MSG:
			log.Println("SCREEN:")
			log.Printf("%q", string(data))

			if _, err := t.Write(data); err != nil {
				// could be ignored
				if err, ok := err.(mterm.EscapeError); ok {
					d := data
					mm := max(err.Offset-20, 0)
					mx := min(err.Offset+20, len(d))
					sub := d[mm:mx]

					off := max(err.Offset-mm, 0)

					quoted := fmt.Sprintf("%q", string(sub))
					// Hacky way to get the offset considering the escaped escape sequences
					off = max(off-2, 0)
					quotedLoc := fmt.Sprintf("%q", string(sub[:off]))
					log.Printf("Error:\n%s\n\033[%dC^ %v\n", quoted, len(quotedLoc), err)
				}
			}

			fmt.Printf("\033]0m%s\a", t.Title)
			fmt.Println("Title: ", t.Title)
			fmt.Println()
			lines := strings.Split(string(t.DBG()), "\r\n")
			for i, line := range lines {
				fmt.Printf("%02d|%s\033[0m|%02d\n", i, line, i)
			}
		case constants.RESIZE:
			var c, l int
			_, _ = fmt.Sscanf(string(data), "%d:%d", &c, &l)
			t.Resize(c, l)
		// resize
		default:
			log.Printf("Unknown command: %v\n", command)
		}
	}

	_ = c.Close(websocket.StatusNormalClosure, "")
}
