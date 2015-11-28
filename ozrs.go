package ozcoin

import (
	"bytes"
	"log"
	"math/big"
)

type OZRS struct {
	Preimage ECCPoint                 `json:"pimg"`
	E        SHA256Sum                `json:"e"`
	Rs       [TXN_NUM_INPUTS]*big.Int `json:"rs"`
	Ss       [TXN_NUM_INPUTS]*big.Int `json:"ss"`
}

func (txn *Txn) OZRSSign(pks, ics []ECCPoint,
	sk, yi *big.Int,
	idx int,
	yOut *big.Int) {

	log.Println("sk:", sk)
	log.Println("pks[0]:", pks[0])
	skGx, skGy := CURVE.Params().ScalarBaseMult(sk.Bytes())
	log.Println("skG:", ECCPoint{skGx, skGy})

	M := txn.BodyJson()
	hashM := Hash(M)

	// Calculate signing key preimage
	pimg := Preimage(pks[idx], sk)

	// Calculate commit differences
	diffs := txn.commitDifferences(ics)

	es := [TXN_NUM_INPUTS]SHA256Sum{}
	rs := [TXN_NUM_INPUTS]*big.Int{}
	ss := [TXN_NUM_INPUTS]*big.Int{}

	k1, k2 := RandomInt(), RandomInt()
	k1Gx, k1Gy := CURVE.ScalarBaseMult(k1.Bytes())
	k2Gx, k2Gy := CURVE.ScalarBaseMult(k2.Bytes())
	k2HP := Preimage(pks[idx], k2)

	log.Println("k1G:", k1Gx, k1Gy)
	log.Println("k2G:", k2Gx, k2Gy)
	log.Println("k2HP:", k2HP)

	// e[idx+1] = H( M | k1 G | k2 G | k2 H_P(X_i) )
	eidxData := []byte{}
	eidxData = append(eidxData, hashM[:]...)
	eidxData = append(eidxData, ECCPoint{k1Gx, k1Gy}.Bytes()...)
	eidxData = append(eidxData, ECCPoint{k2Gx, k2Gy}.Bytes()...)
	eidxData = append(eidxData, k2HP.Bytes()...)

	next := (idx + 1) % TXN_NUM_INPUTS
	es[next] = Hash(eidxData)

	for i := next; i != idx; i = (i + 1) % TXN_NUM_INPUTS {
		rs[i], ss[i] = RandomInt(), RandomInt()
		next = (i + 1) % TXN_NUM_INPUTS
		es[next] = computeE3(hashM, rs[i], ss[i], es[i], diffs[i], pks[i], pimg)
	}

	// Complete ring
	e1 := es[idx]
	e2 := Hash(e1[:])

	z := &big.Int{}
	z.Sub(yi, yOut)
	z.Mod(z, CURVE.Params().N)

	rs[idx] = timeTravel(z, k1, e1)
	ss[idx] = timeTravel(sk, k2, e2)

	pimgi := Preimage(pks[idx], nil)

	k1Gp := computeR(rs[idx], e1, diffs[idx])
	k2Gp := computeR(ss[idx], e2, pks[idx])
	k2HPp := computeR2(ss[idx], e2, pimgi, pimg)

	log.Println("k1Gp:", k1Gp)
	log.Println("k2Gp:", k2Gp)
	log.Println("k2HPp:", k2HPp)

	// Fill in signature
	txn.Sig.Preimage = pimg
	txn.Sig.E = es[0]
	txn.Sig.Rs = rs
	txn.Sig.Ss = ss

	log.Println("PKS:", pks)
	log.Println("Commits:", ics)
}

func (txn Txn) VerifyOZRS(pks, ics []ECCPoint) bool {
	log.Println("PKS:", pks)
	log.Println("Commits:", ics)

	M := txn.BodyJson()
	hashM := Hash(M)

	// Calculate commit differences
	diffs := txn.commitDifferences(ics)

	// Retrieve preimage
	pimg := txn.Sig.Preimage

	es := [TXN_NUM_INPUTS]SHA256Sum{}
	es[0] = txn.Sig.E

	for i := 0; i < TXN_NUM_INPUTS-1; i++ {
		r, s := txn.Sig.Rs[i], txn.Sig.Ss[i]
		es[i+1] = computeE3(hashM, r, s, es[i], diffs[i], pks[i], pimg)
	}

	li := TXN_NUM_INPUTS - 1
	e0 := computeE3(hashM, txn.Sig.Rs[li], txn.Sig.Ss[li], es[li], diffs[li], pks[li], pimg)

	return bytes.Compare(txn.Sig.E[:], e0[:]) == 0
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

func (txn Txn) commitDifferences(ics []ECCPoint) []ECCPoint {
	// Sum output commitments and take negative
	ocx, ocy := &big.Int{}, &big.Int{}
	for _, otpt := range txn.Body.Outputs {
		log.Println("adding commitment:", otpt.Commit.ECCPoint)
		ocx, ocy = CURVE.Params().Add(ocx, ocy, otpt.Commit.X, otpt.Commit.Y)
	}

	// Add fee*H
	zero := &big.Int{}
	feeBytes := UIntBytes(txn.Body.Fee)
	feec := PedersenSum(zero.Bytes(), feeBytes)
	log.Println("adding fee:", feec)

	ocx, ocy = CURVE.Params().Add(ocx, ocy, feec.X, feec.Y)
	log.Println("output commitment:", ocx, ocy)

	// Take negative
	ocy.Neg(ocy)
	oc := ECCPoint{ocx, ocy}

	// Subtract total output commitment from each input commitment
	diffs := []ECCPoint{}
	for _, c := range ics {
		dix, diy := CURVE.Params().Add(c.X, c.Y, oc.X, oc.Y)
		diffs = append(diffs, ECCPoint{dix, diy})
	}

	return diffs
}
