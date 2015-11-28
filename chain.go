package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"errors"
	"log"
	"math/big"
)

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

func (c *Client) MainChainTotalDifficulty() (uint64, error) {
	total := uint64(0)
	prevHash := c.LastHeader.Hash()
	for {
		header, err := c.GetHeader(prevHash)
		if err != nil {
			return 0, err
		}

		if header == nil {
			return total, nil
		}

		total += header.Difficulty
		prevHash = header.PrevHash
	}
}

func (c *Client) SideChainTotalDifficulty(hash SHA256Sum) (uint64, error) {
	total := uint64(0)
	prevHash := hash
	for {
		header, err := c.GetHeader(prevHash)
		if err != nil {
			return 0, err
		}

		if header != nil {
			total += header.Difficulty
			prevHash = header.PrevHash
			continue
		}

		header, err = c.GetSideHeader(prevHash)
		if err != nil {
			return 0, err
		}

		if header != nil {
			total += header.Difficulty
			prevHash = header.PrevHash
		}

		return total, nil
	}
}

func (c *Client) ComputeDifficulty(b Block) uint64 {
	if b.Header.SeqNum <= DIFFICULTY_SPACING {
		return INITIAL_DIFFICULTY
	}

	last, err := c.NthAncestorHeader(b.Header.PrevHash, DIFFICULTY_SPACING)
	if err != nil {
		log.Println(err)
		return uint64(1) << 63
	}

	currTime := b.Header.Time.Unix()
	lastTime := last.Time.Unix()

	actualTime := currTime - lastTime
	oldTarget := last.Difficulty

	newTarget := uint64(float64(oldTarget) * (float64(TWO_WEEKS_SEC) /
		float64(actualTime)))

	lowerBound := uint64(0)
	if oldTarget > 0 {
		lowerBound = oldTarget - 1
	}
	upperBound := oldTarget + 1

	if newTarget < lowerBound {
		newTarget = lowerBound
	}
	if newTarget > upperBound {
		newTarget = upperBound
	}

	return newTarget
}

func (c *Client) ValidDifficulty(b Block) bool {

	return uint64(b.Header.Difficulty) == c.ComputeDifficulty(b)
}

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

func (c *Client) NthAncestorHeader(hash SHA256Sum, n int) (*BlockHeader, error) {
	prevHash := hash
	prevHeader := &BlockHeader{}
	depth := 0
	for depth < n {
		// Load previous header from main database
		header, err := c.GetHeader(prevHash)
		if err != nil {
			// Load previous header from sidechain database
			header, err = c.GetSideHeader(prevHash)
			if err != nil {
				log.Println("Could not find nth ancestor", err)
				return nil, err
			}
		}

		prevHash = header.PrevHash
		*prevHeader = *header
		depth += 1
	}

	return prevHeader, nil
}

func (b Block) VerifyMerkleHash() bool {
	existing := b.Header.MerkleRoot
	computed := b.MerkleHash()
	return computed == existing
}

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

func (c *Client) ForkTxnsAndPreimages(path []SHA256Sum) (map[SHA256Sum]Output, map[SHA256Sum]struct{}, error) {

	txns := make(map[SHA256Sum]Output)
	pimgs := make(map[SHA256Sum]struct{})

	for _, hash := range path {
		b, err := c.GetBlock(hash)
		if err != nil {
			return nil, nil, err
		}

		if b == nil {
			b, err = c.GetSideBlock(hash)
			if err != nil {
				return nil, nil, err
			}
		}

		if b == nil {
			return nil, nil, errors.New("Path does not exist")
		}

		for _, txn := range b.Txns {
			// Add preimage
			pimgHash := Hash(txn.Sig.Preimage.Bytes())
			pimgs[pimgHash] = SIGNAL

			// Add txn outputs
			for _, output := range txn.Body.Outputs {
				txns[output.Hash()] = output
			}
		}
	}

	return txns, pimgs, nil
}

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
	log.Println("Coinbase:", coinbase)

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

func (c *Client) VerifyCoinbaseTxn(txn Txn, coinbase uint64) bool {
	coinbaseBytes := UIntBytes(coinbase)
	cx, cy := CURVE.Params().ScalarMult(H.X, H.Y, coinbaseBytes)
	cy.Neg(cy)

	commit := txn.Body.Outputs[0].Commit
	cx, cy = CURVE.Params().Add(commit.X, commit.Y, cx, cy)

	zero := &big.Int{}

	return zero.Cmp(cx) == 0 && zero.Cmp(cy) == 0
}

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

func CoinbaseValue(seqnum uint64) uint64 {
	return (50 * 100000000) >> (seqnum / 21000)
}

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

	// Check sum of fees = generation txn fees

	return true
}

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
