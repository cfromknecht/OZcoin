package ozcoin

import (
	"bytes"
	"math/big"
)

/*
 * OZRS
 *
 * Stores the preimage and other validation data for a txn.  The signature
 * proves that the output is equal to exactly the signing input.
 */

type OZRS struct {
	Preimage ECCPoint                 `json:"pimg"`
	E        SHA256Sum                `json:"e"`
	Rs       [TXN_NUM_INPUTS]*big.Int `json:"rs"`
	Ss       [TXN_NUM_INPUTS]*big.Int `json:"ss"`
}

/*
 * Signs a txn given the public keys, input commitments, and other secret data.
 */
func (txn *Txn) OZRSSign(pks, ics []ECCPoint,
	sk, yi *big.Int,
	idx int,
	yOut *big.Int) {

	// Message is hash of txn body.
	M := txn.BodyJson()
	hashM := Hash(M)

	// Calculate signing key preimage
	pimg := Preimage(pks[idx], sk)

	// Calculate commit differences
	diffs := txn.commitDifferences(ics)

	es := [TXN_NUM_INPUTS]SHA256Sum{}
	rs := [TXN_NUM_INPUTS]*big.Int{}
	ss := [TXN_NUM_INPUTS]*big.Int{}

	// Compute target e[idx+1] = H( M | k1 G | k2 G | k2 H_P(X_i) )
	next := (idx + 1) % TXN_NUM_INPUTS

	// Start with k1 G, k2 G, and k2 H_P(X_i)
	k1, k2 := RandomInt(), RandomInt()
	k1Gx, k1Gy := CURVE.ScalarBaseMult(k1.Bytes())
	k2Gx, k2Gy := CURVE.ScalarBaseMult(k2.Bytes())
	k2HP := Preimage(pks[idx], k2)

	// Hash with message
	eidxData := hashM.Bytes()
	eidxData = append(eidxData, ECCPoint{k1Gx, k1Gy}.Bytes()...)
	eidxData = append(eidxData, ECCPoint{k2Gx, k2Gy}.Bytes()...)
	eidxData = append(eidxData, k2HP.Bytes()...)
	es[next] = Hash(eidxData)

	// Compute forward in ring
	for i := next; i != idx; i = (i + 1) % TXN_NUM_INPUTS {
		// Choose arbitrarily
		rs[i], ss[i] = RandomInt(), RandomInt()
		next = (i + 1) % TXN_NUM_INPUTS
		es[next] = computeE3(hashM, rs[i], ss[i], es[i], diffs[i], pks[i], pimg)
	}

	e1 := es[idx]
	e2 := Hash(e1[:])

	// z = input blinding factor - output blinding factor, the sk for diffs[idx]
	z := &big.Int{}
	z.Sub(yi, yOut)
	z.Mod(z, CURVE.Params().N)

	// Complete ring
	rs[idx] = timeTravel(z, k1, e1)
	ss[idx] = timeTravel(sk, k2, e2)

	// Fill in signature
	txn.Sig.Preimage = pimg
	txn.Sig.E = es[0]
	txn.Sig.Rs = rs
	txn.Sig.Ss = ss
}

/*
 * Verifies OZRS Signture given the public keys and input commitments.
 */
func (txn Txn) VerifyOZRS(pks, ics []ECCPoint) bool {
	M := txn.BodyJson()
	hashM := Hash(M)

	// Calculate commit differences
	diffs := txn.commitDifferences(ics)

	// Retrieve preimage
	pimg := txn.Sig.Preimage

	es := [TXN_NUM_INPUTS]SHA256Sum{}
	es[0] = txn.Sig.E

	// Forward compute in ring
	for i := 0; i < TXN_NUM_INPUTS-1; i++ {
		r, s := txn.Sig.Rs[i], txn.Sig.Ss[i]
		es[i+1] = computeE3(hashM, r, s, es[i], diffs[i], pks[i], pimg)
	}

	// Loop back to beginning
	li := TXN_NUM_INPUTS - 1
	e0 := computeE3(hashM, txn.Sig.Rs[li], txn.Sig.Ss[li], es[li], diffs[li], pks[li], pimg)

	// Should be equal to Sig.E in txn
	return bytes.Compare(txn.Sig.E[:], e0[:]) == 0
}

/*
 * Computes the triple-wide Chameleon hash for OZRS.
 */
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

/*
 * Computes used to compute Pedersen differences with arbitrary bases.
 */
func computeR2(s *big.Int, e SHA256Sum, base, pk ECCPoint) ECCPoint {
	return PedersenDiffPK2(s.Bytes(), e[:], base, pk)
}

/*
 * Computes blind * BASE - amt * PK
 */
func PedersenDiffPK2(blind, amt []byte, base, pk ECCPoint) ECCPoint {
	xGx, xGy := CURVE.Params().ScalarMult(base.X, base.Y, blind)
	ePx, ePy := CURVE.Params().ScalarMult(pk.X, pk.Y, amt)
	ePy.Neg(ePy)

	x, y := CURVE.Params().Add(xGx, xGy, ePx, ePy)

	return ECCPoint{x, y}
}

/*
 * Computes the difference between each input commitment and the total output
 * commitment including fees.
 */
func (txn Txn) commitDifferences(ics []ECCPoint) []ECCPoint {
	// Sum output commitments and take negative
	ocx, ocy := &big.Int{}, &big.Int{}
	for _, otpt := range txn.Body.Outputs {
		ocx, ocy = CURVE.Params().Add(ocx, ocy, otpt.Commit.X, otpt.Commit.Y)
	}

	// Add fee*H
	zero := &big.Int{}
	feeBytes := UIntBytes(txn.Body.Fee)
	feec := PedersenSum(zero.Bytes(), feeBytes)

	ocx, ocy = CURVE.Params().Add(ocx, ocy, feec.X, feec.Y)

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
