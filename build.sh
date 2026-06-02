#!/usr/bin/env bash
# build.sh — cross-compilation helper for dropserve
# Usage: ./build.sh [build|dist|test|clean]
set -euo pipefail

BINARY="dropserve"
VERSION="0.1.0"
CMD="./cmd/dropserve"
DIST="dist"
LDFLAGS="-s -w -X 'main.version=${VERSION}'"

cmd="${1:-dist}"

case "$cmd" in
  build)
    echo "Building ${BINARY}..."
    go build -ldflags "${LDFLAGS}" -o "${BINARY}" "${CMD}"
    echo "Done: ./${BINARY}"
    ;;

  test)
    go test ./...
    ;;

  dist)
    rm -rf "${DIST}"
    mkdir -p "${DIST}"
    platforms=(
      "linux/amd64"
      "linux/arm64"
      "darwin/amd64"
      "darwin/arm64"
      "windows/amd64"
    )
    for platform in "${platforms[@]}"; do
      IFS='/' read -r goos goarch <<< "${platform}"
      out="${DIST}/${BINARY}-${goos}-${goarch}"
      [[ "$goos" == "windows" ]] && out="${out}.exe"
      echo "  → ${out}"
      GOOS="${goos}" GOARCH="${goarch}" go build -ldflags "${LDFLAGS}" -o "${out}" "${CMD}"
    done
    echo ""
    echo "Built for ${#platforms[@]} platforms:"
    ls -lh "${DIST}/"
    ;;

  clean)
    rm -rf "${DIST}" "${BINARY}" "${BINARY}.exe"
    echo "Cleaned."
    ;;

  *)
    echo "Usage: $0 [build|dist|test|clean]"
    exit 1
    ;;
esac
