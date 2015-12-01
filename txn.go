package ozcoin

import (
	"encoding/json"
	"errors"
	"log"
	"math/big"
)

const (
	TXN_NUM_INPUTS  = 8
	TXN_NUM_OUTPUTS = 2
)

/*
 * Txn
 *
 * Describes how transaction `Ouptut`s are to be transferred.  Each OZCoin txn
 * draws from 8 inputs to standardize anonymity. Each txn has exactly 2 outputs,
 * which can be used to return the difference the sender.  This value can be 0
 * if the entire amount should be sent.
 */

type Txn struct {
	Body TxnBody `json:"body"`
	Sig  OZRS    `json:"sig"`
}

/*
 * TxnBody
 *
 * The portion of the `Txn` to be signed.
 */
type TxnBody struct {
	Inputs  []SHA256Sum `json:"inputs"`
	Outputs []Output    `json:"outputs"`
	Fee     uint64      `json:"fee"`
}

/*
 * Creates a new `Txn` that spends the input at index `idx`.  The secret key
 * `sk` and blinding factor `yi` allow the sender to compute a valid OZRS
 * signature.
 */
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

/*
 * Builds a new coinbase txn given the block sequence, total block fees, and the
 * destination address.
 */
func NewCoinbaseTxn(address WalletPublicKey, seqNum, fee uint64) Txn {
	tpk := address.TPK
	ppk := address.PPK

	zero := &big.Int{}
	coinbase := CoinbaseValue(seqNum) + fee
	coinbaseBytes := UIntBytes(coinbase)
	commit := PedersenSum(zero.Bytes(), coinbaseBytes)

	// Public Key
	r := RandomBytes()
	rGx, rGy := CURVE.ScalarBaseMult(r.Bytes())

	// Destination Key
	secx, secy := CURVE.Params().ScalarMult(tpk.X, tpk.Y, r.Bytes())
	h := Hash(ECCPoint{secx, secy}.Bytes())
	dkx, dky := CURVE.Params().ScalarBaseMult(h.Bytes())
	dkx, dky = CURVE.Params().Add(dkx, dky, ppk.X, ppk.Y)

	ss := [RANGE_PROOF_LENGTH][2]*big.Int{}
	for i, pair := range ss {
		for j := range pair {
			ss[i][j] = zero
		}
	}
	rs := [TXN_NUM_INPUTS]*big.Int{}
	for i := range rs {
		rs[i] = zero
	}

	return Txn{
		Body: TxnBody{
			Inputs: []SHA256Sum{
				SHA256Sum{},
			},
			Outputs: []Output{
				Output{
					PublicKey: ECCPoint{rGx, rGy},
					DestKey:   ECCPoint{dkx, dky},
					BlindSeed: ECCPoint{zero, zero},
					Commit: Commitment{
						ECCPoint: commit,
						RangeProof: RangeProof{
							Ss: ss,
						},
					},
				},
			},
		},
		Sig: OZRS{
			Rs: rs,
			Ss: rs,
		},
	}
}

/*
 * Computes the txn public key, destination key, blind seed, and commitment that
 * sends each amount to its corresponding recipient.
 */
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

/*
 * Prevalidates the txns in a block.
 */
func (c *Client) ValidTxns(b Block) bool {
	// Check validity of each transaction
	for i, txn := range b.Txns {
		if i == 0 {
			if !ValidCoinbaseTxn(txn) {
				log.Println("Invalid CoinbaseTxn")
				return false
			}
		} else {
			if !ValidTxn(txn) {
				log.Println("Invalid Txn")
				return false
			}
		}
	}

	return true
}

/*
 * Less intensive txn validations.
 */
func ValidTxn(txn Txn) bool {

	if txn.Body.Inputs == nil || len(txn.Body.Inputs) != TXN_NUM_INPUTS {
		log.Println("Invalid number of txn inputs")
		return false
	}

	if txn.Body.Outputs == nil || len(txn.Body.Outputs) != TXN_NUM_OUTPUTS {
		log.Println("Invalid number of txn outputs")
		return false
	}

	if len(txn.Json()) >= MAX_BLOCK_SIZE {
		log.Println("TXN TOO BIG")
		return false
	}

	for _, output := range txn.Body.Outputs {
		if output.PublicKey.Empty() ||
			output.DestKey.Empty() ||
			output.BlindSeed.Empty() ||
			output.Commit.Empty() {
			log.Println("Missing data")
			return false
		}
	}

	return true
}

/*
 * Less intensive coinbase txn validations.
 */
func ValidCoinbaseTxn(txn Txn) bool {
	if txn.Body.Inputs == nil || len(txn.Body.Inputs) != 1 {
		log.Println("Invalid number of txn inputs")
		return false
	}

	input := txn.Body.Inputs[0]
	if (input != SHA256Sum{}) {
		log.Println("Invalid coinbase inputs")
		return false
	}

	if txn.Body.Outputs == nil || len(txn.Body.Outputs) != 1 {
		log.Println("Invalid number of txn outputs")
		return false
	}

	if len(txn.Json()) >= MAX_BLOCK_SIZE {
		log.Println("Txn exceeds MAX_BLOCK_SIZE")
		return false
	}

	output := txn.Body.Outputs[0]

	if output.PublicKey.Empty() ||
		output.DestKey.Empty() ||
		output.BlindSeed.Empty() ||
		output.Commit.Empty() {
		log.Println("Missing data")
		return false
	}

	return true
}

/*
 * Verifies a block's transactions given the forking context.
 */
func (c *Client) VerifyTxns(b Block, mainPath, sidePath []SHA256Sum) bool {
	// Check that mainPath and sidePath are set properly
	if (mainPath == nil && sidePath != nil) ||
		(mainPath != nil && sidePath == nil) {
		panic("Both mainPath and sidePath should be set or nil")
	}

	// Compute invalid unspent txns and preimages
	mainTxns, mainPimgs, err := c.ForkTxnsAndPreimages(mainPath)
	if err != nil {
		log.Println(err)
		return false
	}

	sideTxns, sidePimgs, err := c.ForkTxnsAndPreimages(sidePath)
	if err != nil {
		log.Println(err)
		return false
	}

	coinbase := CoinbaseValue(b.Header.SeqNum)

	// Check validity of each transaction
	for i, txn := range b.Txns {
		if i != 0 {
			if !c.VerifyTxn(txn, mainTxns, sideTxns, mainPimgs, sidePimgs) {
				log.Println("Invalid Txn")
				return false
			}

			coinbase += txn.Body.Fee
		}
	}

	if !c.VerifyCoinbaseTxn(b.Txns[0], coinbase) {
		log.Println("Invalid Coinbase txn")
		return false
	}

	return true
}

/*
 * More intensive txn validation.
 */
func (c *Client) VerifyTxn(txn Txn, mainTxns, sideTxns map[SHA256Sum]Output, mainPimgs, sidePimgs map[SHA256Sum]struct{}) bool {
	// Check that maps are all nil or all non-nil
	forking := false
	if mainTxns != nil &&
		mainPimgs != nil &&
		sideTxns != nil &&
		sidePimgs != nil {

		forking = true
	} else if !(mainTxns == nil &&
		mainPimgs == nil &&
		sideTxns == nil &&
		sidePimgs == nil) {

		panic("All maps should be nil or non-nil")
	}

	// Check for preimage
	pimg := Hash(txn.Sig.Preimage.Bytes())
	found := c.GetPreimage(pimg)

	if forking {
		_, mainok := mainPimgs[pimg]
		if found || mainok {
			return false
		}
	} else if found {
		return false
	}

	// Get inputs
	inputs := []Output{}
	for _, inp := range txn.Body.Inputs {
		output, err := c.FindOutput(inp)
		if err != nil {
			log.Println("Could not load txn")
			return false
		}

		_, err = c.MapToBlock(inp)

		// Check main forks for output
		if forking {
			_, mainok := mainTxns[inp]
			if err != nil || mainok {
				return false
			}

		} else if err != nil {
			log.Println("No map:", err)
			return false
		}

		inputs = append(inputs, *output)
	}

	// Get Public Keys and commitments
	pks, ics := []ECCPoint{}, []ECCPoint{}
	for _, inp := range inputs {
		pks = append(pks, inp.DestKey)
		ics = append(ics, inp.Commit.ECCPoint)
	}

	if !txn.VerifyOZRS(pks, ics) {
		return false
	}

	for _, output := range txn.Body.Outputs {
		if !output.Commit.RangeProof.Verify() {
			return false
		}
	}

	return true
}

/*
 *More intensive coinbase validations.
 */
func (c *Client) VerifyCoinbaseTxn(txn Txn, coinbase uint64) bool {
	coinbaseBytes := UIntBytes(coinbase)
	cx, cy := CURVE.Params().ScalarMult(H.X, H.Y, coinbaseBytes)
	cy.Neg(cy)

	commit := txn.Body.Outputs[0].Commit
	cx, cy = CURVE.Params().Add(commit.X, commit.Y, cx, cy)

	zero := &big.Int{}

	return zero.Cmp(cx) == 0 && zero.Cmp(cy) == 0
}

/*
 * The json bytes to be signed.
 */
func (txn Txn) BodyJson() []byte {
	b, err := json.Marshal(txn.Body)
	if err != nil {
		log.Println(err)
		panic("Could not marshal txn body")
	}

	return b
}

/*
 * Full json for txn.
 */
func (txn Txn) Json() []byte {
	b, err := json.Marshal(txn)
	if err != nil {
		log.Println(err)
		panic("Could not marshal txn")
	}

	return b
}

/*
 * Hash of full txn json.
 */
func (txn Txn) Hash() SHA256Sum {
	return Hash(txn.Json())
}

/*
 * Computes the preimage of a public key, H_p(pk), and multiplies it by the
 * secret key. If `sk` is nil, the base point is simply returned.
 */
func Preimage(pk ECCPoint, sk *big.Int) ECCPoint {
	hp := HashToPt(pk.Bytes())
	if sk != nil {
		hp.X, hp.Y = CURVE.Params().ScalarMult(hp.X, hp.Y, sk.Bytes())
	}

	return hp
}

/*
 * Sends the txn all peers.
 */
func (c *Client) BcastTxn(hash SHA256Sum) error {
	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		go c.sendBcast("GossipCore.BcastTxnRPC", address, hash)
	}
	iter.Release()

	return iter.Error()
}

/*
 * Tries to load a txn from local storage. If this fails, the txn is fetched
 * from `req.Address`.
 */
func (c *Client) LoadOrFetchTxn(req HashMsg) (*Txn, error) {
	txn, err := c.LoadTxn(req.Hash)
	if err != nil {
		txn, err = c.FetchTxn(req.Hash, req.Address)
		if err != nil {
			log.Println("FETCH TXN FAILED")
			return nil, err
		}
	}

	if txn == nil {
		return nil, errors.New("Unable to load txn")
	}

	log.Println("Txn fetched successfully")

	return txn, nil
}

/*
 * Tries to load a txn from local storage.
 */
func (c *Client) LoadTxn(hash SHA256Sum) (*Txn, error) {
	txn, err := c.GetTxnPool(hash)
	if err == nil {
		return txn, nil
	}

	if c.Type != BLOCKCHAIN_CLIENT {
		return nil, errors.New("Not BLOCKCHAIN_CLIENT")
	}

	// Lookup mapping
	blockHash, err := c.MapToBlock(hash)
	if err != nil {
		return nil, err
	}

	// Load block
	block, err := c.GetBlock(blockHash)
	if err != nil {
		return nil, err
	}

	// Iterate though transactions to find preimage
	for _, t := range block.Txns {
		if hash == Hash(t.Sig.Preimage.Bytes()) {
			*txn = t
			return txn, nil
		}
	}

	return nil, errors.New("Could not load txn")
}
