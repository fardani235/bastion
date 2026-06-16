#!/usr/bin/env bash
set -euo pipefail
#
# package.sh — Builds Bastion for Windows and creates an NSIS installer.
#
# Native Windows build (run on Windows with Wails + NSIS installed):
#   wails build -clean -platform windows/amd64 -nsis
#
# Cross-compilation from Linux (requires MinGW-w64 + NSIS):
#   sudo apt install gcc-mingw-w64-x86-64 nsis
#   ./build/windows/package.sh
#
# The output will be at build/bin/bastion-amd64-installer.exe
#

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="${VERSION:-1.0.0}"

echo "==> Building Bastion for windows/amd64…"
cd "$ROOT"

# Check for MinGW cross-compiler.
if ! command -v x86_64-w64-mingw32-gcc &>/dev/null; then
  echo ""
  echo "  MinGW cross-compiler not found. To install:"
  echo "    sudo apt install gcc-mingw-w64-x86-64 nsis"
  echo ""
  echo "  Alternatively, build natively on Windows:"
  echo "    wails build -clean -platform windows/amd64 -nsis"
  echo ""
  exit 1
fi

# Wails v2 cross-compilation to Windows.
CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
PKG_CONFIG=/bin/false \
wails build -clean -platform windows/amd64 -nsis -skipbindings

echo ""
echo "  Binary:     ${ROOT}/build/bin/bastion.exe"
echo "  Installer:  ${ROOT}/build/bin/bastion-amd64-installer.exe"
echo ""
echo "  The installer is a standalone .exe — distribute as-is."
ls -lh "${ROOT}/build/bin/"*.exe 2>/dev/null || true
