package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"encoding/json"
	"log"
)

/*
 * Main Header Database
 */

func (s *Client) OpenHeaderDB() *db.DB {
	headerDB, err := db.OpenFile(s.HeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open header database")
	}

	return headerDB
}

func (s *Client) GetHeader(hash SHA256Sum) (*BlockHeader, error) {
	headerDB := s.OpenHeaderDB()
	defer headerDB.Close()

	headerBytes, err := headerDB.Get(hash[:], nil)
	if err != nil {
		return nil, err
	}

	if headerBytes == nil {
		return nil, nil
	}

	var header *BlockHeader
	err = json.Unmarshal(headerBytes, header)
	if err != nil {
		return nil, err
	}

	return header, nil
}

func (c *Client) ExtendHeader(header BlockHeader) error {
	headerDB := c.OpenHeaderDB()
	defer headerDB.Close()

	hash := header.Hash()

	batch := &db.Batch{}
	batch.Put(hash[:], header.Json())
	batch.Delete(header.PrevHash[:])

	return headerDB.Write(batch, nil)
}

func (c *Client) PutHeader(header BlockHeader) error {
	headerDB := c.OpenHeaderDB()
	defer headerDB.Close()

	hash := header.Hash()
	return headerDB.Put(hash[:], header.Json(), nil)
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

func (s *Client) GetSideHeader(hash SHA256Sum) (*BlockHeader, error) {
	sideDB := s.OpenSideHeaderDB()
	defer sideDB.Close()

	headerBytes, err := sideDB.Get(hash[:], nil)
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
	sideDB := c.OpenSideHeaderDB()
	defer sideDB.Close()

	hash := header.Hash()
	return sideDB.Put(hash[:], header.Json(), nil)
}

func (c *Client) ExtendSideHeader(header BlockHeader) error {
	sideDB := c.OpenSideHeaderDB()
	defer sideDB.Close()

	hash := header.Hash()

	batch := &db.Batch{}
	batch.Put(hash[:], header.Json())
	batch.Delete(header.PrevHash[:])

	return sideDB.Write(batch, nil)
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

func (s *Client) GetOrphanHeader(hash SHA256Sum) (*BlockHeader, error) {
	orphanDB := s.OpenOrphanHeaderDB()
	defer orphanDB.Close()

	headerBytes, err := orphanDB.Get(hash[:], nil)
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
	orphanDB := c.OpenOrphanHeaderDB()
	defer orphanDB.Close()

	hash := header.Hash()
	return orphanDB.Put(hash[:], header.Json(), nil)
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
	blockDB := c.OpenBlockDB()
	defer blockDB.Close()

	blockBytes, err := blockDB.Get(hash[:], nil)
	if err != nil {
		return &Block{}, err
	}

	var block *Block
	err = json.Unmarshal(blockBytes, block)
	if err != nil {
		return &Block{}, err
	}

	return block, nil
}

func (c *Client) PutBlock(block Block) error {
	blockDB := c.OpenBlockDB()
	defer blockDB.Close()

	hash := block.Header.Hash()
	return blockDB.Put(hash[:], block.Json(), nil)
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
	sblockDB := c.OpenSideBlockDB()
	defer sblockDB.Close()

	sblockBytes, err := sblockDB.Get(hash[:], nil)
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
	sblockDB := c.OpenSideBlockDB()
	defer sblockDB.Close()

	hash := block.Header.Hash()
	return sblockDB.Put(hash[:], block.Json(), nil)
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
	oblockDB := c.OpenOrphanBlockDB()
	defer oblockDB.Close()

	oblockBytes, err := oblockDB.Get(hash[:], nil)
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
	oblockDB := c.OpenOrphanBlockDB()
	defer oblockDB.Close()

	hash := block.Header.Hash()
	return oblockDB.Put(hash[:], block.Json(), nil)
}

/*
 * Preimage Database
 */

func (s *Client) OpenPreimageDB() *db.DB {
	pimgDB, err := db.OpenFile(s.PImgDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open preimage database")
	}

	return pimgDB
}

func (s *Client) GetPreimage(hash SHA256Sum) (bool, error) {
	pimgDB := s.OpenPreimageDB()
	defer pimgDB.Close()

	preimageBytes, err := pimgDB.Get(hash[:], nil)
	if err != nil {
		return false, err
	}

	if len(preimageBytes) != SHA256_SUM_LENGTH {
		return false, nil
	}

	for i := 0; i < SHA256_SUM_LENGTH; i++ {
		if hash[i] != preimageBytes[i] {
			return false, nil
		}
	}

	return true, nil
}

func (c *Client) PutPreimage(hash SHA256Sum) error {
	pimgDB := c.OpenPreimageDB()
	defer pimgDB.Close()

	return pimgDB.Put(hash[:], hash[:], nil)
}

func (s *Client) OpenUTxnDB() *db.DB {
	utxnDB, err := db.OpenFile(s.UTxnDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open unspent txn database")
	}

	return utxnDB
}

/*
 * Unspent Transaction Database
 */

func (c *Client) GetUTxn(hash SHA256Sum, index uint8) (*Output, error) {
	utxnDB := c.OpenUTxnDB()
	defer utxnDB.Close()

	utxnBytes, err := utxnDB.Get(hash[:], nil)
	if err != nil {
		return nil, err
	}

	var txn *Txn
	err = json.Unmarshal(utxnBytes, txn)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	output := &Output{}
	*output = txn.Body.Outputs[index]

	return output, nil
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
	peerDB := c.OpenPeerDB()
	defer peerDB.Close()

	return peerDB.Put([]byte(address), []byte{}, nil)
}
