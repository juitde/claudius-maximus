#!/bin/sh
# Installs claudius-maximus: detects OS/arch, downloads the matching release
# archive, verifies it against that release's checksums.txt, and installs the
# binary. See RELEASING.md and README.md for the reasoning behind the pinned
# commit this script is meant to be fetched at, rather than `main` or a tag.
#
# Usage:
#   curl -fsSL <pinned-url> | sh
#   curl -fsSL <pinned-url> | sh -s -- --version v0.1.0 --dir ~/bin
#   ./install.sh --version v0.1.0 --dir ~/bin
set -eu

REPO="juitde/claudius-maximus"
BINARY="claudius-maximus"

usage() {
  cat <<EOF
Usage: install.sh [--version vX.Y.Z] [--dir DIRECTORY] [--postinstall]

  --version      Version to install (default: the latest release)
  --dir          Directory to install into (default: platform-specific)
  --postinstall  Also run "<binary> install" to register it with Claude Code
                 (default: print a hint instead - see issue #14)
  -h, --help     Show this help
EOF
}

err() {
  echo "install.sh: $*" >&2
  exit 1
}

# --- Refuse under WSL, on purpose ------------------------------------------
# A Linux binary running inside WSL raises ambiguous questions (which PATH,
# which filesystem, whether the Windows-native tool was actually wanted) this
# project has not tested against. install.ps1 covers native Windows instead;
# see the design discussion linked from issue #14.
is_wsl=0
if [ -r /proc/version ] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
  is_wsl=1
fi
if [ -n "${WSL_DISTRO_NAME:-}" ]; then
  is_wsl=1
fi
if [ "$is_wsl" = 1 ]; then
  err "this looks like WSL. Use install.ps1 from a native Windows PowerShell prompt instead - installing the Linux binary inside WSL is not a supported combination."
fi

command -v curl >/dev/null 2>&1 || err "curl is required but was not found"

# --- Argument parsing -------------------------------------------------------
VERSION=""
DIR=""
POSTINSTALL=0
while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || err "--version needs a value"
      VERSION="$2"
      shift 2
      ;;
    --version=*)
      VERSION="${1#--version=}"
      shift
      ;;
    --dir)
      [ $# -ge 2 ] || err "--dir needs a value"
      DIR="$2"
      shift 2
      ;;
    --dir=*)
      DIR="${1#--dir=}"
      shift
      ;;
    --postinstall)
      POSTINSTALL=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      err "unrecognized argument: $1 (see --help)"
      ;;
  esac
done

# --- OS/arch detection -------------------------------------------------------
os="$(uname -s)"
case "$os" in
  Darwin) goos="darwin" ;;
  Linux) goos="linux" ;;
  *) err "unsupported OS: $os (this script supports macOS and Linux only; see install.ps1 for Windows)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) goarch="amd64" ;;
  arm64 | aarch64) goarch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- Default install directory, OS-aware -------------------------------------
if [ -z "$DIR" ]; then
  case "$goos" in
    darwin) DIR="/usr/local/bin" ;;
    linux) DIR="$HOME/.local/bin" ;;
  esac
fi

# A leading ~ is a shell convention, not something the shell itself expands
# once it has passed through as a literal argument (e.g. --dir=~/bin, or any
# quoted form) - expand it by hand the same way an interactive shell would.
# shellcheck disable=SC2088 # intentional: matching a literal ~, not expanding one
case "$DIR" in
  "~") DIR="$HOME" ;;
  "~/"*) DIR="$HOME/${DIR#\~/}" ;;
esac

# A relative --dir would otherwise be created relative to wherever this
# script happens to be invoked from, and neither the final "installed to"
# message nor the PATH check below would mean much against a relative path.
case "$DIR" in
  /*) ;;
  *) DIR="$(pwd)/$DIR" ;;
esac

# Strip a trailing slash so the PATH check below (which compares $DIR against
# colon-separated $PATH entries verbatim) isn't defeated by e.g. --dir ~/bin/
# already being on PATH as ~/bin.
DIR="${DIR%/}"
[ -n "$DIR" ] || DIR="/"

# --- Resolve the version ------------------------------------------------------
# Following the releases/latest redirect, rather than parsing the GitHub API's
# JSON, avoids both a jq dependency this script doesn't otherwise need and
# the unauthenticated API's low per-IP rate limit.
if [ -z "$VERSION" ]; then
  # Captured directly (not piped into sed) so curl's own exit status - not
  # sed's, which would always succeed even on curl failure without pipefail,
  # a `set -o` dash lacks - is what triggers the error below.
  url_effective="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")" \
    || err "failed to reach GitHub to determine the latest release version"
  VERSION="$(printf '%s' "$url_effective" | sed -E 's#.*/tag/##')"
  [ -n "$VERSION" ] || err "could not determine the latest release version"
fi

archive="${BINARY}_${goos}_${goarch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$VERSION"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

echo "Downloading $archive ($VERSION)..."
curl -fsSL -o "$workdir/$archive" "$base_url/$archive" \
  || err "failed to download $archive - does $VERSION exist for $goos/$goarch?"
curl -fsSL -o "$workdir/checksums.txt" "$base_url/checksums.txt" \
  || err "failed to download checksums.txt"

echo "Verifying checksum..."
# awk compares the filename field for an exact match, rather than grep with a
# pattern - regex or literal - that a filename sharing a prefix with $archive
# (e.g. a future .sig/SBOM entry) could also match.
expected="$(awk -v f="$archive" '$2 == f { print $1 }' "$workdir/checksums.txt")"
[ -n "$expected" ] || err "no checksum entry found for $archive in checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$workdir/$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$workdir/$archive" | awk '{print $1}')"
else
  err "neither sha256sum nor shasum is available to verify the download"
fi

[ "$expected" = "$actual" ] \
  || err "checksum mismatch for $archive (expected $expected, got $actual) - refusing to install"

echo "Extracting..."
tar -xzf "$workdir/$archive" -C "$workdir" "$BINARY"

mkdir -p "$DIR"
install_path="$DIR/$BINARY"
mv "$workdir/$BINARY" "$install_path"
chmod +x "$install_path"

echo "Installed $BINARY $VERSION to $install_path"

# --- PATH check --------------------------------------------------------------
case ":$PATH:" in
  *":$DIR:"*) ;;
  *)
    echo ""
    echo "Warning: $DIR is not in your PATH."
    echo "Add this to your shell profile (e.g. ~/.bashrc, ~/.zshrc):"
    echo ""
    echo "  export PATH=\"$DIR:\$PATH\""
    echo ""
    ;;
esac

# --- Postinstall registration -------------------------------------------------
# Placing the binary and registering it with Claude Code are two different
# actions with two different blast radii (the second mutates the caller's
# Claude Code configuration and requires `claude` on PATH) - opt-in via
# --postinstall, not automatic. See issue #14.
if [ "$POSTINSTALL" = 1 ]; then
  echo ""
  echo "Running \"$install_path\" install..."
  "$install_path" install \
    || err "postinstall failed - $BINARY is installed at $install_path, but registering it with Claude Code did not succeed. Run \"$install_path install\" yourself to retry."
else
  echo ""
  echo "Next: run \"$install_path\" install to register $BINARY with Claude Code."
fi
