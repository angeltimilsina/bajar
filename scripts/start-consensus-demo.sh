#!/usr/bin/env bash
set -euo pipefail

if ! command -v cometbft >/dev/null 2>&1; then
  echo "CometBFT not found. Install v0.40.0 first:"
  echo "  go install github.com/cometbft/cometbft/cmd/cometbft@v0.40.0"
  exit 1
fi

export CMTHOME="${CMTHOME:-$HOME/.bajar-cometbft-demo}"
if [[ ! -f "$CMTHOME/config/genesis.json" ]]; then
  mkdir -p "$CMTHOME"
  cometbft init
fi

echo "Running vanilla CometBFT kvstore DEMO, not a BAJAR chain."
echo "RPC: http://localhost:26657/status"
echo "Validator signature keys are DEFAULT (NOT post-quantum in this demo)."
cometbft node --proxy_app=kvstore
