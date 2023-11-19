package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
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
	signal.Notify(ch, syscall.SIGWINCH, syscall.SIGTERM, os.Interrupt)
	go func() {
		for caux := range ch {
			switch caux {
			case syscall.SIGWINCH:
				// Send clear scape sequence to the pty them send the size of the terminal to the websocket.
				clear := "\033[H\033[2J\033[3J\033[;H\033[0m"
				sizeWidth, sizeHeight, err := term.GetSize(int(os.Stdin.Fd()))
				if err != nil {
					log.Fatalf("error getting size: %s\r\n", err)
				}

				os.Stdout.Write([]byte(clear))

				os.Stdout.Write([]byte("\033[34;40m"))
				termBuffer := strings.Repeat("•", sizeWidth*sizeHeight)
				os.Stdout.Write([]byte(termBuffer))
				os.Stdout.Write([]byte("\033[0m\033[H"))

			case syscall.SIGTERM, os.Interrupt:
				c.Close(websocket.StatusNormalClosure, "")
				restoreTerm()
				os.Exit(0)
			}
		}
	}()
	ch <- syscall.SIGWINCH

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
		os.Stdout.Write(data)
	}

	c.Close(websocket.StatusNormalClosure, "")
}
