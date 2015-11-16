package zebracoin

import (
	"math/big"
)

type WalletPublicKey struct {
	TPK ECCPoint // Tracking Public Key
	PPK ECCPoint // Private Public Key
}

type WalletTrackingKey struct {
	WalletPublicKey
	TSK *big.Int // Tracking Secret Key
}

type WalletPrivateKey struct {
	WalletTrackingKey
	PSK *big.Int // Private Secret Key
}

func (track WalletTrackingKey) StandardAddress() WalletPublicKey {
	return track.WalletPublicKey
}

func (priv WalletPrivateKey) StandardAddress() WalletPublicKey {
	return priv.WalletTrackingKey.StandardAddress()
}

func (priv WalletPrivateKey) TrackingAddress() WalletTrackingKey {
	return priv.WalletTrackingKey
}
