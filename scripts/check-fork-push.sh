#!/bin/sh
# Checks before a release that PACKAGES_GITHUB_TOKEN can write to a
# repository GoReleaser publishes to (Padrio/homebrew-tap, the winget fork
# Padrio/winget-pkgs), so a missing permission never leaves half a release
# (.github/workflows/release.yml, docs/release.md). A real write test: it
# creates a temporary branch on the head of the default branch, the same
# calls GoReleaser makes for the winget branch (GET the repository, GET
# its default branch ref, POST a ref), and deletes it again.
#
# Usage: scripts/check-fork-push.sh <owner/repo>
#
# The token comes from PACKAGES_GITHUB_TOKEN and reaches gh as GH_TOKEN in
# its environment, never in argv or in the output. CHECK_FORK_PUSH_GH
# replaces the gh command (self-test in .github/workflows/ci.yml, without
# network).
#
# Exit 0: the token can write (a failed cleanup is only a warning: the
# write itself worked, the leftover branch is harmless and named). Exit 1:
# token missing, repository not readable or not writable. Exit 2: usage.
set -eu

# owner/repo on one line, nothing else
nl='
'
valid=false
if [ $# -eq 1 ]; then
	case "$1" in
	*"$nl"*) ;;
	*) if printf '%s\n' "$1" | grep -Eqx '[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+'; then valid=true; fi ;;
	esac
fi
if [ "$valid" != true ]; then
	echo "Verwendung: scripts/check-fork-push.sh <owner/repo>" >&2
	exit 2
fi
repo="$1"
gh="${CHECK_FORK_PUSH_GH:-gh}"

fail() {
	echo "check-fork-push.sh: $1" >&2
	echo "PACKAGES_GITHUB_TOKEN kann $repo nicht beschreiben (docs/release.md, Voraussetzungen)" >&2
	exit 1
}

if [ -z "${PACKAGES_GITHUB_TOKEN:-}" ]; then
	fail "PACKAGES_GITHUB_TOKEN fehlt oder ist leer"
fi

# gh with the token under test only: GH_TOKEN wins over GITHUB_TOKEN and a
# stored login, GH_HOST pins github.com; nothing is prompted
api() {
	GH_TOKEN="$PACKAGES_GITHUB_TOKEN" GH_HOST=github.com GH_PROMPT_DISABLED=1 "$gh" api "$@"
}

default_branch="$(api "repos/$repo" --jq .default_branch)" || fail "$repo nicht lesbar"
if [ -z "$default_branch" ]; then
	fail "$repo nennt keinen Standard-Branch"
fi
sha="$(api "repos/$repo/git/ref/heads/$default_branch" --jq .object.sha)" || fail "Branch $default_branch von $repo nicht lesbar"
if ! printf '%s\n' "$sha" | grep -Eqx '[0-9a-f]{40}'; then
	fail "Branch $default_branch von $repo ohne Commit-SHA"
fi

# one name per workflow run and attempt (locally per process): a leftover
# of an earlier attempt never blocks a rerun
branch="pk-write-check-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-$$}"
api -X POST "repos/$repo/git/refs" -f "ref=refs/heads/$branch" -f "sha=$sha" --silent ||
	fail "Branch $branch nicht angelegt"

if ! api -X DELETE "repos/$repo/git/refs/heads/$branch" --silent; then
	msg="Prüf-Branch $branch in $repo nicht gelöscht; von Hand löschen: gh api -X DELETE repos/$repo/git/refs/heads/$branch"
	if [ "${GITHUB_ACTIONS:-}" = true ]; then
		echo "::warning::$msg"
	fi
	echo "Warnung: $msg" >&2
fi
echo "ok: PACKAGES_GITHUB_TOKEN kann $repo beschreiben"
