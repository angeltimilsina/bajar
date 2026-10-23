// bajard is a development-only embedded CometBFT node and BAJ transfer CLI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"bajar-network-starter/internal/bootstrap"
	"bajar-network-starter/internal/ledger"
	dbm "github.com/cometbft/cometbft-db"
	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/mldsa65"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	rpc "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cometbft/cometbft/types"
	"github.com/spf13/viper"
)

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
func wallet(path string) (mldsa65.PrivKey, error) {
	k, err := mldsa65.GenPrivKey()
	if err != nil {
		return k, err
	}
	b, _ := json.Marshal(k.Bytes())
	return k, writeNew(path, b)
}
func readKey(path string) (mldsa65.PrivKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return mldsa65.PrivKey{}, err
	}
	var raw []byte
	if err := ledger.Decode(b, &raw); err != nil {
		return mldsa65.PrivKey{}, err
	}
	return mldsa65.NewPrivKeyFromBytes(raw)
}
func initDevnet(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		return errors.New("devnet path must not exist; existing keys and signing state are never overwritten")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(root), ".bajar-init-")
	if err != nil {
		return err
	}
	// Preserve partial initialization for inspection on failure; never remove key directories.
	if err := os.Mkdir(filepath.Join(staging, "wallets"), 0700); err != nil {
		return err
	}
	alice, err := wallet(filepath.Join(staging, "wallets/alice.key"))
	if err != nil {
		return err
	}
	bob, err := wallet(filepath.Join(staging, "wallets/bob.key"))
	if err != nil {
		return err
	}
	addresses := map[string]string{"alice": ledger.Address(alice.PubKey().Bytes()), "bob": ledger.Address(bob.PubKey().Bytes())}
	b, _ := json.MarshalIndent(addresses, "", "  ")
	if err := writeNew(filepath.Join(staging, "wallets/addresses.json"), b); err != nil {
		return err
	}
	chainID := fmt.Sprintf("bajar-ledger-local-%d", time.Now().UnixNano())
	pending, _ := new(big.Int).SetString("80000000000000000000", 10)
	pending.Sub(pending, big.NewInt(1000_000_000_000))
	g := ledger.Genesis{ChainID: chainID, Decimals: 9, MaxSupply: ledger.MaxSupply, Fee: "1000", Accounts: map[string]ledger.Account{
		ledger.ReserveAddress: {Balance: ledger.SeparateReserve}, ledger.FormerAddress: {Balance: ledger.FormerReserve},
		ledger.PendingAddress: {Balance: pending.String()}, ledger.FeeAddress: {Balance: "0"},
		addresses["alice"]: {Balance: "1000000000000"}, addresses["bob"]: {Balance: "0"},
	}}
	if err := g.Validate(); err != nil {
		return err
	}
	appState, _ := json.Marshal(g)
	vals := make([]types.GenesisValidator, 4)
	ids := make([]p2p.ID, 4)
	for i := 0; i < 4; i++ {
		home := filepath.Join(staging, fmt.Sprintf("nodes/node%d", i))
		c := cfg.DefaultConfig()
		c.SetRoot(home)
		cfg.EnsureRoot(home)
		k, err := mldsa65.GenPrivKey()
		if err != nil {
			return err
		}
		pv := privval.NewFilePV(k, c.PrivValidatorKeyFile(), c.PrivValidatorStateFile())
		pv.Save()
		nk, err := p2p.LoadOrGenNodeKey(c.NodeKeyFile())
		if err != nil {
			return err
		}
		ids[i] = nk.ID()
		vals[i] = types.GenesisValidator{Address: k.PubKey().Address(), PubKey: k.PubKey(), Power: 1, Name: fmt.Sprintf("local-%d", i)}
	}
	params := types.DefaultConsensusParams()
	params.Validator.PubKeyTypes = []string{mldsa65.KeyType}
	genesis := &types.GenesisDoc{ChainID: chainID, GenesisTime: time.Now().UTC(), InitialHeight: 1, ConsensusParams: params, Validators: vals, AppState: appState}
	if err := genesis.ValidateAndComplete(); err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		home := filepath.Join(staging, fmt.Sprintf("nodes/node%d", i))
		c := cfg.DefaultConfig()
		c.SetRoot(filepath.Join(root, fmt.Sprintf("nodes/node%d", i)))
		c.Moniker = fmt.Sprintf("bajar-ledger-%d", i)
		c.ProxyApp = "bajar-embedded"
		c.RPC.ListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", 27657+i*10)
		c.RPC.PprofListenAddress = ""
		c.P2P.ListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", 27656+i*10)
		c.P2P.AddrBookStrict = false
		c.P2P.AllowDuplicateIP = true
		c.P2P.PexReactor = false
		var peers []string
		for j := 0; j < 4; j++ {
			if i != j {
				peers = append(peers, fmt.Sprintf("%s@127.0.0.1:%d", ids[j], 27656+j*10))
			}
		}
		c.P2P.PersistentPeers = strings.Join(peers, ",")
		c.Consensus.TimeoutCommit = time.Second
		c.Instrumentation.Prometheus = false
		if err := c.ValidateBasic(); err != nil {
			return err
		}
		cfg.WriteConfigFile(filepath.Join(home, "config/config.toml"), c)
		if err := genesis.SaveAs(filepath.Join(home, "config/genesis.json")); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, root); err != nil {
		return err
	}
	fmt.Println("Initialized four ML-DSA-65 local validators and disposable wallets at", root)
	fmt.Println("Alice has 1,000 test BAJ. All other allocations remain locked; these are not production beneficiaries.")
	return nil
}
func start(home string) error { return startProfile(home, "local") }
func startProfile(home string, profile string) error {
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	c := cfg.DefaultConfig()
	c.SetRoot(home)
	v := viper.New()
	v.SetConfigFile(filepath.Join(home, "config/config.toml"))
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	if err := v.Unmarshal(c); err != nil {
		return err
	}
	c.SetRoot(home)
	if err := c.ValidateBasic(); err != nil {
		return err
	}
	g, err := types.GenesisDocFromFile(c.GenesisFile())
	if err != nil {
		return err
	}
	var pv types.PrivValidator
	switch profile {
	case "local":
		if !strings.HasPrefix(g.ChainID, "bajar-ledger-local-") || bootstrap.Listen(c.RPC.ListenAddress, true) != nil || bootstrap.Listen(c.P2P.ListenAddress, true) != nil {
			return errors.New("local profile requires a local chain ID and loopback listeners")
		}
		for _, path := range []string{c.PrivValidatorKeyFile(), c.PrivValidatorStateFile(), c.NodeKeyFile()} {
			if _, err := os.Stat(path); err != nil {
				return err
			}
		}
		localPV := privval.LoadFilePV(c.PrivValidatorKeyFile(), c.PrivValidatorStateFile())
		if localPV.Key.PubKey.Type() != mldsa65.KeyType {
			return errors.New("ML-DSA-65 validator key required")
		}
		pv = localPV
	case "testnet":
		pv, err = bootstrap.ValidateStart(home, c, g)
		if err != nil {
			return err
		}
	default:
		return errors.New("profile must be local or testnet")
	}
	nk, err := p2p.LoadNodeKey(c.NodeKeyFile())
	if err != nil {
		return err
	}
	db, err := dbm.NewGoLevelDB("bajar", c.DBDir())
	if err != nil {
		return err
	}
	defer db.Close()
	app, err := ledger.New(db)
	if err != nil {
		return err
	}
	logger := log.NewFilter(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), log.AllowInfo())
	n, err := node.NewNode(c, pv, nk, proxy.NewLocalClientCreator(app), node.DefaultGenesisDocProviderFunc(c), cfg.DefaultDBProvider, node.DefaultMetricsProvider(c.Instrumentation), logger)
	if err != nil {
		return err
	}
	if err := n.Start(); err != nil {
		return err
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals
	err = n.Stop()
	n.Wait()
	return err
}
func printJSON(v any) error {
	b, err := cmtjson.MarshalIndent(v, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
	return err
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: bajard init-devnet|init-testnet|assemble-genesis|install-genesis|configure-testnet|check-testnet|start|wallet-new|address|transfer|query|verify-commit [flags]")
	}
	command := os.Args[1]
	if handled, err := bootstrapCommand(command, os.Args[2:]); handled {
		return err
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	home := f.String("home", ".ledger-devnet/nodes/node0", "node home")
	dir := f.String("dir", ".ledger-devnet", "new local devnet directory")
	key := f.String("key", "", "wallet .key file (never a validator key)")
	endpoint := f.String("rpc", "http://127.0.0.1:27657", "local RPC")
	to := f.String("to", "", "recipient address")
	value := f.String("amount", "", "amount in integer base units (1 BAJ = 1000000000)")
	path := f.String("path", "/supply", "query path: /account, /supply, /state")
	address := f.String("address", "", "account to query")
	profile := f.String("profile", "local", "local or explicit testnet profile")
	trustedGenesis := f.String("genesis", "", "trusted testnet genesis for commit verification")
	genesisHash := f.String("sha256", "", "out-of-band trusted genesis SHA-256")
	if err := f.Parse(os.Args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch command {
	case "init-devnet":
		return initDevnet(*dir)
	case "start":
		return startProfile(*home, *profile)
	case "wallet-new":
		k, err := wallet(*key)
		if err != nil {
			return err
		}
		fmt.Println(ledger.Address(k.PubKey().Bytes()))
		return nil
	case "address":
		k, err := readKey(*key)
		if err != nil {
			return err
		}
		fmt.Println(ledger.Address(k.PubKey().Bytes()))
		return nil
	}
	if err := validateEndpoint(*endpoint, *profile); err != nil {
		return err
	}
	client, err := rpc.New(*endpoint, "/websocket")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *profile == "testnet" {
		supply, err := client.ABCIQuery(ctx, "/supply", nil)
		if err != nil {
			return err
		}
		var info struct {
			ChainID string `json:"chain_id"`
		}
		if err := json.Unmarshal(supply.Response.Value, &info); err != nil || supply.Response.Code != 0 || info.ChainID != bootstrap.ChainID {
			return errors.New("RPC is not serving bajar-testnet-1")
		}
	}
	switch command {
	case "query":
		r, err := client.ABCIQuery(ctx, *path, []byte(*address))
		if err != nil {
			return err
		}
		if r.Response.Code != 0 {
			return errors.New(r.Response.Log)
		}
		fmt.Println(string(r.Response.Value))
		return nil
	case "transfer":
		k, err := readKey(*key)
		if err != nil {
			return err
		}
		supply, err := client.ABCIQuery(ctx, "/supply", nil)
		if err != nil {
			return err
		}
		if supply.Response.Code != 0 {
			return errors.New(supply.Response.Log)
		}
		var info struct {
			ChainID string `json:"chain_id"`
			Fee     string `json:"fee"`
		}
		if err := json.Unmarshal(supply.Response.Value, &info); err != nil {
			return err
		}
		account, err := client.ABCIQuery(ctx, "/account", []byte(ledger.Address(k.PubKey().Bytes())))
		if err != nil {
			return err
		}
		if account.Response.Code != 0 {
			return errors.New(account.Response.Log)
		}
		var a ledger.Account
		if err := json.Unmarshal(account.Response.Value, &a); err != nil {
			return err
		}
		tx, err := ledger.Sign(k, ledger.UnsignedTx{Version: 1, ChainID: info.ChainID, To: *to, Amount: *value, Fee: info.Fee, Nonce: a.Nonce + 1})
		if err != nil {
			return err
		}
		r, err := client.BroadcastTxCommit(ctx, tx)
		if err != nil {
			return err
		}
		if r.CheckTx.Code != 0 {
			return errors.New(r.CheckTx.Log)
		}
		if r.TxResult.Code != 0 {
			return errors.New(r.TxResult.Log)
		}
		return printJSON(r)
	case "verify-commit":
		r, err := client.Commit(ctx, nil)
		if err != nil {
			return err
		}
		if err := r.SignedHeader.ValidateBasic(r.ChainID); err != nil {
			return err
		}
		h := r.Height
		page, perPage := 1, 100
		vs, err := client.Validators(ctx, &h, &page, &perPage)
		if err != nil {
			return err
		}
		if len(vs.Validators) != vs.Total {
			return errors.New("validator pagination unsupported")
		}
		for _, v := range vs.Validators {
			if v.PubKey.Type() != mldsa65.KeyType {
				return errors.New("non ML-DSA-65 validator")
			}
		}
		set := types.NewValidatorSet(vs.Validators)
		if *profile == "testnet" {
			genesis, _, err := bootstrap.ReadGenesis(*trustedGenesis, *genesisHash)
			if err != nil {
				return err
			}
			trusted := make([]*types.Validator, 0, len(genesis.Validators))
			for _, v := range genesis.Validators {
				trusted = append(trusted, types.NewValidator(v.PubKey, v.Power))
			}
			set = types.NewValidatorSet(trusted)
			if r.ChainID != bootstrap.ChainID {
				return errors.New("commit chain mismatch")
			}
		}
		if err := set.VerifyCommit(r.ChainID, r.Commit.BlockID, h, r.Commit); err != nil {
			return err
		}
		fmt.Printf("PASS: verified ML-DSA-65 commit at height %d with %d validators\n", h, len(vs.Validators))
		return nil
	default:
		return errors.New("unknown command: " + command)
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
