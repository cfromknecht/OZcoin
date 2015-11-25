package ozcoin

import (
	"crypto/elliptic"
	"fmt"
	"math/big"
)

var (
	CURVE = elliptic.P256()
	H     = computeH()
)

type PublicCommitment struct {
	ECCPoint
	PublicRangeProof
}

type Commitment struct {
	ECCPoint
	Proof RangeProof
	Blind *big.Int
	Amt   uint64
}

func (c Commitment) Public() PublicCommitment {
	return PublicCommitment{
		c.ECCPoint,
		c.Proof.PublicRangeProof,
	}
}

func RangeCommit(amt uint64, targetBlind *big.Int) Commitment {
	rp := RangeSign(amt, targetBlind)

	x, y := &big.Int{}, &big.Int{}
	sumBlind := &big.Int{}
	sum := uint64(0)
	for i := uint64(0); i < RANGE_PROOF_LENGTH; i++ {
		pk := rp.PKs[i][0]
		x, y = CURVE.Params().Add(x, y, pk.X, pk.Y)
		sumBlind.Add(sumBlind, rp.Blinds[i])

		sum += (uint64(1) << i) & amt
	}

	return Commitment{
		ECCPoint: ECCPoint{x, y},
		Proof:    rp,
		Blind:    sumBlind,
		Amt:      sum,
	}
}

func (c Commitment) Verify() bool {
	amtBytes := UIntBytes(c.Amt)

	xGx, xGy := CURVE.Params().ScalarBaseMult(c.Blind.Bytes())
	aHx, aHy := CURVE.Params().ScalarMult(H.X, H.Y, amtBytes[:])

	x, y := CURVE.Params().Add(xGx, xGy, aHx, aHy)

	return c.Proof.Verify() &&
		x.Cmp(c.X) == 0 &&
		y.Cmp(c.Y) == 0
}

func CommitTxn(inputs, outputs []uint64) ([]Commitment,
	[]Commitment) {
	pcsi := make([]Commitment, len(inputs))
	pcso := make([]Commitment, len(outputs))

	inTotal := &big.Int{}
	for i, inAmt := range inputs {
		r := RandomInt()
		inTotal.Add(inTotal, r)
		pcsi[i] = RangeCommit(inAmt, r)
	}

	outTotal := &big.Int{}
	for i, outAmt := range outputs {
		r := &big.Int{}
		if i == len(outputs)-1 {
			r.Sub(inTotal, outTotal)
			r.Mod(r, CURVE.Params().N)
		} else {
			r = RandomInt()
			outTotal.Add(outTotal, r)
		}

		pcso[i] = RangeCommit(outAmt, r)
	}

	return pcsi, pcso
}

func SumZero(pcsi, pcso []Commitment) bool {
	ix, iy := &big.Int{}, &big.Int{}

	for i, c := range pcsi {
		if !c.Verify() {
			fmt.Println("c", i, "failed to verify")
			return false
		}

		ix, iy = CURVE.Params().Add(ix, iy, c.X, c.Y)
	}

	ox, oy := &big.Int{}, &big.Int{}
	for i, c := range pcso {
		if !c.Verify() {
			fmt.Println("c", i, "failed to verify")
			return false
		}
		ox, oy = CURVE.Params().Add(ox, oy, c.X, c.Y)
	}

	return ix.Cmp(ox) == 0 && iy.Cmp(oy) == 0
}

func pointFromRAndAmt(r *big.Int, amt int64) (*big.Int, *big.Int) {
	amtInt := big.NewInt(amt)

	xGx, xGy := CURVE.Params().ScalarBaseMult(r.Bytes())
	aHx, aHy := CURVE.Params().ScalarMult(H.X, H.Y, amtInt.Bytes())

	return CURVE.Params().Add(xGx, xGy, aHx, aHy)
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
