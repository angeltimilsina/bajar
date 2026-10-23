# BAJAR Network — development ledger

BAJAR includes a deterministic BAJ bank application connected to CometBFT
v0.40.0 through ABCI++, a four-validator local devnet, wallet/transfer CLI,
and specification checks. It is development software, not an audited public
network, production wallet, or real-value token launch.

The root Go module is the native BAJ ledger. The `blockchain/` directory in the
GitHub repository contains an earlier Solidity prototype and is outside these
Go build/test results. See [public testnet preparation](docs/TESTNET-PREPARATION.md)
for the work required before `bajar-testnet-1` can be deployed.

The bootstrap branch adds independent validator/RPC-node initialization, public-key
genesis assembly, immutable genesis hash pins and a `bajar-testnet-1` startup
profile. [Operator commands and isolated-network tests](docs/TESTNET-BOOTSTRAP.md)
describe this preparation. `start` still defaults to the loopback-only local
profile; testnet startup requires `--profile testnet`. Nothing is publicly deployed.

## Current design

| Parameter | Value |
| --- | --- |
| Maximum supply | 108,000,000,000 BAJ |
| Tokenomics allocation | 100,000,000,000 BAJ |
| Separate reserve | 8,000,000,000 BAJ |
| Decimals | 9 |
| Smallest unit | 0.000000001 BAJ |
| Total base units | 108,000,000,000,000,000,000 |
| Transaction signatures | ML-DSA-65 using CometBFT's CIRCL-backed library |
| Local validator signatures | ML-DSA-65 |
| Consensus | CometBFT v0.40.0; four equal-power local validators |
| Validator stake/delegation | Not required |

The 108B cap supersedes the former 100B proposal. All amounts use `math/big`,
encoded as canonical decimal strings in base units. No mint transaction exists.
The 20B former staking reserve remains inside the 100B tokenomics allocation;
it is not the new separate 8B reserve. Neither reserve has assigned beneficiaries.

## Build and run (Linux or WSL2)

Use Go 1.25+ and Python 3.9+. From the repository root:

```bash
go test ./...
go run ./cmd/bajarcheck network/spec.json
bash scripts/build-ledger.sh
python3 scripts/ledger-devnet.py e2e
```

The end-to-end command initializes a fresh local chain if needed, exercises it,
and stops it. To run interactively:

```bash
python3 scripts/ledger-devnet.py start
.tools/bin/bajard query --path /supply
.tools/bin/bajard query --path /account --address "$(.tools/bin/bajard address --key .ledger-devnet/wallets/alice.key)"
.tools/bin/bajard transfer --key .ledger-devnet/wallets/alice.key \
  --to "$(.tools/bin/bajard address --key .ledger-devnet/wallets/bob.key)" \
  --amount 1000000000
.tools/bin/bajard verify-commit
python3 scripts/ledger-devnet.py stop
```

`--amount` is integer base units: `1000000000` transfers 1 BAJ. Local transfers
charge 1,000 base units (0.000001 BAJ), held in a locked fee pool. This is a
test policy, not finalized production economics.

Each initialization generates random, separate ML-DSA-65 validator and wallet
keys. Alice receives **1,000 disposable test BAJ** from the pending 80B allocation;
the rest stays locked. These test identities do not choose production beneficiaries.
Generated files are ignored under `.ledger-devnet/`; never commit or share keys.
Restart preserves wallets, databases and validator signing state.

See [ledger architecture, protocol, ports and limitations](docs/LEDGER.md),
[verification results](docs/LEDGER-RESULTS.md), and [roadmap](docs/ROADMAP.md).

## Legacy consensus example

`scripts/start-devnet.sh` and [DEVNET.md](docs/DEVNET.md) describe the older
Ed25519 persistent-kvstore example on ports 266xx. It does not run the BAJ ledger.
Its `.devnet/` data is retained independently. The ledger uses ports 276xx and
`.ledger-devnet/`; no existing validator identity is converted or reset.

ML-DSA-65 is standardized in [NIST FIPS 204](https://csrc.nist.gov/pubs/fips/204/final).
Using it locally does not establish audited end-to-end post-quantum security.
Transport, operational key protection, production governance, state sync,
load testing and external cryptographic review remain outstanding.
