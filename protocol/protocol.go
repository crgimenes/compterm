package protocol

import (
	"encoding/binary"
	"fmt"
)

/*
Protocol format
XYYYYZZZZZZ...ZZZZZZCCCC

Where:

X: command byte
YYYY: data length (32 bits, big endian)
ZZZZZZ...ZZZZZZ: data (array of	bytes)
CCCC: checksum (FNV-1a, 32 bits, big endian)

The checksum is calculated over the command
byte, the data length and the data itself.
*/

const (
	MAX_DATA_SIZE    = 256 * 1024
	MAX_PACKAGE_SIZE = 5 + MAX_DATA_SIZE + 4
)

var (
	ErrInvalidSize     = fmt.Errorf("invalid data size")
	ErrInvalidChecksum = fmt.Errorf("invalid checksum")
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

func Encode(dest []byte, cmd byte, data []byte) (int, error) {
	lenData := len(data)
	if lenData > MAX_PACKAGE_SIZE {
		return 0, ErrInvalidSize
	}
	if len(dest) < lenData+9 {
		return 0, ErrInvalidSize
	}

	dest[0] = cmd
	binary.BigEndian.PutUint32(dest[1:], uint32(lenData))
	copy(dest[5:], data)
	checksum := checksum(dest[0 : 5+lenData])
	binary.BigEndian.PutUint32(dest[5+lenData:], checksum)
	n := 9 + lenData
	return n, nil
}

func Decode(src []byte) (cmd byte, data []byte, err error) {
	// command byte + data length + checksum
	if len(src) < 9 {
		return 0, nil, ErrInvalidSize
	}
	lenData := int(binary.BigEndian.Uint32(src[1:]))
	if lenData > MAX_DATA_SIZE {
		return 0, nil, ErrInvalidSize
	}
	if len(src) < 5+lenData+4 {
		return 0, nil, ErrInvalidSize
	}
	h := binary.BigEndian.Uint32(src[5+lenData:])
	if h != checksum(src[0:5+lenData]) {
		return 0, nil, ErrInvalidChecksum
	}
	return src[0], src[5 : 5+lenData], nil
}
