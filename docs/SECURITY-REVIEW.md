# Development security review

This is an engineering self-review, not an independent audit or a mainnet
certification. Oracle provisioning and public deployment are deferred.

## Changes

Dependency scanning exposed vulnerabilities despite passing functional tests.
The toolchain is now pinned to Go 1.27.3. Network/cryptography dependencies,
gRPC and libp2p/WebRTC transitives were updated; CometBFT remains pinned to
v0.40.0. No consensus or signature algorithm was implemented locally.

Genesis startup now compares the supplied genesis document with the pinned file,
not only its chain ID. Independent node identities cannot be mixed across v1/v2
networks. Strict JSON decoding rejects duplicate members, including escaped key
aliases, and excessive nesting. Transaction JSON remains canonical and signed.

Protocol v2 introduces isolated state, signatures and chain identifiers. Reward
and principal backing are checked with integer arithmetic, failed actions are
atomic, and restart tests cover committed state and unbonding serialization.
An empty-list/null discrepancy found during restart testing was corrected before
accepting the end-to-end result.

## Verification evidence

Local verification includes Go compilation, unit/race tests, static analysis,
module checksums, both supply specifications, four-validator legacy recovery,
v2 staking/reward/unbonding/withdrawal recovery, and a five-network-namespace
test of four validators plus a non-validator RPC node. Namespace tests cover
signed transfers, replay rejection, quorum loss and recovery. They simulate
separate hosts on one Linux machine; they are not an internet deployment.

The updated `govulncheck` v1.9.0 text-mode scan reported no reachable
vulnerabilities and no additional imported-package findings, while reporting
two module-only advisories outside the analyzed call graph. These are retained
as findings; no exclusions or suppressions are used. CI repeats scanning against
the then-current vulnerability database. A clean call graph is not proof that
all dependency or runtime risks are absent.

The residual advisories are [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)
(unused OpenPGP packages in `x/crypto`) and
[GO-2025-3442](https://pkg.go.dev/vuln/GO-2025-3442) (CometBFT blocksync).
The latter lists a fix in v1.0.1 for the internal package. The current scanner
does not match an affected imported package on this pinned branch; that is not
a manual proof of safety. Review upstream patch applicability and compatibility
before public deployment rather than silently suppressing the advisory or
changing consensus-engine major versions within the staking change.

Linux ARM64 cross-compilation is a build check only. Execution on actual Oracle
ARM hardware, internet latency/partition behavior and production performance
have not been verified. The Solidity prototype is outside the Go test scope.

## Release gates still open

- Independent protocol, cryptographic and economic audits and remediation.
- Production capacity tests, durable backup/restore and operator key custody drills.
- Approved governance and validator-change implementation; reserve policy remains locked.
- Public testnet hosting, TLS/rate limiting, monitoring and external client validation.
- Explicit mainnet authorization after those checks; no real funds are moved here.

Reproduce with the workflow in `.github/workflows/ledger.yml`. Source publication
uses `scripts/review-publication.py` to restrict files and scan common secret
patterns. Generated keys, validator state, databases and local audit artifacts
remain ignored. Pattern scanning does not replace manual secret review.
