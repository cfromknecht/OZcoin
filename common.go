package cloakcoin

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"math/big"
)

const RANGE_PROOF_LENGTH = 32

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
