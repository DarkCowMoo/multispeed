#!/usr/bin/env sh
# Prints the CHANGELOG.md section for one version, for the release page.
# Usage: scripts/release-notes.sh 0.1.0 [CHANGELOG.md]
# Exits with an error if the version has no section or the section is empty,
# so a release cannot go out without notes.
set -eu

version="${1:?usage: release-notes.sh VERSION [CHANGELOG.md]}"
file="${2:-CHANGELOG.md}"

notes="$(awk -v v="$version" '
  /^## \[/ { if (found) exit; if (index($0, "## [" v "]") == 1) { found = 1; next } }
  found && /^\[[^]]+\]: / { next }   # skip the link definitions at the end
  found { print }
' "$file" | sed -e '/./,$!d')"   # drop leading blank lines

if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
  echo "release-notes.sh: no notes for version $version in $file" >&2
  exit 1
fi
printf '%s\n' "$notes"
