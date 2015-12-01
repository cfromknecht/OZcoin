package ozcoin

import (
	db "github.com/syndtr/goleveldb/leveldb"

	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"
)

var (
	PASSWORD_KEY = []byte("password")
	SALT_KEY     = []byte("salt")
	TOKEN_KEY    = []byte("token")
)

type WalletServer struct {
	*Client
	Address    string
	AuthDBPath string
	PrivPDBath string
	TxnDBPath  string
	authDB     *db.DB
	privDB     *db.DB
	txnDB      *db.DB
	Privs      []WalletPrivateKey
	Outputs    []OutputPlaintext
}

func NewWalletServer(miningAddress, svpAddress, walletAddress, password string) *WalletServer {
	log.Println("Starting client with", svpAddress, walletAddress)
	ws := &WalletServer{
		Client:     NewSPV(svpAddress, walletAddress, password),
		Address:    walletAddress,
		AuthDBPath: "db/wallet-auth.db",
		PrivPDBath: "db/wallet-priv.db",
		TxnDBPath:  "db/wallet-txn.db",
	}

	log.Println("Registering with", miningAddress)
	err := ws.PutPeer(miningAddress)
	if err != nil {
		panic("Could not add peer")
	}

	go func() {
		t := time.After(5 * time.Second)
		_ = <-t

		log.Println("Broadcasting hello", miningAddress)
		err = ws.BcastBlock(SHA256Sum{})
		if err != nil {
			panic("Could not register with miner")
		}
	}()

	ws.authDB = ws.OpenAuthDB()
	ws.privDB = ws.OpenPrivDB()
	ws.txnDB = ws.OpenTxnDB()

	http.HandleFunc("/open", ws.handleOpen)
	http.HandleFunc("/tracking", ws.handleTracking)
	http.HandleFunc("/balance", ws.handleBalance)
	http.HandleFunc("/sign", ws.handleSign)
	http.HandleFunc("/new-block", ws.handleNewBlock)
	http.HandleFunc("/delete-block", ws.handleDeleteBlock)

	log.Println("Wallet Server listening", walletAddress)

	go http.ListenAndServe(walletAddress, nil)

	err = ws.Client.Wallet.OpenWallet(password)
	if err != nil {
		log.Println(err)
		panic("Could not authenticate wallet")
	}

	return ws
}

type OutputPlaintext struct {
	Output  *Output `json:"output"`
	HashPub string  `json:"hash_pub"`
	Time    string  `json:"time"`
	Amount  uint64  `json:"amount"`
	Height  uint64  `json:"height"`
}

func (op *OutputPlaintext) Json() []byte {
	b, err := json.Marshal(op)
	if err != nil {
		panic(err)
	}

	return b
}

type BalanceMsg struct {
	Balance uint64            `json:"balance"`
	Outputs []OutputPlaintext `json:"outputs"`
}

type TrackingKeysMsg struct {
	TrackingKeys []WalletTrackingKey `json:"track_keys"`
}

type PasswordMsg struct {
	Password string `json:"password"`
}

type TokenMsg struct {
	Token SHA256Sum `json:"token"`
}

type KeyMsg struct {
	Key WalletPublicKey `json:"key"`
}

/*
 * Route handlers
 */

func (ws *WalletServer) handleOpen(w http.ResponseWriter, r *http.Request) {
	log.Println("handleOpen")
	decoder := json.NewDecoder(r.Body)
	var req PasswordMsg
	err := decoder.Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token, err := ws.Open(req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if (token == SHA256Sum{}) {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	res := TokenMsg{
		Token: token,
	}
	jsonWrite(w, res)
}

func (ws *WalletServer) handleBalance(w http.ResponseWriter, r *http.Request) {
	if err := ws.authenticateToken(w, r); err != nil {
		return
	}

	// Refresh wallet
	err := ws.Refresh()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	balance := uint64(0)
	lightOutputs := []OutputPlaintext{}
	for _, o := range ws.Outputs {
		balance += o.Amount
		lo := OutputPlaintext{
			HashPub: o.HashPub,
			Time:    o.Time,
			Amount:  o.Amount,
		}
		lightOutputs = append(lightOutputs, lo)
	}

	log.Println("balance:", balance)

	res := BalanceMsg{
		Balance: balance,
		Outputs: lightOutputs,
	}
	jsonWrite(w, res)
}

func (ws *WalletServer) handleTracking(w http.ResponseWriter, r *http.Request) {
	if err := ws.authenticateToken(w, r); err != nil {
		return
	}

	// Refresh wallet
	err := ws.Refresh()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check for private keys
	if len(ws.Privs) == 0 {
		err = errors.New("No private keys")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	addrs := []WalletTrackingKey{}
	for _, priv := range ws.Privs {
		log.Println("priv:", priv)
		log.Println("priv.tracking:", priv.TrackingKey())
		addrs = append(addrs, priv.TrackingKey())
	}

	res := TrackingKeysMsg{
		TrackingKeys: addrs,
	}
	jsonWrite(w, res)
}

type SignMsg struct {
	Address WalletPublicKey `json:"address"`
	Amount  uint64          `json:"amount"`
	Fee     uint64          `json:"fee"`
}

func (ws *WalletServer) handleSign(w http.ResponseWriter, r *http.Request) {
	// Require token
	if err := ws.authenticateToken(w, r); err != nil {
		return
	}

	// Refresh wallet
	err := ws.Refresh()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	decoder := json.NewDecoder(r.Body)
	var req SignMsg
	err = decoder.Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check input validity
	if req.Address.PPK.Empty() || req.Address.TPK.Empty() {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Amount >= uint64(1)<<RANGE_PROOF_LENGTH {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	total := req.Amount + req.Fee

	// Find funding transaction
	fundingTxn, priv := ws.findFundingTxn(total)
	if fundingTxn == nil {
		err = errors.New("Cannot find funding txn")
		log.Println(err)
		http.Error(w, err.Error(), 422)
		return
	}

	log.Println("Finding random outputs")
	inputs, err := ws.RandomOutputs()
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	inputs[0] = *fundingTxn.Output

	sk := fundingTxn.Output.ComputeTxnPrivateKey(*priv)
	yi := fundingTxn.Output.ComputeBlindingFactor(*priv)

	var amts []uint64
	var rcpts []WalletPublicKey
	if req.Amount+req.Fee == fundingTxn.Amount {
		amts = []uint64{req.Amount}
		rcpts = []WalletPublicKey{req.Address}
	} else {
		amts = []uint64{req.Amount, fundingTxn.Amount - total}
		rcpts = []WalletPublicKey{req.Address, ws.Privs[0].PublicKey()}
	}

	txn := ws.NewTxn(inputs, sk, yi, 0, amts, rcpts, req.Fee)

	ws.TxnChan <- *txn

	jsonWrite(w, txn)
}

func (c *Client) RandomOutputs() ([]Output, error) {
	outputs := []Output{}
	iter := c.dbm.mapDB.NewIterator(nil, nil)
	for iter.Next() {
		pimgHash := iter.Key()
		blockHash := iter.Value()

		log.Println("pimgHash:", string(pimgHash))
		log.Println("blockHash:", string(blockHash))

		if len(pimgHash) != SHA256_SUM_LENGTH || len(blockHash) != SHA256_SUM_LENGTH {
			continue
		}

		blockSum := SHA256Sum{}
		for i, b := range blockHash {
			blockSum[i] = b
		}

		log.Println("finding block")
		block, err := c.FindBlock(blockSum)
		if err != nil {
			continue
		}

		log.Println("INPUT BLOCK HEADER:", block.Header)

		for _, txn := range block.Txns {
		NextOutput:
			for _, output := range txn.Body.Outputs {
				for i, b := range output.Hash() {
					if pimgHash[i] != b {
						goto NextOutput
					}
				}

				outputs = append(outputs, output)

				if len(outputs) == TXN_NUM_INPUTS {
					return outputs, nil
				}
			}
		}
	}

	return nil, errors.New("Not enough txns found")
}

func (ws *WalletServer) handleNewBlock(w http.ResponseWriter, r *http.Request) {
	log.Println("handleNewBlock")
	// Require token
	if err := ws.authenticateToken(w, r); err != nil {
		return
	}

	// Refresh wallet
	err := ws.Refresh()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Decode block
	log.Println("Decoding block")
	decoder := json.NewDecoder(r.Body)
	var block Block
	err = decoder.Decode(&block)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Println("Looking for my txns")
	// Save outputs belong to me
	err = ws.saveMyTxns(block)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond success
	w.WriteHeader(http.StatusAccepted)
	w.Write(nil)
}

func (ws *WalletServer) handleDeleteBlock(w http.ResponseWriter, r *http.Request) {
	// Require token
	if err := ws.authenticateToken(w, r); err != nil {
		return
	}

	// Refresh
	err := ws.Refresh()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Decode block
	decoder := json.NewDecoder(r.Body)
	var block Block
	err = decoder.Decode(&block)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Delete all outputs
	err = ws.deleteTxns(block)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond success
	w.WriteHeader(http.StatusAccepted)
	w.Write(nil)
}

/*
 * Internal methods
 */

func (ws *WalletServer) Open(password string) (SHA256Sum, error) {
	token, err := ws.AuthenicatePassword(password)
	if err != nil || (token == SHA256Sum{}) {
		return ws.Create(password)
	}

	return token, ws.Refresh()
}

func (ws *WalletServer) AuthenicatePassword(password string) (SHA256Sum, error) {
	log.Println("Authenticating wallet")

	saltBytes, err := ws.authDB.Get(SALT_KEY, nil)
	if err != nil {
		return SHA256Sum{}, err
	}
	hashBytes, err := ws.authDB.Get(PASSWORD_KEY, nil)
	if err != nil {
		return SHA256Sum{}, err
	}

	if saltBytes == nil || hashBytes == nil {
		return SHA256Sum{}, errors.New("No wallet credentials in database")
	}

	bytes := append(saltBytes, []byte(password)...)
	hash := Hash(bytes)

	for i, b := range hashBytes {
		if i >= SHA256_SUM_LENGTH {
			return SHA256Sum{}, nil
		}
		if b != hash[i] {
			return SHA256Sum{}, errors.New("Authentication unsucessful")
		}
	}

	// Load token
	tokenBytes, err := ws.authDB.Get(TOKEN_KEY, nil)
	if err != nil {
		return SHA256Sum{}, err
	}

	token := SHA256Sum{}
	for i := 0; i < SHA256_SUM_LENGTH; i++ {
		token[i] = tokenBytes[i]
	}

	return token, nil
}

func (ws *WalletServer) Create(password string) (SHA256Sum, error) {
	log.Println("Creating wallet")
	salt := RandomInt()
	bytes := append(salt.Bytes(), []byte(password)...)
	hash := Hash(bytes)

	token := Hash(RandomInt().Bytes())

	err := ws.authDB.Put(PASSWORD_KEY, hash[:], nil)
	if err != nil {
		return SHA256Sum{}, err
	}
	err = ws.authDB.Put(SALT_KEY, salt.Bytes(), nil)
	if err != nil {
		return SHA256Sum{}, err
	}
	err = ws.authDB.Put(TOKEN_KEY, token[:], nil)
	if err != nil {
		return SHA256Sum{}, err
	}

	address := NewPrivateKey()
	addressHash := address.Hash()
	err = ws.privDB.Put(addressHash.Bytes(), address.Json(), nil)
	if err != nil {
		return SHA256Sum{}, err
	}

	return token, ws.Refresh()
}

func (ws *WalletServer) Refresh() error {
	log.Println("Refreshing wallet")
	privs := []WalletPrivateKey{}
	iter := ws.privDB.NewIterator(nil, nil)
	for iter.Next() {

		privBytes := iter.Value()
		var priv WalletPrivateKey
		err := json.Unmarshal(privBytes, &priv)
		if err != nil {
			return err
		}

		privs = append(privs, priv)
	}

	ws.Privs = privs

	txns := []OutputPlaintext{}
	iter = ws.txnDB.NewIterator(nil, nil)
	for iter.Next() {
		txnBytes := iter.Value()
		var txn OutputPlaintext
		err := json.Unmarshal(txnBytes, &txn)
		if err != nil {
			return err
		}

		txns = append(txns, txn)
	}

	ws.Outputs = txns

	return nil
}

func (ws *WalletServer) authenticateToken(w http.ResponseWriter, r *http.Request) error {
	log.Println("Authenticateing token")
	log.Println("Header:", r.Header)

	cookie, err := r.Cookie("X-Wallet-Token")
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return err
	}
	token, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return err
	}

	if len(token) != SHA256_SUM_LENGTH {
		err := errors.New("Invalid token length")
		log.Println(err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return err
	}

	tokenBytes, err := ws.authDB.Get(TOKEN_KEY, nil)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	for i, b := range token {
		if b != tokenBytes[i] {
			err = errors.New("Invalid token")
			log.Println(err)
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return err
		}
	}

	log.Println("Authentication successful")

	return nil
}

func jsonWrite(w http.ResponseWriter, o interface{}) {
	b, err := json.Marshal(o)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(b)
}

func (ws *WalletServer) findFundingTxn(amount uint64) (*OutputPlaintext, *WalletPrivateKey) {
	var skPtr *WalletPrivateKey
	fundingTxn := &OutputPlaintext{}
	found := false
	for _, txn := range ws.Outputs {
		if amount <= txn.Amount {
			if !found || txn.Amount < fundingTxn.Amount {
				*fundingTxn = txn
				skPtr = nil
				for _, priv := range ws.Privs {
					if fundingTxn.Output.BelongsToMe(priv.TrackingKey()) {
						skPtr = &priv
					}
				}
				found = true
			}
		}
	}

	if !found || skPtr == nil {
		return nil, nil
	}

	sk := &WalletPrivateKey{}
	*sk = *skPtr

	return fundingTxn, sk
}

func (ws *WalletServer) saveMyTxns(b Block) error {
	coinbase := CoinbaseValue(b.Header.SeqNum)
	for _, txn := range b.Txns {
		coinbase += txn.Body.Fee
	}
	// Iterate over all output-secret combinations
	for i, txn := range b.Txns {
		for _, priv := range ws.Privs {
			for _, output := range txn.Body.Outputs {
				// Save decrypted output if belongs to me
				if output.BelongsToMe(priv.TrackingKey()) {
					// Create plaintext output
					var outputPlaintext *OutputPlaintext
					if i == 0 {
						log.Println("Decrypting coinbase txn")
						outputPlaintext = output.DecryptCoinbase()
						outputPlaintext.Amount = coinbase
					} else {
						log.Println("Decrypting txn")
						outputPlaintext = output.Decrypt(priv)
					}

					if outputPlaintext == nil {
						log.Println("Unable to decrypt txn")
						continue
					}

					outputPlaintext.Time = b.Header.Time.String()
					outputPlaintext.Height = b.Header.SeqNum

					// Write to database
					hash := output.Hash()
					log.Println("Writing my output to database:", outputPlaintext)
					err := ws.txnDB.Put(hash[:], outputPlaintext.Json(), nil)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (ws *WalletServer) deleteTxns(b Block) error {
	for _, txn := range b.Txns {
		for _, output := range txn.Body.Outputs {
			hash := output.Hash()
			err := ws.txnDB.Delete(hash[:], nil)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
