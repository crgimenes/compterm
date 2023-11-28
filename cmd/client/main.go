package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
	"nhooyr.io/websocket"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	c, _, err := websocket.Dial(context.Background(), "ws://localhost:2200/ws", nil)
	if err != nil {
		log.Println(err)
	}
	defer c.CloseNow()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("error making raw: %s\r\n", err)
	}
	restoreTerm := func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}
	defer restoreTerm()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, os.Interrupt)
	go func() {
		for caux := range ch {
			switch caux {
			case syscall.SIGTERM, os.Interrupt:
				c.Close(websocket.StatusNormalClosure, "")
				restoreTerm()
				os.Exit(0)
			}
		}
	}()

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

		// command is the first byte of data
		command := data[0]
		switch command {
		case 0x1:
			// stdout
			os.Stdout.Write(data[1:])
		case 0x2:
		// resize
		default:
			log.Printf("Unknown command: %v\n", command)
		}
	}

	c.Close(websocket.StatusNormalClosure, "")
}
