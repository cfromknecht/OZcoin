package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

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
 * Less intensive validations.
 */
func (c *Client) PrevalidBlock(b Block) bool {
	if b.Txns == nil || len(b.Txns) == 0 {
		log.Println("Txns nil or len 0")
		return false
	}

	if !c.ValidTxns(b) {
		log.Println("Invalid txns")
		return false
	}

	if !b.VerifyMerkleHash() {
		log.Println("Merkle hash failed")
		return false
	}

	return true
}

/*
 * More intensive validations.
 */
func (c *Client) PostValidBlock(b Block, mainPath, sidePath []SHA256Sum) bool {
	if !c.ValidDifficulty(b) {
		return false
	}

	// Check median of last 11 blocks

	if !c.VerifyTxns(b, mainPath, sidePath) {
		return false
	}

	return true
}

/*
 * Computes merkle hash and compares with block header.
 */
func (b Block) VerifyMerkleHash() bool {
	existing := b.Header.MerkleRoot
	computed := b.MerkleHash()
	return computed == existing
}

/*
 * Computes merkle hash of txns
 */
func (b Block) MerkleHash() SHA256Sum {
	// Calculate number of initial slots
	numSlots := 1
	for numSlots < len(b.Txns) {
		numSlots *= 2
	}

	// Fill in slots
	slots := make([]SHA256Sum, numSlots)
	for i := 0; i < len(slots); i++ {
		if i < len(b.Txns) {
			slots[i] = Hash(b.Txns[i].Json())
		} else {
			slots[i] = SHA256Sum{}
		}
	}

	// Combine and reduce
	for len(slots) > 1 {
		numNewSlots := len(slots) / 2
		newSlots := make([]SHA256Sum, numNewSlots)
		for i := 0; i < numNewSlots; i++ {
			bytes := []byte{}
			bytes = append(bytes, slots[2*i][:]...)
			bytes = append(bytes, slots[2*i+1][:]...)
			newSlots[i] = Hash(bytes)
		}
		slots = slots[:numNewSlots]
	}

	return slots[0]
}

/*
 * Writes block depending on client type
 */
func (c *Client) WriteBlock(b Block) error {
	if c.Type == BLOCKCHAIN_CLIENT {
		log.Println("Writing block")
		err := c.PutBlock(b)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	// Build batched writes
	pimgBatch := &db.Batch{}
	txnPoolBatch := &db.Batch{}
	for i, txn := range b.Txns {
		// Only preimages for non-coinbase txns
		if i != 0 {
			pimgHash := Hash(txn.Sig.Preimage.Bytes()).Bytes()
			pimgBatch.Put(pimgHash, pimgHash)
			log.Println("Deleteing pimg from txn pool")
			txnPoolBatch.Delete(pimgHash)
		}
	}

	// Write preimages
	err := c.dbm.pimgDB.Write(pimgBatch, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	// Write txns
	err = c.PutMapToBlock(b)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println("Writing to txn pool batch")
	// Write txn pool
	err = c.dbm.txnPoolDB.Write(txnPoolBatch, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println("Write block success")

	return nil
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
