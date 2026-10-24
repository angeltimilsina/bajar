package main

import (
	"bajar-network-starter/internal/ledger"
	"bajar-network-starter/internal/ledgerv2"
	"math/big"
)

// Only new disposable local chains get this explicit test allocation.
func newStakingGenesis(chainID, alice string, unbonding int64) (ledgerv2.Genesis, error) {
	pending, _ := new(big.Int).SetString(ledgerv2.PendingSupply, 10)
	pending.Sub(pending, big.NewInt(1000_000_000))
	g := ledgerv2.Genesis{ProtocolVersion: 2, ChainID: chainID, Decimals: 6, MaxSupply: ledgerv2.MaxSupply, Fee: "1", UnbondingBlocks: unbonding, Accounts: map[string]ledger.Account{
		ledger.ReserveAddress: {Balance: ledgerv2.SeparateReserve}, ledger.FormerAddress: {Balance: ledgerv2.FormerReserve}, ledger.PendingAddress: {Balance: pending.String()}, ledger.FeeAddress: {Balance: "0"}, ledgerv2.StakeAddress: {Balance: "0"}, alice: {Balance: "1000000000"},
	}}
	return g, g.Validate()
}
