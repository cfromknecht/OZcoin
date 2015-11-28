package main

import (
	"github.com/cfromknecht/ozcoin"

	"log"
)

func main() {
	miningAddress := "127.0.0.1:6000"
	walletAddress := "127.0.0.1:6002"
	password := "test"

	miner := ozcoin.NewMiner(miningAddress, walletAddress, password)
	if miner == nil {
		log.Println("Could not create miner")
	}

	doneChan := make(chan []struct{})
	<-doneChan
}
