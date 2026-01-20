#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/docs/wiki"
REMOTE_NAME="${WIKI_REMOTE:-github}"
WIKI_DIR="${WIKI_DIR:-$ROOT_DIR/.wiki}"
COMMIT_MESSAGE="${WIKI_COMMIT_MESSAGE:-Update wiki}"

if [[ ! -d "$SRC_DIR" ]]; then
  echo "Wiki source not found: $SRC_DIR" >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "git not found in PATH." >&2
  exit 1
fi

REMOTE_URL=""
if git -C "$ROOT_DIR" remote get-url "$REMOTE_NAME" >/dev/null 2>&1; then
  REMOTE_URL="$(git -C "$ROOT_DIR" remote get-url "$REMOTE_NAME")"
elif git -C "$ROOT_DIR" remote get-url origin >/dev/null 2>&1; then
  REMOTE_URL="$(git -C "$ROOT_DIR" remote get-url origin)"
fi

WIKI_URL="${WIKI_URL:-}"
if [[ -z "$WIKI_URL" ]]; then
  case "$REMOTE_URL" in
    https://github.com/*)
      WIKI_URL="${REMOTE_URL%.git}.wiki.git"
      ;;
    git@github.com:*)
      WIKI_URL="https://github.com/${REMOTE_URL#git@github.com:}"
      WIKI_URL="${WIKI_URL%.git}.wiki.git"
      ;;
    ssh://git@github.com/*)
      WIKI_URL="https://github.com/${REMOTE_URL#ssh://git@github.com/}"
      WIKI_URL="${WIKI_URL%.git}.wiki.git"
      ;;
  esac
fi

if [[ -z "$WIKI_URL" ]]; then
  echo "Cannot infer GitHub wiki URL. Set WIKI_URL explicitly." >&2
  exit 1
fi

if [[ ! -d "$WIKI_DIR/.git" ]]; then
  git clone "$WIKI_URL" "$WIKI_DIR"
else
  git -C "$WIKI_DIR" remote set-url origin "$WIKI_URL" >/dev/null 2>&1 || true
  git -C "$WIKI_DIR" pull --ff-only
fi

if command -v rsync >/dev/null 2>&1; then
  rsync -a --delete "$SRC_DIR"/ "$WIKI_DIR"/
else
  echo "rsync not found; falling back to copy (deletions will not be removed)." >&2
  find "$WIKI_DIR" -mindepth 1 -maxdepth 1 ! -name ".git" -exec rm -rf {} + >/dev/null 2>&1 || true
  cp -R "$SRC_DIR"/. "$WIKI_DIR"/
fi

git -C "$WIKI_DIR" add .
if git -C "$WIKI_DIR" diff --cached --quiet; then
  echo "No wiki changes to commit."
  exit 0
fi

git -C "$WIKI_DIR" commit -m "$COMMIT_MESSAGE"
git -C "$WIKI_DIR" push
