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
also not be added to sucessive blocks. So for now they are begign.

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

Run `rm -rf miner/db/*` to reset the blockcahin databases. Also remember to
reset the wallet databases.

Ozcoin writeup: OZRSwriteup.pdf

Website: jinglan.github.io/zebracoin