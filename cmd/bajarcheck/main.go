package main

import (
	"bajar-network-starter/internal/spec"
	"fmt"
	"os"
)

func main() {
	path := "network/spec.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	s, err := spec.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "INVALID BAJAR specification:", err)
		os.Exit(1)
	}
	units, err := spec.BaseUnits(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("BAJAR specification: VALID")
	fmt.Println("Chain ID (planned):", s.ChainID)
	fmt.Println("Supply:", s.MaxSupplyBAJ, s.Denom)
	fmt.Println("Decimals:", s.Decimals)
	if s.ProtocolVersion == 2 {
		fmt.Println("Staking:", s.StakingMode, "Rewards:", s.RewardFunding)
	}
	fmt.Println("Base units (big integer):", units.String())
	fmt.Println("Consensus:", s.ConsensusEngine)
	fmt.Println("Validator signature target:", s.PostQuantumConsensusTarget)
	fmt.Println("Transaction signature target:", s.TransactionSignatureTarget)
	fmt.Println("Validator stake required:", s.ValidatorStakeRequired)
	fmt.Println("NOTE: Design validation only. Local ledger tests do not constitute a public network or token launch.")
}
