package ozcoin

import (
	"bytes"
	"crypto/sha256"
	"math/big"
)

const RANGE_PROOF_LENGTH = 34

type RangeProof struct {
	E   SHA256Sum                       `json:"e"`
	Ss  [RANGE_PROOF_LENGTH][2]*big.Int `json:"ss"`
	PKs [RANGE_PROOF_LENGTH][2]ECCPoint `json:"pub_keys"`
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

	blinds := ComputeBlinds(targetBlind)
	for i, blind := range blinds {
		sig.PKs[i] = PKsForAmt(amt, uint64(i), blind)
	}

	ocx, ocy := &big.Int{}, &big.Int{}
	for _, pks := range sig.PKs {
		ocx, ocy = CURVE.Params().Add(ocx, ocy, pks[0].X, pks[0].Y)
	}

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
			e1 := HashPt(hashM.Bytes(), ECCPoint{kGx, kGy})
			sig.Ss[i][1] = RandomInt()
			rs[i] = computeR(sig.Ss[i][1], e1, sig.PKs[i][1])
		}
	}

	// Compute e0 from hashes of rs
	e0data := []byte{}
	e0data = append(e0data, hashM.Bytes()...)
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
			sig.Ss[i][1] = timeTravel(blinds[i], ks[i], e1)
		} else {
			sig.Ss[i][0] = timeTravel(blinds[i], ks[i], sig.E)
		}
	}

	return sig
}

func (rp RangeProof) Verify() bool {
	hashM := rp.HashPKs()
	e0data := []byte{}
	e0data = append(e0data, hashM.Bytes()...)
	for i := 0; i < RANGE_PROOF_LENGTH; i++ {
		e1 := computeE(hashM, rp.Ss[i][0], rp.E, rp.PKs[i][0])
		r2 := computeR(rp.Ss[i][1], e1, rp.PKs[i][1])
		e0data = append(e0data, r2.Bytes()...)
	}
	e0Prime := Hash(e0data)

	return bytes.Compare(rp.E.Bytes(), e0Prime.Bytes()) == 0
}

func ComputeBlinds(targetBlind *big.Int) []*big.Int {
	// Compute PKs and blinds for amt
	sumBlind := &big.Int{}
	blinds := []*big.Int{}
	for i := uint64(0); i < RANGE_PROOF_LENGTH; i++ {
		r := &big.Int{}
		if i == 0 {
			r.Set(targetBlind)
		}

		if i == RANGE_PROOF_LENGTH-1 {
			r.Sub(targetBlind, sumBlind)
			r.Mod(r, CURVE.Params().N)

			sumBlind.Add(sumBlind, r)
			sumBlind.Mod(sumBlind, CURVE.Params().N)
		} else {
			hash := Hash(r.Bytes())
			r.SetBytes(hash.Bytes())
			sumBlind.Add(sumBlind, r)
		}

		blinds = append(blinds, r)
	}

	return blinds
}

func PKsForAmt(amt uint64, bit uint64, blind *big.Int) [2]ECCPoint {
	value := uint64(1) << bit
	valueBytes := UIntBytes(value)

	commit := value & amt
	commitBytes := UIntBytes(commit)

	diff := PedersenSum(big.NewInt(0).Bytes(), valueBytes)
	diff.Y.Neg(diff.Y)

	c0 := PedersenSum(blind.Bytes(), commitBytes)
	c1x, c1y := CURVE.Params().Add(c0.X, c0.Y, diff.X, diff.Y)

	return [2]ECCPoint{
		c0,
		ECCPoint{c1x, c1y},
	}
}

func computeE(hashM SHA256Sum, s *big.Int, e SHA256Sum, pk ECCPoint) SHA256Sum {
	r := computeR(s, e, pk)
	return HashPt(hashM.Bytes(), r)
}

func computeR(s *big.Int, e SHA256Sum, pk ECCPoint) ECCPoint {
	return PedersenDiffPK(s.Bytes(), e.Bytes(), pk)
}

func timeTravel(blind, k *big.Int, e SHA256Sum) *big.Int {
	eInt := &big.Int{}
	eInt.SetBytes(e.Bytes())

	s := &big.Int{}
	s.Mul(eInt, blind)
	s.Add(s, k)

	return s
}
