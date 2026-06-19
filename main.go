package main

import (
	"crypto/subtle"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/crgimenes/compterm/assets"
	"github.com/crgimenes/compterm/config"
	"github.com/crgimenes/compterm/screen"
	"github.com/crgimenes/compterm/session"

	"github.com/coder/websocket"
	"github.com/creack/pty"
	"golang.org/x/term"
)

const cookieName = "compterm"

var (
	defaultScreen = screen.New(25, 80) // rows, columns
	ptmx          *os.File
	GitTag        string           = "0.0.0v"
	sc            *session.Control = session.New(cookieName)
	mx            sync.Mutex
)

func runCmd() {
	var err error

	cmd := strings.Split(config.CFG.Command, " ")
	c := exec.Command(cmd[0], cmd[1:]...) // #nosec G204 -- operator-provided command

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

	_ = pty.InheritSize(os.Stdin, ptmx)

	// Copy stdin to the pty, and the pty to both stdout and the broadcast.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()

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
				_, _ = defaultScreen.Write(buf[:n])
				_, _ = os.Stdout.Write(buf[:n])
			}
		}
	}()

	// Wait for the command to finish.
	err = c.Wait()
	if err != nil {
		log.Printf("error waiting for command: %s\r\n", err)
	}
}

// authorize reports whether a connection is allowed. An empty requiredToken
// disables authentication entirely.
func authorize(requiredToken, providedToken string, sessionAuthed bool) bool {
	if requiredToken == "" {
		return true
	}
	if sessionAuthed {
		return true
	}
	return providedToken != "" &&
		subtle.ConstantTimeCompare([]byte(providedToken), []byte(requiredToken)) == 1
}

// tokenFromRequest extracts an access token from the query string or header,
// supporting shared links and non-browser clients.
func tokenFromRequest(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return r.Header.Get("X-Auth-Token")
}

func isAuthorized(r *http.Request, sd *session.SessionData) bool {
	return authorize(config.CFG.AuthToken, tokenFromRequest(r), sd != nil && sd.Authenticated)
}

// loginPageFmt is a self-contained login page; %s is an optional error block.
const loginPageFmt = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>compterm — login</title>
<style>
  html,body{height:100%%;margin:0;background:#000;color:#d4d4d4;font-family:monospace}
  form{position:absolute;top:50%%;left:50%%;transform:translate(-50%%,-50%%);
    display:flex;flex-direction:column;gap:.75rem;min-width:16rem}
  h1{margin:0 0 .5rem;font-size:1.25rem;text-align:center}
  input,button{padding:.6rem;font:inherit;border:1px solid #444;background:#111;
    color:#d4d4d4;border-radius:4px}
  button{cursor:pointer;background:#1b3a1b;border-color:#2d5a2d}
  .error{margin:0;color:#ff6d67;text-align:center}
</style>
</head>
<body>
<form method="post" action="login">
<h1>compterm</h1>
%s<input type="password" name="token" placeholder="Access token" autofocus
  autocomplete="current-password">
<button type="submit">Enter</button>
</form>
</body>
</html>
`

func serveLogin(w http.ResponseWriter, status int, errMsg string) {
	errBlock := ""
	if errMsg != "" {
		errBlock = `<p class="error">` + html.EscapeString(errMsg) + "</p>\n"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, loginPageFmt, errBlock)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// nothing to log into when authentication is disabled
	if config.CFG.AuthToken == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	sid, sd, ok := sc.Get(r)
	if !ok {
		sid, sd = sc.Create()
	}

	if r.Method != http.MethodPost {
		sc.Save(w, r, sid, sd)
		serveLogin(w, http.StatusOK, "")
		return
	}

	if authorize(config.CFG.AuthToken, r.PostFormValue("token"), false) {
		sd.Authenticated = true
		sc.Save(w, r, sid, sd)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	sc.Save(w, r, sid, sd)
	serveLogin(w, http.StatusUnauthorized, "Invalid token.")
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	sid, sd, ok := sc.Get(r)
	if !ok {
		sid, sd = sc.Create()
	}

	// a valid token in the URL (a shared link) authenticates the session
	if config.CFG.AuthToken != "" && !sd.Authenticated && isAuthorized(r, sd) {
		sd.Authenticated = true
	}

	sc.Save(w, r, sid, sd)

	if config.CFG.AuthToken != "" && !sd.Authenticated {
		serveLogin(w, http.StatusOK, "")
		return
	}

	http.FileServer(assets.FS).ServeHTTP(w, r)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	sid, sd, ok := sc.Get(r)
	if !ok {
		sid, sd = sc.Create()
	}

	sc.Save(w, r, sid, sd)

	if !isAuthorized(r, sd) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client := screen.NewClient(c)
	client.SessionID = sid
	defaultScreen.AttachClient(client)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/", mainHandler)
	return mux
}

func serveHTTP() {
	s := &http.Server{
		Handler:        newMux(),
		Addr:           config.CFG.Listen,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Listening on %v\n", config.CFG.Listen)
	log.Fatal(s.ListenAndServe())
}

func updateTerminalSize() {
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

	// refuse to nest inside another compterm session
	if !config.CFG.IgnorePID {
		if pid := os.Getenv("COMPTERM"); pid != "" {
			fmt.Printf("There is already a compterm running, pid: %s\n", pid)
			os.Exit(1)
		}
	}

	logFile := config.CFG.Path + "/compterm.log"
	f, err := os.OpenFile(filepath.Clean(logFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("error opening log file: %s %s\n", logFile, err)
	}
	log.SetOutput(f)

	log.Printf("compterm version %s\n", GitTag)
	log.Printf("pid: %d\n", os.Getpid())

	// Handle terminal resize.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			updateTerminalSize()
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	updateTerminalSize()

	// expire idle sessions periodically
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sc.RemoveExpired()
		}
	}()

	go serveHTTP()

	runCmd()
}
