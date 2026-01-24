#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: scripts/publish_docker_beta.sh -r <repo> [-v <version>] [-p <platforms>] [--no-push]

Options:
  -r, --repo        Docker Hub repo, e.g. username/nginxpulse
  -v, --version     Beta version tag (defaults to git describe + beta timestamp)
  -p, --platforms   Build platforms (default: linux/amd64,linux/arm64)
  --no-push         Build only (no push)

Environment:
  DOCKERHUB_REPO    Same as --repo
  VERSION           Same as --version
  PLATFORMS         Same as --platforms

Notes:
  - This script never pushes the :latest tag.
  - If --version does not include "beta", "-beta" will be appended.
EOF
}

REPO="${DOCKERHUB_REPO:-}"
VERSION="${VERSION:-}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
PUSH=true

while [[ $# -gt 0 ]]; do
  case "$1" in
    -r|--repo)
      REPO="$2"
      shift 2
      ;;
    -v|--version)
      VERSION="$2"
      shift 2
      ;;
    -p|--platforms)
      PLATFORMS="$2"
      shift 2
      ;;
    --no-push)
      PUSH=false
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$REPO" ]]; then
  echo "Missing repo. Use -r or DOCKERHUB_REPO." >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  if git -C "$ROOT_DIR" describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git -C "$ROOT_DIR" describe --tags --exact-match)"
  else
    VERSION="$(git -C "$ROOT_DIR" describe --tags --abbrev=7 --always 2>/dev/null || date -u +%Y%m%d%H%M%S)"
  fi
  VERSION="${VERSION}-beta.$(date -u +%Y%m%d%H%M%S)"
else
  if [[ "$VERSION" != *beta* ]]; then
    VERSION="${VERSION}-beta"
  fi
fi

GO_MOD_VERSION="$(awk '/^go[[:space:]]+/{print $2; exit}' "$ROOT_DIR/go.mod" 2>/dev/null || true)"
DOCKER_GO_VERSION="$(awk 'BEGIN{IGNORECASE=1} /^FROM[[:space:]]+golang:/{print $2; exit}' "$ROOT_DIR/Dockerfile" 2>/dev/null | sed -e 's/^golang://' -e 's/-.*$//' || true)"
if [[ -n "$GO_MOD_VERSION" && -n "$DOCKER_GO_VERSION" ]]; then
  if [[ "$(printf '%s\n' "$GO_MOD_VERSION" "$DOCKER_GO_VERSION" | sort -V | head -n1)" != "$GO_MOD_VERSION" ]]; then
    echo "Go version mismatch: go.mod requires $GO_MOD_VERSION, Dockerfile uses $DOCKER_GO_VERSION." >&2
    echo "Update Dockerfile golang image or adjust go.mod before building." >&2
    exit 1
  fi
else
  echo "Warning: unable to detect Go versions from go.mod or Dockerfile." >&2
fi

BUILD_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
GIT_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short=7 HEAD 2>/dev/null || echo "unknown")"

TAG_LIST=("$REPO:$VERSION")
TAGS=()
for tag in "${TAG_LIST[@]}"; do
  TAGS+=(-t "$tag")
done

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker CLI not found." >&2
  exit 1
fi

BUILD_ARGS=(
  --build-arg "BUILD_TIME=$BUILD_TIME"
  --build-arg "GIT_COMMIT=$GIT_COMMIT"
  --build-arg "VERSION=$VERSION"
)
MULTI_PLATFORM=false
if [[ "$PLATFORMS" == *","* ]]; then
  MULTI_PLATFORM=true
fi

echo "Repo:     $REPO"
echo "Version:  $VERSION"
echo "Platforms:$PLATFORMS"
echo "Commit:   $GIT_COMMIT"
echo "Time:     $BUILD_TIME"

if $PUSH; then
  if docker buildx version >/dev/null 2>&1; then
    docker buildx build \
      --platform "$PLATFORMS" \
      --push \
      "${TAGS[@]}" \
      "${BUILD_ARGS[@]}" \
      -f "$ROOT_DIR/Dockerfile" \
      "$ROOT_DIR"
  else
    if [[ "$PLATFORMS" != "linux/amd64" ]]; then
      echo "Docker buildx is required for multi-arch builds." >&2
      exit 1
    fi
    docker build \
      "${TAGS[@]}" \
      "${BUILD_ARGS[@]}" \
      -f "$ROOT_DIR/Dockerfile" \
      "$ROOT_DIR"
    for tag in "${TAG_LIST[@]}"; do
      docker push "$tag"
    done
  fi
else
  if docker buildx version >/dev/null 2>&1; then
    if $MULTI_PLATFORM; then
      echo "Multi-arch build without push is not supported. Use --push or set -p to a single platform." >&2
      exit 1
    fi
    docker buildx build \
      --platform "$PLATFORMS" \
      --load \
      "${TAGS[@]}" \
      "${BUILD_ARGS[@]}" \
      -f "$ROOT_DIR/Dockerfile" \
      "$ROOT_DIR"
  else
    if [[ "$PLATFORMS" != "linux/amd64" ]]; then
      echo "Docker buildx is required for non-default platform builds." >&2
      exit 1
    fi
    docker build \
      "${TAGS[@]}" \
      "${BUILD_ARGS[@]}" \
      -f "$ROOT_DIR/Dockerfile" \
      "$ROOT_DIR"
  fi
fi
