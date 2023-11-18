package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kr/pty"
	"golang.org/x/term"
	"nhooyr.io/websocket"
)

const readTimeout = 10 * time.Second

type out struct {
}

var (
	o           = out{}
	connections []*websocket.Conn
	connMutex   sync.Mutex
)

// appendToOutFile append bytes to out.txt file
func appendToOutFile(p []byte) {
	f, err := os.OpenFile("out.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("error opening file: %s\r\n", err)
	}
	defer f.Close()

	if _, err := f.Write(p); err != nil {
		log.Fatalf("error writing to file: %s\r\n", err)
	}
}

var writeWSChan = make(chan []byte, 1024)

func writeWSLoop() {
	for {
		select {
		case msg := <-writeWSChan:
			for _, c := range connections {
				err := c.Write(context.Background(), websocket.MessageText, msg)
				if err != nil {
					log.Println(err)
					removeConnection(c) // TODO: is this safe?
				}
			}
		}
	}
}

func (o out) Write(p []byte) (n int, err error) {

	// append to out.txt file
	//appendToOutFile(p)

	n, err = os.Stdout.Write(p)

	return
}

func runCmd() {
	c := exec.Command(os.Args[1], os.Args[2:]...)

	// Start the command with a pty.
	ptmx, err := pty.Start(c)
	if err != nil {
		log.Fatalf("error starting pty: %s\r\n", err)
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle pty size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			err := pty.InheritSize(os.Stdin, ptmx)
			if err != nil {
				log.Printf("error resizing pty: %s\r\n", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(o, ptmx)

}

func removeConnection(c *websocket.Conn) {
	connMutex.Lock()
	defer connMutex.Unlock()

	for i, conn := range connections {
		if conn == c {
			connections = append(connections[:i], connections[i+1:]...)
			break
		}
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, %q", r.URL.Path)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	connMutex.Lock()
	connections = append(connections, c)
	connMutex.Unlock()

	defer func() {
		c.Close(websocket.StatusInternalError, "the sky is falling")
		removeConnection(c)
	}()

	for {
		ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
		_, msg, err := c.Read(ctx)
		cancel()
		if err != nil {
			log.Println(err)
			c.Close(websocket.StatusNormalClosure, "")
			return
		}

		err = c.Write(r.Context(), websocket.MessageText, msg)
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func main() {
	go runCmd()
	go writeWSLoop()

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/", homeHandler)

	s := &http.Server{
		Handler:        mux,
		Addr:           fmt.Sprintf(":%d", 8080),
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Listening on port %d\n", 8080)
	log.Fatal(s.ListenAndServe())

}
