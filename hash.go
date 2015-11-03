package cloakcoin

import (
	"crypto/sha256"
	"math/big"
)

func HashPt(m []byte, p ECCPoint) SHA256Sum {
	data := []byte{}
	data = append(data, m...)
	data = append(data, p.X.Bytes()...)
	data = append(data, p.Y.Bytes()...)

	return sha256.Sum256(data)
}

func HashPtIdx(m []byte, p ECCPoint, i, j uint64) SHA256Sum {
	data := []byte{}
	data = append(data, m...)
	data = append(data, p.X.Bytes()...)
	data = append(data, p.Y.Bytes()...)
	iInt, jInt := &big.Int{}, &big.Int{}
	iInt.SetUint64(i)
	jInt.SetUint64(j)
	data = append(data, iInt.Bytes()...)
	data = append(data, jInt.Bytes()...)

	return sha256.Sum256(data)
}
