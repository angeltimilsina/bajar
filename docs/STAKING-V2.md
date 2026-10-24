# Six-decimal optional staking (protocol v2)

This is development software. Oracle hosting is deferred. No public network or
mainnet has been launched. These rules implement the user's approved choices:
six decimals, optional staking independent of consensus, and fee-funded rewards.

## Supply and compatibility

The cap remains 108,000,000,000 BAJ: 100B tokenomics plus a separate 8B reserve.
With six decimals the cap is `108000000000000000` base units. Amounts use
arbitrary-precision integers and canonical decimal strings. There is no minting.
The former 20B reserve and separate 8B reserve remain locked; no release policy
or beneficiaries have been chosen. Remaining allocations stay in the pending pool.

Legacy v1 retains nine decimals and its existing state/hash representation. The
v2 transaction signature domain, genesis version, chain ID, database name and
state key are separate. No initializer converts or resets an existing home.
`bajar-testnet-1` uses v1; a fresh `bajar-testnet-2` genesis uses v2.

There is no automatic migration. Dividing each v1 balance by 1,000 is lossless
only when every affected balance is divisible by 1,000. Dust handling, snapshots,
claims and an audited activation process require a separate approved plan.
Never simply change decimals on a retained chain.

## Actions and accounting

All actions require a wallet ML-DSA-65 signature, matching chain/version, next
nonce and the exact configured fee. The sender pays fees from liquid balance,
including for claims and withdrawals. Invalid actions leave all state unchanged.

| Action | Effect |
| --- | --- |
| transfer | Transfer a positive number of base units to a wallet |
| stake | Move positive liquid principal into the staking collateral pool |
| unstake | Stop earning on that principal immediately and create an unlock entry |
| withdraw | Return all matured unlock entries to liquid balance |
| claim | Transfer whole earned reward units from the fee pool to liquid balance |

Each successful transaction fee is allocated to stake active immediately before
that action. New stake earns neither its own joining fee nor historical fees.
An exiting stake earns its unstake transaction's fee before it leaves. With no
active stake the fee stays unallocated in the fee pool; later stakers cannot claim it.

The reward index uses a fixed integer scale of `10^24`. Each fee increments the
index by `floor(fee * scale / active principal)`. Positions checkpoint the index
and retain fractional entitlements. Claims pay only whole base units; fractional
amounts stay attached to the position. Division dust remains backed in the fee
pool and is not reassigned. No fixed APR, inflation, commissions, delegation or
slashing is implemented. Staking never changes validator voting power.

The fresh development genesis uses a one-base-unit fee and a 100-block unbonding
delay. These are test settings requiring production economic review. The release
height is execution height plus the configured delay. Unbonding principal earns
no subsequent fees. Reserve pools cannot sign transactions.

Every successful transition checks exact total supply, total bonded principal,
collateral backing of bonded/unbonding principal, and scaled fee backing of all
reward liabilities. Persisted state is validated again when reopened.

## Reproduce

```bash
go test -race ./...
go run ./cmd/bajarcheck network/spec-v2.json
bash scripts/build-ledger.sh
python3 scripts/staking-devnet.py
sudo python3 scripts/testnet-integration.py --protocol-version 2
```

The staking harness creates a fresh ignored `.staking-devnet-<timestamp>` home,
tests four ML-DSA-65 validators, rewards, unbonding, withdrawal and restart, then
stops its processes. It preserves the legacy `.ledger-devnet` home. See CLI help
for `stake`, `unstake`, `claim`, `withdraw`; amounts are integer base units. Testnet
initialization accepts `--chain-id bajar-testnet-2`; every identity and installed
genesis must match that chain. Existing bootstrap documentation otherwise applies.

## Limits and remaining work

The app serializes the full state at commit and validates all reward liabilities
on each action. Caps are 10,000 accounts, 1,000 staking positions and 16 pending
unbondings per position; they bound development fixtures, not production load.
There is no historical query, proof API, state sync, dynamic validator governance
transaction, hardware-backed signer integration or independent cryptographic audit.
Permissioned genesis membership is fixed. Public release requires operator drills,
capacity tests, external economic/security review and an explicit release decision.
