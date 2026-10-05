#!/usr/bin/env python3
"""Exercise lint diagnostics/exits in owned modules, never inventory the repo.

The invoking Make lane prepares the pinned invocation before fixture scenarios.
Only the full-threshold config copy disables diff filtering; all ratchet cases
consume the repository configuration unchanged. Artifacts stay in .artifacts.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]


def execute(command: list[str], cwd: Path) -> subprocess.CompletedProcess:
    # Isolate git fixtures from caller identity, hooks, signing, and global config.
    env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
    for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
        env.pop(key, None)
    return subprocess.run(command, cwd=cwd, env=env, capture_output=True,
                          text=True, encoding="utf-8", errors="replace", timeout=180)


def checked(command: list[str], cwd: Path) -> str:
    result = execute(command, cwd)
    if result.returncode:
        raise RuntimeError(f"{command}: exit {result.returncode}\n{result.stdout}{result.stderr}")
    return result.stdout


def write(root: Path, name: str, source: str) -> None:
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(source, encoding="utf-8")


def long_function(lines: int, name: str = "longFunction") -> str:
    return f"func {name}() {{\n" + "\t_ = 1\n" * lines + "}\n"


def complex_function(complexity: int, name: str = "complexFunction") -> str:
    # One base path plus one per if statement.
    return (f"func {name}(x int) int {{\n" +
            "\tif x > 0 { x-- }\n" * (complexity - 1) + "\treturn x\n}\n")


def file_lines(lines: int) -> str:
    return "package fixture\n" + "\n// excluded comment\n" + "".join(
        f"var value{i} = {i}\n" for i in range(lines - 1))


class SizeFixtures:
    def __init__(self, tool: list[str], artifacts: Path, config: str):
        self.tool = tool
        self.artifacts = artifacts
        self.config = config
        self.results: list[dict] = []

    def module(self, name: str, ratchet: bool = False) -> Path:
        root = self.artifacts / name
        root.mkdir()
        write(root, "go.mod", "module lintfixture\n\ngo 1.25.0\n")
        config = self.config if ratchet else self.config.replace(
            "  new-from-merge-base: origin/main\n", "")
        write(root, ".golangci.yml", config)
        return root

    def lint(self, root: Path, label: str, expected: list[tuple[str, str]],
             error: str = "") -> list[dict]:
        output = root / f"{label}.json"
        command = self.tool + ["run", "--concurrency=2", "--allow-parallel-runners",
                               "--uniq-by-line=false",
                               "--output.json.path", str(output), "./..."]
        # Match the canonical Make prerequisite even when lint finds no issues.
        # golangci itself may never consult git for clean input.
        if "  new-from-merge-base: origin/main\n" in (root / ".golangci.yml").read_text():
            result = execute(["git", "merge-base", "HEAD", "origin/main"], root)
            if result.returncode == 0:
                result = execute(command, root)
        else:
            result = execute(command, root)
        write(root, f"{label}.log", result.stdout + result.stderr)
        if error:
            assert result.returncode != 0, f"{label}: missing failure"
            assert error in result.stdout + result.stderr, f"{label}: missing {error}"
            issues = []
        else:
            assert output.exists(), f"{label}: missing diagnostics: {result.stderr}"
            issues = json.loads(output.read_text(encoding="utf-8")).get("Issues") or []
            assert len(issues) == len(expected), f"{label}: unexpected diagnostics {issues}"
            for issue, (linter, message) in zip(
                    sorted(issues, key=lambda i: (i["FromLinter"], i["Text"])),
                    sorted(expected)):
                assert issue["FromLinter"] == linter and message in issue["Text"], issue
                assert issue["Pos"]["Line"] > 0 and issue["Pos"]["Filename"], issue
            assert result.returncode == (1 if expected else 0), (
                f"{label}: wrong exit {result.returncode}: {result.stderr}")
        self.results.append({"case": label, "exit": result.returncode, "issues": issues})
        print(f"PASS {label}: exit={result.returncode}, diagnostics={len(issues)}", flush=True)
        return issues

    def thresholds(self) -> None:
        for kind, valid, invalid, diagnostic in (
            ("file", file_lines(1000), file_lines(1001),
             ("revive", "file-length-limit: file length is 1001 lines")),
            ("function", "package fixture\n" + long_function(100),
             "package fixture\n" + long_function(101),
             ("revive", "function-length: maximum number of lines per function exceeded; max 100 but got 101")),
            ("complexity", "package fixture\n" + complex_function(15),
             "package fixture\n" + complex_function(16),
             ("gocyclo", "cyclomatic complexity 16")),
        ):
            root = self.module(kind)
            write(root, "pkg/fixture/source.go", valid)
            self.lint(root, f"{kind}-limit", [])
            write(root, "pkg/fixture/source.go", invalid)
            issues = self.lint(root, f"{kind}-over-limit", [diagnostic])
            expected_line = len(invalid.splitlines()) if kind == "file" else 2
            assert issues[0]["Pos"]["Line"] == expected_line, issues
            generated = "// Code generated by fixture. DO NOT EDIT.\n" + invalid
            write(root, "pkg/fixture/source.go", generated)
            self.lint(root, f"{kind}-generated", [])

    def test_and_suppression(self) -> None:
        root = self.module("tests-and-suppression")
        write(root, "pkg/fixture/source.go", "package fixture\n")
        write(root, "pkg/fixture/source_test.go", "package fixture\n" + long_function(101))
        length = ("revive", "function-length: maximum number of lines per function exceeded")
        self.lint(root, "test-function-eligible", [length])
        write(root, "pkg/fixture/source_test.go", "package fixture\n")
        source = complex_function(16).replace("\treturn x\n", "\t_ = 1\n" * 86 + "\treturn x\n")
        write(root, "pkg/fixture/source.go", "package fixture\n" + source)
        complexity = ("gocyclo", "cyclomatic complexity 16")
        self.lint(root, "both-linters", [length, complexity])
        write(root, "pkg/fixture/source.go", "package fixture\n" +
              "//nolint:revive // Keep this deliberate length witness while testing complexity.\n" + source)
        self.lint(root, "revive-only-suppression", [complexity])
        write(root, "pkg/fixture/source.go", "package fixture\n" +
              "//nolint:gocyclo // Keep this deliberate branching witness while testing length.\n" + source)
        self.lint(root, "gocyclo-only-suppression", [length])

    def scope(self) -> None:
        root = self.module("owned-roots")
        for family in ("cmd", "internal", "pkg", "tests"):
            write(root, f"{family}/fixture/source.go", "package fixture\n" + long_function(101))
        diagnostic = ("revive", "function-length: maximum number of lines per function exceeded")
        self.lint(root, "all-owned-roots", [diagnostic] * 4)
        outside = self.module("outside-owned-roots")
        write(outside, "ui/fixture/source.go", "package fixture\n" + long_function(101))
        self.lint(outside, "outside-owned-roots", [])

    def commit(self, root: Path) -> None:
        checked(["git", "add", "go.mod", ".golangci.yml", "pkg"], root)
        checked(["git", "-c", "user.name=Lint fixture", "-c", "user.email=lint@example.invalid",
                 "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.devnull,
                 "commit", "-qm", "fixture"], root)

    def ratchet(self) -> None:
        root = self.module("ratchet", ratchet=True)
        checked(["git", "init", "-q"], root)
        source = "package fixture\n" + long_function(101)
        write(root, "pkg/fixture/source.go", source)
        self.commit(root)
        checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
        self.lint(root, "unchanged-debt", [])
        source = source.replace("func longFunction()", "func movedFunction()")
        write(root, "pkg/fixture/source.go", source)
        self.commit(root)
        diagnostic = ("revive", "function-length: maximum number of lines per function exceeded")
        self.lint(root, "changed-declaration", [diagnostic])
        write(root, "pkg/fixture/source.go", "package fixture\n" +
              "//nolint:revive // Refactoring keeps this existing oversized operation temporarily.\n" +
              long_function(101, "movedFunction"))
        self.commit(root)
        self.lint(root, "ratchet-reasoned-suppression", [])
        write(root, "pkg/fixture/new.go", "package fixture\n" + long_function(101, "newFunction"))
        self.commit(root)
        self.lint(root, "new-declaration", [diagnostic])

        body = self.module("body-growth", ratchet=True)
        checked(["git", "init", "-q"], body)
        write(body, "pkg/fixture/source.go", "package fixture\n" + long_function(100))
        self.commit(body)
        checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], body)
        write(body, "pkg/fixture/source.go", "package fixture\n" + long_function(101))
        self.commit(body)
        self.lint(body, "body-only-growth-filtered", [])
        write(body, ".golangci.yml", self.config.replace("  new-from-merge-base: origin/main\n", ""))
        self.lint(body, "body-growth-without-filter", [diagnostic])

        file = self.module("file-ratchet", ratchet=True)
        checked(["git", "init", "-q"], file)
        write(file, "pkg/fixture/source.go", file_lines(1001))
        self.commit(file)
        checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], file)
        self.lint(file, "unchanged-file-debt", [])
        write(file, "pkg/fixture/source.go", file_lines(1002))
        self.commit(file)
        self.lint(file, "changed-file-final-line", [
            ("revive", "file-length-limit: file length is 1002 lines")])

    def errors(self) -> None:
        root = self.module("missing-history", ratchet=True)
        checked(["git", "init", "-q"], root)
        write(root, "pkg/fixture/source.go", "package fixture\n")
        self.commit(root)
        self.lint(root, "missing-origin-main", [], error="origin/main")
        write(root, ".golangci.yml", self.config.replace('version: "2"', 'version: "invalid"'))
        result = execute(self.tool + ["config", "verify"], root)
        write(root, "invalid-config.log", result.stdout + result.stderr)
        assert result.returncode != 0, "invalid configuration accepted"
        self.results.append({"case": "invalid-config", "exit": result.returncode})
        print(f"PASS invalid-config: exit={result.returncode}", flush=True)


class PkgFixtures(SizeFixtures):
    # Each witness exercises the real pinned linter, including type resolution.
    witnesses = {
        "bodyclose": ('import "net/http"\n', 'func witness() error {\n r, err := http.Get("https://example.invalid")\n if err != nil { return err }; _ = r; return nil\n}\n', "response body must be closed"),
        "contextcheck": ('import ("context"; "net/http")\n', 'func witness(ctx context.Context) error {\n r, err := http.NewRequestWithContext(context.Background(), "GET", "https://example.invalid", nil)\n _ = r; return err\n}\n', "Non-inherited"),
        "errcheck": ('import "os"\n', 'func witness() { os.Chdir(".") }\n', "not checked"),
        "errorlint": ('import "io"\n', 'func witness(err error) bool { return err == io.EOF }\n', "comparing with =="),
        "govet": ('import "fmt"\n', 'func witness() { fmt.Printf("%d", "text") }\n', "wrong type"),
        "ineffassign": ('', 'func witness() int {\n x := 1\n x = 2\n return x\n}\n', "ineffectual assignment"),
        "nilerr": ('import "os"\n', 'func witness() error {\n err := os.Chdir(".")\n if err != nil { return nil }; return err\n}\n', "returns nil"),
        "staticcheck": ('import "strings"\n', 'func witness(s string) string { return strings.Replace(s, "a", "b", 0) }\n', "SA1018"),
    }

    def rules(self) -> None:
        root = self.module("pkg-rules")
        expected = []
        for rule, (imports, body, message) in self.witnesses.items():
            write(root, f"pkg/{rule}/source.go", "package fixture\n" + imports + body)
            expected.append((rule, message))
        # govet's printf witness also intentionally triggers staticcheck SA5009.
        expected.append(("staticcheck", "SA5009"))
        self.lint(root, "eight-pkg-rules", expected)
        # The same violations in tests remain eligible, as in the old wrapper.
        for rule in self.witnesses:
            source = root / f"pkg/{rule}/source.go"
            source.rename(source.with_name("source_test.go"))
        self.lint(root, "eight-pkg-test-rules", expected)

        for rule, (imports, body, _) in self.witnesses.items():
            write(root, f"pkg/{rule}/source_test.go", "package fixture\n")
            write(root, f"pkg/{rule}/generated.go",
                  "// Code generated by fixture. DO NOT EDIT.\npackage fixture\n" + imports + body)
        self.lint(root, "pkg-generated-excluded", [])

        outside = self.module("pkg-outside-scope")
        for rule, (imports, body, _) in self.witnesses.items():
            write(outside, f"internal/{rule}/source.go", "package fixture\n" + imports + body)
        self.lint(outside, "pkg-rules-outside-pkg", [])

        clean = self.module("pkg-clean")
        write(clean, "pkg/fixture/source.go", 'package fixture\nimport "os"\nfunc witness() error { return os.Chdir(".") }\n')
        self.lint(clean, "pkg-clean", [])

    def pkg_ratchet(self) -> None:
        root = self.module("pkg-ratchet", ratchet=True)
        checked(["git", "init", "-q"], root)
        for rule, (imports, body, _) in self.witnesses.items():
            write(root, f"pkg/{rule}/source.go", "package fixture\n" + imports + body)
        self.commit(root)
        checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
        self.lint(root, "eight-pkg-unchanged-debt", [])
        # Add wholly new files while leaving the existing debt untouched.
        for rule, (imports, body, _) in self.witnesses.items():
            write(root, f"pkg/{rule}/new.go", "package fixture\n" + imports + body.replace("witness", "newWitness"))
        self.commit(root)
        issues = self.lint(root, "eight-pkg-new-debt", [
            (rule, message) for rule, (_, _, message) in self.witnesses.items()
        ] + [("staticcheck", "SA5009")])
        assert {issue["FromLinter"] for issue in issues} == set(self.witnesses)

    def missing_inputs(self) -> None:
        root = self.module("pkg-missing-config")
        write(root, "pkg/fixture/source.go", "package fixture\n")
        result = execute(self.tool + ["run", "--config", "missing.yml", "./..."], root)
        write(root, "missing-config.log", result.stdout + result.stderr)
        assert result.returncode != 0 and "missing.yml" in result.stderr
        self.results.append({"case": "missing-config", "exit": result.returncode})
        print(f"PASS missing-config: exit={result.returncode}", flush=True)
        try:
            execute([str(root / "missing-golangci")], root)
        except FileNotFoundError:
            self.results.append({"case": "missing-tool", "error": "FileNotFoundError"})
            print("PASS missing-tool: FileNotFoundError", flush=True)
        else:
            raise AssertionError("missing tool accepted")
        self.errors()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cohort", choices=("size", "pkg-rules", "all"))
    parser.add_argument("--golangci", required=True)
    args = parser.parse_args()
    tool = shlex.split(args.golangci)
    version = checked(tool + ["version"], ROOT)
    assert "version 2.11.4 " in version, f"expected pinned golangci-lint v2.11.4: {version}"
    checked(tool + ["config", "verify"], ROOT)
    parent = ROOT / ".artifacts/lint-migration-smoke"
    parent.mkdir(parents=True, exist_ok=True)
    artifacts = Path(tempfile.mkdtemp(prefix=args.cohort + "-", dir=parent))
    write(artifacts, "tool.txt", version)
    print(f"Artifacts: {artifacts}\n{version.strip()}", flush=True)
    fixtures = SizeFixtures(tool, artifacts, (ROOT / ".golangci.yml").read_text(encoding="utf-8"))
    try:
        if args.cohort in ("size", "all"):
            fixtures.thresholds()
            fixtures.test_and_suppression()
            fixtures.scope()
            fixtures.ratchet()
            fixtures.errors()
        if args.cohort in ("pkg-rules", "all"):
            pkg_artifacts = artifacts / "pkg-cohort"
            pkg_artifacts.mkdir()
            pkg = PkgFixtures(tool, pkg_artifacts, fixtures.config)
            pkg.results = fixtures.results
            pkg.rules()
            pkg.pkg_ratchet()
            pkg.missing_inputs()
    finally:
        write(artifacts, "results.json", json.dumps(fixtures.results, indent=2) + "\n")
    print(f"PASS {args.cohort}: {len(fixtures.results)} scenarios", flush=True)


if __name__ == "__main__":
    main()
