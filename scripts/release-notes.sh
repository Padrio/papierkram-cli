#!/bin/sh
# Prints the section of a release tag from CHANGELOG.md (Keep a Changelog)
# for goreleaser --release-notes (.github/workflows/release.yml,
# docs/release.md). Fails if the section is missing or empty, so a release
# never goes out without notes.
#
# Usage: scripts/release-notes.sh vX.Y.Z [changelog]
set -eu

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
	echo "Verwendung: scripts/release-notes.sh vX.Y.Z [changelog]" >&2
	exit 2
fi
tag="$1"
changelog="${2:-CHANGELOG.md}"

# SemVer with v, optional prerelease and build metadata; one line only
nl='
'
case "$tag" in
*"$nl"*) valid=false ;;
*) if printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'; then valid=true; else valid=false; fi ;;
esac
if [ "$valid" != true ]; then
	echo "release-notes.sh: kein Versions-Tag: $tag (erwartet vX.Y.Z, z. B. v1.2.3 oder v1.2.3-rc.1)" >&2
	exit 2
fi
version="${tag#v}"

if [ ! -f "$changelog" ] || [ ! -r "$changelog" ]; then
	echo "release-notes.sh: $changelog fehlt oder ist nicht lesbar" >&2
	exit 1
fi

# the lines after "## [X.Y.Z]" up to the next "## [" heading, without the
# link definitions at the end of the file and without leading and trailing
# blank lines; headings and link definitions inside code fences belong to
# the notes. A fence opened with ``` closes only with ```, one opened with
# ~~~ only with ~~~ (CommonMark: at least as long, nothing but blanks
# after it). A fence still open at the end of the file (in the section or
# before it) is an error: the section has no safe end.
rc=0
notes="$(awk -v v="$version" '
	!on && fence == "" && index($0, "## [" v "]") == 1 { found = 1; on = 1; next }
	/^(```|~~~)/ {
		c = substr($0, 1, 1)
		n = 0
		while (substr($0, n + 1, 1) == c) n++
		if (fence == "") { fence = c; flen = n }
		else if (c == fence && n >= flen && substr($0, n + 1) ~ /^[ \t]*$/) fence = ""
	}
	on && fence == "" && /^## \[/ { exit }
	on && fence == "" && /^\[[^]]*\]: / { next }
	on { print }
	END {
		if (fence != "" && (on || !found)) exit 4
		if (!found) exit 3
	}
' "$changelog")" || rc=$?
case $rc in
0) ;;
3)
	echo "release-notes.sh: Abschnitt \"## [$version]\" fehlt in $changelog" >&2
	exit 1
	;;
4)
	echo "release-notes.sh: Code-Block (\`\`\` oder ~~~) in $changelog nicht geschlossen (im Abschnitt \"## [$version]\" oder davor)" >&2
	exit 4
	;;
*)
	echo "release-notes.sh: $changelog nicht auswertbar (awk: Exit $rc)" >&2
	exit 1
	;;
esac
notes="$(printf '%s\n' "$notes" | sed '/./,$!d')"
if [ -z "$notes" ]; then
	echo "release-notes.sh: Abschnitt \"## [$version]\" in $changelog ist leer" >&2
	exit 1
fi
printf '%s\n' "$notes"
