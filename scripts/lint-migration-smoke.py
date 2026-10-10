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
import re
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parents[1]


def execute(command: list[str], cwd: Path, compiler_env: dict | None = None) -> subprocess.CompletedProcess:
    # Isolate git fixtures from caller identity, hooks, signing, and global config.
    env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
    for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
        env.pop(key, None)
    env.update(compiler_env or {})
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
        shutil.copytree(ROOT / "scripts/testdata/packagedfactorycatalog/packages", root / "packages")
        # Both validators require compiler-declared source and publication inputs.
        write(root, "packages/packaged-factories/embed.go",
              'package packagedfactories\nimport "embed"\n'
              '//go:embed factories\nvar Source embed.FS\n')
        config = self.config if ratchet else self.config.replace(
            "  new-from-merge-base: origin/main\n", "")
        # Synthetic modules use the real import prefix to exercise ownership
        # rules, but do not contain the repository's retained recording reads.
        # Defer only stale embedded recording debt here; new forbidden reads
        # remain errors. Production configs and exact debt stay unchanged.
        config = re.sub(r"defer-stale: \[([^\]]*)\]",
                        r"defer-stale: [\1, recordingreads]", config)
        write(root, ".golangci.yml", config)
        return root

    def lint(self, root: Path, label: str, expected: list[tuple[str, str]],
             error: str = "", flags: tuple[str, ...] = (),
             compiler_env: dict | None = None, tags: str = "") -> list[dict]:
        output = root / f"{label}.json"
        command = self.tool + ["run", "--concurrency=2", "--allow-parallel-runners",
                               "--uniq-by-line=false",
                               "--output.json.path", str(output), *flags, "./..."]
        if tags:
            command += ["--build-tags", tags]
        # Match the canonical Make prerequisite even when lint finds no issues.
        # golangci itself may never consult git for clean input.
        if "  new-from-merge-base: origin/main\n" in (root / ".golangci.yml").read_text():
            result = execute(["git", "merge-base", "HEAD", "origin/main"], root)
            if result.returncode == 0:
                result = execute(command, root, dict(compiler_env or {}, GOLANGCI_LINT_CACHE=str(root / ".golangci-cache")))
        else:
            result = execute(command, root, dict(compiler_env or {}, GOLANGCI_LINT_CACHE=str(root / ".golangci-cache")))
        write(root, f"{label}.log", result.stdout + result.stderr)
        if error:
            assert result.returncode != 0, f"{label}: missing failure"
            assert error in result.stdout + result.stderr, f"{label}: missing {error}"
            issues = []
        else:
            assert output.exists(), f"{label}: missing diagnostics: {result.stderr}"
            issues = json.loads(output.read_text(encoding="utf-8")).get("Issues") or []
            assert len(issues) == len(expected), f"{label}: unexpected diagnostics {issues}"
            remaining = list(issues)
            for linter, message in expected:
                matches = [issue for issue in remaining
                           if issue["FromLinter"] == linter and message in issue["Text"]]
                assert matches, f"{label}: missing {(linter, message)} in {remaining}"
                issue = matches[0]
                assert issue["Pos"]["Line"] > 0 and issue["Pos"]["Filename"], issue
                remaining.remove(issue)
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


class BoundaryFixtures(SizeFixtures):
    """Seven grouped witnesses through the compiled plugin, including cache reuse."""

    cycle_key = "service-cycle-weight|internal/lint/analyzers|0\n"
    effect_source = 'package effects\nimport "time"\nfunc Run() { _ = time.Now() }\n'
    # Exercise an existing embedded production key; fixture text cannot change
    # the delivered host's tolerance, and this lane never adds fixture debt.
    debt_unit = "pkg/services/factory_definitions/internal/services/distribution/scaffoldfacts"
    debt_path = debt_unit + "/resolver.go"
    effect_key = ("production-default|" + debt_unit + "|" + debt_path +
                  "#LocalFactoryNameResolver#filesystem#os.ReadFile::count=1\n")
    debt_source = ('package scaffoldfacts\nimport "os"\n'
                   'func LocalFactoryNameResolver() { _, _ = os.ReadFile("fixture") }\n')

    def prepared(self, name: str, sources: dict[str, str], debt: str = "") -> Path:
        root = self.module(name)
        write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
        write(root, "pkg/wire/wire.go", "package wire\n")
        write(root, "internal/lint/analyzers/source.go", "package analyzers\n")
        service = next((path.split("/")[2] for path in sources
                        if path.startswith("pkg/services/")), None)
        # These isolated service graphs are acyclic; preserve their numeric
        # policy while independently mutating package-boundary debt.
        write(root, "internal/lint/analyzers/baseline.txt", (self.cycle_key if service else "") + debt)
        if service:
            write(root, "pkg/services/lint_fixture/source.go", "package fixture\ntype Service interface {}\n")
        for name, source in sources.items():
            write(root, name, source)
        checked(["git", "init", "-q"], root)
        checked(["git", "config", "core.longpaths", "true"], root)
        checked(["git", "add", "internal"], root)
        self.commit(root)
        checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
        return root

    def allowed_and_selection(self) -> None:
        root = self.prepared("A-D-selection", {
            "pkg/platform/clock/clock.go": 'package clock\nimport "time"\ntype Real struct{}\nfunc (Real) Now() time.Time { return time.Now() }\n',
        })
        selection = ('package wire\nimport "github.com/portpowered/infinite-you/pkg/platform/clock"\n'
                     'var _ = clock.Real{}\n')
        write(root, "pkg/wire/wire.go", selection)
        self.lint(root, "A-exact-selected-leaf", [])
        expected = [("repolint", "Real.Now#clock#time.Now::count=1")]
        write(root, "pkg/wire/wire.go", "package wire\n")
        self.lint(root, "D-removed-selection-cached", expected)
        write(root, "pkg/wire/wire.go", "package wire\n// clock.Real{} is only a comment\n")
        self.lint(root, "D-fake-selection-cached", expected)
        write(root, "pkg/wire/wire.go", selection)
        self.lint(root, "D-restored-selection", [])

    def new_violations(self) -> None:
        root = self.prepared("B-new-violations", {
            "pkg/services/fixture/wire/source.go": "package fixture\nfunc NewService() {}\n",
            "cmd/fixture/source.go": ('package fixture\n'
                'import "github.com/portpowered/infinite-you/pkg/services/fixture/wire"\n'
                'func Run() { fixture.NewService() }\n'),
            "pkg/platform/effects/source.go": self.effect_source,
        })
        issues = self.lint(root, "B-qualified-construction-and-effect", [
            ("repolint", "construction: service-construction:"),
            ("repolint", "packageboundary: production-default:"),
        ])
        assert any("pkg/services/fixture/wire.NewService" in issue["Text"] for issue in issues)
        assert any("Run#clock#time.Now::count=1" in issue["Text"] for issue in issues)
        assert all("inject" in issue["Text"] and "pkg/wire" in issue["Text"] for issue in issues)

    def counted_debt(self) -> None:
        root = self.prepared("C-counted-debt", {self.debt_path: self.debt_source}, self.effect_key)
        self.lint(root, "C-exact-count", [])
        write(root, self.debt_path, self.debt_source.replace("func Local", "\n// line motion\nfunc Local"))
        self.lint(root, "C-line-motion", [])
        write(root, self.debt_path, self.debt_source.replace('_, _ = os.ReadFile("fixture")',
            '_, _ = os.ReadFile("fixture"); _, _ = os.ReadFile("fixture")'))
        self.lint(root, "C-extra-occurrence", [
            ("repolint", "LocalFactoryNameResolver#filesystem#os.ReadFile::count=2"),
            ("repolint", "stale baseline entry"),
        ])

    def stale_debt(self) -> None:
        root = self.prepared("E-stale-debt", {self.debt_path: self.debt_source}, self.effect_key)
        write(root, self.debt_path, "package scaffoldfacts\n")
        self.lint(root, "E-removed-use", [("repolint", "stale baseline entry")])
        (root / self.debt_path).unlink()
        self.lint(root, "E-vanished-owner", [("repolint", "compiler-ownership:")])
        write(root, "internal/lint/analyzers/baseline.txt", self.cycle_key)
        self.lint(root, "E-deleted-resolved-key", [])

    def missing_inputs(self) -> None:
        root = self.prepared("F-missing-inputs", {"pkg/platform/effects/source.go": "package effects\n"})
        (root / "pkg/wire/wire.go").unlink()
        self.lint(root, "F-missing-wire", [
            ("repolint", "wire-selection-metadata:"),
            ("repolint", "package-boundary-metadata:"),
        ])
        write(root, "pkg/wire/wire.go", "package wire\n")
        checked(["git", "update-ref", "-d", "refs/remotes/origin/main"], root)
        self.lint(root, "F-missing-history", [("repolint", "origin/main")])

    def compiler_context(self) -> None:
        root = self.prepared("G-compiler-context", {
            "pkg/platform/effects/plain.go": "package effects\n",
            "pkg/platform/effects/source_windows.go": "//go:build integration\n\n" + self.effect_source,
            "pkg/platform/effects/source_linux.go": "//go:build integration\n\n" + self.effect_source,
        })
        self.lint(root, "G-inactive-tag", [], compiler_env={"GOOS": "windows"})
        tags = ("--build-tags=integration,functionallong,backendconformance,factoryartifact,managed_process_integration",)
        for platform in ("windows", "linux"):
            issues = self.lint(root, "G-active-" + platform,
                [("repolint", "Run#clock#time.Now::count=1")], flags=tags, compiler_env={"GOOS": platform})
            assert issues[0]["Pos"]["Filename"].endswith("source_" + platform + ".go")


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


def packaged_source(fixtures: SizeFixtures) -> None:
    # Consume the delivered plugin with both canonical configurations. Each
    # cell owns its module; the warm-cache steps keep all Go inputs unchanged.
    for mode, config in (("default", ".golangci-repository-default.yml"),
                         ("complete", ".golangci-repository.yml")):
        fixtures.config = (ROOT / config).read_text(encoding="utf-8")
        root = fixtures.module(f"packaged-source-{mode}")
        write(root, "pkg/wire/wire.go", "package wire\n")
        write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
        write(root, "internal/lint/analyzers/source.go", "package analyzers\n")
        write(root, "internal/lint/analyzers/baseline.txt", "")
        write(root, "pkg/transports/fixture/source.go", "package fixture\n")
        write(root, "packages/packaged-factories/embed.go",
              'package packagedfactories\nimport "embed"\n'
              '//go:embed factories\nvar Source embed.FS\n')
        path = "packages/packaged-factories/factories/example/factory.js"
        definition = (root / path).read_bytes()
        checked(["git", "init", "-q"], root)
        checked(["git", "config", "core.longpaths", "true"], root)
        checked(["git", "add", "."], root)
        fixtures.commit(root)
        checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
        if mode == "complete":
            write(root, ".golangci.yml", fixtures.config.replace(
                "  timeout: 30m\n", "  timeout: 30m\n  build-tags: [integration, functionallong, backendconformance, factoryartifact, managed_process_integration]\n"))
        fixtures.lint(root, f"{mode}-source-clean", [])
        write(root, "pkg/transports/fixture/source.go",
              'package fixture\nvar Definition = `{"name":" @you/off-boundary "}`\n')
        issues = fixtures.lint(root, f"{mode}-source-literal", [("repolint", "@you/off-boundary")])
        assert issues[0]["Pos"]["Line"] == 2, issues
        write(root, "pkg/transports/fixture/source.go", "package fixture\n")
        write(root, "packages/packaged-factories/factories/example/factory.yaml", "name: '@you/new'\n")
        fixtures.lint(root, f"{mode}-source-duplicate-root", [("repolint", "packaged-factory-source:"), ("repolint", "projection failed:")])
        (root / "packages/packaged-factories/factories/example/factory.yaml").unlink()
        fixtures.lint(root, f"{mode}-source-warm-clean", [])
        write(root, path, "return {};\n")
        fixtures.lint(root, f"{mode}-source-warm-invalid", [("repolint", "packaged-factory-source:"), ("repolint", "projection failed:")])
        (root / path).write_bytes(definition)
        fixtures.lint(root, f"{mode}-source-recovered", [])
        # Break the actual compiler embed boundary, leaving its owning unit.
        write(root, "packages/packaged-factories/embed.go",
              'package packagedfactories\nimport "embed"\n'
              '//go:embed absent\nvar Source embed.FS\n')
        fixtures.lint(root, f"{mode}-source-metadata-failure", [("typecheck", "pattern absent: no matching files found")])
        (root / "packages/packaged-factories/embed.go").unlink()
        fixtures.lint(root, f"{mode}-source-missing-owner", [("repolint", "incomplete compiler metadata")])


def consumption(fixtures: SizeFixtures) -> None:
    prefix = "github.com/portpowered/infinite-you/"
    fixtures.config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
    root = fixtures.module("consumption")
    write(root, "go.mod", "module " + prefix.rstrip("/") + "\n\ngo 1.25.0\n")
    write(root, "packages/packaged-factories/publication.go", "package packagedfactories\nconst Bytes = 1\n")
    write(root, "internal/packagedfactorycatalog/catalog.go",
          'package packagedfactorycatalog\nimport _ "' + prefix + 'packages/packaged-factories"\n'
          'func LoadPublishedDefinitionCatalog() {}\n')
    call = ('import c "' + prefix + 'internal/packagedfactorycatalog"\n'
            'func load() { c.LoadPublishedDefinitionCatalog() }\n')
    for path in ("pkg/wire/profiles.go", "pkg/transports/http/handlers_models.go",
                 "pkg/services/factory_definitions/internal/services/distribution/goal/prompt_drift.go"):
        write(root, path, "package approved\n" + call)
    write(root, "internal/exempt/generated.go", "// Code generated by fixture. DO NOT EDIT.\npackage exempt\n" + call)
    write(root, "internal/exempt/source.go", "package exempt\n")
    write(root, "internal/exempt/bypass_test.go", "package exempt\n" + call.replace("load()", "testLoad()"))
    write(root, "internal/consumer/source.go", "package consumer\n")
    fixtures.lint(root, "consumption-approved-default", [])
    # The tiny HTTP stub proves its approved filename in ordinary mode. It
    # does not implement the unrelated real server's embedded Petri debt;
    # omit that owner from strict runs rather than suppress stale diagnostics.
    (root / "pkg/transports/http/handlers_models.go").unlink()
    fixtures.config = (ROOT / ".golangci-repository.yml").read_text(encoding="utf-8")
    write(root, ".golangci.yml", fixtures.config)
    fixtures.lint(root, "consumption-approved-complete", [], tags="integration,functionallong,backendconformance,factoryartifact,managed_process_integration")
    write(root, "internal/consumer/source.go", 'package consumer\nimport _ "' + prefix + 'packages/packaged-factories"\n')
    issues = fixtures.lint(root, "consumption-publication", [("repolint", "packaged-factory-direct-publication:")])
    assert issues[0]["Pos"]["Line"] == 2 and "Factory Definitions catalog resolve/install" in issues[0]["Text"], issues
    write(root, "internal/consumer/source.go", "package consumer\n" + call)
    issues = fixtures.lint(root, "consumption-loader-alias", [("repolint", "packaged-factory-catalog-loader:")])
    assert issues[0]["Pos"]["Line"] == 3 and "Factory Definitions catalog operations" in issues[0]["Text"], issues
    assert Path(issues[0]["Pos"]["Filename"]).name == "source.go", issues
    write(root, "internal/consumer/source.go", "package consumer\n")
    native = checked(["go", "env", "GOOS"], root).strip()
    write(root, "internal/consumer/source_" + native + ".go", "package consumer\n" + call)
    fixtures.lint(root, "consumption-loader-native-" + native, [("repolint", "packaged-factory-catalog-loader:")])
    (root / ("internal/consumer/source_" + native + ".go")).unlink()
    write(root, "internal/consumer/source_tagged.go", "//go:build integration\n\npackage consumer\n" + call)
    fixtures.lint(root, "consumption-loader-tagged-default", [])
    fixtures.lint(root, "consumption-loader-tagged-selected", [("repolint", "packaged-factory-catalog-loader:")], tags="integration")
    write(root, ".golangci.yml", fixtures.config.replace("repolint", "missing"))
    fixtures.lint(root, "consumption-unregistered-plugin", [], error="not found")
    write(root, ".golangci.yml", fixtures.config.replace("defer-stale: []", "defer-stale: [missing]"))
    fixtures.lint(root, "consumption-invalid-config", [], error="unknown deferred analyzer")


class MarkdownFixtures:
    """Static enforcement scenarios use the real Make recipe and prepared tools."""

    def __init__(self, artifacts: Path):
        self.artifacts = artifacts
        self.results: list[dict] = []
        self.cache = (ROOT / '.cache/docs-markdown-lint').resolve()
        checked(['make', 'docs-reference-check'], ROOT)
        self.makefile = (ROOT / 'Makefile').read_text(encoding='utf-8')
        self.config = (ROOT / '.gomarklint-docs.json').read_text(encoding='utf-8')
        self.schema = (ROOT / '.gomarklint-docs.schema.json').read_text(encoding='utf-8')
        tool = self.cache / 'bin' / ('gomarklint.exe' if os.name == 'nt' else 'gomarklint')
        version = checked([str(tool), '--version'], ROOT)
        identity = checked(['go', 'version', '-m', str(tool)], ROOT)
        assert 'github.com/shinagawa-web/gomarklint/v3\tv3.3.1' in identity, identity
        write(artifacts, 'markdown-tools.txt', identity + version +
              (ROOT / 'scripts/docs-markdown-lint-requirements.txt').read_text())
        print(version.strip(), flush=True)

    def root(self, label: str) -> Path:
        root = self.artifacts / label
        root.mkdir()
        write(root, 'Makefile', self.makefile)
        write(root, '.gomarklint-docs.json', self.config)
        write(root, '.gomarklint-docs.schema.json', self.schema)
        write(root, 'docs/README.md', '# Valid\n')
        write(root, 'docs/reference/valid.md', '# Valid\n')
        return root

    def run(self, root: Path, label: str, expected: tuple[str, ...] = (),
            variables: tuple[str, ...] = (), env: dict | None = None) -> None:
        # The explicit fixture inputs are the observer; no source inventory check.
        paths = ['docs/README.md', 'docs/reference/valid.md', 'docs/reference/nested/.hidden/notes.MD']
        before = {}
        for path in paths:
            try:
                before[path] = (root / path).read_bytes()
            except (FileNotFoundError, PermissionError):
                # Missing/inaccessible inputs are recipe faults, not observer faults.
                # Keep permission handling confined to snapshot collection.
                continue
        result = execute(['make', '-o', self.cache.as_posix() + '/ready',
                          'docs-reference-check', 'DOCS_MARKDOWN_CACHE=' + self.cache.as_posix(),
                          *variables], root, env)
        output = result.stdout + result.stderr
        write(root, label + '.log', output)
        assert (result.returncode != 0) == bool(expected), f'{label}: exit {result.returncode}\n{output}'
        for message in expected:
            assert message in output, f'{label}: missing {message}\n{output}'
        for path, content in before.items():
            assert (root / path).read_bytes() == content, f'{label}: modified {path}'
        self.results.append({'case': label, 'exit': result.returncode, 'expected': expected})
        print(f'PASS {label}: exit={result.returncode}', flush=True)

    def content(self) -> None:
        root = self.root('M02-scope')
        nested = root / 'docs/reference/nested/.hidden/notes.MD'
        nested.parent.mkdir(parents=True)
        nested.write_bytes(b'# Hidden\n')
        write(root, 'docs/reference/space name.md', '# Space\n')
        write(root, 'docs/reference/ignored.txt', 'no newline')
        self.run(root, 'M02-valid')
        nested.write_bytes(b'```\n')
        self.run(root, 'M02-hidden-invalid', ('docs/reference/nested/.hidden/notes.MD', 'Unclosed code block'))
        for label, content, diagnostic in (
            ('M03-backtick', b'```go\ntext\n', 'Unclosed code block'),
            ('M03-tilde', b'~~~\ntext\n', 'Unclosed code block'),
            ('M03-mismatch', b'```\ntext\n~~~\n', 'Unclosed code block'),
            ('M03-short', b'````\ntext\n```\n', 'Unclosed code block'),
            ('M04-newline', b'# Title', 'MD047'),
            ('M04-frontmatter', b'---\ntitle: Example\n---', 'MD047'),
            ('M05-encoding', b'\xff\n', 'iconv'),
            ('M11-CR-only', b'# Title\r', 'Missing final blank line'),
        ):
            root = self.root(label)
            (root / 'docs/README.md').write_bytes(content)
            self.run(root, label, ('docs/README.md', diagnostic))
        for label, content in (
            ('empty', b''), ('backtick', b'```go\ntext\n```\n'),
            ('tilde', b'~~~\ntext\n~~~\n'), ('CRLF', b'# Title\r\n'),
            ('long-line', b'a' * 70000 + b'\n'),
            ('outer-fence', b'````\n```literal\n````\n'),
        ):
            root = self.root('M10-' + label)
            (root / 'docs/README.md').write_bytes(content)
            self.run(root, 'M10-' + label)

    def inputs(self) -> None:
        for operand in ('docs/README.md', 'docs/reference'):
            root = self.root('M06-' + Path(operand).name)
            path = root / operand
            if path.is_dir():
                shutil.rmtree(path)
            else:
                path.unlink()
            self.run(root, 'M06-missing', (operand, 'find'))
        if os.name == 'nt' or os.geteuid() == 0:
            self.results.append({'case': 'M07-permissions', 'unavailable':
                                 'Requires nonprivileged POSIX identity; hosted Backend Lint owns proof'})
            print('UNAVAILABLE M07: requires nonprivileged POSIX identity', flush=True)
        else:
            for operand in ('docs/README.md', 'docs/reference/nested'):
                root = self.root('M07-' + Path(operand).name)
                path = root / operand
                if operand.endswith('nested'):
                    write(root, operand + '/guide.md', '# Valid\n')
                source = path / 'guide.md' if path.is_dir() else path
                content = source.read_bytes()
                path.chmod(0)
                try:
                    self.run(root, 'M07-unreadable', (operand, 'Permission denied'))
                finally:
                    path.chmod(0o700)
                assert source.read_bytes() == content, f'M07-unreadable: modified {source}'

    def configuration(self) -> None:
        for label, content in (
            ('malformed', '{'), ('unknown', self.config.replace('final-blank-line', 'unknown')),
            ('disabled', self.config.replace('"error"', '"off"')),
            ('missing-rule', '{"default":false,"output":"text","rules":{}}'),
        ):
            root = self.root('M08-' + label)
            write(root, '.gomarklint-docs.json', content)
            self.run(root, 'M08-' + label, ('.gomarklint-docs.json',))
        for name in ('.gomarklint-docs.json', '.gomarklint-docs.schema.json'):
            root = self.root('M08-missing-' + name)
            (root / name).unlink()
            self.run(root, 'M08-missing', (name,))
        root = self.root('M08-malformed-schema')
        write(root, '.gomarklint-docs.schema.json', '{')
        self.run(root, 'M08-malformed-schema', ('.gomarklint-docs.schema.json',))
        root = self.root('M08-pin')
        write(root, 'scripts/docs-markdown-lint-requirements.txt', 'pymarkdownlnt==invalid-version\n')
        # Force the real preparation recipe offline, with an impossible exact pin.
        write(root, 'Makefile', self.makefile.replace('github.com/shinagawa-web/gomarklint/v3@v3.3.1',
                                                    'github.com/shinagawa-web/gomarklint/v3@v999.999.999'))
        result = execute(['make', 'docs-reference-check',
                          'DOCS_MARKDOWN_CACHE=' + (root / 'cache').as_posix()], root,
                         {'GOPROXY': 'off'})
        output = result.stdout + result.stderr
        write(root, 'go-pin.log', output)
        assert result.returncode and 'v999.999.999' in output, output
        self.results.append({'case': 'M08-go-pin', 'exit': result.returncode})
        write(root, 'Makefile', self.makefile)
        self.run(root, 'M08-python-pin', ('invalid-version',),
                 ('DOCS_MARKDOWN_CACHE=' + (root / 'cache').as_posix(), 'GO=true'),
                 {'PIP_NO_INDEX': '1'})

    def unavailable(self) -> None:
        for command in ('find', 'iconv'):
            root = self.root('M09-' + command)
            write(root, 'Makefile', self.makefile.replace(command + ' ', 'missing-' + command + ' '))
            self.run(root, 'M09-' + command, ('missing-' + command,))
        for variable in ('PYTHON', 'GO'):
            root = self.root('M09-' + variable)
            overrides = (variable + '=missing-' + variable,)
            if variable == 'GO':
                overrides += ('DOCS_MARKDOWN_CACHE=' + (root / 'cache').as_posix(),)
                write(root, 'scripts/docs-markdown-lint-requirements.txt', '')
            self.run(root, 'M09-' + variable, ('missing-' + variable,), overrides)
        root = self.root('M09-pip')
        write(root, 'scripts/docs-markdown-lint-requirements.txt', '')
        self.run(root, 'M09-pip', ('No module named pip',),
                 ('GO=true', 'PYTHON=python -S', 'DOCS_MARKDOWN_CACHE=' + (root / 'cache').as_posix()))
        root = self.root('M09-linter')
        write(root, 'Makefile', self.makefile.replace('/gomarklint$(if', '/missing-gomarklint$(if'))
        self.run(root, 'M09-linter', ('missing-gomarklint',))

    def scenarios(self) -> None:
        self.results.append({'case': 'M01-current-docs', 'exit': 0})
        self.content()
        self.inputs()
        self.configuration()
        self.unavailable()


def construction_smoke_sources(root: Path, seeded: bool) -> list[tuple[str, str]]:
    """Compiler-valid declarations exercise the production registry, not a test registry."""
    module = "github.com/portpowered/infinite-you"
    owner = "pkg/services/chat_sessions/internal/service"
    for path, name in (("pkg/services/operator_settings", "Service"),
                       ("pkg/services/factory_definitions", "CatalogPathsService"),
                       ("pkg/platform/logging", "Logger")):
        write(root, path + "/contract.go", f"package {Path(path).name}\ntype {name} interface {{ Observe() }}\n")
    imports = (f'import settings "{module}/pkg/services/operator_settings"\n'
               f'import definitions "{module}/pkg/services/factory_definitions"\n'
               f'import logging "{module}/pkg/platform/logging"\n')
    parameters = "settings.Service, definitions.CatalogPathsService, logging.Logger"
    # Keep the exact registered signature; omission from constructor metadata
    # must not hide a newly introduced constructor returning the same type.
    write(root, owner + "/service.go", "package service\n" + imports +
          "type Service struct{}\n" +
          f"func New({parameters}) *Service {{ return &Service{{}} }}\n" +
          f"func NewAlternate({parameters}) *Service {{ return &Service{{}} }}\n" +
          ("func Operation() { New(nil, nil, nil) }\n" if seeded else ""))
    write(root, "pkg/services/chat_sessions/wire/provider.go", "package wire\n" + imports +
          f'import service "{module}/{owner}"\n' +
          "func NewCatalog(a settings.Service, b definitions.CatalogPathsService, c logging.Logger) *service.Service {\n"
          " return service.New(a, b, c)\n}\n")
    write(root, "pkg/services/chat_sessions/internal/consumer/consumer.go", "package consumer\n" +
          (f'import service "{module}/{owner}"\n'
           "func Operation() { service.NewAlternate(nil, nil, nil) }\n" if seeded else
           "type Scope struct { Values map[string]string }\n"
           "func NewScope() *Scope { return &Scope{Values: make(map[string]string)} }\n"))
    write(root, "pkg/services/unrelated/internal/consumer/consumer.go", "package consumer\n" +
          (f'import catalog "{module}/pkg/services/chat_sessions/wire"\n'
           "func Operation() { catalog.NewCatalog(nil, nil, nil) }\n" if seeded else ""))
    events = "pkg/services/events"
    store = events + "/internal/service"
    write(root, events + "/service.go", "package events\ntype Service interface { Observe() }\n")
    write(root, store + "/store.go", "package service\n" +
          f'import logging "{module}/pkg/platform/logging"\n' +
          f'import events "{module}/{events}"\n' +
          "type topicState struct { Values map[string]string }\n"
          "type Store struct { logger logging.Logger; topics map[string]*topicState; peer events.Service }\n"
          "func NewWithRetention(retention int, logger logging.Logger) *Store {\n" +
          (" if logger == nil { return nil }\n" if seeded else "") +
          " if retention <= 0 { retention = 10000 }; _ = retention\n"
          " return &Store{logger: logger, topics: make(map[string]*topicState)}\n}\n"
          "func (s *Store) Observe() { s.logger.Observe() }\n"
          "func (s *Store) Topic() { s.topics[\"topic\"] = &topicState{Values: make(map[string]string)} }\n" +
          "func NewGeneric[T any](value T, logger logging.Logger) *Store { _ = value; return &Store{logger: logger} }\n" +
          "func NewPeer(peer events.Service) *Store {\n" +
          (" if peer == nil { return nil }\n" if seeded else "") +
          " return &Store{peer: peer}\n}\n"
          "func (s *Store) Peer() events.Service { return s.peer }\n"
          "func (s *Store) PeerView(_ string) events.Service { return s.peer }\n"
          "func (s *Store) View() { s.PeerView(\"scope\") }\n" +
          ("func (s *Store) UsePeer() { if s.peer != nil { s.peer.Observe() }; s.Peer() }\n"
           "func (s *Store) EscapePeer() any { return s.Peer }\n" if seeded else "") +
          ("func Operation(logger logging.Logger) { NewWithRetention(0, logger); NewGeneric(\"scope\", logger) }\n" if seeded else ""))
    write(root, events + "/wire/provider.go", "package wire\n" +
          f'import logging "{module}/pkg/platform/logging"\n' +
          f'import events "{module}/{events}"\n' +
          f'import service "{module}/{store}"\n' +
          "func NewService(logger logging.Logger) (events.Service, error) {\n" +
          (" if logger == nil { return nil, nil }\n" if seeded else "") +
          " return service.NewWithRetention(0, logger), nil\n}\n")
    # The consumer sees compiler object facts, including promoted method
    # identity; it must not need the owning package's source-body index.
    write(root, events + "/internal/consumer/consumer.go", "package consumer\n" +
          f'import service "{module}/{store}"\n' +
          "type View struct { *service.Store }\n"
          "func Scoped(view *View) { view.PeerView(\"scope\") }\n" +
          ("func Use(view *View) { view.Peer() }\n"
           "func Escape(view *View) any { return view.Peer }\n" if seeded else ""))
    write(root, events + "/internal/consumer/dot.go", "package consumer\n" +
          (f'import . "{module}/{store}"\n' if seeded else "") +
          f'import logging "{module}/pkg/platform/logging"\n' +
          "func Shadow(logger logging.Logger) {\n"
          " NewWithRetention := func(_ int, _ logging.Logger) int { return 1 }\n"
          " _ = NewWithRetention(0, logger)\n}\n" +
          ("func Dot(logger logging.Logger) { NewWithRetention(0, logger) }\n" if seeded else ""))
    # Keep the operational caller in the existing private consumer package;
    # public service roots own contracts rather than floating operations.
    write(root, "pkg/services/unrelated/internal/consumer/events.go", "package consumer\n" +
          (f'import eventswire "{module}/{events}/wire"\n'
           "func EventsOperation() { eventswire.NewService(nil) }\n" if seeded else ""))
    return [] if not seeded else [
        ("repolint", f"{module}/{owner}.Operation->{module}/{owner}.New"),
        ("repolint", f"{module}/pkg/services/chat_sessions/internal/consumer.Operation->{module}/{owner}.NewAlternate"),
        ("repolint", f"{module}/pkg/services/unrelated/internal/consumer.Operation->{module}/pkg/services/chat_sessions/wire.NewCatalog"),
        ("repolint", "service-construction: pkg/services/unrelated/internal/consumer -> pkg/services/chat_sessions/wire.NewCatalog"),
        ("repolint", "service-subpackage: pkg/services/unrelated/internal/consumer -> pkg/services/chat_sessions/wire"),
        ("repolint", f"{module}/{store}.Operation->{module}/{store}.NewWithRetention"),
        ("repolint", f"{module}/{store}.Operation->{module}/{store}.NewGeneric"),
        ("repolint", f"{module}/{events}/internal/consumer.Dot->{module}/{store}.NewWithRetention"),
        ("repolint", f"{module}/pkg/services/unrelated/internal/consumer.EventsOperation->{module}/{events}/wire.NewService"),
        ("repolint", f"service-construction: pkg/services/unrelated/internal/consumer -> {events}/wire.NewService"),
        ("repolint", f"service-subpackage: pkg/services/unrelated/internal/consumer -> {events}/wire"),
        ("repolint", f"required-dependency-guard: {store} -> {module}/{store}.NewWithRetention->{module}/{store}.NewWithRetention"),
        ("repolint", f"required-dependency-guard: {events}/wire -> {module}/{events}/wire.NewService->{module}/{events}/wire.NewService"),
        ("repolint", f"required-dependency-guard: {store} -> {module}/{store}.NewPeer->{module}/{store}.NewPeer"),
        ("repolint", f"{module}/{store}.(Store).UsePeer->{module}/{store}.NewPeer"),
        ("repolint", f"service-getter-locator: {store} -> {module}/{store}.(Store).UsePeer->{module}/{store}.(Store).Peer"),
        ("repolint", f"unresolved-service-getter-reference: {store} -> {module}/{store}.(Store).EscapePeer->{module}/{store}.(Store).Peer"),
        ("repolint", f"service-getter-locator: {events}/internal/consumer -> {module}/{events}/internal/consumer.Use->{module}/{store}.(Store).Peer"),
        ("repolint", f"unresolved-service-getter-reference: {events}/internal/consumer -> {module}/{events}/internal/consumer.Escape->{module}/{store}.(Store).Peer"),
    ]


def provider_sessions_smoke_sources(root: Path, seeded: bool) -> list[tuple[str, str]]:
    """Use exact delivered constructor signatures with the production registry."""
    module = "github.com/portpowered/infinite-you"
    owner = "pkg/services/provider_sessions"
    private = owner + "/internal/service"
    transport = owner + "/transports/http"
    caller = "pkg/services/new_caller/internal/consumer"
    # One root interface serves both typed captured-reader construction and
    # the independent historical-recording diagnostic control.
    write(root, "pkg/services/recordings/reader.go",
          "package recordings\ntype WorkerCapturedActivityReader interface {\n"
          " Read()\n QueryHistoricalRecording() Recording\n}\n")
    # The HTTP signature consumes a Zap pointer, not the platform Logger view.
    write(root, "stubs/zap/go.mod", "module go.uber.org/zap\n\ngo 1.25.0\n")
    write(root, "stubs/zap/logger.go", "package zap\ntype Logger struct{}\n")
    write(root, owner + "/contract.go", "package provider_sessions\ntype Service interface { Observe() }\n")
    write(root, private + "/service.go", "package service\n" +
          f'import sessions "{module}/{owner}"\n' +
          f'import recordings "{module}/pkg/services/recordings"\n' +
          "type inspectionService struct{ reader recordings.WorkerCapturedActivityReader }\n"
          "func New(reader recordings.WorkerCapturedActivityReader) (sessions.Service, error) {\n" +
          (" if reader == nil { return nil, nil }\n" if seeded else "") +
          " return &inspectionService{reader: reader}, nil\n}\n"
          "func (s *inspectionService) Observe() {\n" +
          (" if s == nil { return }\n if s.reader == nil { return }\n" if seeded else "") +
          " s.reader.Read()\n}\n" +
          ("func Operation(reader recordings.WorkerCapturedActivityReader) { New(reader) }\n" if seeded else ""))
    write(root, owner + "/wire/provider.go", "package wire\n" +
          f'import sessions "{module}/{owner}"\n' +
          f'import recordings "{module}/pkg/services/recordings"\n' +
          f'import service "{module}/{private}"\n' +
          "func NewService(reader recordings.WorkerCapturedActivityReader) (sessions.Service, error) { return service.New(reader) }\n")
    write(root, transport + "/adapter.go", "package http\n" +
          f'import sessions "{module}/{owner}"\n' +
          'import zap "go.uber.org/zap"\n' +
          "type Adapter struct{ sessions sessions.Service }\n"
          "type Handler struct{ adapter *Adapter; logger *zap.Logger }\n"
          "func NewAdapter(peer sessions.Service) *Adapter { return &Adapter{sessions: peer} }\n"
          "func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {\n" +
          (" if logger == nil { return nil }\n" if seeded else "") +
          " return &Handler{adapter: adapter, logger: logger}\n}\n"
          "func (a *Adapter) Peer() sessions.Service { return a.sessions }\n"
          "func (a *Adapter) View(id string) sessions.Service { _ = id; return a.sessions }\n" +
          ("func (a *Adapter) Use() { a.Peer().Observe() }\n"
           "func (a *Adapter) Escape() any { return a.Peer }\n" if seeded else ""))
    write(root, "pkg/services/new_caller/contract.go",
          "package new_caller\n" +
          f'import recordings "{module}/pkg/services/recordings"\n' +
          "type Service interface { Run(recordings.WorkerCapturedActivityReader) }\n")
    write(root, caller + "/caller.go", "package consumer\n" +
          (f'import wire "{module}/{owner}/wire"\n' +
           f'import recordings "{module}/pkg/services/recordings"\n' +
           "func Run(reader recordings.WorkerCapturedActivityReader) { wire.NewService(reader) }\n" if seeded else ""))
    write(root, "pkg/wire/provider_sessions.go", "package wire\n" +
          f'import sessions "{module}/{owner}"\n' +
          f'import transport "{module}/{transport}"\n' +
          'import zap "go.uber.org/zap"\n' +
          "func ProvideProviderSessions(peer sessions.Service, logger *zap.Logger) *transport.Handler {\n"
          " return transport.NewHandler(transport.NewAdapter(peer), logger)\n}\n")
    return [
        ("repolint", f"registered-construction: {private} -> {module}/{private}.Operation->{module}/{private}.New"),
        ("repolint", f"service-construction: {caller} -> {owner}/wire.NewService"),
        ("repolint", f"service-subpackage: {caller} -> {owner}/wire"),
        ("repolint", f"registered-construction: {caller} -> {module}/{caller}.Run->{module}/{owner}/wire.NewService"),
        ("repolint", f"required-dependency-guard: {private} -> {module}/{private}.New->{module}/{private}.New"),
        ("repolint", f"required-receiver-guard: {private} -> {module}/{private}.(inspectionService).Observe->{module}/{private}.New"),
        ("repolint", f"required-dependency-guard: {private} -> {module}/{private}.(inspectionService).Observe->{module}/{private}.New"),
        ("repolint", f"required-dependency-guard: {transport} -> {module}/{transport}.NewHandler->{module}/{transport}.NewHandler"),
        ("repolint", f"service-getter-locator: {transport} -> {module}/{transport}.(Adapter).Use->{module}/{transport}.(Adapter).Peer"),
        ("repolint", f"unresolved-service-getter-reference: {transport} -> {module}/{transport}.(Adapter).Escape->{module}/{transport}.(Adapter).Peer"),
    ]


def ci_smoke(tool: list[str], artifacts: Path) -> None:
    """Clean/seeded/recovered real plugin checks share one warm module."""
    started = time.monotonic()
    config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
    fixtures = SizeFixtures(tool, artifacts, config)
    root = fixtures.module("ci-smoke")
    write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n"
          "require go.uber.org/zap v0.0.0\nreplace go.uber.org/zap => ./stubs/zap\n")
    manifest = "pkg/transports/cli/root_work.go"
    consumer = "internal/consumer/source.go"
    clean_manifest = "package cli\nfunc executeWork(inputID string) string { return inputID }\n"
    clean_consumer = "package consumer\n"
    write(root, "pkg/wire/wire.go", "package wire\n")
    write(root, manifest, clean_manifest)
    write(root, consumer, clean_consumer)
    write(root, "packages/packaged-factories/publication.go", "package packagedfactories\nconst Bytes = 1\n")
    write(root, "pkg/services/recordings/recording.go",
          "package recordings\ntype Recording struct{}\n")
    construction_smoke_sources(root, False)
    provider_sessions_smoke_sources(root, False)
    complete_tags = "integration,functionallong,backendconformance,factoryartifact,managed_process_integration"

    try:
        fixtures.lint(root, "clean", [])
        fixtures.lint(root, "clean-complete", [], tags=complete_tags)
        write(root, "cmd/vetfixture/source.go", '// Code generated by fixture. DO NOT EDIT.\npackage fixture\nimport "fmt"\nfunc witness() { fmt.Printf("%d", "text") }\n')
        write(root, manifest, "package cli\nfunc newRunCommand() {}\n")
        write(root, consumer, 'package consumer\nimport _ "github.com/portpowered/infinite-you/packages/packaged-factories"\n')
        write(root, "pkg/wire/wire.go", 'package wire\nimport "github.com/portpowered/infinite-you/pkg/services/recordings"\nfunc forbidden(reader recordings.WorkerCapturedActivityReader) { _ = reader.QueryHistoricalRecording() }\n')
        construction = construction_smoke_sources(root, True)
        construction += provider_sessions_smoke_sources(root, True)
        expected_issues = [
            ("repolint", "recording-read:"),
            ("repolint", "cli-manifest-authority:"),
            ("repolint", "packaged-factory-direct-publication:"),
            ("govet", "wrong type"),
        ] + construction
        issues = fixtures.lint(root, "seeded", expected_issues)
        complete_issues = fixtures.lint(root, "seeded-complete", expected_issues, tags=complete_tags)
        construction_issues = [issue for issue in issues if "registered-construction:" in issue["Text"]]
        assert len(construction_issues) == 9, construction_issues
        complete_construction = [issue for issue in complete_issues if "registered-construction:" in issue["Text"]]
        assert len(complete_construction) == 9, complete_construction
        expected = {manifest: "cli-manifest-authority:", consumer: "packaged-factory-direct-publication:"}
        for name, diagnostic in expected.items():
            assert any((issue["Pos"]["Filename"].replace("\\", "/") == name
                        or issue["Pos"]["Filename"].replace("\\", "/").endswith("/" + name))
                       and issue["Pos"]["Line"] == 2 and diagnostic in issue["Text"] for issue in issues), issues
        write(root, manifest, clean_manifest)
        write(root, consumer, clean_consumer)
        write(root, "pkg/wire/wire.go", "package wire\n")
        write(root, "cmd/vetfixture/source.go", "package fixture\n")
        construction_smoke_sources(root, False)
        provider_sessions_smoke_sources(root, False)
        fixtures.lint(root, "recovered", [])
        fixtures.lint(root, "recovered-complete", [], tags=complete_tags)
    finally:
        write(artifacts, "results.json", json.dumps(fixtures.results, indent=2) + "\n")
        print(f"ci-smoke elapsed: {time.monotonic() - started:.3f}s; artifacts: {artifacts}", flush=True)
    print("PASS ci-smoke: clean/seeded/recovered real plugin diagnostics", flush=True)


def lint_r2_seed(seed: str, binary: Path, artifacts: Path) -> None:
    """Own-head canonical checks consume one externally prepared plugin.

    Run one class per bounded visit. This is a dedicated static witness, not
    a unit/functional test, and never builds a replacement lint artifact.
    """
    import importlib.util

    root = artifacts / "checkout"
    checked(["git", "-c", "core.longpaths=true", "clone", "--quiet", "--shared", str(ROOT), str(root)], ROOT)
    anchor = checked(["git", "rev-parse", "origin/main"], ROOT).strip()
    checked(["git", "update-ref", "refs/remotes/origin/main", anchor], root)
    head = checked(["git", "rev-parse", "HEAD"], root).strip()
    artifact = root / ".artifacts/golangci"
    artifact.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(binary.parent, artifact, ignore=shutil.ignore_patterns("current-host", "host-path.txt"))
    spec = importlib.util.spec_from_file_location("lint_builder", ROOT / "scripts/build-golangci.py")
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    builder.restore(artifact, root)
    baselines = {name: (root / name).read_bytes() for name in (
        "docs/internal/baselines/deadcode-baseline.txt", "internal/lint/analyzers/baseline.txt")}
    cases = {
        "golangci": ("pkg/platform/clock/clock.go", "log", 'func lintR2Seed() { log.Print("lint r2 seed") }\n', "golangci", "forbidigo"),
        "govet": ("cmd/factory/main.go", "fmt", 'func lintR2Seed() { fmt.Printf("%d", "lint r2 seed") }\n', "golangci", "govet"),
        "deadcode": ("pkg/platform/clock/clock.go", "", "func LintR2UnusedSeed() {}\n", "deadcode", "LintR2UnusedSeed"),
        "docs": ("docs/reference/run.md", "", "", "docs-reference-check", "MD047"),
    }
    name, imported, function, target, diagnostic = cases[seed]
    path = root / name
    original = path.read_bytes()
    results = []
    environment = dict(os.environ, GOWORK="off", GOLANGCI_LINT_CACHE=str(artifacts / "analysis-cache"))
    command = ["make", target, "GOLANGCI_PREBUILT=1"]
    # Docs deliberately sets GOTOOLCHAIN=local; select the compiler already
    # resolved by this module instead of falling back to an older PATH Go.
    compiler_bin = Path(checked(["go", "env", "GOROOT"], ROOT).strip()) / "bin"
    compiler = compiler_bin / ("go.exe" if os.name == "nt" else "go")
    command.append("GO=" + compiler.as_posix())
    environment["PATH"] = str(compiler_bin) + os.pathsep + environment["PATH"]
    if os.name == "nt":
        # system32/bash.exe is the WSL launcher on the worker host. Native
        # Make requires the installed Git POSIX shell, with no spaces in its
        # executable path; do not change any repository recipe for this host.
        import ctypes
        shell = Path(os.environ.get("ProgramW6432", os.environ["ProgramFiles"])) / "Git/bin/sh.exe"
        if not shell.is_file():
            raise RuntimeError("r2-seeds requires the installed Git POSIX shell")
        buffer = ctypes.create_unicode_buffer(32768)
        if not ctypes.windll.kernel32.GetShortPathNameW(str(shell), buffer, len(buffer)):
            raise RuntimeError("cannot resolve Git shell path for native Make")
        command.append("SHELL=" + buffer.value.replace("\\", "/"))
        environment["PATH"] = str(shell.parent) + os.pathsep + environment["PATH"]

    def check(label, expected):
        started = time.monotonic()
        result = subprocess.run(command, cwd=root, env=environment, capture_output=True,
                                text=True, encoding="utf-8", errors="replace", timeout=180)
        output = result.stdout + result.stderr
        write(artifacts, label + ".log", output)
        results.append({"case": label, "head": head, "command": command,
                        "exit": result.returncode, "seconds": time.monotonic() - started})
        assert (result.returncode == 0) == (expected == 0), f"{label}: unexpected exit; see {artifacts / (label + '.log')}"
        diagnostic_output = output
        if seed == "deadcode":
            # The canonical checker publishes named findings in its report;
            # stdout/stderr contains only the verdict and aggregate counts.
            diagnostic_output = (root / "bin/deadcode-current.txt").read_text(encoding="utf-8")
            write(artifacts, label + ".txt", diagnostic_output)
            baseline = baselines["docs/internal/baselines/deadcode-baseline.txt"].decode("utf-8")
            expected_report = baseline.splitlines()
            if expected:
                expected_report.append(f"{name}: unreachable func: {diagnostic}")
                assert "deadcode baseline drift detected" in output, f"{label}: missing drift verdict"
            assert sorted(diagnostic_output.splitlines()) == sorted(expected_report), f"{label}: unexpected finding delta"
        if expected:
            assert diagnostic in diagnostic_output, f"{label}: missing {diagnostic} diagnostic"
            assert name in diagnostic_output.replace("\\", "/"), f"{label}: missing seeded filename"
        for baseline, content in baselines.items():
            assert (root / baseline).read_bytes() == content, f"{label}: changed baseline"
        print(f"PASS {label}: exit={result.returncode}", flush=True)

    try:
        check(seed + "-clean", 0)
        if seed == "docs":
            path.write_bytes(original.rstrip(b"\r\n"))
        else:
            source = original.decode("utf-8")
            if imported:
                source = source.replace("import (", f'import (\n\t"{imported}"', 1)
            path.write_text(source + "\n" + function, encoding="utf-8")
        # The clean pass warms analysis. Mutation must invalidate it.
        check(seed + "-seeded-warm", 1)
        path.write_bytes(original)
        check(seed + "-recovered", 0)
    finally:
        path.write_bytes(original)
        write(artifacts, "results.json", json.dumps(results, indent=2) + "\n")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cohort", choices=("r2-seeds", "ci-smoke", "size", "pkg-rules", "manifest", "baseline", "owners", "service-cycle", "provider-catalog", "catalog", "consumption", "package-boundary", "packaged-source", "markdown", "all"))
    parser.add_argument("--golangci")
    parser.add_argument("--seed", choices=("golangci", "govet", "deadcode", "docs"))
    args = parser.parse_args()
    if args.cohort == "r2-seeds":
        if not args.seed or not args.golangci or not Path(args.golangci).is_file():
            parser.error("r2-seeds requires --seed and an externally prepared --golangci binary/host")
        parent = ROOT / ".artifacts/lint-migration-smoke"
        parent.mkdir(parents=True, exist_ok=True)
        artifacts = Path(tempfile.mkdtemp(prefix="r2-" + args.seed + "-", dir=parent))
        lint_r2_seed(args.seed, Path(args.golangci).resolve(), artifacts)
        print(f"PASS r2-seeds {args.seed}; artifacts: {artifacts}", flush=True)
        return
    if args.cohort == "ci-smoke":
        if not args.golangci or not Path(args.golangci).is_file():
            parser.error("ci-smoke requires a real prebuilt --golangci binary")
        parent = ROOT / ".artifacts/lint-migration-smoke"
        parent.mkdir(parents=True, exist_ok=True)
        artifacts = Path(tempfile.mkdtemp(prefix="ci-smoke-", dir=parent))
        ci_smoke([str(Path(args.golangci).resolve())], artifacts)
        return
    if args.cohort != "markdown" and not args.golangci:
        parser.error("--golangci is required except for markdown")
    if args.cohort in ("markdown", "all"):
        parent = ROOT / ".artifacts/lint-migration-smoke"
        parent.mkdir(parents=True, exist_ok=True)
        artifacts = Path(tempfile.mkdtemp(prefix="markdown-", dir=parent))
        markdown = MarkdownFixtures(artifacts)
        try:
            markdown.scenarios()
        finally:
            write(artifacts, "results.json", json.dumps(markdown.results, indent=2) + "\n")
        print(f"PASS markdown: {len(markdown.results)} scenarios; artifacts: {artifacts}", flush=True)
        if args.cohort == "markdown":
            return
    tool = ([str(Path(args.golangci).resolve())] if Path(args.golangci).is_file()
            else shlex.split(args.golangci))
    version = checked(tool + ["version"], ROOT)
    assert ("version 2.11.4 " in version or "version v2.11.4-custom-gcl-" in version), (
        f"expected pinned golangci-lint v2.11.4: {version}")
    checked(tool + ["config", "verify"], ROOT)
    parent = ROOT / ".artifacts/lint-migration-smoke"
    parent.mkdir(parents=True, exist_ok=True)
    artifacts = Path(tempfile.mkdtemp(prefix=args.cohort + "-", dir=parent))
    write(artifacts, "tool.txt", version)
    print(f"Artifacts: {artifacts}\n{version.strip()}", flush=True)
    fixtures = SizeFixtures(tool, artifacts, (ROOT / ".golangci.yml").read_text(encoding="utf-8"))
    try:
        if args.cohort in ("provider-catalog", "all"):
            fixtures.config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
            root = fixtures.module("provider-catalog")
            write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(root, "internal/providercatalog/source.go", "package providercatalog\n")
            write(root, "pkg/wire/wire.go", "package wire\n")
            write(root, "pkg/transports/fixture/source.go", "package fixture\n")
            names = checked(["git", "ls-files", "-z", "--", "packages/model-providers/providers"], ROOT).split("\0")
            names += ["api/openapi.yaml"] + ["packages/model-providers/generated/" + name for name in
                       ("catalog.json", "provider-manifest.schema.json", "provider-catalog.schema.json", "runtime-acp.json")]
            for name in filter(None, names):
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes((ROOT / name).read_bytes())
            checked(["git", "init", "-q"], root)
            checked(["git", "add", "."], root)
            fixtures.commit(root)
            checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
            tracked = {name: (root / name).read_bytes() for name in names if name}
            fixtures.lint(root, "provider-clean-default", [])
            write(root, ".golangci.yml", (ROOT / ".golangci-repository.yml").read_text(encoding="utf-8"))
            fixtures.lint(root, "provider-clean-complete", [])
            manifest = "packages/model-providers/providers/claude/provider.yaml"
            original = (root / manifest).read_bytes()
            changed = original.replace(b"Anthropic Claude Code CLI integration.", b"Claude CLI description changed for cache witness.")
            assert changed != original
            (root / manifest).write_bytes(changed)
            fixtures.lint(root, "provider-yaml-only-warm-cache", [("repolint", "provider-catalog: stale:")])
            assert (root / manifest).read_bytes() == changed
            assert (root / "packages/model-providers/generated/catalog.json").read_bytes() == tracked["packages/model-providers/generated/catalog.json"]
            (root / manifest).write_bytes(original)
            fixtures.lint(root, "provider-yaml-recovery", [])
            projection = root / "packages/model-providers/generated/runtime-acp.json"
            projection.unlink()
            fixtures.lint(root, "provider-missing-warm-cache", [("repolint", "provider-catalog: missing:")])
            assert not projection.exists()
            projection.write_bytes(tracked["packages/model-providers/generated/runtime-acp.json"])
            write(root, "packages/model-providers/generated/unused.txt", "unused")
            fixtures.lint(root, "provider-output-recovery-extra-ignored", [])
            for name, payload in tracked.items():
                assert (root / name).read_bytes() == payload, f"lint modified {name}"
            # Real Git ignored/deleted acquisition belongs in this static lane.
            scaffold = "packages/model-providers/providers/new/scaffold.txt"
            write(root, ".gitignore", "scaffold.txt\n")
            write(root, scaffold, "ignored populated input")
            fixtures.lint(root, "provider-ignored-populated-input", [("repolint", "new/provider.yaml")])
            assert (root / scaffold).read_text() == "ignored populated input"
            checked(["git", "add", "-f", scaffold], root)
            fixtures.lint(root, "provider-tracked-populated-input", [("repolint", "new/provider.yaml")])
            (root / scaffold).unlink()
            fixtures.lint(root, "provider-deleted-tracked-input-recovery", [])
            for name, payload in tracked.items():
                assert (root / name).read_bytes() == payload, f"lint modified {name}"
            outside = fixtures.module("provider-unrelated")
            write(outside, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(outside, "internal/unrelated/source.go", "package unrelated\n")
            checked(["git", "init", "-q"], outside)
            fixtures.lint(outside, "provider-unrelated-owner", [])
        if args.cohort in ("packaged-source", "all"):
            packaged_source(fixtures)
        if args.cohort in ("catalog", "all"):
            fixtures.config = (ROOT / ".golangci-repository.yml").read_text(encoding="utf-8")
            root = fixtures.module("catalog-publication")
            write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(root, "pkg/wire/wire.go", "package wire\n")
            write(root, "pkg/transports/fixture/source.go", "package fixture\n")
            write(root, "internal/lint/analyzers/source.go", "package analyzers\n")
            write(root, "internal/lint/analyzers/baseline.txt", "")
            checked(["git", "init", "-q"], root)
            checked(["git", "config", "core.longpaths", "true"], root)
            checked(["git", "add", "."], root)
            fixtures.commit(root)
            checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
            package = root / "packages/packaged-factories"
            # The same Go sources/cache survive every non-Go mutation.
            before = {p.relative_to(package): p.read_bytes() for p in package.rglob("*") if p.is_file()}
            fixtures.lint(root, "catalog-clean", [])
            assert before == {p.relative_to(package): p.read_bytes() for p in package.rglob("*") if p.is_file()}
            manifest = package / "generated/manifest.json"
            manifest.write_text("{}\n", encoding="utf-8")
            fixtures.lint(root, "catalog-stale-cached", [("repolint", "stale: generated/manifest.json")])
            manifest.write_bytes(before[Path("generated/manifest.json")])
            artifact = package / "generated/factories/example/factory.yaml"
            artifact.unlink()
            fixtures.lint(root, "catalog-missing-artifact-cached", [("repolint", "missing: generated/factories/example/factory.yaml")])
            artifact.write_bytes(before[Path("generated/factories/example/factory.yaml")])
            notice = package / "generated/README.md"
            notice.write_text("Edited publication prose\n", encoding="utf-8")
            (package / "factories/empty").mkdir()
            fixtures.lint(root, "catalog-prose-empty-directory", [])
            notice.unlink()
            fixtures.lint(root, "catalog-missing-notice", [("repolint", "missing: generated/README.md")])
            notice.write_bytes(before[Path("generated/README.md")])
            write(root, ".gitignore", "packages/packaged-factories/generated/ignored file.json\n")
            extra = package / "generated/ignored file.json"
            extra.write_text("{}\n", encoding="utf-8")
            fixtures.lint(root, "catalog-ignored-output-cached", [("repolint", "unexpected: generated/ignored file.json")])
            extra.unlink()
            source = package / "factories/example/factory.js"
            source.write_text("return {};\n", encoding="utf-8")
            fixtures.lint(root, "catalog-projection-cached", [("repolint", "projection failed:"), ("repolint", "packaged-factory-source:")])
            source.write_bytes(before[Path("factories/example/factory.js")])
            schema = package / "schemas/factory.schema.json"
            schema.unlink()
            fixtures.lint(root, "catalog-missing-schema-cached", [("repolint", "projection failed:")])
            schema.write_bytes(before[Path("schemas/factory.schema.json")])
            write(root, "packages/packaged-factories/factories/empty/note.md", "missing root\n")
            fixtures.lint(root, "catalog-file-backed-missing-root", [("repolint", "no root Factory document"), ("repolint", "packaged-factory-source:")])
            (package / "factories/empty/note.md").unlink()
            fixtures.lint(root, "catalog-restored-cached", [])
            git_directory = root / ".git"
            saved_git = root / ".saved-git"
            git_directory.rename(saved_git)
            try:
                write(root, ".git", "gitdir: missing-catalog-git-directory\n")
                fixtures.lint(root, "catalog-git-input-failure", [
                    ("repolint", "baseline-growth:"),
                    ("repolint", "compiler-ownership:"),
                    ("repolint", "service-cycle-weight: service-cycle metadata git:"),
                    ("repolint", "input failure:")])
            finally:
                git_directory.unlink()
                saved_git.rename(git_directory)
        if args.cohort in ("consumption", "all"):
            consumption(fixtures)
        if args.cohort in ("package-boundary", "all"):
            boundary = BoundaryFixtures(tool, artifacts, (ROOT / ".golangci-repository.yml").read_text(encoding="utf-8"))
            boundary.results = fixtures.results
            boundary.allowed_and_selection()
            boundary.new_violations()
            boundary.counted_debt()
            boundary.stale_debt()
            boundary.missing_inputs()
            boundary.compiler_context()
        if args.cohort in ("owners", "all"):
            fixtures.config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
            root = fixtures.module("compiler-owners")
            write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(root, "pkg/wire/wire.go", "package wire\n")
            write(root, "pkg/transports/fixture/source.go", "package fixture\n")
            write(root, "internal/lint/analyzers/source.go", "package analyzers\n")
            write(root, "internal/fixture/source.go", "package fixture\n")
            key = "test-cross-owner-policy|internal/fixture|internal/fixture/source.go#Load::count=1\n"
            write(root, "internal/lint/analyzers/baseline.txt", key)
            checked(["git", "init", "-q"], root)
            checked(["git", "config", "core.longpaths", "true"], root)
            checked(["git", "add", "internal"], root)
            fixtures.commit(root)
            checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
            fixtures.lint(root, "owners-exact-source", [])
            (root / "internal/fixture/source.go").rename(root / "internal/fixture/other.go")
            fixtures.lint(root, "owners-deleted-source-cached", [("repolint", "vanished compiler owner/source")])
            (root / "internal/fixture/other.go").unlink()
            fixtures.lint(root, "owners-deleted-package", [("repolint", "compiler-ownership:")])
            write(root, "internal/lint/analyzers/baseline.txt", "")
            fixtures.lint(root, "owners-deleted-debt", [])
            write(root, "internal/platform/source_windows_test.go", "package platform_test\n")
            platform = "petri-reference|internal/platform_test|internal/platform/source_windows_test.go#Load::count=1\n"
            write(root, "internal/lint/analyzers/baseline.txt", platform)
            fixtures.lint(root, "owners-inactive-platform-external", [])
            (root / "internal/platform/source_windows_test.go").unlink()
            fixtures.lint(root, "owners-vanished-platform-external", [("repolint", "compiler-ownership:")])
            write(root, "internal/lint/analyzers/baseline.txt", "testsleep-unknown|internal/fixture|source.go::Load::unknown::0\n")
            fixtures.lint(root, "owners-malformed-debt", [("repolint", "malformed timing baseline key")])
            write(root, "internal/lint/analyzers/baseline.txt", "")
            (root / "pkg/wire/wire.go").unlink()
            fixtures.lint(root, "wire-missing-compiler-source-cached", [
                ("repolint", "wire-selection-metadata:"),
                ("repolint", "package-boundary-metadata:"),
            ])
            write(root, "pkg/wire/wire.go", "package wire\n")
            fixtures.lint(root, "wire-restored-compiler-source", [])
        if args.cohort in ("service-cycle", "all"):
            fixtures.config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
            root = fixtures.module("service-cycle")
            write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(root, "pkg/wire/wire.go", "package wire\n")
            write(root, "internal/lint/analyzers/source.go", "package analyzers\n")
            baseline = "internal/lint/analyzers/baseline.txt"
            prefix = "service-cycle-weight|internal/lint/analyzers|"
            write(root, baseline, prefix + "1\n")
            write(root, "pkg/services/a/internal/one/source.go", 'package one\nimport _ "github.com/portpowered/infinite-you/pkg/services/b"\n')
            write(root, "pkg/services/a/source.go", "package two\ntype Service interface {}\n")
            write(root, "pkg/services/b/source.go", "package one\ntype Service interface {}\n")
            back = 'package two\nimport _ "github.com/portpowered/infinite-you/pkg/services/a"\n'
            write(root, "pkg/services/b/internal/two/source.go", back)
            checked(["git", "init", "-q"], root)
            checked(["git", "config", "core.longpaths", "true"], root)
            checked(["git", "add", "."], root)
            fixtures.commit(root)
            checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
            fixtures.lint(root, "cycle-equality", [])
            write(root, "pkg/services/b/internal/two/generated.go", "// Code generated. DO NOT EDIT.\n" + back)
            # Repeated incoming edges alone do not increase the optimal cut.
            write(root, "pkg/services/a/internal/one/repeated.go", 'package one\nimport _ "github.com/portpowered/infinite-you/pkg/services/b"\n')
            fixtures.lint(root, "cycle-regression-cached", [("repolint", "regression: measured 2, ceiling 1, drift +1")])
            (root / "pkg/services/a/internal/one/repeated.go").unlink()
            (root / "pkg/services/b/internal/two/generated.go").unlink()
            (root / "pkg/services/b/internal/two/source.go").unlink()
            fixtures.lint(root, "cycle-source-deletion-cached", [("repolint", "uncaptured improvement; lower the ceiling: measured 0")])
            write(root, "pkg/services/b/internal/two/inactive.go", "//go:build custom_inactive\n\n" + back)
            fixtures.lint(root, "cycle-wholly-inactive-package", [])
            write(root, "pkg/services/a/internal/one/repeated.go", 'package one\nimport _ "github.com/portpowered/infinite-you/pkg/services/b"\n')
            write(root, "pkg/services/b/internal/two/inactive_windows.go", "//go:build windows\n\n" + back)
            fixtures.lint(root, "cycle-inactive-goos-cached", [("repolint", "regression: measured 2")])
            (root / "pkg/services/a/internal/one/repeated.go").unlink()
            (root / "pkg/services/b/internal/two/inactive_windows.go").unlink()
            write(root, baseline, prefix + "0\n")
            fixtures.lint(root, "cycle-ceiling-edit-cached", [("repolint", "regression: measured 1, ceiling 0")])
            (root / "pkg/services/b/internal/two/inactive.go").unlink()
            fixtures.lint(root, "cycle-captured-improvement", [])
            write(root, baseline, prefix + "2\n")
            fixtures.lint(root, "cycle-increased-history", [("repolint", "ceiling may only decrease"), ("repolint", "uncaptured improvement")])
            write(root, baseline, "")
            fixtures.lint(root, "cycle-missing-ceiling", [("repolint", "ceiling may only decrease"), ("repolint", "missing service-cycle-weight")])
            write(root, baseline, prefix + "bad\n")
            fixtures.lint(root, "cycle-corrupt-ceiling", [("repolint", "invalid service-cycle-weight ceiling")] * 3)
            write(root, baseline, prefix + "0\n")
            checked(["git", "update-ref", "-d", "refs/remotes/origin/main"], root)
            fixtures.lint(root, "cycle-missing-history", [("repolint", "origin/main")])
        if args.cohort in ("baseline", "all"):
            fixtures.config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
            root = fixtures.module("baseline-growth")
            write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(root, "pkg/wire/wire.go", "package wire\n")
            write(root, "internal/lint/analyzers/source.go", "package analyzers\n")
            write(root, "internal/lint/analyzers/baseline.txt", "old|a|b\n")
            write(root, "pkg/transports/fixture/source.go", "package fixture\n")
            checked(["git", "init", "-q"], root)
            checked(["git", "config", "core.longpaths", "true"], root)
            checked(["git", "add", "internal"], root)
            fixtures.commit(root)
            checked(["git", "update-ref", "refs/remotes/origin/main", "HEAD"], root)
            fixtures.lint(root, "baseline-unchanged", [])
            write(root, "internal/lint/analyzers/baseline.txt", "old|a|b\nnew|a|b\n")
            fixtures.lint(root, "baseline-new-rule", [])
            write(root, "internal/lint/analyzers/baseline.txt", "old|a|c\n")
            fixtures.lint(root, "baseline-count-neutral-replacement", [])
            write(root, "internal/lint/analyzers/baseline.txt", "old|a|c\nold|a|d\n")
            fixtures.lint(root, "baseline-established-growth", [("repolint", "established baseline rules gained keys")])
            write(root, "internal/lint/analyzers/baseline.txt", "")
            fixtures.lint(root, "baseline-deletion", [])
            checked(["git", "update-ref", "-d", "refs/remotes/origin/main"], root)
            fixtures.lint(root, "baseline-missing-origin", [("repolint", "origin/main")])
        if args.cohort in ("manifest", "all"):
            fixtures.config = (ROOT / ".golangci-repository-default.yml").read_text(encoding="utf-8")
            root = fixtures.module("manifest-authority")
            write(root, "go.mod", "module github.com/portpowered/infinite-you\n\ngo 1.25.0\n")
            write(root, "pkg/wire/wire.go", "package wire\n")
            write(root, "pkg/transports/cli/root_work.go", "package cli\nfunc executeWork(inputID string) string { return inputID }\n")
            fixtures.lint(root, "manifest-permitted", [])
            write(root, "pkg/transports/cli/root_work.go", "package cli\nfunc newRunCommand() {}\n")
            fixtures.lint(root, "manifest-handwritten-command", [("repolint", "cli-manifest-authority:")])
            write(root, "pkg/transports/cli/root_work.go", "package cli\nfunc execute(key string) { switch key { case \"with-server\": } }\n")
            fixtures.lint(root, "manifest-binding-switch", [("repolint", "binding with-server")])
            write(root, "contracts/fixture/shape.go", """package shape
type Command struct{}
type flagSet struct{}
func (*Command) Flags() *flagSet { return &flagSet{} }
func (*flagSet) String(name, value, usage string) {}
""")
            write(root, "pkg/transports/cli/root_work.go", """package cli
import "github.com/portpowered/infinite-you/contracts/fixture"
var command = &shape.Command{}
""")
            fixtures.lint(root, "manifest-command-metadata", [("repolint", "cobra.Command metadata")])
            write(root, "pkg/transports/cli/root_work.go", """package cli
import "github.com/portpowered/infinite-you/contracts/fixture"
func execute(command *shape.Command) { command.Flags().String("name", "", "help") }
""")
            fixtures.lint(root, "manifest-direct-flags", [("repolint", "public input registration")])
            write(root, "pkg/transports/cli/root_work.go", "package cli\ntype SessionFamilyBindings struct{}\n")
            fixtures.lint(root, "manifest-mirror-storage", [("repolint", "CLI-shape mirror")])
            write(root, "pkg/transports/cli/root_work.go", "package cli\n")
            write(root, ".golangci.yml", re.sub(
                r"defer-stale: \[[^\]]*\]", "defer-stale: [missing]", fixtures.config))
            fixtures.lint(root, "manifest-invalid-settings", [], error="unknown deferred analyzer")
            write(root, ".golangci.yml", fixtures.config.replace("repolint", "missing"))
            fixtures.lint(root, "manifest-unregistered-plugin", [], error="not found")
        fixtures.config = (ROOT / ".golangci.yml").read_text(encoding="utf-8")
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
