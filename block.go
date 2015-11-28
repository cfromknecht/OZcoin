package ozcoin

import (
	"encoding/json"
	"log"
	"math/big"
	"time"
)

const (
	HASH_GENESIS_BLOCK = "hwm5tjMI9ZsvG2VwNKeMtJq7PDLDvPM/hLFQ2VYdE88="
)

type BlockHeader struct {
	SeqNum     uint64    `json:"seq_num"`
	PrevHash   SHA256Sum `json:"prev_hash"`
	MerkleRoot SHA256Sum `json:"merkle_root"`
	Time       time.Time `json:"time"`
	Difficulty uint64    `json:"difficulty"`
	Nonce      uint64    `json:"nonce:"`
}

type Block struct {
	Header BlockHeader `json:"header"`
	Txns   []Txn       `json:"txns"`
}

func (b Block) Json() []byte {
	blockJson, err := json.Marshal(b)
	if err != nil {
		log.Println(err)
		panic("Unable to marshal block")
	}

	return blockJson
}

func (c *Client) NewBlock(prev BlockHeader, address WalletPublicKey) Block {
	seqNum := prev.SeqNum + 1

	// Gather txns and add fees
	txns := c.TxnsFromPool()
	fees := uint64(0)
	for _, t := range txns {
		fees += t.Body.Fee
	}

	// Create new coinbase commitment
	coinbaseTxn := NewCoinbaseTxn(address, seqNum, fees)

	block := Block{
		Header: BlockHeader{
			SeqNum:     seqNum,
			PrevHash:   prev.Hash(),
			MerkleRoot: SHA256Sum{},
			Time:       time.Now(),
			Difficulty: INITIAL_DIFFICULTY,
			Nonce:      0,
		},
		Txns: []Txn{
			coinbaseTxn,
		},
	}

	block.Txns = append(block.Txns, txns...)

	block.Header.MerkleRoot = block.MerkleHash()
	block.Header.Difficulty = c.ComputeDifficulty(block)

	return block
}

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

func GenesisBlock(address WalletPublicKey) Block {
	coinbaseTxn := NewCoinbaseTxn(address, 0, 0)

	b := Block{
		Header: BlockHeader{
			SeqNum:     0,
			PrevHash:   SHA256Sum{},
			MerkleRoot: SHA256Sum{},
			Time:       time.Now(),
			Difficulty: INITIAL_DIFFICULTY,
			Nonce:      0,
		},
		Txns: []Txn{
			coinbaseTxn,
		},
	}

	b.Header.MerkleRoot = b.MerkleHash()

	for !b.Header.ValidPoW() {
		b.Header.Nonce += 1
	}

	return b
}

func (b BlockHeader) Json() []byte {
	headerJson, err := json.Marshal(b)
	if err != nil {
		log.Println(err)
		panic("Unable to marshal block header")
	}

	return headerJson
}

func (b BlockHeader) Hash() SHA256Sum {
	return Hash(b.Json())
}

func (bh BlockHeader) ValidPoW() bool {
	difficulty := bh.Difficulty
	zeroBytes := difficulty / 8
	bitOffset := difficulty % 8

	h := bh.Hash()
	for i, c := range h {
		if uint64(i) < zeroBytes {
			if c != 0 {
				return false
			}
		} else {
			break
		}
	}

	c := h[zeroBytes]
	for j := 0; j < 8; j++ {
		if uint64(j) < bitOffset {
			if (c >> (7 - uint64(j)) & 1) != 0 {
				return false
			}
		} else {
			break
		}
	}

	return true
}
