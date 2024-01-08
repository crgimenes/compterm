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
	"github.com/crgimenes/compterm/config"
	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/luaengine"
	"github.com/crgimenes/compterm/screen"
	"github.com/crgimenes/compterm/session"

	"github.com/creack/pty"
	"golang.org/x/term"
	"nhooyr.io/websocket"
)

var (
	screenManager    = screen.NewManager()
	_, defaultScreen = screenManager.GetScreenByID(0)
	ptmx             *os.File
	connMutex        sync.Mutex
	GitTag           string = "0.0.0v"
	sc               *session.Control
	mx               sync.Mutex
)

func sendToAll(command byte, params []byte) {
	// - Mover este método para screen, e usar a lista de clientes
	connMutex.Lock()
	for _, c := range defaultScreen.Clients {
		err := c.Send(command, params)
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

	pty.InheritSize(os.Stdin, ptmx)

	defaultScreen.Input = ptmx // resive input from from user

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
	for _, c := range defaultScreen.Clients {
		c.Close()
	}
	defaultScreen.Clients = nil
	connMutex.Unlock()
}

func removeConnection(c *screen.Client) {
	connMutex.Lock()
	for i, client := range defaultScreen.Clients {
		if client == c {
			client.Close()
			defaultScreen.Clients = append(
				defaultScreen.Clients[:i],
				defaultScreen.Clients[i+1:]...)
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

	client := screen.NewClient(c)
	client.SessionID = sid

	defaultScreen.AttachClient(client, true)

	///////////////////////////////////////////////

	// TODO: move this to screen and send evry time a client is attached to the screen
	motd := config.CFG.MOTD
	if motd == "" {
		motd = "\033[1;36mcompterm\033[0m " +
			GitTag + "\r\nWelcome to compterm, please wait...\r\n"
	}
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

	log.Printf("Listening on %v\n", config.CFG.Listen)
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

	log.Printf("Listening API on %v\n", config.CFG.APIListen)
	log.Fatal(s.ListenAndServe())
}

func updateTerminalSize() {
	// Update window size.
	mx.Lock()
	_ = pty.InheritSize(os.Stdin, ptmx)
	mx.Unlock()
	columns, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("error getting size: %s\r\n", err)
	}

	defaultScreen.Resize(rows, columns)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

	err := config.Load()
	if err != nil {
		log.Fatalf("error loading config: %s\n", err)
	}

	/////////////////////////////////////////////////

	// read file init.lua from assets

	luaInit := config.CFG.Path + "/" + config.CFG.InitFile

	_, err = os.Stat(luaInit)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("error reading %q : %s\r\n", luaInit, err)
		return
	}

	if os.IsNotExist(err) && config.CFG.InitFile == "init.lua" {
		f, err := os.Create(luaInit)
		if err != nil {
			return
		}
		finit, err := assets.FS.Open("init.lua")
		if err != nil {
			log.Printf("error reading init.lua from assets: %s\r\n", err)
			return
		}
		_, err = io.Copy(f, finit)
		if err != nil {
			log.Printf("error writing init.lua: %s\r\n", err)
			return
		}
		f.Close()
	}

	err = luaengine.Startup(luaInit)
	if err != nil {
		return
	}

	/////////////////////////////////////////////////

	// verify if there is a pid file
	if !config.CFG.IgnorePID {
		pidFile := config.CFG.Path + "/compterm.pid"
		_, err = os.Stat(pidFile)
		if err == nil {
			b, err := os.ReadFile(pidFile)
			if err != nil {
				log.Fatalf("error reading pid file: %s\n", err)
			}
			fmt.Printf("There is already a compterm running, pid: %s pid file: %s\n", string(b), pidFile)
			os.Exit(1)
		}

		// create pid file
		err = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0640)
		if err != nil {
			log.Fatalf("error writing pid file: %s\n", err)
		}

		defer os.Remove(pidFile)
	}

	/////////////////////////////////////////////////
	logFile := config.CFG.Path + "/compterm.log"
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

	//go writeAllWS()
	go serveAPI()
	go serveHTTP()
	runtime.Gosched()

	runCmd()

}
