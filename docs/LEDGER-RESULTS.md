# Ledger verification — 2026-10-23

Environment: WSL2 Ubuntu, Linux x86_64, Go 1.25.0. The existing Go archive was
extracted to a native Linux tool directory because the Windows-mounted
toolchain stalled. Add that toolchain's `bin` directory to PATH. CometBFT
is pinned to v0.40.0 through go.mod/go.sum and embedded in `.tools/bin/bajard`.

Passed:

- `go test ./...`: 14 top-level tests plus invalid-transaction subtests. Includes
  specification invariants; genesis/key separation and overwrite refusal; transfers
  larger than signed int64; exact fees and supply; self-transfers; exhausted nonces;
  invalid signatures/amounts/chain IDs; canonical encoding; proposal selection and
  rejection; crash-before-commit replay; reopen-after-commit; and injected storage
  failure without publishing uncommitted state.
- `go vet ./...`.
- `go run ./cmd/bajarcheck network/spec.json`: VALID, 108B BAJ,
  108000000000000000000 base units, 9 decimals, ML-DSA-65 targets, no validator stake.
- `go test github.com/cometbft/cometbft/crypto/mldsa65`: upstream key generation,
  round trips, deterministic seed derivation, sign/verify and tamper checks.
- `go mod verify`: all downloaded modules verified against recorded checksums.
- `python3 -m unittest discover -s scripts -v`: three legacy tooling tests.
- `bash -n scripts/build-ledger.sh` and Python byte-compilation of ledger-devnet.py.
- `bash scripts/build-ledger.sh` and `python3 scripts/ledger-devnet.py e2e`.
  Four independent nodes advanced from height 26 to 28, agreed on committed block
  and app root, and cryptographically verified ML-DSA-65 commits on every node.
  A 1.234567890 BAJ transfer charged exactly 0.000001 BAJ. All four balances,
  nonces, fees, locked reserves and total supply matched expected values.
  Forged transactions and replays were rejected. Stop/restart preserved balances
  and replay protection; every node advanced from 31 to 33. After node 3 stopped,
  the remaining quorum advanced at least two heights; height 35's commit had
  exactly three participating signatures and passed cryptographic verification.
- Git ignore checks matched wallet keys, validator keys and the built binary.

The integration test uses committed block data rather than the asynchronous
transaction index. Replay is tested with CheckTx to bypass CometBFT's duplicate
cache and exercise ledger nonce validation directly. Earlier harness runs exposed
these RPC/cache behaviors; they were corrected before the successful complete run.

Nodes were stopped after testing. Local test wallets, signing state and databases
remain under ignored `.ledger-devnet/`. The legacy `.devnet/` was not modified.

Not completed: race detector (`go test -race ./...` requires cgo and a C compiler,
which this WSL environment lacks); the full upstream CometBFT suite; Byzantine
equivocation/partition injection; sustained load benchmarks; NIST module validation;
independent cryptographic/security audits; PQ transport; dynamic validator governance;
public deployment or real-value transfers. Local signature integration is not a
production post-quantum security claim.
