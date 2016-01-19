OZcoin
======

Pretty Good Money.

OZcoin builds upon recent developments in Confidential Transactions for ring
signature schemes to deliver higher anonymity and perfect secrecy for
transaction values.  This anonymity is further improved by standardizing
transaction sizes and providing a default level of anonymity for all users.
Mixing coins is done by repeatedly sending to an owned address; note that the 
holder never loses control of his own coins.  Together, these properties of
anonymity, confidentiality, and uniformity make OZcoin transactions look more or
less indistinguishable from each other, and subject to less discrimination that
plaintext transactions.  OZcoin uses this transaction equality to provide a
fungible cryptocurrency.

This project was developed as a demo the CCN Borderless Block Party hackathon,
it should not be regarded as an official software release in its current state.

Dependencies
============
`github.com/syndtr/goleveldb/leveldb`

Running
=======
The current demo creates a mining client and wallet client.  The mining client
produces new blocks and broadcasts them to the wallet client.  Every 30 seconds,
the miner initiates a sign request to the wallet server. The txn is constructed,
signed, and broadcast to its the wallet client's peers.  After receiving the txn
from the wallet server, the mining client incorporates the new txn into the next
block and broadcasts the new block.  The wallet client then decrypts and 
collects both the coinbase txn and the signed txn to itself.

Note: Currently, the txn's aren't properly removed the the txn pool, but will
also not be added to successive blocks. So for now they are benign.

Wallet Client
=====================

Run `go run wallet/run.go`.
This runs an SVP client that watches for incoming txns to the receiving address.

Run `rm -rf wallet/db/*` to reset the wallet databases.

Mining Client
=====================

Run `go run miner/run.go`
This runs a mining client that mines new blocks and accepts txn broadcasts.
Make sure to run this within 5 seconds of starting the wallet client.

Run `rm -rf miner/db/*` to reset the blockchain databases. Also remember to
reset the wallet databases.

Ozcoin writeup: OZRSwriteup.pdf

Website: jinglan.github.io/zebracoin

Related Works
=============

Similar work for adapting CT to ring signatures can be found [here](https://eprint.iacr.org/2015/1098.pdf) courtesy of MRL.
