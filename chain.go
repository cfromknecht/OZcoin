package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"errors"
	"log"
)

func (c *Client) ValidBlock(b Block) bool {
	return c.ValidHeader(b.Header) &&
		c.ValidTxns(b)
}

func (c *Client) WriteBlock(b Block) error {
	// Open block database
	blockDB := c.OpenBlockDB()
	defer blockDB.Close()

	// Write block
	hash := b.Header.Hash()
	err = blockDB.Put(hash[:], b.Json(), nil)
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
			if !c.ValidGenerationTxn(txn) {
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

func (c *Client) ValidGenerationTxn(txn Txn) bool {
	if txn.Body.Inputs != nil {
		log.Println("Txn inputs should be nil")
		return false
	}

	if txn.Body.Outputs == nil || len(txn.Body.Outputs) != 1 {
		log.Println("Invalid number of txn outputs")
		return false
	}

	return !txn.Body.Outputs[0].PublicKey.Empty() &&
		!txn.Body.Outputs[0].DestKey.Empty() &&
		!txn.Body.Outputs[0].BlindSeed.Empty() &&
		!txn.Body.Outputs[0].Commit.Empty()
}

func (c *Client) fetchInputTxns(inputs []Input) ([]Output, error) {
	utxns := []Output{}
	for _, input := range inputs {
		if input.Index > 1 {
			msg := "Cannot allow txn input index greater than 1"
			log.Println(msg)
			return nil, errors.New(msg)
		}

		otpt, err := c.LookupUTxn(input.Hash, input.Index)
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
