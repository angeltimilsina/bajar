# Phase 1 local consensus devnet

Historical Ed25519 kvstore demo. For the new BAJ ledger and ML-DSA-65 local
validators, use [LEDGER.md](LEDGER.md). The current supply is **108B BAJ** at 9
decimals, superseding the 100B proposal recorded below. The two devnets use
separate data directories, keys and ports.

This runs four independent CometBFT **v0.40.0** processes under Linux or
WSL2 Ubuntu with the bundled `persistent_kvstore` example. It does not issue
BAJ, implement transfers, wallets, fees, staking, permissioned admission rules,
or post-quantum consensus. Local validators have equal voting power without BAJ
stake. Do not submit validator-update transactions to this example app.

## Prerequisites and reproduction

Use Python 3.9+ and Go compatible with the pinned CometBFT module (currently
Go 1.25+). Docker is optional and is not used. From the repository root in WSL:

```bash
go version
python3 --version
go test ./...
go run ./cmd/bajarcheck network/spec.json
go install github.com/cometbft/cometbft/cmd/cometbft@v0.40.0
export PATH="$(go env GOPATH)/bin:$PATH"
cometbft version
python3 -m unittest discover -s scripts -p 'test_*.py' -v
bash scripts/start-devnet.sh
bash scripts/status-devnet.sh
bash scripts/smoke-devnet.sh
curl http://localhost:26657/status
bash scripts/stop-devnet.sh
```

On Windows enter `wsl` first and change to the repository's `/mnt/c/...` path.
If Go is installed in `.tools/go`, add `$PWD/.tools/go/bin` to PATH and set
`GOBIN="$PWD/.tools/bin"`; add that bin directory to PATH too.

Start generates identities only on first use using CometBFT's `testnet` command,
then verifies identical genesis and four distinct equal-power Ed25519 validators.
It verifies the embedded module version and checksum with `go version -m`
and rejects replaced modules. Go must remain on PATH for startup. The upstream
v0.40.0 module's CLI fallback string prints `0.39.0`; this is why startup checks
build metadata instead. Ports must be free:

| Node | P2P (loopback) | RPC (loopback) |
| --- | --- | --- |
| 0 | 26656 | 26657 |
| 1 | 26666 | 26667 |
| 2 | 26676 | 26677 |
| 3 | 26686 | 26687 |

Each node connects to the other three. Peer discovery is disabled, duplicate IPs
are allowed for this single-host setup, and RPC/P2P bind only to 127.0.0.1.
Start waits up to about a minute for readiness and runs the smoke test. The test
checks all four heights increase, three peers per node, four validators, distinct
node IDs, a common block ID at a committed height, and at least three commit
signatures. Failure exits nonzero. Start failure stops its newly started nodes.
Stop sends SIGTERM only to recorded processes whose command line contains the
expected node home, and waits for exit. Start refuses an existing process record;
use stop before restarting. Logs are `.devnet/node0.log` through `node3.log`.

All generated files live under ignored `.devnet/`. Never stage or copy its keys
into source control. Stop preserves validator signing state, databases and keys;
restart reuses them. Never delete signing state while reusing an identity. To
make a fresh disposable chain, stop it and move the entire `.devnet` directory
outside the repository, then start again (all identities and genesis regenerate).
Linux restrictive modes are applied; Windows-mounted WSL folders may inherit
Windows ACLs instead. Keep the Windows workspace private or use a Linux filesystem
checkout for stronger filesystem isolation. Validator identities are not wallets.

## Protocol boundary and next-phase plan

The revised BAJ specification is 108,000,000,000 BAJ with 9 decimals:
108,000,000,000,000,000,000 base units, represented as arbitrary-precision integers.
No kvstore demo genesis allocation represents BAJ. The former 20-billion reserve,
remaining tokenomics allocation and separate 8-billion reserve policies stay undecided.

The user approved Phase 2 on 2026-10-23; the implementation is documented in
[LEDGER.md](LEDGER.md). The original plan was to replace the example app with an ABCI++ ledger:
validate genesis supply and balances in InitChain; validate canonical transactions,
chain IDs, signatures, nonces and funds in CheckTx; deterministically select and
validate proposals in PrepareProposal/ProcessProposal; apply ordered transfers,
fees and nonces in FinalizeBlock; atomically persist state and height in Commit
and return a deterministic state root. Info must support crash recovery and replay.
Use supported cryptographic libraries, explicit validator-governance rules, and
tests for duplicate transactions, supply conservation, restart/replay and divergent
proposals. CometBFT remains the consensus engine. The legacy Phase 1 scripts still use kvstore.

Official versioned references:

- [Testnet generation](https://github.com/cometbft/cometbft/blob/v0.40.0/cmd/cometbft/commands/testnet.go)
- [ABCI++ example application](https://github.com/cometbft/cometbft/blob/v0.40.0/abci/example/kvstore/kvstore.go)
- [ML-DSA-65 key encoding support](https://github.com/cometbft/cometbft/blob/v0.40.0/crypto/encoding/codec.go)
- [Toolchain requirements](https://github.com/cometbft/cometbft/blob/v0.40.0/go.mod)

ML-DSA-65 is not enabled in this legacy demo. The separate ledger devnet enables
it explicitly and tests commit signatures; production review and performance testing remain.
