package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/kr/pty"
	"golang.org/x/term"
)

type out struct {
}

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

func (o out) Write(p []byte) (n int, err error) {

	// append to out.txt file
	//appendToOutFile(p)

	n, err = os.Stdout.Write(p)

	return
}

var o = out{}

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

func main() {
	go runCmd()

	mux := http.NewServeMux()

	//mux.HandleFunc("/", homeHandler)

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
