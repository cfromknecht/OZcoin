package ozcoin

import (
	"math/big"
)

type ECCPoint struct {
	X *big.Int `json:"x"`
	Y *big.Int `json:"y"`
}

func (p ECCPoint) Bytes() []byte {
	data := []byte{}
	data = append(data, p.X.Bytes()...)
	data = append(data, p.Y.Bytes()...)

	return data
}

func (p ECCPoint) Empty() bool {
	return p.X == nil ||
		p.Y == nil
}
