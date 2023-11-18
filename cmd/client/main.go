package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"nhooyr.io/websocket"
	//"nhooyr.io/websocket/wsjson"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c, _, err := websocket.Dial(ctx, "ws://localhost:8080/ws", nil)
	if err != nil {
		fmt.Println(err)
	}
	defer c.CloseNow()

	/*
		err = wsjson.Write(ctx, c, "hi")
		if err != nil {
			// ...
		}
	*/

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			fmt.Println(err)
			break
		}
		os.Stdout.Write(data)
	}

	c.Close(websocket.StatusNormalClosure, "")
}
