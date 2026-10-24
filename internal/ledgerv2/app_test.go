package ledgerv2

import (
	"bytes"
	"context"
	"encoding/json"
	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	encoding "github.com/cometbft/cometbft/crypto/encoding"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	proto "github.com/cometbft/cometbft/proto/tendermint/types"
	"testing"
)

func TestABCIStakingReplayPersistenceAndProposalIsolation(t *testing.T) {
	ctx := context.Background()
	s, k, a := fixture(t)
	raw, e := json.Marshal(s.Genesis)
	must(t, e)
	var validators []abci.ValidatorUpdate
	for i := 0; i < 4; i++ {
		key, e := mldsa65.GenPrivKey()
		must(t, e)
		pk, e := encoding.PubKeyToProto(key.PubKey())
		must(t, e)
		validators = append(validators, abci.ValidatorUpdate{PubKey: pk, Power: 1})
	}
	init := &abci.RequestInitChain{ChainId: s.Genesis.ChainID, InitialHeight: 1, AppStateBytes: raw, Validators: validators, ConsensusParams: &proto.ConsensusParams{Validator: &proto.ValidatorParams{PubKeyTypes: []string{mldsa65.KeyType}}}}
	dir := t.TempDir()
	db, e := dbm.NewGoLevelDB("v2", dir)
	must(t, e)
	app, e := New(db)
	must(t, e)
	_, e = app.InitChain(ctx, init)
	must(t, e)
	stake := tx(t, s, k[0], "stake", "", "100")
	before := app.committed.Hash()
	proposal, e := app.PrepareProposal(ctx, &abci.RequestPrepareProposal{Height: 1, Txs: [][]byte{stake, stake}, MaxTxBytes: 100000})
	must(t, e)
	if len(proposal.Txs) != 1 {
		t.Fatal("duplicate stake selected")
	}
	if !bytes.Equal(before, app.committed.Hash()) {
		t.Fatal("proposal mutated committed state")
	}
	rejected, e := app.ProcessProposal(ctx, &abci.RequestProcessProposal{Height: 1, Txs: [][]byte{stake, stake}})
	must(t, e)
	if rejected.Status != abci.ResponseProcessProposal_REJECT {
		t.Fatal("double stake accepted")
	}
	result, e := app.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 1, Txs: [][]byte{stake}})
	must(t, e)
	if result.TxResults[0].Code != 0 {
		t.Fatal(result.TxResults[0].Log)
	}
	expected := result.AppHash
	must(t, db.Close()) // uncommitted stake must not survive
	db, e = dbm.NewGoLevelDB("v2", dir)
	must(t, e)
	app, e = New(db)
	must(t, e)
	if app.committed.TotalBonded != "0" {
		t.Fatal("uncommitted collateral survived")
	}
	_, e = app.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{Height: 1, Txs: [][]byte{stake}})
	must(t, e)
	_, e = app.Commit(ctx, &abci.RequestCommit{})
	must(t, e)
	must(t, db.Close())
	db, e = dbm.NewGoLevelDB("v2", dir)
	must(t, e)
	defer db.Close()
	app, e = New(db)
	must(t, e)
	if !bytes.Equal(expected, app.committed.Hash()) || app.committed.Positions[a[0]].Bonded != "100" {
		t.Fatal("stake state/root changed on restart")
	}
	check, e := app.CheckTx(ctx, &abci.RequestCheckTx{Tx: stake})
	must(t, e)
	if check.Code == 0 {
		t.Fatal("stake replay after restart")
	}
}
func TestClonePreservesEmptyUnbondingSerialization(t *testing.T) {
	s, _, a := fixture(t)
	s.Positions[a[0]] = Position{Bonded: "0", Index: "0", AccruedScaled: "0", Unbonding: []Unbonding{}}
	if !bytes.Equal(s.Hash(), s.Clone().Hash()) {
		t.Fatal("clone changed empty queue encoding")
	}
}
