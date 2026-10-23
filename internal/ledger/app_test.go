package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	encoding "github.com/cometbft/cometbft/crypto/encoding"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	proto "github.com/cometbft/cometbft/proto/tendermint/types"
)

var ctx = context.Background()

type failingDB struct {
	dbm.DB
	fail bool
}

func (db *failingDB) SetSync(k, v []byte) error {
	if db.fail {
		return fmt.Errorf("injected storage failure")
	}
	return db.DB.SetSync(k, v)
}

func TestCommitFailureDoesNotPublishState(t *testing.T) {
	db := &failingDB{DB: dbm.NewMemDB()}
	a, alice, bob, _ := fixture(t, db)
	before := a.committed.Hash()
	_, err := a.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 1, Txs: [][]byte{signed(t, alice, bob, "1", 1)}})
	must(t, err)
	db.fail = true
	if _, err := a.Commit(ctx, &abci.RequestCommit{}); err == nil {
		t.Fatal("commit ignored storage error")
	}
	if !bytes.Equal(before, a.committed.Hash()) {
		t.Fatal("failed commit exposed staged state")
	}
	loaded, err := New(db)
	must(t, err)
	if !bytes.Equal(before, loaded.committed.Hash()) {
		t.Fatal("failed commit persisted state")
	}
	db.fail = false
	_, err = a.Commit(ctx, &abci.RequestCommit{})
	must(t, err)
	if a.committed.Accounts[bob].Balance != "1" {
		t.Fatal("commit retry lost state")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func key(t *testing.T) mldsa65.PrivKey {
	t.Helper()
	k, err := mldsa65.GenPrivKey()
	must(t, err)
	return k
}
func fixture(t *testing.T, db dbm.DB) (*App, mldsa65.PrivKey, string, *abci.RequestInitChain) {
	t.Helper()
	alice := key(t)
	bob := Address(key(t).PubKey().Bytes())
	g := Genesis{ChainID: "ledger-test", Decimals: 9, MaxSupply: MaxSupply, Fee: "1000", Accounts: map[string]Account{
		ReserveAddress: {Balance: SeparateReserve}, FormerAddress: {Balance: FormerReserve}, PendingAddress: {Balance: "0"}, FeeAddress: {Balance: "0"},
		Address(alice.PubKey().Bytes()): {Balance: "80000000000000000000"}, bob: {Balance: "0"},
	}}
	b, _ := json.Marshal(g)
	vk, err := encoding.PubKeyToProto(key(t).PubKey())
	must(t, err)
	r := &abci.RequestInitChain{ChainId: g.ChainID, InitialHeight: 1, AppStateBytes: b, Validators: []abci.ValidatorUpdate{{PubKey: vk, Power: 1}}, ConsensusParams: &proto.ConsensusParams{Validator: &proto.ValidatorParams{PubKeyTypes: []string{mldsa65.KeyType}}}}
	a, err := New(db)
	must(t, err)
	_, err = a.InitChain(ctx, r)
	must(t, err)
	return a, alice, bob, r
}
func signed(t *testing.T, k mldsa65.PrivKey, to, value string, nonce uint64) []byte {
	t.Helper()
	b, err := Sign(k, UnsignedTx{Version: 1, ChainID: "ledger-test", To: to, Amount: value, Fee: "1000", Nonce: nonce})
	must(t, err)
	return b
}
func TestTransferSupplyFeesAndReplay(t *testing.T) {
	a, alice, bob, _ := fixture(t, dbm.NewMemDB())
	from := Address(alice.PubKey().Bytes())
	tx := signed(t, alice, bob, "10000000000000000000", 1) // exceeds int64; must transfer exactly
	check, err := a.CheckTx(ctx, &abci.RequestCheckTx{Tx: tx})
	must(t, err)
	if check.Code != 0 {
		t.Fatal(check.Log)
	}
	before := a.committed.Hash()
	r, err := a.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 1, Txs: [][]byte{tx, tx}})
	must(t, err)
	if r.TxResults[0].Code != 0 || r.TxResults[1].Code == 0 {
		t.Fatal("duplicate execution was not rejected")
	}
	if !bytes.Equal(before, a.committed.Hash()) {
		t.Fatal("FinalizeBlock changed committed state")
	}
	_, err = a.Commit(ctx, &abci.RequestCommit{})
	must(t, err)
	if a.committed.Accounts[bob].Balance != "10000000000000000000" || a.committed.Accounts[from].Balance != "69999999999999999000" || a.committed.Accounts[FeeAddress].Balance != "1000" {
		t.Fatal(a.committed.Accounts)
	}
	must(t, a.committed.Validate())
	check, err = a.CheckTx(ctx, &abci.RequestCheckTx{Tx: tx})
	must(t, err)
	if check.Code == 0 {
		t.Fatal("replay accepted")
	}
}
func TestInvalidTransfersNeverMutate(t *testing.T) {
	a, alice, bob, _ := fixture(t, dbm.NewMemDB())
	good := signed(t, alice, bob, "1", 1)
	var tx Transaction
	must(t, json.Unmarshal(good, &tx))
	cases := map[string][]byte{"overspend": signed(t, alice, bob, "80000000000000000000", 1), "negative": signed(t, alice, bob, "-1", 1), "zero": signed(t, alice, bob, "0", 1), "leading-zero": signed(t, alice, bob, "01", 1), "overflow": signed(t, alice, bob, "108000000000000000001", 1), "future-nonce": signed(t, alice, bob, "1", 2), "locked-pool": signed(t, alice, ReserveAddress, "1", 1), "unknown-sender": signed(t, key(t), bob, "1", 1), "whitespace": append(good, '\n'), "duplicate-json": append([]byte(`{"version":1,`), good[1:]...), "oversized": bytes.Repeat([]byte("x"), MaxTxBytes+1)}
	tx.Amount = "2"
	cases["tampered-amount"], _ = json.Marshal(tx)
	tx.Amount = "1"
	tx.Signature[0] ^= 1
	cases["tampered-signature"], _ = json.Marshal(tx)
	tx.Signature = nil
	cases["missing-signature"], _ = json.Marshal(tx)
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			s := a.committed.Clone()
			before := s.Hash()
			if s.Apply(b) == nil {
				t.Fatal("accepted invalid transfer")
			}
			if !bytes.Equal(before, s.Hash()) {
				t.Fatal("invalid transfer mutated state")
			}
		})
	}
	for _, mutate := range []func(*UnsignedTx){func(u *UnsignedTx) { u.ChainID = "other-chain" }, func(u *UnsignedTx) { u.Fee = "0" }, func(u *UnsignedTx) { u.Version = 2 }} {
		u := UnsignedTx{Version: 1, ChainID: "ledger-test", To: bob, Amount: "1", Fee: "1000", Nonce: 1}
		mutate(&u)
		raw, err := Sign(alice, u)
		must(t, err)
		s := a.committed.Clone()
		if s.Apply(raw) == nil {
			t.Fatal("invalid signed domain accepted")
		}
	}
}
func TestProposalsValidateSequentialSpending(t *testing.T) {
	a, alice, bob, _ := fixture(t, dbm.NewMemDB())
	tx := signed(t, alice, bob, "70000000000000000000", 1)
	next := signed(t, alice, bob, "20000000000000000000", 2)
	p, err := a.PrepareProposal(ctx, &abci.RequestPrepareProposal{Txs: [][]byte{tx, tx, next}, MaxTxBytes: 1 << 20})
	must(t, err)
	if len(p.Txs) != 1 {
		t.Fatal("invalid proposal selection", len(p.Txs))
	}
	p, err = a.PrepareProposal(ctx, &abci.RequestPrepareProposal{Txs: [][]byte{tx}, MaxTxBytes: int64(len(tx) - 1)})
	must(t, err)
	if len(p.Txs) != 0 {
		t.Fatal("byte limit exceeded")
	}
	for _, raw := range [][][]byte{{tx, tx}, {tx, next}} {
		r, err := a.ProcessProposal(ctx, &abci.RequestProcessProposal{Txs: raw})
		must(t, err)
		if r.Status != abci.ResponseProcessProposal_REJECT {
			t.Fatal("invalid proposal accepted")
		}
	}
	r, err := a.ProcessProposal(ctx, &abci.RequestProcessProposal{Txs: [][]byte{tx}})
	must(t, err)
	if r.Status != abci.ResponseProcessProposal_ACCEPT {
		t.Fatal("valid proposal rejected")
	}
	if a.committed.Accounts[bob].Balance != "0" {
		t.Fatal("proposal mutated state")
	}
}
func TestSelfTransferAndNonceExhaustion(t *testing.T) {
	a, alice, _, _ := fixture(t, dbm.NewMemDB())
	from := Address(alice.PubKey().Bytes())
	s := a.committed.Clone()
	must(t, s.Apply(signed(t, alice, from, "1", 1)))
	if s.Accounts[from].Balance != "79999999999999999000" || s.Accounts[from].Nonce != 1 {
		t.Fatal("self-transfer accounting failed")
	}
	must(t, s.Validate())
	account := s.Accounts[from]
	account.Nonce = ^uint64(0)
	s.Accounts[from] = account
	if s.Apply(signed(t, alice, from, "1", 0)) == nil {
		t.Fatal("nonce wrapped")
	}
}
func TestPersistenceCrashRecoveryAndDeterminism(t *testing.T) {
	dir := t.TempDir()
	db, err := dbm.NewGoLevelDB("ledger", dir)
	must(t, err)
	a, alice, bob, genesis := fixture(t, db)
	tx := signed(t, alice, bob, "12345", 1)
	r, err := a.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 1, Txs: [][]byte{tx}})
	must(t, err)
	expected := r.AppHash
	must(t, db.Close()) // crash before Commit: finalized state must be discarded
	db, err = dbm.NewGoLevelDB("ledger", dir)
	must(t, err)
	a, err = New(db)
	must(t, err)
	if a.committed.Height != 0 || a.committed.Accounts[bob].Balance != "0" {
		t.Fatal("uncommitted state survived")
	}
	_, err = a.InitChain(ctx, genesis)
	must(t, err)
	r, err = a.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 1, Txs: [][]byte{tx}})
	must(t, err)
	if !bytes.Equal(expected, r.AppHash) {
		t.Fatal("replay changed state root")
	}
	_, err = a.Commit(ctx, &abci.RequestCommit{})
	must(t, err)
	must(t, db.Close())
	db, err = dbm.NewGoLevelDB("ledger", dir)
	must(t, err)
	defer db.Close()
	a, err = New(db)
	must(t, err)
	info, err := a.Info(ctx, &abci.RequestInfo{})
	must(t, err)
	if info.LastBlockHeight != 1 || !bytes.Equal(info.LastBlockAppHash, expected) {
		t.Fatal("committed state lost")
	}
	if _, err := a.InitChain(ctx, genesis); err == nil {
		t.Fatal("existing chain reinitialized")
	}
	if _, err := a.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 3}); err == nil {
		t.Fatal("skipped height")
	}
	// Same content with a different map insertion order must have the same root.
	s := a.committed.Clone()
	s.Accounts = map[string]Account{}
	for k, v := range a.committed.Accounts {
		s.Accounts[k] = v
	}
	if !bytes.Equal(s.Hash(), expected) {
		t.Fatal("nondeterministic root")
	}
}
func TestGenesisAndQueryBoundaries(t *testing.T) {
	a, _, bob, init := fixture(t, dbm.NewMemDB())
	corrupt := a.committed.Clone()
	delete(corrupt.Accounts, FeeAddress)
	if corrupt.Validate() == nil {
		t.Fatal("accepted stored state without fee pool")
	}
	for _, mutate := range []func(*Genesis){func(g *Genesis) { g.MaxSupply = "100000000000000000000" }, func(g *Genesis) { g.Decimals = 8 }, func(g *Genesis) { g.Accounts[ReserveAddress] = Account{Balance: "0"} }, func(g *Genesis) { g.Accounts[bob] = Account{Balance: "1"} }} {
		var g Genesis
		must(t, json.Unmarshal(init.AppStateBytes, &g))
		mutate(&g)
		if g.Validate() == nil {
			t.Fatal("invalid genesis accepted")
		}
	}
	for _, r := range []*abci.RequestQuery{{Path: "/supply", Prove: true}, {Path: "/supply", Height: 99}, {Path: "/bad"}, {Path: "/account", Data: []byte("bad")}} {
		q, err := a.Query(ctx, r)
		must(t, err)
		if q.Code == 0 {
			t.Fatal("unsupported query accepted")
		}
	}
	q, err := a.Query(ctx, &abci.RequestQuery{Path: "/account", Data: []byte(bob)})
	must(t, err)
	if q.Code != 0 {
		t.Fatal(q.Log)
	}
}
