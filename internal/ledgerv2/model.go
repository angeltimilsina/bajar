// Package ledgerv2 is the six-decimal, optional-staking protocol. It never opens
// or migrates the nine-decimal ledger's database.
package ledgerv2

import (
	"bajar-network-starter/internal/ledger"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	"math"
	"math/big"
)

const (
	MaxSupply       = "108000000000000000"
	SeparateReserve = "8000000000000000"
	FormerReserve   = "20000000000000000"
	PendingSupply   = "80000000000000000"
	StakeAddress    = "pool:staking"
	Scale           = "1000000000000000000000000"
	MaxTxBytes      = 16384
	MaxAccounts     = 10000
	MaxPositions    = 1000
	MaxUnbondings   = 16
)

type Account = ledger.Account
type Genesis struct {
	ProtocolVersion int                `json:"protocol_version"`
	ChainID         string             `json:"chain_id"`
	Decimals        int                `json:"decimals"`
	MaxSupply       string             `json:"max_supply"`
	Fee             string             `json:"fee"`
	UnbondingBlocks int64              `json:"unbonding_blocks"`
	Accounts        map[string]Account `json:"accounts"`
}
type Unbonding struct {
	Amount        string `json:"amount"`
	ReleaseHeight int64  `json:"release_height"`
}
type Position struct {
	Bonded        string      `json:"bonded"`
	Index         string      `json:"index"`
	AccruedScaled string      `json:"accrued_scaled"`
	Unbonding     []Unbonding `json:"unbonding"`
}
type State struct {
	Genesis     Genesis             `json:"genesis"`
	Height      int64               `json:"height"`
	Accounts    map[string]Account  `json:"accounts"`
	Positions   map[string]Position `json:"positions"`
	TotalBonded string              `json:"total_bonded"`
	RewardIndex string              `json:"reward_index"`
}
type UnsignedTx struct {
	Version   uint32 `json:"version"`
	ChainID   string `json:"chain_id"`
	PublicKey []byte `json:"public_key"`
	Kind      string `json:"kind"`
	To        string `json:"to"`
	Amount    string `json:"amount"`
	Fee       string `json:"fee"`
	Nonce     uint64 `json:"nonce"`
}
type Transaction struct {
	UnsignedTx
	Signature []byte `json:"signature"`
}

func integer(s string, limit int) (*big.Int, error) {
	if len(s) == 0 || len(s) > limit || (len(s) > 1 && s[0] == '0') {
		return nil, errors.New("noncanonical integer")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return nil, errors.New("invalid integer")
		}
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, errors.New("invalid integer")
	}
	return n, nil
}
func coin(s string) (*big.Int, error) {
	n, e := integer(s, len(MaxSupply))
	if e != nil {
		return nil, e
	}
	if n.Cmp(num(MaxSupply)) > 0 {
		return nil, errors.New("amount exceeds cap")
	}
	return n, nil
}
func num(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid validated integer")
	}
	return n
}
func pool(a string) bool {
	return a == ledger.ReserveAddress || a == ledger.FormerAddress || a == ledger.PendingAddress || a == ledger.FeeAddress || a == StakeAddress
}
func (g Genesis) Validate() error {
	if g.ProtocolVersion != 2 || g.Decimals != 6 || g.MaxSupply != MaxSupply || g.ChainID == "" || len(g.ChainID) > 50 || g.UnbondingBlocks < 1 || g.UnbondingBlocks > 1_000_000 {
		return errors.New("invalid v2 genesis parameters")
	}
	fee, e := coin(g.Fee)
	if e != nil || fee.Sign() == 0 {
		return errors.New("positive fixed fee required")
	}
	for a, v := range g.Accounts {
		if (!pool(a) && !ledger.ValidAddress(a)) || v.Nonce != 0 {
			return errors.New("invalid genesis account")
		}
	}
	if g.Accounts[ledger.FeeAddress].Balance != "0" || g.Accounts[StakeAddress].Balance != "0" {
		return errors.New("fee and staking pools must start empty")
	}
	return validateAccounts(g.Accounts)
}
func validateAccounts(a map[string]Account) error {
	if len(a) > MaxAccounts {
		return errors.New("account limit reached")
	}
	for _, p := range []string{ledger.ReserveAddress, ledger.FormerAddress, ledger.PendingAddress, ledger.FeeAddress, StakeAddress} {
		if _, ok := a[p]; !ok {
			return errors.New("missing pool")
		}
	}
	total := new(big.Int)
	for address, v := range a {
		if !pool(address) && !ledger.ValidAddress(address) {
			return errors.New("invalid address")
		}
		n, e := coin(v.Balance)
		if e != nil {
			return e
		}
		total.Add(total, n)
		if pool(address) && v.Nonce != 0 {
			return errors.New("pool nonce changed")
		}
	}
	if total.String() != MaxSupply {
		return errors.New("108B supply conservation failed")
	}
	if a[ledger.ReserveAddress].Balance != SeparateReserve || a[ledger.FormerAddress].Balance != FormerReserve {
		return errors.New("locked reserve changed")
	}
	return nil
}
func NewState(g Genesis) (State, error) {
	s := State{Genesis: g, Accounts: copyAccounts(g.Accounts), Positions: map[string]Position{}, TotalBonded: "0", RewardIndex: "0"}
	return s, s.Validate()
}
func copyAccounts(a map[string]Account) map[string]Account {
	b := make(map[string]Account, len(a))
	for k, v := range a {
		b[k] = v
	}
	return b
}
func (s State) Clone() State {
	s.Accounts = copyAccounts(s.Accounts)
	p := make(map[string]Position, len(s.Positions))
	for k, v := range s.Positions {
		if v.Unbonding != nil {
			v.Unbonding = append(make([]Unbonding, 0, len(v.Unbonding)), v.Unbonding...)
		}
		p[k] = v
	}
	s.Positions = p
	return s
}
func (s State) Hash() []byte {
	b, e := json.Marshal(s)
	if e != nil {
		panic(e)
	}
	h := sha256.Sum256(b)
	return h[:]
}
func (s State) Validate() error {
	if len(s.Positions) > MaxPositions {
		return errors.New("staking position limit reached")
	}
	if e := s.Genesis.Validate(); e != nil {
		return e
	}
	if e := validateAccounts(s.Accounts); e != nil {
		return e
	}
	if s.Height < 0 || s.Accounts[ledger.PendingAddress] != s.Genesis.Accounts[ledger.PendingAddress] {
		return errors.New("invalid height or pending pool")
	}
	index, e := integer(s.RewardIndex, 128)
	if e != nil {
		return e
	}
	bonded, e := coin(s.TotalBonded)
	if e != nil {
		return e
	}
	active, held, liability := new(big.Int), new(big.Int), new(big.Int)
	for a, p := range s.Positions {
		if !ledger.ValidAddress(a) {
			return errors.New("invalid position address")
		}
		if _, ok := s.Accounts[a]; !ok {
			return errors.New("position without account")
		}
		b, e := coin(p.Bonded)
		if e != nil {
			return e
		}
		at, e := integer(p.Index, 128)
		if e != nil || at.Cmp(index) > 0 {
			return errors.New("invalid position checkpoint")
		}
		pending, e := integer(p.AccruedScaled, 128)
		if e != nil {
			return e
		}
		active.Add(active, b)
		held.Add(held, b)
		owed := new(big.Int).Mul(b, new(big.Int).Sub(index, at))
		owed.Add(owed, pending)
		liability.Add(liability, owed)
		if len(p.Unbonding) > MaxUnbondings {
			return errors.New("too many unbonding entries")
		}
		for _, u := range p.Unbonding {
			n, e := coin(u.Amount)
			if e != nil || n.Sign() == 0 || u.ReleaseHeight < 1 {
				return errors.New("invalid unbonding entry")
			}
			held.Add(held, n)
		}
	}
	if active.Cmp(bonded) != 0 || held.String() != s.Accounts[StakeAddress].Balance {
		return errors.New("staking collateral mismatch")
	}
	backing := new(big.Int).Mul(num(s.Accounts[ledger.FeeAddress].Balance), num(Scale))
	if liability.Cmp(backing) > 0 {
		return errors.New("rewards exceed collected fees")
	}
	return nil
}
func Sign(k mldsa65.PrivKey, t UnsignedTx) ([]byte, error) {
	t.PublicKey = k.PubKey().Bytes()
	b, _ := json.Marshal(t)
	sig, e := k.Sign(append([]byte("BAJAR/staking/v2\n"), b...))
	if e != nil {
		return nil, e
	}
	return json.Marshal(Transaction{UnsignedTx: t, Signature: sig})
}
func parse(raw []byte, g Genesis) (Transaction, string, error) {
	var t Transaction
	if len(raw) > MaxTxBytes {
		return t, "", errors.New("transaction too large")
	}
	if e := ledger.Decode(raw, &t); e != nil {
		return t, "", e
	}
	b, _ := json.Marshal(t)
	if !bytes.Equal(raw, b) {
		return t, "", errors.New("noncanonical transaction")
	}
	if t.Version != 2 || t.ChainID != g.ChainID || t.Fee != g.Fee || t.Nonce == 0 {
		return t, "", errors.New("invalid transaction domain, fee or nonce")
	}
	n, e := coin(t.Amount)
	if e != nil {
		return t, "", e
	}
	switch t.Kind {
	case "transfer":
		if !ledger.ValidAddress(t.To) || n.Sign() == 0 {
			return t, "", errors.New("invalid transfer")
		}
	case "stake", "unstake":
		if t.To != "" || n.Sign() == 0 {
			return t, "", errors.New("invalid stake amount")
		}
	case "claim", "withdraw":
		if t.To != "" || t.Amount != "0" {
			return t, "", errors.New("claim/withdraw take no amount")
		}
	default:
		return t, "", errors.New("unsupported transaction kind")
	}
	k, e := mldsa65.NewPubKeyFromBytes(t.PublicKey)
	if e != nil {
		return t, "", e
	}
	b, _ = json.Marshal(t.UnsignedTx)
	if !k.VerifySignature(append([]byte("BAJAR/staking/v2\n"), b...), t.Signature) {
		return t, "", errors.New("invalid ML-DSA-65 signature")
	}
	return t, ledger.Address(t.PublicKey), nil
}
func (s *State) credit(a string, n *big.Int) {
	v := s.Accounts[a]
	if v.Balance == "" {
		v.Balance = "0"
	}
	v.Balance = new(big.Int).Add(num(v.Balance), n).String()
	s.Accounts[a] = v
}
func (s *State) debit(a string, n *big.Int) {
	v := s.Accounts[a]
	v.Balance = new(big.Int).Sub(num(v.Balance), n).String()
	s.Accounts[a] = v
}
func (s *State) settle(a string) Position {
	p, ok := s.Positions[a]
	if !ok {
		return Position{Bonded: "0", Index: s.RewardIndex, AccruedScaled: "0"}
	}
	earned := new(big.Int).Mul(num(p.Bonded), new(big.Int).Sub(num(s.RewardIndex), num(p.Index)))
	p.AccruedScaled = earned.Add(earned, num(p.AccruedScaled)).String()
	p.Index = s.RewardIndex
	return p
}

// PositionView includes earned but unclaimed fractional rewards without mutation.
func (s State) PositionView(a string) (Position, string) {
	p := s.settle(a)
	claimable := new(big.Int).Quo(num(p.AccruedScaled), num(Scale))
	return p, claimable.String()
}

// Apply is atomic, including fees and reward allocation, on all failure paths.
func (s *State) Apply(raw []byte, height int64) error {
	t, from, e := parse(raw, s.Genesis)
	if e != nil {
		return e
	}
	if height < 1 || height > math.MaxInt64-s.Genesis.UnbondingBlocks {
		return errors.New("invalid execution height")
	}
	a, ok := s.Accounts[from]
	if !ok || a.Nonce == math.MaxUint64 || t.Nonce != a.Nonce+1 {
		return errors.New("unknown sender or nonce replay")
	}
	cost := num(t.Fee)
	if t.Kind == "transfer" || t.Kind == "stake" {
		cost.Add(cost, num(t.Amount))
	}
	if num(a.Balance).Cmp(cost) < 0 {
		return errors.New("insufficient liquid balance for amount and fee")
	}
	next := s.Clone()
	if e := next.execute(t, from, height); e != nil {
		return e
	}
	if e := next.Validate(); e != nil {
		return fmt.Errorf("invalid transition: %w", e)
	}
	*s = next
	return nil
}
func (s *State) execute(t Transaction, from string, height int64) error {
	fee := num(t.Fee)
	s.debit(from, fee)
	s.credit(ledger.FeeAddress, fee)
	// Fees are allocated to stake active immediately before this action. New stake
	// never receives its entry fee or fees accumulated while nobody was staking.
	if num(s.TotalBonded).Sign() > 0 {
		delta := new(big.Int).Quo(new(big.Int).Mul(fee, num(Scale)), num(s.TotalBonded))
		s.RewardIndex = new(big.Int).Add(num(s.RewardIndex), delta).String()
	}
	p := s.settle(from)
	_, hadPosition := s.Positions[from]
	switch t.Kind {
	case "transfer":
		s.debit(from, num(t.Amount))
		s.credit(t.To, num(t.Amount))
	case "stake":
		s.debit(from, num(t.Amount))
		s.credit(StakeAddress, num(t.Amount))
		p.Bonded = new(big.Int).Add(num(p.Bonded), num(t.Amount)).String()
		s.TotalBonded = new(big.Int).Add(num(s.TotalBonded), num(t.Amount)).String()
		hadPosition = true
	case "unstake":
		if num(p.Bonded).Cmp(num(t.Amount)) < 0 || len(p.Unbonding) >= MaxUnbondings {
			return errors.New("insufficient bonded stake or unbonding queue full")
		}
		p.Bonded = new(big.Int).Sub(num(p.Bonded), num(t.Amount)).String()
		s.TotalBonded = new(big.Int).Sub(num(s.TotalBonded), num(t.Amount)).String()
		p.Unbonding = append(p.Unbonding, Unbonding{Amount: t.Amount, ReleaseHeight: height + s.Genesis.UnbondingBlocks})
	case "claim":
		reward, remainder := new(big.Int), new(big.Int)
		reward.QuoRem(num(p.AccruedScaled), num(Scale), remainder)
		if reward.Sign() == 0 {
			return errors.New("no whole base-unit reward to claim")
		}
		s.debit(ledger.FeeAddress, reward)
		s.credit(from, reward)
		p.AccruedScaled = remainder.String()
	case "withdraw":
		total := new(big.Int)
		pending := make([]Unbonding, 0, len(p.Unbonding))
		for _, u := range p.Unbonding {
			if u.ReleaseHeight <= height {
				total.Add(total, num(u.Amount))
			} else {
				pending = append(pending, u)
			}
		}
		if total.Sign() == 0 {
			return errors.New("no matured unbonding balance")
		}
		p.Unbonding = pending
		s.debit(StakeAddress, total)
		s.credit(from, total)
	}
	if hadPosition {
		s.Positions[from] = p
	}
	a := s.Accounts[from]
	a.Nonce = t.Nonce
	s.Accounts[from] = a
	return nil
}
