#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .tools/bin
go mod download
go build -o .tools/bin/bajard ./cmd/bajard
