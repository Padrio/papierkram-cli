#!/bin/sh
# Writes the man pages (section 1, German) into man/ at the repository root,
# gzipped copies into man-gz/
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

rm -rf man man-gz
"$tmp/papierkram" gen-docs --man man >/dev/null

# gzipped copies for deb/rpm (man-gz/, also in .gitignore); -n leaves out
# name and time, so the files depend only on the pages (and on the gzip
# implementation)
mkdir man-gz
for page in man/*.1; do
	gzip -9 -n -c "$page" >"man-gz/${page#man/}.gz"
done
