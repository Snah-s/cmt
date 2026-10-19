#!/bin/sh
# Install cmt from its GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/Snah-s/cmt/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/Snah-s/cmt/main/install.sh | sh -s -- --version 1.0.0 --prefix /usr/local
#   curl -fsSL https://raw.githubusercontent.com/Snah-s/cmt/main/install.sh | sh -s -- --uninstall
#
# Installs PREFIX/bin/cmt and PREFIX/share/man/man1/cmt.1 (default PREFIX: ~/.local).
set -eu

releases=${CMT_RELEASES_URL:-https://github.com/Snah-s/cmt/releases}

say() { printf 'cmt-install: %s\n' "$*" >&2; }
die() { say "error: $*"; exit 1; }

usage() {
  cat >&2 <<'EOF'
Usage: install.sh [--version X.Y.Z] [--prefix DIR] [--uninstall]

  --version X.Y.Z  Install that release instead of the latest
  --prefix DIR     Install into DIR/bin and DIR/share/man/man1 (default: ~/.local)
  --uninstall      Remove cmt and its man page from DIR instead
EOF
}

fetch() { # fetch URL FILE
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --proto '=https,file' -o "$2" "$1"
  else
    wget -q -O "$2" "$1"
  fi
}

latest_version() {
  # The /latest page redirects to /tag/vX.Y.Z; reading the redirect avoids the rate-limited API.
  if command -v curl >/dev/null 2>&1; then
    url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$releases/latest")
  else
    url=$(wget -S --spider "$releases/latest" 2>&1 | sed -n 's/^ *Location: \([^ ]*\).*/\1/p' | tail -n 1)
  fi
  tag=${url##*/tag/}
  case "$tag" in v[0-9]*) printf '%s\n' "${tag#v}" ;; *) die "could not find the latest release at $releases" ;; esac
}

target() {
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "$(uname -s) is not supported yet; see $releases" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "$(uname -m) is not supported yet; see $releases" ;;
  esac
  # A shell under Rosetta reports x86_64 on Apple silicon; prefer the native build.
  if [ "$os/$arch" = darwin/amd64 ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || true)" = 1 ]; then
    arch=arm64
  fi
  if [ "$os/$arch" = darwin/amd64 ]; then
    die "Intel Macs are not supported yet; see $releases"
  fi
  printf '%s_%s\n' "$os" "$arch"
}

verify() { # verify DIR FILE.sha256
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$1" && sha256sum -c "$2" >/dev/null)
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$1" && shasum -a 256 -c "$2" >/dev/null)
  else
    die "sha256sum or shasum is required to verify the download"
  fi
}

uninstall() { # uninstall BIN MAN
  if [ ! -e "$1/cmt" ] && [ ! -e "$2/cmt.1" ]; then
    say "cmt is not installed in ${1%/bin}"
    return
  fi
  # Managed hooks exec this path, so git commit fails where they remain.
  say "repositories where you ran 'cmt init' keep a hook that calls $1/cmt;"
  say "run 'cmt remove' in them, or git commit will fail there"
  rm -f "$1/cmt" "$2/cmt.1"
  say "removed $1/cmt and $2/cmt.1"
  say "preferences and the catalog cache are kept; delete them by hand if you want:"
  say "  the cmt folder in your user configuration directory, and ~/.cmt"
}

main() {
  version='' prefix="${HOME:?HOME is not set}/.local" remove=false
  while [ $# -gt 0 ]; do
    case "$1" in
      --version) [ $# -ge 2 ] || die "--version needs a value"; version=${2#v}; shift 2 ;;
      --prefix) [ $# -ge 2 ] || die "--prefix needs a value"; prefix=$2; shift 2 ;;
      --uninstall) remove=true; shift ;;
      -h | --help) usage; exit 0 ;;
      *) usage; die "unknown option: $1" ;;
    esac
  done
  if [ "$remove" = true ]; then
    uninstall "$prefix/bin" "$prefix/share/man/man1"
    return
  fi
  command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || die "curl or wget is required"
  command -v tar >/dev/null 2>&1 || die "tar is required"

  platform=$(target)
  [ -n "$version" ] || version=$(latest_version)
  case "$version" in [0-9]*) ;; *) die "invalid version: $version" ;; esac
  case "$version" in *[!A-Za-z0-9._-]*) die "invalid version: $version" ;; esac
  name="cmt_${version}_${platform}"

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT HUP INT TERM
  say "downloading cmt $version for ${platform%_*}/${platform#*_}"
  fetch "$releases/download/v$version/$name.tar.gz" "$tmp/$name.tar.gz" || die "download failed; does release v$version exist?"
  fetch "$releases/download/v$version/$name.tar.gz.sha256" "$tmp/$name.tar.gz.sha256" || die "checksum download failed"
  verify "$tmp" "$name.tar.gz.sha256" || die "checksum mismatch; nothing was installed"
  tar -xzf "$tmp/$name.tar.gz" -C "$tmp"

  bin="$prefix/bin" man="$prefix/share/man/man1"
  previous=$("$bin/cmt" --version 2>/dev/null | cut -d' ' -f2 || true)
  mkdir -p "$bin" "$man"
  # Copy beside the target and rename, so a running cmt is never left half-written.
  cp "$tmp/$name/cmt" "$bin/.cmt.new"
  chmod 755 "$bin/.cmt.new"
  mv -f "$bin/.cmt.new" "$bin/cmt"
  cp "$tmp/$name/cmt.1" "$man/cmt.1"

  if [ "$previous" = "$version" ]; then
    say "reinstalled $bin/cmt $version"
  elif [ -n "$previous" ]; then
    say "updated $bin/cmt from $previous to $version"
  else
    say "installed $bin/cmt $version"
  fi
  case ":$PATH:" in
    *":$bin:"*) ;;
    *) say "add $bin to your PATH, for example: export PATH=\"$bin:\$PATH\"" ;;
  esac
  found=$(command -v cmt 2>/dev/null || true)
  if [ -n "$found" ] && [ "$found" != "$bin/cmt" ]; then
    say "warning: $found comes first in your PATH and hides this installation"
  fi
}

# Everything runs from here, so a download cut short by `curl | sh` executes nothing.
main "$@"
