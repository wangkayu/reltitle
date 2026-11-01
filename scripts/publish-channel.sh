#!/usr/bin/env bash
# Publish a staged channel tree to a channel branch.
#
#   scripts/publish-channel.sh <branch> <staged-root> <version> <commit-message>
#
# <staged-root> is what `reltitle-release channel --out` wrote: a `lines/`
# directory holding the signed manifest and the binaries it names. The branch
# is rewritten to exactly that tree in one commit, so a reader never sees a
# manifest next to the wrong binaries. The push is a fast-forward, never a
# force: a concurrent publisher's push wins and this one fails closed.
set -euo pipefail

branch="${1:?branch}"
staged="${2:?staged root}"
version="${3:?version}"
message="${4:-channel: publish $version}"

if [ ! -d "$staged/lines" ]; then
  echo "publish: $staged/lines does not exist" >&2
  exit 1
fi
if ! git config user.email >/dev/null 2>&1; then
  git config user.email "release@users.noreply.github.com"
  git config user.name "reltitle release"
fi

worktree="$(mktemp -d)"
cleanup() {
  git worktree remove --force "$worktree" >/dev/null 2>&1 || true
  rm -rf "$worktree"
}
trap cleanup EXIT

if git fetch --depth=1 origin "$branch" >/dev/null 2>&1; then
  git worktree add --detach "$worktree" FETCH_HEAD >/dev/null
  base="$(git rev-parse FETCH_HEAD)"
else
  # First publication on this channel: start an empty branch so the push
  # creates it instead of forcing over anything.
  git worktree add --detach "$worktree" >/dev/null
  (
    cd "$worktree"
    git checkout --orphan channel-publish >/dev/null 2>&1
    git rm -rf --quiet . >/dev/null 2>&1 || true
  )
  base=""
fi

rm -rf "$worktree/lines"
cp -R "$staged/lines" "$worktree/lines"
(
  cd "$worktree"
  git add -A
  if git diff --cached --quiet; then
    echo "publish: $branch already holds this content"
    exit 0
  fi
  git commit -q -m "$message"
  git push origin "HEAD:refs/heads/$branch"
)

if [ -n "$base" ]; then
  echo "publish: $branch advanced from $base"
else
  echo "publish: $branch created"
fi
