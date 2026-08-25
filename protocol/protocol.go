// Protocol package implements a simple framing protocol for sending and
// receiving data.
//
// Frame format: A CCCC DDD...DDD FFFF
// Where:
// A: command byte
// C: payload length (32 bits, big endian)
// D: payload (array of bytes)
// F: checksum (FNV-1a, 32 bits, big endian)
//
// The checksum covers the command byte, payload length, and the data itself.
package protocol

import (
	"encoding/binary"
	"errors"
)

// Frame command bytes. Mirrored in assets/term.js; change both together.
const (
	MSG    = 0x1
	RESIZE = 0x2
)

// BufferSize is the largest payload a frame carries, and the read buffer size
// used throughout the project.
const BufferSize = 262144

// Overhead is the number of framing bytes around a payload:
// command (1) + length (4) + checksum (4).
const Overhead = 9

// MaxPackageSize is the largest a whole frame can be.
const MaxPackageSize = BufferSize + Overhead

var (
	ErrInvalidSize     = errors.New("invalid size")
	ErrInvalidChecksum = errors.New("invalid checksum")
)

// checksum returns the FNV-1a 32-bit hash of data.
func checksum(data []byte) uint32 {
	const (
		offset = 2166136261
		prime  = 16777619
	)
	h := uint32(offset)
	for _, b := range data {
		h ^= uint32(b)
		h *= prime
	}
	return h
}

// Encode frames src into dest with the given command and returns the frame
// length.
func Encode(dest, src []byte, cmd byte) (int, error) {
	lenData := len(src)
	if lenData > MaxPackageSize {
		return 0, ErrInvalidSize
	}
	if len(dest) < lenData+Overhead {
		return 0, ErrInvalidSize
	}
	dest[0] = cmd
	binary.BigEndian.PutUint32(dest[1:], uint32(lenData))
	copy(dest[5:], src)
	binary.BigEndian.PutUint32(dest[5+lenData:], checksum(dest[0:5+lenData]))
	return lenData + Overhead, nil
}

// Decode reads one frame from src into dest, returning the command byte and the
// payload length.
func Decode(dest, src []byte) (cmd byte, n int, err error) {
	if len(src) < Overhead {
		return 0, 0, ErrInvalidSize
	}
	lenData := int(binary.BigEndian.Uint32(src[1:]))
	if lenData > BufferSize {
		return 0, 0, ErrInvalidSize
	}
	if len(src) < lenData+Overhead {
		return 0, 0, ErrInvalidSize
	}
	expected := binary.BigEndian.Uint32(src[5+lenData:])
	if expected != checksum(src[0:5+lenData]) {
		return 0, 0, ErrInvalidChecksum
	}
	copy(dest, src[5:5+lenData])
	return src[0], lenData, nil
}
