package ozcoin

import (
	"encoding/json"
	"log"
	"time"
)

const (
	HASH_GENESIS_BLOCK = "hwm5tjMI9ZsvG2VwNKeMtJq7PDLDvPM/hLFQ2VYdE88="
)

var (
	CURRENT_DIFFICULTY   = uint64(16)
	CURRENT_BLOCK_REWARD = uint64(50000000)
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
	Header      BlockHeader `json:"header"`
	CoinbaseTxn Txn         `json:"coinbase_txn"`
	Txns        []Txn       `json:"txns"`
}

func (b Block) Json() []byte {
	blockJson, err := json.Marshal(b)
	if err != nil {
		log.Println(err)
		panic("Unable to marshal block")
	}

	return blockJson
}

func NewBlock(prev BlockHeader, minerAddress SHA256Sum) Block {
	return Block{
		Header: BlockHeader{
			SeqNum:     prev.SeqNum + 1,
			PrevHash:   prev.Hash(),
			MerkleRoot: SHA256Sum{},
			Time:       time.Now(),
			Difficulty: CURRENT_DIFFICULTY,
			Nonce:      0,
		},
		CoinbaseTxn: Txn{},
		Txns:        []Txn{},
	}
}

func GenesisBlock() Block {
	return Block{
		Header: BlockHeader{
			SeqNum:     0,
			PrevHash:   SHA256Sum{},
			MerkleRoot: SHA256Sum{},
			Time:       time.Now(),
			Difficulty: CURRENT_DIFFICULTY,
			Nonce:      0,
		},
		CoinbaseTxn: Txn{},
		Txns:        []Txn{},
	}
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

/*
func Mine() {
	bc := NewBlockchain()

	g := GenesisBlock()
	for !g.Header.ValidPoW() {
		g.Header.Nonce += 1
	}
	fmt.Println(fmt.Sprintf("Gen: %v", string(g.Json())))

	fromKey := crypto.NewKey()
	toKey := crypto.NewKey()

	online := crypto.NewKey()
	offline := crypto.NewKey()

	//for {
	b := NewBlock(bc.LastHeader, crypto.Address(fromKey.PublicKey))

	// Create and sign txn
	txn := NewPaymentTxn(fromKey.PublicKey, crypto.Address(toKey.PublicKey), 10)
	txn.Inputs[0].Signature = crypto.Sign("", fromKey)
	b.Txns = append(b.Txns, txn)

	identity, err := NewIdentity("certcoin.net", "")
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	// Create and sign registration txn
	rtxn := NewRegisterTxn(online, offline, fromKey.PublicKey, identity)
	rtxn.Inputs[2].Signature = crypto.Sign("", fromKey)
	b.Txns = append(b.Txns, rtxn)

	newOnline := crypto.NewKey()
	// Create and sign update txn
	utxn := NewUpdateTxn(newOnline, offline, fromKey.PublicKey, identity)
	utxn.Inputs[1].Signature = crypto.Sign("", offline)
	utxn.Inputs[2].Signature = crypto.Sign("", fromKey)
	b.Txns = append(b.Txns, utxn)

	// Create and sign revoke txn
	vtxn := NewRevokeTxn(newOnline, offline, fromKey.PublicKey, identity)
	b.Txns = append(b.Txns, vtxn)

	for !b.Header.ValidPoW() {
		b.Header.Nonce += 1
	}

	//fmt.Println(fmt.Sprintf("%v", b.Json()))
	//fmt.Println(b.Header.Hash())

	if bc.ValidBlock(b) {
		err = bc.WriteBlock(b)
		if err != nil {
			log.Println(err)
			panic("Unable to save block")
		} else {
			fmt.Println("Saved block successfully")
		}
	} else {
		fmt.Println("Invalid Block")
	}
	//}
}
*/
