#!/usr/bin/env bash
set -euo pipefail
#
# package.sh — Builds Bastion and packages it as a .deb installer.
#
# Usage:
#   ./build/linux/package.sh            # build + package
#   VERSION=1.0.1 ./build/linux/package.sh  # custom version
#
# Prerequisites: wails CLI, dpkg-deb, fakeroot.
#

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN="${ROOT}/build/bin/bastion"
VERSION="${VERSION:-1.0.0}"
PKGNAME="bastion_${VERSION}_amd64"
STAGING="/tmp/${PKGNAME}"

# ---- 1. Build the binary ------------------------------------------------
echo "==> Building Bastion binary with wails…"
cd "$ROOT"
wails build -clean -tags "webkit2_41"
echo "    Binary: ${BIN}"

if [ ! -f "$BIN" ]; then
  echo "FATAL: binary not found at ${BIN}." >&2
  exit 1
fi

# ---- 2. Prepare staging directory ---------------------------------------
echo "==> Preparing .deb staging directory…"
rm -rf "$STAGING"
mkdir -p "${STAGING}/DEBIAN"
mkdir -p "${STAGING}/usr/bin"
mkdir -p "${STAGING}/usr/share/applications"
mkdir -p "${STAGING}/usr/share/icons/hicolor/scalable/apps"
mkdir -p "${STAGING}/usr/share/icons/hicolor/256x256/apps"

# Copy binary.
cp "$BIN" "${STAGING}/usr/bin/bastion"

# Copy desktop entry.
cp "${ROOT}/build/linux/bastion.desktop" "${STAGING}/usr/share/applications/"

# Copy icon (resized to 256 for hicolor; DE scales as needed).
cp "${ROOT}/build/appicon.png" "${STAGING}/usr/share/icons/hicolor/256x256/apps/bastion.png"

# ---- 3. Write control file ----------------------------------------------
cat > "${STAGING}/DEBIAN/control" <<CONTROL
Package: bastion
Version: ${VERSION}
Section: net
Priority: optional
Architecture: amd64
Maintainer: fardani235 <ridwan.fardani@gmail.com>
Depends: libgtk-3-0 (>= 3.24), libwebkit2gtk-4.1-0 (>= 2.44), libjavascriptcoregtk-4.1-0, libappindicator3-1 | libayatana-appindicator3-1
Description: SSH client and session manager with encrypted vault
 Bastion is a local-first SSH client with an encrypted credential vault,
 PTY terminal emulation, host/group management, snippet manager, and
 local port forwarding.
CONTROL

# ---- 4. Build .deb ------------------------------------------------------
echo "==> Building .deb package…"
fakeroot dpkg-deb --build "$STAGING" "${ROOT}/build/${PKGNAME}.deb"
echo ""
echo "    Package: ${ROOT}/build/${PKGNAME}.deb"
echo "    Install: sudo dpkg -i ${ROOT}/build/${PKGNAME}.deb"
