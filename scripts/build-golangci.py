"""Build the supported custom binary and retain its actual host for deadcode."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/portpowered/infinite-you"
SOURCE_FIELDS = ("GoFiles", "CgoFiles", "CFiles", "CXXFiles", "MFiles", "HFiles",
                 "FFiles", "SFiles", "SwigFiles", "SwigCXXFiles", "SysoFiles", "EmbedFiles")


def run(command, cwd):
    return subprocess.check_output(command, cwd=cwd, text=True, encoding="utf-8")


def json_stream(value):
    decoder = json.JSONDecoder()
    while value.strip():
        item, end = decoder.raw_decode(value.lstrip())
        yield item
        value = value.lstrip()[end:]


def cache_key(root=ROOT, runner=run):
    """Hash compiler-selected inputs, not a repository file inventory."""
    environment = runner(["go", "env", "-json", "GOVERSION", "GOOS", "GOARCH",
                          "CGO_ENABLED", "GOEXPERIMENT", "GOFLAGS", "GOTOOLCHAIN"], root)
    digest = hashlib.sha256(environment.encode())
    for name in ("go.mod", "go.sum", ".custom-gcl.yml", "scripts/build-golangci.py"):
        digest.update(name.encode() + b"\0" + (root / name).read_bytes())
    packages = list(json_stream(runner(
        ["go", "list", "-deps", "-json", "./tools/golangcilintplugin"], root)))
    for package in sorted(packages, key=lambda item: item["ImportPath"]):
        if package.get("Error") or package.get("DepsErrors") or package.get("Incomplete"):
            raise ValueError("incomplete plugin compiler metadata")
        digest.update(package["ImportPath"].encode() + b"\0")
        if package.get("Standard"):
            continue  # The selected toolchain identifies its standard library.
        directory = Path(package["Dir"])
        for name in sorted({name for field in SOURCE_FIELDS for name in package.get(field, [])}):
            digest.update(name.encode() + b"\0" + (directory / name).read_bytes())
    if not any(package["ImportPath"] == MODULE + "/tools/golangcilintplugin" for package in packages):
        raise ValueError("compiler metadata is missing the plugin")
    return digest.hexdigest()


def file_digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def host_digest(host):
    # Inventory only the disposable generated host, never repository sources.
    digest = hashlib.sha256()
    for path in sorted(host.rglob("*")):
        if path.is_symlink():
            raise ValueError("cached host must not contain symlinks")
        if path.is_file():
            digest.update(path.relative_to(host).as_posix().encode() + b"\0")
            digest.update(path.read_bytes())
    return digest.hexdigest()


def copy_host(source, destination):
    def ignored(directory, names):
        excluded = {".git"}
        if Path(directory) == source / "test" / "testdata":
            # The upstream linter's symlink-loop fixture is not a host input.
            excluded.add("symlink_loop")
        return excluded.intersection(names)

    # Preserve unexpected links so validation rejects them without following.
    shutil.copytree(source, destination, symlinks=True, ignore=ignored)


def activate_host(destination, root=ROOT, runner=run):
    # Keep the cached host pristine so repeated relocations validate identically.
    with tempfile.TemporaryDirectory(prefix="golangci-host-") as temporary:
        active = destination / "current-host"
        staging = Path(temporary) / "host"
        shutil.copytree(destination / "host", staging)
        runner(["go", "mod", "edit", "-replace", MODULE + "=" + str(root.resolve())], staging)
        metadata = json.loads(runner(["go", "mod", "edit", "-json"], staging))
        if metadata.get("Module", {}).get("Path") != "github.com/golangci/golangci-lint/v2":
            raise ValueError("cached host is not the golangci module")
        replacements = metadata.get("Replace") or []
        if not any(item["Old"]["Path"] == MODULE and
                   Path(item["New"]["Path"]).resolve() == root.resolve() for item in replacements):
            raise ValueError("cached host does not select this checkout")
        # This is a fixed disposable artifact path owned by the build helper.
        if active.exists():
            if active.is_symlink() or active.resolve().parent != destination.resolve():
                raise ValueError("active host escapes the artifact directory")
            shutil.rmtree(active)
        shutil.copytree(staging, active)
    (destination / "host-path.txt").write_text(str(active.resolve()) + "\n", encoding="utf-8")


def restore(destination, root=ROOT, runner=run):
    pointer = destination / "host-path.txt"
    pointer.unlink(missing_ok=True)
    metadata = json.loads((destination / "cache.json").read_text(encoding="utf-8"))
    if not (destination / "host/go.mod").is_file() or not (destination / "host/go.sum").is_file():
        raise ValueError("cached generated host is missing its module inputs")
    if metadata["key"] != cache_key(root, runner):
        raise ValueError("cached plugin compiler inputs changed")
    binary_name = "golangci-repository.exe" if os.name == "nt" else "golangci-repository"
    binary = destination / binary_name
    if metadata["binary"] != file_digest(binary) or metadata["host"] != host_digest(destination / "host"):
        raise ValueError("cached plugin or generated host is corrupt")
    version = runner([str(binary.resolve()), "version"], root)
    if "version v2.11.4-custom-gcl-" not in version:
        raise ValueError("cached binary is not the pinned custom golangci")
    activate_host(destination, root, runner)
    print("Validated exact plugin cache hit; custom build skipped", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--destination")
    parser.add_argument("--cache-key", action="store_true")
    parser.add_argument("--restore", action="store_true")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    if args.cache_key:
        print(cache_key())
        return 0
    if not args.destination:
        parser.error("--destination is required for build/restore")
    destination = Path(args.destination).resolve()
    if args.restore:
        restore(destination)
        return 0
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        parser.error("a golangci custom build command is required")

    destination.mkdir(parents=True, exist_ok=True)
    pointer = destination / "host-path.txt"
    pointer.unlink(missing_ok=True)
    build_key = cache_key()
    # Versioned go run can select a newer toolchain than this module selected.
    # Compile the host with the same exact compiler identified by cache_key.
    compiler = run(["go", "env", "GOVERSION"], ROOT).strip()
    environment = dict(os.environ, CUSTOM_GCL_KEEP_TEMP_FILES="1", GOTOOLCHAIN=compiler)
    marker = "the temporary directory is preserved: "
    host = None
    with subprocess.Popen(
        command, env=environment, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, text=True, encoding="utf-8",
    ) as process:
        for line in process.stdout:
            print(line, end="", flush=True)
            if marker in line:
                host = Path(line.split(marker, 1)[1].strip()) / "golangci-lint"
        status = process.wait()
    if status:
        return status
    if host is None or not (host / "go.mod").is_file():
        raise RuntimeError("custom build did not retain its generated golangci host")
    cached_host = destination / "host"
    if cached_host.exists():
        if cached_host.is_symlink() or cached_host.resolve().parent != destination.resolve():
            raise ValueError("cached host escapes the artifact directory")
        shutil.rmtree(cached_host)
    copy_host(host, cached_host)
    binary = destination / ("golangci-repository.exe" if os.name == "nt" else "golangci-repository")
    if cache_key() != build_key:
        raise ValueError("plugin compiler inputs changed during the custom build")
    metadata = {"key": build_key, "binary": file_digest(binary), "host": host_digest(cached_host)}
    (destination / "cache.json").write_text(json.dumps(metadata) + "\n", encoding="utf-8")
    activate_host(destination)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
