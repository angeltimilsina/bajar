package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bajar-network-starter/internal/ledger"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	"github.com/cometbft/cometbft/types"
)

func TestDevnetGenesisKeySeparationAndNoOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "chain")
	if err := initDevnet(root); err != nil {
		t.Fatal(err)
	}
	var addresses map[string]string
	b, err := os.ReadFile(filepath.Join(root, "wallets/addresses.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &addresses); err != nil {
		t.Fatal(err)
	}
	var genesisBytes []byte
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		path := filepath.Join(root, "nodes", []string{"node0", "node1", "node2", "node3"}[i], "config/genesis.json")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			genesisBytes = b
		} else if string(b) != string(genesisBytes) {
			t.Fatal("genesis differs")
		}
	}
	g, err := types.GenesisDocFromJSON(genesisBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Validators) != 4 || len(g.ConsensusParams.Validator.PubKeyTypes) != 1 || g.ConsensusParams.Validator.PubKeyTypes[0] != mldsa65.KeyType {
		t.Fatal("incorrect consensus configuration")
	}
	for _, v := range g.Validators {
		if v.Power != 1 || v.PubKey.Type() != mldsa65.KeyType {
			t.Fatal("incorrect validator key or power")
		}
		address := ledger.Address(v.PubKey.Bytes())
		if seen[address] || address == addresses["alice"] || address == addresses["bob"] {
			t.Fatal("keys reused")
		}
		seen[address] = true
	}
	var state ledger.Genesis
	if err := json.Unmarshal(g.AppState, &state); err != nil {
		t.Fatal(err)
	}
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	if state.Accounts[addresses["alice"]].Balance != "1000000000000" {
		t.Fatal("incorrect test funding")
	}
	alice, err := readKey(filepath.Join(root, "wallets/alice.key"))
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Address(alice.PubKey().Bytes()) != addresses["alice"] {
		t.Fatal("wallet address mismatch")
	}
	if err := initDevnet(root); err == nil {
		t.Fatal("overwrote existing devnet")
	}
	if _, err := wallet(filepath.Join(root, "wallets/alice.key")); err == nil {
		t.Fatal("overwrote wallet")
	}
	same, err := readKey(filepath.Join(root, "wallets/alice.key"))
	if err != nil || !same.Equals(alice) {
		t.Fatal("existing key changed")
	}
}
