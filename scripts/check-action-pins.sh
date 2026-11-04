#!/bin/sh
# Fails if a GitHub Actions "uses:" in .github (workflows, local actions,
# *.yml and *.yaml) is not pinned to a full commit SHA with the release tag
# as comment:
#
#   - uses: owner/repo@<40 hex> # vX.Y.Z
#
# Only this block style is accepted; the flow style ({uses: …}) is
# rejected. Local actions (./…) and docker:// references fail as well
# (none exist; allowing one needs a change here). .github/dependabot.yml
# keeps the pins current.
set -eu

cd "$(dirname "$0")/.."

pinned='^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+[^@[:space:]#]+@[0-9a-f]{40}[[:space:]]+#[[:space:]]+v[0-9]+(\.[0-9]+)*$'
bad="$(find .github -type f \( -name '*.yml' -o -name '*.yaml' \) -exec grep -nHE '(^|[[:space:]{,-])uses:' {} + |
	grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' |
	grep -vE "^[^:]+:[0-9]+:${pinned#^}" || true)"
if [ -n "$bad" ]; then
	echo "nicht per Commit-SHA gepinnt (erwartet: uses: owner/repo@<40 hex> # vX.Y.Z):" >&2
	printf '%s\n' "$bad" >&2
	exit 1
fi
echo "ok: alle Actions per Commit-SHA gepinnt"
