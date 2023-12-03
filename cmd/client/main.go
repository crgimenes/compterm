package main

import (
	"compterm/constants"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"compterm/mterm"

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
		mt, data, err := c.Read(context.Background())
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
			data[0],
			len(data),
		)

		// command is the first byte of data
		command := data[0]
		switch command {
		case 0x1:
			log.Println("SCREEN:")
			log.Printf("%q", string(data[1:]))

			if _, err := t.Write(data[1:]); err != nil {
				// could be ignored
				if err, ok := err.(mterm.EscapeError); ok {
					d := data[1:]
					mm := max(err.Offset-20, 0)
					mx := min(err.Offset+20, len(d))
					sub := d[mm:mx]

					off := max(err.Offset-mm, 0)

					quoted := fmt.Sprintf("%q", string(sub))
					// Hacky way to get the offset considering the escaped escape sequences
					quotedLoc := fmt.Sprintf("%q", string(sub[:off-2]))
					log.Fatalf("Error:\n%s\n\033[%dC^ %v\n", quoted, len(quotedLoc), err)
				}
			}

			fmt.Printf("\033]0m%s\a", t.Title)
			fmt.Println("Title: ", t.Title)
			lines := strings.Split(string(t.DBG()), "\n")
			for i, line := range lines {
				fmt.Printf("%02d|%s\033[0m|%02d\n", i, line, i)
			}
		case 0x2:
			var c, l int
			fmt.Sscanf(string(data[1:]), "%d:%d", &c, &l)
			// log.Println("Resizing cols:", c, "lines:", l)
			t.Resize(c, l)
		// resize
		default:
			log.Printf("Unknown command: %v\n", command)
		}
	}

	c.Close(websocket.StatusNormalClosure, "")
}
