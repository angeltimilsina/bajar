package ledgerv2

import (
	"bajar-network-starter/internal/ledger"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	encoding "github.com/cometbft/cometbft/crypto/encoding"
	"github.com/cometbft/cometbft/crypto/mldsa65"
)

var stateKey = []byte("bajar/state/v2")

type App struct {
	abci.BaseApplication
	mu        sync.Mutex
	db        dbm.DB
	committed *State
	pending   *State
}

var _ abci.Application = (*App)(nil)

func New(db dbm.DB) (*App, error) {
	a := &App{db: db}
	raw, err := db.Get(stateKey)
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		var s State
		if err := ledger.Decode(raw, &s); err != nil {
			return nil, err
		}
		if err := s.Validate(); err != nil {
			return nil, err
		}
		a.committed = &s
	}
	return a, nil
}
func (a *App) save(s *State) error {
	if err := s.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return a.db.SetSync(stateKey, b)
}
func (a *App) Info(context.Context, *abci.RequestInfo) (*abci.ResponseInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := &abci.ResponseInfo{Data: "BAJAR development ledger; six-decimal ML-DSA-65 staking ledger", AppVersion: 2}
	if a.committed != nil {
		r.LastBlockHeight = a.committed.Height
		r.LastBlockAppHash = a.committed.Hash()
	}
	return r, nil
}
func (a *App) InitChain(_ context.Context, r *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var g Genesis
	if err := ledger.Decode(r.AppStateBytes, &g); err != nil {
		return nil, err
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	if g.ChainID != r.ChainId || r.InitialHeight != 1 {
		return nil, errors.New("genesis chain ID or height mismatch")
	}
	if len(r.Validators) != 4 || r.ConsensusParams == nil || r.ConsensusParams.Validator == nil || len(r.ConsensusParams.Validator.PubKeyTypes) != 1 || r.ConsensusParams.Validator.PubKeyTypes[0] != mldsa65.KeyType {
		return nil, errors.New("ML-DSA-65 validator consensus parameters required")
	}
	seen := map[string]bool{}
	for _, v := range r.Validators {
		k, err := encoding.PubKeyFromProto(v.PubKey)
		if err != nil || k.Type() != mldsa65.KeyType || v.Power != 1 {
			return nil, errors.New("expected equal-power ML-DSA-65 validators")
		}
		id := string(k.Bytes())
		if seen[id] {
			return nil, errors.New("duplicate validator")
		}
		seen[id] = true
	}
	s, err := NewState(g)
	if err != nil {
		return nil, err
	}
	if a.committed != nil {
		b, _ := json.Marshal(a.committed.Genesis)
		c, _ := json.Marshal(g)
		if !bytes.Equal(b, c) || a.committed.Height != 0 {
			return nil, errors.New("cannot reinitialize existing ledger")
		}
		return &abci.ResponseInitChain{AppHash: a.committed.Hash()}, nil
	}
	if err := a.save(&s); err != nil {
		return nil, err
	}
	a.committed = &s
	return &abci.ResponseInitChain{AppHash: s.Hash()}, nil
}
func (a *App) CheckTx(_ context.Context, r *abci.RequestCheckTx) (*abci.ResponseCheckTx, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.committed == nil {
		return &abci.ResponseCheckTx{Code: 1, Log: "ledger not initialized"}, nil
	}
	s := a.committed.Clone()
	if err := s.Apply(r.Tx, a.committed.Height+1); err != nil {
		return &abci.ResponseCheckTx{Code: 1, Log: err.Error()}, nil
	}
	return &abci.ResponseCheckTx{GasWanted: 1}, nil
}
func (a *App) PrepareProposal(_ context.Context, r *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.committed == nil {
		return nil, errors.New("ledger not initialized")
	}
	s := a.committed.Clone()
	var txs [][]byte
	var used int64
	for _, tx := range r.Txs {
		if int64(len(tx)) > r.MaxTxBytes-used {
			continue
		}
		if s.Apply(tx, a.committed.Height+1) == nil {
			txs = append(txs, tx)
			used += int64(len(tx))
		}
	}
	return &abci.ResponsePrepareProposal{Txs: txs}, nil
}
func (a *App) ProcessProposal(_ context.Context, r *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.committed == nil {
		return nil, errors.New("ledger not initialized")
	}
	s := a.committed.Clone()
	for _, tx := range r.Txs {
		if s.Apply(tx, a.committed.Height+1) != nil {
			return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
		}
	}
	return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
}
func (a *App) FinalizeBlock(_ context.Context, r *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.committed == nil {
		return nil, errors.New("ledger not initialized")
	}
	next := a.committed.Height + 1
	if r.Height != next {
		return nil, fmt.Errorf("unexpected height %d, want %d", r.Height, next)
	}
	s := a.committed.Clone()
	results := make([]*abci.ExecTxResult, len(r.Txs))
	for i, tx := range r.Txs {
		results[i] = &abci.ExecTxResult{}
		if err := s.Apply(tx, a.committed.Height+1); err != nil {
			results[i].Code = 1
			results[i].Log = err.Error()
		}
	}
	s.Height = r.Height
	if err := s.Validate(); err != nil {
		return nil, err
	}
	a.pending = &s
	return &abci.ResponseFinalizeBlock{TxResults: results, AppHash: s.Hash()}, nil
}
func (a *App) Commit(context.Context, *abci.RequestCommit) (*abci.ResponseCommit, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending == nil {
		return nil, errors.New("no finalized state")
	}
	if err := a.save(a.pending); err != nil {
		return nil, err
	}
	a.committed = a.pending
	a.pending = nil
	return &abci.ResponseCommit{}, nil
}
func (a *App) Query(_ context.Context, r *abci.RequestQuery) (*abci.ResponseQuery, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := &abci.ResponseQuery{}
	if a.committed == nil {
		out.Code = 1
		out.Log = "ledger not initialized"
		return out, nil
	}
	out.Height = a.committed.Height
	if r.Prove || (r.Height != 0 && r.Height != out.Height) {
		out.Code = 1
		out.Log = "historical queries and proofs unsupported"
		return out, nil
	}
	var v any
	switch r.Path {
	case "/account":
		address := string(r.Data)
		if !ledger.ValidAddress(address) && !pool(address) {
			out.Code = 1
			out.Log = "invalid address"
			return out, nil
		}
		acc, ok := a.committed.Accounts[address]
		if !ok {
			acc = Account{Balance: "0"}
		}
		v = acc
	case "/supply":
		v = map[string]any{"protocol_version": 2, "base_units": MaxSupply, "decimals": 6, "fee": a.committed.Genesis.Fee, "chain_id": a.committed.Genesis.ChainID, "separate_reserve": SeparateReserve}
	case "/staking":
		address := string(r.Data)
		if !ledger.ValidAddress(address) {
			out.Code = 1
			out.Log = "invalid address"
			return out, nil
		}
		position, claimable := a.committed.PositionView(address)
		v = map[string]any{"position": position, "claimable": claimable, "total_bonded": a.committed.TotalBonded, "reward_index": a.committed.RewardIndex}
	case "/state":
		v = a.committed
	default:
		out.Code = 1
		out.Log = "unknown query path"
		return out, nil
	}
	out.Value, _ = json.Marshal(v)
	return out, nil
}
