"""Local-real F-01..F-14 witnesses for setup packet preflight."""

import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from preflight_test_support import add_file_descriptor, git, valid_packet, write_packet


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"


def load_setup_workspace_module():
    spec = importlib.util.spec_from_file_location("setup_workspace_preflight", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def init_repository(repo_path):
    git(["init", "-b", "main"], repo_path)
    git(["config", "user.email", "setup-workspace-test@example.com"], repo_path)
    git(["config", "user.name", "Setup Workspace Test"], repo_path)
    (repo_path / "README.md").write_text("base\n", encoding="utf-8")
    git(["add", "README.md"], repo_path)
    git(["commit", "-m", "base"], repo_path)


def run_setup(repo_path, prd_name):
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), prd_name],
        cwd=repo_path,
        capture_output=True,
        text=True,
        check=False,
    )


def snapshot(repo_path):
    return {
        "head": git(["rev-parse", "HEAD"], repo_path).stdout,
        "main": git(["rev-parse", "refs/heads/main"], repo_path).stdout,
        "status": git(
            ["status", "--porcelain=v1", "--untracked-files=all"], repo_path,
        ).stdout,
        "index": git(["ls-files", "--stage"], repo_path).stdout,
        "refs": git(["show-ref"], repo_path, check=False).stdout,
    }


class PreDispatchAuthorityInputPreflightTest(unittest.TestCase):
    def setUp(self):
        self.module = load_setup_workspace_module()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name) / "repo"
        self.repo_path.mkdir()
        init_repository(self.repo_path)

    def tearDown(self):
        self.temp_dir.cleanup()

    def write_valid(self, prd_name, packet=None):
        packet = valid_packet(self.repo_path, prd_name) if packet is None else packet
        return write_packet(self.repo_path, prd_name, packet)[0]

    def assert_refusal_preserves_root(self, prd_name, expected):
        before = snapshot(self.repo_path)
        result = run_setup(self.repo_path, prd_name)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertIn(expected, result.stderr)
        self.assertEqual(snapshot(self.repo_path), before)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertFalse((self.repo_path / ".claude" / "worktrees" / prd_name).exists())
        self.assertLessEqual(len(result.stderr), 1400)
        return result

    def test_f01_valid_packet_returns_ready_identity_and_prepared_head(self):
        prd_name = "valid-preflight"
        packet_path = self.write_valid(prd_name)

        result = run_setup(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "ready")
        self.assertEqual(payload["preflight"]["status"], "verified")
        self.assertEqual(
            payload["preflight"]["packet"]["sha256"],
            hashlib.sha256(packet_path.read_bytes()).hexdigest(),
        )
        self.assertEqual(len(payload["preflight"]["verifiedFiles"]), 3)
        self.assertEqual(
            payload["preflight"]["intendedMainline"]["resolvedCheckoutHead"],
            git(["rev-parse", "HEAD"], self.repo_path).stdout.strip(),
        )
        self.assertTrue(Path(payload["worktree"]).exists())
        self.assertFalse((self.repo_path / "dispatch.marker").exists())

    def test_f02_repeated_admission_reuses_one_verified_destination(self):
        prd_name = "repeat-preflight"
        self.write_valid(prd_name)
        first = run_setup(self.repo_path, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        first_payload = json.loads(first.stdout)
        marker = Path(first_payload["worktree"]) / "worker-marker.txt"
        marker.write_text("preserve\n", encoding="utf-8")
        first_head = git(["rev-parse", "HEAD"], Path(first_payload["worktree"]).resolve()).stdout

        second = run_setup(self.repo_path, prd_name)

        self.assertEqual(second.returncode, 0, second.stderr)
        second_payload = json.loads(second.stdout)
        self.assertTrue(second_payload["reused"])
        self.assertEqual(second_payload["worktree"], first_payload["worktree"])
        self.assertEqual(second_payload["preflight"], first_payload["preflight"])
        self.assertEqual(
            git(["rev-parse", "HEAD"], Path(second_payload["worktree"]).resolve()).stdout,
            first_head,
        )
        self.assertTrue(marker.exists())

    def test_f03_missing_authority_is_rejected_before_root_sync(self):
        prd_name = "missing-authority"
        packet_path = self.write_valid(prd_name)
        packet = json.loads(packet_path.read_text(encoding="utf-8"))
        source_plan = Path(packet["preflight"]["authority"]["sourcePlan"]["path"])
        source_plan.unlink()

        self.assert_refusal_preserves_root(
            prd_name,
            "category=authority-input code=missing-input field=preflight.authority.sourcePlan",
        )

    def test_f04_missing_fixture_is_rejected_without_destination(self):
        prd_name = "missing-fixture"
        fixture = add_file_descriptor(
            self.repo_path, prd_name, "fixtures", "fixture.txt", b"fixture\n",
        )
        packet = valid_packet(self.repo_path, prd_name, fixtures=[fixture])
        packet_path = write_packet(self.repo_path, prd_name, packet)[0]
        Path(fixture["path"]).unlink()
        self.assertTrue(packet_path.exists())

        self.assert_refusal_preserves_root(
            prd_name,
            "category=artifact-input code=missing-input field=fixtures[0]",
        )

    def test_f05_malformed_digest_is_bounded_and_typed(self):
        prd_name = "malformed-digest"
        packet = valid_packet(self.repo_path, prd_name)
        packet["preflight"]["authority"]["request"]["sha256"] = "not-a-digest"
        self.write_valid(prd_name, packet)

        result = self.assert_refusal_preserves_root(prd_name, "code=malformed-digest")

        self.assertIn("field=preflight.authority.request.sha256", result.stderr)
        self.assertNotIn("not-a-digest", result.stderr)

    def test_f06_digest_drift_preserves_source_bytes_and_destination_state(self):
        prd_name = "digest-drift"
        packet_path = self.write_valid(prd_name)
        packet = json.loads(packet_path.read_text(encoding="utf-8"))
        source_plan = Path(packet["preflight"]["authority"]["sourcePlan"]["path"])
        source_plan.write_text("drifted but preserved\n", encoding="utf-8")
        before_bytes = source_plan.read_bytes()

        result = self.assert_refusal_preserves_root(
            prd_name,
            "category=authority-input code=digest-mismatch field=preflight.authority.sourcePlan",
        )

        self.assertIn("expected=", result.stderr)
        self.assertIn("observed=", result.stderr)
        self.assertEqual(source_plan.read_bytes(), before_bytes)

    def test_f07_relative_and_non_regular_paths_are_rejected(self):
        for suffix, path_value, observed in (
            ("relative", "authority.md", "observed=\"relative\""),
            ("directory", str(self.repo_path), "observed=\"non-regular\""),
        ):
            with self.subTest(suffix=suffix):
                prd_name = f"invalid-path-{suffix}"
                packet = valid_packet(self.repo_path, prd_name)
                descriptor = packet["preflight"]["authority"]["request"]
                descriptor["path"] = path_value
                self.write_valid(prd_name, packet)
                result = self.assert_refusal_preserves_root(
                    prd_name, "code=input-path field=preflight.authority.request",
                )
                self.assertIn(observed, result.stderr)

    def test_f08_read_interruption_returns_bounded_input_read(self):
        path = self.repo_path / "read-interruption.bin"
        path.write_bytes(b"bytes")
        original = self.module.stream_file_sha256

        def interrupted(_path):
            raise self.module.FileReadFailure(3)

        self.module.stream_file_sha256 = interrupted
        try:
            with self.assertRaises(self.module.PacketPreflightError) as raised:
                self.module.validate_declared_file_descriptor(
                    {
                        "path": str(path),
                        "identity": "sha256:" + "a" * 64,
                        "sha256": "a" * 64,
                    },
                    "fixtures[0]",
                    "artifact-input",
                )
        finally:
            self.module.stream_file_sha256 = original
        self.assertIn("code=input-read", str(raised.exception))
        self.assertIn("observed=\"partial\"", str(raised.exception))

    def test_f09_absent_mainline_is_rejected_without_destination(self):
        prd_name = "missing-mainline"
        packet = valid_packet(self.repo_path, prd_name)
        packet["preflight"]["intendedMainline"]["commit"] = "a" * 40
        self.write_valid(prd_name, packet)

        result = self.assert_refusal_preserves_root(prd_name, "code=missing-mainline")

        self.assertIn("requiredCommit=", result.stderr)
        self.assertIn("resolvedCheckoutHead=", result.stderr)

    def test_f10_non_ancestor_mainline_is_rejected(self):
        prd_name = "non-ancestor-mainline"
        base = git(["rev-parse", "HEAD"], self.repo_path).stdout.strip()
        git(["checkout", "-b", "unrelated"], self.repo_path)
        (self.repo_path / "unrelated.txt").write_text("unrelated\n", encoding="utf-8")
        git(["add", "unrelated.txt"], self.repo_path)
        git(["commit", "-m", "unrelated"], self.repo_path)
        unrelated = git(["rev-parse", "HEAD"], self.repo_path).stdout.strip()
        git(["checkout", "main"], self.repo_path)
        self.assertEqual(git(["rev-parse", "HEAD"], self.repo_path).stdout.strip(), base)
        packet = valid_packet(self.repo_path, prd_name)
        packet["preflight"]["intendedMainline"]["commit"] = unrelated
        self.write_valid(prd_name, packet)

        result = self.assert_refusal_preserves_root(prd_name, "code=non-ancestor")

        self.assertIn(f"resolvedCheckoutHead=\"{base}\"", result.stderr)

    def test_f11_project_mismatch_and_unknown_v1_key_are_rejected(self):
        mismatch_name = "project-mismatch"
        packet = valid_packet(self.repo_path, mismatch_name)
        packet["project"] = "other-project"
        self.write_valid(mismatch_name, packet)
        self.assert_refusal_preserves_root(mismatch_name, "code=contract-mismatch field=project")

        unknown_name = "unknown-preflight-field"
        packet = valid_packet(self.repo_path, unknown_name)
        packet["preflight"]["unexpected"] = "ignored"
        self.write_valid(unknown_name, packet)
        self.assert_refusal_preserves_root(
            unknown_name,
            "code=unknown-field field=preflight.unexpected",
        )

    def test_f12_dirty_unrelated_root_is_identity_equivalent_on_invalid_packet(self):
        prd_name = "dirty-invalid"
        packet = valid_packet(self.repo_path, prd_name)
        missing = add_file_descriptor(
            self.repo_path, prd_name, "fixtures", "gone.txt", b"gone\n",
        )
        packet["fixtures"] = [missing]
        self.write_valid(prd_name, packet)
        Path(missing["path"]).unlink()
        (self.repo_path / "README.md").write_text("unstaged\n", encoding="utf-8")
        (self.repo_path / "staged.txt").write_text("staged\n", encoding="utf-8")
        git(["add", "staged.txt"], self.repo_path)
        (self.repo_path / "untracked.txt").write_text("untracked\n", encoding="utf-8")
        before = snapshot(self.repo_path)

        self.assert_refusal_preserves_root(prd_name, "code=missing-input field=fixtures[0]")
        self.assertEqual(snapshot(self.repo_path), before)

    def test_f13_concurrent_valid_admission_converges_on_one_destination(self):
        prd_name = "concurrent-valid"
        self.write_valid(prd_name)
        command = [sys.executable, str(SCRIPT_PATH), prd_name]
        first = subprocess.Popen(
            command, cwd=self.repo_path, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        second = subprocess.Popen(
            command, cwd=self.repo_path, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        first_stdout, first_stderr = first.communicate()
        second_stdout, second_stderr = second.communicate()

        self.assertEqual(first.returncode, 0, first_stderr)
        self.assertEqual(second.returncode, 0, second_stderr)
        first_payload = json.loads(first_stdout)
        second_payload = json.loads(second_stdout)
        self.assertEqual(first_payload["worktree"], second_payload["worktree"])
        self.assertEqual(first_payload["preflight"], second_payload["preflight"])
        worktree_records = git(["worktree", "list", "--porcelain"], self.repo_path).stdout
        self.assertEqual(worktree_records.count(f"branch refs/heads/{prd_name}"), 1)

    def test_f14_diagnostics_redact_long_identity_and_secret_like_text(self):
        prd_name = "bounded-diagnostic"
        packet = valid_packet(self.repo_path, prd_name)
        packet["preflight"]["authority"]["sourcePlan"]["identity"] = (
            "secret-token-" + "x" * 5000
        )
        self.write_valid(prd_name, packet)

        result = self.assert_refusal_preserves_root(prd_name, "code=malformed-identity")

        self.assertLessEqual(len(result.stderr), 1400)
        self.assertNotIn("secret-token", result.stderr)
        self.assertIn("field=preflight.authority.sourcePlan.identity", result.stderr)
        self.assertIn("next=", result.stderr)


if __name__ == "__main__":
    unittest.main()
