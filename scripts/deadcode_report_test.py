"""Component tests of compiler-report adaptation; no source inspection."""

import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("deadcode_report", Path(__file__).with_name("deadcode-report.py"))
report = importlib.util.module_from_spec(spec)
spec.loader.exec_module(report)
UPSTREAM = ["go", "run", "golang.org/x/tools/cmd/deadcode@v0.25.1"]


class Reports(unittest.TestCase):
    def test_normalization_preserves_multiplicity_and_empty_lines(self):
        self.assertEqual(report.normalize(" b\\b.go:12:3: B\r\n a.go:1:2: A\r a.go:1:2: A "),
                         "a.go: A\na.go: A\nb/b.go: B\n")
        self.assertEqual(report.normalize("\r\n \r"), "")
        self.assertEqual(report.normalize("a.go: A\n\nb.go: B"), "\na.go: A\nb.go: B\n")
        self.assertEqual(report.count(""), 0)
        self.assertEqual(report.count("a\na\n"), 2)

    def test_exact_exclusions_keep_authored_sources(self):
        for suffix in report.SUFFIXES:
            with self.subTest(suffix=suffix):
                self.assertEqual(report.normalize(f"pkg/file_{suffix}.go: unreachable func: Dead"), "")
        self.assertEqual(report.normalize("third_party/acp-go-sdk/file.go: unreachable func: Dead"), "")
        for source in ["third_party/acp-go-sdk-helper/f.go", "pkg/third_party/acp-go-sdk/f.go",
                       "third_party/other/f.go", "cmd/tool/f.go", "internal/lint/analyzers/f.go",
                       "tools/golangcilintplugin/f.go"]:
            with self.subTest(source=source):
                self.assertEqual(report.normalize(source + ": unreachable func: Dead"), source + ": unreachable func: Dead\n")

    def test_reconcile_function_identities(self):
        repository = "tools/plugin/f.go: Live\ntools/plugin/f.go: unreachable func: Dead\npkg/f.go: Product\n"
        self.assertEqual(report.reconcile(repository, "tools/plugin/f.go: unreachable func: Dead\n", {"tools/plugin"}),
                         "pkg/f.go: Product\ntools/plugin/f.go: unreachable func: Dead\n")

    def test_environment_preserves_other_flags(self):
        original = {"GODEBUG": "gocachehash=1,gotypesalias=0,,gotypesalias=0", "OTHER": "yes"}
        env = report.production_environment(original, "go1.25.0")
        self.assertEqual(env, {"GODEBUG": "gocachehash=1,gotypesalias=1,gotypesalias=1",
                              "OTHER": "yes", "GOWORK": "off", "GOTOOLCHAIN": "go1.25.0+auto"})
        self.assertIn("gotypesalias=0", original["GODEBUG"])
        self.assertEqual(report.production_environment({}, "go1.25.0")["GODEBUG"], "gotypesalias=1")


class Reporter(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve() / "repository"
        self.host = Path(self.temp.name).resolve() / "host"
        self.root.mkdir()
        self.host.mkdir()
        self.pointer = self.root / report.HOST_POINTER
        self.pointer.parent.mkdir(parents=True)
        self.pointer.write_text(str(self.host))
        self.baseline = self.root / report.BASELINE
        self.baseline.parent.mkdir(parents=True)
        self.baseline.write_text("pkg/f.go: unreachable func: Dead\n")
        self.metadata = {"Module": {"Path": "github.com/golangci/golangci-lint/v2"},
                         "Replace": [{"Old": {"Path": report.MODULE}, "New": {"Path": str(self.root)}}]}
        self.directories = "\n".join(str(self.root / path) for path in
                                     ["tools/golangcilintplugin", "internal/lint/analyzers"])
        self.calls = []
        self.out, self.err = io.StringIO(), io.StringIO()

    def executor(self, arguments, **kwargs):
        self.calls.append((arguments, kwargs))
        if arguments[0] == "git":
            if arguments[1] == "show":
                value = self.baseline.read_text()
            elif arguments[1] in ("rev-parse", "merge-base"):
                value = "a" * 40
            else:
                value = ""
        elif arguments[1:3] == ["env", "GOVERSION"]:
            value = "go1.25.0\n"
        elif arguments[1:3] == ["mod", "edit"]:
            value = json.dumps(self.metadata)
        elif arguments[1] == "list":
            value = self.directories
        elif arguments[-1] == "./...":
            value = "pkg/f.go:12:3: unreachable func: Dead\n"
        else:
            value = ""
        return subprocess.CompletedProcess(arguments, 0, value, "")

    def run_report(self, executor=None, argv=None):
        return report.main(argv or ["--"] + UPSTREAM, root=self.root,
                           environment={"GODEBUG": "other=1,gotypesalias=0"},
                           stdout=self.out, stderr=self.err, executor=executor or self.executor)

    def test_cli_success_and_compiler_calls(self):
        self.assertEqual(self.run_report(), 0)
        self.assertEqual(self.out.getvalue(), "[agent-factory:deadcode] baseline matches\n")
        self.assertEqual((self.root / report.CURRENT).read_bytes(), b"pkg/f.go: unreachable func: Dead\n")
        self.assertEqual(len([call for call in self.calls if call[0][0] == "go"]), 5)
        for arguments, kwargs in self.calls[1:]:
            self.assertNotIn("-test", arguments)
            self.assertEqual(kwargs["env"]["GOWORK"], "off")
            self.assertEqual(kwargs["env"]["GOTOOLCHAIN"], "go1.25.0+auto")
            self.assertEqual(kwargs["env"]["GODEBUG"], "other=1,gotypesalias=1")
        for index, directory, pattern in [(2, self.root, "./..."), (3, self.host, "./cmd/golangci-lint")]:
            arguments, kwargs = self.calls[index]
            self.assertEqual(arguments, UPSTREAM + ["-filter=^github\\.com/portpowered/infinite-you(/|$)", pattern])
            self.assertEqual(kwargs["cwd"], directory)

    def test_exact_drift_additions_removals_and_equal_counts(self):
        for baseline, actual in [("a.go: unreachable func: Dead\n", "a.go: unreachable func: Dead\nb.go: unreachable func: Dead\n"),
                                 ("a.go: unreachable func: Dead\nb.go: unreachable func: Dead\n", "a.go: unreachable func: Dead\n"),
                                 ("a.go: unreachable func: Dead\n", "b.go: unreachable func: Dead\n"), ("a.go: unreachable func: Dead\n", "")]:
            with self.subTest(baseline=baseline, actual=actual):
                self.baseline.write_text(baseline)
                out, err = io.StringIO(), io.StringIO()
                self.assertEqual(report.persist(self.root, actual, out, err, executor=self.executor), 1)
                self.assertEqual(out.getvalue(), "")
                for text in [report.CURRENT, report.BASELINE, f"LINT_VIOLATION_COUNT: {report.count(actual)}",
                             f"baseline findings: {report.count(baseline)}, current findings: {report.count(actual)}"]:
                    self.assertIn(text, err.getvalue())
                self.assertEqual(self.baseline.read_text(), baseline)
        self.baseline.write_bytes(b"pkg\\f.go:1:2: unreachable func: Dead\r\n")
        self.assertEqual(report.persist(self.root, "pkg/f.go: unreachable func: Dead\n", self.out, self.err, executor=self.executor), 0)
        self.assertEqual(self.baseline.read_bytes(), b"pkg\\f.go:1:2: unreachable func: Dead\r\n")

    def test_host_positions_relative_absolute_and_invalid(self):
        source = self.root / "pkg/f.go"
        for location in [str(source), "../repository/pkg/f.go"]:
            self.assertEqual(report.repository_positions(location + ":12:3: unreachable func: Dead", self.host, self.root),
                             "pkg/f.go:12:3: unreachable func: Dead")
        for line in ["invalid", str(self.host / "f.go") + ":1:1: unreachable func: Dead"]:
            with self.assertRaises(ValueError):
                report.repository_positions(line, self.host, self.root)
        self.directories += "\n" + str(self.host)
        self.assertEqual(self.run_report(), 1)
        self.assertIn("outside repository", self.err.getvalue())

    def test_invalid_pointers(self):
        for value in ["", "relative/path", str(self.root / "missing")]:
            with self.subTest(value=value):
                self.pointer.write_text(value)
                def executor(args, **kwargs):
                    if kwargs["cwd"] != self.root:
                        raise FileNotFoundError("missing module")
                    return self.executor(args, **kwargs)
                self.assertEqual(self.run_report(executor), 1)
        self.pointer.unlink()
        self.assertEqual(self.run_report(), 1)
        self.assertIn("read generated golangci host", self.err.getvalue())

    def test_host_metadata_fails_closed(self):
        candidates = [{}, {"Module": {"Path": "other"}},
                      {"Module": self.metadata["Module"]},
                      {**self.metadata, "Replace": [{"Old": {"Path": report.MODULE},
                                                       "New": {"Path": str(self.host)}}]},
                      {**self.metadata, "Replace": [{"Old": {"Path": report.MODULE},
                                                       "New": {"Path": str(self.root), "Version": "v1"}}]},
                      {**self.metadata, "Replace": [{"Old": {"Path": report.MODULE, "Version": "v1"},
                                                       "New": {"Path": str(self.root)}}]}]
        for metadata in candidates:
            with self.subTest(metadata=metadata), self.assertRaises(ValueError):
                report.validate_host(metadata, self.host, self.root)
        relative = {**self.metadata, "Replace": [{"Old": {"Path": report.MODULE},
                                                  "New": {"Path": "../repository"}}]}
        report.validate_host(relative, self.host, self.root)
        self.directories = str(self.root / "tools/golangcilintplugin")
        self.assertEqual(self.run_report(), 1)
        self.assertIn("does not compile", self.err.getvalue())

    def test_command_failures_and_success_stderr(self):
        for stage in ["env", "mod", "run", "list"]:
            with self.subTest(stage=stage):
                def executor(args, **kwargs):
                    if args[1] == stage:
                        return subprocess.CompletedProcess(args, 7, "", "compiler diagnostic")
                    return self.executor(args, **kwargs)
                self.assertEqual(self.run_report(executor), 1)
                self.assertIn("compiler diagnostic", self.err.getvalue())
                self.assertEqual(self.out.getvalue(), "")
        def executor(args, **kwargs):
            result = self.executor(args, **kwargs)
            result.stderr = "upstream warning\n" if args[1] == "run" else ""
            return result
        self.assertEqual(self.run_report(executor), 0)
        self.assertIn("upstream warning", self.err.getvalue())

    def test_malformed_module_json(self):
        def executor(args, **kwargs):
            if args[1] == "mod":
                return subprocess.CompletedProcess(args, 0, "broken JSON", "")
            return self.executor(args, **kwargs)
        self.assertEqual(self.run_report(executor), 1)
        self.assertEqual(self.out.getvalue(), "")

    def test_report_io_errors_and_retained_artifact(self):
        self.baseline.unlink()
        self.assertEqual(self.run_report(), 1)
        self.assertIn("read deadcode baseline", self.err.getvalue())
        self.assertTrue((self.root / report.CURRENT).is_file())
        with patch.object(Path, "mkdir", side_effect=PermissionError("denied")):
            self.assertEqual(self.run_report(), 1)
        self.assertIn("create deadcode output directory", self.err.getvalue())
        with patch.object(Path, "write_text", side_effect=PermissionError("denied")):
            self.assertEqual(self.run_report(), 1)
        self.assertIn("write current deadcode report", self.err.getvalue())
        self.assertEqual(self.out.getvalue(), "")

    def test_cli_flags_help_and_status(self):
        self.assertEqual(self.run_report(argv=["--golangci-host-file", str(self.pointer), "--"] + UPSTREAM), 0)
        for argv, code in [(["--help"], 0), (["--unknown"], 2), (["--", "go", "run", "wrong"], 2)]:
            with self.subTest(argv=argv), patch("sys.stdout", io.StringIO()), patch("sys.stderr", io.StringIO()):
                with self.assertRaises(SystemExit) as context:
                    report.main(argv)
                self.assertEqual(context.exception.code, code)


class HistoricalReports(unittest.TestCase):
    """Lint fixtures own real Git history; only compiler output is controlled."""

    def setUp(self):
        Reporter.setUp(self)
        self.environment = {key: value for key, value in os.environ.items()
                            if not key.startswith("GIT_")}
        self.environment.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
        self.source = self.root / "pkg/f.go"
        self.source.parent.mkdir()
        self.source.write_text("package fixture\nfunc Dead() {}\n")
        self.git("init", "--initial-branch=main")
        for key, value in [("user.name", "Deadcode Fixture"), ("user.email", "fixture@example.invalid"),
                           ("commit.gpgsign", "false"), ("core.hooksPath", str(self.root / "no-hooks"))]:
            self.git("config", key, value)
        self.commit()
        self.anchor = self.git("rev-parse", "HEAD").strip()
        self.git("update-ref", "refs/remotes/origin/main", self.anchor)
        self.actual = self.baseline.read_text()
        self.selected = {"pkg/f.go"}

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, env=self.environment,
                              text=True, capture_output=True, check=True).stdout

    def commit(self):
        self.git("add", ".")
        self.git("commit", "-m", "fixture")

    def executor(self, args, **kwargs):
        if args[0] == "git":
            return subprocess.run(args, cwd=self.root, env=self.environment,
                                  text=True, capture_output=True)
        if "-compiled" in args:
            return subprocess.CompletedProcess(args, 0, json.dumps({
                "Dir": str(self.root), "GoFiles": sorted(self.selected),
                "CompiledGoFiles": sorted(self.selected)}), "")
        if args[-1] == "./...":
            return subprocess.CompletedProcess(args, 0, self.actual, "")
        return Reporter.executor(self, args, **kwargs)

    def invoke(self, expected, diagnostic="", anchor=None):
        self.out, self.err = io.StringIO(), io.StringIO()
        before = self.baseline.read_bytes()
        result = report.main(["--historical-base", anchor or self.anchor, "--"] + UPSTREAM,
                             root=self.root, environment=self.environment,
                             stdout=self.out, stderr=self.err, executor=self.executor)
        self.assertEqual(result, expected, self.err.getvalue())
        self.assertIn(diagnostic, self.out.getvalue() + self.err.getvalue())
        self.assertEqual(self.baseline.read_bytes(), before)
        if expected:
            self.assertNotIn("baseline matches", self.out.getvalue())

    def matching(self, findings):
        self.actual = findings
        self.baseline.write_text(findings)

    def test_S01_S03_S11_unchanged_deletion_empty(self):
        self.invoke(0, "baseline matches")
        self.matching("")
        self.invoke(0, "baseline matches")
        self.git("update-ref", "-d", "refs/remotes/origin/main")
        self.invoke(1, "fetch origin/main and full history")

    def test_S02_growth_and_S10_enlarged_main(self):
        additions = "".join(f"pkg/new.go: unreachable func: New{i}\n" for i in range(3))
        self.matching(self.actual + additions)
        self.invoke(1, "net delta: +3")
        for diagnostic in ["historical findings: 1, baseline findings: 4, current findings: 4",
                           "added (1): pkg/new.go: unreachable func: New0", self.anchor]:
            self.assertIn(diagnostic, self.err.getvalue())
        self.commit()
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        self.invoke(1, "historical anchor")

    def test_S02_canonical_command_exit_with_real_git(self):
        self.matching(self.actual + "pkg/new.go: unreachable func: New\n")
        compiler = f'''import sys
from pathlib import Path
command = Path(sys.argv[0]).name
if command == "env":
    print("go1.25.0")
elif command == "mod":
    print({json.dumps(self.metadata)!r})
elif command == "list":
    print({self.directories!r})
elif sys.argv[-1] == "./...":
    print({self.actual!r}, end="")
'''
        # Python accepts each command name as a script, avoiding shell/batch
        # interpretation of the upstream filter argument on every platform.
        for directory in [self.root, self.host]:
            for name in ["env", "mod", "list", "run"]:
                (directory / name).write_text(compiler, encoding="utf-8")
        result = subprocess.run([sys.executable, str(Path(report.__file__).resolve()),
                                 "--historical-base", self.anchor, "--", sys.executable,
                                 "run", UPSTREAM[2]], cwd=self.root, env=self.environment,
                                text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("net delta: +1", result.stderr)
        self.assertIn("added (1): pkg/new.go: unreachable func: New", result.stderr)
        self.assertEqual((self.root / report.CURRENT).read_text(), report.normalize(self.actual))

    def test_S05_unbaselined_production_debt(self):
        self.actual += "pkg/f.go: unreachable func: New\n"
        self.invoke(1, "baseline drift")
        self.assertIn("New", (self.root / report.CURRENT).read_text())

    def test_S06_substitution_does_not_spend_deletions(self):
        self.matching("pkg/f.go: unreachable func: Other\n")
        self.invoke(1, "net delta: +0")
        self.assertIn("added (1): pkg/f.go: unreachable func: Other", self.err.getvalue())
        self.assertIn("removed (1): pkg/f.go: unreachable func: Dead", self.err.getvalue())

    def relocate(self):
        target = self.root / "moved/f.go"
        target.parent.mkdir()
        self.git("mv", "pkg/f.go", "moved/f.go")
        self.matching(self.actual.replace("pkg/f.go", "moved/f.go"))
        self.selected = {"moved/f.go"}

    def test_S04_owned_relocation_and_S09_multiplicity(self):
        self.relocate()
        self.invoke(0, "baseline matches")
        self.matching(self.actual * 2)
        self.invoke(1, "added (1): moved/f.go: unreachable func: Dead")
        self.assertIn("relocated (1):", self.err.getvalue())

    def test_S09_unselected_relocation(self):
        self.relocate()
        self.selected = set()
        self.invoke(1, "added (1): moved/f.go")

    def test_S09_copy_and_changed_identity(self):
        target = self.root / "copy.go"
        target.write_text(self.source.read_text())
        self.selected.add("copy.go")
        self.matching("copy.go: unreachable func: Dead\n")
        self.invoke(1, "added (1): copy.go")
        target.unlink()
        self.actual = "pkg/f.go: unreachable func: Dead\n"
        self.relocate()
        self.matching("moved/f.go: unreachable func: Other\n")
        self.invoke(1, "added (1): moved/f.go: unreachable func: Other")

    def test_S09_ambiguous_identical_blob_renames(self):
        second = self.root / "pkg/second.go"
        second.write_text(self.source.read_text())
        self.matching(self.actual + "pkg/second.go: unreachable func: Dead\n")
        self.commit()
        self.anchor = self.git("rev-parse", "HEAD").strip()
        self.git("update-ref", "refs/remotes/origin/main", self.anchor)
        self.relocate()
        self.git("mv", "pkg/second.go", "moved/second.go")
        self.matching(self.actual.replace("pkg/second.go", "moved/second.go"))
        self.selected.add("moved/second.go")
        self.invoke(1, "historical allowance growth")

    def test_S07_missing_refs_and_unrelated_ancestry(self):
        self.invoke(1, "fetch origin/main and full history", anchor="missing-anchor")
        self.git("checkout", "--orphan", "unrelated")
        self.git("rm", "-rf", "--cached", ".")
        self.commit()
        self.invoke(1, "merge-base")
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        self.invoke(1, "--is-ancestor")

    def test_S08_invalid_working_and_historical_allowances(self):
        for finding in ["invalid", "../escape.go: unreachable func: Dead",
                        "/absolute.go: unreachable func: Dead", "C:/absolute.go: unreachable func: Dead",
                        "pkg//f.go: unreachable func: Dead", "pkg/f.go: unreachable func: "]:
            with self.subTest(finding=finding):
                self.matching(finding + "\n")
                self.invoke(1, "finding" if finding in ["invalid", "pkg/f.go: unreachable func: "] else "source path")
        self.matching("malformed historical\n")
        self.commit()
        self.anchor = self.git("rev-parse", "HEAD").strip()
        self.git("update-ref", "refs/remotes/origin/main", self.anchor)
        self.matching("")
        self.invoke(1, "malformed historical anchor")

    def test_S07_shallow_history_missing_anchor(self):
        self.matching("")
        self.commit()
        clone = self.root.parent / "shallow"
        subprocess.run(["git", "clone", "--depth=1", self.root.as_uri(), str(clone)],
                       env=self.environment, capture_output=True, check=True)
        self.root = clone
        self.host = self.root.parent / "host"
        self.baseline = self.root / report.BASELINE
        self.metadata["Replace"][0]["New"]["Path"] = str(self.root)
        self.directories = "\n".join(str(self.root / path) for path in
                                     ["tools/golangcilintplugin", "internal/lint/analyzers"])
        self.invoke(1, "fetch origin/main and full history")


if __name__ == "__main__":
    unittest.main()
