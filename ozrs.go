package zebracoin

import (
	"math/big"
)

type OZRS struct {
	E  SHA256Sum
	Rs []*big.Int
	Ss []*big.Int
}

func (txn Txn) OZRSSign(pks, ics []ECCPoint,
	sk, yi *big.Int,
	idx int,
	yOut *big.Int) {

	M := txn.MsgBytes()
	hashM := Hash(M)

	// Sum output commitments and take negative
	ocx, ocy := &big.Int{}, &big.Int{}
	for _, otpt := range txn.Outputs {
		ocx, ocy = CURVE.Params().Add(ocx, ocy, otpt.Commit.X, otpt.Commit.Y)
	}
	ocy.Neg(ocy)

	// Calculate commit differences
	diffs := []ECCPoint{}
	for _, c := range ics {
		dix, diy := CURVE.Params().Add(c.X, c.Y, ocx, ocy)
		diffs = append(diffs, ECCPoint{dix, diy})
	}

	es := make([]SHA256Sum, len(txn.Inputs))
	rs := make([]*big.Int, len(txn.Inputs))
	ss := make([]*big.Int, len(txn.Inputs))

	k1, k2 := RandomInt(), RandomInt()
	k1Gx, k1Gy := CURVE.ScalarBaseMult(k1.Bytes())
	k2Gx, k2Gy := CURVE.ScalarBaseMult(k2.Bytes())
	k2HP := Preimage(pks[idx], k2)

	// H( M | k1 G | k2 G | k2 H_P(X_i) )
	eidxData := []byte{}
	eidxData = append(eidxData, hashM[:]...)
	eidxData = append(eidxData, ECCPoint{k1Gx, k1Gy}.Bytes()...)
	eidxData = append(eidxData, ECCPoint{k2Gx, k2Gy}.Bytes()...)
	eidxData = append(eidxData, k2HP.Bytes()...)
	es[idx] = Hash(eidxData)

	for i := idx; i < idx-1; i = (i + 1) % len(pks) {
		next := (i + 1) % len(pks)
		rs[i], ss[i] = RandomInt(), RandomInt()
		es[next] = computeE3(hashM, rs[i], ss[i], es[i], diffs[i], pks[i], txn.Preimage)
	}

	// Complete ring
	prev := (idx - 1) % len(pks)
	e1 := es[prev]
	e2 := Hash(e1[:])

	z := &big.Int{}
	z.Sub(yi, yOut)

	rs[prev] = timeTravel(z, k1, e1)
	ss[prev] = timeTravel(sk, k2, e2)

	// Fill in signature
	txn.Sig.E = es[0]
	txn.Sig.Rs = rs
	txn.Sig.Ss = ss
}

func computeE3(hashM SHA256Sum,
	r, s *big.Int,
	e1 SHA256Sum,
	di, pki, I ECCPoint) SHA256Sum {

	e2 := Hash(e1[:])

	imgi := Preimage(pki, nil)

	r1 := computeR(r, e1, di)
	r2 := computeR(s, e2, pki)
	r3 := computeR2(s, e2, imgi, I)

	data := []byte{}
	data = append(data, hashM[:]...)
	data = append(data, r1.Bytes()...)
	data = append(data, r2.Bytes()...)
	data = append(data, r3.Bytes()...)

	return Hash(data)
}

func computeR2(s *big.Int, e SHA256Sum, base, pk ECCPoint) ECCPoint {
	return PedersenDiffPK2(s.Bytes(), e[:], base, pk)
}

func PedersenDiffPK2(blind, amt []byte, base, pk ECCPoint) ECCPoint {
	xGx, xGy := CURVE.Params().ScalarMult(base.X, base.Y, blind)
	ePx, ePy := CURVE.Params().ScalarMult(pk.X, pk.Y, amt)
	ePy.Neg(ePy)

	x, y := CURVE.Params().Add(xGx, xGy, ePx, ePy)

	return ECCPoint{x, y}
}
