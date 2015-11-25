package ozcoin

import (
	"log"
	"time"
)

type ClientType uint8

const (
	BLOCKCHAIN_CLIENT ClientType = iota
	SVP_CLIENT
)

func NewBlockchain(address string) *Client {
	return newClient(BLOCKCHAIN_CLIENT, address)
}

func NewSPV(address string) *Client {
	return newClient(SVP_CLIENT, address)
}

type Client struct {
	Type               ClientType
	HeaderDBPath       string
	SideHeaderDBPath   string
	OrphanHeaderDBPath string
	BlockDBPath        string
	SideBlockDBPath    string
	OrphanBlockDBPath  string
	UTxnDBPath         string
	PImgDBPath         string
	PeerDBPath         string
	LastHeader         BlockHeader
	Address            string
	Sources            []string
	HashChan           chan BcastRequest
}

func newClient(t ClientType, address string) *Client {
	client := &Client{
		Type:               t,
		HeaderDBPath:       "db/header.db",
		SideHeaderDBPath:   "db/side-header.db",
		OrphanHeaderDBPath: "db/orphan-header.db",
		BlockDBPath:        "db/block.db",
		SideBlockDBPath:    "db/side-block.db",
		OrphanBlockDBPath:  "db/orphan-block.db",
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

	/*
		err = client.WriteHeader(GenesisBlock().Header)
		if err != nil {
			log.Println(err)
			panic("Unable to add genesis block header to database")
		}
	*/

	go client.run()

	return client
}

/*
 *Checks to see if hash is recorded, otherwise spawns a goroutine to
 * resolve hash.
 */
func (c *Client) run() {
	headerDB := c.OpenHeaderDB()
	defer headerDB.Close()

	frontier := make(map[SHA256Sum]struct{})
	doneChan := make(chan SHA256Sum)

	for {
		select {
		case req, ok := <-c.HashChan:
			// Shutdown
			if !ok {
				return
			}

			// Currently resolving
			if _, ok := frontier[req.Hash]; ok {
				continue
			}

			// Load header from main header database
			header, err := c.GetHeader(req.Hash)
			if err != nil {
				log.Println(err)
				continue
			}

			// Already recorded
			if header != nil {
				continue
			}

			// Load header from sidechain header datbase
			header, err = c.GetSideHeader(req.Hash)
			if err != nil {
				log.Println(err)
				continue
			}

			// Already recorded
			if header != nil {
				continue
			}

			// Load header from orphan header database
			header, err = c.GetOrphanHeader(req.Hash)
			if err != nil {
				log.Println(err)
				continue
			}

			// Already recorded
			if header != nil {
				continue
			}

			// Unknown hash, resolve in background
			frontier[req.Hash] = struct{}{}
			go c.AddOrOrphan(req, doneChan)

		case hash := <-doneChan:
			delete(frontier, hash)
		}
	}
}

func (c *Client) ValidHeader(header BlockHeader) bool {
	if !header.ValidPoW() {
		return false
	}

	if header.Time.Add(2 * time.Hour).Before(time.Now()) {
		return false
	}

	if (header.SeqNum == 0) && (header.PrevHash != SHA256Sum{}) {
		return false
	}

	return true
}
