# Go ledger publication checks — 2026-10-23

Destination selected by the user: `angeltimilsina/bajar`, on a dedicated review
branch. This publishes the root Go module while retaining the existing Solidity
prototype under `blockchain/`. It does not modify the default branch or deploy hosts.

Local verification completed before publication:

- `go test ./...`: passed, all 14 top-level Go tests and subtests.
- Retained local devnet start/smoke/stop: every validator advanced from height 35
  to 38, with matching block/application roots and verified ML-DSA-65 commits.
  Validator identities, signing state, wallet keys and databases were preserved.
- Installed `build-essential` in WSL as requested; `CGO_ENABLED=1 go test -race ./...`
  passed. This resolves the previously recorded race-test prerequisite limitation.
- Source publication uses an explicit allowlist, not `git add .` against the
  enclosing Windows-home repository. Candidate files were scanned for private-key
  blocks, common credential formats and embedded secret values, and manually reviewed.
  `.devnet/`, `.ledger-devnet/`, `.tools/`, generated keys, databases, logs, `.env`
  files and other private workstation files are excluded.
- The ignored `.tools/publication/manifest.json` records file paths and SHA-256
  values for the normalized UTF-8 source snapshot. GitHub publication builds on the
  existing main tree so unrelated prototype files are retained.

The new GitHub workflow adds reproducible dependency, race, vet, signature-library,
source-review and four-node integration checks. Remote workflow status must be
reported separately from the local results; adding a workflow does not prove it passed.

Public testnet remains undeployed. `bajard` still enforces local-only startup;
per-host testnet initialization, a non-validator RPC node and the server configuration
profile are future implementation work described in TESTNET-PREPARATION.md.
