package main

import (
	"compterm/byteStream"
	"context"
	"crypto/md5"
	"encoding/base64"
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

type termIO struct{}

var (
	termio      = termIO{}
	connections []*websocket.Conn
	connMutex   sync.Mutex
	bs          = byteStream.NewByteStream()
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

var contadorDePacotes = 1

func writeAllWS() {

	msg := make([]byte, 1024)
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

		if n == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// convert to base64

		msgB64 := base64.StdEncoding.EncodeToString(msg[:n])

		// md5 of base64
		//md5msg := md5.Sum(msg)

		//fmt.Printf("md5: %d -> %x\r\n", contadorDePacotes, md5msg)
		//os.Stderr.WriteString(fmt.Sprintf("md5: %d -> %x\r\n", contadorDePacotes, md5msg))

		//payload := fmt.Sprintf("%d", contadorDePacotes) + ";" + fmt.Sprintf("%x", md5msg) + "|" + string(msgB64)
		payload := fmt.Sprintf("%d", contadorDePacotes) + ";x|" + string(msgB64)
		contadorDePacotes++

		for _, c := range connections {
			err := c.Write(context.Background(), websocket.MessageText, []byte(payload))
			if err != nil {
				if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					log.Printf("error writing to websocket: %s, %v\r\n", err, websocket.CloseStatus(err)) // TODO: send to file, not the screen
				}
				removeConnection(c) // TODO: is this safe?
			}
		}
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

	// write to websocket
	//writeWSChan <- p

	// writeAllWS(p)

	bs.Write(p)

	return
}

func runCmd() {
	c := exec.Command(os.Args[1], os.Args[2:]...)
	// Start the command with a pty.
	ptmx, err := pty.Start(c)
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

				sizeWidth, sizeHeight, err := term.GetSize(int(os.Stdin.Fd()))
				if err != nil {
					log.Fatalf("error getting size: %s\r\n", err)
				}
				bs.Write([]byte(fmt.Sprintf("\033[8;%d;%dt", sizeHeight, sizeWidth)))
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

	for _, conn := range connections {
		err := conn.Close(websocket.StatusNormalClosure, "server shutdown")
		if err != nil {
			log.Printf("error closing websocket: %s\r\n", err)
		}
	}
	connections = []*websocket.Conn{}
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

func readMessages(c *websocket.Conn) {
	for {
		_, msg, err := c.Read(context.Background())
		if err != nil {
			log.Printf("error reading from websocket: %s\r\n", err)
			removeConnection(c)

			return
		}

		processInput(c, msg)
	}
}

func processInput(c *websocket.Conn, b []byte) {
	log.Printf("received: %q\r\n", b)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)

		return
	}

	b64 := base64.StdEncoding.EncodeToString([]byte("Welcome to the hall of tortured souls!\r\n"))

	msg := fmt.Sprintf("255;%x|%s", md5.Sum([]byte(b64)), b64)

	c.Write(context.Background(), websocket.MessageText, []byte(msg))

	connMutex.Lock()
	connections = append(connections, c)
	connMutex.Unlock()

	//go readMessages(c)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	go runCmd()
	go writeAllWS()

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler)
	mux.Handle("/", http.FileServer(http.Dir("assets")))

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
