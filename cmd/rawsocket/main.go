package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"

	"nhooyr.io/websocket"
)

// nc localhost 2211
// echo "Hello World" | nc localhost 2211

var (
	ErrNormalWSClosure = errors.New("websocket normal closure")

	wsConn    *websocket.Conn
	clients   = []net.Conn{}
	mx        = &sync.Mutex{}
	wsServer  string
	tcpServer string
)

func removeClient(c net.Conn) {
	mx.Lock()
	defer mx.Unlock()
	for i, v := range clients {
		if v == c {
			clients = append(clients[:i], clients[i+1:]...)
			break
		}
	}
}

func closeClients() {
	mx.Lock()
	defer mx.Unlock()
	for _, c := range clients {
		_ = c.Close()
	}
	clients = []net.Conn{}
}

func handleTCPClient(conn net.Conn) {
	data := make([]byte, constants.BufferSize)
	mx.Lock()
	clients = append(clients, conn)
	mx.Unlock()

	defer func() {
		err := conn.Close()
		if err != nil {
			log.Println(err)
		}
		removeClient(conn)
	}()

	for {
		n, err := conn.Read(data)
		if err != nil {
			log.Println(err)
			removeClient(conn)
			return
		}

		if wsConn == nil {
			continue
		}

		err = wsConn.Write(
			context.Background(),
			websocket.MessageBinary,
			data[:n])
		if err != nil {
			log.Println(err)
			removeClient(conn)
			return
		}
	}
}

func tcpSocketServer() error {
	ln, err := net.Listen("tcp", tcpServer)
	if err != nil {
		return err
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		go handleTCPClient(conn)
	}
}

func wsClient() error {
	var (
		err  error
		ctx  = context.Background()
		data = make([]byte, constants.BufferSize)
	)

	wsConn, _, err = websocket.Dial(ctx, wsServer, nil)
	if err != nil {
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			return ErrNormalWSClosure
		}
		return err
	}
	defer func() {
		if wsConn != nil {
			_ = wsConn.Close(websocket.StatusNormalClosure, "")
		}
		closeClients()
	}()

	log.Println("Connected to websocket server")

	for {
		mt, msg, err := wsConn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return ErrNormalWSClosure
			}
			return err
		}

		if mt != websocket.MessageBinary {
			log.Printf("unexpected message type: %d", mt)
			continue
		}

		cmd, n, _, err := protocol.Decode(data, msg)
		if err != nil {
			log.Println(err)
			continue
		}

		if cmd == constants.MSG {
			//os.Stdout.Write(data[:n])
			for _, c := range clients {
				_, err = c.Write(data[:n])
				if err != nil {
					log.Println(err)
					removeClient(c)
				}
			}
		}
	}
}

func main() {
	// pega parametros da linha de comando

	flag.StringVar(&wsServer, "ws", "", "websocket server address, example: ws://localhost:2200/ws")
	flag.StringVar(&tcpServer, "tcp", "0.0.0.0:2211", "tcp server address, example: 0.0.0.0:2211")

	flag.Parse()

	if wsServer == "" {
		log.Fatal("websocket server address is required")
	}

	// handle signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		closeClients()
		if wsConn != nil {
			wsConn.Close(websocket.StatusNormalClosure, "")
		}
		os.Exit(0)
	}()

	log.Printf("TCP Server: %s", tcpServer)
	go func() {
		err := tcpSocketServer()
		if err != nil {
			log.Println(err)
			sigs <- syscall.SIGINT
		}
	}()

	log.Printf("Connecting to websocket server: %s", wsServer)
	go func() {
		for {
			err := wsClient()
			if err != nil {
				log.Printf("websocket client error: %s", err)
			}

			log.Println("Reconnecting to websocket server in 5 seconds")
			<-time.After(5 * time.Second)
		}
	}()

	c := make(chan struct{})
	<-c

}
