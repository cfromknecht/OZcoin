package ozcoin

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/rpc"
	"time"
)

const (
	NUM_PEERS             = 10
	BLOCK_RETRIEVAL_LIMIT = 25
)

type GossipCore struct {
	c *Client
}

func (s *Client) Serve() error {
	rpc.Register(&GossipCore{s})

	l, err := net.Listen("tcp", s.Address)
	if err != nil {
		return err
	}

	go rpc.Accept(l)

	return nil
}

type RPCHeader struct {
	Address string
}

type HashMsg struct {
	RPCHeader
	Hash SHA256Sum
}

type HeaderMsg struct {
	RPCHeader
	Header BlockHeader
}

type BlockMsg struct {
	RPCHeader
	Block
}

type TxnMsg struct {
	RPCHeader
	Txn
}

type OutputMsg struct {
	RPCHeader
	Output
}

func (c *Client) HandleRPC(request RPCHeader, response *RPCHeader) error {
	err := c.PutPeer(request.Address)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println("Peer Added:", request.Address)

	response.Address = c.Address

	return nil
}

func (c *Client) NewHashMsg(hash SHA256Sum) HashMsg {
	return HashMsg{
		RPCHeader: RPCHeader{
			Address: c.Address,
		},
		Hash: hash,
	}
}

func (hm HashMsg) NewHash(hash SHA256Sum) HashMsg {
	return HashMsg{
		RPCHeader: hm.RPCHeader,
		Hash:      hash,
	}
}

func (c *Client) BcastBlock(hash SHA256Sum) error {
	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		go c.sendBcast("GossipCore.BcastBlockRPC", address, hash)
	}
	iter.Release()

	return iter.Error()
}

func (c *Client) BcastTxn(hash SHA256Sum) error {
	iter := c.dbm.peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())
		go c.sendBcast("GossipCore.BcastTxnRPC", address, hash)
	}
	iter.Release()

	return iter.Error()
}

func (c *Client) sendBcast(method, address string, hash SHA256Sum) {
	peer, err := c.dialPeer(address)
	if err != nil {
		err = c.dbm.peerDB.Delete([]byte(address), nil)
		if err != nil {
			log.Println(err)
		}
		return
	}

	req := c.NewHashMsg(hash)
	err = peer.Call(method, &req, &RPCHeader{})
	if err != nil {
		log.Println("Failed to broadcast", err)
	}
}

func (gc *GossipCore) BcastBlockRPC(req HashMsg, res *RPCHeader) error {
	err := gc.c.HandleRPC(req.RPCHeader, res)
	if err != nil {
		return err
	}

	if (req.Hash != SHA256Sum{}) {
		gc.c.BlockHashChan <- req
	}

	return nil
}

func (gc *GossipCore) BcastTxnRPC(req HashMsg, res *RPCHeader) error {
	err := gc.c.HandleRPC(req.RPCHeader, res)
	if err != nil {
		return err
	}

	if (req.Hash != SHA256Sum{}) {
		gc.c.TxnHashChan <- req
	}

	return nil
}

func (s *Client) dialPeer(address string) (*rpc.Client, error) {
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}

	return rpc.NewClient(conn), nil
}

func (s *Client) FetchHeader(hash SHA256Sum, address string) (*BlockHeader, error) {
	peer, err := s.dialPeer(address)
	if err != nil {
		return nil, err
	}

	req := s.NewHashMsg(hash)
	res := HeaderMsg{}

	err = peer.Call("GossipCore.FetchHeaderRPC", &req, &res)
	if err != nil {
		return nil, err
	}
	header := &BlockHeader{}
	*header = res.Header

	if header.Hash() != hash {
		return nil, errors.New("Wrong header")
	}

	return header, nil
}

func (gc *GossipCore) FetchHeaderRPC(req HashMsg, res *HeaderMsg) error {
	err := gc.c.HandleRPC(req.RPCHeader, &res.RPCHeader)
	if err != nil {
		return err
	}

	headerBytes, err := gc.c.dbm.headerDB.Get(req.Hash[:], nil)
	if err != nil {
		return err
	}

	var header BlockHeader
	err = json.Unmarshal(headerBytes, &header)
	if err != nil {
		return err
	}

	res.Header = header

	return nil
}

func (s *Client) FetchBlock(hash SHA256Sum, address string) (*Block, error) {
	peer, err := s.dialPeer(address)
	if err != nil {
		return nil, err
	}

	req := s.NewHashMsg(hash)
	res := BlockMsg{}

	err = peer.Call("GossipCore.FetchBlockRPC", &req, &res)
	if err != nil {
		return nil, err
	}
	block := &Block{}
	*block = res.Block

	if block.Header.Hash() != hash {
		return nil, errors.New("Wrong block")
	}

	if !block.VerifyMerkleHash() {
		return nil, errors.New("Wrong block")
	}

	return block, nil
}

func (gc *GossipCore) FetchBlockRPC(req HashMsg, res *BlockMsg) error {
	err := gc.c.HandleRPC(req.RPCHeader, &res.RPCHeader)
	if err != nil {
		return err
	}

	block, err := gc.c.LoadBlock(req.Hash)
	if err != nil {
		return err
	}

	res.Block = *block

	return nil
}

func (c *Client) FetchTxn(hash SHA256Sum, address string) (*Txn, error) {
	peer, err := c.dialPeer(address)
	if err != nil {
		return nil, err
	}

	req := c.NewHashMsg(hash)
	res := TxnMsg{}

	err = peer.Call("GossipCore.FetchTxnRPC", &req, &res)
	if err != nil {
		return nil, err
	}
	txn := &Txn{}
	*txn = res.Txn

	if txn.Hash() != hash {
		return nil, errors.New("Wrong txn")
	}

	return txn, nil
}

func (gc *GossipCore) FetchTxnRPC(req HashMsg, res *TxnMsg) error {
	err := gc.c.HandleRPC(req.RPCHeader, &res.RPCHeader)
	if err != nil {
		return err
	}

	txn, err := gc.c.LoadTxn(req.Hash)
	if err != nil {
		return err
	}

	res.Txn = *txn

	return nil
}

func (c *Client) FetchOutput(hash SHA256Sum, address string) (*Output, error) {
	peer, err := c.dialPeer(address)
	if err != nil {
		return nil, err
	}

	req := c.NewHashMsg(hash)
	res := OutputMsg{}

	err = peer.Call("GossipCore.FetchOutputRPC", &req, &res)
	if err != nil {
		return nil, err
	}
	output := &Output{}
	*output = res.Output

	if output.Hash() != hash {
		return nil, errors.New("Wrong output")
	}

	return output, nil
}

func (gc *GossipCore) FetchOutputRPC(req HashMsg, res *OutputMsg) error {
	err := gc.c.HandleRPC(req.RPCHeader, &res.RPCHeader)
	if err != nil {
		return err
	}

	output, err := gc.c.LoadOutput(req.Hash)
	if err != nil {
		return err
	}

	res.Output = *output

	return nil
}
