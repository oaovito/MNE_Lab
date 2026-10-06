#!/usr/bin/env bash
# Builds MNE Lab and its Portable USB launcher into out/.
#
#   scripts/build.sh [goos/goarch ...]
#
# Without targets it builds Windows (x64, x86, ARM64) and Linux (x64, ARM64).
# macOS needs cgo for the tray icon and is built on a Mac with the same
# command (target darwin/arm64 or darwin/amd64).
#
# VERSION (default 0.1.0-dev) and CHANNEL (default dev) are written into the
# executables. Windows builds get the MNE Lab icon and version information
# and run without a console window.
#
# The interface must be built first: (cd web && npm ci && npm run build).
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=${VERSION:-0.1.0-dev}
CHANNEL=${CHANNEL:-dev}
WINRES=github.com/tc-hib/go-winres@v0.3.3
targets=("$@")
[ ${#targets[@]} -gt 0 ] || targets=(windows/amd64 windows/386 windows/arm64 linux/amd64 linux/arm64)

if [ ! -f web/dist/index.html ]; then
  echo "The interface is not built. Run: (cd web && npm ci && npm run build)" >&2
  exit 1
fi

# Windows version numbers have four parts and no pre-release suffix.
num=${VERSION%%[-+]*}
case $num in
  *.*.*.*) ;;
  *.*.*) num=$num.0 ;;
  *) echo "VERSION must look like 1.2.3" >&2; exit 1 ;;
esac

cleanup() { rm -f cmd/mnelab/rsrc_windows_*.syso cmd/mnelab-launcher/rsrc_windows_*.syso; }
trap cleanup EXIT

pkg=github.com/oaovito/mne_lab/internal/version
mkdir -p out
for t in "${targets[@]}"; do
  os=${t%/*} arch=${t#*/} ext="" ldflags="-s -w -X $pkg.Version=$VERSION -X $pkg.Channel=$CHANNEL"
  cgo=0
  if [ "$os" = windows ]; then
    ext=.exe
    ldflags="$ldflags -H windowsgui"
    for c in mnelab mnelab-launcher; do
      (cd cmd/$c && go run $WINRES make --in winres/winres.json --out rsrc --arch "$arch" \
        --product-version "$num" --file-version "$num")
    done
  fi
  [ "$os" = darwin ] && cgo=1
  for c in mnelab mnelab-launcher; do
    CGO_ENABLED=$cgo GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false -ldflags "$ldflags" \
      -o "out/$c-$os-$arch$ext" ./cmd/$c
  done
  cleanup
  echo "built $os/$arch"
done
