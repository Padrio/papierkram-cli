#!/usr/bin/env bash
# Usage: scripts/coverage.sh <threshold-percent>
# Runs all tests with a coverage profile (excluding generated code, tools and
# e2e packages), prints the total and exits non-zero below the threshold.
set -euo pipefail

threshold="${1:?usage: scripts/coverage.sh <threshold-percent>}"
if ! [[ "$threshold" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "invalid threshold: $threshold" >&2
  exit 2
fi

cd "$(dirname "$0")/.."

# Packages that do not exist yet simply do not show up in the list.
packages="$(go list ./... | grep -v -E '/internal/api/gen($|/)|/tools($|/)|/e2e($|/)' || true)"
if [ -z "$packages" ]; then
  echo "no packages to measure" >&2
  exit 1
fi

profile="coverage.out"
# shellcheck disable=SC2086
go test -coverprofile="$profile" $packages

total="$(go tool cover -func="$profile" | awk '/^total:/ {gsub("%", "", $NF); print $NF}')"
echo "total coverage: ${total}% (threshold: ${threshold}%)"

if awk -v t="$total" -v th="$threshold" 'BEGIN { exit !(t + 0 < th + 0) }'; then
  echo "coverage below threshold" >&2
  exit 1
fi
