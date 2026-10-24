package spec

import (
	"math/big"
	"testing"
)

func valid() Specification {
	return Specification{
		ChainID: "bajar-devnet-1", Denom: "BAJ", MaxSupplyBAJ: "108000000000",
		Decimals: 9, ConsensusEngine: "CometBFT", ValidatorAdmission: "governance-approved",
		ValidatorTarget: 4, ValidatorStakeRequired: false, PostQuantumConsensusTarget: "mldsa65",
		TransactionSignatureTarget: "mldsa65",
		Allocations:                []Allocation{{"former_staking_reserve_policy_pending", "20000000000"}, {"remaining_genesis_policy_pending", "80000000000"}, {"separate_reserve_policy_pending", "8000000000"}},
	}
}

func TestSupplyAndBaseUnits(t *testing.T) {
	s := valid()
	if err := Validate(s); err != nil {
		t.Fatal(err)
	}
	n, err := BaseUnits(s)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := new(big.Int).SetString("108000000000000000000", 10)
	if n.Cmp(expected) != 0 {
		t.Fatalf("got %s, want %s", n, expected)
	}
	if n.IsInt64() {
		t.Fatal("108B BAJ with 9 decimals should NOT fit in int64")
	}
}

func TestWrongAllocationSum(t *testing.T) {
	s := valid()
	s.Allocations[1].BAJ = "79999999999"
	if err := Validate(s); err == nil {
		t.Fatal("expected allocation total error")
	}
}

func TestDuplicateAllocations(t *testing.T) {
	s := valid()
	s.Allocations[1].Name = s.Allocations[0].Name
	if err := Validate(s); err == nil {
		t.Fatal("expected duplicate allocation error")
	}
}

func TestStakingNotRequired(t *testing.T) {
	s := valid()
	s.ValidatorStakeRequired = true
	if err := Validate(s); err == nil {
		t.Fatal("expected no-staking invariant error")
	}
}

func TestInvalidAmount(t *testing.T) {
	s := valid()
	s.Allocations[0].BAJ = "-1"
	if err := Validate(s); err == nil {
		t.Fatal("expected nonnegative amount error")
	}
}

func TestPreviousSupplyAndReserveMismatchRejected(t *testing.T) {
	s := valid()
	s.MaxSupplyBAJ = "100000000000"
	if Validate(s) == nil {
		t.Fatal("accepted superseded 100B supply")
	}
	s = valid()
	s.Allocations[2].BAJ = "7000000000"
	s.Allocations[1].BAJ = "81000000000"
	if Validate(s) == nil {
		t.Fatal("accepted incorrect separate reserve despite matching total")
	}
}

func TestSixDecimalStakingProtocol(t *testing.T) {
	s := valid()
	s.ProtocolVersion = 2
	s.Decimals = 6
	s.StakingMode = "optional"
	s.RewardFunding = "fees-only"
	n, err := BaseUnits(s)
	if err != nil {
		t.Fatal(err)
	}
	if n.String() != "108000000000000000" {
		t.Fatal(n)
	}
	s.ProtocolVersion = 1
	if Validate(s) == nil {
		t.Fatal("silently changed legacy precision")
	}
	s.ProtocolVersion = 2
	s.RewardFunding = "minting"
	if Validate(s) == nil {
		t.Fatal("inflationary rewards accepted")
	}
}
