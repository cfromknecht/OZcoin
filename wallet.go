package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

type WalletClient struct {
	Address     string
	WalletToken SHA256Sum
	Keys        []WalletTrackingKey
	Outputs     []OutputPlaintext
}

func (wp *WalletClient) OpenWallet(password string) error {
	serializedPassword, err := MarshalPassword(password)
	if err != nil {
		return err
	}

	log.Println("POST /open")
	bytes, err := wp.POST("/open", serializedPassword)
	if err != nil {
		return err
	}

	log.Println("Unmarshalling token")
	var tm TokenMsg
	err = json.Unmarshal(bytes, &tm)
	if err != nil {
		return err
	}

	log.Println("Setting WalletToken")
	wp.WalletToken = tm.Token

	return nil
}

func (wc *WalletClient) TrackingKeys() ([]WalletTrackingKey, error) {
	bytes, err := wc.POST("/tracking", nil)
	if err != nil {
		return nil, err
	}

	var tkm TrackingKeysMsg
	err = json.Unmarshal(bytes, &tkm)
	if err != nil {
		return nil, err
	}

	return tkm.TrackingKeys, nil
}

func (wc *WalletClient) NewBlock(b Block) error {
	_, err := wc.POST("/new-block", b.Json())
	return err
}

func (wc *WalletClient) DeleteBlock(b Block) error {
	_, err := wc.POST("/delete-block", b.Json())
	return err
}

func (wc *WalletClient) SignTxn(addr *WalletPublicKey, amt, fee uint64) (*Txn, error) {
	signMsg := SignMsg{
		Address: *addr,
		Amount:  amt,
		Fee:     fee,
	}
	b, err := json.Marshal(signMsg)
	if err != nil {
		return nil, err
	}

	bytes, err := wc.POST("/sign", b)
	if err != nil {
		log.Println("Sign RPC failed", err)
		return nil, err
	}

	log.Println("Bytes:", string(bytes))

	txn := &Txn{}
	err = json.Unmarshal(bytes, txn)
	if err != nil {
		log.Println("Json marhsalling failed", err)
		return nil, err
	}

	return txn, nil
}

func (wc *WalletClient) Balance() (*BalanceMsg, error) {
	bytes, err := wc.POST("/balance", nil)
	if err != nil {
		return nil, err
	}

	bm := &BalanceMsg{}
	err = json.Unmarshal(bytes, bm)
	if err != nil {
		return nil, err
	}

	return bm, nil
}

func (wc *WalletClient) POST(relativeUrl string, data []byte) ([]byte, error) {
	url := "http://" + wc.Address + relativeUrl
	dataBuf := bytes.NewBuffer(data)
	req, err := http.NewRequest("POST", url, dataBuf)
	if err != nil {
		return nil, err
	}

	// Require cookie on all requests except open
	if relativeUrl != "/open" {
		token := wc.MarshalToken()
		cookie := &http.Cookie{
			Name:  "X-Wallet-Token",
			Value: token,
		}
		req.AddCookie(cookie)
	}

	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Println("Sending request:", relativeUrl)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (wc *WalletClient) MarshalToken() string {
	return base64.StdEncoding.EncodeToString(wc.WalletToken[:])
}

func MarshalPassword(password string) ([]byte, error) {
	ar := PasswordMsg{
		Password: password,
	}

	b, err := json.Marshal(ar)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (w *WalletServer) OpenAuthDB() *db.DB {
	authDB, err := db.OpenFile(w.AuthDBPath, nil)
	if err != nil {
		log.Println("[OpenAuthDB]:", err)
		panic(err)
	}

	return authDB
}

func (w *WalletServer) OpenPrivDB() *db.DB {
	privDB, err := db.OpenFile(w.PrivPDBath, nil)
	if err != nil {
		log.Println("[OpenPrivDB]:", err)
		panic(err)
	}

	return privDB
}

func (w *WalletServer) OpenTxnDB() *db.DB {
	txnDB, err := db.OpenFile(w.TxnDBPath, nil)
	if err != nil {
		log.Println("[OpenTxnDB]:", err)
		panic(err)
	}

	return txnDB
}
