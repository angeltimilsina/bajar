package bootstrap

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bajar-network-starter/internal/ledger"
	cfg "github.com/cometbft/cometbft/config"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	proto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/types"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) ([]string, []Identity, *types.GenesisDoc, string, string) {
	t.Helper()
	root := t.TempDir()
	var homes []string
	var ids []Identity
	for n, name := range []string{"v0", "v1", "v2", "v3", "rpc"} {
		role := "validator"
		if n == 4 {
			role = "rpc"
		}
		home := filepath.Join(root, name)
		id, err := Init(InitOptions{Home: home, Role: role, Name: name, Advertise: name + ".example:26656", P2PListen: "tcp://0.0.0.0:26656", RPCListen: "tcp://127.0.0.1:26657"})
		must(t, err)
		homes = append(homes, home)
		ids = append(ids, id)
	}
	g, err := Assemble(ids[:4], time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), nil)
	must(t, err)
	b, err := cmtjson.MarshalIndent(g, "", "  ")
	must(t, err)
	path := filepath.Join(root, "genesis.json")
	must(t, os.WriteFile(path, b, 0600))
	hash := Digest(b)
	for n, home := range homes {
		must(t, Install(home, path, hash))
		var peers []Identity
		for k, id := range ids {
			if k != n {
				peers = append(peers, id)
			}
		}
		must(t, Configure(home, peers))
	}
	return homes, ids, g, path, hash
}
func TestIndependentInitializationAndObserver(t *testing.T) {
	homes, ids, g, _, _ := fixture(t)
	keys := map[string]bool{}
	for n, home := range homes {
		c, err := LoadConfig(home)
		must(t, err)
		signer, err := ValidateStart(home, c, g)
		must(t, err)
		if n < 4 {
			if keys[string(ids[n].PublicKey)] {
				t.Fatal("validator keys reused")
			}
			keys[string(ids[n].PublicKey)] = true
		} else {
			for _, p := range []string{c.PrivValidatorKeyFile(), c.PrivValidatorStateFile()} {
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Fatal("observer has consensus secret/state")
				}
			}
			if signer.SignVote(ChainID, &proto.Vote{}) == nil || signer.SignProposal(ChainID, &proto.Proposal{}) == nil {
				t.Fatal("observer signed")
			}
		}
		if _, err := Init(InitOptions{Home: home, Role: "validator"}); err == nil {
			t.Fatal("overwrote node home")
		}
	}
}
func TestAssemblySupplyAndDeterminism(t *testing.T) {
	_, ids, g, _, _ := fixture(t)
	reversed := []Identity{ids[3], ids[2], ids[1], ids[0]}
	other, err := Assemble(reversed, g.GenesisTime, nil)
	must(t, err)
	a, _ := cmtjson.Marshal(g)
	b, _ := cmtjson.Marshal(other)
	if !bytes.Equal(a, b) {
		t.Fatal("assembly depends on input order")
	}
	var state ledger.Genesis
	must(t, json.Unmarshal(g.AppState, &state))
	must(t, state.Validate())
	if state.Accounts[ledger.PendingAddress].Balance != "80000000000000000000" {
		t.Fatal("default genesis assigned beneficiaries")
	}
	bad := append([]Identity(nil), ids[:4]...)
	bad[3] = bad[0]
	if _, err := Assemble(bad, g.GenesisTime, nil); err == nil {
		t.Fatal("duplicate validator")
	}
	bad = append([]Identity(nil), ids[:4]...)
	bad[3] = ids[4]
	if _, err := Assemble(bad, g.GenesisTime, nil); err == nil {
		t.Fatal("RPC in validator set")
	}
	for _, funding := range []map[string]string{{ledger.ReserveAddress: "1"}, {ledger.Address([]byte("test")): "80000000000000000001"}, {ledger.Address([]byte("test")): "-1"}} {
		if _, err := Assemble(ids[:4], g.GenesisTime, funding); err == nil {
			t.Fatal("invalid test allocations")
		}
	}
}
func TestGenesisPinAndInstallBoundaries(t *testing.T) {
	homes, _, g, path, hash := fixture(t)
	missingTime := *g
	missingTime.GenesisTime = time.Time{}
	badBytes, err := cmtjson.Marshal(missingTime)
	must(t, err)
	missingPath := filepath.Join(t.TempDir(), "genesis.json")
	must(t, os.WriteFile(missingPath, badBytes, 0600))
	if _, _, err := ReadGenesis(missingPath, Digest(badBytes)); err == nil {
		t.Fatal("allowed wall-clock defaulting of genesis time")
	}
	must(t, Install(homes[0], path, hash))
	if err := Install(homes[0], path, "bad"); err == nil {
		t.Fatal("accepted unverified genesis")
	}
	original, err := os.ReadFile(path)
	must(t, err)
	must(t, os.WriteFile(path, append(original, '\n'), 0600))
	if err := Install(homes[0], path, hash); err == nil {
		t.Fatal("hash mismatch")
	}
	if err := Install(homes[0], path, Digest(append(original, '\n'))); err == nil {
		t.Fatal("replaced pinned genesis")
	}
	c, err := LoadConfig(homes[0])
	must(t, err)
	altered := *g
	altered.GenesisTime = g.GenesisTime.Add(time.Second)
	if _, err := ValidateStart(homes[0], c, &altered); err == nil {
		t.Fatal("startup accepted a different in-memory genesis with the same chain ID")
	}
	must(t, os.WriteFile(c.GenesisFile(), append(original, '\n'), 0600))
	if _, err := ValidateStart(homes[0], c, g); err == nil {
		t.Fatal("startup ignored changed genesis")
	}
}
func TestTestnetPolicyRejectsExposedServices(t *testing.T) {
	homes, _, g, _, _ := fixture(t)
	cases := map[string]func(*cfg.Config){
		"public-rpc": func(c *cfg.Config) { c.RPC.ListenAddress = "tcp://0.0.0.0:26657" },
		"unsafe":     func(c *cfg.Config) { c.RPC.Unsafe = true }, "pprof": func(c *cfg.Config) { c.RPC.PprofListenAddress = ":6060" },
		"grpc": func(c *cfg.Config) { c.RPC.GRPCListenAddress = ":9090" }, "remote-signer": func(c *cfg.Config) { c.PrivValidatorListenAddr = ":1234" },
		"prometheus": func(c *cfg.Config) { c.Instrumentation.Prometheus = true }, "cors": func(c *cfg.Config) { c.RPC.CORSAllowedOrigins = []string{"*"} },
		"unlimited-rpc": func(c *cfg.Config) { c.RPC.MaxOpenConnections = 0 }, "request-batch": func(c *cfg.Config) { c.RPC.MaxRequestBatchSize = 100 },
		"redirected-genesis": func(c *cfg.Config) { c.Genesis = "other.json" }, "pex": func(c *cfg.Config) { c.P2P.PexReactor = true },
		"unapproved-peers": func(c *cfg.Config) { c.P2P.PersistentPeers = "" }, "state-sync": func(c *cfg.Config) { c.StateSync.Enable = true },
		"experimental-transport": func(c *cfg.Config) { c.P2P.LibP2PConfig.Enabled = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c, err := LoadConfig(homes[0])
			must(t, err)
			mutate(c)
			if _, err := ValidateStart(homes[0], c, g); err == nil {
				t.Fatal("unsafe profile accepted")
			}
		})
	}
	// Importing validator material into an RPC home must fail, even if not in genesis.
	c, err := LoadConfig(homes[4])
	must(t, err)
	must(t, os.WriteFile(c.PrivValidatorKeyFile(), []byte("{}"), 0600))
	if _, err := ValidateStart(homes[4], c, g); err == nil {
		t.Fatal("RPC accepted consensus secret")
	}
}
func TestIdentityAndPeerValidation(t *testing.T) {
	_, ids, _, _, _ := fixture(t)
	if _, err := PeerString(ids[0], []Identity{ids[0]}); err == nil {
		t.Fatal("self peer")
	}
	if _, err := PeerString(ids[0], []Identity{ids[1], ids[1]}); err == nil {
		t.Fatal("duplicate peer")
	}
	bad := ids[0]
	bad.P2PAddress = "0.0.0.0:26656"
	if bad.Validate() == nil {
		t.Fatal("wildcard advertisement")
	}
	bad = ids[0]
	bad.ChainID = StakingChainID
	if _, err := PeerString(ids[1], []Identity{bad}); err == nil {
		t.Fatal("mixed protocol peers")
	}
	bad = ids[0]
	bad.Name = "escape\"\n"
	if bad.Validate() == nil {
		t.Fatal("unsafe config string")
	}
	if Listen("tcp://localhost:26657", true) == nil || Listen("tcp://0.0.0.0:26657", true) == nil {
		t.Fatal("RPC listener policy")
	}
}
