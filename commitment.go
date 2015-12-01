package ozcoin

import (
	"crypto/elliptic"
	"math/big"
)

var (
	CURVE = elliptic.P256()
	H     = computeH()
)

type Commitment struct {
	ECCPoint
	RangeProof
}

func RangeCommit(amt uint64, targetBlind *big.Int) Commitment {
	rp := RangeSign(amt, targetBlind)

	x, y := &big.Int{}, &big.Int{}
	for i := uint64(0); i < RANGE_PROOF_LENGTH; i++ {
		pk := rp.PKs[i][0]
		x, y = CURVE.Params().Add(x, y, pk.X, pk.Y)
	}

	return Commitment{
		ECCPoint:   ECCPoint{x, y},
		RangeProof: rp,
	}
}

func computeH() ECCPoint {
	hx, hy := CURVE.Params().ScalarBaseMult(big.NewInt(11235).Bytes())
	if !CURVE.Params().IsOnCurve(hx, hy) {
		panic("hx, hy is not on the curve")
	}

	return ECCPoint{hx, hy}
}

func PedersenSum(blind, amt []byte) ECCPoint {
	return PedersenSumPK(blind, amt, H)
}

func PedersenSumPK(blind, amt []byte, pk ECCPoint) ECCPoint {
	xGx, xGy := CURVE.Params().ScalarBaseMult(blind)
	ePx, ePy := CURVE.Params().ScalarMult(pk.X, pk.Y, amt)

	x, y := CURVE.Params().Add(xGx, xGy, ePx, ePy)

	return ECCPoint{x, y}
}

func PedersenDiff(blind, amt []byte) ECCPoint {
	return PedersenDiffPK(blind, amt, H)
}

func PedersenDiffPK(blind, amt []byte, pk ECCPoint) ECCPoint {
	xGx, xGy := CURVE.Params().ScalarBaseMult(blind)
	ePx, ePy := CURVE.Params().ScalarMult(pk.X, pk.Y, amt)
	ePy.Neg(ePy)

	x, y := CURVE.Params().Add(xGx, xGy, ePx, ePy)

	return ECCPoint{x, y}
}
