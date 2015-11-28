package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"encoding/json"
	"errors"
	"log"
)

type DBManager struct {
	headerDB       *db.DB
	sideHeaderDB   *db.DB
	orphanHeaderDB *db.DB
	blockDB        *db.DB
	sideBlockDB    *db.DB
	orphanBlockDB  *db.DB
	mapDB          *db.DB
	pimgDB         *db.DB
	peerDB         *db.DB
	txnPoolDB      *db.DB
}

func (c *Client) OpenDatabases() *DBManager {
	dbm := &DBManager{}
	dbm.OpenConnections(c)

	return dbm
}

func (dbm *DBManager) OpenConnections(c *Client) {
	dbm.headerDB = c.OpenHeaderDB()
	dbm.sideHeaderDB = c.OpenSideHeaderDB()
	dbm.orphanHeaderDB = c.OpenOrphanHeaderDB()
	dbm.blockDB = c.OpenBlockDB()
	dbm.sideBlockDB = c.OpenSideBlockDB()
	dbm.orphanBlockDB = c.OpenOrphanBlockDB()
	dbm.mapDB = c.OpenMapDB()
	dbm.pimgDB = c.OpenPreimageDB()
	dbm.peerDB = c.OpenPeerDB()
	dbm.txnPoolDB = c.OpenTxnPoolDB()
}

/*
 * Main Header Database
 */

func (c *Client) OpenHeaderDB() *db.DB {
	headerDB, err := db.OpenFile(c.HeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open header database")
	}

	return headerDB
}

func (c *Client) GetHeader(hash SHA256Sum) (*BlockHeader, error) {
	headerBytes, err := c.dbm.headerDB.Get(hash[:], nil)
	if err != nil {
		return nil, err
	}

	if headerBytes == nil {
		return nil, nil
	}

	header := &BlockHeader{}
	err = json.Unmarshal(headerBytes, header)
	if err != nil {
		return nil, err
	}

	return header, nil
}

func (c *Client) ExtendHeader(header BlockHeader) error {
	hash := header.Hash()

	batch := &db.Batch{}
	batch.Put(hash[:], header.Json())
	batch.Delete(header.PrevHash[:])

	return c.dbm.headerDB.Write(batch, nil)
}

func (c *Client) PutHeader(header BlockHeader) error {
	hash := header.Hash()
	log.Println("Putting header")
	return c.dbm.headerDB.Put(hash[:], header.Json(), nil)
}

/*
 * Sidechain Header Database
 */

func (s *Client) OpenSideHeaderDB() *db.DB {
	sideDB, err := db.OpenFile(s.SideHeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open sidechain header database")
	}

	return sideDB
}

func (c *Client) GetSideHeader(hash SHA256Sum) (*BlockHeader, error) {
	headerBytes, err := c.dbm.sideHeaderDB.Get(hash[:], nil)
	if err != nil {
		return nil, err
	}

	if headerBytes == nil {
		return nil, nil
	}

	var header *BlockHeader
	err = json.Unmarshal(headerBytes, header)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return header, nil
}

func (c *Client) PutSideHeader(header BlockHeader) error {
	hash := header.Hash()
	return c.dbm.sideHeaderDB.Put(hash[:], header.Json(), nil)
}

/*
 * Orphan Header Database
 */

func (s *Client) OpenOrphanHeaderDB() *db.DB {
	orphanDB, err := db.OpenFile(s.OrphanHeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open orphan header database")
	}

	return orphanDB
}

func (c *Client) GetOrphanHeader(hash SHA256Sum) (*BlockHeader, error) {
	headerBytes, err := c.dbm.orphanHeaderDB.Get(hash[:], nil)
	if err != nil {
		return nil, err
	}

	if headerBytes == nil {
		return nil, nil
	}

	var header *BlockHeader
	err = json.Unmarshal(headerBytes, header)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return header, nil
}

func (c *Client) PutOrphanHeader(header BlockHeader) error {
	hash := header.Hash()
	return c.dbm.orphanHeaderDB.Put(hash[:], header.Json(), nil)
}

/*
 * Main Block Database
 */

func (s *Client) OpenBlockDB() *db.DB {
	blockDB, err := db.OpenFile(s.BlockDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open block database")
	}

	return blockDB
}

func (c *Client) GetBlock(hash SHA256Sum) (*Block, error) {
	blockBytes, err := c.dbm.blockDB.Get(hash[:], nil)
	if err != nil {
		return &Block{}, err
	}

	block := &Block{}
	err = json.Unmarshal(blockBytes, block)
	if err != nil {
		return block, err
	}

	return block, nil
}

func (c *Client) PutBlock(block Block) error {
	hash := block.Header.Hash()
	return c.dbm.blockDB.Put(hash[:], block.Json(), nil)
}

/*
 * Sidechain Block Database
 */

func (s *Client) OpenSideBlockDB() *db.DB {
	sblockDB, err := db.OpenFile(s.SideBlockDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open side block database")
	}

	return sblockDB
}

func (c *Client) GetSideBlock(hash SHA256Sum) (*Block, error) {
	sblockBytes, err := c.dbm.sideBlockDB.Get(hash[:], nil)
	if err != nil {
		return &Block{}, err
	}

	var sblock *Block
	err = json.Unmarshal(sblockBytes, sblock)
	if err != nil {
		return &Block{}, err
	}

	return sblock, nil
}

func (c *Client) PutSideBlock(block Block) error {
	hash := block.Header.Hash()
	return c.dbm.sideBlockDB.Put(hash[:], block.Json(), nil)
}

/*
 * Orphan Block Database
 */

func (s *Client) OpenOrphanBlockDB() *db.DB {
	oblockDB, err := db.OpenFile(s.OrphanBlockDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open orphan block database")
	}

	return oblockDB
}

func (c *Client) GetOrphanBlock(hash SHA256Sum) (*Block, error) {
	oblockBytes, err := c.dbm.orphanBlockDB.Get(hash[:], nil)
	if err != nil {
		return &Block{}, err
	}

	var oblock *Block
	err = json.Unmarshal(oblockBytes, oblock)
	if err != nil {
		return &Block{}, err
	}

	return oblock, nil
}

func (c *Client) PutOrphanBlock(block Block) error {
	hash := block.Header.Hash()
	return c.dbm.orphanBlockDB.Put(hash[:], block.Json(), nil)
}

/*
 * Preimage Database
 */

func (c *Client) OpenPreimageDB() *db.DB {
	pimgDB, err := db.OpenFile(c.PImgDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open preimage database")
	}

	return pimgDB
}

func (c *Client) GetPreimage(hash SHA256Sum) bool {
	preimageBytes, err := c.dbm.pimgDB.Get(hash[:], nil)
	if err != nil {
		return false
	}

	if len(preimageBytes) != SHA256_SUM_LENGTH {
		return false
	}

	for i := 0; i < SHA256_SUM_LENGTH; i++ {
		if hash[i] != preimageBytes[i] {
			return false
		}
	}

	return true
}

func (c *Client) PutPreimage(hash SHA256Sum) error {
	return c.dbm.pimgDB.Put(hash[:], hash[:], nil)
}

/*
 * Txn and Output to Block Map Database
 */

func (c *Client) OpenMapDB() *db.DB {
	mDB, err := db.OpenFile(c.MapDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open map database")
	}

	return mDB
}

func (c *Client) MapToBlock(hash SHA256Sum) (SHA256Sum, error) {
	hashBytes, err := c.dbm.mapDB.Get(hash.Bytes(), nil)
	if err != nil {
		return SHA256Sum{}, err
	}

	if len(hashBytes) != SHA256_SUM_LENGTH {
		return SHA256Sum{}, errors.New("Invalid hash length")
	}

	s := SHA256Sum{}
	for i, b := range hashBytes {
		s[i] = b
	}

	return s, nil
}

func (c *Client) PutMapToBlock(block Block) error {
	blockHash := block.Header.Hash()

	log.Println("Making map batch")
	batch := &db.Batch{}
	for i, txn := range block.Txns {
		if i != 0 {
			log.Println("Adding preimage")
			pimgHash := Hash(txn.Sig.Preimage.Bytes())
			batch.Put(pimgHash.Bytes(), blockHash.Bytes())
		}
		for _, output := range txn.Body.Outputs {
			log.Println("Adding output")
			batch.Put(output.Hash().Bytes(), blockHash.Bytes())
		}
	}

	log.Println("Writing batch")

	return c.dbm.mapDB.Write(batch, nil)
}

func (c *Client) DeleteMapToBlock(block Block) error {
	batch := &db.Batch{}
	for i, txn := range block.Txns {
		if i != 0 {
			pimgHash := Hash(txn.Sig.Preimage.Bytes())
			batch.Delete(pimgHash.Bytes())
		}
		for _, output := range txn.Body.Outputs {
			batch.Delete(output.Hash().Bytes())
		}
	}

	return c.dbm.mapDB.Write(batch, nil)
}

/*
 * Peer Database
 */

func (c *Client) OpenPeerDB() *db.DB {
	peerDB, err := db.OpenFile(c.PeerDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open peer database")
	}

	return peerDB
}

func (c *Client) PutPeer(address string) error {
	return c.dbm.peerDB.Put([]byte(address), []byte{}, nil)
}

/*
 * Txn Pool Database
 */

func (c *Client) OpenTxnPoolDB() *db.DB {
	txnPoolDB, err := db.OpenFile(c.TxnPoolDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open txn pool database")
	}

	return txnPoolDB
}

func (c *Client) GetTxnPool(hash SHA256Sum) (*Txn, error) {
	txnBytes, err := c.dbm.txnPoolDB.Get(hash[:], nil)
	if err != nil {
		return nil, err
	}

	txn := &Txn{}
	err = json.Unmarshal(txnBytes, txn)
	if err != nil {
		return nil, err
	}

	return txn, nil
}

func (c *Client) PutTxnPool(txn Txn) error {
	hash := txn.Hash()
	return c.dbm.txnPoolDB.Put(hash[:], txn.Json(), nil)
}

func (c *Client) DeleteTxnPool(txn Txn) error {
	hash := txn.Hash()
	return c.dbm.txnPoolDB.Delete(hash[:], nil)
}

func (c *Client) TxnsFromPool() []Txn {
	txns := []Txn{}
	iter := c.dbm.txnPoolDB.NewIterator(nil, nil)
	for iter.Next() {
		txnBytes := iter.Value()

		txn := Txn{}
		err := json.Unmarshal(txnBytes, &txn)
		if err != nil {
			log.Println("Could not marshal txn:", err)
			continue
		}

		if !ValidTxn(txn) {
			log.Println("INVALID TXN")
			continue
		}

		if !c.VerifyTxn(txn, nil, nil, nil, nil) {
			log.Println("TXN FAILED TO VERIFY")
			continue
		}

		txns = append(txns, txn)
	}
	iter.Release()

	return txns
}
