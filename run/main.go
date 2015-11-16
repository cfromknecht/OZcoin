package main

import (
	"github.com/cfromknecht/zebracoin"

	"fmt"
	"unsafe"
)

func main() {
	rpcis, rpcos := zebracoin.CommitTxn([]uint64{1, 2, 3}, []uint64{6})
	verifies := zebracoin.SumZero(rpcis, rpcos)
	fmt.Println("verifies:", verifies)

	size := unsafe.Sizeof(rpcis)
	fmt.Println("size:", size)
}
