#!/bin/sh
# Fails unless the Go versions have one source each:
# - .go-version holds exactly one version spec (such as 1.27.x), the
#   toolchain line of CI and release. Every actions/setup-go step in
#   .github reads it (go-version-file: .go-version) and no step sets
#   go-version itself. actions/setup-go passes the trimmed content of
#   .go-version on as version spec, ranges such as 1.27.x included.
# - go.mod keeps the minimum version for users (go 1.26.0; README: "Go
#   1.26 oder neuer"). A dependency bump must not raise it silently.
#
# Every key that looks like go-version is found, also quoted ("go-version":,
# 'go-version':), with blanks before the colon and in flow maps
# ({go-version: …}); comment lines are skipped. go-version-file counts only
# in the block style go-version-file: .go-version (quotes around the value
# allowed), as many per file as setup-go steps; go-version-file is an input
# of setup-go only. A grep error fails the check: it proves nothing.
set -eu

cd "$(dirname "$0")/.."

min_go=1.26.0

# g ARGS: grep with its output in $out; exit 1 (no line) is a result, an
# error (exit 2) stops the check
g() {
	rc=0
	out="$(grep "$@")" || rc=$?
	if [ "$rc" -gt 1 ]; then
		echo "Prüfung gescheitert (grep-Fehler $rc: grep $*)" >&2
		exit 1
	fi
}

if [ ! -f .go-version ]; then
	echo ".go-version fehlt" >&2
	exit 1
fi
g -c '' .go-version
lines="$out"
g -xE '1\.[0-9]+(\.([0-9]+|x))?' .go-version
if [ "$lines" != 1 ] || [ -z "$out" ]; then
	echo ".go-version: genau eine Zeile mit einer Go-Version erwartet (z. B. 1.27.x)" >&2
	exit 1
fi

g -E '^go[[:space:]]' go.mod
if [ "$out" != "go $min_go" ]; then
	echo "go.mod: Mindestversion „go ${min_go}“ erwartet (README, Spezifikation 12.1), gefunden: „${out:-keine go-Zeile}“; nur bewusst ändern, zusammen mit README, Spezifikation und diesem Skript" >&2
	exit 1
fi

status=0
total=0
workflows="$(find .github -type f \( -name '*.yml' -o -name '*.yaml' \))"
for f in $workflows; do
	g -nE "(^|[[:space:]{,-])[\"']?go-version[\"']?[[:space:]]*:" "$f"
	if [ -n "$out" ]; then
		rc=0
		out="$(printf '%s\n' "$out" | grep -vE '^[0-9]+:[[:space:]]*#')" || rc=$?
		if [ "$rc" -gt 1 ]; then
			echo "Prüfung gescheitert (grep-Fehler $rc)" >&2
			exit 1
		fi
	fi
	if [ -n "$out" ]; then
		echo "$f: go-version statt go-version-file: .go-version:" >&2
		printf '%s\n' "$out" >&2
		status=1
	fi
	g -cE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+actions/setup-go@' "$f"
	steps="$out"
	g -cE "^[[:space:]]*go-version-file:[[:space:]]+[\"']?\\.go-version[\"']?[[:space:]]*\$" "$f"
	if [ "$steps" != "$out" ]; then
		echo "$f: $steps setup-go-Schritte, aber $out mit go-version-file: .go-version" >&2
		status=1
	fi
	total=$((total + steps))
done
if [ "$total" -eq 0 ]; then
	echo "keine actions/setup-go-Schritte unter .github gefunden" >&2
	status=1
fi
if [ "$status" -ne 0 ]; then
	exit 1
fi
echo "ok: Go-Version für CI und Release nur aus .go-version ($(cat .go-version)), Mindestversion go $min_go in go.mod"
