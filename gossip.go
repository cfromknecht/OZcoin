package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

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

func (s *Client) HandleRPC(request RPCHeader, response *RPCHeader) error {
	err := s.WritePeer(request.Address)
	if err != nil {
		log.Println(err)
		return err
	}

	response.Address = s.Address

	return nil
}

func (s *Client) OpenPeerDB() *db.DB {
	peerDB, err := db.OpenFile(s.PeerDBPath, nil)
	if err != nil {
		log.Println(err)
		panic("Unable to open peer database")
	}

	return peerDB
}

func (s *Client) WritePeer(address string) error {
	peerDB := s.OpenPeerDB()
	defer peerDB.Close()

	return peerDB.Put([]byte(address), []byte{}, nil)
}

type BcastRequest struct {
	RPCHeader
	Hash SHA256Sum
}

func (s *Client) NewBcastRequest(hash SHA256Sum) BcastRequest {
	return BcastRequest{
		RPCHeader: RPCHeader{
			Address: s.Address,
		},
		Hash: hash,
	}
}

func (s *Client) Bcast(hash SHA256Sum) error {
	peerDB := s.OpenPeerDB()
	defer peerDB.Close()

	iter := peerDB.NewIterator(nil, nil)
	for iter.Next() {
		address := string(iter.Key())

		go func() {
			peer, err := s.dialPeer(address)
			if err != nil {
				err = peerDB.Delete([]byte(address), nil)
				if err != nil {
					log.Println(err)
				}
				return
			}

			req := s.NewBcastRequest(hash)

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

func (s *Client) RetrieveHeader(hash SHA256Sum, address string) (BlockHeader, error) {
	peer, err := s.dialPeer(address)
	if err != nil {
		log.Println(err)
		continue
	}

	req := s.NewRetrieveRequest(hash)
	res := RetrieveHeaderResponse{}

	err = peer.Call("GossipCore.RetrieveHeaderRPC", &req, &res)
	if err != nil {
		log.Println(err)
		continue
	}

	return res.Header, nil
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

func (s *Client) RetrieveBlock(hash SHA256Sum, address string) (Block, error) {
	peer, err := s.dialPeer(address)
	if err != nil {
		log.Println(err)
		continue
	}

	req := s.NewRetrieveRequest(hash)
	res := RetrieveBlockResponse{}

	err = peer.Call("GossipCore.RetrieveBlockRPC", &req, &res)
	if err != nil {
		log.Println(err)
		continue
	}

	return res.Block, nil
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
