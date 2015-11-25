package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"bytes"
	"encoding/json"
	"log"
	_ "time"
)

type ClientType uint8

const (
	BLOCKCHAIN_CLIENT ClientType = iota
	SVP_CLIENT
)

type Client struct {
	Type               ClientType
	HeaderDBPath       string
	SideHeaderDBPath   string
	OrphanHeaderDBPath string
	BlockDBPath        string
	UTxnDBPath         string
	PImgDBPath         string
	PeerDBPath         string
	LastHeader         BlockHeader
	Address            string
	Sources            []string
	HashChan           chan BcastRequest
}

func NewClient(t ClientType, address string) Client {
	client := Client{
		Type:               t,
		HeaderDBPath:       "db/header.db",
		SideHeaderDBPath:   "db/side-header.db",
		OrphanHeaderDBPath: "db/orphan-header.db",
		BlockDBPath:        "db/block.db",
		UTxnDBPath:         "db/utxn.db",
		PImgDBPath:         "db/pimg.db",
		PeerDBPath:         "db/peer.db",
		Address:            address,
		Sources:            []string{},
		HashChan:           make(chan BcastRequest),
	}

	err := client.Serve()
	if err != nil {
		log.Println(err)
		panic("Unable to start rpc server")
	}

	err = client.WriteHeader(GenesisBlock().Header)
	if err != nil {
		log.Println(err)
		panic("Unable to add genesis block header to database")
	}

	go client.run()

	return client
}

func (s *Client) run() {
	headerDB := s.OpenHeaderDB()
	defer headerDB.Close()

	frontier := make(map[SHA256Sum]struct{})
	doneChan := make(chan SHA256Sum)

	for {
		select {
		case req, ok := <-s.HashChan:
			if !ok {
				return
			}

			if _, ok := frontier[req.Hash]; ok {
				continue
			}

			headerBytes, err := headerDB.Get(req.Hash[:], nil)
			if err != nil {
				log.Println(err)
				continue
			}

			if headerBytes == nil {
				frontier[req.Hash] = struct{}{}
				go s.AddOrOrphan(req, doneChan)
			}
		case hash := <-doneChan:
			delete(frontier, hash)
		}
	}
}

func (s *Client) AddOrOrphan(req BcastRequest, doneChan chan SHA256Sum) {
	_, err := s.AddOrOrphanHelper(req)
	if err != nil {
		log.Println(err)
	}
	doneChan <- req.Hash
}

func (s *Client) AddOrOrphanHelper(req BcastRequest, pull bool) (bool, error) {
	var header *BlockHeader
	var block *Block
	var err error

	// Retrieve block or header
	if s.Type == BLOCKCHAIN_CLIENT {
		block, err = s.RetrieveHeader(req.Hash, req.Address)
		header = block.Header
	} else {
		header, err = s.RetrieveHeader(req.Hash, req.Address)
	}
	if err != nil {
		return false, err
	}
	if header == nil ||
		(s.Type == BLOCKCHAIN_CLIENT && block == nil) {
		return false, errors.New("Unable to retrieve proper data")
	}

	// Valid header
	if !header.ValidPoW() || header.Hash() != req.Hash {
		return false, errors.New("Invalid header")
	}

	if s.Type == BLOCKCHAIN_CLIENT {
		// other validations
	}

	// Check if header adds to the tip of a side chain
	prevHeader, err := s.LookupSideHeader(header.PrevHash)
	if err != nil {
		return false, err
	}

	// If previous header is found, remove old and add new
	if prevHeader != nil {
		sideDB := s.OpenSideHeaderDB()
		defer sideDB.Close()

		batch := &db.Batch{}
		batch.Put(req.Hash[:], header.Json())
		batch.Delete(header.PrevHash[:])

		err = sideDB.Write(batch, nil)
		if err != nil {
			return false, err
		}

		headerDB := s.OpenHeaderDB()
		defer headerDB.Close()

		// Add to valid header db
		err = headerDB.Put(req.Hash[:], header.Json(), nil)
		if err != nil {
			return false, err
		}

		if s.Type == BLOCKCHAIN_CLIENT {
			err = c.WriteBlock(block)
			if err != nil {
				return false, err
			}
		}

		// Update most recent block header
		if header.SeqNum > s.LastHeader.SeqNum {
			s.LastHeader = header
		}

		return true, nil
	}

	// Check if header extends an orhpaned block
	prevHeader, err = s.LookupOrphanHeader(header.PrevHash)
	if err != nil {
		return false, err
	}

	// If previous header is found, remove old and add new
	if prevHeader != nil {
		orphanDB := s.OpenOrphanHeaderDB()
		defer orphanDB.Close()

		err = orphanDB.Put(req.Hash[:], header.Json())
		if err != nil {
			return false, err
		}

		// Walk back through chain to see if it attaches to a valid chain, add all
		// blocks in chain if so.
		newReq := RetrieveRequest{
			RPCHeader: req.RPCHeader,
			Hash:      req.Hash,
		}
		success, err := s.TraverseOrphanChain(newReq)
		if err != nil {
			return false, err
		}

		if !success {
			return false, errors.New("Failed to add orphan chain")
		}

		if s.Type == BLOCKCHAIN_CLIENT {
			err = c.WriteBlock(block)
			if err != nil {
				return false, err
			}
		}

		return true, nil
	}

	// Check if header extends an arbitrary block in database
	prevHeader, err = s.LookupHeader(header.PrevHash)
	if err != nil {
		return false, err
	}

	// If previous header is found, add new header
	if prevHeader != nil {
		// Add header to side chains
		sideDB := s.OpenSideHeaderDB()
		defer sideDB.Close()

		err = sideDB.Put(req.Hash[:], header.Json())
		if err != nil {
			return false, err
		}

		// Add header to valid database
		headerDB := s.OpenHeaderDB()
		defer headerDB.Close()

		err = headerDB.Put(req.Hash[:], header.Json())
		if err != nil {
			return false, err
		}

		if s.Type == BLOCKCHAIN_CLIENT {
			err = c.WriteBlock(block)
			if err != nil {
				return false, err
			}
		}

		return false, nil
	}

	// Otherwise orphan block
	orphanDB := s.OpenOrphanHeaderDB()
	defer orphanDB.Close()

	err = orphanDB.Put(req.Hash[:], header.Json())
	return false, err
}

func (c *Client) AdoptOrphanChain(req BcastRequest) (bool, error) {
	var header *BlockHeader
	var block *Block
	var err error

	header, err := c.LookupSideHeader(req.Hash)
	if err != nil {
		return false, err
	}

	if header != nil {
		return true, nil
	}

	header, err := c.LookupOrphanHeader(req.Hash)
	if err != nil {
		return false, err
	}

	if header == nil {
		if c.Type == BLOCKCHAIN_CLIENT {
			block, err = c.RetrieveBlock(req.Hash, req.Address)
			header = block.Header
		} else {
			header, err = c.RetrieveHeader(req.Hash, req.Address)
		}
		if err != nil {
			return false, err
		}
		if header == nil ||
			(c.Type == BLOCKCHAIN_CLIENT && block == nil) {
			return false, errors.New("Unable to retrieve proper data")
		}
	}

	newReq := RetrieveRequest{
		RPCHeader: req.RPCHeader,
		Hash:      header.PrevHash,
	}
	success, err := c.AdoptOrphanChain(req)
	if err != nil {
		return false, err
	}

	if !success {
		return false, nil
	}

	if c.Type == BLOCKCHAIN_CLIENT {
		err = c.WriteBlock()
		if err != nil {
			return false, err
		}

	}

	return true, nil
}

func (s *Client) ValidHeader(header BlockHeader) bool {
	if header.SeqNum == 0 {
		return header.PrevHash == SHA256Sum{} && header.ValidPoW()
	}

	return s.LastHeader.Hash() == header.PrevHash && header.ValidPoW()
}

func (s *Client) WriteHeader(header BlockHeader) error {
	headerDB := s.OpenHeaderDB()
	defer headerDB.Close()

	headerJson := header.Json()
	hash := header.Hash()

	err := headerDB.Put(hash[:], headerJson, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println("Last header:", header)
	s.LastHeader = header

	return nil
}

func (s *Client) OpenHeaderDB() *db.DB {
	headerDB, err := db.OpenFile(s.HeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open header database")
	}

	return headerDB
}

func (s *Client) LookupHeader(hash SHA256Sum) (*BlockHeader, error) {
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

func (s *Client) OpenSideHeaderDB() *db.DB {
	sideDB, err := db.OpenFile(s.SideHeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open sidechain header database")
	}

	return sideDB
}

func (s *Client) LookupSideHeader(hash SHA256Sum) (*BlockHeader, error) {
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
	err := json.Unmarshal(headerBytes, header)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return header, nil
}

func (s *Client) OpenOrphanHeaderDB() *db.DB {
	orphanDB, err := db.OpenFile(s.OrphanHeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open orphan header database")
	}

	return orphanDB
}

func (s *Client) LookupOrphanHeader(hash SHA256Sum) (*BlockHeader, error) {
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
	err := json.Unmarshal(headerBytes, header)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return header, nil
}

func (s *Client) OpenBlockDB() *db.DB {
	blockDB, err := db.OpenFile(s.BlockDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open block database")
	}

	return blockDB
}

func (bc *Blockchain) LookupBlock(hash SHA256Sum) (Block, error) {
	blockDB := s.OpenBlockDB()
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

func (s *Client) OpenPreimageDB() *db.DB {
	pimgDB, err := db.OpenFile(s.PImgDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open preimage database")
	}

	return pimgDB
}

func (s *Client) LookupPreimage(hash SHA256Sum) (bool, error) {
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

func (s *Client) OpenUTxnDB() *db.DB {
	utxnDB, err := db.OpenFile(s.UTxnDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open unspent txn database")
	}

	return utxnDB
}

func (s *Client) LookupUTxn(hash SHA256Sum, index uint8) (*Output, error) {
	utxnDB := s.OpenUTxnDB()
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

	output := &Output{txn.Outputs[index]}

	return output, nil
}
