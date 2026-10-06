import importlib.util
from pathlib import Path
import tempfile
import unittest
import os
import re
import shutil
import subprocess

spec = importlib.util.spec_from_file_location("compiler_cache", Path(__file__).with_name("functional-compile-cache.py"))
cache = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cache)


class CompilerCacheTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which("go"), "requires the Go compiler")
    def test_real_go_reuse_and_source_invalidation(self):
        with tempfile.TemporaryDirectory(prefix="compiler-cache-proof-") as directory:
            root = Path(directory)
            project = root / "project"
            (project / "witness").mkdir(parents=True)
            (project / "go.mod").write_text("module example.com/cacheproof\n\ngo 1.25.0\n")
            (project / "main.go").write_text('package main\nimport "example.com/cacheproof/witness"\nfunc main(){println(witness.Value())}\n')
            value = project / "witness/value.go"
            value.write_text('package witness\nfunc Value() string{return "one"}\n')

            (root / "coverage").mkdir()

            def build(name, go_cache):
                environment = dict(os.environ, GOCACHE=str(go_cache), GOMAXPROCS="4", GOFLAGS="", GOCOVERDIR=str(root / "coverage"))
                binary = root / (name + (".exe" if os.name == "nt" else ""))
                result = subprocess.run(["go", "build", "-p=4", "-cover", "-coverpkg=./...", "-covermode=count", "-x", "-o", str(binary), "."], cwd=project, env=environment, capture_output=True, text=True, check=True)
                output = subprocess.check_output([str(binary)], env=environment, stderr=subprocess.STDOUT, text=True).strip()
                compiles = len(re.findall(r'[/\\]compile(?:\.exe)?(?=["\s]|$)', result.stderr))
                return compiles, output

            cold, _ = build("cold", root / "cache-a")
            cache.transfer(root / "cache-a", root / "snapshot", limit=1024**3)
            cache.transfer(root / "snapshot", root / "cache-b", limit=1024**3)
            reused, original = build("restored", root / "cache-b")
            self.assertGreater(cold, 0)
            self.assertEqual(reused, 0)
            self.assertEqual(original, "one")
            value.write_text('package witness\nfunc Value() string{return "two"}\n')
            changed, output = build("changed", root / "cache-b")
            self.assertGreaterEqual(changed, 2)
            self.assertEqual(output, "two")

    def test_executable_and_result_payloads_do_not_transfer(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, target = root / "source", root / "target"
            for index, data in enumerate([b"!<arch>\nobject", b"v1 metadata", b"\x7fELFbinary", b"testlog\nresult"]):
                identity = f"{index:064x}"
                path = source / identity[:2] / (identity + ("-a" if index == 1 else "-d"))
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
            result = cache.transfer(source, target)
            self.assertEqual(result["files"], 2)
            self.assertEqual(len(list(cache.entries(target))), 2)

    def test_budget_is_bounded_and_metadata_survives(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, target = root / "source", root / "target"
            for identity, suffix, data in [("a" * 64, "-a", b"v1 metadata"), ("b" * 64, "-d", b"!<arch>\n" + bytes(100))]:
                path = source / identity[:2] / (identity + suffix)
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
            result = cache.transfer(source, target, limit=20)
            self.assertEqual(result["files"], 1)
            self.assertEqual(result["omitted"], 1)
            self.assertLessEqual(result["bytes"], 20)
            self.assertTrue(next(cache.entries(target)).name.endswith("-a"))


if __name__ == "__main__":
    unittest.main()
