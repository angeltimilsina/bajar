// Package ledger implements a local-development BAJ bank, not a production chain.
package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"

	"github.com/cometbft/cometbft/crypto/mldsa65"
)

const (
	MaxSupply       = "108000000000000000000"
	SeparateReserve = "8000000000000000000"
	FormerReserve   = "20000000000000000000"
	ReserveAddress  = "pool:separate_reserve"
	FormerAddress   = "pool:former_staking_reserve"
	PendingAddress  = "pool:tokenomics_pending"
	FeeAddress      = "pool:fees"
	MaxTxBytes      = 16384
)

type Account struct {
	Balance string `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}
type Genesis struct {
	ChainID   string             `json:"chain_id"`
	Decimals  int                `json:"decimals"`
	MaxSupply string             `json:"max_supply"`
	Fee       string             `json:"fee"`
	Accounts  map[string]Account `json:"accounts"`
}
type State struct {
	Genesis       Genesis            `json:"genesis"`
	InitialHeight int64              `json:"initial_height"`
	Height        int64              `json:"height"`
	Accounts      map[string]Account `json:"accounts"`
}

// UnsignedTx is serialized in this field order. Amounts and fees are base units.
type UnsignedTx struct {
	Version   uint32 `json:"version"`
	ChainID   string `json:"chain_id"`
	PublicKey []byte `json:"public_key"`
	To        string `json:"to"`
	Amount    string `json:"amount"`
	Fee       string `json:"fee"`
	Nonce     uint64 `json:"nonce"`
}
type Transaction struct {
	UnsignedTx
	Signature []byte `json:"signature"`
}

func Decode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}
func amount(s string) (*big.Int, error) {
	if len(s) == 0 || len(s) > len(MaxSupply) || (len(s) > 1 && s[0] == '0') {
		return nil, errors.New("noncanonical amount")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return nil, errors.New("invalid amount")
		}
	}
	n, _ := new(big.Int).SetString(s, 10)
	max, _ := new(big.Int).SetString(MaxSupply, 10)
	if n.Cmp(max) > 0 {
		return nil, errors.New("amount exceeds supply")
	}
	return n, nil
}
func Address(publicKey []byte) string {
	h := sha256.Sum256(append([]byte("BAJAR/wallet/ml-dsa-65/v1\x00"), publicKey...))
	return "baj1" + hex.EncodeToString(h[:])
}
func ValidAddress(s string) bool {
	if len(s) != 68 || !strings.HasPrefix(s, "baj1") || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s[4:])
	return err == nil
}
func pool(s string) bool {
	return s == ReserveAddress || s == FormerAddress || s == PendingAddress || s == FeeAddress
}
func (g Genesis) Validate() error {
	if g.ChainID == "" || len(g.ChainID) > 50 || strings.TrimSpace(g.ChainID) != g.ChainID || g.Decimals != 9 || g.MaxSupply != MaxSupply {
		return errors.New("invalid genesis chain, precision or supply")
	}
	f, err := amount(g.Fee)
	if err != nil || f.Sign() == 0 {
		return errors.New("genesis fee must be positive base units")
	}
	if g.Accounts[ReserveAddress].Balance != SeparateReserve || g.Accounts[FormerAddress].Balance != FormerReserve || g.Accounts[FeeAddress].Balance != "0" {
		return errors.New("reserve pools must remain locked and fees initially zero")
	}
	if _, ok := g.Accounts[PendingAddress]; !ok {
		return errors.New("missing pending tokenomics pool")
	}
	for a, v := range g.Accounts {
		if (!ValidAddress(a) && !pool(a)) || v.Nonce != 0 {
			return errors.New("invalid genesis account")
		}
	}
	return validateAccounts(g.Accounts)
}
func validateAccounts(accounts map[string]Account) error {
	for _, address := range []string{ReserveAddress, FormerAddress, PendingAddress, FeeAddress} {
		if _, ok := accounts[address]; !ok {
			return errors.New("missing required pool")
		}
	}
	total := new(big.Int)
	for a, v := range accounts {
		if !ValidAddress(a) && !pool(a) {
			return errors.New("invalid state address")
		}
		n, err := amount(v.Balance)
		if err != nil {
			return err
		}
		total.Add(total, n)
		if pool(a) && v.Nonce != 0 {
			return errors.New("pool nonce must be zero")
		}
	}
	if total.String() != MaxSupply {
		return fmt.Errorf("supply conservation failed: %s", total)
	}
	if accounts[ReserveAddress].Balance != SeparateReserve || accounts[FormerAddress].Balance != FormerReserve {
		return errors.New("locked reserve changed")
	}
	return nil
}
func cloneAccounts(src map[string]Account) map[string]Account {
	dst := make(map[string]Account, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
func (s State) Clone() State { s.Accounts = cloneAccounts(s.Accounts); return s }
func (s State) Hash() []byte {
	// encoding/json sorts map keys; only fixed structs, strings and integers occur.
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)
	return h[:]
}
func (s State) Validate() error {
	if err := s.Genesis.Validate(); err != nil {
		return err
	}
	if s.InitialHeight < 1 || (s.Height != 0 && s.Height < s.InitialHeight) {
		return errors.New("invalid stored height")
	}
	if s.Accounts[PendingAddress] != s.Genesis.Accounts[PendingAddress] {
		return errors.New("pending pool changed")
	}
	return validateAccounts(s.Accounts)
}
func Sign(key mldsa65.PrivKey, tx UnsignedTx) ([]byte, error) {
	tx.PublicKey = key.PubKey().Bytes()
	b, _ := json.Marshal(tx)
	sig, err := key.Sign(append([]byte("BAJAR/transfer/v1\n"), b...))
	if err != nil {
		return nil, err
	}
	return json.Marshal(Transaction{UnsignedTx: tx, Signature: sig})
}
func parseTx(raw []byte, chainID string, fee string) (Transaction, string, error) {
	var t Transaction
	if len(raw) > MaxTxBytes {
		return t, "", errors.New("transaction too large")
	}
	if err := Decode(raw, &t); err != nil {
		return t, "", err
	}
	canonical, _ := json.Marshal(t)
	if !bytes.Equal(canonical, raw) {
		return t, "", errors.New("transaction must use canonical JSON")
	}
	if t.Version != 1 || t.ChainID != chainID || !ValidAddress(t.To) || t.Nonce == 0 || t.Fee != fee {
		return t, "", errors.New("invalid version, chain, recipient, nonce or fee")
	}
	n, err := amount(t.Amount)
	if err != nil || n.Sign() == 0 {
		return t, "", errors.New("amount must be positive")
	}
	key, err := mldsa65.NewPubKeyFromBytes(t.PublicKey)
	if err != nil {
		return t, "", errors.New("invalid ML-DSA-65 public key")
	}
	msg, _ := json.Marshal(t.UnsignedTx)
	if !key.VerifySignature(append([]byte("BAJAR/transfer/v1\n"), msg...), t.Signature) {
		return t, "", errors.New("invalid ML-DSA-65 signature")
	}
	return t, Address(t.PublicKey), nil
}

// Apply changes state only after every check succeeds. Locked pools have no signing key.
func (s *State) Apply(raw []byte) error {
	t, from, err := parseTx(raw, s.Genesis.ChainID, s.Genesis.Fee)
	if err != nil {
		return err
	}
	a, ok := s.Accounts[from]
	if !ok || a.Nonce == math.MaxUint64 || t.Nonce != a.Nonce+1 {
		return errors.New("unknown sender or incorrect nonce")
	}
	balance, _ := amount(a.Balance)
	value, _ := amount(t.Amount)
	fee, _ := amount(t.Fee)
	cost := new(big.Int).Add(value, fee)
	if balance.Cmp(cost) < 0 {
		return errors.New("insufficient funds")
	}
	a.Balance = new(big.Int).Sub(balance, cost).String()
	a.Nonce = t.Nonce
	s.Accounts[from] = a
	recipient := s.Accounts[t.To]
	if recipient.Balance == "" {
		recipient.Balance = "0"
	}
	n, _ := amount(recipient.Balance)
	recipient.Balance = n.Add(n, value).String()
	s.Accounts[t.To] = recipient
	fees := s.Accounts[FeeAddress]
	n, _ = amount(fees.Balance)
	fees.Balance = n.Add(n, fee).String()
	s.Accounts[FeeAddress] = fees
	return nil
}
