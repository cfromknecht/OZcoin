package ozcoin

import (
	"encoding/json"
	"log"
	"math/big"
)

const (
	TXN_NUM_INPUTS  = 8
	TXN_NUM_OUTPUTS = 2
)

type Txn struct {
	Body TxnBody
	Sig  OZRS
}

type TxnBody struct {
	Inputs  []Input
	Outputs []Output
	Fee     uint64
}

type Input struct {
	Hash  SHA256Sum
	Index uint8
}

type Output struct {
	PublicKey ECCPoint
	DestKey   ECCPoint
	BlindSeed ECCPoint
	Commit    Commitment
}

func (bc *Blockchain) NewTxn(inputs []Input,
	sk, yi *big.Int,
	idx int,
	amts []uint64,
	rcpts []WalletPublicKey,
	fee uint64) *Txn {

	if inputs == nil || amts == nil || rcpts == nil {
		return nil
	}

	if idx < 0 || idx >= TXN_NUM_INPUTS {
		return nil
	}

	utxns, err := bc.fetchInputTxns(inputs)
	if err != nil {
		log.Println(err)
		panic(err)
	}

	// gather public keys and commitments
	pks, ics := []ECCPoint{}, []Commitment{}
	for _, utxn := range utxns {
		pks = append(pks, utxn.PublicKey)
		ics = append(ics, utxn.Commit)
	}

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
		Body: TxnBody{
			Inputs:  inputs,
			Outputs: outputs,
			Fee:     fee,
		},
		Sig: OZRS{},
	}
	txn.OZRSSign(pks, ics, sk, yi, idx, sumYOut)

	return txn
}

func (txn Txn) BodyJson() []byte {
	b, err := json.Marshal(txn.Body)
	if err != nil {
		log.Println(err)
		panic("Could not marshal txn body")
	}

	return b
}

func (txn Txn) Json() []byte {
	b, err := json.Marshal(txn)
	if err != nil {
		log.Println(err)
		panic("Could not marshal txn")
	}

	return b
}

func (txn Txn) Hash() SHA256Sum {
	return Hash(txn.Json())
}

func Preimage(pk ECCPoint, sk *big.Int) ECCPoint {
	hp := HashToPt(pk.Bytes())
	if sk != nil {
		hp.X, hp.Y = CURVE.Params().ScalarMult(hp.X, hp.Y, sk.Bytes())
	}

	return hp
}
