#!/usr/bin/env sh
set -eu

BINARY_NAME="agent-factory"
RELEASE_BASE_URL="${AGENT_FACTORY_INSTALL_BASE_URL:-https://github.com/portpowered/infinite-you/releases}"
INSTALL_DIR="${AGENT_FACTORY_INSTALL_DIR:-$HOME/.local/bin}"
VERSION_OVERRIDE="${AGENT_FACTORY_VERSION:-}"
OS_OVERRIDE="${AGENT_FACTORY_INSTALL_OS:-}"
ARCH_OVERRIDE="${AGENT_FACTORY_INSTALL_ARCH:-}"

say() {
  printf '%s\n' "$*"
}

fail() {
  printf 'agent-factory install: %s\n' "$*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

require_command() {
  if ! command_exists "$1"; then
    fail "missing required tool '$1'"
  fi
}

normalize_os() {
  value="$1"
  case "$value" in
    linux|darwin)
      printf '%s\n' "$value"
      ;;
    *)
      fail "unsupported operating system '$value'; supported values are linux and darwin"
      ;;
  esac
}

detect_os() {
  if [ -n "$OS_OVERRIDE" ]; then
    normalize_os "$OS_OVERRIDE"
    return
  fi

  case "$(uname -s)" in
    Linux)
      printf 'linux\n'
      ;;
    Darwin)
      printf 'darwin\n'
      ;;
    *)
      fail "unsupported operating system '$(uname -s)'; supported platforms are macOS and Linux"
      ;;
  esac
}

normalize_arch() {
  value="$1"
  case "$value" in
    x86_64|amd64)
      printf 'amd64\n'
      ;;
    arm64|aarch64)
      printf 'arm64\n'
      ;;
    *)
      fail "unsupported architecture '$value'; supported values are amd64 and arm64"
      ;;
  esac
}

detect_arch() {
  if [ -n "$ARCH_OVERRIDE" ]; then
    normalize_arch "$ARCH_OVERRIDE"
    return
  fi

  normalize_arch "$(uname -m)"
}

resolve_tag() {
  if [ -n "$VERSION_OVERRIDE" ]; then
    case "$VERSION_OVERRIDE" in
      v*)
        printf '%s\n' "$VERSION_OVERRIDE"
        ;;
      *)
        printf 'v%s\n' "$VERSION_OVERRIDE"
        ;;
    esac
    return
  fi

  require_command curl
  effective_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$RELEASE_BASE_URL/latest")" ||
    fail "failed to resolve the latest agent-factory release from $RELEASE_BASE_URL/latest"

  tag="${effective_url##*/}"
  case "$tag" in
    v*)
      printf '%s\n' "$tag"
      ;;
    *)
      fail "could not determine the latest release tag from $effective_url"
      ;;
  esac
}

download_to() {
  url="$1"
  destination="$2"
  require_command curl
  if ! curl -fsSL "$url" -o "$destination"; then
    fail "failed to download $url"
  fi
}

sha256_file() {
  file_path="$1"
  if command_exists sha256sum; then
    sha256sum "$file_path" | awk '{print $1}'
    return
  fi
  if command_exists shasum; then
    shasum -a 256 "$file_path" | awk '{print $1}'
    return
  fi
  fail "missing required tool 'sha256sum' or 'shasum'"
}

verify_checksum() {
  archive_path="$1"
  checksum_path="$2"
  archive_name="$3"

  expected="$(awk -v name="$archive_name" '$2 == name { print $1; exit }' "$checksum_path")"
  if [ -z "$expected" ]; then
    fail "checksum entry for $archive_name was not found in $(basename "$checksum_path")"
  fi

  actual="$(sha256_file "$archive_path")"
  if [ "$expected" != "$actual" ]; then
    fail "checksum mismatch for $archive_name"
  fi
}

install_binary() {
  source_path="$1"
  target_path="$2"

  target_dir="$(dirname "$target_path")"
  if ! mkdir -p "$target_dir"; then
    fail "could not create install directory $target_dir; set AGENT_FACTORY_INSTALL_DIR to a writable path"
  fi

  if command_exists install; then
    if ! install -m 0755 "$source_path" "$target_path"; then
      fail "could not install $BINARY_NAME to $target_path; set AGENT_FACTORY_INSTALL_DIR to a writable path"
    fi
    return
  fi

  if ! cp "$source_path" "$target_path"; then
    fail "could not copy $BINARY_NAME to $target_path; set AGENT_FACTORY_INSTALL_DIR to a writable path"
  fi
  if ! chmod 0755 "$target_path"; then
    fail "installed $BINARY_NAME but could not mark it executable at $target_path"
  fi
}

maybe_handle_macos_quarantine() {
  os_name="$1"
  binary_path="$2"

  if [ "$os_name" != "darwin" ]; then
    return
  fi

  if command_exists xattr; then
    if xattr -dr com.apple.quarantine "$binary_path" >/dev/null 2>&1; then
      say "Removed macOS quarantine attributes from $binary_path."
      return
    fi
  fi

  say "If macOS blocks launch, run: xattr -dr com.apple.quarantine \"$binary_path\""
}

path_contains_dir() {
  target_dir="$1"
  old_ifs=$IFS
  IFS=:
  for path_dir in $PATH; do
    if [ "$path_dir" = "$target_dir" ]; then
      IFS=$old_ifs
      return 0
    fi
  done
  IFS=$old_ifs
  return 1
}

main() {
  os_name="$(detect_os)"
  arch_name="$(detect_arch)"
  tag="$(resolve_tag)"
  version="${tag#v}"
  archive_name="${BINARY_NAME}_${version}_${os_name}_${arch_name}.tar.gz"
  checksum_name="${BINARY_NAME}_${version}_checksums.txt"

  require_command tar
  require_command mktemp

  tmp_dir="$(mktemp -d 2>/dev/null || mktemp -d -t agent-factory-install)"
  trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

  archive_path="$tmp_dir/$archive_name"
  checksum_path="$tmp_dir/$checksum_name"
  extract_dir="$tmp_dir/extracted"
  binary_path="$INSTALL_DIR/$BINARY_NAME"

  say "Downloading $archive_name from $RELEASE_BASE_URL/download/$tag/."
  download_to "$RELEASE_BASE_URL/download/$tag/$archive_name" "$archive_path"
  download_to "$RELEASE_BASE_URL/download/$tag/$checksum_name" "$checksum_path"

  verify_checksum "$archive_path" "$checksum_path" "$archive_name"

  if ! mkdir -p "$extract_dir"; then
    fail "could not create temporary extraction directory"
  fi
  if ! tar -xzf "$archive_path" -C "$extract_dir"; then
    fail "failed to extract $archive_name"
  fi
  if [ ! -f "$extract_dir/$BINARY_NAME" ]; then
    fail "archive $archive_name did not contain $BINARY_NAME"
  fi

  install_binary "$extract_dir/$BINARY_NAME" "$binary_path"
  maybe_handle_macos_quarantine "$os_name" "$binary_path"

  say "Installed $BINARY_NAME $tag to $binary_path"
  if path_contains_dir "$INSTALL_DIR"; then
    say "Run '$BINARY_NAME --help' to get started."
    return
  fi

  say "Add it to your PATH with:"
  say "  export PATH=\"$INSTALL_DIR:\$PATH\""
}

main "$@"
