"""Prove production host reachability without exempting plugin dead functions.

This static fixture consumes a prebuilt deadcodecheck. The pinned external
deadcode tool loads real Go main packages; no tests are reachability roots.
"""

import argparse
import os
from pathlib import Path
import subprocess
import tempfile


MODULE = "github.com/portpowered/infinite-you"


def write(root, name, text):
    target = root / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(text, encoding="utf-8")


def invoke(checker, root, expected, diagnostic):
    environment = dict(os.environ, GOWORK="off")
    result = subprocess.run(
        [str(checker)], cwd=root, env=environment,
        text=True, capture_output=True, timeout=180,
    )
    output = result.stdout + result.stderr
    if result.returncode != expected or diagnostic not in output:
        raise AssertionError(f"exit={result.returncode}, expected={expected}\n{output}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checker", required=True, type=Path)
    args = parser.parse_args()
    checker = args.checker.resolve(strict=True)
    with tempfile.TemporaryDirectory(prefix="deadcode-host-smoke-") as directory:
        base = Path(directory)
        root, host = base / "repository", base / "host"
        write(root, "go.mod", f"module {MODULE}\ngo 1.24.0\n")
        write(root, "cmd/product/main.go", f'''package main
import "{MODULE}/pkg/product"
func main() {{ product.Live() }}
''')
        write(root, "pkg/product/product.go", '''package product
func Live() {}
func TestsOnly() {}
''')
        write(root, "pkg/product/product_test.go", '''package product
import "testing"
func TestHelper(t *testing.T) { TestsOnly() }
''')
        write(root, "internal/lint/analyzers/rule.go", '''package analyzers
func Live() {}
func Abandoned() {}
''')
        write(root, "tools/golangcilintplugin/plugin.go", f'''package golangcilintplugin
import "{MODULE}/internal/lint/analyzers"
func New() {{ analyzers.Live() }}
func Abandoned() {{}}
''')
        write(host, "go.mod", f'''module github.com/golangci/golangci-lint/v2
go 1.24.0
require {MODULE} v0.0.0
replace {MODULE} => "{root.as_posix()}"
''')
        write(host, "cmd/golangci-lint/main.go", f'''package main
import "{MODULE}/tools/golangcilintplugin"
func main() {{ golangcilintplugin.New() }}
''')
        pointer = ".artifacts/golangci/host-path.txt"
        write(root, pointer, str(host) + "\n")
        baseline = "\n".join([
            "internal/lint/analyzers/rule.go: unreachable func: Abandoned",
            "pkg/product/product.go: unreachable func: TestsOnly",
            "tools/golangcilintplugin/plugin.go: unreachable func: Abandoned",
        ]) + "\n"
        write(root, "docs/internal/baselines/deadcode-baseline.txt", baseline)
        invoke(checker, root, 0, "baseline matches")
        assert (root / "bin/deadcode-current.txt").read_text() == baseline
        print("PASS: real host calls are live; abandoned analyzer/plugin and test-only functions remain dead")

        write(root, "tools/golangcilintplugin/plugin.go", f'''package golangcilintplugin
import "{MODULE}/internal/lint/analyzers"
func New() {{ analyzers.Live() }}
func Abandoned() {{}}
func NewDeadFunction() {{}}
''')
        invoke(checker, root, 1, "baseline drift")
        assert "NewDeadFunction" in (root / "bin/deadcode-current.txt").read_text()
        print("PASS: a new unused plugin function fails the unchanged baseline")
        (root / pointer).unlink()
        invoke(checker, root, 1, "read generated golangci host")
        print("PASS: absent production host fails closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
