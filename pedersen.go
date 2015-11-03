package cloakcoin

import (
	"crypto/elliptic"
	"fmt"
	"math/big"
)

var (
	CURVE = elliptic.P256()
	H     = computeH()
)

type PublicRangedPedersenCommitment struct {
	ECCPoint
	PublicRangeProof
}

type RangedPedersenCommitment struct {
	ECCPoint
	Proof RangeProof
	Blind *big.Int
	Amt   uint64
}

func (c RangedPedersenCommitment) Public() PublicRangedPedersenCommitment {
	return PublicRangedPedersenCommitment{
		c.ECCPoint,
		c.Proof.PublicRangeProof,
	}
}

func RangedCommit(amt uint64, targetBlind *big.Int) RangedPedersenCommitment {
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

	fmt.Println("[RangedCommit]")
	fmt.Println("targetBlind:", targetBlind)
	fmt.Println("sumBlind:", sumBlind)

	amtBytes := UIntBytes(amt)

	xGx, xGy := CURVE.Params().ScalarBaseMult(targetBlind.Bytes())
	aHx, aHy := CURVE.Params().ScalarMult(H.X, H.Y, amtBytes[:])

	cx, cy := CURVE.Params().Add(xGx, xGy, aHx, aHy)
	fmt.Println("Cx:", y, "Cy:", x)
	fmt.Println("x:", cx, "y:", cy)

	return RangedPedersenCommitment{
		ECCPoint: ECCPoint{x, y},
		Proof:    rp,
		Blind:    sumBlind,
		Amt:      sum,
	}
}

func (c RangedPedersenCommitment) Verify() bool {
	amtBytes := UIntBytes(c.Amt)

	xGx, xGy := CURVE.Params().ScalarBaseMult(c.Blind.Bytes())
	aHx, aHy := CURVE.Params().ScalarMult(H.X, H.Y, amtBytes[:])

	x, y := CURVE.Params().Add(xGx, xGy, aHx, aHy)
	fmt.Println("Cx:", c.X, "Cy:", c.Y)
	fmt.Println("x:", x, "y:", y)

	return c.Proof.Verify() &&
		x.Cmp(c.X) == 0 &&
		y.Cmp(c.Y) == 0
}

func CommitTxn(inputs, outputs []uint64) ([]RangedPedersenCommitment,
	[]RangedPedersenCommitment) {
	pcsi := make([]RangedPedersenCommitment, len(inputs))
	pcso := make([]RangedPedersenCommitment, len(outputs))

	inTotal := &big.Int{}
	for i, inAmt := range inputs {
		r := RandomInt()
		inTotal.Add(inTotal, r)
		pcsi[i] = RangedCommit(inAmt, r)
	}

	outTotal := &big.Int{}
	for i, outAmt := range outputs {
		r := &big.Int{}
		if i == len(outputs)-1 {
			r.Sub(inTotal, outTotal)
			outTotal.Add(outTotal, r)
		} else {
			r = RandomInt()
			outTotal.Add(outTotal, r)
		}

		fmt.Println("[CommitTxn]")
		fmt.Println("targetBlind:", inTotal)
		fmt.Println("sumBlind:", outTotal)

		pcso[i] = RangedCommit(outAmt, r)
	}

	return pcsi, pcso
}

func SumZero(pcsi, pcso []RangedPedersenCommitment) bool {
	x, y := &big.Int{}, &big.Int{}
	fmt.Println("[SumZero]")

	for i, c := range pcsi {
		if !c.Verify() {
			fmt.Println("c", i, "failed to verify")
			return false
		}

		x, y = CURVE.Params().Add(x, y, c.X, c.Y)
	}

	for i, c := range pcso {
		if !c.Verify() {
			fmt.Println("c", i, "failed to verify")
			return false
		}

		negCy := &big.Int{}
		negCy.Neg(c.Y)
		x, y = CURVE.Params().Add(x, y, c.X, negCy)
	}

	fmt.Println("final x:", y, "final y:", x)

	zero := &big.Int{}

	return zero.Cmp(x) == 0 && zero.Cmp(y) == 0
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
