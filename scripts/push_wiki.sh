#!/usr/bin/env bash
set -euo pipefail

# Env overrides:
# - WIKI_REMOTE: git remote name to infer wiki URL (default: github, fallback: origin)
# - WIKI_URL: explicit wiki repo URL (e.g. https://github.com/user/repo.wiki.git)
# - WIKI_DIR: local wiki checkout dir (default: $ROOT_DIR/.wiki)
# - WIKI_COMMIT_MESSAGE: commit message (default: "Update wiki")
# - KEEP_WIKI_BACKUPS: number of non-git dir backups to keep (default: 3)
# - WIKI_AUTO_CLEAN: when 1, auto-backup and clean dirty wiki repo (default: 1)
# - KEEP_WIKI_DIRTY_BACKUPS: number of dirty backups to keep (default: 3)

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/docs/wiki"
REMOTE_NAME="${WIKI_REMOTE:-github}"
WIKI_DIR="${WIKI_DIR:-$ROOT_DIR/.wiki}"
COMMIT_MESSAGE="${WIKI_COMMIT_MESSAGE:-Update wiki}"
KEEP_WIKI_BACKUPS="${KEEP_WIKI_BACKUPS:-3}"
WIKI_AUTO_CLEAN="${WIKI_AUTO_CLEAN:-1}"
KEEP_WIKI_DIRTY_BACKUPS="${KEEP_WIKI_DIRTY_BACKUPS:-3}"

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

cleanup_backups() {
  local keep="$1"
  if ! [[ "$keep" =~ ^[0-9]+$ ]]; then
    echo "KEEP_WIKI_BACKUPS must be a non-negative integer; got: $keep" >&2
    return 0
  fi

  local backups=()
  if ls -d "${WIKI_DIR}".bak.* >/dev/null 2>&1; then
    IFS=$'\n' read -r -d '' -a backups < <(ls -dt "${WIKI_DIR}".bak.* && printf '\0')
  fi
  if (( ${#backups[@]} <= keep )); then
    return 0
  fi
  for ((i=keep; i<${#backups[@]}; i++)); do
    rm -rf "${backups[$i]}"
  done
}

cleanup_dirty_backups() {
  local keep="$1"
  if ! [[ "$keep" =~ ^[0-9]+$ ]]; then
    echo "KEEP_WIKI_DIRTY_BACKUPS must be a non-negative integer; got: $keep" >&2
    return 0
  fi

  local backups=()
  if ls -d "${WIKI_DIR}".dirty.* >/dev/null 2>&1; then
    IFS=$'\n' read -r -d '' -a backups < <(ls -dt "${WIKI_DIR}".dirty.* && printf '\0')
  fi
  if (( ${#backups[@]} <= keep )); then
    return 0
  fi
  for ((i=keep; i<${#backups[@]}; i++)); do
    rm -rf "${backups[$i]}"
  done
}

ensure_clean_repo() {
  if [[ "$WIKI_AUTO_CLEAN" == "0" ]]; then
    return 0
  fi
  if git -C "$WIKI_DIR" status --porcelain | grep -q .; then
    ts="$(date +%Y%m%d_%H%M%S)"
    dirty_backup="${WIKI_DIR}.dirty.${ts}"
    echo "Wiki repo has uncommitted changes, backing up to: $dirty_backup" >&2
    if command -v rsync >/dev/null 2>&1; then
      rsync -a --exclude ".git" "$WIKI_DIR"/ "$dirty_backup"/
    else
      mkdir -p "$dirty_backup"
      cp -R "$WIKI_DIR"/. "$dirty_backup"/
      rm -rf "$dirty_backup/.git" >/dev/null 2>&1 || true
    fi
    cleanup_dirty_backups "$KEEP_WIKI_DIRTY_BACKUPS"
    git -C "$WIKI_DIR" reset --hard
    git -C "$WIKI_DIR" clean -fd
  fi
}

if [[ ! -d "$WIKI_DIR/.git" ]]; then
  if [[ -d "$WIKI_DIR" ]]; then
    ts="$(date +%Y%m%d_%H%M%S)"
    backup_dir="${WIKI_DIR}.bak.${ts}"
    echo "Existing wiki dir has no .git, moving to: $backup_dir" >&2
    mv "$WIKI_DIR" "$backup_dir"
    cleanup_backups "$KEEP_WIKI_BACKUPS"
  fi
  git clone "$WIKI_URL" "$WIKI_DIR"
else
  git -C "$WIKI_DIR" remote set-url origin "$WIKI_URL" >/dev/null 2>&1 || true
  ensure_clean_repo
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
