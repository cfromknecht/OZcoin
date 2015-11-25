package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"errors"
	"log"
)

func (c *Client) PrevalidBlock(b Block) bool {
	if b.Txns == nil || len(b.Txns) == 0 {
		return false
	}

	if !c.ValidTxns(b) {
		return false
	}

	if !c.VerifyMerkleHash(b) {
		return false
	}

	return true
}

func (c *Client) ValidDifficulty(b Block) bool {
	if b.Header.SeqNum <= 2016 {
		return b.Header.Difficulty == INITIAL_DIFFICULTY
	}

	last, err := c.NthAncestorHeader(b.Header.PrevHash, 2016)
	if err != nil {
		log.Println(err)
		return false
	}

	currTime := b.Header.Time.UnixNano()
	lastTime := last.Time.UnixNano()

	actualTime := currTime - lastTime
	oldTarget := last.Difficulty

	newTarget := oldTarget * uint64(actualTime) / TWO_WEEKS_SEC

	return uint64(b.Header.Difficulty) == newTarget
}

func (c *Client) PostValidBlock(b Block) bool {
	if !c.ValidDifficulty(b) {
		return false
	}

	// Check median of last 11 blocks

	return true
}

func (c *Client) NthAncestorHeader(hash SHA256Sum, n int) (*BlockHeader, error) {
	prevHash := hash
	var prevHeader *BlockHeader
	for i := 0; i < n; i++ {
		// Load previous header from main database
		prevHeader, err := c.GetHeader(prevHash)
		if err != nil {
			return nil, err
		}

		// Use as previous hash
		if prevHeader == nil {
			prevHash = prevHeader.PrevHash
			continue
		}

		// Load previous header from sidechain database
		prevHeader, err = c.GetSideHeader(prevHash)
		if err != nil {
			return nil, err
		}

		// Use as previous hash
		if prevHeader != nil {
			prevHash = prevHeader.PrevHash
			continue
		}

		return prevHeader, nil
	}

	return prevHeader, nil
}

func (c *Client) VerifyMerkleHash(b Block) bool {
	return true
}

func (c *Client) WriteBlock(b Block) error {
	err := c.PutBlock(b)
	if err != nil {
		log.Println(err)
		return err
	}

	// Open transaction database
	utxnDB := c.OpenUTxnDB()
	defer utxnDB.Close()

	// Open preimage database
	pimgDB := c.OpenPreimageDB()
	defer pimgDB.Close()

	// Build batched writes
	txnBatch := &db.Batch{}
	pimgBatch := &db.Batch{}
	for _, txn := range b.Txns {
		txnHash := txn.Hash()
		txnBatch.Put(txnHash[:], txn.Json())
		pimgBatch.Put(txn.Sig.Preimage.Bytes(), txn.Sig.Preimage.Bytes())
	}

	// Write txns
	err = utxnDB.Write(txnBatch, nil)
	if err != nil {
		log.Println(err)
		return err
	}
	// Write preimages
	err = pimgDB.Write(pimgBatch, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func (c *Client) ValidTxns(b Block) bool {
	// Check validity of each transaction
	for i, txn := range b.Txns {
		if i == 0 {
			if !c.ValidCoinbaseTxn(txn) {
				log.Println("Invalid GenerationTxn")
				return false
			}
		} else {
			if !c.ValidTxn(txn) {
				log.Println("Invalid Txn")
				return false
			}
		}
	}

	// Check sum of fees = generation txn fees

	return true
}

func (c *Client) ValidTxn(txn Txn) bool {
	if txn.Body.Inputs == nil || len(txn.Body.Inputs) != TXN_NUM_INPUTS {
		log.Println("Invalid number of txn inputs")
		return false
	}

	if txn.Body.Outputs == nil || len(txn.Body.Outputs) != TXN_NUM_OUTPUTS {
		log.Println("Invalid number of txn outputs")
		return false
	}

	if len(txn.Json()) >= MAX_BLOCK_SIZE {
		return false
	}

	for _, output := range txn.Body.Outputs {
		if output.PublicKey.Empty() ||
			output.DestKey.Empty() ||
			output.BlindSeed.Empty() ||
			output.Commit.Empty() {
			return false
		}
	}

	utxns, err := c.fetchInputTxns(txn.Body.Inputs)
	if err != nil {
		log.Println("Error fetching input txns from database")
		return false
	}

	pks, ics := []ECCPoint{}, []Commitment{}
	for _, utxn := range utxns {
		pks = append(pks, utxn.PublicKey)
		ics = append(ics, utxn.Commit)
	}

	return txn.VerifyOZRS(pks, ics)
}

func (c *Client) ValidCoinbaseTxn(txn Txn) bool {
	if txn.Body.Inputs == nil || len(txn.Body.Inputs) != 1 {
		log.Println("Invalid number of txn inputs")
		return false
	}

	input := txn.Body.Inputs[0]
	if (input.Index != 2) || (input.Hash != SHA256Sum{}) {
		log.Println("Invalid coinbase inputs")
		return false
	}

	if txn.Body.Outputs == nil || len(txn.Body.Outputs) != 1 {
		log.Println("Invalid number of txn outputs")
		return false
	}

	if len(txn.Json()) >= MAX_BLOCK_SIZE {
		return false
	}

	output := txn.Body.Outputs[0]

	return !output.PublicKey.Empty() &&
		!output.DestKey.Empty() &&
		!output.BlindSeed.Empty() &&
		!output.Commit.Empty()
}

func (c *Client) fetchInputTxns(inputs []Input) ([]Output, error) {
	utxns := []Output{}
	for _, input := range inputs {
		if input.Index > 1 {
			msg := "Cannot allow txn input index greater than 1"
			log.Println(msg)
			return nil, errors.New(msg)
		}

		otpt, err := c.GetUTxn(input.Hash, input.Index)
		if err != nil {
			return nil, err
		}

		if otpt == nil {
			msg := "Output not found"
			log.Println(msg)
			return nil, errors.New(msg)
		}

		utxns = append(utxns, *otpt)
	}

	return utxns, nil
}
