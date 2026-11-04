#!/bin/sh
# Writes the man pages (section 1, German) into man/ at the repository root
# (GoReleaser before hook, spec 12.2/12.3). Builds the CLI once into a
# temporary directory and runs the hidden, local "gen-docs --man", which
# sends no request. Two runs give identical files.
set -eu

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

go build -o "$tmp/papierkram" ./cmd/papierkram

# an empty configuration directory: nothing from the local setup may leak
# into the release files
PAPIERKRAM_CONFIG_DIR="$tmp/config"
export PAPIERKRAM_CONFIG_DIR

rm -rf man
"$tmp/papierkram" gen-docs --man man >/dev/null
