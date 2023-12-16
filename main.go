package main

import (
	"fmt"
	"io"
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

	"github.com/crgimenes/compterm/assets"
	"github.com/crgimenes/compterm/client"
	"github.com/crgimenes/compterm/config"
	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/mterm"
	"github.com/crgimenes/compterm/session"
	"github.com/crgimenes/compterm/stream"

	"github.com/kr/pty"
	"golang.org/x/term"
	"nhooyr.io/websocket"
)

type termIO struct{}

var (
	termio                = termIO{}
	mt                    *mterm.Terminal
	clients               []*client.Client
	connMutex             sync.Mutex
	mainStream            = stream.New()
	ptmx                  *os.File
	wsStreamEnabled       bool   // Websocket stream enabled
	GitTag                string = "0.0.0v"
	sizeWidth, sizeHeight int
	sc                    *session.Control
)

func writeAllWS() {
	msg := make([]byte, constants.BufferSize)
	for {
		n, err := mainStream.Read(msg)
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Printf("error reading from byte stream: %s\r\n", err)

			removeAllConnections()
			os.Exit(1)
		}

		// only write to websocket and mterm buffer if wsStreamEnabled is true
		if !wsStreamEnabled {
			continue
		}

		mt.Write(msg[:n]) // write to mterm buffer

		connMutex.Lock()
		for _, c := range clients {
			cn, err := c.Send(
				constants.MSG,
				msg[:n]) // TODO: check if cn < n and if so, write the rest of the buffer
			_ = cn // TODO: remove this line
			if err != nil {
				log.Printf("error writing to websocket: %s\r\n", err)
				removeConnection(c)
			}
		}
		connMutex.Unlock()
	}
}

func (o termIO) Write(p []byte) (n int, err error) {
	// write to stdout
	n, err = os.Stdout.Write(p)
	if err != nil {
		log.Printf("error writing to stdout: %s\r\n", err)
		return
	}

	// write to websocket
	mainStream.Write(p)
	return
}

func sendCommandToAll(command byte, params []byte) {
	connMutex.Lock()
	for _, c := range clients {
		cn, err := c.Send(command, params)
		_ = cn
		if err != nil {
			log.Printf("error writing to websocket: %s\r\n", err)
			removeConnection(c)
		}
	}
	connMutex.Unlock()
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
				var err error
				sizeWidth, sizeHeight, err = term.GetSize(
					int(os.Stdin.Fd()))
				if err != nil {
					log.Fatalf("error getting size: %s\r\n", err)
				}

				mt.Resize(sizeHeight, sizeWidth)

				mainStream.Write([]byte(fmt.Sprintf("\033[8;%d;%dt",
					sizeHeight, sizeWidth)))

				if wsStreamEnabled {
					sendCommandToAll(constants.RESIZE,
						[]byte(fmt.Sprintf("%d:%d",
							sizeHeight, sizeWidth)))
				}

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
	for _, c := range clients {
		err := c.Close()
		if err != nil {
			log.Printf("error closing websocket: %s\r\n", err)
		}
	}
	clients = nil
	connMutex.Unlock()
}

func removeConnection(c *client.Client) {
	connMutex.Lock()
	for i, client := range clients {
		if client == c {
			client.Close()
			clients = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	connMutex.Unlock()
}

func readMessages(client *client.Client) {
	for {
		buffer := make([]byte, constants.BufferSize)
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

func mainHandler(w http.ResponseWriter, r *http.Request) {
	sid, sd, ok := sc.Get(r)
	if !ok {
		sid, sd = sc.Create()
	}

	// renew session
	sc.Save(w, sid, sd)

	///////////////////////////////////////////////

	http.FileServer(assets.FS).ServeHTTP(w, r)

}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	sid, sd, ok := sc.Get(r)
	if !ok {
		sid, sd = sc.Create()
	}

	// renew session
	sc.Save(w, sid, sd)

	////////////////////////////////////////////////

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)

		return
	}

	client := client.New(c)
	client.SessionID = sid

	motd := config.CFG.MOTD

	if motd == "" {
		// azul claro
		motd = "\033[1;36mcompterm\033[0m " +
			GitTag + "\r\nWelcome to compterm, please wait for the command to start...\r\n"
	}

	client.DirectSend(constants.MSG, []byte(motd))

	if wsStreamEnabled {
		// send current terminal size (resize the xtermjs terminal)
		client.DirectSend(constants.RESIZE,
			[]byte(fmt.Sprintf("%d:%d", sizeHeight, sizeWidth)))

		// set terminal size, clear screen and set cursor to 1,1
		client.DirectSend(constants.MSG, []byte(fmt.Sprintf("\033[8;%d;%dt\033[2J\033[0;0H",
			sizeHeight, sizeWidth)))

		// get screen as ansi from mterm buffer
		msg := mt.GetScreenAsAnsi()

		// send screen to xtermjs terminal
		client.DirectSend(constants.MSG, []byte(msg))

		// set cursor position to the current position
		line, col := mt.CursorPos()
		client.DirectSend(constants.MSG, []byte(fmt.Sprintf("\033[%d;%dH", line+1, col+1)))
	}

	connMutex.Lock()
	clients = append(clients, client)
	connMutex.Unlock()

	go client.WriteLoop()
	// go readMessages(client)
}

func serveHTTP() {
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler)
	//mux.Handle("/", http.FileServer(assets.FS))
	mux.HandleFunc("/", mainHandler)

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

func apiHandler(w http.ResponseWriter, r *http.Request) {
	_, err := prelude(w, r, []string{http.MethodGet}, true)
	if err != nil {
		return
	}

	parameters := getParameters("/api/action/", r)

	if len(parameters) < 1 {
		log.Printf("invalid path")
		errorBadRequest(w)
		return
	}

	cmd := parameters[0]
	switch cmd {
	case "enable-ws-stream":
		// curl -X GET http://localhost:2201/api/action/enable-ws-stream

		wsStreamEnabled = true

		sendCommandToAll(
			constants.MSG,
			[]byte(fmt.Sprintf("\033[8;%d;%dt\033[2J\033[0;0H", sizeHeight, sizeWidth)))
		sendCommandToAll(
			constants.RESIZE,
			[]byte(fmt.Sprintf("%d:%d", sizeHeight, sizeWidth)))
	case "disable-ws-stream":
		// curl -X GET http://localhost:2201/api/action/disable-ws-stream

		wsStreamEnabled = false
	case "get-version":
		// curl -X GET http://localhost:2201/api/action/get-version

		_, _ = w.Write([]byte(GitTag))
	default:
		errorBadRequest(w)
		return
	}
	_, _ = w.Write([]byte("{status: \"ok\"}\n"))
}

func serveAPI() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/action/", apiHandler)

	s := &http.Server{
		Handler:        mux,
		Addr:           config.CFG.APIListen,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Listening API on port %v\n", config.CFG.APIListen)
	log.Fatal(s.ListenAndServe())
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	logFile, _ := os.Create("compterm.log")
	log.SetOutput(logFile)

	log.Printf("compterm version %s\n", GitTag)
	log.Printf("pid: %d\r\n", os.Getpid())

	err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %s\n", err)
	}

	const cookieName = "compterm"
	sc = session.New(cookieName)

	mt = mterm.New(24, 80)

	go writeAllWS()
	go runCmd()
	go serveAPI()

	runtime.Gosched()

	serveHTTP()
}
