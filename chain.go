package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"encoding/json"
	"log"
)

type Blockchain struct {
	SVP
	BlockDBPath string
	UTxnDBPath  string
	PImgDBPath  string
}

func NewBlockchain() Blockchain {
	bc := Blockchain{
		SVP:         NewSVP(),
		BlockDBPath: "db/block.db",
		UTxnDBPath:  "db/utxn.db",
		PImgDBPath:  "db/pimg.db",
	}

	g := GenesisBlock()
	for !g.Header.ValidPoW() {
		g.Header.Nonce += 1
	}

	if bc.ValidBlock(g) {
		err := bc.WriteBlock(g)
		if err != nil {
			log.Println(err)
			panic("Unable to add genesis block to database")
		}
	} else {
		panic("Genesis block invalid")
	}

	return bc
}

func (bc *Blockchain) ValidBlock(b Block) bool {
	return bc.ValidHeader(b.Header) &&
		bc.ValidTxns(b)
}

func (bc *Blockchain) WriteBlock(b Block) error {
	// Write Header
	err := bc.WriteHeader(b.Header)
	if err != nil {
		log.Println(err)
		return err
	}

	// Open block database
	blockDB, err := db.OpenFile(bc.BlockDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open block database")
	}
	defer blockDB.Close()

	// Write block
	hash := b.Header.Hash()
	err = blockDB.Put(hash[:], b.Json(), nil)
	if err != nil {
		log.Println(err)
		return err
	}

	// Open transaction database
	utxnDB, err := db.OpenFile(bc.UTxnDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open utxn database")
	}
	defer utxnDB.Close()

	// Open preimage database
	pimgDB, err := db.OpenFile(bc.PImgDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open pimg database")
	}
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

func (bc *Blockchain) ValidTxns(b Block) bool {
	// Check validity of each transaction
	for i, txn := range b.Txns {
		if i == 0 {
			if !bc.ValidGenerationTxn(txn) {
				log.Println("Invalid GenerationTxn")
				return false
			}
		} else {
			if !bc.ValidTxn(txn) {
				log.Println("Invalid Txn")
				return false
			}
		}
	}

	// Check sum of fees = generation txn fees

	return true
}

func (bc *Blockchain) ValidTxn(txn Txn) bool {
	if txn.Body.Inputs == nil || len(txn.Body.Inputs) != TXN_NUM_INPUTS {
		log.Println("Invalid number of txn inputs")
		return false
	}

	if txn.Body.Outputs == nil || len(txn.Body.Outputs) != TXN_NUM_OUTPUTS {
		log.Println("Invalid number of txn outputs")
		return false
	}

	utxns, err := bc.fetchInputTxns(txn.Body.Inputs)
	if err != nil {
		log.Println("Error fetching input txns from database")
		return false
	}

	pks, ics := []ECCPoint{}, []Commitment{}
	for _, utxn := range utxns {
		pks = append(pks, utxn.PublicKey)
		ics = append(ics, utxn.Commit)
	}

	return true //txn.ValidOZRS(pks, ics)
}

func (bc *Blockchain) ValidGenerationTxn(txn Txn) bool {
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

func (bc *Blockchain) fetchInputTxns(inputs []Input) ([]Output, error) {
	utxns := []Output{}
	for _, input := range inputs {
		txn, err := bc.LookupTxn(input.Hash)
		if err != nil {
			return nil, err
		}

		if input.Index > 1 {
			msg := "Cannot allow txn input index greater than 1"
			log.Println(msg)
			panic(msg)
		}

		utxns = append(utxns, txn.Body.Outputs[input.Index])
	}

	return utxns, nil
}

func (bc *Blockchain) LookupTxn(hash SHA256Sum) (Txn, error) {
	utxnDB, err := db.OpenFile(bc.UTxnDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open utxn database")
	}
	defer utxnDB.Close()

	txnBytes, err := utxnDB.Get(hash[:], nil)
	if err != nil {
		return Txn{}, err
	}

	var txn Txn
	err = json.Unmarshal(txnBytes, &txn)
	if err != nil {
		return Txn{}, err
	}

	return txn, nil
}

func (bc *Blockchain) LookupBlock(hash SHA256Sum) (Block, error) {
	blockDB, err := db.OpenFile(bc.BlockDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open block database")
	}
	defer blockDB.Close()

	blockBytes, err := blockDB.Get(hash[:], nil)
	if err != nil {
		return Block{}, err
	}

	var block Block
	err = json.Unmarshal(blockBytes, &block)
	if err != nil {
		return Block{}, err
	}

	return block, nil
}
