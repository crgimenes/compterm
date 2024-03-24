package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/crgimenes/compterm/config"
	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"
	"nhooyr.io/websocket"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type wsClient struct {
	conn *websocket.Conn
}

func (w *wsClient) Write(p []byte) (n int, err error) {
	err = w.conn.Write(context.Background(), websocket.MessageText, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *wsClient) Read(p []byte) (n int, err error) {
	_, r, err := w.conn.Reader(context.Background())
	if err != nil {
		return 0, err
	}
	return r.Read(p)
}

func (w *wsClient) Close() error {
	return w.conn.Close(websocket.StatusNormalClosure, "")
}

func New(wsserver string) (*wsClient, error) {
	w := &wsClient{}
	var err error
	w.conn, _, err = websocket.Dial(
		context.Background(),
		wsserver,
		nil,
	)
	return w, err
}

var (
	ptmx   *os.File
	GitTag string = "0.0.0v"
	mx     sync.Mutex
	ws     *wsClient
)

func runCmd() {
	// clean screen
	//os.Stdout.WriteString("\033[2J\033[0;0H")

	var (
		err error
	)

	cmdAux := config.CFG.Command
	cmd := strings.Split(cmdAux, " ")

	c := exec.Command(cmd[0], cmd[1:]...)
	// Start the command with a pty.
	mx.Lock()
	ptmx, err = pty.Start(c)
	if err != nil {
		log.Fatalf("error starting pty: %s\r\n", err)
	}
	mx.Unlock()

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

	_ = pty.InheritSize(os.Stdin, ptmx)

	//defaultScreen.Input = ptmx // resive input from from user

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	//_, _ = io.Copy(defaultScreen, ptmx)
	//go func() { _, _ = io.Copy(ws, ptmx) }()

	go func() {
		buf := make([]byte, constants.BufferSize)
		encodedBuf := make([]byte, constants.BufferSize)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Fatalf("error reading from pty: %s\r\n", err)
			}
			if n > 0 {
				_, _ = os.Stdout.Write(buf[:n])
				n, err := protocol.Encode(encodedBuf, buf[:n], 01, 0)
				if err != nil {
					log.Fatalf("error encoding data: %s\r\n", err)
				}

				_, _ = ws.Write(encodedBuf[:n])

			}
		}
	}()

	// Wait for the command to finish.
	err = c.Wait()
	if err != nil {
		log.Fatalf("error waiting for command: %s\r\n", err)
	}
}

func updateTerminalSize() {
	// Update window size.
	mx.Lock()
	_ = pty.InheritSize(os.Stdin, ptmx)
	mx.Unlock()

	// encode with protocol package and send to websocket
	columns, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("error getting size: %s\r\n", err)
	}

	///////////////////////////////////
	encodedBuf := make([]byte, constants.BufferSize)
	n, err := protocol.Encode(
		encodedBuf,
		[]byte(fmt.Sprintf("\033[8;%d;%dt", rows, columns)),
		constants.MSG,
		0)
	if err != nil {
		log.Fatalf("error encoding data: %s\r\n", err)
	}
	_, _ = ws.Write(encodedBuf[:n])

	///////////////////////////////////

	n, err = protocol.Encode(
		encodedBuf,
		[]byte(fmt.Sprintf("%d:%d", rows, columns)),
		constants.RESIZE,
		0)
	if err != nil {
		log.Fatalf("error encoding data: %s\r\n", err)
	}
	_, _ = ws.Write(encodedBuf[:n])

}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

	err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %s\n", err)
	}

	ws, err = New("ws://localhost:2200/wsproxy")
	if err != nil {
		log.Fatalf("error connecting to websocket: %s\n", err)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for caux := range ch {
			switch caux {
			case syscall.SIGWINCH:
				updateTerminalSize()
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	updateTerminalSize()

	runCmd()

}
