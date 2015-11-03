package cloakcoin

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
)

type PublicRangeProof struct {
	E   SHA256Sum
	Ss  [RANGE_PROOF_LENGTH][2]*big.Int
	PKs [RANGE_PROOF_LENGTH][2]ECCPoint
}

type RangeProof struct {
	PublicRangeProof
	Blinds [RANGE_PROOF_LENGTH]*big.Int
}

func (s RangeProof) HashPKs() SHA256Sum {
	data := []byte{}
	for i := 0; i < RANGE_PROOF_LENGTH; i++ {
		data = append(data, s.PKs[i][0].Bytes()...)
		data = append(data, s.PKs[i][1].Bytes()...)
	}

	return sha256.Sum256(data)
}

func RangeSign(amt uint64, targetBlind *big.Int) RangeProof {
	sig := RangeProof{}

	sumBlind := &big.Int{}
	for i := uint64(0); i < RANGE_PROOF_LENGTH; i++ {
		r := &big.Int{}
		if i == RANGE_PROOF_LENGTH-1 {
			r = RandomInt()
			r.Sub(targetBlind, sumBlind)
			sumBlind.Add(sumBlind, r)
		} else {
			r = RandomInt()
			sumBlind.Add(sumBlind, r)
		}

		sig.Blinds[i] = r
		sig.PKs[i] = PKsForAmt(amt, i, r)
	}

	fmt.Println("[RangeSign]")
	fmt.Println("targetBlind:", targetBlind)
	fmt.Println("sumBlind:", sumBlind)

	hashM := sig.HashPKs()

	// Compute forward chain
	ks := [RANGE_PROOF_LENGTH]*big.Int{}
	rs := [RANGE_PROOF_LENGTH]ECCPoint{}
	for i := uint64(0); i < RANGE_PROOF_LENGTH; i++ {
		value := uint64(1) << i
		signNonZero := value&amt > 0

		ks[i] = RandomInt()
		kGx, kGy := CURVE.Params().ScalarBaseMult(ks[i].Bytes())
		if signNonZero {
			rs[i] = ECCPoint{kGx, kGy}
		} else {
			e1 := HashPt(hashM[:], ECCPoint{kGx, kGy})
			sig.Ss[i][1] = RandomInt()
			rs[i] = computeR(sig.Ss[i][1], e1, sig.PKs[i][1])
		}
	}

	// Compute e0 from hashes of rs
	e0data := []byte{}
	for i := 0; i < RANGE_PROOF_LENGTH; i++ {
		e0data = append(e0data, rs[i].Bytes()...)
	}
	sig.E = sha256.Sum256(e0data)

	// Compute forward and time travel complete cycle
	for i := uint64(0); i < RANGE_PROOF_LENGTH; i++ {
		value := uint64(1) << i
		signNonZero := value&amt > 0

		if signNonZero {
			sig.Ss[i][0] = RandomInt()
			e1 := computeE(hashM, sig.Ss[i][0], sig.E, sig.PKs[i][0])
			sig.Ss[i][1] = timeTravel(sig.Blinds[i], ks[i], e1)
		} else {
			sig.Ss[i][0] = timeTravel(sig.Blinds[i], ks[i], sig.E)
		}
	}

	return sig
}

func (rs RangeProof) Verify() bool {
	hashM := rs.HashPKs()
	e0data := []byte{}
	for i := 0; i < RANGE_PROOF_LENGTH; i++ {
		e1 := computeE(hashM, rs.Ss[i][0], rs.E, rs.PKs[i][0])
		r2 := computeR(rs.Ss[i][1], e1, rs.PKs[i][1])
		e0data = append(e0data, r2.Bytes()...)
	}
	e0Prime := sha256.Sum256(e0data)
	return bytes.Compare(rs.E[:], e0Prime[:]) == 0
}

func PKsForAmt(amt uint64, bit uint64, blind *big.Int) [2]ECCPoint {
	value := uint64(1) << bit
	valueBytes := [8]byte{}
	binary.PutUvarint(valueBytes[:], value)

	commit := value & amt
	commitBytes := [8]byte{}
	binary.PutUvarint(commitBytes[:], commit)

	diff := PedersenSum(big.NewInt(0).Bytes(), valueBytes[:])
	diff.Y.Neg(diff.Y)

	c0 := PedersenSum(blind.Bytes(), commitBytes[:])
	c1x, c1y := CURVE.Params().Add(c0.X, c0.Y, diff.X, diff.Y)

	return [2]ECCPoint{
		c0,
		ECCPoint{c1x, c1y},
	}
}

func computeE(hashM SHA256Sum, s *big.Int, e SHA256Sum, pk ECCPoint) SHA256Sum {
	r := computeR(s, e, pk)
	return HashPt(hashM[:], r)
}

func computeR(s *big.Int, e SHA256Sum, pk ECCPoint) ECCPoint {
	return PedersenDiffPK(s.Bytes(), e[:], pk)
}

func timeTravel(blind, k *big.Int, e SHA256Sum) *big.Int {
	eInt := &big.Int{}
	eInt.SetBytes(e[:])

	s := &big.Int{}
	s.Mul(eInt, blind)
	s.Add(s, k)

	return s
}
