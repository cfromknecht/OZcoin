package ozcoin

import (
	"testing"

	"log"
	"math/big"
)

func TestOZRSSign(t *testing.T) {
	zero := &big.Int{}
	prevAmt := uint64(5000000000)
	prevBytes := UIntBytes(prevAmt)
	prevCommit := PedersenSum(zero.Bytes(), prevBytes)
	log.Println("Previous:", prevCommit)

	amts := []uint64{1, 4999999998}
	rcpts := []WalletPublicKey{
		NewPrivateKey().PublicKey(),
		NewPrivateKey().PublicKey(),
	}

	pks, sec := pksAndSecret()
	ics, yi := commitmentsAndBF(prevAmt)
	outputs, bf := BuildOutputs(amts, rcpts)

	txn := Txn{
		Body: TxnBody{
			Outputs: outputs,
			Fee:     1,
		},
	}

	txn.OZRSSign(pks, ics, sec, yi, 0, bf)

	if !txn.VerifyOZRS(pks, ics) {
		t.Error("OZRS Failed to verify")
	}
}

func pksAndSecret() ([]ECCPoint, *big.Int) {
	var sec *big.Int
	pks := []ECCPoint{}
	for i := 0; i < TXN_NUM_INPUTS; i++ {
		s := RandomInt()
		if i == 0 {
			sec = s
		}
		pkx, pky := CURVE.Params().ScalarBaseMult(s.Bytes())
		pks = append(pks, ECCPoint{pkx, pky})
	}

	log.Println("pk0:", pks[0])
	x, y := CURVE.Params().ScalarBaseMult(sec.Bytes())
	log.Println("sk0G:", ECCPoint{x, y})

	return pks, sec
}

func commitmentsAndBF(amt uint64) ([]ECCPoint, *big.Int) {
	var yi *big.Int
	ics := []ECCPoint{}
	for i := 0; i < TXN_NUM_INPUTS; i++ {
		//b := RandomInt()
		b := &big.Int{}
		commit := RangeCommit(amt, b)
		if i == 0 {
			yi = b
			log.Println("RangeCommit amt:", amt, "pt:", commit.ECCPoint)

			amtBytes := UIntBytes(amt)
			exp := PedersenSum(b.Bytes(), amtBytes)
			log.Println("Expected commit:", exp)
		}
		ics = append(ics, commit.ECCPoint)
	}

	return ics, yi
}
