package protocol

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/crgimenes/compterm/constants"
)

func Test_checksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint32
	}{
		{"empty", []byte{}, 2166136261},
		{"a", []byte{'a'}, 3826002220},
		{"ab", []byte{'a', 'b'}, 1294271946},
		{"abc", []byte{'a', 'b', 'c'}, 440920331},
		{"abcd", []byte{'a', 'b', 'c', 'd'}, 3459545533},
		{"abcde", []byte{'a', 'b', 'c', 'd', 'e'}, 1956368136},
		{"óüçã", []byte("óüçã"), 3909595796},
		{"⠁", []byte("⠁"), 943521466},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checksum(tt.data); got != tt.want {
				t.Errorf("checksum(%q) = %v, want %v", string(tt.data), got, tt.want)
			}
		})
	}
}

func TestEncodeDecode(t *testing.T) {
	in := []byte("hello")
	out := make([]byte, MaxPackageSize)

	nout, err := Encode(out, in, 0x01)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, constants.BufferSize)
	cmd, n, err := Decode(data, out[:nout])
	if err != nil {
		t.Fatal(err)
	}
	if cmd != 0x01 {
		t.Errorf("cmd = %v, want 0x01", cmd)
	}
	if string(data[:n]) != string(in) {
		t.Errorf("data = %q, want %q", string(data[:n]), string(in))
	}

	// tamper the payload: the checksum must no longer match
	out[5] = 0x00
	_, _, err = Decode(data, out[:nout])
	if err != ErrInvalidChecksum {
		t.Errorf("err = %v, want %v", err, ErrInvalidChecksum)
	}

	// a truncated frame is too short
	_, _, err = Decode(data, out[:nout-1])
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// a payload larger than the frame can hold
	_, err = Encode(out, make([]byte, MaxPackageSize+1), 0x01)
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// boundary payload sizes that must encode fine
	for _, size := range []int{0, 1, constants.BufferSize - 1, constants.BufferSize} {
		if _, err := Encode(out, make([]byte, size), 0x01); err != nil {
			t.Errorf("Encode(%d bytes) err = %v, want nil", size, err)
		}
	}

	// a source buffer shorter than the header
	for _, size := range []int{0, 1} {
		if _, _, err := Decode(data, make([]byte, size)); err != ErrInvalidSize {
			t.Errorf("Decode(%d bytes) err = %v, want %v", size, err, ErrInvalidSize)
		}
	}

	// a frame whose declared length is impossibly large
	_, err = Encode(out, []byte(randomPayload(100)), 0x01)
	if err != nil {
		t.Fatal(err)
	}
	out[1], out[2], out[3], out[4] = 0xFF, 0xFF, 0xFF, 0xFF
	_, _, err = Decode(data, out)
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}
}

func randomPayload(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = rune(rand.Intn(256))
	}
	return string(b)
}

func TestEncodeDecodeLoop(t *testing.T) {
	data := make([]byte, constants.BufferSize)
	for i := range 1000 {
		in := []byte(randomPayload(rand.Intn(10 + i)))
		out := make([]byte, MaxPackageSize)

		n, err := Encode(out, in, 0x01)
		if err != nil {
			t.Fatal(err)
		}

		cmd, n, err := Decode(data, out[:n])
		if err != nil {
			t.Fatal(err)
		}
		if cmd != 0x01 {
			t.Errorf("cmd = %v, want 0x01", cmd)
		}
		if string(data[:n]) != string(in) {
			t.Errorf("data = %q, want %q", string(data[:n]), string(in))
		}
	}
}

func ExampleEncode() {
	out := make([]byte, MaxPackageSize)
	n, err := Encode(out, []byte("hello"), 0x01)
	if err != nil {
		panic(err)
	}
	fmt.Printf("frame: %02X\n", out[:n])
	// Output:
	// frame: 010000000568656C6C6FEFE3E091
}

func ExampleDecode() {
	out := make([]byte, MaxPackageSize)
	n, err := Encode(out, []byte("hello"), 0x01)
	if err != nil {
		panic(err)
	}

	data := make([]byte, constants.BufferSize)
	cmd, n, err := Decode(data, out[:n])
	if err != nil {
		panic(err)
	}

	fmt.Printf("cmd: %02X\n", cmd)
	fmt.Printf("data: %v\n", string(data[:n]))
	// Output:
	// cmd: 01
	// data: hello
}
