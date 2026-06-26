// Command client is a terminal (TUI) viewer for a compterm session. It connects
// to the broadcast websocket and renders the shared terminal in place, since the
// stream is already a complete ANSI feed (a snapshot on connect, then live
// deltas). Press q or Ctrl-C to quit.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

func main() {
	wsURL := flag.String("url", "ws://localhost:2200/ws", "compterm websocket URL")
	token := flag.String("token", os.Getenv("COMPTERM_AUTH_TOKEN"), "access token, if the server requires one")
	flag.Parse()

	target, err := buildURL(*wsURL, *token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid url: %v\n", err)
		os.Exit(1)
	}

	cleanup := enterScreen()
	defer cleanup()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cleanup()
		os.Exit(0)
	}()

	// Reconnect until the user quits.
	for {
		err := stream(target)
		_, _ = fmt.Fprintf(os.Stdout, "\r\n\033[33mdisconnected: %v — reconnecting...\033[0m\r\n", err)
		time.Sleep(time.Second)
	}
}

// buildURL appends the access token to the websocket URL when provided.
func buildURL(rawURL, token string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if token != "" {
		q := u.Query()
		q.Set("token", token)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// enterScreen switches to the alternate screen in raw mode and returns a
// cleanup func (safe to call more than once) that restores the terminal.
func enterScreen() func() {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return func() {}
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error setting raw mode: %v\n", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString("\033[?1049h\033[?25l") // alternate screen + hide cursor

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_, _ = os.Stdout.WriteString("\033[?25h\033[?1049l") // show cursor + leave alt screen
			_ = term.Restore(fd, oldState)
		})
	}

	// Raw mode swallows Ctrl-C, so watch stdin for the quit keys ourselves.
	go func() {
		b := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(b)
			if err != nil {
				return
			}
			if n == 1 && (b[0] == 'q' || b[0] == 0x03) {
				cleanup()
				os.Exit(0)
			}
		}
	}()

	return cleanup
}

// stream renders the broadcast until the connection drops, returning the error.
func stream(wsURL string) error {
	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(-1)

	buf := make([]byte, constants.BufferSize)
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return err
		}
		renderFrames(buf, data, os.Stdout)
	}
}

// renderFrames decodes every protocol frame in a websocket message (a single
// message may carry several concatenated frames) and writes the payload of each
// MSG frame to out. RESIZE frames carry no displayable content — the resize
// escape is already part of the MSG stream — so they are skipped.
func renderFrames(buf, data []byte, out io.Writer) {
	for len(data) > 0 {
		cmd, n, err := protocol.Decode(buf, data)
		if err != nil {
			return
		}
		if cmd == constants.MSG {
			_, _ = out.Write(buf[:n])
		}
		advance := n + protocol.Overhead
		if advance > len(data) {
			return
		}
		data = data[advance:]
	}
}
