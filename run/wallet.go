package main

import (
	"github.com/cfromknecht/ozcoin"

	"log"
)

func main() {
	miningAddress := "127.0.0.1:6000"
	svpAddress := "127.0.0.1:6001"
	walletAddress := "127.0.0.1:6002"
	password := "test"

	ws := ozcoin.NewWalletServer(miningAddress, svpAddress, walletAddress, password)
	if ws == nil {
		log.Println("Could not create wallet server")
	}

	doneChan := make(chan []struct{})
	<-doneChan
}
