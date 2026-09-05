"""V-01..V-12 local-real validation preparation witnesses."""

import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


sys.dont_write_bytecode = True
REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory/scripts/prepare-validation.py"
SPEC = importlib.util.spec_from_file_location("prepare_validation", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def git(args, cwd, check=True):
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=check,
        capture_output=True,
        text=True,
    )


def descriptor(path):
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    return {
        "path": str(path),
        "identity": f"sha256:{digest}",
        "sha256": digest,
    }


def init_repository(root):
    git(["init", "-b", "main"], root)
    git(["config", "user.email", "prepare-validation-test@example.com"], root)
    git(["config", "user.name", "Prepare Validation Test"], root)
    (root / "README.md").write_text("base\n", encoding="utf-8")
    git(["add", "README.md"], root)
    git(["commit", "-m", "base"], root)


class PrepareValidationTest(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name) / "repo"
        self.root.mkdir()
        init_repository(self.root)

    def tearDown(self):
        self.temp_dir.cleanup()

    def mission(self, role="engineering"):
        authority_root = self.root / "authority"
        authority_root.mkdir(exist_ok=True)
        authority = {}
        for field, contents in (
            ("sourcePlan", "source plan\n"),
            ("request", "request\n"),
            ("acceptance", "acceptance\n"),
        ):
            path = authority_root / f"{field}.md"
            path.write_text(contents, encoding="utf-8")
            authority[field] = descriptor(path)

        project_root = self.root / "project"
        project_root.mkdir(exist_ok=True)
        build_path = self.root / "inputs" / "you.exe"
        build_path.parent.mkdir(exist_ok=True)
        build_path.write_bytes(b"prebuilt-test-artifact")
        fixture_path = self.root / "inputs" / "fixture.txt"
        fixture_path.write_bytes(b"fixture bytes\n")
        public_doc_path = self.root / "inputs" / "public.md"
        public_doc_path.write_text("public evidence\n", encoding="utf-8")
        head = git(["rev-parse", "HEAD"], self.root).stdout.strip()
        return {
            "role": role,
            "project": "factory-service-health",
            "mission": "run the validation probe",
            "criteria": [
                {
                    "id": "HEALTH-02",
                    "rubric": "The pre-dispatch input checks fail closed.",
                }
            ],
            "reportPath": str(
                self.root / "docs/temp/projects/factory-service-health/report.md"
            ),
            "budget": {
                "time": "45 minutes",
                "download": "0 bytes",
                "disk": "512 MiB",
                "process": "4 disposable child processes",
                "paid": "USD 0",
            },
            "preflight": {
                "version": "factory-preflight.v1",
                "projectRoot": str(project_root),
                "projectIdentity": "factory-service-health",
                "contractRevision": "factory-service-health-v1",
                "authority": authority,
                "intendedMainline": {"commit": head},
            },
            "build": None if role == "retrospective" else descriptor(build_path),
            "fixtures": [descriptor(fixture_path)],
            "publicDocs": [descriptor(public_doc_path)],
        }

    def run_cli(self, name, mission):
        return subprocess.run(
            [sys.executable, str(SCRIPT_PATH), name, json.dumps(mission)],
            cwd=self.root,
            capture_output=True,
            text=True,
            check=False,
        )

    def target(self, name):
        return self.root / "docs/temp/probes" / name

    def assert_failure(self, name, mission, expected):
        result = self.run_cli(name, mission)
        self.assertEqual(result.returncode, 2, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertIn(expected, result.stderr)
        self.assertLessEqual(len(result.stderr), 1400)
        self.assertFalse((self.target(name) / "mission.json").exists())
        return result

    def test_v01_valid_engineering_mission_records_sanitized_identities(self):
        name = "v01-valid"
        mission = self.mission()
        payload = json.dumps(mission)

        result = self.run_cli(name, mission)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "validation workspace ready\n")
        target = self.target(name)
        record = json.loads((target / "mission.json").read_text(encoding="utf-8"))
        preflight = record["preflight"]
        self.assertEqual(preflight["version"], "factory-preflight.v1")
        self.assertEqual(preflight["status"], "verified")
        self.assertEqual(
            preflight["packet"]["sha256"],
            hashlib.sha256(payload.encode("utf-8")).hexdigest(),
        )
        self.assertEqual(preflight["projectIdentity"], "factory-service-health")
        self.assertEqual(preflight["contractRevision"], "factory-service-health-v1")
        self.assertEqual(
            [entry["field"] for entry in preflight["verifiedAuthority"]],
            [
                "preflight.authority.sourcePlan",
                "preflight.authority.request",
                "preflight.authority.acceptance",
            ],
        )
        self.assertEqual(
            preflight["intendedMainline"]["resolvedCheckoutHead"],
            git(["rev-parse", "HEAD"], self.root).stdout.strip(),
        )
        self.assertNotIn("projectRoot", preflight)
        serialized_record = json.dumps(record)
        for authority_file in mission["preflight"]["authority"].values():
            self.assertNotIn(authority_file["path"], serialized_record)

        for field in ("build", "fixtures", "publicDocs"):
            values = record[field]
            if isinstance(values, dict):
                values = [values]
            for value in values:
                staged = Path(value["path"])
                self.assertTrue(staged.is_relative_to(target))
                self.assertEqual(
                    hashlib.sha256(staged.read_bytes()).hexdigest(),
                    value["sha256"],
                )
        for path in record["environment"].values():
            self.assertTrue(Path(path).is_relative_to(target))
            self.assertTrue(Path(path).is_dir())
        self.assertTrue(
            (self.root / "docs/temp/projects/factory-service-health").is_dir()
        )

    def test_customer_role_keeps_prebuilt_artifact_policy(self):
        name = "customer-role"
        mission = self.mission(role="customer")

        result = self.run_cli(name, mission)

        self.assertEqual(result.returncode, 0, result.stderr)
        record = json.loads(
            (self.target(name) / "mission.json").read_text(encoding="utf-8")
        )
        self.assertEqual(record["role"], "customer")
        self.assertTrue(Path(record["build"]["path"]).is_relative_to(self.target(name)))

    def test_v02_missing_authority_fails_before_target_readiness(self):
        mission = self.mission()
        source_plan = Path(mission["preflight"]["authority"]["sourcePlan"]["path"])
        source_plan.unlink()

        self.assert_failure(
            "v02-missing-authority",
            mission,
            "category=authority-input code=missing-input "
            "field=preflight.authority.sourcePlan",
        )
        self.assertFalse(self.target("v02-missing-authority").exists())

    def test_v03_missing_fixture_fails_before_target_readiness(self):
        mission = self.mission()
        fixture = Path(mission["fixtures"][0]["path"])
        fixture.unlink()

        self.assert_failure(
            "v03-missing-fixture",
            mission,
            "category=artifact-input code=missing-input field=fixtures[0]",
        )
        self.assertFalse(self.target("v03-missing-fixture").exists())

    def test_v04_malformed_digest_is_typed_and_bounded(self):
        mission = self.mission()
        mission["fixtures"][0]["sha256"] = "not-a-digest"

        result = self.assert_failure(
            "v04-malformed-digest",
            mission,
            "code=malformed-digest field=fixtures[0].sha256",
        )

        self.assertNotIn("not-a-digest", result.stderr)

    def test_v05_identity_mismatch_is_typed_and_bounded(self):
        mission = self.mission()
        mission["build"]["identity"] = "sha256:" + "b" * 64

        result = self.assert_failure(
            "v05-identity-mismatch",
            mission,
            "code=identity-mismatch field=build",
        )

        self.assertIn("expected=", result.stderr)
        self.assertIn("observed=", result.stderr)

    def test_v06_digest_drift_preserves_source_and_creates_no_target(self):
        mission = self.mission()
        source_plan = Path(mission["preflight"]["authority"]["sourcePlan"]["path"])
        source_plan.write_text("drifted authority bytes\n", encoding="utf-8")
        source_bytes = source_plan.read_bytes()

        result = self.assert_failure(
            "v06-digest-drift",
            mission,
            "category=authority-input code=digest-mismatch "
            "field=preflight.authority.sourcePlan",
        )

        self.assertIn("expected=", result.stderr)
        self.assertIn("observed=", result.stderr)
        self.assertEqual(source_plan.read_bytes(), source_bytes)
        self.assertFalse(self.target("v06-digest-drift").exists())

    def test_v07_absent_mainline_reports_required_and_resolved_heads(self):
        mission = self.mission()
        mission["preflight"]["intendedMainline"]["commit"] = "a" * 40

        result = self.assert_failure(
            "v07-missing-mainline",
            mission,
            "code=missing-mainline field=preflight.intendedMainline.commit",
        )

        self.assertIn("requiredCommit=", result.stderr)
        self.assertIn("resolvedCheckoutHead=", result.stderr)
        self.assertFalse(self.target("v07-missing-mainline").exists())

    def test_v08_non_ancestor_mainline_is_rejected(self):
        mission = self.mission()
        base = git(["rev-parse", "HEAD"], self.root).stdout.strip()
        git(["checkout", "-b", "unrelated"], self.root)
        (self.root / "unrelated.txt").write_text("unrelated\n", encoding="utf-8")
        git(["add", "unrelated.txt"], self.root)
        git(["commit", "-m", "unrelated"], self.root)
        unrelated = git(["rev-parse", "HEAD"], self.root).stdout.strip()
        git(["checkout", "main"], self.root)
        self.assertEqual(
            git(["rev-parse", "HEAD"], self.root).stdout.strip(), base,
        )
        mission["preflight"]["intendedMainline"]["commit"] = unrelated

        result = self.assert_failure(
            "v08-non-ancestor",
            mission,
            "code=non-ancestor field=preflight.intendedMainline.commit",
        )

        self.assertIn(f'resolvedCheckoutHead="{base}"', result.stderr)
        self.assertFalse(self.target("v08-non-ancestor").exists())

    def test_v09_post_target_read_interruption_leaves_recoverable_evidence(self):
        name = "v09-interrupted-stage"
        mission = self.mission()
        original = MODULE._PACKET_PREFLIGHT.stream_file_sha256

        def interrupted(path):
            if Path(path).is_relative_to(self.target(name)):
                raise MODULE._PACKET_PREFLIGHT.FileReadFailure(3)
            return original(path)

        MODULE._PACKET_PREFLIGHT.stream_file_sha256 = interrupted
        try:
            with self.assertRaises(MODULE.PacketPreflightError) as raised:
                MODULE.prepare(self.root, name, json.dumps(mission))
        finally:
            MODULE._PACKET_PREFLIGHT.stream_file_sha256 = original

        self.assertIn(
            "category=artifact-staging code=input-read field=build",
            str(raised.exception),
        )
        target = self.target(name)
        self.assertTrue(target.is_dir())
        self.assertTrue((target / "bin" / "you.exe").exists())
        self.assertFalse((target / "mission.json").exists())

    def test_v10_repeated_admission_preserves_first_mission_and_staged_bytes(self):
        name = "v10-repeated"
        mission = self.mission()
        first = self.run_cli(name, mission)
        self.assertEqual(first.returncode, 0, first.stderr)
        target = self.target(name)
        first_record = (target / "mission.json").read_bytes()
        first_build = (target / "bin" / "you.exe").read_bytes()

        second = self.run_cli(name, mission)

        self.assertEqual(second.returncode, 2)
        self.assertIn("validation workspace already exists", second.stderr)
        self.assertEqual((target / "mission.json").read_bytes(), first_record)
        self.assertEqual((target / "bin" / "you.exe").read_bytes(), first_build)

    def test_v11_concurrent_admission_has_one_success_and_one_fresh_name_failure(self):
        name = "v11-concurrent"
        mission = self.mission()
        command = [sys.executable, str(SCRIPT_PATH), name, json.dumps(mission)]
        first = subprocess.Popen(
            command,
            cwd=self.root,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        second = subprocess.Popen(
            command,
            cwd=self.root,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        first_stdout, first_stderr = first.communicate()
        second_stdout, second_stderr = second.communicate()
        results = [
            (first.returncode, first_stdout, first_stderr),
            (second.returncode, second_stdout, second_stderr),
        ]

        self.assertEqual(sorted(result[0] for result in results), [0, 2])
        success = next(result for result in results if result[0] == 0)
        failure = next(result for result in results if result[0] == 2)
        self.assertEqual(success[1], "validation workspace ready\n")
        self.assertIn("validation workspace already exists", failure[2])
        self.assertTrue((self.target(name) / "mission.json").exists())
        self.assertLessEqual(len(failure[2]), 1400)

    def test_v12_retrospective_mission_keeps_no_build_policy_with_preflight(self):
        name = "v12-retrospective"
        mission = self.mission(role="retrospective")

        result = self.run_cli(name, mission)

        self.assertEqual(result.returncode, 0, result.stderr)
        record = json.loads(
            (self.target(name) / "mission.json").read_text(encoding="utf-8")
        )
        self.assertIsNone(record["build"])
        self.assertEqual(record["preflight"]["status"], "verified")
        self.assertEqual(
            record["preflight"]["intendedMainline"]["requiredCommit"],
            git(["rev-parse", "HEAD"], self.root).stdout.strip(),
        )

    def test_rejects_contract_leaks_and_invalid_admission(self):
        for field, value in (
            ("role", "unknown"),
            ("build", None),
            ("criteria", []),
            ("reportPath", "../outside.md"),
            ("implementationPlan", "secret recipe"),
        ):
            with self.subTest(field=field):
                mission = self.mission()
                mission[field] = value
                with self.assertRaises((ValueError, MODULE.PacketPreflightError)):
                    MODULE.prepare(
                        self.root, f"invalid-{field}", json.dumps(mission),
                    )
        with self.assertRaises(ValueError):
            MODULE.prepare(self.root, "../escape", json.dumps(self.mission()))


if __name__ == "__main__":
    unittest.main()
