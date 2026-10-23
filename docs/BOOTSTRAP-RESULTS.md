# Public-testnet bootstrap verification — 2026-10-23

Base: PR #1 commit `1befbca0e0c53d5aeb33a6fc4acfc17dc15a1d6c`.
Branch: `codex/public-testnet-bootstrap`. The foundation PR and main are not merged
or modified by this bootstrap change.

Implemented: independent per-host ML-DSA-65 initialization, public-only identity
exports, deterministic four-validator genesis assembly, explicit optional test
funding, exact 108B supply/9 decimals and locked reserve enforcement, immutable
genesis SHA-256 pins, approved peer configuration, explicit testnet startup policy,
non-signing RPC role, trusted-genesis commit verification, a systemd template and
five-network integration harness. Local startup remains the default profile.

Verified in WSL2 Ubuntu on Linux x86_64 with Go 1.25.0, GCC and iproute2:

- `go test -race ./...`: 20 top-level Go tests plus subtests, including unchanged
  ledger tests and bootstrap policy, genesis pin, omitted-time rejection, independent
  key separation, overwrite refusal, RPC no-signing behavior and client URL policy.
- `go vet ./...`, the supply checker, three existing Python tooling tests and
  Python byte-compilation checks.
- The legacy local-profile ledger integration passed: signed transfers, forged
  signatures/replay rejection, fees, exact supply, restart and one-node outage.
- `sudo python3 scripts/testnet-integration.py`: four validators and a fifth RPC
  node in five isolated Linux network namespaces, on distinct IPs/private P2P,
  with separate homes and independent initialization commands. The ceremony used
  public identities only. Every node installed the same hash-pinned genesis.
  Nodes matched committed blocks/application roots and verified ML-DSA-65 commits
  against the trusted genesis. A transfer through the RPC node replicated with
  exact balances/fees; replay failed before and after all-node restart. One
  validator outage retained quorum; two outages halted finalization; restoring one
  validator restored progress. Validator key hashes were unchanged and the RPC
  home had no consensus private key or signing-state file.
- The namespace harness stopped its processes and removed its namespaces/bridge;
  ignored test homes/keys/logs remain for diagnostics. The original workspace's
  local devnet was not used for public-profile testing or exposed.
- `sudo python3 scripts/check-service.py`: systemd-analyze verified a temporary
  copy using the existing built executable path. No service was installed or
  started. Direct verification of the deployment template initially reported its
  intentionally uninstalled `/usr/local/bin/bajard` path and Windows mount modes;
  the verification helper uses a Linux temporary file with correct modes.
- Source-only publication review and `git diff --check`; generated identity secrets,
  chain data and logs are ignored and excluded from the publication manifest.

Initial integration found an identity-export integer encoding mismatch, corrected
to standard JSON before the passing run. Final review also disabled experimental
transport options and rejected omitted genesis times before upstream defaulting.

Not run: independent VPS/WAN tests; public DNS/HTTPS/firewall frontend tests;
actual systemd service operation; backup rollback, load/partition/equivocation
campaigns; security/cryptographic audits; public deployment, cloud provisioning,
real-value transfers or mainnet. Raw RPC remains loopback-only and requires a
separately reviewed HTTPS frontend before public exposure. The network simulation
does not establish public availability or overall post-quantum security.
