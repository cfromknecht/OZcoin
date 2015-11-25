package ozcoin

import (
	"encoding/json"
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

func (c *Client) HandleRPC(request RPCHeader, response *RPCHeader) error {
	err := c.PutPeer(request.Address)
	if err != nil {
		log.Println(err)
		return err
	}

	response.Address = c.Address

	return nil
}

type BcastRequest struct {
	RPCHeader
	Hash SHA256Sum
}

func (c *Client) NewBcastRequest(hash SHA256Sum) BcastRequest {
	return BcastRequest{
		RPCHeader: RPCHeader{
			Address: c.Address,
		},
		Hash: hash,
	}
}

func (c *Client) Bcast(hash SHA256Sum) error {
	peerDB := c.OpenPeerDB()
	defer peerDB.Close()

	iter := peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())

		go func() {
			peer, err := c.dialPeer(address)
			if err != nil {
				err = peerDB.Delete([]byte(address), nil)
				if err != nil {
					log.Println(err)
				}
				return
			}

			req := c.NewBcastRequest(hash)

			peer.Call("GossipCore.BcastRPC", &req, &RPCHeader{})
		}()
	}
	iter.Release()

	return iter.Error()
}

func (gc *GossipCore) BcastRPC(req BcastRequest, res *RPCHeader) error {
	err := gc.c.HandleRPC(req.RPCHeader, res)
	if err != nil {
		return err
	}

	gc.c.HashChan <- req

	return nil
}

func (s *Client) dialPeer(address string) (*rpc.Client, error) {
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}

	return rpc.NewClient(conn), nil
}

type RetrieveRequest struct {
	RPCHeader
	Hash SHA256Sum
}

func (s *Client) NewRetrieveRequest(hash SHA256Sum) RetrieveRequest {
	return RetrieveRequest{
		RPCHeader: RPCHeader{
			Address: s.Address,
		},
		Hash: hash,
	}
}

type RetrieveHeaderResponse struct {
	RPCHeader
	Header BlockHeader
}

func (s *Client) RetrieveHeader(hash SHA256Sum, address string) (*BlockHeader, error) {
	peer, err := s.dialPeer(address)
	if err != nil {
		return nil, err
	}

	req := s.NewRetrieveRequest(hash)
	res := RetrieveHeaderResponse{}

	err = peer.Call("GossipCore.RetrieveHeaderRPC", &req, &res)
	if err != nil {
		return nil, err
	}
	header := &BlockHeader{}
	*header = res.Header

	return header, nil
}

func (gc *GossipCore) RetrieveHeaderRPC(req RetrieveRequest, res RetrieveHeaderResponse) error {
	err := gc.c.HandleRPC(req.RPCHeader, &res.RPCHeader)
	if err != nil {
		return err
	}

	headerDB := gc.c.OpenHeaderDB()
	defer headerDB.Close()

	headerBytes, err := headerDB.Get(req.Hash[:], nil)
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

type RetrieveBlockResponse struct {
	RPCHeader
	Block
}

func (s *Client) RetrieveBlock(hash SHA256Sum, address string) (*Block, error) {
	peer, err := s.dialPeer(address)
	if err != nil {
		return nil, err
	}

	req := s.NewRetrieveRequest(hash)
	res := RetrieveBlockResponse{}

	err = peer.Call("GossipCore.RetrieveBlockRPC", &req, &res)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	block := &Block{}
	*block = res.Block

	return block, nil
}

func (gc *GossipCore) RetrieveBlockRPC(req RetrieveRequest, res RetrieveBlockResponse) error {
	err := gc.c.HandleRPC(req.RPCHeader, &res.RPCHeader)
	if err != nil {
		return err
	}

	blockDB := gc.c.OpenBlockDB()
	defer blockDB.Close()

	blockBytes, err := blockDB.Get(req.Hash[:], nil)
	if err != nil {
		return err
	}

	var block Block
	err = json.Unmarshal(blockBytes, &block)
	if err != nil {
		return err
	}

	res.Block = block

	return nil
}
