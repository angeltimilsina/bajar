package ledgerv2

import (
	"bajar-network-starter/internal/ledger"
	"bytes"
	"encoding/json"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	"math/big"
	"math/rand"
	"testing"
)

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T) (State, []mldsa65.PrivKey, []string) {
	t.Helper()
	var keys []mldsa65.PrivKey
	var addresses []string
	a := map[string]Account{ledger.ReserveAddress: {Balance: SeparateReserve}, ledger.FormerAddress: {Balance: FormerReserve}, ledger.PendingAddress: {Balance: "0"}, ledger.FeeAddress: {Balance: "0"}, StakeAddress: {Balance: "0"}}
	for i := 0; i < 3; i++ {
		k, e := mldsa65.GenPrivKey()
		must(t, e)
		keys = append(keys, k)
		address := ledger.Address(k.PubKey().Bytes())
		addresses = append(addresses, address)
		a[address] = Account{Balance: "1000000000"}
	}
	a[addresses[0]] = Account{Balance: "79999998000000000"}
	s, e := NewState(Genesis{ProtocolVersion: 2, ChainID: "staking-test", Decimals: 6, MaxSupply: MaxSupply, Fee: "1", UnbondingBlocks: 3, Accounts: a})
	must(t, e)
	return s, keys, addresses
}
func tx(t *testing.T, s State, k mldsa65.PrivKey, kind, to, amount string) []byte {
	t.Helper()
	from := ledger.Address(k.PubKey().Bytes())
	b, e := Sign(k, UnsignedTx{Version: 2, ChainID: s.Genesis.ChainID, Kind: kind, To: to, Amount: amount, Fee: s.Genesis.Fee, Nonce: s.Accounts[from].Nonce + 1})
	must(t, e)
	return b
}
func apply(t *testing.T, s *State, k mldsa65.PrivKey, kind, to, amount string, height int64) {
	t.Helper()
	must(t, s.Apply(tx(t, *s, k, kind, to, amount), height))
	must(t, s.Validate())
}
func TestSupplySixDecimalsAndLegacyIsolation(t *testing.T) {
	s, _, _ := fixture(t)
	must(t, s.Validate())
	if MaxSupply != "108000000000000000" {
		t.Fatal("incorrect base units")
	}
	if ledger.MaxSupply != "108000000000000000000" {
		t.Fatal("legacy supply changed")
	}
	g := s.Genesis
	g.Decimals = 9
	if g.Validate() == nil {
		t.Fatal("v2 accepted legacy precision")
	}
}
func TestFeeOnlyRewardsEntryAndNoHistoricalWindfall(t *testing.T) {
	s, k, a := fixture(t)
	apply(t, &s, k[2], "transfer", a[1], "100", 1) // fees with no active stake stay unallocated
	apply(t, &s, k[0], "stake", "", "100", 1)
	if _, owed := s.PositionView(a[0]); owed != "0" {
		t.Fatal("new staker received historical fees")
	}
	apply(t, &s, k[1], "stake", "", "100", 1) // entry fee belongs to pre-existing staker
	_, alice := s.PositionView(a[0])
	_, bob := s.PositionView(a[1])
	if alice != "1" || bob != "0" {
		t.Fatal(alice, bob)
	}
	apply(t, &s, k[2], "transfer", a[1], "1", 2)
	_, alice = s.PositionView(a[0])
	_, bob = s.PositionView(a[1])
	if alice != "1" || bob != "0" {
		t.Fatal("fractional rewards rounded up")
	}
	apply(t, &s, k[2], "transfer", a[1], "1", 2)
	_, alice = s.PositionView(a[0])
	_, bob = s.PositionView(a[1])
	if alice != "2" || bob != "1" {
		t.Fatal(alice, bob)
	}
	before := num(s.Accounts[a[0]].Balance)
	apply(t, &s, k[0], "claim", "", "0", 2)
	if new(big.Int).Sub(num(s.Accounts[a[0]].Balance), before).String() != "1" {
		t.Fatal("claim did not charge fee and settle whole rewards")
	}
	must(t, s.Validate())
}
func TestUnbondingLockAndWithdrawal(t *testing.T) {
	s, k, a := fixture(t)
	before := num(s.Accounts[a[0]].Balance)
	apply(t, &s, k[0], "stake", "", "100", 1)
	apply(t, &s, k[0], "unstake", "", "100", 2)
	old := s.Hash()
	if s.Apply(tx(t, s, k[0], "withdraw", "", "0"), 4) == nil {
		t.Fatal("early withdrawal")
	}
	if !bytes.Equal(old, s.Hash()) {
		t.Fatal("failed withdrawal charged fees")
	}
	apply(t, &s, k[0], "withdraw", "", "0", 5)
	if s.Accounts[StakeAddress].Balance != "0" || s.TotalBonded != "0" {
		t.Fatal("collateral not returned")
	}
	if new(big.Int).Sub(before, num(s.Accounts[a[0]].Balance)).String() != "3" {
		t.Fatal("principal loss beyond three fixed fees")
	}
	if s.Positions[a[0]].Bonded != "0" || len(s.Positions[a[0]].Unbonding) != 0 {
		t.Fatal("unbonding not cleared")
	}
}
func TestFailuresAtomicAndReplay(t *testing.T) {
	s, k, a := fixture(t)
	good := tx(t, s, k[0], "stake", "", "100")
	must(t, s.Apply(good, 1))
	before := s.Hash()
	bad := [][]byte{good, tx(t, s, k[0], "stake", "", "-1"), tx(t, s, k[0], "unstake", "", "101"), tx(t, s, k[1], "claim", "", "0"), tx(t, s, k[1], "withdraw", "", "0"), tx(t, s, k[0], "transfer", ledger.ReserveAddress, "1")}
	var forged Transaction
	must(t, json.Unmarshal(tx(t, s, k[0], "transfer", a[1], "1"), &forged))
	forged.Amount = "2"
	b, _ := json.Marshal(forged)
	bad = append(bad, b)
	for _, b := range bad {
		if s.Apply(b, 2) == nil {
			t.Fatal("invalid action accepted")
		}
		if !bytes.Equal(before, s.Hash()) {
			t.Fatal("invalid action changed accounting")
		}
	}
}
func TestRandomizedConservationAndRewardBacking(t *testing.T) {
	s, k, a := fixture(t)
	r := rand.New(rand.NewSource(42))
	kinds := []string{"transfer", "stake", "unstake", "claim", "withdraw"}
	for height := int64(1); height <= 400; height++ {
		i := r.Intn(3)
		kind := kinds[r.Intn(len(kinds))]
		value := "1"
		to := ""
		if kind == "transfer" {
			to = a[(i+1)%3]
		}
		if kind == "claim" || kind == "withdraw" {
			value = "0"
		}
		before := s.Hash()
		err := s.Apply(tx(t, s, k[i], kind, to, value), height)
		if err != nil && !bytes.Equal(before, s.Hash()) {
			t.Fatal("failed action mutated state")
		}
		must(t, s.Validate())
	}
}
