package ozcoin

import (
	"math/big"
	"testing"
)

type rangeSignInput struct {
	Amt    uint64
	Actual uint64
}

var rangeSignInputs = []rangeSignInput{
	rangeSignInput{0, 0},
	rangeSignInput{1, 1},
	rangeSignInput{5000000000, 5000000000},
	rangeSignInput{17179869184, 0},
}

func TestRangeCommit(t *testing.T) {
	for i, inp := range rangeSignInputs {
		r := &big.Int{}
		rp := RangeCommit(inp.Amt, r)

		// Expected commit
		actualBytes := UIntBytes(inp.Actual)
		exp := PedersenSum(r.Bytes(), actualBytes)

		// Subtract expected from commit
		exp.Y.Neg(exp.Y)
		exp.X, exp.Y = CURVE.Params().Add(rp.X, rp.Y, exp.X, exp.Y)

		// Should be 0's
		zero := &big.Int{}
		if zero.Cmp(exp.X) != 0 || zero.Cmp(exp.Y) != 0 {
			t.Error("Actual commit different from expected commit")
		}

		// Should always verify
		if !rp.Verify() {
			t.Error("Range proof", i, " failed to verify unexpectedly")
		}
	}
}
