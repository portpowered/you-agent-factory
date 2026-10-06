"""Prove production host reachability without exempting plugin dead functions.

This static fixture uses the same report adapter and upstream command as Make.
The pinned external deadcode tool loads real Go main packages; no tests are
reachability roots.
"""

import argparse
import os
from pathlib import Path
import subprocess
import tempfile
import sys


MODULE = "github.com/portpowered/infinite-you"


def write(root, name, text):
    target = root / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(text, encoding="utf-8")


def invoke(reporter, upstream, root, anchor, expected, diagnostic):
    environment = dict(os.environ, GOWORK="off")
    result = subprocess.run(
        [sys.executable, str(reporter), "--historical-base", anchor, "--"] + upstream, cwd=root, env=environment,
        text=True, capture_output=True, timeout=180,
    )
    output = result.stdout + result.stderr
    if result.returncode != expected or diagnostic not in output:
        raise AssertionError(f"exit={result.returncode}, expected={expected}\n{output}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reporter", required=True, type=Path)
    parser.add_argument("upstream", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    reporter = args.reporter.resolve(strict=True)
    upstream = args.upstream[1:] if args.upstream[:1] == ["--"] else args.upstream
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
        environment = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
        environment.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
        def git(*arguments):
            return subprocess.run(["git", *arguments], cwd=root, env=environment,
                                  text=True, capture_output=True, check=True).stdout.strip()
        git("init", "--initial-branch=main")
        for key, value in [("user.name", "Deadcode Smoke"), ("user.email", "fixture@example.invalid"),
                           ("commit.gpgsign", "false"), ("core.hooksPath", str(root / "no-hooks"))]:
            git("config", key, value)
        git("add", ".")
        git("commit", "-m", "established allowances")
        anchor = git("rev-parse", "HEAD")
        git("update-ref", "refs/remotes/origin/main", anchor)
        invoke(reporter, upstream, root, anchor, 0, "baseline matches")
        assert (root / "bin/deadcode-current.txt").read_text() == baseline
        print("PASS: real host calls are live; abandoned analyzer/plugin and test-only functions remain dead")

        (root / "pkg/relocated").mkdir()
        git("mv", "pkg/product/product.go", "pkg/relocated/product.go")
        write(root, "cmd/product/main.go", f'''package main
import "{MODULE}/pkg/relocated"
func main() {{ product.Live() }}
''')
        (root / "pkg/product/product_test.go").unlink()
        baseline = baseline.replace("pkg/product/product.go", "pkg/relocated/product.go")
        write(root, "docs/internal/baselines/deadcode-baseline.txt", baseline)
        git("add", ".")
        git("commit", "-m", "relocate compiler-owned source")
        invoke(reporter, upstream, root, anchor, 0, "baseline matches")
        print("PASS: faithful Git relocation retains only its compiler-owned allowance")

        write(root, "tools/golangcilintplugin/plugin.go", f'''package golangcilintplugin
import "{MODULE}/internal/lint/analyzers"
func New() {{ analyzers.Live() }}
func Abandoned() {{}}
func NewDeadFunction() {{}}
''')
        invoke(reporter, upstream, root, anchor, 1, "baseline drift")
        assert "NewDeadFunction" in (root / "bin/deadcode-current.txt").read_text()
        assert (root / "docs/internal/baselines/deadcode-baseline.txt").read_text() == baseline
        print("PASS: a new unused plugin function fails the unchanged baseline")
        write(root, "docs/internal/baselines/deadcode-baseline.txt",
              (root / "bin/deadcode-current.txt").read_text())
        invoke(reporter, upstream, root, anchor, 1, "deadcode historical allowance growth")
        print("PASS: matching enlarged actual/baseline still rejects new plugin debt")
        (root / pointer).unlink()
        invoke(reporter, upstream, root, anchor, 1, "read generated golangci host")
        print("PASS: absent production host fails closed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
