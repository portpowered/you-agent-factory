#!/usr/bin/env bash
set -euo pipefail

command_name="${OMNIVOICE_COMMAND_NAME:-omnivoice-llamacpp}"
command_url="${OMNIVOICE_COMMAND_URL:-}"
install_dir="${OMNIVOICE_COMMAND_INSTALL_DIR:-$(pwd)/.cache/omnivoice-command/bin}"
extract_dir="${OMNIVOICE_COMMAND_EXTRACT_DIR:-$(pwd)/.cache/omnivoice-command/extract}"

if [ -z "$command_url" ]; then
  echo "OMNIVOICE_COMMAND_URL is required so CI can install ${command_name} on $(uname -s)/$(uname -m)." >&2
  echo "Configure the repository variable for this runner in .github/workflows/long-local-inference.yml." >&2
  exit 1
fi

mkdir -p "$install_dir" "$extract_dir"
target_path="${install_dir}/${command_name}"

if [ -x "$target_path" ]; then
  echo "Reusing cached ${command_name} at ${target_path}" >&2
else
  archive_path="${extract_dir}/$(basename "${command_url}")"
  rm -rf "${extract_dir:?}/payload"
  mkdir -p "${extract_dir}/payload"
  echo "Downloading ${command_name} from ${command_url}" >&2
  curl -fsSL "$command_url" -o "$archive_path"

  case "$archive_path" in
    *.tar.gz|*.tgz)
      tar -xzf "$archive_path" -C "${extract_dir}/payload"
      ;;
    *.zip)
      unzip -q "$archive_path" -d "${extract_dir}/payload"
      ;;
    *)
      cp "$archive_path" "$target_path"
      chmod +x "$target_path"
      ;;
  esac

  if [ ! -x "$target_path" ]; then
    candidate="$(find "${extract_dir}/payload" -type f -name "$command_name" | head -n 1 || true)"
    if [ -z "$candidate" ]; then
      echo "Downloaded payload from ${command_url} did not contain ${command_name}" >&2
      find "${extract_dir}/payload" -maxdepth 3 -type f >&2 || true
      exit 1
    fi
    cp "$candidate" "$target_path"
    chmod +x "$target_path"
  fi
fi

echo "command_path=$target_path" >> "$GITHUB_OUTPUT"
echo "$install_dir" >> "$GITHUB_PATH"
