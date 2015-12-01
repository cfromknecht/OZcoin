package ozcoin

import (
	"log"
)

type ClientType uint8

const (
	BLOCKCHAIN_CLIENT ClientType = iota
	SVP_CLIENT
)

var SIGNAL = struct{}{}

/*
 * Stores full copies of every block and txn pool.
 */
func NewBlockchain(clientAddress, walletAddress, password string) *Client {
	return newClient(BLOCKCHAIN_CLIENT, clientAddress, walletAddress, password, false)
}

/*
 * Only stores block headers and preimages.
 */
func NewSPV(clientAddress, walletAddress, password string) *Client {
	return newClient(SVP_CLIENT, clientAddress, walletAddress, password, true)
}

/*
 * Client
 *
 * Provides access to database operations and facilitates the consensus
 * mechanisms.
 */
type Client struct {
	Type               ClientType
	LastHeader         BlockHeader
	UpdateWallet       bool
	Address            string
	HeaderDBPath       string
	SideHeaderDBPath   string
	OrphanHeaderDBPath string
	BlockDBPath        string
	SideBlockDBPath    string
	OrphanBlockDBPath  string
	MapDBPath          string
	UTxnDBPath         string
	PImgDBPath         string
	PeerDBPath         string
	TxnPoolDBPath      string
	Sources            []string
	BlockHashChan      chan HashMsg
	TxnHashChan        chan HashMsg
	BlockChan          chan Block
	TxnChan            chan Txn
	dbm                *DBManager
	Wallet             *WalletClient
}

/*
 * Builds a new client and starts the gossip rpc server.
 */
func newClient(t ClientType, clientAddress, walletAddress, password string, updateWallet bool) *Client {
	client := &Client{
		Type:               t,
		UpdateWallet:       updateWallet,
		HeaderDBPath:       "db/header.db",
		SideHeaderDBPath:   "db/side-header.db",
		OrphanHeaderDBPath: "db/orphan-header.db",
		BlockDBPath:        "db/block.db",
		SideBlockDBPath:    "db/side-block.db",
		OrphanBlockDBPath:  "db/orphan-block.db",
		MapDBPath:          "db/map.db",
		PImgDBPath:         "db/pimg.db",
		PeerDBPath:         "db/peer.db",
		TxnPoolDBPath:      "db/txn-pool.db",
		Address:            clientAddress,
		Sources:            []string{},
		BlockHashChan:      make(chan HashMsg),
		TxnHashChan:        make(chan HashMsg),
		BlockChan:          make(chan Block),
		TxnChan:            make(chan Txn),
		Wallet: &WalletClient{
			Address: walletAddress,
		},
	}
	client.dbm = client.OpenDatabases()

	err := client.Serve()
	if err != nil {
		log.Println(err)
		panic("Unable to start rpc server")
	}

	go client.run()

	return client
}

/*
 * Checks to see if hash is recorded, otherwise spawns a goroutine to
 * resolve hash.
 */
func (c *Client) run() {
	log.Println("Running client...")
	frontier := make(map[SHA256Sum]struct{})
	doneChan := make(chan SHA256Sum)
	startChan := make(chan struct{})

	go func() {
		startChan <- SIGNAL
	}()

	for {
		select {
		case req := <-c.BlockHashChan:
			// Resolve incoming block hash

			// Currently resolving
			if _, ok := frontier[req.Hash]; ok {
				continue
			}

			// Filter if already recorded
			err := c.FilterBlock(req)
			if err == nil {
				continue
			}

			log.Println("Resolving chain")
			// Unknown hash, resolve in background
			frontier[req.Hash] = SIGNAL
			go c.AddOrOrphan(req, startChan, doneChan)

		case req := <-c.TxnHashChan:
			// Resolve incoming txn hash

			// Currently resolving
			if _, ok := frontier[req.Hash]; ok {
				continue
			}

			// Filter if already recorded
			err := c.FilterTxn(req)
			if err == nil {
				continue
			}

			log.Println("Resolving txn")
			// Unknown hash, resolve in background
			frontier[req.Hash] = SIGNAL
			go c.AddToTxnPool(req, startChan, doneChan)

		case block := <-c.BlockChan:
			// New Block
			frontier[block.Header.Hash()] = SIGNAL
			go c.AdoptMinedBlock(block, startChan, doneChan)

		case txn := <-c.TxnChan:
			// New Txn
			frontier[txn.Hash()] = SIGNAL
			go c.AdoptTxn(txn, startChan, doneChan)

		case hash := <-doneChan:
			// Remove from frontier and signal next operation
			delete(frontier, hash)
			go func() { startChan <- SIGNAL }()

		}
	}
}

/*
 * Validates and adds a txn to the txn pool.
 */
func (c *Client) AdoptTxn(txn Txn, startChan chan struct{}, doneChan chan SHA256Sum) {
	_ = <-startChan

	// Signal when complete
	defer func() { doneChan <- txn.Hash() }()

	log.Println("New txn:", string(txn.Json()))

	if !ValidTxn(txn) && !ValidCoinbaseTxn(txn) {
		log.Println("Invalid txn")
		return
	}

	err := c.PutTxnPool(txn)
	if err != nil {
		log.Println("Failed to add txn to txn pool")
		return
	}

	log.Println("Txn added to txn pool, broadcasting")

	err = c.BcastTxn(txn.Hash())
	if err != nil {
		log.Println("Failed to brodcast txn")
		return
	}

}

/*
 * Validates and extends the main chain with mined block.
 */
func (c *Client) AdoptMinedBlock(block Block, startChan chan struct{}, doneChan chan SHA256Sum) {
	_ = <-startChan

	// Signal when complete
	defer func() { doneChan <- block.Header.Hash() }()

	log.Println("New block:", string(block.Json()))

	if block.Header.SeqNum != 0 && c.LastHeader.Hash() != block.Header.PrevHash {
		log.Println("Mined block rejected: PrevHash incorrect or not genesis block")
		return
	}

	if !ValidHeader(block.Header) {
		log.Println("Mined block rejected: invalid header")
		return
	}
	if !c.PrevalidBlock(block) {
		log.Println("Mined block rejected: block prevalidation failed")
		return
	}

	success, err := c.ExtendMainChain(block.Header, &block)
	if err != nil {
		log.Println(err)
		return
	}

	if !success {
		log.Println("Failed to extend main chain")
		return
	}

	log.Println("Mined block accpeted, broadcasting block")
	err = c.BcastBlock(block.Header.Hash())
	if err != nil {
		log.Println("Broadcast failed")
		return
	}

	log.Println("Broadcast successful")
}

/*
 * Returns nil if client is already aware of block.
 */
func (c *Client) FilterBlock(req HashMsg) error {
	// Load header from main header database
	_, err := c.GetHeader(req.Hash)
	if err != nil {
		// Load header from sidechain header datbase
		_, err = c.GetSideHeader(req.Hash)
		if err != nil {
			// Load header from orphan header database
			_, err = c.GetOrphanHeader(req.Hash)
		}
	}

	return err
}

/*
 * Returns nil if client is already aware of txn.
 */
func (c *Client) FilterTxn(req HashMsg) error {
	// Load header from txn pool database
	_, err := c.GetTxnPool(req.Hash)
	if err != nil {
		// Check for preimage
		found := c.GetPreimage(req.Hash)
		if found {
			err = nil
		}
	}

	return err
}
