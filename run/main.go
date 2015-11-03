package main

import (
	"github.com/cfromknecht/cloakcoin"

	"fmt"
	"unsafe"
)

func main() {
	rpcis, rpcos := cloakcoin.CommitTxn([]uint64{1, 2, 3}, []uint64{6})
	verifies := cloakcoin.SumZero(rpcis, rpcos)
	fmt.Println("verifies:", verifies)

	size := unsafe.Sizeof(rpcis)
	fmt.Println("size:", size)
}
