package main

import (
	"bytes"
	"testing"

	"github.com/crgimenes/compterm/constants"
	"github.com/crgimenes/compterm/protocol"
)

func TestRenderFrames(t *testing.T) {
	enc := make([]byte, constants.BufferSize)
	frame := func(cmd byte, payload string) []byte {
		n, err := protocol.Encode(enc, []byte(payload), cmd, 0)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return bytes.Clone(enc[:n])
	}

	// One websocket message carrying MSG, RESIZE (skipped), MSG.
	var msg []byte
	msg = append(msg, frame(constants.MSG, "hello ")...)
	msg = append(msg, frame(constants.RESIZE, "25:80")...)
	msg = append(msg, frame(constants.MSG, "world")...)

	var out bytes.Buffer
	renderFrames(make([]byte, constants.BufferSize), msg, &out)

	if got := out.String(); got != "hello world" {
		t.Fatalf("renderFrames output = %q, want %q", got, "hello world")
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name, raw, token, want string
	}{
		{"no token", "ws://localhost:2200/ws", "", "ws://localhost:2200/ws"},
		{"with token", "ws://localhost:2200/ws", "s3cr3t", "ws://localhost:2200/ws?token=s3cr3t"},
		{"wss with spaced token", "wss://example.com/term/ws", "ab cd", "wss://example.com/term/ws?token=ab+cd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildURL(tt.raw, tt.token)
			if err != nil {
				t.Fatalf("buildURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildURL(%q, %q) = %q, want %q", tt.raw, tt.token, got, tt.want)
			}
		})
	}
}
