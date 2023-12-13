package protocol

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/crgimenes/compterm/constants"
)

func Test_checksum(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name string
		args args
		want uint32
	}{
		{"empty", args{[]byte{}}, 2166136261},
		{"a", args{[]byte{'a'}}, 3826002220},
		{"ab", args{[]byte{'a', 'b'}}, 1294271946},
		{"abc", args{[]byte{'a', 'b', 'c'}}, 440920331},
		{"abcd", args{[]byte{'a', 'b', 'c', 'd'}}, 3459545533},
		{"abcde", args{[]byte{'a', 'b', 'c', 'd', 'e'}}, 1956368136},
		{"óüçã", args{[]byte("óüçã")}, 3909595796},
		{"⠁", args{[]byte("\u2801")}, 943521466},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checksum(tt.args.data); got != tt.want {
				t.Errorf("checksum(%v) = %v, want %v",
					string(tt.args.data), got, tt.want)
			}
		})
	}
}

func TestEncodeDecode(t *testing.T) {
	in := []byte("hello")
	out := make([]byte, MaxPackageSize)

	nout, err := Encode(out, in, 0x01, 0x01)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, constants.BufferSize)
	cmd, n, _, err := Decode(data, out[:nout])
	if err != nil {
		t.Fatal(err)
	}

	t.Log("data:", string(data[:n]))

	if cmd != 0x01 {
		t.Errorf("cmd = %v, want 0x01", cmd)
	}

	if string(data[:n]) != string(in) {
		t.Errorf("data = %q, want %q", string(data[:n]), string(in))
	}

	// test invalid checksum
	out[7] = 0x00
	_, _, _, err = Decode(data, out[:nout])
	if err != ErrInvalidChecksum {
		t.Errorf("err = %v, want %v", err, ErrInvalidChecksum)
	}

	// test invalid size
	_, _, _, err = Decode(data, out[:nout-1])
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, constants.BufferSize+1), 0x01, 0x01)
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, 0), 0x01, 0x01)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, constants.BufferSize), 0x01, 0x01)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, constants.BufferSize-1), 0x01, 0x01)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, 1), 0x01, 0x01)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, 0), 0x01, 0x01)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, make([]byte, MaxPackageSize+1), 0x01, 0x01)
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// decode invalid size
	_, _, _, err = Decode(data, make([]byte, 1))
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// decode invalid size
	_, _, _, err = Decode(data, make([]byte, 0))
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	data = []byte(randonPayload(100))
	_, err = Encode(out, data, 0x01, 0x01)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// change size in data to FFFFFFFF
	out[3] = 0xFF
	out[4] = 0xFF
	out[5] = 0xFF
	out[6] = 0xFF
	_, _, _, err = Decode(data, out)
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}
}

func randonPayload(n int) string {
	ascii := make([]rune, 256)
	for i := range ascii {
		ascii[i] = rune(i)
	}

	b := make([]rune, n)
	for i := range b {
		b[i] = ascii[rand.Intn(len(ascii))]
	}
	return string(b)
}

func TestEncodeDecodeLoop(t *testing.T) {
	data := make([]byte, constants.BufferSize)
	for i := 0; i < 1000; i++ {
		in := []byte(randonPayload(rand.Intn(10 + i)))
		out := make([]byte, MaxPackageSize)

		n, err := Encode(out, in, 0x01, uint16(i))
		if err != nil {
			t.Fatal(err)
		}

		cmd, n, counter, err := Decode(data, out[:n])
		if err != nil {
			t.Fatal(err)
		}

		if cmd != 0x01 {
			t.Errorf("cmd = %v, want 0x01", cmd)
		}

		if string(data[:n]) != string(in) {
			t.Errorf("data = %v, want %v", string(data), string(in))
		}

		// test counter
		if int(counter) != i {
			t.Errorf("counter = %v, want %v", counter, i)
		}

		// test checksum
		if checksum(in) != checksum(data[:n]) {
			t.Errorf("checksum = %v, want %v", checksum(in), checksum(data))
		}

		// test size
		if len(in) != len(data[:n]) {
			t.Errorf("size = %v, want %v", len(data), len(in))
		}
	}
}

// Testable example

func ExampleEncode() {
	data := []byte("hello")
	out := make([]byte, MaxPackageSize)

	n, err := Encode(out, data, 0x01, 0x01)
	if err != nil {
		panic(err)
	}

	fmt.Printf("data: %02X\n", string(out[:n]))
	// Output:
	// data: 0100010000000568656C6C6F7CE86368
}

func ExampleDecode() {
	data := []byte("hello")
	out := make([]byte, MaxPackageSize)

	n, err := Encode(out, data, 0x01, 0x01)
	if err != nil {
		panic(err)
	}

	cmd, n, _, err := Decode(data, out[:n])
	if err != nil {
		panic(err)
	}

	fmt.Printf("cmd: %02X\n", cmd)
	fmt.Printf("data: %v\n", string(data[:n]))
	// Output:
	// cmd: 01
	// data: hello
}
