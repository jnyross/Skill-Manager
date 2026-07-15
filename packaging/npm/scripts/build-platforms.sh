#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
version="${RELEASE_VERSION:?set RELEASE_VERSION without a leading v}"
commit="${RELEASE_COMMIT:?set RELEASE_COMMIT}"
build_date="${RELEASE_BUILD_DATE:?set RELEASE_BUILD_DATE in RFC3339 form}"
gocache="${GOCACHE:-${TMPDIR:-/tmp}/skillet-release-gocache}"

case "$version" in
  v*) echo "RELEASE_VERSION must not include a leading v" >&2; exit 1 ;;
esac

build() {
  local goos="$1" goarch="$2" package_dir="$3"
  local output="$repo_root/packaging/npm/packages/$package_dir/bin/skillet"
  mkdir -p "$(dirname "$output")" "$gocache"
  (
    cd "$repo_root"
    GOCACHE="$gocache" CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath \
      -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$build_date" \
      -o "$output" ./cmd/skillet
  )
  chmod 0755 "$output"
}

build darwin arm64 darwin-arm64
build darwin amd64 darwin-x64
build linux arm64 linux-arm64
build linux amd64 linux-x64

RELEASE_VERSION="$version" node "$repo_root/packaging/npm/scripts/validate-packages.mjs" --require-binaries
