package ozcoin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math/big"
)

/*
 * Output
 *
 * Holds the information necessary to transfer ownership of the committed value
 * to the next entity.  When loading input txn hashes, each one corresponds to
 * exactly one output, instead of a whole txn.
 */
type Output struct {
	PublicKey ECCPoint   `json:"pub_key"`
	DestKey   ECCPoint   `json:"dst_key"`
	BlindSeed ECCPoint   `json:"blind_seed"`
	Commit    Commitment `json:"commit"`
}

/*
 * Returns json bytes.
 */
func (o Output) Json() []byte {
	b, err := json.Marshal(o)
	if err != nil {
		log.Println(err)
		panic(err)
	}

	return b
}

/*
 * Returns hash of json bytes.
 */
func (o Output) Hash() SHA256Sum {
	return Hash(o.Json())
}

/*
 * Gets plaintext info from coinbase txn
 */
func (o *Output) DecryptCoinbase() *OutputPlaintext {
	pkHash := Hash(o.PublicKey.Bytes())
	return &OutputPlaintext{
		Output:  o,
		HashPub: base64.StdEncoding.EncodeToString(pkHash.Bytes()),
	}
}

/*
 * Gets plaintext info from txn
 */
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

/*
 * Tries to recover the plaintext amount from an `Output` using the given
 * blinding factor.
 */
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

/*
 * Uses the `WalletPrivateKey` to recover the output blinding factor.  This is
 * required to spend an output.
 */
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

/*
 * Uses the `WalletTrackingKey` to determine if the `Output` was sent to this
 * address.
 */
func (o Output) BelongsToMe(addr WalletTrackingKey) bool {
	ppk := addr.PPK

	h := o.HashSharedSecret(addr)
	dkx, dky := CURVE.Params().ScalarBaseMult(h.Bytes())
	dkx, dky = CURVE.Params().Add(dkx, dky, ppk.X, ppk.Y)

	return dkx.Cmp(o.DestKey.X) == 0 && dky.Cmp(o.DestKey.Y) == 0
}

/*
 * Computes the private key required to spend an output.
 */
func (o Output) ComputeTxnPrivateKey(addr WalletPrivateKey) *big.Int {
	h := o.HashSharedSecret(addr.TrackingKey())
	x := &big.Int{}
	x.SetBytes(h.Bytes())
	x.Add(x, addr.PSK)

	dkx, dky := CURVE.Params().ScalarBaseMult(x.Bytes())
	log.Println("xG:", dkx, dky)

	return x
}

/*
 * Computes the hash of shared secret.  This allows the receiver to check if the
 * output belongs to me or recover the secret key.
 */
func (o Output) HashSharedSecret(addr WalletTrackingKey) SHA256Sum {
	R := o.PublicKey
	tsk := addr.TSK
	aRx, aRy := CURVE.Params().ScalarMult(R.X, R.Y, tsk.Bytes())

	return Hash(ECCPoint{aRx, aRy}.Bytes())
}

/*
 * Tries to load output from local storage.
 */
func (c *Client) LoadOutput(hash SHA256Sum) (*Output, error) {
	if c.Type != BLOCKCHAIN_CLIENT {
		return nil, errors.New("Not BLOCKCHAIN_CLIENT")
	}

	blockHash, err := c.MapToBlock(hash)
	if err != nil {
		return nil, err
	}

	block, err := c.LoadBlock(blockHash)
	if err != nil {
		return nil, err
	}

	for _, txn := range block.Txns {
		for _, o := range txn.Body.Outputs {
			if o.Hash() == hash {
				output := &Output{}
				*output = o
				return output, nil
			}
		}
	}

	return nil, errors.New("Could not load output")
}

/*
 * Tries to load an output from local storage.  Otherwise iterates through peers
 * until one succeeds.
 */
func (c *Client) FindOutput(hash SHA256Sum) (*Output, error) {
	output, err := c.LoadOutput(hash)
	if err == nil {
		return output, nil
	}

	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		output, err := c.FetchOutput(hash, address)
		if err == nil {
			return output, nil
		}
	}
	iter.Release()

	return nil, iter.Error()
}
