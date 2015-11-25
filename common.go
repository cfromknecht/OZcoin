package ozcoin

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"math/big"
)

const (
	MAX_BLOCK_SIZE     = 2 * 1024 * 1024 * 1024 // 2 MB
	TWO_WEEKS_SEC      = 14 * 24 * 60 * 60      // 2 weeks in seconds
	INITIAL_DIFFICULTY = 1 << 20
)

func RandomBytes() SHA256Sum {
	buf := SHA256Sum{}
	_, err := rand.Read(buf[:])
	if err != nil {
		log.Println(err)
		panic("Unable to generate random int")
	}

	return buf
}

func RandomInt() *big.Int {
	buf := RandomBytes()

	r := &big.Int{}
	r.SetBytes(buf[:])

	return r
}

func UIntBytes(x uint64) []byte {
	b := [8]byte{}
	binary.PutUvarint(b[:], x)

	return b[:]
}
