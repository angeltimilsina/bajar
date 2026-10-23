package main

import (
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/types"
)

func marshalGenesis(g *types.GenesisDoc) ([]byte, error) { return cmtjson.MarshalIndent(g, "", "  ") }
