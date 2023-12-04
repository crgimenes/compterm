package protocol

import (
	"testing"
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
	out := make([]byte, MAX_PACKAGE_SIZE)

	n, err := Encode(out, 0x01, in)
	if err != nil {
		t.Fatal(err)
	}

	cmd, data, err := Decode(out[:n])
	if err != nil {
		t.Fatal(err)
	}

	t.Log("data:", string(data))

	if cmd != 0x01 {
		t.Errorf("cmd = %v, want 0x01", cmd)
	}

	if string(data) != string(in) {
		t.Errorf("data = %v, want %v", string(data), string(in))
	}

	// test invalid checksum
	out[5] = 0x00
	_, _, err = Decode(out[:n])
	if err != ErrInvalidChecksum {
		t.Errorf("err = %v, want %v", err, ErrInvalidChecksum)
	}

	// test invalid size
	_, _, err = Decode(out[:n-1])
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// test invalid size
	_, err = Encode(out, 0x01, make([]byte, MAX_DATA_SIZE+1))
	if err != ErrInvalidSize {
		t.Errorf("err = %v, want %v", err, ErrInvalidSize)
	}

	// test invalid size
	_, err = Encode(out, 0x01, make([]byte, 0))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, 0x01, make([]byte, MAX_DATA_SIZE))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, 0x01, make([]byte, MAX_DATA_SIZE-1))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, 0x01, make([]byte, 1))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}

	// test invalid size
	_, err = Encode(out, 0x01, make([]byte, 0))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}
