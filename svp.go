package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"encoding/json"
	"log"
)

type SVP struct {
	HeaderDBPath string
	LastHeader   BlockHeader
}

func NewSVP() SVP {
	svp := SVP{
		HeaderDBPath: "db/header.db",
	}

	err := svp.WriteHeader(GenesisBlock().Header)
	if err != nil {
		log.Println(err)
		panic("Unable to add genesis block header to database")
	}

	return svp
}

func (s *SVP) ValidHeader(header BlockHeader) bool {
	if header.SeqNum == 0 {
		return header.PrevHash == SHA256Sum{} && header.ValidPoW()
	}

	return s.LastHeader.Hash() == header.PrevHash && header.ValidPoW()
}

func (s *SVP) WriteHeader(header BlockHeader) error {
	headerDB, err := db.OpenFile(s.HeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open header database")
	}
	defer headerDB.Close()

	headerJson := header.Json()
	hash := header.Hash()

	err = headerDB.Put(hash[:], headerJson, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println("Last header:", header)
	s.LastHeader = header

	return nil
}

func (bc *Blockchain) LookupBlockHeader(hash SHA256Sum) (BlockHeader, error) {
	headerDB, err := db.OpenFile(bc.HeaderDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open header database")
	}
	defer headerDB.Close()

	headerBytes, err := headerDB.Get(hash[:], nil)
	if err != nil {
		return BlockHeader{}, err
	}

	var header BlockHeader
	err = json.Unmarshal(headerBytes, &header)
	if err != nil {
		return BlockHeader{}, err
	}

	return header, nil
}
