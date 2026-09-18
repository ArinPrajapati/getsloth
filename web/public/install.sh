#!/bin/sh
# Installs the getsloth CLI (invoked as `sloth` once aliased - see the
# README) by downloading the right prebuilt binary from GitHub Releases.
# No Go toolchain required. Matches the archive naming goreleaser
# produces: getsloth_<os>_<arch>.tar.gz, per .goreleaser.yml.
set -eu

REPO="arinprajapati/getsloth"
BIN_NAME="getsloth"

fail() {
  echo "getsloth-install: $1" >&2
  exit 1
}

os="$(uname -s)"
case "$os" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) fail "unsupported OS: $os (only Linux and macOS have prebuilt binaries - see README for go install)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) fail "unsupported architecture: $arch" ;;
esac

install_dir="${GETSLOTH_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$install_dir"

archive="${BIN_NAME}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/latest/download/${archive}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "getsloth-install: downloading ${url}"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$url" -o "$tmp_dir/$archive" || fail "download failed - is there a release published yet?"
elif command -v wget >/dev/null 2>&1; then
  wget -q "$url" -O "$tmp_dir/$archive" || fail "download failed - is there a release published yet?"
else
  fail "neither curl nor wget is available"
fi

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir" "$BIN_NAME"
chmod +x "$tmp_dir/$BIN_NAME"
mv "$tmp_dir/$BIN_NAME" "$install_dir/$BIN_NAME"

echo "getsloth-install: installed to $install_dir/$BIN_NAME"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    echo "getsloth-install: $install_dir is not on your PATH."
    echo "  Add this to your shell profile:"
    echo "    export PATH=\"$install_dir:\$PATH\""
    ;;
esac

echo "getsloth-install: run '$BIN_NAME --version' to confirm, or alias it: alias sloth=$BIN_NAME"
