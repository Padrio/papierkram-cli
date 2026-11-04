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

rm -rf completions
mkdir completions
for shell in bash zsh fish powershell; do
	"$tmp/papierkram" completion "$shell" >"$tmp/$shell"
done

# The short form pk (deb/rpm link, Homebrew binary target, Scoop shim)
# completes like papierkram: every script runs the program it completes
# (pk __complete), so pk only needs the same registration. Each step first
# checks the line of cobra's script it relies on, so a changed cobra fails
# the release instead of shipping a broken pk completion;
# scripts/check-completions.sh tests the result in the shells.
require() { # file, exact line
	if ! grep -qxF -- "$2" "$1"; then
		echo "completions.sh: Zeile fehlt in $1: $2" >&2
		exit 1
	fi
}

# bash: the same registration as cobra's for papierkram, including the
# nospace fallback for a bash without compopt
require "$tmp/bash" '    complete -o default -F __start_papierkram papierkram'
require "$tmp/bash" '    complete -o default -o nospace -F __start_papierkram papierkram'
{
	cat "$tmp/bash"
	# shellcheck disable=SC2016 # bash code for the script, not run here
	printf '%s\n' '' '# short form pk (scripts/completions.sh)' \
		'if [[ $(type -t compopt) = "builtin" ]]; then' \
		'    complete -o default -F __start_papierkram pk' \
		'else' \
		'    complete -o default -o nospace -F __start_papierkram pk' \
		'fi'
} >completions/papierkram.bash

# zsh: compinit binds the names of the #compdef line when it finds the file
# in fpath (packages, Homebrew); the compdef line binds them when the script
# is sourced. Both get pk. An appended compdef line would only run inside
# the autoloaded function, so the first two lines are replaced instead.
if [ "$(sed -n '1,2p' "$tmp/zsh")" != "#compdef papierkram
compdef _papierkram papierkram" ]; then
	echo "completions.sh: unerwarteter Kopf des zsh-Skripts in $tmp/zsh" >&2
	exit 1
fi
{
	printf '%s\n' '#compdef papierkram pk' 'compdef _papierkram papierkram pk'
	sed '1,2d' "$tmp/zsh"
} >completions/_papierkram

# fish: pk takes over all completions of papierkram
require "$tmp/fish" 'complete -c papierkram -e'
{
	cat "$tmp/fish"
	printf '%s\n' '' '# short form pk (scripts/completions.sh)' 'complete -c pk -w papierkram'
} >completions/papierkram.fish

# PowerShell: the same script block for pk
# shellcheck disable=SC2016 # PowerShell variable, not expanded by sh
block='${__papierkramCompleterBlock}'
require "$tmp/powershell" "Register-ArgumentCompleter -CommandName 'papierkram' -ScriptBlock $block"
{
	cat "$tmp/powershell"
	printf '%s\n' '' '# short form pk (scripts/completions.sh)' \
		"Register-ArgumentCompleter -CommandName 'pk' -ScriptBlock $block"
} >completions/papierkram.ps1
