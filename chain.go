package ozcoin

import (
	"errors"
	"log"
)

func CoinbaseValue(seqnum uint64) uint64 {
	return (50 * 100000000) >> (seqnum / 21000)
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
