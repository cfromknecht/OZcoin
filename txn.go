package zebracoin

import (
	"math/big"
)

type Txn struct {
	Preimage ECCPoint
	Inputs   []string
	Outputs  []Output
	Sig      OZRS
}

type Output struct {
	PublicKey ECCPoint
	DestKey   ECCPoint
	BlindSeed ECCPoint
	Commit    Commitment
}

func NewTxn(inputs []string,
	sk *big.Int,
	yi *big.Int,
	idx int,
	amts []uint64,
	rcpts []WalletPublicKey) *Txn {

	if amts == nil || rcpts == nil {
		return nil
	}

	if idx < 0 || idx >= len(inputs) {
		return nil
	}

	if len(amts) != len(rcpts) {
		return nil
	}

	// fetch pks and check public keys
	pks := []ECCPoint{}
	ics := []ECCPoint{}
	for i, _ := range inputs {
		// lookup prevhash from db to get pks and commitments

		// for now...
		// create pks
		x := RandomInt()
		if i == idx {
			x = sk
		}
		pkx, pky := CURVE.Params().ScalarBaseMult(x.Bytes())
		pks = append(pks, ECCPoint{pkx, pky})

		// create commitments
		y := RandomInt()
		if i == idx {
			y = yi
		}
		amtBytes := UIntBytes(amts[i])
		C := PedersenSum(y.Bytes(), amtBytes)
		ics = append(ics, C)
	}
	spk := pks[idx]
	pimg := Preimage(spk, sk)

	outputs := []Output{}
	sumYOut := &big.Int{}
	for i := range amts {
		tpk := rcpts[i].TPK
		ppk := rcpts[i].PPK

		// Compute transaction public key
		r := RandomBytes()
		pkx, pky := CURVE.Params().ScalarBaseMult(r[:])

		// Compute destination key
		secx, secy := CURVE.Params().ScalarMult(tpk.X, tpk.Y, r[:])
		dk := HashToPt(ECCPoint{secx, secy}.Bytes())
		dk.X, dk.Y = CURVE.Params().Add(dk.X, dk.Y, ppk.X, ppk.Y)

		// Compute blind seed
		q := RandomBytes()
		qGx, qGy := CURVE.Params().ScalarBaseMult(q[:])

		// Compute target blinding factor
		qBx, qBy := CURVE.Params().ScalarMult(ppk.X, ppk.Y, q[:])
		hqB := HashPt([]byte{}, ECCPoint{qBx, qBy})

		// Commit value using hqB as target blinding factor
		yOut := &big.Int{}
		yOut.SetBytes(hqB[:])
		c := RangeCommit(amts[i], yOut)

		sumYOut.Add(sumYOut, yOut)

		output := Output{
			PublicKey: ECCPoint{pkx, pky},
			DestKey:   dk,
			BlindSeed: ECCPoint{qGx, qGy},
			Commit:    c,
		}

		outputs = append(outputs, output)
	}

	txn := &Txn{
		Preimage: pimg,
		Inputs:   inputs,
		Outputs:  outputs,
		Sig:      OZRS{},
	}
	txn.OZRSSign(pks, ics, sk, yi, idx, sumYOut)

	return txn
}

func (txn Txn) MsgBytes() []byte {
	m := []byte{}
	m = append(m, txn.Preimage.Bytes()...)
	for _, pk := range txn.Inputs {
		m = append(m, []byte(pk)...)
	}
	for _, otpt := range txn.Outputs {
		m = append(m, otpt.PublicKey.Bytes()...)
		m = append(m, otpt.DestKey.Bytes()...)
		m = append(m, otpt.BlindSeed.Bytes()...)
		m = append(m, otpt.Commit.Bytes()...)
	}

	return m
}

func Preimage(pk ECCPoint, sk *big.Int) ECCPoint {
	hp := HashToPt(pk.Bytes())
	if sk != nil {
		hp.X, hp.Y = CURVE.Params().ScalarMult(hp.X, hp.Y, sk.Bytes())
	}

	return hp
}
