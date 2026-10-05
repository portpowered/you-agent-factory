"""Component tests of compiler-report adaptation; no source inspection."""

import importlib.util
import io
import json
from pathlib import Path
import subprocess
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
                self.assertEqual(report.normalize(f"pkg/file_{suffix}.go: Dead"), "")
        self.assertEqual(report.normalize("third_party/acp-go-sdk/file.go: Dead"), "")
        for source in ["third_party/acp-go-sdk-helper/f.go", "pkg/third_party/acp-go-sdk/f.go",
                       "third_party/other/f.go", "cmd/tool/f.go", "internal/lint/analyzers/f.go",
                       "tools/golangcilintplugin/f.go"]:
            with self.subTest(source=source):
                self.assertEqual(report.normalize(source + ": Dead"), source + ": Dead\n")

    def test_reconcile_function_identities(self):
        repository = "tools/plugin/f.go: Live\ntools/plugin/f.go: Dead\npkg/f.go: Product\n"
        self.assertEqual(report.reconcile(repository, "tools/plugin/f.go: Dead\n", {"tools/plugin"}),
                         "pkg/f.go: Product\ntools/plugin/f.go: Dead\n")

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
        self.baseline.write_text("pkg/f.go: Dead\n")
        self.metadata = {"Module": {"Path": "github.com/golangci/golangci-lint/v2"},
                         "Replace": [{"Old": {"Path": report.MODULE}, "New": {"Path": str(self.root)}}]}
        self.directories = "\n".join(str(self.root / path) for path in
                                     ["tools/golangcilintplugin", "internal/lint/analyzers"])
        self.calls = []
        self.out, self.err = io.StringIO(), io.StringIO()

    def executor(self, arguments, **kwargs):
        self.calls.append((arguments, kwargs))
        if arguments[1:3] == ["env", "GOVERSION"]:
            value = "go1.25.0\n"
        elif arguments[1:3] == ["mod", "edit"]:
            value = json.dumps(self.metadata)
        elif arguments[1] == "list":
            value = self.directories
        elif arguments[-1] == "./...":
            value = "pkg/f.go:12:3: Dead\n"
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
        self.assertEqual((self.root / report.CURRENT).read_bytes(), b"pkg/f.go: Dead\n")
        self.assertEqual(len(self.calls), 5)
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
        for baseline, actual in [("a: Dead\n", "a: Dead\nb: Dead\n"),
                                 ("a: Dead\nb: Dead\n", "a: Dead\n"),
                                 ("a: Dead\n", "b: Dead\n"), ("a: Dead\n", "")]:
            with self.subTest(baseline=baseline, actual=actual):
                self.baseline.write_text(baseline)
                out, err = io.StringIO(), io.StringIO()
                self.assertEqual(report.persist(self.root, actual, out, err), 1)
                self.assertEqual(out.getvalue(), "")
                for text in [report.CURRENT, report.BASELINE, f"LINT_VIOLATION_COUNT: {report.count(actual)}",
                             f"baseline findings: {report.count(baseline)}, current findings: {report.count(actual)}"]:
                    self.assertIn(text, err.getvalue())
                self.assertEqual(self.baseline.read_text(), baseline)
        self.baseline.write_bytes(b"pkg\\f.go:1:2: Dead\r\n")
        self.assertEqual(report.persist(self.root, "pkg/f.go: Dead\n", self.out, self.err), 0)
        self.assertEqual(self.baseline.read_bytes(), b"pkg\\f.go:1:2: Dead\r\n")

    def test_host_positions_relative_absolute_and_invalid(self):
        source = self.root / "pkg/f.go"
        for location in [str(source), "../repository/pkg/f.go"]:
            self.assertEqual(report.repository_positions(location + ":12:3: Dead", self.host, self.root),
                             "pkg/f.go:12:3: Dead")
        for line in ["invalid", str(self.host / "f.go") + ":1:1: Dead"]:
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


if __name__ == "__main__":
    unittest.main()
