#!/bin/sh
# Fails unless the Go toolchain line of CI and release has one source:
# .go-version holds exactly one version spec (such as 1.27.x), every
# actions/setup-go step in .github reads it (go-version-file: .go-version)
# and no step sets go-version itself. actions/setup-go passes the trimmed
# content of .go-version on as version spec, ranges such as 1.27.x
# included. go.mod keeps the minimum version (go 1.26.0) separately.
#
# The count is per file: as many go-version-file lines naming .go-version
# as setup-go steps. go-version-file is an input of setup-go only.
set -eu

cd "$(dirname "$0")/.."

if [ ! -f .go-version ]; then
	echo ".go-version fehlt" >&2
	exit 1
fi
if [ "$(grep -c '' .go-version)" != 1 ] || ! grep -qxE '1\.[0-9]+(\.([0-9]+|x))?' .go-version; then
	echo ".go-version: genau eine Zeile mit einer Go-Version erwartet (z. B. 1.27.x)" >&2
	exit 1
fi

status=0
for f in $(find .github -type f \( -name '*.yml' -o -name '*.yaml' \) | sort); do
	own="$(grep -nE '^[[:space:]]*(-[[:space:]]+)?go-version[[:space:]]*:' "$f" || true)"
	if [ -n "$own" ]; then
		echo "$f: go-version statt go-version-file: .go-version:" >&2
		printf '%s\n' "$own" >&2
		status=1
	fi
	steps="$(grep -cE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+actions/setup-go@' "$f" || true)"
	files="$(grep -cE "^[[:space:]]*go-version-file:[[:space:]]+[\"']?\\.go-version[\"']?[[:space:]]*\$" "$f" || true)"
	if [ "$steps" != "$files" ]; then
		echo "$f: $steps setup-go-Schritte, aber $files mit go-version-file: .go-version" >&2
		status=1
	fi
done
if [ "$status" -ne 0 ]; then
	exit 1
fi
echo "ok: Go-Version für CI und Release nur aus .go-version ($(cat .go-version))"
