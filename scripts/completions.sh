#!/bin/sh
# Writes the shell completion scripts into completions/ at the repository
# root (GoReleaser before hook, spec 12.2). Builds the CLI once into a
# temporary directory and runs only the local "completion" command, which
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

# The scripts complete the short form pk as well (the completion command
# adds it, internal/cli/completion.go); scripts/check-completions.sh tests
# them in the shells.
rm -rf completions
mkdir completions
"$tmp/papierkram" completion bash >completions/papierkram.bash
"$tmp/papierkram" completion zsh >completions/_papierkram
"$tmp/papierkram" completion fish >completions/papierkram.fish
"$tmp/papierkram" completion powershell >completions/papierkram.ps1
