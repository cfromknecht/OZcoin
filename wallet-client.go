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

/*
 * WalletClient
 *
 * Provides a representation of the current plaintext outputs and, public keys
 * belonging to a user.  Note that the secret keys remain on the server.
 */

type WalletClient struct {
	Address     string
	WalletToken SHA256Sum
	Keys        []WalletTrackingKey
	Outputs     []OutputPlaintext
}

/*
 * Attempts to authorize the client to the server.  If successful, the client
 * receives a token to provide in future requests.
 */
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

/*
 * Retrieves the tracking keys used during mining.
 */
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

/*
 * Informs the wallet server of a new block, storing plaintext outputs if they
 * belong to me.
 */
func (wc *WalletClient) NewBlock(b Block) error {
	_, err := wc.POST("/new-block", b.Json())
	return err
}

/*
 * Informs the wallet server to delete a block, removing plaintext outputs if
 * they belong to me.
 */
func (wc *WalletClient) DeleteBlock(b Block) error {
	_, err := wc.POST("/delete-block", b.Json())
	return err
}

/*
 * Sends the address, amount, and fee to the wallet server to be signed.  If
 * successful, the server initiated a broadcast to the miner's txn pool.
 */

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

/*
 * Retrieves the balance and plaintext outputs from the wallet-server.
 */

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

/*
 * Stub for all POST requests made to the wallet server.  Attaches cookie to all
 * outgoing connections unless performing initial authentication.
 */
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

/*
 * Base64 encodes a token to be sent as a cookie.
 */
func (wc *WalletClient) MarshalToken() string {
	return base64.StdEncoding.EncodeToString(wc.WalletToken[:])
}

/*
 * Formats a password in a JSON PasswordMsg.
 */
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
