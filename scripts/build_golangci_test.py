"""Component tests for compiler input identity and fail-closed artifact restore."""

import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("builder", Path(__file__).with_name("build-golangci.py"))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class CacheTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        for name in ("go.mod", "go.sum", ".custom-gcl.yml", "scripts/build-golangci.py", "source.go", "asset.txt"):
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("fixture", encoding="utf-8")
        self.environment = {"GOVERSION": "go1.25.0", "GOOS": "linux", "GOARCH": "amd64"}
        self.packages = [{"ImportPath": builder.MODULE + "/tools/golangcilintplugin",
                          "Dir": str(self.root), "GoFiles": ["source.go"], "EmbedFiles": ["asset.txt"]}]
        self.calls = []

    def runner(self, command, cwd):
        self.calls.append(command)
        if command[:2] == ["go", "env"]:
            return json.dumps(self.environment)
        if command[:2] == ["go", "list"]:
            return "\n".join(json.dumps(package) for package in self.packages)
        if command[-1] == "version":
            return "golangci-lint has version v2.11.4-custom-gcl-fixture"
        if command == ["go", "mod", "edit", "-json"]:
            return json.dumps({"Module": {"Path": "github.com/golangci/golangci-lint/v2"},
                               "Replace": [{"Old": {"Path": builder.MODULE},
                                            "New": {"Path": str(self.root)}}]})
        if command[:4] == ["go", "mod", "edit", "-replace"]:
            return ""
        raise AssertionError(command)

    def key(self):
        return builder.cache_key(self.root, self.runner)

    def artifact(self):
        destination = self.root / "artifact"
        host = destination / "host"
        host.mkdir(parents=True)
        (host / "go.mod").write_text("host")
        (host / "go.sum").write_text("host dependencies")
        name = "golangci-repository.exe" if os.name == "nt" else "golangci-repository"
        binary = destination / name
        binary.write_bytes(b"binary")
        metadata = {"key": self.key(), "binary": builder.file_digest(binary), "host": builder.host_digest(host)}
        (destination / "cache.json").write_text(json.dumps(metadata))
        return destination, binary

    def test_exact_inputs_are_stable(self):
        self.assertEqual(self.key(), self.key())

    def test_source_embed_and_dependency_inputs_invalidate(self):
        for name in ("source.go", "asset.txt", "go.mod", "go.sum", ".custom-gcl.yml", "scripts/build-golangci.py"):
            with self.subTest(name=name):
                before = self.key()
                path = self.root / name
                path.write_text(path.read_text() + " changed")
                self.assertNotEqual(before, self.key())

    def test_transitive_source_invalidates(self):
        dependency = self.root / "dependency"
        dependency.mkdir()
        (dependency / "dep.go").write_text("package dependency")
        self.packages.append({"ImportPath": "dependency", "Dir": str(dependency), "GoFiles": ["dep.go"]})
        before = self.key()
        (dependency / "dep.go").write_text("package dependency\nvar Changed = 1")
        self.assertNotEqual(before, self.key())

    def test_toolchain_and_build_environment_invalidate(self):
        for field in ("GOVERSION", "GOOS", "GOARCH", "GOFLAGS", "CGO_ENABLED"):
            before = self.key()
            self.environment[field] = "changed"
            self.assertNotEqual(before, self.key())

    def test_incomplete_or_missing_metadata_fails(self):
        for packages in ([], [dict(self.packages[0], Incomplete=True)]):
            self.packages = packages
            with self.assertRaises(ValueError):
                self.key()

    def test_valid_restore_skips_build_and_rebinds_repeatably(self):
        destination, _ = self.artifact()
        self.calls.clear()
        builder.restore(destination, self.root, self.runner)
        builder.restore(destination, self.root, self.runner)
        pointer = Path((destination / "host-path.txt").read_text().strip())
        self.assertEqual(pointer, destination / "current-host")
        self.assertTrue((pointer / "go.mod").is_file())
        self.assertFalse(any("custom" in command for command in self.calls))
        self.assertIn(["go", "mod", "edit", "-replace", builder.MODULE + "=" + str(self.root)], self.calls)

    def test_corrupt_missing_and_stale_artifacts_fail_closed(self):
        destination, binary = self.artifact()
        original = binary.read_bytes()
        mutations = [lambda: binary.write_bytes(b"corrupt"),
                     lambda: binary.unlink(),
                     lambda: (destination / "host/go.mod").unlink(),
                     lambda: (destination / "cache.json").write_text("invalid"),
                     lambda: (self.root / "source.go").write_text("changed")]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                binary.write_bytes(original)
                (destination / "host/go.mod").write_text("host")
                (destination / "cache.json").write_text(json.dumps({
                    "key": self.key(), "binary": builder.file_digest(binary),
                    "host": builder.host_digest(destination / "host")}))
                (destination / "host-path.txt").write_text("stale pointer")
                mutation()
                with self.assertRaises((ValueError, OSError)):
                    builder.restore(destination, self.root, self.runner)
                self.assertFalse((destination / "host-path.txt").exists())

    def test_failed_build_propagates_exit_and_removes_pointer(self):
        destination = self.root / "artifact"
        destination.mkdir()
        (destination / "host-path.txt").write_text("stale")
        process = unittest.mock.MagicMock()
        process.__enter__.return_value = process
        process.stdout = []
        process.wait.return_value = 7
        with patch("sys.argv", ["builder", "--destination", str(destination), "--", "fake-build"]), \
             patch.object(builder, "cache_key", return_value="key"), \
             patch.object(builder.subprocess, "Popen", return_value=process):
            self.assertEqual(builder.main(), 7)
        self.assertFalse((destination / "host-path.txt").exists())

    def test_wrong_binary_or_host_identity_fails_closed(self):
        destination, _ = self.artifact()
        for failure in ("version", "host"):
            with self.subTest(failure=failure):
                def runner(command, cwd):
                    if failure == "version" and command[-1] == "version":
                        return "golangci-lint has version 2.11.4"
                    if failure == "host" and command == ["go", "mod", "edit", "-json"]:
                        return json.dumps({"Module": {"Path": "wrong"}})
                    return self.runner(command, cwd)
                with self.assertRaises(ValueError):
                    builder.restore(destination, self.root, runner)
                self.assertFalse((destination / "host-path.txt").exists())


if __name__ == "__main__":
    unittest.main()
