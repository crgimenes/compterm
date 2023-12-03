package main

import (
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

	fmt.Println("\033[2J\033[H")

	t := mterm.New(24, 80)
	for {
		_, data, err := c.Read(context.Background())
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
		log.Println("Data:", len(data))

		// command is the first byte of data
		command := data[0]
		switch command {
		case 0x1:
			log.Println("\033[2J\033[HSCREEN:")

			t.Write(data[1:])

			fmt.Printf("\033]0m%s\a", t.Title)
			fmt.Println("Title: ", t.Title)
			lines := strings.Split(string(t.DBG()), "\n")
			for i, line := range lines {
				fmt.Printf("%02d|%s\033[0m|%02d\n", i, line, i)
			}
			log.Printf("%q", string(data[1:]))
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
