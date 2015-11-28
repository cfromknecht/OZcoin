package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"errors"
	"log"
)

/*
 * Initiates a recursive call to resolve the incoming block.  The block is
 * either added to the main chain, a side chain, the orphan database, or
 * discard.  Each effects of `AddOrOrphan` are made atomic by waiting for the a
 * SIGNAL on startChan.
 */

func (c *Client) AddOrOrphan(req HashMsg, startChan chan struct{}, doneChan chan SHA256Sum) {
	// Wait to make chain operations atomic
	_ = <-startChan

	// Resolve block
	success, err := c.AddOrOrphanHelper(req)
	if err != nil {
		log.Println("AddOrOrphanHelper failed:", err)
	}

	log.Println("Block resolution successful?:", success)
	// Broadcast transaction if successful
	if success {
		log.Println("Broadcasting block")
		err := c.BcastBlock(req.Hash)
		if err != nil {
			log.Println(err)
		}
	}

	// Signal completion
	doneChan <- req.Hash
}

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

func (c *Client) AddOrOrphanHelper(req HashMsg) (bool, error) {
	// Load block or fetch block
	block, err := c.LoadOrFetchBlock(req)
	if err != nil {
		return false, errors.New("Unable to load previous block")
	}
	header := block.Header

	if !ValidHeader(header) {
		return false, nil
	}

	if !c.PrevalidBlock(*block) {
		return false, nil
	}

	prevHash := header.PrevHash

	// Extending current chain
	if prevHash == c.LastHeader.Hash() || (prevHash == SHA256Sum{}) {
		return c.ExtendMainChain(header, block)
	}

	// Load parent block from main database
	_, err = c.GetHeader(prevHash)
	if err == nil {
		return c.ExtendSideChain(header, block)
	}

	// Load previous header from sidechain database
	_, err = c.GetSideHeader(prevHash)
	if err == nil {
		return c.ExtendSideChain(header, block)
	}

	// Recursively try to adopt orphan chain
	newReq := req.NewHash(prevHash)
	success, err := c.AddOrOrphanHelper(newReq)
	if err != nil {
		return false, err
	}

	// If adoption failed, cache header and block in orphan databases
	if !success {
		err = c.PutOrphanHeader(header)
		if err != nil {
			return false, err
		}

		if c.Type == BLOCKCHAIN_CLIENT {
			err = c.PutOrphanBlock(*block)
			if err != nil {
				return false, err
			}
		}

		return false, nil
	}

	// Load previous header from main database
	_, err = c.GetHeader(prevHash)
	if err == nil {
		return c.ExtendMainChain(header, block)
	}

	// Load previous header from sidechain database
	_, err = c.GetSideHeader(prevHash)
	if err != nil {
		return c.ExtendSideChain(header, block)
	}

	// Otherwise chain was not successfully adopted
	panic("THIS SHOULD NEVER HAPPEN")
}

func (c *Client) ExtendMainChain(header BlockHeader, block *Block) (bool, error) {
	if !c.PostValidBlock(*block, nil, nil) {
		log.Println("Block is not post valid")
		return false, nil
	}

	// Add to valid header database
	err := c.PutHeader(header)
	if err != nil {
		return false, err
	}

	// Write block if blockchain
	err = c.WriteBlock(*block)
	if err != nil {
		return false, err
	}

	if c.UpdateWallet {
		b := *block
		go func() {
			err := c.Wallet.NewBlock(b)
			if err != nil {
				log.Println(err)
			}
		}()
	}

	c.LastHeader = header

	return true, nil
}

func (c *Client) ExtendSideChain(header BlockHeader, block *Block) (bool, error) {
	// Get fork paths
	mainPath, sidePath, err := c.FindForkPaths(header.Hash())
	if err != nil {
		return false, err
	}

	// Get difficulties of main and side chains
	mainDiff, err := c.MainChainTotalDifficulty()
	if err != nil {
		return false, err
	}
	sideDiff, err := c.SideChainTotalDifficulty(header.PrevHash)
	if err != nil {
		return false, err
	}
	sideDiff += header.Difficulty

	// If sidechain is smaller, validate and save blocks
	if mainDiff > sideDiff {
		if !c.PostValidBlock(*block, mainPath, sidePath) {
			return false, nil
		}

		err := c.PutSideHeader(header)
		if err != nil {
			return false, err
		}

		// Write sidechain block if blockchain
		err = c.PutSideBlock(*block)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	err = c.SwapMainFork(mainPath, sidePath)
	if err != nil {
		return false, err
	}

	c.LastHeader = header

	return true, nil
}

func (c *Client) SwapMainFork(mainPath, sidePath []SHA256Sum) error {
	headerBatch := &db.Batch{}
	sideHeaderBatch := &db.Batch{}

	// Remove main chain headers
	for _, hash := range mainPath {
		h, err := c.GetHeader(hash)
		if err != nil {
			return err
		}

		if h == nil {
			log.Println("Header should be in main chain")
			panic("Header should be in main chain")
		}

		headerBatch.Delete(hash[:])
		sideHeaderBatch.Put(hash[:], h.Json())
	}

	// Add sidechain headers
	for _, hash := range sidePath {
		h, err := c.GetHeader(hash)
		if err != nil {
			return err
		}

		if h == nil {
			log.Println("Header should be in side chain")
			panic("Header should be in side chain")
		}

		headerBatch.Put(hash[:], h.Json())
		sideHeaderBatch.Delete(hash[:])
	}

	// Commit batches
	err := c.dbm.headerDB.Write(headerBatch, nil)
	if err != nil {
		return err
	}
	err = c.dbm.sideHeaderDB.Write(headerBatch, nil)
	if err != nil {
		return err
	}

	blockBatch := &db.Batch{}
	sideBlockBatch := &db.Batch{}
	pimgBatch := &db.Batch{}
	txnPoolBatch := &db.Batch{}

	// Remove main chain blocks
	deleteBlocks := []Block{}
	for _, hash := range mainPath {
		// Retrieve block
		b, err := c.GetBlock(hash)
		if err != nil {
			log.Println("Block should be in main chain:", err)
			panic("Block should be in main chain")
		}

		deleteBlocks = append(deleteBlocks, *b)

		blockBatch.Delete(hash.Bytes())
		sideBlockBatch.Put(hash.Bytes(), b.Json())

		// Add deletions to batch
		for _, txn := range b.Txns {
			pimgHash := Hash(txn.Sig.Preimage.Bytes()).Bytes()
			pimgBatch.Delete(pimgHash)
			// Only replace if blockchain client
			if c.Type == BLOCKCHAIN_CLIENT {
				txnPoolBatch.Put(pimgHash, txn.Json())
			}
		}
	}

	// Add side chain blocks
	addBlocks := []Block{}
	for _, hash := range sidePath {
		// Retrieve block
		b, err := c.GetBlock(hash)
		if err != nil {
			log.Println("Block should be in main chain:", err)
			panic("Block should be in main chain")
		}

		addBlocks = append(addBlocks, *b)

		blockBatch.Put(hash[:], b.Json())
		sideBlockBatch.Delete(hash[:])

		// Add deletions to batch
		for _, txn := range b.Txns {
			pimgHash := Hash(txn.Sig.Preimage.Bytes()).Bytes()
			txnPoolBatch.Delete(pimgHash)
			pimgBatch.Put(pimgHash, pimgHash)
		}
	}

	log.Println("Writing preimages")
	// Write preimages
	err = c.dbm.pimgDB.Write(pimgBatch, nil)
	if err != nil {
		return err
	}

	log.Println("Writing txn pool")
	err = c.dbm.txnPoolDB.Write(txnPoolBatch, nil)
	if err != nil {
		return err
	}

	// Commit blocks if blockchain client
	if c.Type == BLOCKCHAIN_CLIENT {
		err = c.dbm.blockDB.Write(blockBatch, nil)
		if err != nil {
			return err
		}
		err = c.dbm.sideBlockDB.Write(sideBlockBatch, nil)
		if err != nil {
			return err
		}
	}

	// Teardown main fork maps
	for _, hash := range mainPath {
		block, err := c.FindBlock(hash)
		if err != nil {
			return err
		}

		err = c.DeleteMapToBlock(*block)
		if err != nil {
			return err
		}
	}

	// Build side fork maps
	for _, hash := range sidePath {
		block, err := c.FindBlock(hash)
		if err != nil {
			return err
		}

		err = c.PutMapToBlock(*block)
		if err != nil {
			return err
		}
	}

	if c.UpdateWallet {
		go func() {
			for _, b := range deleteBlocks {
				err := c.Wallet.DeleteBlock(b)
				if err != nil {
					log.Println("Could not delete block:", err)
				}
			}
			for _, b := range addBlocks {
				err := c.Wallet.NewBlock(b)
				if err != nil {
					log.Println("Could not add block:", err)
				}
			}
		}()
	}

	return nil
}

func (c *Client) FindForkPaths(sidehash SHA256Sum) ([]SHA256Sum, []SHA256Sum, error) {
	prevHeader, err := c.GetSideHeader(sidehash)
	if err != nil {
		return nil, nil, err
	}

	prevHash := prevHeader.PrevHash
	sideHashes := []SHA256Sum{prevHash}
	for {
		prevHeader, err = c.GetHeader(prevHash)
		if err != nil {
			return nil, nil, err
		}

		if prevHeader != nil {
			sideHashes = append(sideHashes, prevHash)
			break
		}

		prevHeader, err = c.GetSideHeader(prevHash)
		if err != nil {
			return nil, nil, err
		}

		if prevHeader == nil {
			return nil, nil, nil
		}

		sideHashes = append(sideHashes, prevHash)
	}

	forkHash := sideHashes[len(sideHashes)-1]
	prevHash = c.LastHeader.Hash()
	mainHashes := []SHA256Sum{}
	for {
		prevHeader, err = c.GetHeader(prevHash)
		if err != nil {
			return nil, nil, err
		}

		if prevHeader == nil {
			return nil, nil, nil
		}

		if prevHash == forkHash {
			return mainHashes, sideHashes, nil
		}

		mainHashes = append(mainHashes, prevHash)
	}
}

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

func (c *Client) FindBlock(hash SHA256Sum) (*Block, error) {
	log.Println("Loading block")
	block, err := c.LoadBlock(hash)
	if err == nil {
		return block, nil
	}

	log.Println("Iterate peers")
	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		log.Println("Fetch block from", address)
		block, err := c.FetchBlock(hash, address)
		if err == nil {
			return block, nil
		}
	}
	iter.Release()

	return nil, iter.Error()
}

func (c *Client) AddToTxnPool(req HashMsg, startChan chan struct{}, doneChan chan SHA256Sum) {
	// Wait to make chain operations atomic
	_ = <-startChan

	success, err := c.AddToTxnPoolHelper(req)
	if err != nil {
		log.Println(err)
	}

	if success {
		err = c.BcastTxn(req.Hash)
		if err != nil {
			log.Println(err)
		}
	}

	// Signal completion
	doneChan <- req.Hash
}

func (c *Client) AddToTxnPoolHelper(req HashMsg) (bool, error) {
	txn, err := c.LoadOrFetchTxn(req)
	if err != nil {
		return false, err
	}

	if !ValidTxn(*txn) && !ValidCoinbaseTxn(*txn) {
		return false, errors.New("Invalid txn")
	}

	err = c.PutTxnPool(*txn)
	if err != nil {
		return false, err
	}

	return true, nil

}
