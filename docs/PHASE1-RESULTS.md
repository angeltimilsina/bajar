# Phase 1 verification — 2026-10-21

Historical results for the earlier 100B specification and Ed25519 kvstore demo.
The 2026-10-23 revision uses 108B BAJ; see [ledger documentation](LEDGER.md).

Verified on WSL2 Ubuntu, Linux x86_64, using Go 1.25.0. Go was provisioned
locally; its downloaded archive matched the official Go release SHA-256.
Docker was unavailable and was not used.

Passed:

- `go test ./...` (existing specification tests).
- `go run ./cmd/bajarcheck network/spec.json`: VALID, supply 100000000000 BAJ,
  base units 100000000000000000000, no validator stake required.
- `python3 -m unittest discover -s scripts -p 'test_*.py' -v`: three tests.
- `bash -n` for all four new shell entry points.
- Built with `go install github.com/cometbft/cometbft/cmd/cometbft@v0.40.0`;
  verified embedded module v0.40.0 and Go module checksum
  `h1:0+zc7FlcnonFfMwzhyDCd5vK/fHZqz0RfRyTCxZsFPU=`.
- Start, status, smoke, stop, immediate restart, smoke, stop on four local nodes.
  Final restart advanced every node from height 15 to 18 and then 18 to 20.
  Every node reported three peers and four validators. All four agreed on
  committed block IDs, with at least three commit signatures at each checked height.
- Git ignore checks matched generated validator keys, databases and the built binary.

Fixed during verification: the upstream v0.40.0 module prints the fallback CLI
version `0.39.0`; verify module provenance instead. Also enabled SO_REUSEADDR
on the port preflight so immediate restart tolerates closed TCP connections
in TIME_WAIT.

The network was stopped after verification. Generated state and keys remain
under ignored `.devnet/`; Go and CometBFT are under ignored `.tools/`.
Use the PATH instructions in [DEVNET.md](DEVNET.md) to start it again.

Not run: full upstream CometBFT test suite, Byzantine fault injection, load
benchmarks, cryptographic audit, ML-DSA-65 consensus, BAJ transfer tests,
Docker deployment or public deployment. No BAJ ledger or wallet is implemented.
