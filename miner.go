package ozcoin

import (
	"log"
	"time"
)

type Miner struct {
	*Client
}

func NewMiner(miningAddr, walletAddr string, password string) *Miner {
	m := &Miner{
		Client: NewBlockchain(miningAddr, walletAddr, password),
	}

	err := m.Client.Wallet.OpenWallet(password)
	if err != nil {
		log.Println(err)
		panic("Could not authenticate wallet")
	}

	go m.run()

	return m
}

func (m *Miner) run() {
	log.Println("Running miner...")

	addresses, err := m.Wallet.TrackingKeys()
	if err != nil {
		panic("Could not load addresses from wallet")
	}
	miningAddress := addresses[0].PublicKey()

	gblock := GenesisBlock(miningAddress)
	err = m.WriteBlock(gblock)
	if err != nil {
		panic("Could not add genesis block")
	}

	updateTime := time.After(30 * time.Second)
	recentlyUpdated := false

	m.LastHeader = gblock.Header

	last := m.LastHeader.Hash()
	block := m.NewBlock(m.LastHeader, miningAddress)
	for {
		select {
		case now := <-updateTime:
			log.Println("Updating block time")
			if !recentlyUpdated {
				block.Header.Time = now
				block.Header.Difficulty = m.ComputeDifficulty(block)
			}

			updateTime = time.After(30 * time.Second)
			recentlyUpdated = false

			// Sign request for testing
			_, err = m.Wallet.SignTxn(&miningAddress, 1, 1)
			if err != nil {
				log.Println(err)
			}

		default:
			if last != m.LastHeader.Hash() {
				log.Println("Building new block")
				// Create new block
				last = m.LastHeader.Hash()
				block = m.NewBlock(m.LastHeader, miningAddress)
				recentlyUpdated = true
			} else {
				// Increment nonce
				block.Header.Nonce += 1
			}

			if !block.Header.ValidPoW() {
				continue
			}

			// Inform self
			log.Println("Block found")
			m.BlockChan <- block
		}
	}
}
