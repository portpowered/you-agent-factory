#!/usr/bin/env bash
set -euo pipefail

command_name="${OMNIVOICE_COMMAND_NAME:-omnivoice-llamacpp}"
command_url="${OMNIVOICE_COMMAND_URL:-}"
install_dir="${OMNIVOICE_COMMAND_INSTALL_DIR:-$(pwd)/.cache/omnivoice-command/bin}"
extract_dir="${OMNIVOICE_COMMAND_EXTRACT_DIR:-$(pwd)/.cache/omnivoice-command/extract}"
source_dir="${OMNIVOICE_COMMAND_SOURCE_DIR:-$(pwd)/.cache/omnivoice-command/src/omnivoice.cpp}"
source_repo="${OMNIVOICE_CPP_SOURCE_REPO:-https://github.com/ServeurpersoCom/omnivoice.cpp.git}"
source_ref="${OMNIVOICE_CPP_SOURCE_REF:-5dff3f17a3e0a73353d8bea35e0fa322fc6dcfdf}"
backend_name="${OMNIVOICE_TTS_COMMAND_NAME:-omnivoice-tts}"

mkdir -p "$install_dir" "$extract_dir"
target_path="${install_dir}/${command_name}"
backend_target_path="${install_dir}/${backend_name}"

emit_outputs() {
  local command_path="$1"
  local skipped="$2"
  local reason="$3"
  {
    echo "command_path=$command_path"
    echo "skipped=$skipped"
    echo "skip_reason=$reason"
  } >> "$GITHUB_OUTPUT"
}

if [ -x "$target_path" ] && [ -x "$backend_target_path" ]; then
  echo "Reusing cached ${command_name} at ${target_path}" >&2
  emit_outputs "$target_path" "false" ""
else
  build_adapter() {
    echo "Building ${command_name} adapter" >&2
    go build -o "$target_path" ./cmd/omnivoice-llamacpp
    chmod +x "$target_path"
  }

  detect_built_backend() {
    local candidate=""
    if [ -x "$backend_target_path" ]; then
      echo "$backend_target_path"
      return 0
    fi
    if candidate="$(command -v "$backend_name" 2>/dev/null)"; then
      echo "$candidate"
      return 0
    fi
    return 1
  }

  copy_backend_candidate() {
    local candidate="$1"
    cp "$candidate" "$backend_target_path"
    chmod +x "$backend_target_path"
  }

  download_backend_from_url() {
    local archive_path="${extract_dir}/$(basename "${command_url}")"
    rm -rf "${extract_dir:?}/payload"
    mkdir -p "${extract_dir}/payload"
    echo "Downloading ${backend_name} payload from ${command_url}" >&2
    curl -fsSL "$command_url" -o "$archive_path"

    case "$archive_path" in
      *.tar.gz|*.tgz)
        tar -xzf "$archive_path" -C "${extract_dir}/payload"
        ;;
      *.zip)
        unzip -q "$archive_path" -d "${extract_dir}/payload"
        ;;
      *)
        cp "$archive_path" "$backend_target_path"
        chmod +x "$backend_target_path"
        return 0
        ;;
    esac

    local candidate
    candidate="$(find "${extract_dir}/payload" -type f -name "$backend_name" | head -n 1 || true)"
    if [ -z "$candidate" ]; then
      echo "Downloaded payload from ${command_url} did not contain ${backend_name}" >&2
      find "${extract_dir}/payload" -maxdepth 3 -type f >&2 || true
      exit 1
    fi
    copy_backend_candidate "$candidate"
  }

  cpu_count() {
    getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4
  }

  clone_source_repo() {
    if [ -d "${source_dir}/.git" ]; then
      return 0
    fi
    rm -rf "$source_dir"
    mkdir -p "$(dirname "$source_dir")"
    git clone --branch master --depth 1 --recurse-submodules --shallow-submodules "$source_repo" "$source_dir"
  }

  build_backend_from_source() {
    clone_source_repo
    (
      cd "$source_dir"
      git fetch --depth 1 origin "$source_ref"
      git checkout --force "$source_ref"
      git submodule sync --recursive
      git submodule update --init --recursive --depth 1
      rm -rf build
      # Keep cached backends portable across GitHub-hosted runners instead of
      # embedding host-native CPU tuning into artifacts that may restore on a
      # different machine generation later.
      cmake -B build -DGGML_NATIVE=OFF
      cmake --build build --config Release -j "$(cpu_count)"
    )
    local candidate
    candidate="$(find "$source_dir/build" -type f -name "$backend_name" | head -n 1 || true)"
    if [ -z "$candidate" ]; then
      echo "Built source tree did not produce ${backend_name}" >&2
      find "$source_dir/build" -maxdepth 3 -type f >&2 || true
      exit 1
    fi
    copy_backend_candidate "$candidate"
  }

  if ! detect_built_backend >/dev/null 2>&1; then
    if [ -n "$command_url" ]; then
      download_backend_from_url
    else
      echo "OMNIVOICE_COMMAND_URL is not configured for $(uname -s)/$(uname -m); building real ${backend_name} from pinned ${source_repo}@${source_ref}" >&2
      build_backend_from_source
    fi
  fi

  build_adapter
  emit_outputs "$target_path" "false" ""
fi

echo "$install_dir" >> "$GITHUB_PATH"
