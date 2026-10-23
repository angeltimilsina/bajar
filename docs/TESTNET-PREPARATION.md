# bajar-testnet-1 preparation

This branch publishes the tested Go ledger for review before server deployment.
It does **not** create a public network. The existing local `.ledger-devnet/`
remains a separate environment and its validator state must never be exposed.
The repository also retains an earlier Solidity prototype under `blockchain/`;
that prototype is not part of the native Go BAJ ledger or its test results.

## Source and build

The Go module at the repository root is the implementation to build. CometBFT
v0.40.0 is embedded in `bajard`; servers need the same built ledger binary, not
an unrelated stock CometBFT executable running kvstore. Dependencies are pinned
through go.mod/go.sum. Use a reviewed commit SHA for server builds, record Go and
module versions and the resulting binary SHA-256, and deploy identical binaries.

The `BAJ ledger` GitHub workflow runs race tests, vet, dependency verification,
the design checker, signature-library tests, Python tooling tests and a fresh
four-validator local integration test. Workflow permissions are read-only and
it does not receive deployment credentials or upload generated keys as artifacts.

## Implementation work before provisioning

The present `bajard start` intentionally accepts only local chain IDs and loopback
listeners. `init-devnet` generates all local validator/wallet keys together and
only configures four nodes. It is not a public-testnet ceremony or server installer.
Do not work around these checks by editing local genesis or exposing its RPC.

The next reviewed change needs to implement:

1. Separate per-host ML-DSA-65 key generation. Exchange public identities only;
   each server retains its private key and signing state. Add non-validator full
   node startup, so the public RPC node has no consensus signing key.
2. An explicit testnet configuration profile for `bajar-testnet-1`, with bounded
   public P2P settings, private validator RPC and configurable peers/addresses.
3. A fresh genesis assembled from approved public validator keys, exactly 108B
   BAJ at 9 decimals, the locked former 20B and separate 8B pools, and explicitly
   approved nonvaluable test-distribution rules. Publish and verify its hash on
   every host; never repurpose local keys, test wallets, or databases.
4. Idempotent host setup with a dedicated service user, persistent storage,
   systemd supervision, restrictive key permissions, monitoring and stopped-node
   backup/recovery procedures that preserve validator signing state.
5. A separate public full/RPC node behind HTTPS and rate/request-size/concurrency
   limits. Validator RPC remains bound privately. Expose configured P2P (normally
   TCP 26656) only according to the chosen direct-peer or sentry topology.
6. Tests of the public profile on isolated hosts, followed by external account,
   signed-transfer, replay-rejection, supply and cryptographic-finality checks;
   include restart, outage, quorum-loss and recovery exercises.

## Host information needed for deployment

Record four validator hosts plus at least one separate public RPC host, provider
and region, OS/architecture, SSH access method, peer topology, allowed management
networks, persistent disk layout, RPC DNS name and TLS ownership. Provisioning
creates cloud costs; use the user's selected provider and budget. No cloud hosts
have been selected or provisioned by this publication step.

Do not expose privileged RPC methods, validator signing endpoints or metrics
directly to the public internet. A generic reverse proxy alone is not a completed
RPC policy. Design the allowed methods and limits, validate against the pinned
CometBFT version and test externally.

Official references:

- [CometBFT v0.40 validator operations and sentries](https://github.com/cometbft/cometbft/blob/v0.40.0/docs/core/validators.md)
- [Pinned node configuration](https://github.com/cometbft/cometbft/blob/v0.40.0/config/config.go)

Public testnet tokens are nonvaluable test assets. Mainnet governance, independent
security/cryptographic review and any liquidity integration are later milestones.
