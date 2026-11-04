#!/bin/sh
# Checks the scripts of scripts/completions.sh (default dir: completions):
# each shell completes both papierkram and the short form pk. Shells that
# are not installed are skipped, unless REQUIRE_ALL_SHELLS=1 (CI).
#
# bash and zsh: the registration (complete -p, $_comps; zsh both from fpath
# like the packages install it and sourced). fish and PowerShell: a real
# completion of "pk " against a stub pk in PATH that answers __complete,
# so the real binary is never run.
set -eu

dir="${1:-completions}"
cd "$(dirname "$0")/.."
dir="$(cd "$dir" && pwd)"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

# stub: one candidate and the directive ShellCompDirectiveNoFileComp
mkdir "$tmp/bin"
cat >"$tmp/bin/pk" <<'EOF'
#!/bin/sh
printf 'invoice\tRechnungen\n:4\n'
EOF
chmod +x "$tmp/bin/pk"

failed=0
fail() {
	echo "FEHLER $1: $2" >&2
	failed=1
}
missing() {
	if [ "${REQUIRE_ALL_SHELLS:-}" = 1 ]; then
		fail "$1" "Shell nicht installiert"
	else
		echo "übersprungen $1: nicht installiert"
	fi
}

if command -v bash >/dev/null 2>&1; then
	got="$(bash --norc --noprofile -c '. "$1/papierkram.bash" && complete -p papierkram pk' bash "$dir")" || got=""
	want="complete -o default -F __start_papierkram papierkram
complete -o default -F __start_papierkram pk"
	if [ "$got" = "$want" ]; then echo "ok bash"; else fail bash "$got"; fi
else
	missing bash
fi

if command -v zsh >/dev/null 2>&1; then
	# -D: compinit writes no dump file
	got="$(zsh -f -c 'fpath=("$1" $fpath); autoload -U compinit; compinit -u -D; print -r -- "$_comps[papierkram] $_comps[pk]"' zsh "$dir")" || got=""
	if [ "$got" = "_papierkram _papierkram" ]; then echo "ok zsh (fpath)"; else fail "zsh (fpath)" "$got"; fi
	got="$(zsh -f -c 'autoload -U compinit; compinit -u -D; source "$1/_papierkram"; print -r -- "$_comps[papierkram] $_comps[pk]"' zsh "$dir")" || got=""
	if [ "$got" = "_papierkram _papierkram" ]; then echo "ok zsh (source)"; else fail "zsh (source)" "$got"; fi
else
	missing zsh
fi

if command -v fish >/dev/null 2>&1; then
	# shellcheck disable=SC2016 # fish expands $argv, not sh
	got="$(PATH="$tmp/bin:$PATH" fish --no-config -c 'source $argv[1]/papierkram.fish; complete --do-complete "pk "' "$dir")" || got=""
	if printf '%s\n' "$got" | grep -q '^invoice'; then echo "ok fish"; else fail fish "$got"; fi
else
	missing fish
fi

if command -v pwsh >/dev/null 2>&1; then
	# shellcheck disable=SC2016 # PowerShell expands $env, not sh
	got="$(PATH="$tmp/bin:$PATH" COMPLETION_FILE="$dir/papierkram.ps1" pwsh -NoProfile -NonInteractive -Command \
		'. $env:COMPLETION_FILE; (TabExpansion2 -inputScript "pk " -cursorColumn 3).CompletionMatches.CompletionText')" || got=""
	if printf '%s\n' "$got" | grep -q '^invoice'; then echo "ok powershell"; else fail powershell "$got"; fi
else
	missing powershell
fi

exit "$failed"
