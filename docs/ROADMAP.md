# BAJAR build milestones

## Implemented for local development

- Specification checks: revised 108B supply, 9 decimals, separate 8B reserve,
  no validator stake requirement, arbitrary-precision accounting.
- Legacy four-validator Ed25519 kvstore consensus demo (phase 1).
- BAJ ABCI++ ledger: fixed genesis supply, ML-DSA-65 signed transfers, chain-bound
  signatures, strict canonical transaction encoding, sequential nonces, deterministic
  fees, proposal simulation, state roots, persistent atomic state commits and replay.
- Four-node ledger devnet with separate ML-DSA-65 validator and wallet keys,
  cryptographic commit verification and CLI queries/transfers.
- Locked undecided allocations: former 20B staking reserve, separate 8B reserve,
  remaining tokenomics pool. Local-only test funding does not finalize beneficiaries.

## Next engineering work

- Public-testnet bootstrap now has independent node initialization, public-key
  genesis assembly, pinned hash verification, an explicit testnet profile,
  non-signing RPC-node mode, systemd template and isolated-network tests. Actual
  hosts, HTTPS frontend, monitoring and public availability remain undeployed.

- Design and implement governance-approved validator admission/removal with explicit
  authorization and tests. Current ledger validator membership is fixed in genesis.
- Decide allocation beneficiaries and reserve-release rules; no reserve release exists.
- Select finalized fees and transaction resource limits based on measured workloads.
- Add efficient authenticated state storage, proof queries, pruning, state sync,
  pagination, account indexing and a versioned upgrade/migration protocol.
- Expand fault injection: crashes around every write, partitions, equivocation,
  malformed proposals, fuzzing, long-running and high-load tests.
- Independently operated validator deployment, monitoring, backups and recovery.
- Review PQ transport, wallet key storage and recovery, address design and hybrid
  migration separately from local ML-DSA-65 signature integration.
- Independent security and cryptographic audits before security or production claims.

## Later product milestones

- Public testnet using nonvaluable tokens, faucet, explorer and wallet UI.
- Protocol documentation, economics and compliance review.
- Production readiness review and explicit authorization before any real-value launch.
