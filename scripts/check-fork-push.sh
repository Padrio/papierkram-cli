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
#
# The branch counts as created only if the API answers with its ref. On any
# other outcome the script still tries to delete it (best effort, errors
# ignored: an unreadable answer may hide a created branch) and names the
# branch and the command to delete a leftover. There is deliberately no
# trap: a cancelled job ends the step with a signal and kills it shortly
# after, so a cleanup in a trap may not run or may be cut off mid-request;
# it would only promise a cleanup it cannot guarantee. A leftover is
# harmless, and its name pk-write-check-<run>-<attempt> says where it came
# from.
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
# exactly 40 lowercase hex digits, one line
case "$sha" in
*[!0-9a-f]*) sha_ok=false ;;
*) if [ "${#sha}" -eq 40 ]; then sha_ok=true; else sha_ok=false; fi ;;
esac
if [ "$sha_ok" != true ]; then
	fail "Branch $default_branch von $repo ohne gültige Commit-SHA"
fi

# one name per workflow run and attempt (locally per process): a leftover
# of an earlier attempt never blocks a rerun
branch="pk-write-check-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-$$}"
delete_cmd="gh api -X DELETE repos/$repo/git/refs/heads/$branch"
created="$(api -X POST "repos/$repo/git/refs" -f "ref=refs/heads/$branch" -f "sha=$sha" --jq .ref)" || created=""
if [ "$created" != "refs/heads/$branch" ]; then
	api -X DELETE "repos/$repo/git/refs/heads/$branch" --silent >/dev/null 2>&1 || true
	fail "Branch $branch nicht angelegt (falls er doch existiert, von Hand löschen: $delete_cmd)"
fi

if ! api -X DELETE "repos/$repo/git/refs/heads/$branch" --silent; then
	msg="Prüf-Branch $branch in $repo nicht gelöscht; von Hand löschen: $delete_cmd"
	if [ "${GITHUB_ACTIONS:-}" = true ]; then
		echo "::warning::$msg"
	fi
	echo "Warnung: $msg" >&2
fi
echo "ok: PACKAGES_GITHUB_TOKEN kann $repo beschreiben"
