package main

import (
	"compterm/byteStream"
	"compterm/client"
	"compterm/config"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kr/pty"
	"golang.org/x/term"
	"nhooyr.io/websocket"
)

type termIO struct{}

var (
	termio    = termIO{}
	clients   []*client.Client
	connMutex sync.Mutex
	bs        = byteStream.NewByteStream()
	ptmx      *os.File

	// enbed assets
	//go:embed assets/*
	assets embed.FS
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

func writeAllWS() {

	msg := make([]byte, 8192)
	for {
		n, err := bs.Read(msg)
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Printf("error reading from byte stream: %s\r\n", err)
			os.Exit(1)
		}

		connMutex.Lock()
		for _, c := range clients {
			cn, err := c.Write(msg[:n]) // TODO: check if cn < n and if so, write the rest
			_ = cn                      // TODO: remove this line
			if err != nil {
				log.Printf("error writing to websocket: %s\r\n", err)
				removeConnection(c)
				connMutex.Unlock()
			}
		}
		connMutex.Unlock()
	}
}

func (o termIO) Write(p []byte) (n int, err error) {
	// append to out.txt file
	//appendToOutFile(p)

	// write to stdout
	n, err = os.Stdout.Write(p)
	if err != nil {
		return
	}

	bs.Write(p)

	return
}

func runCmd() {
	var err error
	cmdAux := config.CFG.Command
	cmd := strings.Split(cmdAux, " ")

	c := exec.Command(cmd[0], cmd[1:]...)
	// Start the command with a pty.
	ptmx, err = pty.Start(c)
	if err != nil {
		log.Fatalf("error starting pty: %s\r\n", err)
	}

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("error setting stdin in raw mode: %s\r\n", err)
	}

	restoreTerm := func() {
		_ = ptmx.Close()
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}
	defer restoreTerm()

	// Handle signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH, syscall.SIGTERM, os.Interrupt)
	go func() {
		for caux := range ch {
			switch caux {
			case syscall.SIGWINCH:
				// Update window size.
				_ = pty.InheritSize(os.Stdin, ptmx)

				sizeWidth, sizeHeight, err := term.GetSize(
					int(os.Stdin.Fd()))
				if err != nil {
					log.Fatalf("error getting size: %s\r\n", err)
				}
				bs.Write([]byte(fmt.Sprintf("\033[8;%d;%dt",
					sizeHeight, sizeWidth)))
			case syscall.SIGTERM, os.Interrupt:
				removeAllConnections()
				restoreTerm()
				os.Exit(0)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(termio, ptmx)

	// Wait for the command to finish.
	err = c.Wait()
	if err != nil {
		log.Fatalf("error waiting for command: %s\r\n", err)
	}

	// Close the websocket connections
	removeAllConnections()
	restoreTerm()
	os.Exit(0)
}

func removeAllConnections() {
	connMutex.Lock()
	defer connMutex.Unlock()

	for _, c := range clients {
		err := c.Close()
		if err != nil {
			log.Printf("error closing websocket: %s\r\n", err)
		}
	}
	clients = nil
}

func removeConnection(c *client.Client) {
	connMutex.Lock()
	defer connMutex.Unlock()

	for i, client := range clients {
		if client == c {
			client.Close()
			clients = append(clients[:i], clients[i+1:]...)
			break
		}
	}
}

func readMessages(client *client.Client) {
	for {
		buffer := make([]byte, 8192)
		n, err := client.ReadFromWS(buffer)
		if err != nil {
			log.Printf("error reading from websocket: %s\r\n", err)
			removeConnection(client)
			return
		}

		processInput(client, buffer[:n])
	}
}

func processInput(client *client.Client, b []byte) {
	_, _ = io.Copy(ptmx, strings.NewReader(string(b)))
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)

		return
	}

	client := client.New(c)
	if config.CFG.MOTD != "" {
		client.Write([]byte(config.CFG.MOTD + "\r\n"))
	}

	connMutex.Lock()
	clients = append(clients, client)
	connMutex.Unlock()

	go client.WriteLoop()

	// go readMessages(client)
}

func serveHTTP() {
	subFS, err := fs.Sub(assets, "assets")
	if err != nil {
		log.Fatalf("error getting sub filesystem: %s\r\n", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler)
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	s := &http.Server{
		Handler:        mux,
		Addr:           config.CFG.Listen,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Listening on port %v\n", config.CFG.Listen)
	log.Fatal(s.ListenAndServe())
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %s\r\n", err)
	}

	go writeAllWS()
	go runCmd()

	runtime.Gosched()

	serveHTTP()
}
