package ozcoin

import (
	"encoding/json"
	"math/big"
)

type WalletPublicKey struct {
	TPK ECCPoint `json:"pk_track"`
	PPK ECCPoint `json:"pk_priv"`
}

type WalletTrackingKey struct {
	WalletPublicKey
	TSK *big.Int `json:"sk_track"`
}

type WalletPrivateKey struct {
	WalletTrackingKey
	PSK *big.Int `json:"sk_priv"`
}

func (track WalletTrackingKey) PublicKey() WalletPublicKey {
	return track.WalletPublicKey
}

func (priv WalletPrivateKey) PublicKey() WalletPublicKey {
	return priv.WalletPublicKey
}

func (priv WalletPrivateKey) TrackingKey() WalletTrackingKey {
	return priv.WalletTrackingKey
}

func NewPrivateKey() *WalletPrivateKey {
	tsk := RandomInt()
	psk := RandomInt()

	tskx, tsky := CURVE.Params().ScalarBaseMult(tsk.Bytes())
	pskx, psky := CURVE.Params().ScalarBaseMult(psk.Bytes())

	w := &WalletPrivateKey{
		WalletTrackingKey: WalletTrackingKey{
			WalletPublicKey: WalletPublicKey{
				TPK: ECCPoint{tskx, tsky},
				PPK: ECCPoint{pskx, psky},
			},
			TSK: tsk,
		},
		PSK: psk,
	}

	return w
}

func (w *WalletPublicKey) Json() []byte {
	b, err := json.Marshal(w)
	if err != nil {
		panic(err)
	}

	return b
}

func (w *WalletPrivateKey) Json() []byte {
	b, err := json.Marshal(w)
	if err != nil {
		panic(err)
	}

	return b
}

func (w *WalletPublicKey) Hash() SHA256Sum {
	return Hash(w.Json())
}
