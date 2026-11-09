#!/usr/bin/env bash
# Releases multispeed version X.Y.Z, from a clean checkout, in one go:
#
#   1. Prepare: on a branch from dev, merge main into it (so the previous
#      release tag is in dev's history), set VERSION, and turn the changelog's
#      "Unreleased" section into "X.Y.Z - today". PR into dev, wait for CI, merge.
#   2. PR dev -> main, wait for CI, merge.
#   3. The Release workflow sees the new VERSION on main, tags vX.Y.Z and
#      publishes the release; this script follows it and prints the link.
#
# Each step is skipped when it is already done, so the script can be re-run.
# Merging uses your admin rights (gh pr merge --admin): only the maintainer can
# run this. Needs git, gh (logged in) and bash (Git Bash on Windows).
#
# Usage:
#   scripts/release.sh X.Y.Z                  release
#   scripts/release.sh --prepare-only X.Y.Z   only make the local release commit (to check it)
set -euo pipefail

prepare_only=false
if [ "${1:-}" = "--prepare-only" ]; then
  prepare_only=true
  shift
fi
version="${1:-}"
tag="v$version"

say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
die() { printf '\n\033[1;31mrelease: %s\033[0m\n' "$*" >&2; exit 1; }

[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "usage: scripts/release.sh [--prepare-only] X.Y.Z"
cd "$(git rev-parse --show-toplevel)"
git diff --quiet && git diff --cached --quiet || die "you have uncommitted changes; commit or stash them first"
if ! $prepare_only; then
  gh auth status > /dev/null 2>&1 || die "gh is not logged in (run: gh auth login)"
fi
repo="${RELEASE_REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}" # override for tests
url="https://github.com/$repo"

git fetch origin --prune --tags --quiet
git rev-parse -q --verify "refs/tags/$tag" > /dev/null && die "$tag is already released"
start_branch="$(git rev-parse --abbrev-ref HEAD)"

# wait_and_merge PR STRATEGY: waits for the PR's required checks, then merges.
wait_and_merge() {
  local pr="$1" how="$2"
  echo "Waiting for CI on #$pr..."
  for _ in $(seq 1 40); do # checks can take a few seconds to appear
    gh pr checks "$pr" --repo "$repo" --required 2>/dev/null | grep -q . && break
    sleep 10
  done
  gh pr checks "$pr" --repo "$repo" --required --watch --fail-fast --interval 15 > /dev/null \
    || die "required checks failed on #$pr: $url/pull/$pr"
  gh pr merge "$pr" --repo "$repo" "$how" --admin
  echo "Merged #$pr."
}

# open_pr BASE HEAD TITLE BODY: opens a PR, or reuses an open one, and prints its number.
open_pr() {
  local base="$1" head="$2" title="$3" body="$4" pr
  pr="$(gh pr list --repo "$repo" --base "$base" --head "$head" --state open --json number --jq '.[0].number // empty')"
  if [ -z "$pr" ]; then
    gh pr create --repo "$repo" --base "$base" --head "$head" --title "$title" --body "$body" > /dev/null
    pr="$(gh pr list --repo "$repo" --base "$base" --head "$head" --state open --json number --jq '.[0].number')"
  fi
  echo "$pr"
}

# -- 1. Prepare on dev ---------------------------------------------------------
dev_version="$(git show origin/dev:VERSION | tr -d '[:space:]')"
if [ "$dev_version" = "$version" ]; then
  say "1/3 dev already has VERSION $version; skipping preparation"
  git show origin/dev:CHANGELOG.md | sh scripts/release-notes.sh "$version" /dev/stdin > /dev/null \
    || die "dev's CHANGELOG.md has no section for $version"
else
  say "1/3 Preparing $version (dev is at $dev_version)"
  newest="$(printf '%s\n%s\n' "$dev_version" "$version" | sort -V | tail -1)"
  [ "$newest" = "$version" ] || die "$version is not newer than $dev_version"
  git show origin/dev:CHANGELOG.md | sh scripts/release-notes.sh Unreleased /dev/stdin > /dev/null \
    || die "CHANGELOG.md has nothing under Unreleased on dev: add the changes first"

  branch="release/$tag"
  git switch --quiet -c "$branch" origin/dev
  git merge --quiet --no-ff --no-edit origin/main -m "Merge main into dev before $tag (keeps release tags in dev's history)" \
    || die "merging main into $branch failed; resolve it, or delete the branch and retry"
  printf '%s\n' "$version" > VERSION
  awk -v v="$version" -v old="$dev_version" -v d="$(date +%F)" -v url="$url" '
    $0 == "## [Unreleased]" { print; print ""; print "## [" v "] - " d; next }
    index($0, "[Unreleased]: ") == 1 {
      print "[Unreleased]: " url "/compare/v" v "...dev"
      print "[" v "]: " url "/compare/v" old "...v" v
      next
    }
    { print }
  ' CHANGELOG.md > CHANGELOG.md.new && mv CHANGELOG.md.new CHANGELOG.md
  sh scripts/release-notes.sh "$version" > /dev/null || die "changelog update failed"
  git add VERSION CHANGELOG.md
  git commit --quiet -m "Prepare release $version"
  echo "Release commit on $branch:"
  git --no-pager show --stat --format='  %h %s' HEAD

  if $prepare_only; then
    say "Stopped (--prepare-only). Check it with: git show $branch"
    echo "To undo: git switch $start_branch && git branch -D $branch"
    exit 0
  fi
  git push --quiet -u origin "$branch"
  pr="$(open_pr dev "$branch" "Prepare release $tag" \
    "Sets VERSION to $version and turns the changelog's Unreleased section into $version. Also merges main into dev, so the previous release tag is in dev's history. Opened by scripts/release.sh.")"
  echo "PR #$pr: $url/pull/$pr"
  wait_and_merge "$pr" --merge # a merge commit keeps the merge of main
  git switch --quiet "$start_branch"
  git branch --quiet -D "$branch"
  git push --quiet origin --delete "$branch" 2>/dev/null || true
  git fetch origin --prune --quiet
fi
$prepare_only && { say "Nothing to prepare: dev is already at $version"; exit 0; }

# -- 2. dev -> main ------------------------------------------------------------
main_version="$(git show origin/main:VERSION 2>/dev/null | tr -d '[:space:]' || true)"
if [ "$main_version" = "$version" ]; then
  say "2/3 main already has VERSION $version; skipping the dev -> main merge"
else
  say "2/3 Merging dev into main"
  pr="$(open_pr main dev "Release $tag" \
    "Release $tag. Merging this sets VERSION $version on main; the Release workflow then tags and publishes it. Opened by scripts/release.sh.")"
  echo "PR #$pr: $url/pull/$pr"
  wait_and_merge "$pr" --merge
  git fetch origin --prune --quiet
fi

# -- 3. The Release workflow tags and publishes --------------------------------
say "3/3 Waiting for the Release workflow"
main_sha="$(git rev-parse origin/main)"
run=""
for _ in $(seq 1 30); do
  run="$(gh run list --repo "$repo" --workflow Release --commit "$main_sha" --limit 1 --json databaseId --jq '.[0].databaseId // empty')"
  [ -n "$run" ] && break
  sleep 10
done
[ -n "$run" ] || die "no Release run started for main ($main_sha); see $url/actions"
gh run watch "$run" --repo "$repo" --exit-status --interval 15 > /dev/null \
  || die "the Release workflow failed: $url/actions/runs/$run"
gh release view "$tag" --repo "$repo" --json url --jq .url > /dev/null 2>&1 \
  || die "the workflow finished, but $tag has no release; see $url/actions/runs/$run"
say "Released $tag: $(gh release view "$tag" --repo "$repo" --json url --jq .url)"
