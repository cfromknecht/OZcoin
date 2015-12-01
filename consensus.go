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

/*
 * Primary consensus algorithm
 *
 * Recursively tries to adopt a given block by:
 *   1. Load block
 *   2. Extends main or side chain? Extend and done
 *   3. Extends Orphan? AddOrOrphanHelper for PrevHash
 *   4. If adoption failed? Save block and done
 *   5. Now extends main or side chain? Extend and done
 */
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

/*
 * Adds the block to the main chain database.
 */
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

/*
 * Adds a block to an existing side chain. If this new chain is heavier than the
 * main chain, the main chain is swapped for the side chain.
 */
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

/*
 * Replaces the main chain with a side chain.
 */
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

/*
 * Computes the fork hash for where the side chain meets the main chain. Then
 * returns a list of hashes in each fork after the fork hash.
 */
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

/*
 * Wraps the AddToTxnPoolHelper to be atomic and broadcast on success.
 */
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

/*
 * Checks that a txn is Valid and then adds it to Txn Pool.
 */
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
