package protocol

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
)

/*
Protocol format

ABBCCCCDDD...DDDFFFF

Where:

A: command byte
B: counter (2 bytes, big endian) optional used to debug and validate package order
C: payload length (32 bits, big endian)
D: payload (array of bytes)
F: checksum (FNV-1a, 32 bits, big endian)

The checksum is calculated over the command byte, counter, payload length, and the data itself.
*/

const (
	MAX_DATA_SIZE    = 256 * 1024         // max payload size
	MAX_PACKAGE_SIZE = MAX_DATA_SIZE + 11 // max package size
)

var (
	ErrInvalidSize     = errors.New("invalid size")
	ErrInvalidChecksum = errors.New("invalid checksum")
)

// checksum calculates the FNV-1a checksum of the given data.
func checksum(data []byte) uint32 {
	hash := fnv.New32a()
	hash.Write(data)
	return hash.Sum32()
}

// Encode encodes the source data into the destination buffer
// using the specified command.
// It returns the number of bytes written and an error, if any.
func Encode(dest, src []byte, cmd byte, counter uint16) (int, error) {
	lenData := len(src)
	if lenData > MAX_PACKAGE_SIZE {
		return 0, ErrInvalidSize
	}
	if len(dest) < lenData+11 {
		return 0, ErrInvalidSize
	}
	dest[0] = cmd
	binary.BigEndian.PutUint16(dest[1:], counter)
	binary.BigEndian.PutUint32(dest[3:], uint32(lenData))
	copy(dest[7:], src)
	checksum := checksum(dest[0 : 7+lenData])
	binary.BigEndian.PutUint32(dest[7+lenData:], checksum)
	n := lenData + 11
	return n, nil
}

// Decode decodes the source buffer into the destination buffer.
// It returns the command byte, the number of bytes read, the
// counter value, and an error, if any.
func Decode(dest, src []byte) (cmd byte, n int, counter uint16, err error) {
	// command byte + counter + data length + checksum length = 11
	if len(src) < 11 {
		return 0, 0, 0, ErrInvalidSize
	}
	lenData := int(binary.BigEndian.Uint32(src[3:]))
	if lenData > MAX_DATA_SIZE {
		return 0, 0, 0, ErrInvalidSize
	}
	if len(src) < lenData+11 {
		return 0, 0, 0, ErrInvalidSize
	}
	counter = binary.BigEndian.Uint16(src[1:])
	h := binary.BigEndian.Uint32(src[7+lenData:])
	if h != checksum(src[0:7+lenData]) {
		return 0, 0, 0, ErrInvalidChecksum
	}
	copy(dest, src[7:7+lenData])
	return src[0], lenData, counter, nil
}
