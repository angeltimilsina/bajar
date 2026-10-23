package bootstrap

import (
	"path/filepath"
	"sort"

	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/types"
	"github.com/spf13/viper"
)

func readConfig(home string, c *cfg.Config) (*cfg.Config, error) {
	v := viper.New()
	v.SetConfigFile(filepath.Join(home, "config/config.toml"))
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := v.Unmarshal(c); err != nil {
		return nil, err
	}
	c.SetRoot(home)
	return c, c.ValidateBasic()
}
func sortValidators(vals []types.GenesisValidator) {
	sort.Slice(vals, func(i, j int) bool { return vals[i].Address.String() < vals[j].Address.String() })
}
