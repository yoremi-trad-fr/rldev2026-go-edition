#!/bin/bash
# ============================================================
#  RLdev2026-Go — Build Script (Linux/macOS)
#  Compile les 4 outils depuis les sources Go
# ============================================================

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTDIR="$SCRIPT_DIR/bin"
mkdir -p "$OUTDIR"

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

echo "[1/4] Building kprl16${EXT}..."
cd "$SCRIPT_DIR/kprl" && go build -o "$OUTDIR/kprl16${EXT}" ./cmd/kprl/

echo "[2/4] Building rlc2026${EXT}..."
cd "$SCRIPT_DIR/rlc" && go build -o "$OUTDIR/rlc2026${EXT}" ./cmd/rlc/

echo "[3/4] Building rlxml${EXT}..."
cd "$SCRIPT_DIR/rlxml" && go build -o "$OUTDIR/rlxml${EXT}" ./cmd/rlxml/

echo "[4/4] Building vaconv${EXT}..."
cd "$SCRIPT_DIR/vaconv" && go build -o "$OUTDIR/vaconv${EXT}" ./cmd/vaconv/

echo ""
echo "  All tools built in: $OUTDIR"
ls -lh "$OUTDIR"/*${EXT}
echo ""

# Cross-compile for Windows from Linux/macOS:
# GOOS=windows GOARCH=amd64 ./build.sh
