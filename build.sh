#!/bin/sh
# Builds the UI and the binary for each target into dist/<os>-<arch>/.
# Copy that folder to the server and run ./moonshed install.
set -e
cd "$(dirname "$0")"

VERSION=${VERSION:-$(date +%Y.%m.%d)}
TARGETS=${TARGETS:-"linux/amd64 linux/arm64 windows/amd64"}

(cd frontend && npm ci && npm run build)

for target in $TARGETS; do
  os=${target%/*}
  arch=${target#*/}
  out=dist/$os-$arch
  ext=""
  [ "$os" = windows ] && ext=.exe

  rm -rf "$out"
  mkdir -p "$out/runtime"
  (cd backend && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" -o "../$out/moonshed$ext" ./cmd/moonshed)
  cp -r frontend/dist "$out/runtime/web"
  echo "built $out"
done
