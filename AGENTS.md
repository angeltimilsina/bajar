# BAJAR Network coding instructions (for VS Code Codex)

This repository contains design checks and a development-only BAJ ABCI++ ledger with a local CometBFT network. It is not a production BAJAR blockchain.

## Product decisions
- BAJ native coin maximum supply: exactly 108,000,000,000 BAJ (user-approved revision, 2026-10-23).
- Tokenomics: 100,000,000,000 BAJ; separate locked reserve: 8,000,000,000 BAJ.
- Current target (user-approved 2026-10-24): protocol v2 uses 6 decimals and 108,000,000,000,000,000 base units. Keep arbitrary-precision accounting. Existing protocol v1 uses 9 decimals and remains unchanged; never reinterpret or reset its data.
- Optional staking and transaction-fee-only rewards are approved for v2. Staking must not affect validator admission or voting power. No inflation, reserve release or reassignment is approved.
- Initial validators: governance-approved cloud endpoints; no BAJ stake/delegation required for consensus.
- Use Byzantine fault-tolerant consensus; target CometBFT v0.40+.
- Target ML-DSA-65 for transaction and validator signatures; distinguish local implementation/testing from audited production security.
- Validator node signing keys are distinct from receiving wallet addresses.
- Former 20% staking reserve (20 billion BAJ) has not been reassigned; do not choose its beneficiaries without asking.
- Phase-0 checks, the legacy phase-1 kvstore demo, and the local-development ledger exist. No public network or real-value launch is authorized.
- Public-testnet bootstrap preparation was approved on 2026-10-23. Use the explicit testnet profile and pinned genesis; keep local defaults. Do not provision hosts or deploy publicly in this preparation task.
- Oracle hosting is explicitly deferred until a later development stage (2026-10-24). No cloud provisioning or pipeline diagnosis is currently required.

## Software engineering
- Use audited libraries: NEVER implement signature algorithms or consensus by hand.
- Do not claim PQ security until it is enabled, end-to-end tested, and cryptographically validated.
- No new token minting beyond genesis; account for supply as arbitrary-precision integers.
- Real app must implement deterministic ABCI++ state transitions, replay protection, fee accounting, state roots, persistent DB, genesis supply enforcement, and permissioned validator governance.
- Always test and disclose what was run versus not run.
- Never commit signing keys, .env, or production secrets.
- Never start a mainnet or move real funds without explicit authorization and audits.
- Prefer smallest reviewable working change with a test and a reproducible command.

## First IDE task
1. Explain what this starter does and does not do.
2. Run `go test ./...` and `go run ./cmd/bajarcheck network/spec.json`.
3. Inspect official CometBFT v0.40 + ML-DSA-65 docs.
4. Draft a minimal ABCI++ implementation plan (do not write your own BFT engine).
5. The user approved the local bank integration on 2026-10-23. Verify with `python3 scripts/ledger-devnet.py e2e`; do not deploy publicly.
