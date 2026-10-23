// Package bootstrap defines the explicit bajar-testnet-1 ceremony and startup policy.
package bootstrap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bajar-network-starter/internal/ledger"
	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	proto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/types"
)

const ChainID = "bajar-testnet-1"
const ProfileFile = "config/testnet.json"
const MaxJSON = 2 << 20

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`)
var hostRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)

// Identity contains only public information and is safe to exchange out of band.
type Identity struct {
	Version    int    `json:"version"`
	ChainID    string `json:"chain_id"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	NodeID     string `json:"node_id"`
	P2PAddress string `json:"p2p_address"`
	PublicKey  []byte `json:"public_key,omitempty"`
}
type Profile struct {
	Identity      Identity   `json:"identity"`
	GenesisSHA256 string     `json:"genesis_sha256"`
	Peers         []Identity `json:"peers"`
}
type InitOptions struct{ Home, Role, Name, Advertise, P2PListen, RPCListen string }

func ReadJSON(path string, v any) error {
	b, err := readBounded(path)
	if err != nil {
		return err
	}
	if len(b) > MaxJSON {
		return errors.New("JSON file too large")
	}
	return ledger.Decode(b, v)
}
func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxJSON+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxJSON {
		return nil, errors.New("JSON file too large")
	}
	return b, nil
}
func readSignState(path string, state *privval.FilePVLastSignState) error {
	b, err := readBounded(path)
	if err != nil {
		return err
	}
	return cmtjson.Unmarshal(b, state)
}
func writeNew(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
func replaceProfile(home string, p Profile) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	// A same-directory rename keeps readers from seeing partial policy files.
	f, err := os.CreateTemp(filepath.Join(home, "config"), ".testnet-policy-")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(home, ProfileFile))
}
func validHostPort(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("expected host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return errors.New("invalid port")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("advertised address cannot be wildcard or multicast")
		}
		return nil
	}
	if !hostRE.MatchString(host) || strings.Contains(host, "..") {
		return errors.New("invalid advertised hostname")
	}
	return nil
}
func Listen(address string, loopback bool) error {
	if !strings.HasPrefix(address, "tcp://") {
		return errors.New("TCP listener required")
	}
	host, port, err := net.SplitHostPort(strings.TrimPrefix(address, "tcp://"))
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	n, e := strconv.Atoi(port)
	if ip == nil || ip.IsMulticast() || e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port || (loopback && !ip.IsLoopback()) {
		return errors.New("invalid listener; RPC must bind a numeric loopback IP")
	}
	return nil
}
func (i Identity) Validate() error {
	if i.Version != 1 || i.ChainID != ChainID || !nameRE.MatchString(i.Name) {
		return errors.New("invalid identity version, chain or name")
	}
	b, err := hex.DecodeString(i.NodeID)
	if err != nil || len(b) != 20 || strings.ToLower(i.NodeID) != i.NodeID {
		return errors.New("invalid P2P node ID")
	}
	if err := validHostPort(i.P2PAddress); err != nil {
		return err
	}
	switch i.Role {
	case "validator":
		if _, err := mldsa65.NewPubKeyFromBytes(i.PublicKey); err != nil {
			return errors.New("invalid ML-DSA-65 validator public key")
		}
	case "rpc":
		if len(i.PublicKey) != 0 {
			return errors.New("RPC identity must not contain a consensus key")
		}
	default:
		return errors.New("role must be validator or rpc")
	}
	return nil
}
func NewConfig(home string, i Identity, p2pListen, rpcListen string) (*cfg.Config, error) {
	if err := i.Validate(); err != nil {
		return nil, err
	}
	if err := Listen(p2pListen, false); err != nil {
		return nil, err
	}
	if err := Listen(rpcListen, true); err != nil {
		return nil, err
	}
	c := cfg.DefaultConfig()
	c.SetRoot(home)
	c.Moniker = i.Name
	c.ProxyApp = "bajar-embedded"
	c.P2P.ListenAddress = p2pListen
	c.P2P.ExternalAddress = i.P2PAddress
	c.P2P.PexReactor = false
	c.P2P.AddrBookStrict = false
	c.P2P.AllowDuplicateIP = false
	c.P2P.MaxNumInboundPeers = 20
	c.P2P.MaxNumOutboundPeers = 10
	c.RPC.ListenAddress = rpcListen
	c.RPC.Unsafe = false
	c.RPC.PprofListenAddress = ""
	c.RPC.GRPCListenAddress = ""
	c.RPC.MaxOpenConnections = 64
	c.RPC.MaxSubscriptionClients = 16
	c.RPC.MaxSubscriptionsPerClient = 4
	c.RPC.MaxRequestBatchSize = 1
	c.RPC.MaxBodyBytes = 64 << 10
	c.Instrumentation.Prometheus = false
	c.StateSync.Enable = false
	c.Consensus.TimeoutCommit = time.Second
	return c, c.ValidateBasic()
}

// Init creates exactly one node's secrets on that node. Existing homes are refused.
func Init(o InitOptions) (Identity, error) {
	var i Identity
	home, err := filepath.Abs(o.Home)
	if err != nil {
		return i, err
	}
	if _, err := os.Lstat(home); !os.IsNotExist(err) {
		return i, errors.New("node home already exists; refusing to replace identities")
	}
	if o.Role != "validator" && o.Role != "rpc" {
		return i, errors.New("role must be validator or rpc")
	}
	if !nameRE.MatchString(o.Name) || validHostPort(o.Advertise) != nil || Listen(o.P2PListen, false) != nil || Listen(o.RPCListen, true) != nil {
		return i, errors.New("invalid initialization name or listeners")
	}
	if err := os.MkdirAll(filepath.Dir(home), 0700); err != nil {
		return i, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(home), ".bajar-init-")
	if err != nil {
		return i, err
	}
	for _, dir := range []string{"config", "data"} {
		if err := os.Mkdir(filepath.Join(staging, dir), 0700); err != nil {
			return i, err
		}
	}
	c := cfg.DefaultConfig()
	c.SetRoot(staging)
	nk, err := p2p.LoadOrGenNodeKey(c.NodeKeyFile())
	if err != nil {
		return i, err
	}
	i = Identity{Version: 1, ChainID: ChainID, Role: o.Role, Name: o.Name, NodeID: string(nk.ID()), P2PAddress: o.Advertise}
	if o.Role == "validator" {
		key, err := mldsa65.GenPrivKey()
		if err != nil {
			return i, err
		}
		i.PublicKey = key.PubKey().Bytes()
		privval.NewFilePV(key, c.PrivValidatorKeyFile(), c.PrivValidatorStateFile()).Save()
	}
	c, err = NewConfig(home, i, o.P2PListen, o.RPCListen)
	if err != nil {
		return i, err
	}
	cfg.WriteConfigFile(filepath.Join(staging, "config/config.toml"), c)
	b, _ := json.MarshalIndent(Profile{Identity: i}, "", "  ")
	if err := writeNew(filepath.Join(staging, ProfileFile), b); err != nil {
		return i, err
	}
	b, _ = json.MarshalIndent(i, "", "  ")
	if err := writeNew(filepath.Join(staging, "identity.json"), b); err != nil {
		return i, err
	}
	if err := os.Rename(staging, home); err != nil {
		return i, err
	}
	return i, nil
}
func LoadProfile(home string) (Profile, error) {
	var p Profile
	err := ReadJSON(filepath.Join(home, ProfileFile), &p)
	if err == nil {
		err = p.Identity.Validate()
	}
	return p, err
}

func PeerString(own Identity, peers []Identity) (string, error) {
	if len(peers) < 1 || len(peers) > 16 {
		return "", errors.New("one to sixteen explicitly approved peers required")
	}
	seen := map[string]bool{own.NodeID: true}
	addresses := map[string]bool{own.P2PAddress: true}
	var result []string
	for _, i := range peers {
		if err := i.Validate(); err != nil {
			return "", err
		}
		if seen[i.NodeID] || addresses[i.P2PAddress] {
			return "", errors.New("duplicate/self peer identity or address")
		}
		seen[i.NodeID] = true
		addresses[i.P2PAddress] = true
		result = append(result, i.NodeID+"@"+i.P2PAddress)
	}
	return strings.Join(result, ","), nil
}
func Configure(home string, peers []Identity) error {
	p, err := LoadProfile(home)
	if err != nil {
		return err
	}
	s, err := PeerString(p.Identity, peers)
	if err != nil {
		return err
	}
	c, err := LoadConfig(home)
	if err != nil {
		return err
	}
	c.P2P.PersistentPeers = s
	// Save policy first: an interrupted update fails closed until repeated.
	p.Peers = peers
	if err := replaceProfile(home, p); err != nil {
		return err
	}
	cfg.WriteConfigFile(filepath.Join(home, "config/config.toml"), c)
	return nil
}
func LoadConfig(home string) (*cfg.Config, error) { // shared with startup
	c := cfg.DefaultConfig()
	c.SetRoot(home)
	return readConfig(home, c)
}

// Assemble uses public validator identities only. Optional funding is explicitly
// supplied nonvaluable test allocation, deducted solely from the pending 80B pool.
func Assemble(ids []Identity, when time.Time, funding map[string]string) (*types.GenesisDoc, error) {
	if len(ids) != 4 || when.IsZero() || when.Location() != time.UTC {
		return nil, errors.New("four validators and an explicit UTC genesis time required")
	}
	accounts := map[string]ledger.Account{ledger.ReserveAddress: {Balance: ledger.SeparateReserve}, ledger.FormerAddress: {Balance: ledger.FormerReserve}, ledger.PendingAddress: {Balance: "80000000000000000000"}, ledger.FeeAddress: {Balance: "0"}}
	remaining, _ := new(big.Int).SetString(accounts[ledger.PendingAddress].Balance, 10)
	for address, value := range funding {
		if !ledger.ValidAddress(address) || len(value) > 20 || len(value) == 0 || (len(value) > 1 && value[0] == '0') {
			return nil, errors.New("invalid test allocation")
		}
		for _, c := range value {
			if c < '0' || c > '9' {
				return nil, errors.New("invalid test allocation amount")
			}
		}
		n, ok := new(big.Int).SetString(value, 10)
		if !ok || n.Sign() <= 0 {
			return nil, errors.New("test allocation must be positive")
		}
		remaining.Sub(remaining, n)
		if remaining.Sign() < 0 {
			return nil, errors.New("test allocation exceeds pending pool")
		}
		accounts[address] = ledger.Account{Balance: value}
	}
	accounts[ledger.PendingAddress] = ledger.Account{Balance: remaining.String()}
	g := ledger.Genesis{ChainID: ChainID, Decimals: 9, MaxSupply: ledger.MaxSupply, Fee: "1000", Accounts: accounts}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	state, _ := json.Marshal(g)
	vals := make([]types.GenesisValidator, 0, 4)
	nodes, keys, names, addresses := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, i := range ids {
		if err := i.Validate(); err != nil {
			return nil, err
		}
		if i.Role != "validator" || nodes[i.NodeID] || keys[string(i.PublicKey)] || names[i.Name] || addresses[i.P2PAddress] {
			return nil, errors.New("expected four distinct validator identities")
		}
		nodes[i.NodeID] = true
		keys[string(i.PublicKey)] = true
		names[i.Name] = true
		addresses[i.P2PAddress] = true
		key, _ := mldsa65.NewPubKeyFromBytes(i.PublicKey)
		vals = append(vals, types.GenesisValidator{Address: key.Address(), PubKey: key, Power: 1, Name: i.Name})
	}
	// Canonical order makes assembly independent of descriptor file ordering.
	sortValidators(vals)
	params := types.DefaultConsensusParams()
	params.Validator.PubKeyTypes = []string{mldsa65.KeyType}
	params.Block.MaxBytes = 1 << 20
	doc := &types.GenesisDoc{ChainID: ChainID, GenesisTime: when, InitialHeight: 1, ConsensusParams: params, Validators: vals, AppState: state}
	return doc, doc.ValidateAndComplete()
}
func ValidateGenesis(g *types.GenesisDoc) error {
	if g.ChainID != ChainID || g.InitialHeight != 1 || len(g.Validators) != 4 || g.ConsensusParams == nil || g.GenesisTime.IsZero() {
		return errors.New("unexpected testnet genesis")
	}
	if err := g.ValidateAndComplete(); err != nil {
		return err
	}
	if err := g.ConsensusParams.ValidateBasic(); err != nil {
		return err
	}
	if len(g.ConsensusParams.Validator.PubKeyTypes) != 1 || g.ConsensusParams.Validator.PubKeyTypes[0] != mldsa65.KeyType || g.ConsensusParams.Block.MaxBytes != 1<<20 || g.ConsensusParams.ABCI.VoteExtensionsEnableHeight != 0 {
		return errors.New("unexpected testnet consensus parameters")
	}
	seen := map[string]bool{}
	for _, v := range g.Validators {
		if v.PubKey == nil || v.PubKey.Type() != mldsa65.KeyType || v.Power != 1 || seen[string(v.PubKey.Bytes())] {
			return errors.New("invalid validator set")
		}
		seen[string(v.PubKey.Bytes())] = true
	}
	var app ledger.Genesis
	if err := ledger.Decode(g.AppState, &app); err != nil {
		return err
	}
	if app.ChainID != ChainID {
		return errors.New("ledger chain mismatch")
	}
	return app.Validate()
}
func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}
func ReadGenesis(path, expected string) (*types.GenesisDoc, []byte, error) {
	if !validDigest(expected) {
		return nil, nil, errors.New("explicit lowercase genesis SHA-256 required")
	}
	b, err := readBounded(path)
	if err != nil {
		return nil, nil, err
	}
	if len(b) > MaxJSON || Digest(b) != expected {
		return nil, nil, errors.New("genesis hash mismatch or oversized genesis")
	}
	// Do not let upstream default an omitted genesis time to each node's wall clock.
	var g types.GenesisDoc
	if err := cmtjson.Unmarshal(b, &g); err != nil {
		return nil, nil, err
	}
	if err := ValidateGenesis(&g); err != nil {
		return nil, nil, err
	}
	return &g, b, nil
}
func member(g *types.GenesisDoc, i Identity) bool {
	for _, v := range g.Validators {
		if bytes.Equal(v.PubKey.Bytes(), i.PublicKey) {
			return true
		}
	}
	return false
}
func Install(home, path, expected string) error {
	p, err := LoadProfile(home)
	if err != nil {
		return err
	}
	g, b, err := ReadGenesis(path, expected)
	if err != nil {
		return err
	}
	if p.Identity.Role == "validator" && !member(g, p.Identity) {
		return errors.New("local validator absent from genesis")
	}
	if p.GenesisSHA256 != "" && p.GenesisSHA256 != expected {
		return errors.New("refusing to change pinned genesis")
	}
	target := filepath.Join(home, "config/genesis.json")
	if old, err := os.ReadFile(target); err == nil {
		if !bytes.Equal(old, b) {
			return errors.New("existing genesis differs; refusing overwrite")
		}
	} else {
		if !os.IsNotExist(err) {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(home, "data"))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name() != "priv_validator_state.json" {
				return errors.New("cannot install genesis into existing chain data")
			}
		}
		if p.Identity.Role == "validator" {
			var state privval.FilePVLastSignState
			if err := readSignState(filepath.Join(home, "data/priv_validator_state.json"), &state); err != nil {
				return err
			}
			if state.Height != 0 || state.Round != 0 || state.Step != 0 {
				return errors.New("validator has already signed")
			}
		}
		if err := writeNew(target, b); err != nil {
			return err
		}
	}
	p.GenesisSHA256 = expected
	return replaceProfile(home, p)
}

// Observer supplies only a public transport identity to the upstream interface.
// It has no consensus secret and unconditionally refuses all signing requests.
type Observer struct{ PublicKey crypto.PubKey }

func (o Observer) GetPubKey() (crypto.PubKey, error) { return o.PublicKey, nil }
func (Observer) SignVote(string, *proto.Vote) error  { return errors.New("RPC node cannot sign votes") }
func (Observer) SignProposal(string, *proto.Proposal) error {
	return errors.New("RPC node cannot sign proposals")
}

// ValidateStart enforces the profile before opening chain DBs or starting listeners.
func ValidateStart(home string, c *cfg.Config, g *types.GenesisDoc) (types.PrivValidator, error) {
	p, err := LoadProfile(home)
	if err != nil {
		return nil, err
	}
	pinned, _, err := ReadGenesis(filepath.Join(home, "config/genesis.json"), p.GenesisSHA256)
	if err != nil {
		return nil, err
	}
	if pinned.ChainID != g.ChainID {
		return nil, errors.New("genesis mismatch")
	}
	if err := Listen(c.RPC.ListenAddress, true); err != nil {
		return nil, err
	}
	if err := Listen(c.P2P.ListenAddress, false); err != nil {
		return nil, err
	}
	peers, err := PeerString(p.Identity, p.Peers)
	if err != nil {
		return nil, err
	}
	if c.P2P.PersistentPeers != peers || c.P2P.ExternalAddress != p.Identity.P2PAddress || c.P2P.PexReactor || c.P2P.AllowDuplicateIP || c.P2P.Seeds != "" || c.P2P.MaxNumInboundPeers > 20 || c.P2P.MaxNumOutboundPeers > 10 || c.P2P.LibP2PConfig.Enabled || c.P2P.SeedMode || c.P2P.UnconditionalPeerIDs != "" || c.P2P.TestDialFail || c.P2P.TestFuzz {
		return nil, errors.New("unexpected testnet P2P settings")
	}
	if c.RPC.Unsafe || c.RPC.GRPCListenAddress != "" || c.RPC.PprofListenAddress != "" || c.RPC.TLSCertFile != "" || c.RPC.TLSKeyFile != "" || len(c.RPC.CORSAllowedOrigins) != 0 || c.PrivValidatorListenAddr != "" || c.Instrumentation.Prometheus || c.StateSync.Enable || c.Mempool.Type != cfg.MempoolTypeFlood {
		return nil, errors.New("unsupported or exposed testnet service")
	}
	if c.RPC.MaxOpenConnections < 1 || c.RPC.MaxOpenConnections > 64 || c.RPC.MaxBodyBytes < 1 || c.RPC.MaxBodyBytes > 64<<10 || c.RPC.MaxRequestBatchSize != 1 || c.RPC.MaxSubscriptionClients < 1 || c.RPC.MaxSubscriptionClients > 16 || c.RPC.MaxSubscriptionsPerClient < 1 || c.RPC.MaxSubscriptionsPerClient > 4 {
		return nil, errors.New("RPC limits exceed testnet policy")
	}
	if c.Genesis != "config/genesis.json" || c.DBPath != "data" || c.PrivValidatorKey != "config/priv_validator_key.json" || c.PrivValidatorState != "data/priv_validator_state.json" || c.NodeKey != "config/node_key.json" || c.DBBackend != "goleveldb" {
		return nil, errors.New("unexpected testnet key, database or genesis paths")
	}
	nk, err := p2p.LoadNodeKey(c.NodeKeyFile())
	if err != nil {
		return nil, err
	}
	if string(nk.ID()) != p.Identity.NodeID {
		return nil, errors.New("P2P identity mismatch")
	}
	if p.Identity.Role == "rpc" {
		for _, path := range []string{c.PrivValidatorKeyFile(), c.PrivValidatorStateFile()} {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				return nil, errors.New("RPC home must not contain consensus signing material")
			}
		}
		for _, v := range g.Validators {
			if bytes.Equal(v.Address, nk.PrivKey.PubKey().Address()) {
				return nil, errors.New("RPC identity unexpectedly belongs to validator set")
			}
		}
		return Observer{PublicKey: nk.PrivKey.PubKey()}, nil
	}
	var key privval.FilePVKey
	raw, err := os.ReadFile(c.PrivValidatorKeyFile())
	if err != nil {
		return nil, err
	}
	if err := cmtjson.Unmarshal(raw, &key); err != nil {
		return nil, errors.New("invalid validator key file")
	}
	if key.PrivKey == nil || key.PubKey == nil || key.PrivKey.Type() != mldsa65.KeyType || !bytes.Equal(key.PrivKey.PubKey().Bytes(), p.Identity.PublicKey) || !bytes.Equal(key.PubKey.Bytes(), p.Identity.PublicKey) || !bytes.Equal(key.Address, key.PubKey.Address()) || !member(g, p.Identity) {
		return nil, errors.New("validator identity does not match genesis")
	}
	var state privval.FilePVLastSignState
	if err := readSignState(c.PrivValidatorStateFile(), &state); err != nil {
		return nil, err
	}
	return privval.LoadFilePV(c.PrivValidatorKeyFile(), c.PrivValidatorStateFile()), nil
}
