# bajar-testnet-1 bootstrap

This branch is based on the verified ledger foundation in PR #1. It adds testnet
bootstrap code and service templates. It does not provision servers, expose a
public RPC endpoint or finalize production allocations.

## Build provenance

Use a reviewed commit of this branch, Go 1.25.0 and the pinned go.mod/go.sum.
Build the embedded ledger, not stock CometBFT kvstore:

```bash
go mod download
go mod verify
go test -race ./...
go vet ./...
bash scripts/build-ledger.sh
go version -m .tools/bin/bajard
sha256sum .tools/bin/bajard
```

Record the commit, toolchain, module versions, architecture and binary checksum.
Use the same reviewed binary on matching-architecture hosts. Verify the artifact
checksum before installation. No release binary or server installer is published
by this change.

## Independent node initialization

On each selected validator host, generate its own identity in a **new** home:

```bash
bajard init-testnet --home /var/lib/bajar --role validator --name validator-1 \
  --advertise validator-1.example.org:26656
```

Use unique names and addresses for validators 1 through 4. Run initialization
under an account allowed to create that home; if run as root, make the complete
home owned by the dedicated `bajar` service user afterward. Init refuses an
existing home and does not create a wallet or transfer any BAJ.

On the separate RPC host:

```bash
bajard init-testnet --home /var/lib/bajar --role rpc --name public-rpc \
  --advertise rpc-p2p.example.org:26656
```

Only `identity.json` is exchanged with the ceremony coordinator. This contains
the node ID, advertised P2P address, role, name and (validators only) public
ML-DSA-65 key. Verify ownership and fingerprints out of band. Never copy node
private keys, validator private keys or signing state to the coordinator or Git.

The RPC role has a normal P2P transport key but **no consensus private key** or
validator signing-state file. Upstream v0.40.0 requires a PrivValidator interface
even for full nodes, so a public-only adapter supplies the P2P public identity and
refuses every vote/proposal signing request. The fixed genesis excludes that
identity. Importing consensus files into this home makes startup fail.

## Genesis ceremony

The coordinator puts the four approved validator identities in `validators.json`
(a JSON array). The RPC identity is not a genesis validator. Assembly sorts
validators and uses an explicit UTC timestamp, making the result reproducible:

```bash
bajard assemble-genesis --identities validators.json \
  --genesis-time 2026-10-23T00:00:00Z --out genesis.json
sha256sum genesis.json
```

Replace the example timestamp with the agreed ceremony time. The command prints
the SHA-256 digest of the exact output bytes. Publish the approved digest out of
band and distribute the identical genesis file to every host.

Supply is exactly 108B BAJ at 9 decimals. By default 20B BAJ is locked in the
former staking pool, 80B in the pending tokenomics pool, and 8B in the separate
reserve; the fee pool begins at zero. No beneficiary is chosen by default.
For an explicitly approved **nonvaluable test distribution**, an optional
`--test-allocations allocations.json` accepts `{ "baj1...": "1000000000" }`
using integer base-unit strings, deducted only from the pending 80B pool. There
is no faucet/mint or reserve-release method; both reserve pools remain locked.
The integration test supplies disposable fixture funding, not public beneficiaries.

On each node, verify and install:

```bash
bajard install-genesis --home /var/lib/bajar --genesis genesis.json \
  --sha256 AGREED_LOWERCASE_SHA256
```

Installation validates chain ID, four distinct equal-power ML-DSA-65 validators,
consensus limits, supply, reserves and local validator membership. It refuses a
changed existing genesis, a changed pin, or a new install over existing chain
data/signatures. Reinstalling the identical bytes is idempotent. Every startup
rechecks the pinned bytes before opening databases or listeners.

## Peers and explicit startup

Create a per-host `peers.json` containing approved public identities, excluding
self. In this five-host profile, validators list the other three validators and
the RPC node; the RPC node lists all four validators. Duplicate IDs/addresses,
self-peers, invalid descriptors and empty peer lists fail validation.

```bash
bajard configure-testnet --home /var/lib/bajar --peers peers.json
bajard check-testnet --home /var/lib/bajar
bajard start --home /var/lib/bajar --profile testnet
```

The testnet profile accepts only `bajar-testnet-1`, requires its ceremony metadata
and pin, and verifies the local key against genesis. PEX and duplicate-IP peers
are disabled; peers are explicitly configured. Inbound/outbound peers, RPC
connections, batch size and request body size are bounded. State sync, remote
signer listeners, gRPC, pprof, unsafe RPC, broad CORS and public metrics listeners
are rejected. Arbitrary key/database/genesis paths are rejected.

P2P binds `0.0.0.0:26656` by default and advertises the supplied address; use host
firewalls to permit the selected peer topology. RPC remains numeric-loopback
only, including on the RPC host. A separately reviewed HTTPS frontend, method
allowlist, abuse limits and firewall/DNS/TLS configuration are still required
before public RPC exposure. Do not forward validator RPC to the internet.

The local default still rejects this testnet. `init-devnet` and its `.ledger-devnet/`
are unchanged, use different keys/genesis, and must remain private.

## Supervision and operations

`deploy/systemd/bajard.service` runs either role using its local profile. It uses
a dedicated `bajar` user, restrictive umask, `/var/lib/bajar` persistent state,
read-only system paths, private temporary storage, a preflight policy check,
bounded restart retries and a 90-second graceful shutdown timeout. After installing
the checksum-verified binary at `/usr/local/bin/bajard`, arranging the service
user/home ownership and completing the ceremony, install the unit:

```bash
sudo install -m 0644 deploy/systemd/bajard.service /etc/systemd/system/bajard.service
sudo systemctl daemon-reload
sudo systemctl enable --now bajard
sudo journalctl -u bajard -f
```

These are operator instructions; this preparation task does not execute them on
servers. Monitor private `/status` for height progress, catch-up, peer count and
disk capacity; configure external alerting before deployment. A startup failure
should be investigated, not bypassed by removing profile checks.

For backup, stop the service cleanly and preserve the **complete** home, including
validator identity, signing state and every database, with restrictive access.
Never run the same validator identity on two hosts. Never restore old signing
state while reusing a key; mismatched or stale backups need a reviewed recovery
procedure. This branch tests restart recovery, not arbitrary backup rollback.

Client commands default to local mode. Testnet queries/transfers require explicit
`--profile testnet`; remote endpoints require HTTPS (loopback HTTP is allowed
for SSH tunnels and tests). For commit verification, additionally pin the trusted
genesis rather than trusting validator keys returned by the RPC server:

```bash
bajard verify-commit --profile testnet --rpc https://rpc.example.org \
  --genesis genesis.json --sha256 AGREED_LOWERCASE_SHA256
```

The genesis validator set is fixed; dynamic permissioned governance and trusted
upgrades are separate milestones. Post-quantum transport and independent security
review remain outstanding. ML-DSA-65 integration alone is not an overall security claim.

## Reproducible integration tests

```bash
go test -race ./...
python3 scripts/ledger-devnet.py e2e
sudo python3 scripts/testnet-integration.py
sudo python3 scripts/check-service.py
python3 scripts/review-publication.py
```

The namespace test requires Linux, root and iproute2. It creates five isolated
network namespaces on a temporary private bridge, with distinct IPs, separate
homes and independently invoked initialization. There is no default route or
public exposure. It checks public-only genesis exchange, hash pins, four different
validator secrets, an RPC home with no consensus secret, default-profile rejection,
matching block/state roots, trusted-genesis ML-DSA-65 commit verification, signed
transfers through the RPC node, replay rejection, fees/reserves, full restart,
one-validator outage, two-validator quorum loss and recovery. Namespaces and
processes are cleaned up; ignored test homes/logs remain for inspection.

This is a five-host **network simulation**, not a test on independent VPS servers.
WAN latency, DNS/TLS, service operation, firewall rules, load, partitions, upgrades
and external-client availability must be tested on the selected real hosts later.
