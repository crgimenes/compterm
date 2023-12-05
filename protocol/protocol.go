package protocol

import (
	"encoding/binary"
	"fmt"
	"sync"
)

/*
Protocol format

ABBCCCCDDD...DDDFFFF

Where:

A: command byte
B: conter (2 bytes, big endian) optional used to debug and validate package order
C: payload length (32 bits, big endian)
D: payload (array of bytes)
F: checksum (FNV-1a, 32 bits, big endian)

The checksum is calculated over the command
byte, conter, payload length and the data itself.
*/

const (
	MAX_DATA_SIZE    = 256 * 1024
	MAX_PACKAGE_SIZE = MAX_DATA_SIZE + 11
)

var (
	ErrInvalidSize     = fmt.Errorf("invalid data size")
	ErrInvalidChecksum = fmt.Errorf("invalid checksum")

	mu    sync.Mutex
	count uint16
)

func checksum(data []byte) uint32 {
	const (
		offsetBasis = uint32(2166136261)
		prime       = 16777619
	)

	h := offsetBasis
	for _, b := range data {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

func Encode(dest, src []byte, cmd byte) (int, error) {
	lenData := len(src)
	if lenData > MAX_PACKAGE_SIZE {
		return 0, ErrInvalidSize
	}
	if len(dest) < lenData+11 {
		return 0, ErrInvalidSize
	}
	dest[0] = cmd
	binary.BigEndian.PutUint16(dest[1:], count)
	binary.BigEndian.PutUint32(dest[3:], uint32(lenData))
	copy(dest[7:], src)
	checksum := checksum(dest[0 : 7+lenData])
	binary.BigEndian.PutUint32(dest[7+lenData:], checksum)
	n := lenData + 11
	mu.Lock()
	count++
	mu.Unlock()
	return n, nil
}

func Decode(dest, src []byte) (cmd byte, n int, i int16, err error) {
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
	i = int16(binary.BigEndian.Uint16(src[1:]))
	h := binary.BigEndian.Uint32(src[7+lenData:])
	if h != checksum(src[0:7+lenData]) {
		return 0, 0, 0, ErrInvalidChecksum
	}
	copy(dest, src[7:7+lenData])
	return src[0], lenData, i, nil
}
