"""Adapt upstream production deadcode/compiler reports to the exact baseline.

No source discovery or reachability policy lives here. Both production module
graphs are analyzed by the supplied pinned upstream command; go list owns paths.
"""

import argparse
import json
import os
from pathlib import Path
import posixpath
import re
import subprocess
import sys

MODULE = "github.com/portpowered/infinite-you"
BASELINE = "docs/internal/baselines/deadcode-baseline.txt"
CURRENT = "bin/deadcode-current.txt"
HOST_POINTER = ".artifacts/golangci/host-path.txt"
SUFFIXES = (
    "windows linux darwin freebsd netbsd openbsd dragonfly solaris aix illumos "
    "android ios plan9 js wasip1 unix amd64 386 arm arm64 riscv64 ppc64 ppc64le "
    "s390x loong64 mips mipsle mips64 mips64le wasm"
).split()


def normalize(report):
    report = report.replace("\r\n", "\n").replace("\r", "\n").replace("\\", "/").strip()
    if not report:
        return ""
    lines = []
    for line in report.split("\n"):
        line = re.sub(r":([0-9]+):([0-9]+):", ":", line.strip())
        source, separator, _ = line.partition(".go:")
        if separator and (source.startswith("third_party/acp-go-sdk/") or
                          any(posixpath.basename(source).endswith("_" + suffix)
                              for suffix in SUFFIXES)):
            continue
        lines.append(line)
    return "\n".join(sorted(lines)) + "\n" if lines else ""


def count(report):
    return len(report.strip().split("\n")) if report.strip() else 0


def relative_source(source, directory, root):
    source = Path(source)
    if not source.is_absolute():
        source = directory / source
    # Lexical containment matches the previous compiler-report adapter.
    try:
        return Path(os.path.abspath(source)).relative_to(root).as_posix()
    except ValueError as error:
        raise ValueError(f"deadcode source/package outside repository: {source}") from error


def repository_positions(report, host, root):
    lines = []
    for line in report.strip().split("\n"):
        if not line:
            continue
        source, separator, diagnostic = line.partition(".go:")
        if not separator:
            raise ValueError(f"malformed deadcode finding: {line!r}")
        lines.append(relative_source(source + ".go", host, root) + ":" + diagnostic)
    return "\n".join(lines)


def reconcile(repository, host, packages):
    host_dead = set(normalize(host).strip().splitlines())
    return "".join(line + "\n" for line in normalize(repository).strip().splitlines()
                   if posixpath.dirname(line.partition(".go:")[0] + ".go") not in packages
                   or line in host_dead)


def production_environment(environment, version):
    result = dict(environment)
    parts = ["gotypesalias=1" if part.partition("=")[0] == "gotypesalias" else part
             for part in result.get("GODEBUG", "").split(",") if part]
    if "gotypesalias=1" not in parts:
        parts.append("gotypesalias=1")
    result.update(GODEBUG=",".join(parts), GOWORK="off", GOTOOLCHAIN=version + "+auto")
    return result


def command(arguments, directory, environment, stderr, executor):
    result = executor(arguments, cwd=directory, env=environment,
                      text=True, capture_output=True)
    if result.returncode:
        raise ValueError(f"command {arguments!r} in {directory} failed "
                         f"(exit {result.returncode})\n{result.stderr}")
    if result.stderr:
        stderr.write(result.stderr)
    return result.stdout


def validate_host(metadata, host, root):
    if metadata.get("Module", {}).get("Path") != "github.com/golangci/golangci-lint/v2":
        raise ValueError("generated host is not the golangci module")
    for replacement in metadata.get("Replace", []) or []:
        old, new = replacement["Old"], replacement["New"]
        if old["Path"] != MODULE or old.get("Version") or new.get("Version"):
            continue
        source = Path(new["Path"])
        if not source.is_absolute():
            source = host / source
        try:
            if source.samefile(root):
                return
        except OSError:
            pass
    raise ValueError(f"generated golangci host must replace {MODULE} with this checkout")


def collect(root, pointer, upstream, environment, stderr, executor):
    try:
        host = Path((root / pointer).read_text(encoding="utf-8").strip())
    except OSError as error:
        raise ValueError(f"read generated golangci host (run make golangci-build): {error}") from error
    if not host.is_absolute():
        raise ValueError(f"generated golangci host path must be absolute: {host}")
    version = command([upstream[0], "env", "GOVERSION"], root, environment, stderr, executor).strip()
    environment = production_environment(environment, version)
    metadata = command([upstream[0], "mod", "edit", "-json"], host, environment, stderr, executor)
    validate_host(json.loads(metadata), host, root)
    filter_arg = "-filter=^github\\.com/portpowered/infinite-you(/|$)"
    repository = command(upstream + [filter_arg, "./..."], root, environment, stderr, executor)
    host_report = command(upstream + [filter_arg, "./cmd/golangci-lint"], host, environment, stderr, executor)
    host_report = repository_positions(host_report, host, root)
    template = '{{if .Module}}{{if eq .Module.Path "' + MODULE + '"}}{{.Dir}}{{end}}{{end}}'
    directories = command([upstream[0], "list", "-deps", "-f", template, "./cmd/golangci-lint"],
                          host, environment, stderr, executor)
    packages = {relative_source(line.strip(), host, root)
                for line in directories.splitlines() if line.strip()}
    if not {"tools/golangcilintplugin", "internal/lint/analyzers"}.issubset(packages):
        raise ValueError("generated host does not compile the repository plugin and analyzers")
    return reconcile(repository, host_report, packages)


def persist(root, actual, stdout, stderr):
    actual = normalize(actual)
    try:
        (root / "bin").mkdir(parents=True, exist_ok=True)
    except OSError as error:
        raise ValueError(f"create deadcode output directory: {error}") from error
    try:
        (root / CURRENT).write_text(actual, encoding="utf-8", newline="\n")
    except OSError as error:
        raise ValueError(f"write current deadcode report: {error}") from error
    try:
        baseline = normalize((root / BASELINE).read_text(encoding="utf-8"))
    except OSError as error:
        raise ValueError(f"read deadcode baseline: {error}") from error
    if actual != baseline:
        stderr.write(f"deadcode baseline drift detected; review {CURRENT} and update {BASELINE} when intentional\n")
        stderr.write(f"baseline findings: {count(baseline)}, current findings: {count(actual)}\n")
        stderr.write(f"LINT_VIOLATION_COUNT: {count(actual)}\n")
        return 1
    stdout.write("[agent-factory:deadcode] baseline matches\n")
    return 0


def main(argv=None, *, root=None, environment=None, stdout=None, stderr=None,
         executor=subprocess.run):
    stdout = sys.stdout if stdout is None else stdout
    stderr = sys.stderr if stderr is None else stderr
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--golangci-host-file", default=HOST_POINTER)
    parser.add_argument("upstream", nargs=argparse.REMAINDER,
                        help="-- go run golang.org/x/tools/cmd/deadcode@v0.25.1")
    args = parser.parse_args(argv)
    upstream = args.upstream
    if upstream[:1] == ["--"]:
        upstream = upstream[1:]
    if len(upstream) != 3 or upstream[1:] != ["run", "golang.org/x/tools/cmd/deadcode@v0.25.1"]:
        parser.error("supply the pinned upstream command after --")
    root = Path(os.path.abspath(root or Path.cwd()))
    try:
        actual = collect(root, args.golangci_host_file, upstream,
                         dict(os.environ) if environment is None else environment, stderr, executor)
        return persist(root, actual, stdout, stderr)
    except (OSError, ValueError, KeyError, TypeError) as error:
        stderr.write(f"deadcode report: {error}\n")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
