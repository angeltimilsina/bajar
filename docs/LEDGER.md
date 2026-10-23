# Local BAJ ledger

## Scope

`cmd/bajard` embeds CometBFT v0.40.0 and `internal/ledger` in each node. No
consensus or signature algorithm is implemented here. ML-DSA-65 uses CometBFT's
`crypto/mldsa65` wrapper around Cloudflare CIRCL. Startup accepts only local-chain
IDs and loopback RPC/P2P listeners. This is development software without public
deployment or real-value issuance.

Genesis must total exactly 108,000,000,000,000,000,000 base units at 9 decimals.
The separate reserve is exactly 8B BAJ. The former staking pool is exactly 20B
BAJ inside the 100B tokenomics allocation. Both remain locked, as does the pending
allocation pool. A locked fee pool accumulates fees without changing total supply.
No mint, burn, reserve-release or validator-update transaction is supported.

The local genesis gives Alice 1,000 test BAJ, Bob zero, locks 20B BAJ in the former
reserve and 8B in the separate reserve, and locks the remainder of the 80B pending
allocation. These are disposable fixtures, not production beneficiaries. Production
genesis and allocation decisions remain pending.

## Commands and ports

Build: `bash scripts/build-ledger.sh` (Go 1.25+).
Control: `python3 scripts/ledger-devnet.py start|status|smoke|stop|e2e` (Python 3.9+).
Start initializes only when `.ledger-devnet/` does not exist and refuses an existing
process record. Stop preserves databases, keys and signing state. E2e stops nodes
in its final cleanup. The old `.devnet/` Ed25519 example remains separate.

| Node | P2P | RPC |
| --- | --- | --- |
| 0 | 127.0.0.1:27656 | 127.0.0.1:27657 |
| 1 | 127.0.0.1:27666 | 127.0.0.1:27667 |
| 2 | 127.0.0.1:27676 | 127.0.0.1:27677 |
| 3 | 127.0.0.1:27686 | 127.0.0.1:27687 |

Homes, logs and wallets live under ignored `.ledger-devnet/`.
`bajard init-devnet --dir PATH` refuses any existing target and stages new keys
in an ignored `.bajar-init-*` sibling directory, renamed on success. A failed
initialization is retained for inspection. Linux restrictive modes are requested;
Windows-mounted folders may inherit Windows ACLs. Protect the workspace or use a
native Linux checkout for stronger isolation. Never delete validator signing state
while reusing identities, or convert the legacy demo's existing keys.

```bash
.tools/bin/bajard wallet-new --key .ledger-devnet/wallets/new.key
.tools/bin/bajard address --key .ledger-devnet/wallets/new.key
.tools/bin/bajard query --path /supply
.tools/bin/bajard query --path /account --address ADDRESS
.tools/bin/bajard query --path /state
.tools/bin/bajard transfer --key WALLET.key --to ADDRESS --amount BASE_UNITS
.tools/bin/bajard verify-commit
```

`1000000000` base units = 1 BAJ. The fixed local fee is 1,000 base units
(0.000001 BAJ); production fee policy remains undecided. The CLI fetches the chain
ID, fee and current nonce from local RPC, signs and broadcasts, and reports success
only after successful CheckTx and execution. Use one outstanding transfer per
sender; mempool nonce reservation is not implemented. A broadcast timeout can have
an uncertain outcome: query state before retrying.

Wallet files are unencrypted development secrets, not a production keystore.
Wallet and validator files use different formats and independently generated keys.

## Transaction protocol

Canonical compact JSON uses this exact field order: `version`, `chain_id`,
`public_key`, `to`, `amount`, `fee`, `nonce`, `signature`. Version is 1. Public keys
and signatures are standard base64. Nonce is a positive uint64 equal to sender
nonce + 1; monetary values are canonical decimal strings bounded by supply.
Amounts are positive with no leading zeros. Maximum encoded size is 16 KiB.
Unknown fields, duplicate fields, alternate formatting and trailing JSON fail
parsing or canonical-byte comparison.

ML-DSA-65 signs `BAJAR/transfer/v1\n` followed by canonical unsigned JSON (the first
seven fields). Chain ID and nonce provide cross-chain and same-chain replay
protection. Wallet addresses are `baj1` plus the full lowercase SHA-256 digest of
`BAJAR/wallet/ml-dsa-65/v1\x00 || public_key` (68 characters). This is a development
format without a user-entry checksum. Pool names are not wallet addresses, so no
signed transfer can spend or directly fund them.

## ABCI++ and storage

- InitChain validates matching chain ID, exact supply, locked reserves, fee,
  and equal-power ML-DSA-65 validator keys and consensus parameters.
- CheckTx validates against committed state without mutation.
- PrepareProposal simulates candidates sequentially, drops invalid, duplicate or
  overspending transfers, and respects MaxTxBytes.
- ProcessProposal independently simulates the ordered proposal and rejects invalid
  execution.
- FinalizeBlock applies ordered transfers, fees and nonces to staged state. Invalid
  transactions return nonzero codes without effects. Height must be next.
- The state root is SHA-256 of canonical Go JSON with genesis, initial/current
  height and sorted account maps. No floats, wall clock or randomness enter execution.
- Commit validates conservation and saves the complete state in one LevelDB
  SetSync operation, then exposes it as committed. Info returns persisted height/hash.
  A crash before Commit discards staged changes; replay regenerates the same root.
  A crash after Commit reloads committed state. InitChain is idempotent only for
  the same height-zero genesis, supporting restart before the first block.
- Query returns current committed accounts, supply or full state. Proof requests
  and historical queries fail explicitly. State sync is unsupported.

Validator membership is fixed in genesis; no transaction can change it. This is
not a dynamic governance workflow. All four validators run on one machine and do
not establish decentralization. Peer transport uses CometBFT defaults; ML-DSA-65
votes do not establish post-quantum transport or overall system security.

Whole-state JSON snapshots are deliberately simple for review and unsuitable for
a large production ledger. Scalable authenticated storage, query proofs, resource
accounting, upgrades and benchmarks remain future work.

## Official references

- [CometBFT v0.40.0 ML-DSA-65](https://github.com/cometbft/cometbft/tree/v0.40.0/crypto/mldsa65)
- [ABCI++ interface](https://github.com/cometbft/cometbft/blob/v0.40.0/abci/types/application.go)
- [Validator signing state](https://github.com/cometbft/cometbft/blob/v0.40.0/privval/file.go)
- [NIST FIPS 204](https://csrc.nist.gov/pubs/fips/204/final)

Local tests do not constitute independent cryptographic review, NIST module
validation or production-security certification.
