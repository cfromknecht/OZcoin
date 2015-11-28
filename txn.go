package ozcoin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math/big"
)

const (
	TXN_NUM_INPUTS  = 8
	TXN_NUM_OUTPUTS = 2
)

type Txn struct {
	Body TxnBody `json:"body"`
	Sig  OZRS    `json:"sig"`
}

type TxnBody struct {
	Inputs  []SHA256Sum `json:"inputs"`
	Outputs []Output    `json:"outputs"`
	Fee     uint64      `json:"fee"`
}

type Output struct {
	PublicKey ECCPoint   `json:"pub_key"`
	DestKey   ECCPoint   `json:"dst_key"`
	BlindSeed ECCPoint   `json:"blind_seed"`
	Commit    Commitment `json:"commit"`
}

func (o Output) Json() []byte {
	b, err := json.Marshal(o)
	if err != nil {
		log.Println(err)
		panic(err)
	}

	return b
}

func (o Output) Hash() SHA256Sum {
	return Hash(o.Json())
}

func (o *Output) DecryptCoinbase() *OutputPlaintext {
	pkHash := Hash(o.PublicKey.Bytes())
	return &OutputPlaintext{
		Output:  o,
		HashPub: base64.StdEncoding.EncodeToString(pkHash.Bytes()),
	}
}

func (o *Output) Decrypt(addr WalletPrivateKey) *OutputPlaintext {
	pkHash := Hash(o.PublicKey.Bytes())
	op := &OutputPlaintext{
		Output:  o,
		HashPub: base64.StdEncoding.EncodeToString(pkHash.Bytes()),
	}

	// Decrypt amount
	yOut := o.ComputeBlindingFactor(addr)
	log.Println("computed blinding factor:", yOut)
	amount, err := o.DecryptAmount(yOut)
	if err != nil {
		log.Println("FAILED TO DECRYPT AMOUNT:", err)
		return nil
	}

	op.Amount = amount

	return op
}

func (o Output) DecryptAmount(yOut *big.Int) (uint64, error) {
	total := uint64(0)
	zero := &big.Int{}

	pks := o.Commit.RangeProof.PKs
	for i, blind := range ComputeBlinds(yOut) {
		rGx, rGy := CURVE.Params().ScalarBaseMult(blind.Bytes())
		rGy.Neg(rGy)

		success := false
		for j, pk := range pks[i] {
			x, y := CURVE.Params().Add(pk.X, pk.Y, rGx, rGy)
			if zero.Cmp(x) == 0 && zero.Cmp(y) == 0 {
				include := uint64(1 - j)
				total += (uint64(1) << uint64(i)) * include
				success = true
				break
			}
		}

		if success {
			continue
		}

		return 0, errors.New("Couldnt not decrypt amount")
	}

	return total, nil
}

func (o Output) ComputeBlindingFactor(addr WalletPrivateKey) *big.Int {
	zero := &big.Int{}
	Q := o.BlindSeed
	// Coinbase txn, blinding factor is 0
	if zero.Cmp(Q.X) == 0 && zero.Cmp(Q.Y) == 0 {
		return zero
	}

	psk := addr.PSK
	bQx, bQy := CURVE.Params().ScalarMult(Q.X, Q.Y, psk.Bytes())
	blind := Hash(ECCPoint{bQx, bQy}.Bytes())

	return blind.Int()
}

func (o Output) BelongsToMe(addr WalletTrackingKey) bool {
	ppk := addr.PPK

	h := o.HashSharedSecret(addr)
	dkx, dky := CURVE.Params().ScalarBaseMult(h.Bytes())
	dkx, dky = CURVE.Params().Add(dkx, dky, ppk.X, ppk.Y)

	return dkx.Cmp(o.DestKey.X) == 0 && dky.Cmp(o.DestKey.Y) == 0
}

func (o Output) ComputeTxnPrivateKey(addr WalletPrivateKey) *big.Int {
	h := o.HashSharedSecret(addr.TrackingKey())
	x := &big.Int{}
	x.SetBytes(h.Bytes())
	x.Add(x, addr.PSK)

	dkx, dky := CURVE.Params().ScalarBaseMult(x.Bytes())
	log.Println("xG:", dkx, dky)

	return x
}

func (o Output) HashSharedSecret(addr WalletTrackingKey) SHA256Sum {
	R := o.PublicKey
	tsk := addr.TSK
	aRx, aRy := CURVE.Params().ScalarMult(R.X, R.Y, tsk.Bytes())

	return Hash(ECCPoint{aRx, aRy}.Bytes())
}

func (c *Client) NewTxn(inputs []Output,
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

	// gather public keys and commitments
	pks := []ECCPoint{}
	ics := []ECCPoint{}
	hashes := []SHA256Sum{}
	for _, inp := range inputs {
		pks = append(pks, inp.DestKey)
		ics = append(ics, inp.Commit.ECCPoint)
		hashes = append(hashes, inp.Hash())
	}

	outputs, blindSum := BuildOutputs(amts, rcpts)

	txn := &Txn{
		Body: TxnBody{
			Inputs:  hashes,
			Outputs: outputs,
			Fee:     fee,
		},
		Sig: OZRS{},
	}
	txn.OZRSSign(pks, ics, sk, yi, idx, blindSum)

	return txn
}

func BuildOutputs(amts []uint64, rcpts []WalletPublicKey) ([]Output, *big.Int) {
	outputs := []Output{}
	blindSum := &big.Int{}
	for i := range amts {
		tpk := rcpts[i].TPK
		ppk := rcpts[i].PPK

		// Compute transaction public key
		r := RandomBytes()
		pkx, pky := CURVE.Params().ScalarBaseMult(r.Bytes())

		// Compute destination key
		secx, secy := CURVE.Params().ScalarMult(tpk.X, tpk.Y, r.Bytes())
		h := Hash(ECCPoint{secx, secy}.Bytes())
		dkx, dky := CURVE.Params().ScalarBaseMult(h[:])
		dkx, dky = CURVE.Params().Add(dkx, dky, ppk.X, ppk.Y)

		// Compute blind seed
		q := RandomBytes()
		qGx, qGy := CURVE.Params().ScalarBaseMult(q.Bytes())

		// Compute target blinding factor
		qBx, qBy := CURVE.Params().ScalarMult(ppk.X, ppk.Y, q.Bytes())
		blind := Hash(ECCPoint{qBx, qBy}.Bytes())

		commit := RangeCommit(amts[i], blind.Int())
		log.Println("Commited to:", commit.ECCPoint)
		amtBytes := UIntBytes(amts[i])
		commitp := PedersenSum(blind.Bytes(), amtBytes)
		log.Println("Computed commitment:", commitp)

		blindSum.Add(blindSum, blind.Int())
		blindSum.Mod(blindSum, CURVE.Params().N)

		output := Output{
			PublicKey: ECCPoint{pkx, pky},
			DestKey:   ECCPoint{dkx, dky},
			BlindSeed: ECCPoint{qGx, qGy},
			Commit:    commit,
		}

		outputs = append(outputs, output)
	}

	return outputs, blindSum
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
