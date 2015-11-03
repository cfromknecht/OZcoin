package cloakcoin

import (
	"math/big"
)

type SHA256Sum [32]byte

type ECCPoint struct {
	X, Y *big.Int
}

func (p ECCPoint) Bytes() []byte {
	data := []byte{}
	data = append(data, p.X.Bytes()...)
	data = append(data, p.Y.Bytes()...)

	return data
}
