#!/usr/bin/env bash
set -euo pipefail

# RLdev2026-Go command line tools build script.
# Native Linux builds produce bin/kprl16, bin/rlc2026, bin/rlxml and bin/vaconv.
# Windows builds produce the same names with .exe and embed version resources.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUTDIR="${OUTDIR:-"$SCRIPT_DIR/bin"}"
TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"
EXT=""

if [[ "$TARGET_OS" == "windows" ]]; then
  EXT=".exe"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required. Install Go 1.22 or newer, then run this script again." >&2
  exit 1
fi

mkdir -p "$OUTDIR"
export GOCACHE="${GOCACHE:-"$SCRIPT_DIR/.gocache"}"
export GOTMPDIR="${GOTMPDIR:-"$SCRIPT_DIR/.gotmp"}"
mkdir -p "$GOCACHE" "$GOTMPDIR"

export GOOS="$TARGET_OS"
export GOARCH="$TARGET_ARCH"

if [[ "$TARGET_OS" != "windows" ]]; then
  export CGO_ENABLED="${CGO_ENABLED:-0}"
fi

echo
echo "  RLdev2026-Go Build Script"
echo "  Target : ${GOOS}/${GOARCH}"
echo "  Output : $OUTDIR"
echo "  Go     : $(go env GOVERSION)"
echo

if [[ "$GOOS" == "windows" && "$GOARCH" == "amd64" ]]; then
  echo "[prep] Embedding Windows version resources..."
  (cd "$SCRIPT_DIR" && GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)" go run ./kprl/internal/winresgen -root "$SCRIPT_DIR")
elif [[ "$GOOS" == "windows" ]]; then
  echo "[prep] Windows resources are currently generated for amd64 only; continuing without them for ${GOARCH}."
fi

build_tool() {
  local step="$1"
  local name="$2"
  local pkg="$3"

  echo "[$step/4] Building ${name}${EXT}..."
  (cd "$SCRIPT_DIR" && go build -trimpath -o "$OUTDIR/${name}${EXT}" "$pkg")
}

build_tool 1 kprl16 ./kprl/cmd/kprl
build_tool 2 rlc2026 ./rlc/cmd/rlc
build_tool 3 rlxml ./rlxml/cmd/rlxml
build_tool 4 vaconv ./vaconv/cmd/vaconv

echo
echo "  All tools built successfully:"
for tool in kprl16 rlc2026 rlxml vaconv; do
  if [[ -f "$OUTDIR/${tool}${EXT}" ]]; then
    ls -lh "$OUTDIR/${tool}${EXT}"
  fi
done
echo

