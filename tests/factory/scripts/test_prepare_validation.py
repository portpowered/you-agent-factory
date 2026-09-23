"""Artifact and isolation admission proofs, without provider calls."""

import hashlib
import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.dont_write_bytecode = True
SPEC = importlib.util.spec_from_file_location(
    "prepare_validation",
    Path(__file__).resolve().parents[3] / "factory/scripts/prepare-validation.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class PrepareValidationTest(unittest.TestCase):
    def mission(self, root):
        binary = root / "you.exe"
        binary.write_bytes(b"prebuilt-test-artifact")
        return {
            "role": "customer",
            "project": "localai",
            "mission": "Transcribe the fixture",
            "criteria": [
                {"id": "LA-01", "rubric": "The transcript matches the fixture."},
            ],
            "reportPath": str(
                root / "docs/temp/projects/localai/validation/probe.md"
            ),
            "budget": {
                "time": "20m",
                "download": "0",
                "disk": "1MB",
                "process": "2",
                "paid": "0",
            },
            "build": {
                "identity": "fixture-build",
                "path": str(binary),
                "sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
            },
        }

    def test_stages_verified_bytes_without_packet_preflight(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            mission = self.mission(root)
            self.assertNotIn("preflight", mission)

            target = MODULE.prepare(root, "probe-1", json.dumps(mission))

            staged = json.loads((target / "mission.json").read_text())
            self.assertNotIn("preflight", staged)
            self.assertEqual(
                Path(staged["build"]["path"]).read_bytes(),
                b"prebuilt-test-artifact",
            )
            self.assertNotEqual(staged["build"]["path"], mission["build"]["path"])
            for path in staged["environment"].values():
                self.assertTrue(Path(path).is_relative_to(target))
                self.assertTrue(Path(path).is_dir())
            with self.assertRaisesRegex(ValueError, "already exists"):
                MODULE.prepare(root, "probe-1", json.dumps(mission))

    def test_wrong_digest_never_creates_usable_mission(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            mission = self.mission(root)
            mission["build"]["sha256"] = "0" * 64
            with self.assertRaisesRegex(ValueError, "digest-mismatch"):
                MODULE.prepare(root, "probe-bad", json.dumps(mission))
            self.assertFalse(
                (root / "docs/temp/probes/probe-bad/mission.json").exists()
            )

    def test_rejects_contract_leaks_and_invalid_admission(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for field, value in (
                ("role", "unknown"),
                ("build", None),
                ("criteria", []),
                ("reportPath", "../outside.md"),
                ("implementationPlan", "secret recipe"),
            ):
                with self.subTest(field=field):
                    mission = self.mission(root)
                    mission[field] = value
                    with self.assertRaises(ValueError):
                        MODULE.prepare(root, "probe-1", json.dumps(mission))
            with self.assertRaises(ValueError):
                MODULE.prepare(root, "../escape", json.dumps(self.mission(root)))

    def test_malformed_artifacts_fail_before_probe_creation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            malformed = (
                ("non-object-build", "build", "not-an-artifact"),
                (
                    "missing-build-identity",
                    "build",
                    {"path": str(root / "you.exe"), "sha256": "0" * 64},
                ),
                ("non-array-fixtures", "fixtures", {}),
                (
                    "missing-fixture-identity",
                    "fixtures",
                    [{"path": str(root / "you.exe"), "sha256": "0" * 64}],
                ),
            )
            for name, field, value in malformed:
                with self.subTest(name=name):
                    mission = self.mission(root)
                    mission[field] = value
                    with self.assertRaises((ValueError, TypeError)):
                        MODULE.prepare(root, name, json.dumps(mission))
                    self.assertFalse((root / "docs/temp/probes" / name).exists())


if __name__ == "__main__":
    unittest.main()
