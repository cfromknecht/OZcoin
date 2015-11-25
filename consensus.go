package ozcoin

import (
	"errors"
	"log"
)

func (c *Client) AddOrOrphan(req BcastRequest, doneChan chan SHA256Sum) {
	_, err := c.AddOrOrphanHelper(req)
	if err != nil {
		log.Println(err)
	}
	doneChan <- req.Hash
}

func (c *Client) LoadOrFetchHeaderAndBlock(req BcastRequest) (*BlockHeader, *Block, error) {
	var block *Block

	// Try to load header and block from valid database
	header, err := c.GetHeader(req.Hash)
	if err != nil {
		return nil, nil, err
	}
	if c.Type == BLOCKCHAIN_CLIENT {
		block, err = c.GetBlock(req.Hash)
		if err != nil {
			return nil, nil, err
		}
	}

	// Try to load header and block from sidechain database
	if header == nil {
		header, err = c.GetSideHeader(req.Hash)
		if err != nil {
			return nil, nil, err
		}
		if c.Type == BLOCKCHAIN_CLIENT {
			block, err = c.GetSideBlock(req.Hash)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	// Try to load header and block from orphan database
	if header == nil {
		header, err = c.GetOrphanHeader(req.Hash)
		if err != nil {
			return nil, nil, err
		}
		if c.Type == BLOCKCHAIN_CLIENT {
			block, err = c.GetOrphanBlock(req.Hash)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	// If both failed, request block
	if header == nil {
		header, block, err = c.FetchHeaderAndBlock(req)
		if err != nil {
			return nil, nil, err
		}
	}

	return header, block, nil
}

func (c *Client) AddOrOrphanHelper(req BcastRequest) (bool, error) {
	// Load block or fetch block
	header, block, err := c.LoadOrFetchHeaderAndBlock(req)
	if err != nil {
		return false, err
	}

	// Check that header and block are properly set, otherwise fail
	if header == nil ||
		(c.Type == BLOCKCHAIN_CLIENT && block == nil) {
		return false, errors.New("Unable to load header or block")
	}

	if !c.ValidHeader(*header) {
		return false, nil
	}

	if c.Type == BLOCKCHAIN_CLIENT {
		if !c.PrevalidBlock(*block) {
			return false, nil
		}
	}

	prevHash := header.PrevHash

	// Load previous header from main database
	prevHeader, err := c.GetHeader(prevHash)
	if err != nil {
		return false, err
	}

	// Create new sidechain and return
	if prevHeader != nil {
		return c.ValidateAndExtendChain(header, block)
	}

	// Load previous header from sidechain database
	prevHeader, err = c.GetSideHeader(prevHash)
	if err != nil {
		return false, err
	}

	// Extends sidechain and return
	if prevHeader != nil {
		return c.ValidateAndExtendSideChain(header, block)
	}

	// Load previous header from orphan database
	prevHeader, err = c.GetOrphanHeader(prevHash)
	if err != nil {
		return false, err
	}

	// Header was not found in orphan database, retrieve from source
	if prevHeader == nil {
		prevHeader, _, err = c.FetchHeaderAndBlock(req)
		if err != nil {
			return false, err
		}

	}

	// Check that previous header and block are properly set, otherwise fail
	if prevHeader == nil {
		return false, errors.New("Unable to load previous header")
	}

	// Recursively try to adopt orphan chain
	newReq := BcastRequest{
		RPCHeader: req.RPCHeader,
		Hash:      prevHash,
	}
	success, err := c.AddOrOrphanHelper(newReq)
	if err != nil {
		return false, err
	}

	// If adoption failed, cache header and block in orphan databases
	if !success {
		err = c.PutOrphanHeader(*header)
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
	prevHeader, err = c.GetHeader(prevHash)
	if err != nil {
		return false, err
	}

	// Create new sidechain and return
	if prevHeader != nil {
		return c.ValidateAndExtendChain(header, block)
	}

	// Load previous header from sidechain database
	prevHeader, err = c.GetSideHeader(prevHash)
	if err != nil {
		return false, err
	}

	// Extends sidechain and return
	if prevHeader != nil {
		return c.ValidateAndExtendSideChain(header, block)
	}

	// Otherwise chain was not successfully adopted
	return false, errors.New("THIS SHOULD NEVER HAPPEN")
}

func (c *Client) ValidateAndExtendChain(header *BlockHeader, block *Block) (bool, error) {
	if !c.PostValidBlock(*block) {
		return false, nil
	}

	// Add to valid header database
	err := c.PutHeader(*header)
	if err != nil {
		return false, err
	}

	// Write block if blockchain
	if c.Type == BLOCKCHAIN_CLIENT {
		err = c.WriteBlock(*block)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (c *Client) ValidateAndExtendSideChain(header *BlockHeader, block *Block) (bool, error) {
	if !c.PostValidBlock(*block) {
		return false, nil
	}

	// Extend side chain
	err := c.PutSideHeader(*header)
	if err != nil {
		return false, err
	}

	// Write sidechain block if blockchain
	if c.Type == BLOCKCHAIN_CLIENT {
		err = c.PutSideBlock(*block)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (c *Client) ValidateAndCreateChain(header *BlockHeader, block *Block) (bool, error) {
	if c.Type == BLOCKCHAIN_CLIENT {
		// Validate block first
	}

	// Add header to side chains
	err := c.PutSideHeader(*header)
	if err != nil {
		return false, err
	}

	// Add header to valid database
	err = c.PutHeader(*header)
	if err != nil {
		return false, err
	}

	// Write block if blockchain
	if c.Type == BLOCKCHAIN_CLIENT {
		err = c.WriteBlock(*block)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (s *Client) FetchHeaderAndBlock(req BcastRequest) (*BlockHeader, *Block, error) {
	var header *BlockHeader
	var block *Block
	var err error

	if s.Type == BLOCKCHAIN_CLIENT {
		block, err = s.RetrieveBlock(req.Hash, req.Address)
		*header = block.Header
	} else {
		header, err = s.RetrieveHeader(req.Hash, req.Address)
	}
	if err != nil {
		return nil, nil, err
	}

	if header == nil ||
		(s.Type == BLOCKCHAIN_CLIENT && block == nil) {
		return nil, nil, errors.New("Unable to retrieve proper data")
	}

	// Valid header
	if !header.ValidPoW() || header.Hash() != req.Hash {
		return nil, nil, errors.New("Invalid header")
	}

	return header, block, nil
}
