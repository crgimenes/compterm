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
	"github.com/crgimenes/compterm/screen"
	"github.com/crgimenes/compterm/session"

	"github.com/creack/pty"
	"golang.org/x/term"
	"nhooyr.io/websocket"
)

var (
	screenManager    = screen.NewManager()
	_, defaultScreen = screenManager.GetScreenByID(0)
	clients          []*client.Client
	connMutex        sync.Mutex
	ptmx             *os.File
	GitTag           string = "0.0.0v"
	sc               *session.Control
)

func writeAllWS() {
	msg := make([]byte, constants.BufferSize)
	for {
		n, err := defaultScreen.Stream.Read(msg)
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Printf("error reading from byte stream: %s\r\n", err)

			removeAllConnections()
			os.Exit(1)
		}

		connMutex.Lock()
		for _, c := range clients {
			cn, err := c.Write(msg[:n])
			for cn < n && err == nil {
				msg = msg[cn:]
				n -= cn
				cn, err = c.Write(msg[:n])
			}
			if err != nil {
				log.Printf("error writing to websocket: %s\r\n", err)
				removeConnection(c)
			}
		}
		connMutex.Unlock()
	}
}

func sendToAll(command byte, params []byte) {
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
	// clean screen
	//os.Stdout.WriteString("\033[2J\033[0;0H")

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

	pty.InheritSize(os.Stdin, ptmx)

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(defaultScreen, ptmx)

	// Wait for the command to finish.
	err = c.Wait()
	if err != nil {
		log.Fatalf("error waiting for command: %s\r\n", err)
	}

	// Close the websocket connections
	removeAllConnections()
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

	connMutex.Lock()
	clients = append(clients, client)
	connMutex.Unlock()

	go client.WriteLoop()
	//go client.ReadLoop(ptmx)

	runtime.Gosched()
	// TODO: make sure the client is ready to receive messages

	///////////////////////////////////////////////

	if motd == "" {
		// azul claro
		motd = "\033[1;36mcompterm\033[0m " +
			GitTag + "\r\nWelcome to compterm, please wait for the command to start...\r\n"
	}

	client.Send(constants.MSG, []byte(motd))

	// get terminal size
	rows, columns := defaultScreen.Size()
	crows, ccolumns := defaultScreen.CursorPos()

	// set terminal size, clear screen and set cursor to 0,0
	client.Send(constants.MSG, []byte(fmt.Sprintf("\033[8;%d;%dt\033[0;0H",
		rows, columns)))

	// send current terminal size (resize the xtermjs terminal)
	client.Send(constants.RESIZE,
		[]byte(fmt.Sprintf("%d:%d", rows, columns)))

	// get screen as ansi from mterm buffer
	msg := defaultScreen.GetScreenAsANSI()

	// send screen to xtermjs terminal
	client.Send(constants.MSG, []byte(msg))

	// set cursor position to the current position
	client.Send(constants.MSG, []byte(fmt.Sprintf("\033[%d;%dH", crows+1, ccolumns+1)))
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

		rows, columns := defaultScreen.Size()

		sendToAll(
			constants.MSG,
			[]byte(fmt.Sprintf("\033[8;%d;%dt\033[2J\033[0;0H", rows, columns)))
		sendToAll(
			constants.RESIZE,
			[]byte(fmt.Sprintf("%d:%d", rows, columns)))
	case "disable-ws-stream":
		// curl -X GET http://localhost:2201/api/action/disable-ws-stream

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

func updateTerminalSize() {
	// Update window size.
	_ = pty.InheritSize(os.Stdin, ptmx)
	columns, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("error getting size: %s\r\n", err)
	}

	defaultScreen.Resize(rows, columns)

	sendToAll(constants.RESIZE,
		[]byte(fmt.Sprintf("%d:%d", rows, columns)))
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

	err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %s\n", err)
	}

	// verify if there is a pid file
	pidFile := config.CFG.CFGPath + "/compterm.pid"
	_, err = os.Stat(pidFile)
	if err == nil {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			log.Fatalf("error reading pid file: %s\n", err)
		}
		fmt.Printf("There is already a compterm running, pid: %s\n", b)
		os.Exit(1)
	}

	// create pid file
	err = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0640)
	if err != nil {
		log.Fatalf("error writing pid file: %s\n", err)
	}
	defer os.Remove(pidFile)

	logFile := config.CFG.CFGPath + "/compterm.log"
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		log.Fatalf("error opening log file: %s %s\n", logFile, err)
	}
	log.SetOutput(f)

	log.Printf("compterm version %s\n", GitTag)
	log.Printf("pid: %d\n", os.Getpid())

	// Handle signals
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH) //, syscall.SIGTERM, os.Interrupt)
	go func() {
		for caux := range ch {
			switch caux {
			case syscall.SIGWINCH:
				updateTerminalSize()
				//case syscall.SIGTERM, os.Interrupt:
				//	removeAllConnections()
				//	os.Exit(0)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	updateTerminalSize()

	const cookieName = "compterm"
	sc = session.New(cookieName)

	go writeAllWS()
	go serveAPI()
	go serveHTTP()
	runtime.Gosched()

	runCmd()

}
