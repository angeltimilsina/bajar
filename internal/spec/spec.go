package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
)

// Specification describes design constraints only: this package is NOT a blockchain.
type Specification struct {
	ChainID                    string       `json:"chain_id"`
	Denom                      string       `json:"denom"`
	MaxSupplyBAJ               string       `json:"max_supply_baj"`
	Decimals                   int          `json:"decimals"`
	ConsensusEngine            string       `json:"consensus_engine"`
	ValidatorAdmission         string       `json:"validator_admission"`
	ValidatorTarget            int          `json:"validator_target"`
	ValidatorStakeRequired     bool         `json:"validator_stake_required"`
	PostQuantumConsensusTarget string       `json:"post_quantum_consensus_target"`
	TransactionSignatureTarget string       `json:"transaction_signature_target"`
	Allocations                []Allocation `json:"allocations"`
}

type Allocation struct {
	Name string `json:"name"`
	BAJ  string `json:"baj"`
}

func Load(path string) (Specification, error) {
	var s Specification
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return s, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return s, errors.New("unexpected second JSON document")
		}
		return s, err
	}
	return s, Validate(s)
}

func parseUnsignedInt(name, value string) (*big.Int, error) {
	if value == "" {
		return nil, fmt.Errorf("%s is empty", name)
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("%s must be an unsigned decimal integer", name)
		}
	}
	v, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("invalid %s", name)
	}
	return v, nil
}

func Validate(s Specification) error {
	if strings.TrimSpace(s.ChainID) == "" {
		return errors.New("chain ID is missing")
	}
	if s.Denom != "BAJ" {
		return errors.New("native denom must be BAJ")
	}
	if s.Decimals != 9 {
		return errors.New("this proposed design specifies 9 decimals")
	}
	if s.ConsensusEngine != "CometBFT" {
		return errors.New("unsupported consensus engine")
	}
	if s.ValidatorAdmission != "governance-approved" {
		return errors.New("validator admission must be governance-approved in phase 0")
	}
	if s.ValidatorStakeRequired {
		return errors.New("validator stake must not be required")
	}
	if s.ValidatorTarget < 1 {
		return errors.New("validator target must be positive")
	}
	if s.PostQuantumConsensusTarget != "mldsa65" {
		return errors.New("expected mldsa65 as post-quantum target")
	}
	if s.TransactionSignatureTarget != "mldsa65" {
		return errors.New("expected mldsa65 transaction signature target")
	}

	supply, err := parseUnsignedInt("max_supply_baj", s.MaxSupplyBAJ)
	if err != nil {
		return err
	}
	target := new(big.Int).SetUint64(108_000_000_000)
	if supply.Cmp(target) != 0 {
		return errors.New("maximum supply must equal 108,000,000,000 BAJ")
	}

	total := new(big.Int)
	names := map[string]bool{}
	if len(s.Allocations) == 0 {
		return errors.New("at least one provisional allocation is required")
	}
	for _, a := range s.Allocations {
		if strings.TrimSpace(a.Name) == "" {
			return errors.New("allocation has no name")
		}
		if names[a.Name] {
			return fmt.Errorf("duplicate allocation: %s", a.Name)
		}
		names[a.Name] = true
		v, err := parseUnsignedInt("allocation "+a.Name, a.BAJ)
		if err != nil {
			return err
		}
		total.Add(total, v)
	}
	if total.Cmp(supply) != 0 {
		return fmt.Errorf("allocation total %s BAJ != maximum supply %s BAJ", total, supply)
	}
	reserve := false
	for _, a := range s.Allocations {
		if a.Name == "separate_reserve_policy_pending" {
			v, _ := parseUnsignedInt("reserve", a.BAJ)
			reserve = v.Cmp(big.NewInt(8_000_000_000)) == 0
		}
	}
	if !reserve {
		return errors.New("separate reserve must equal 8,000,000,000 BAJ")
	}
	return nil
}

func BaseUnits(s Specification) (*big.Int, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}
	tokens, _ := parseUnsignedInt("supply", s.MaxSupplyBAJ)
	ten := big.NewInt(10)
	scale := new(big.Int).Exp(ten, big.NewInt(int64(s.Decimals)), nil)
	return new(big.Int).Mul(tokens, scale), nil
}
