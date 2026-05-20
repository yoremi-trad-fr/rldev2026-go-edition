#!/bin/bash
# ============================================================
#  RLdev2026-Go — Build Script (Linux/macOS)
#  Compile les 4 outils depuis les sources Go
# ============================================================

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTDIR="$SCRIPT_DIR/bin"
mkdir -p "$OUTDIR"

export GOCACHE="${GOCACHE:-$SCRIPT_DIR/.gocache}"
export GOTMPDIR="${GOTMPDIR:-$SCRIPT_DIR/.gotmp}"
mkdir -p "$GOCACHE" "$GOTMPDIR"

# Detect target OS
TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"
EXT=""
if [ "$TARGET_OS" = "windows" ]; then EXT=".exe"; fi

echo ""
echo "  RLdev2026-Go Build Script"
echo "  Target: ${TARGET_OS}/${TARGET_ARCH}"
echo "  ========================="
echo ""
echo "  Go cache: $GOCACHE"
echo "  Go temp : $GOTMPDIR"
echo ""

echo "[1/4] Building kprl16${EXT}..."
cd "$SCRIPT_DIR" && go build -trimpath -ldflags "-buildid=" -o "$OUTDIR/kprl16${EXT}" ./kprl/cmd/kprl

echo "[2/4] Building rlc2026${EXT}..."
cd "$SCRIPT_DIR" && go build -trimpath -ldflags "-buildid=" -o "$OUTDIR/rlc2026${EXT}" ./rlc/cmd/rlc

echo "[3/4] Building rlxml${EXT}..."
cd "$SCRIPT_DIR" && go build -trimpath -ldflags "-buildid=" -o "$OUTDIR/rlxml${EXT}" ./rlxml/cmd/rlxml

echo "[4/4] Building vaconv${EXT}..."
cd "$SCRIPT_DIR" && go build -trimpath -ldflags "-buildid=" -o "$OUTDIR/vaconv${EXT}" ./vaconv/cmd/vaconv

echo ""
echo "  All tools built in: $OUTDIR"
ls -lh "$OUTDIR"/*${EXT}
echo ""

# Cross-compile for Windows from Linux/macOS:
# GOOS=windows GOARCH=amd64 ./build.sh
