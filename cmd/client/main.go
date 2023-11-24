package main

import (
	"context"
	"encoding/base64"
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

	c, _, err := websocket.Dial(context.Background(), "ws://localhost:8080/ws", nil)
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

		b, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			log.Println(err)

			return
		}
		os.Stdout.Write(b)
	}

	c.Close(websocket.StatusNormalClosure, "")
}
