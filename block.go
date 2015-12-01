package ozcoin

import (
	"encoding/json"
	"errors"
	"log"
	"time"
)

/*
 * BlockHeader
 *
 * Records information for properly sequencing and verifying blocks.
 */
type BlockHeader struct {
	SeqNum     uint64    `json:"seq_num"`
	PrevHash   SHA256Sum `json:"prev_hash"`
	MerkleRoot SHA256Sum `json:"merkle_root"`
	Time       time.Time `json:"time"`
	Difficulty uint64    `json:"difficulty"`
	Nonce      uint64    `json:"nonce:"`
}

/*
 * Checks PoW, Time, and Genesis Hash.
 */
func ValidHeader(header BlockHeader) bool {
	if !header.ValidPoW() {
		return false
	}

	if header.Time.Add(2 * time.Hour).Before(time.Now()) {
		return false
	}

	if (header.SeqNum == 0) && (header.PrevHash != SHA256Sum{}) {
		return false
	}

	return true
}

/*
 * Checks that a block header's hash is lower than the claimed difficulty.
 */
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
 * Block
 *
 * Stores a block header along with a list of txns.
 */
type Block struct {
	Header BlockHeader `json:"header"`
	Txns   []Txn       `json:"txns"`
}

/*
 * The blocks's json bytes.
 */
func (b Block) Json() []byte {
	blockJson, err := json.Marshal(b)
	if err != nil {
		log.Println(err)
		panic("Unable to marshal block")
	}

	return blockJson
}

/*
 * Creates a new block to mined that extends the current `LastHeader`.  The
 * coinbase txn is sent to the address provided.
 */
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

/*
 * Mines the Genesis block and sends the coinbase to `address`.
 */
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

/*
 * The json bytes for the block header.
 */
func (b BlockHeader) Json() []byte {
	headerJson, err := json.Marshal(b)
	if err != nil {
		log.Println(err)
		panic("Unable to marshal block header")
	}

	return headerJson
}

/*
 * The hash to end all hashes.
 */
func (b BlockHeader) Hash() SHA256Sum {
	return Hash(b.Json())
}

/*
 * Tries to load a block from local storage. If this fails, the block is fetched
 * from `req.Address`.
 */
func (c *Client) LoadOrFetchBlock(req HashMsg) (*Block, error) {
	log.Println("Requesting block from:", req.Address)

	// If this failed, request block
	block, err := c.LoadBlock(req.Hash)
	if err != nil {
		block, err = c.FetchBlock(req.Hash, req.Address)
		if err != nil {
			log.Println("FETCH BLOCK FAILED:", err)
			return nil, err
		}
	}

	if block == nil {
		return nil, errors.New("Unable to load block")

	}
	log.Println("LOADED BLOCK:", *block)

	return block, nil
}

/*
 * Tries to load a block from local storage.
 */
func (c *Client) LoadBlock(hash SHA256Sum) (*Block, error) {
	if c.Type != BLOCKCHAIN_CLIENT {
		return nil, errors.New("Not BLOCKCHAIN_CLIENT")
	}

	block, err := c.GetBlock(hash)
	if err != nil {
		block, err = c.GetSideBlock(hash)
		if err != nil {
			block, err = c.GetOrphanBlock(hash)
		}
	}

	return block, err
}

/*
 * Tries to load a block from local storage, otherwise iterates through peers
 * until one succeeds.
 */
func (c *Client) FindBlock(hash SHA256Sum) (*Block, error) {
	block, err := c.LoadBlock(hash)
	if err == nil {
		return block, nil
	}

	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		block, err := c.FetchBlock(hash, address)
		if err == nil {
			return block, nil
		}
	}
	iter.Release()

	return nil, iter.Error()
}

/*
 * Sends block to all peers.
 */
func (c *Client) BcastBlock(hash SHA256Sum) error {
	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		go c.sendBcast("GossipCore.BcastBlockRPC", address, hash)
	}
	iter.Release()

	return iter.Error()
}
