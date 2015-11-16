package zebracoin

import (
	"math/big"
)

type ECCPoint struct {
	X, Y *big.Int
}

func (p ECCPoint) Bytes() []byte {
	data := []byte{}
	data = append(data, p.X.Bytes()...)
	data = append(data, p.Y.Bytes()...)

	return data
}
