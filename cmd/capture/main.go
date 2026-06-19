// Command capture opens a shell in a pty, mirrors it to your terminal so you
// can interact normally, and writes the raw terminal output to a file. Run a
// program (e.g. nvim) inside it to capture exactly what it emits, for analysis.
//
//	go run ./cmd/capture -o /tmp/nv.raw
package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func main() {
	out := flag.String("o", "capture.raw", "file to write the raw terminal output to")
	flag.Parse()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("creating %s: %v", *out, err)
	}
	defer func() { _ = f.Close() }()

	c := exec.Command(shell) // #nosec G204 G702 -- operator-controlled shell ($SHELL)
	ptmx, err := pty.Start(c)
	if err != nil {
		log.Fatalf("starting pty: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// keep the pty sized to the real terminal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("raw mode: %v", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// your keystrokes go to the shell
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()

	// the shell's output goes to both your terminal and the capture file
	_, _ = io.Copy(io.MultiWriter(os.Stdout, f), ptmx)

	log.Printf("captured raw output to %s", *out)
}
