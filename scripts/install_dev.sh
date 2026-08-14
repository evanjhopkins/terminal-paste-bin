#!/usr/bin/env bash

set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
	echo "install_dev.sh is intended for macOS." >&2
	exit 1
fi

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="$(go env GOBIN)"

if [[ -z "$go_bin" ]]; then
	go_bin="$(go env GOPATH)/bin"
fi

mkdir -p "$go_bin"

(
	cd "$repo_root"
	go build -o "$go_bin/tpbd" ./cmd/tpb
)

echo "Installed development build: $go_bin/tpbd"
