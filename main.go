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
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/crgimenes/compterm/assets"
	"github.com/crgimenes/compterm/config"
	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/luaengine"
	"github.com/crgimenes/compterm/prelude"
	"github.com/crgimenes/compterm/protocol"
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
	GitTag           string = "0.0.0v"
	sc               *session.Control
	mx               sync.Mutex
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

	c.Env = os.Environ()
	c.Env = append(c.Env, fmt.Sprintf("COMPTERM=%d", os.Getpid()))

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
	//_, _ = io.Copy(defaultScreen, ptmx)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err == io.EOF {
					return
				}
				log.Fatalf("error reading from pty: %s\r\n", err)
			}
			if n > 0 {
				defaultScreen.Write(buf[:n])
				os.Stdout.Write(buf[:n])
			}
		}
	}()

	// Wait for the command to finish.
	err = c.Wait()
	if err != nil {
		log.Printf("error waiting for command: %s\r\n", err)
		// TODO: send error to screen and close clients
	}
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

///////////////////////////////////////////////

type dummyProvider struct {
	Screen *screen.Screen
}

func (d dummyProvider) Write(p []byte) (n int, err error) {
	// input from webbrowser / websocket
	d.Screen.Write([]byte(fmt.Sprintf("- %s\r\n", string(p))))
	return len(p), nil
}

func (d dummyProvider) LoopWrite() {
	// output to webbrowser / websocket
	for {
		time.Sleep(1 * time.Second)
		d.Screen.Write([]byte(fmt.Sprintf("dummyProvider: %s\r\n", time.Now().String())))
	}
}

///////////////////////////////////////////////

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

	///////////////////////////////////////////////
	defaultScreen.AttachClient(client, false)

	/*
			// TODO: use the manager to create a new screen
			// TODO: enable change screen

		   // test dummyProvider
		   s := screen.New(24, 80)

		   d := dummyProvider{Screen: s}
		   s.Input = d

		   go d.LoopWrite()
		   s.AttachClient(client, true)
	*/
}

////////////////////////////////////////

type wsWriter struct {
	c *websocket.Conn
}

func (w wsWriter) Write(p []byte) (n int, err error) {
	// input from webbrowser / websocket
	err = w.c.Write(context.Background(), websocket.MessageText, p)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return len(p), nil
}

func (w wsWriter) LoopWrite() {
	buf := make([]byte, constants.BufferSize)
	for {
		_, data, err := w.c.Read(context.Background())
		if err != nil {
			log.Println(err)
			return
		}

		cmd, n, _, err := protocol.Decode(buf, data)
		if err != nil {
			log.Println(err)
			return
		}

		err = defaultScreen.Send(cmd, buf[:n])
		if err != nil {
			log.Println(err)
			return
		}
	}
}

////////////////////////////////////////

func wsproxyHandler(w http.ResponseWriter, r *http.Request) {
	if !config.CFG.ProxyMode {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("proxy mode is disabled"))
		return
	}

	sid, sd, ok := sc.Get(r)
	if !ok {
		sid, sd = sc.Create()
	}

	// renew session
	sc.Save(w, sid, sd)

	////////////////////////////////////////////////
	// TODO: verify client credentials (api-key to send data to websocket)

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)

		return
	}

	// TODO: permit multiple clients send data to websocket

	ws := wsWriter{c: c}
	defaultScreen.Input = ws
	ws.LoopWrite()
}

func serveHTTP() {
	mux := http.NewServeMux()

	mux.HandleFunc("/wsproxy", wsproxyHandler)
	mux.HandleFunc("/ws", wsHandler)
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
	_, err := prelude.Prepare(w, r, []string{http.MethodGet}, true)
	if err != nil {
		return
	}

	parameters := prelude.GetParameters("/api/action/", r)

	if len(parameters) < 1 {
		log.Printf("invalid path")
		prelude.RErrorBadRequest(w)
		return
	}

	cmd := parameters[0]
	switch cmd {
	case "enable-ws-stream":
		// curl -X GET http://localhost:2201/api/action/enable-ws-stream

		rows, columns := defaultScreen.Size()
		s := defaultScreen

		s.Send(
			constants.MSG,
			[]byte(fmt.Sprintf("\033[8;%d;%dt\033[2J\033[0;0H", rows, columns)))
		s.Send(
			constants.RESIZE,
			[]byte(fmt.Sprintf("%d:%d", rows, columns)))
	case "disable-ws-stream":
		// curl -X GET http://localhost:2201/api/action/disable-ws-stream

	case "get-version":
		// curl -X GET http://localhost:2201/api/action/get-version

		_, _ = w.Write([]byte(GitTag))
	default:
		prelude.RErrorBadRequest(w)
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
		comptermPID := os.Getenv("COMPTERM")
		if comptermPID != "" {
			fmt.Printf("There is already a compterm running, pid: %s\n", comptermPID)
			os.Exit(1)
		}
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

	const cookieName = "compterm"
	sc = session.New(cookieName)

	go serveAPI()
	go serveHTTP()

	runtime.Gosched()

	if !config.CFG.ProxyMode {
		runCmd()
		return
	}

	<-make(chan struct{})

}
