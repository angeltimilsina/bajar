package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"bajar-network-starter/internal/bootstrap"
)

func validateEndpoint(endpoint, profile string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("invalid RPC URL")
	}
	if profile == "local" {
		if u.Scheme != "http" || !net.ParseIP(u.Hostname()).IsLoopback() {
			return errors.New("local RPC endpoint required")
		}
		return nil
	}
	if profile != "testnet" {
		return errors.New("profile must be local or testnet")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback()) {
		return errors.New("testnet RPC requires HTTPS or loopback HTTP")
	}
	return nil
}
func bootstrapCommand(command string, args []string) (bool, error) {
	switch command {
	case "init-testnet", "assemble-genesis", "install-genesis", "configure-testnet", "check-testnet":
	default:
		return false, nil
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	home := f.String("home", "", "new per-host home")
	role := f.String("role", "validator", "validator or rpc")
	chain := f.String("chain-id", bootstrap.ChainID, "explicit testnet ID; v2 is bajar-testnet-2")
	name := f.String("name", "", "unique node name")
	advertise := f.String("advertise", "", "P2P host:port, reachable by peers")
	p2p := f.String("p2p-listen", "tcp://0.0.0.0:26656", "P2P bind address")
	rpc := f.String("rpc-listen", "tcp://127.0.0.1:26657", "private loopback RPC bind address")
	identities := f.String("identities", "", "JSON array of four public validator identities")
	peers := f.String("peers", "", "JSON array of approved peer identities, excluding self")
	when := f.String("genesis-time", "", "explicit UTC RFC3339 genesis time")
	funding := f.String("test-allocations", "", "optional explicit test-only address/base-unit map")
	genesis := f.String("genesis", "", "genesis file to install")
	digest := f.String("sha256", "", "out-of-band SHA-256 of genesis bytes")
	out := f.String("out", "", "new assembled genesis file")
	if err := f.Parse(args); err != nil {
		return true, err
	}
	if f.NArg() != 0 {
		return true, errors.New("unexpected positional arguments")
	}
	if command != "assemble-genesis" && *home == "" {
		return true, errors.New("explicit --home required")
	}
	switch command {
	case "init-testnet":
		i, err := bootstrap.Init(bootstrap.InitOptions{Home: *home, Role: *role, Name: *name, Advertise: *advertise, P2PListen: *p2p, RPCListen: *rpc, ChainID: *chain})
		if err != nil {
			return true, err
		}
		b, err := json.MarshalIndent(i, "", "  ")
		if err != nil {
			return true, err
		}
		fmt.Println(string(b))
		return true, nil
	case "assemble-genesis":
		if *out == "" || !strings.HasSuffix(*when, "Z") {
			return true, errors.New("--out and explicit --genesis-time ending in Z required")
		}
		t, err := time.Parse(time.RFC3339, *when)
		if err != nil {
			return true, err
		}
		var ids []bootstrap.Identity
		if err := bootstrap.ReadJSON(*identities, &ids); err != nil {
			return true, err
		}
		var allocations map[string]string
		if *funding != "" {
			if err := bootstrap.ReadJSON(*funding, &allocations); err != nil {
				return true, err
			}
		}
		g, err := bootstrap.Assemble(ids, t, allocations)
		if err != nil {
			return true, err
		}
		// Marshal once; the digest pins exactly the bytes distributed to every host.
		b, err := marshalGenesis(g)
		if err != nil {
			return true, err
		}
		if len(b) > bootstrap.MaxJSON {
			return true, errors.New("genesis too large")
		}
		if err := writeNew(*out, b); err != nil {
			return true, err
		}
		fmt.Println(bootstrap.Digest(b))
		return true, nil
	case "install-genesis":
		return true, bootstrap.Install(*home, *genesis, *digest)
	case "configure-testnet":
		var ids []bootstrap.Identity
		if err := bootstrap.ReadJSON(*peers, &ids); err != nil {
			return true, err
		}
		return true, bootstrap.Configure(*home, ids)
	case "check-testnet":
		c, err := bootstrap.LoadConfig(*home)
		if err != nil {
			return true, err
		}
		p, err := bootstrap.LoadProfile(*home)
		if err != nil {
			return true, err
		}
		g, _, err := bootstrap.ReadGenesis(c.GenesisFile(), p.GenesisSHA256)
		if err != nil {
			return true, err
		}
		if _, err := bootstrap.ValidateStart(*home, c, g); err != nil {
			return true, err
		}
		fmt.Println("PASS: pinned bajar-testnet-1 genesis, identity, role and private RPC policy")
		return true, nil
	}
	return true, os.ErrInvalid
}
