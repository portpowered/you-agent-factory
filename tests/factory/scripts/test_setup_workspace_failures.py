#!/usr/bin/env python3
"""Regression tests for setup-workspace categorized failure output."""

import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr
from pathlib import Path
from unittest import mock

from preflight_test_support import write_packet


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"


def load_setup_workspace_module():
    spec = importlib.util.spec_from_file_location("setup_workspace", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def init_local_repo(repo_path):
    subprocess.run(["git", "init", "-b", "main"], cwd=repo_path, check=True)
    subprocess.run(
        ["git", "config", "user.email", "setup-workspace-test@example.com"],
        cwd=repo_path,
        check=True,
    )
    subprocess.run(
        ["git", "config", "user.name", "Setup Workspace Test"],
        cwd=repo_path,
        check=True,
    )
    readme = repo_path / "README.md"
    readme.write_text("setup-workspace test repo\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=repo_path, check=True)
    subprocess.run(["git", "commit", "-m", "init"], cwd=repo_path, check=True)


def write_prd(repo_path, prd_name, include_md=False):
    return write_packet(repo_path, prd_name, include_md=include_md)


def run_setup_workspace(repo_path, prd_name):
    return subprocess.run(
        ["python3", str(SCRIPT_PATH), prd_name],
        cwd=repo_path,
        capture_output=True,
        text=True,
        check=False,
    )


class SetupWorkspaceFailureTest(unittest.TestCase):
    def setUp(self):
        self.module = load_setup_workspace_module()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name)

    def tearDown(self):
        self.temp_dir.cleanup()

    def run_main_in_repo(self, prd_name):
        stderr = io.StringIO()
        original_cwd = os.getcwd()
        os.chdir(self.repo_path)
        try:
            with mock.patch.object(sys, "argv", ["setup-workspace.py", prd_name]):
                with redirect_stderr(stderr):
                    with self.assertRaises(SystemExit) as raised:
                        self.module.main()
        finally:
            os.chdir(original_cwd)
        return raised.exception.code, stderr.getvalue()

    def test_reports_root_sync_failure_with_concrete_reason(self):
        init_local_repo(self.repo_path)
        prd_name = "root-sync-fail-prd"
        write_prd(self.repo_path, prd_name)

        original_prune = self.module.prune_worktrees

        def failing_prune(_repo_root):
            raise ValueError(
                "git worktree prune failed (exit 128): simulated prune failure"
            )

        self.module.prune_worktrees = failing_prune
        try:
            exit_code, stderr = self.run_main_in_repo(prd_name)
        finally:
            self.module.prune_worktrees = original_prune

        self.assertEqual(exit_code, 1)
        self.assertIn("Root sync failed:", stderr)
        self.assertIn("simulated prune failure", stderr)
        self.assertNotIn("Worktree preparation failed", stderr)
        self.assertNotIn("PRD copy failed", stderr)

    def test_reports_prd_read_failure_with_bounded_unexpected_detail(self):
        init_local_repo(self.repo_path)
        prd_name = "prd-read-fail-prd"
        write_prd(self.repo_path, prd_name)

        def failing_read(_prd_path):
            raise ValueError("malformed PRD detail\n" + ("x" * 5000))

        original_read = self.module.read_prd
        self.module.read_prd = failing_read
        try:
            exit_code, stderr = self.run_main_in_repo(prd_name)
        finally:
            self.module.read_prd = original_read

        self.assertEqual(exit_code, 1)
        self.assertLess(len(stderr), 1300)
        self.assertLess(stderr.count("x"), 1100)
        self.assertIn("Failed to read PRD: malformed PRD detail", stderr)
        self.assertIn("more characters", stderr)
        self.assertNotIn("Traceback", stderr)
        self.assertNotIn("Root sync failed", stderr)

    def test_reports_missing_prd_as_a_prd_read_failure(self):
        init_local_repo(self.repo_path)
        prd_name = "missing-prd"

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertIn("PRD not found", result.stderr)
        self.assertNotIn("Root sync failed", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_reports_unexpected_worktree_failure_with_stage_and_detail(self):
        init_local_repo(self.repo_path)
        prd_name = "worktree-unexpected-fail-prd"
        write_prd(self.repo_path, prd_name)

        def failing_worktree(*_args):
            raise ValueError("simulated worktree preparation failure")

        original_create = self.module.create_or_reuse_worktree
        self.module.create_or_reuse_worktree = failing_worktree
        try:
            exit_code, stderr = self.run_main_in_repo(prd_name)
        finally:
            self.module.create_or_reuse_worktree = original_create

        self.assertEqual(exit_code, 1)
        self.assertIn(
            "Worktree preparation failed: simulated worktree preparation failure",
            stderr,
        )
        self.assertNotIn("Traceback", stderr)
        self.assertNotIn("Root sync failed", stderr)
        self.assertNotIn("PRD copy failed", stderr)

    def test_classifies_windows_reserved_path_with_manual_recovery(self):
        init_local_repo(self.repo_path)
        prd_name = "windows-reserved-path-prd"
        write_prd(self.repo_path, prd_name)

        def failing_worktree(*_args):
            raise RuntimeError(
                "git worktree add failed (exit 128): error: invalid path 'NUL'"
            )

        original_create = self.module.create_or_reuse_worktree
        self.module.create_or_reuse_worktree = failing_worktree
        try:
            exit_code, stderr = self.run_main_in_repo(prd_name)
        finally:
            self.module.create_or_reuse_worktree = original_create

        self.assertEqual(exit_code, 1)
        self.assertIn("Worktree preparation failed:", stderr)
        self.assertIn("Windows-reserved", stderr)
        self.assertIn("literal NUL device name", stderr)
        self.assertIn("manually back up", stderr)
        self.assertIn("remove or rename", stderr)
        self.assertIn("invalid path 'NUL'", stderr)
        self.assertNotIn("Traceback", stderr)

    @unittest.skipUnless(os.name == "nt", "Git path restriction is Windows-specific")
    def test_classifies_reproducible_windows_reserved_nul_worktree_failure(self):
        init_local_repo(self.repo_path)
        prd_name = "windows-nul-fixture-prd"
        write_prd(self.repo_path, prd_name)

        blob = subprocess.run(
            ["git", "hash-object", "-w", "--stdin"],
            cwd=self.repo_path,
            input=b"reserved path\n",
            capture_output=True,
            check=True,
        ).stdout.decode().strip()
        tree = subprocess.run(
            ["git", "mktree"],
            cwd=self.repo_path,
            input=(
                "100644 blob "
                + blob
                + chr(9)
                + "NUL"
                + chr(10)
            ).encode(),
            capture_output=True,
            check=True,
        ).stdout.decode().strip()
        commit = subprocess.run(
            ["git", "commit-tree", tree, "-m", "reserved path fixture"],
            cwd=self.repo_path,
            capture_output=True,
            check=True,
        ).stdout.decode().strip()
        subprocess.run(
            ["git", "update-ref", f"refs/heads/{prd_name}", commit],
            cwd=self.repo_path,
            check=True,
        )

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertIn("Worktree preparation failed:", result.stderr)
        self.assertIn("Windows-reserved", result.stderr)
        self.assertIn("literal NUL device name", result.stderr)
        self.assertIn("manually back up", result.stderr)
        self.assertIn("remove or rename", result.stderr)
        self.assertIn("invalid path 'NUL'", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_reports_unexpected_prd_copy_failure_with_stage_and_detail(self):
        init_local_repo(self.repo_path)
        prd_name = "prd-copy-unexpected-fail-prd"
        write_prd(self.repo_path, prd_name)

        with mock.patch.object(
            self.module,
            "sync_main",
            return_value="already up to date",
        ), mock.patch.object(
            self.module,
            "prune_worktrees",
        ), mock.patch.object(
            self.module,
            "create_or_reuse_worktree",
            return_value=False,
        ), mock.patch.object(
            self.module,
            "copy_prd_files",
            side_effect=ValueError("simulated PRD copy failure"),
        ):
            exit_code, stderr = self.run_main_in_repo(prd_name)

        self.assertEqual(exit_code, 1)
        self.assertIn("PRD copy failed: simulated PRD copy failure", stderr)
        self.assertNotIn("Traceback", stderr)
        self.assertNotIn("Root sync failed", stderr)
        self.assertNotIn("Worktree preparation failed", stderr)

    def test_reports_root_sync_failure_when_no_origin_and_local_main_missing(self):
        init_local_repo(self.repo_path)
        subprocess.run(
            ["git", "checkout", "-b", "feature-branch"],
            cwd=self.repo_path,
            check=True,
        )
        subprocess.run(
            ["git", "update-ref", "-d", "refs/heads/main"],
            cwd=self.repo_path,
            check=True,
        )
        subprocess.run(
            ["git", "remote", "remove", "origin"],
            cwd=self.repo_path,
            check=False,
        )

        prd_name = "no-origin-missing-main-prd"
        write_prd(self.repo_path, prd_name)

        result = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn("Root sync failed:", result.stderr)
        self.assertIn("no origin remote", result.stderr.lower())
        self.assertIn("refs/heads/main is missing", result.stderr)
        self.assertNotIn("Worktree preparation failed", result.stderr)
        self.assertNotIn("PRD copy failed", result.stderr)

    def test_reports_root_sync_failure_when_fetch_fails_without_local_main(self):
        bare_remote = self.repo_path / "remote.git"
        bare_remote.mkdir()
        subprocess.run(
            ["git", "init", "--bare", "-b", "main"],
            cwd=bare_remote,
            check=True,
        )

        local_repo = self.repo_path / "local"
        subprocess.run(
            ["git", "clone", str(bare_remote), str(local_repo.name)],
            cwd=self.repo_path,
            check=True,
        )
        subprocess.run(
            ["git", "checkout", "-b", "feature-branch"],
            cwd=local_repo,
            check=True,
        )
        subprocess.run(
            [
                "git", "config", "user.email",
                "setup-workspace-test@example.com",
            ],
            cwd=local_repo,
            check=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Setup Workspace Test"],
            cwd=local_repo,
            check=True,
        )
        (local_repo / "README.md").write_text("feature base\n", encoding="utf-8")
        subprocess.run(["git", "add", "README.md"], cwd=local_repo, check=True)
        subprocess.run(
            ["git", "commit", "-m", "feature base"],
            cwd=local_repo,
            check=True,
        )
        subprocess.run(
            ["git", "update-ref", "-d", "refs/heads/main"],
            cwd=local_repo,
            check=True,
        )
        subprocess.run(
            ["git", "update-ref", "-d", "refs/remotes/origin/main"],
            cwd=local_repo,
            check=True,
        )

        unreachable_remote = self.repo_path / "missing-remote.git"
        subprocess.run(
            ["git", "remote", "set-url", "origin", str(unreachable_remote)],
            cwd=local_repo,
            check=True,
        )

        prd_name = "blocking-root-sync-prd"
        write_prd(local_repo, prd_name)

        result = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn("Root sync failed:", result.stderr)
        self.assertIn("fetch failed", result.stderr.lower())
        self.assertIn("refs/heads/main is missing", result.stderr)
        self.assertNotIn("Worktree preparation failed", result.stderr)
        self.assertNotIn("PRD copy failed", result.stderr)

    def test_diverged_branch_reuse_reports_no_failure_labels(self):
        bare_remote = self.repo_path / "remote.git"
        bare_remote.mkdir()
        subprocess.run(
            ["git", "init", "--bare", "-b", "main"],
            cwd=bare_remote,
            check=True,
        )

        upstream = self.repo_path / "upstream"
        upstream.mkdir()
        init_local_repo(upstream)
        subprocess.run(
            ["git", "remote", "add", "origin", str(bare_remote)],
            cwd=upstream,
            check=True,
        )
        subprocess.run(["git", "push", "-u", "origin", "main"], cwd=upstream, check=True)

        local_repo = self.repo_path / "local"
        subprocess.run(
            ["git", "clone", str(bare_remote), str(local_repo.name)],
            cwd=self.repo_path,
            check=True,
        )

        prd_name = "worktree-failure-prd"
        write_prd(local_repo, prd_name)
        subprocess.run(["git", "checkout", "-b", prd_name], cwd=local_repo, check=True)
        (local_repo / "seed.txt").write_text("seed\n", encoding="utf-8")
        subprocess.run(["git", "add", "seed.txt"], cwd=local_repo, check=True)
        subprocess.run(["git", "commit", "-m", "seed branch"], cwd=local_repo, check=True)
        subprocess.run(
            ["git", "push", "-u", "origin", prd_name],
            cwd=local_repo,
            check=True,
        )
        subprocess.run(["git", "checkout", "main"], cwd=local_repo, check=True)

        first = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        worktree_path = Path(json.loads(first.stdout)["worktree"])

        (worktree_path / "local-only.txt").write_text("local commit\n", encoding="utf-8")
        subprocess.run(["git", "add", "local-only.txt"], cwd=worktree_path, check=True)
        subprocess.run(
            ["git", "commit", "-m", "diverge locally"],
            cwd=worktree_path,
            check=True,
        )

        subprocess.run(["git", "fetch", "origin"], cwd=upstream, check=True)
        subprocess.run(
            ["git", "checkout", "-B", prd_name, f"origin/{prd_name}"],
            cwd=upstream,
            check=True,
        )
        (upstream / "remote-only.txt").write_text("remote commit\n", encoding="utf-8")
        subprocess.run(["git", "add", "remote-only.txt"], cwd=upstream, check=True)
        subprocess.run(
            ["git", "commit", "-m", "diverge on remote"],
            cwd=upstream,
            check=True,
        )
        subprocess.run(["git", "push", "origin", prd_name], cwd=upstream, check=True)
        subprocess.run(["git", "fetch", "origin"], cwd=local_repo, check=True)

        second = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertIn(
            "skipped (local branch diverged from upstream", second.stderr,
        )
        self.assertNotIn("Worktree preparation failed", second.stderr)
        self.assertNotIn("Root sync failed", second.stderr)
        self.assertNotIn("PRD copy failed", second.stderr)

    def test_reports_prd_copy_failure_after_worktree_preparation(self):
        init_local_repo(self.repo_path)
        prd_name = "prd-copy-fail-prd"
        prd_json, _ = write_prd(self.repo_path, prd_name)

        first = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        worktree_path = Path(json.loads(first.stdout)["worktree"])
        dest_json = worktree_path / "prd.json"
        dest_json.chmod(0o444)

        packet = json.loads(prd_json.read_text(encoding="utf-8"))
        packet["updated"] = True
        prd_json.write_text(json.dumps(packet), encoding="utf-8")

        try:
            second = run_setup_workspace(self.repo_path, prd_name)
        finally:
            dest_json.chmod(0o644)

        self.assertEqual(second.returncode, 1, second.stdout)
        self.assertIn("PRD copy failed:", second.stderr)
        self.assertIn("Permission denied", second.stderr)
        self.assertNotIn("Root sync failed", second.stderr)
        self.assertNotIn("Worktree preparation failed", second.stderr)


if __name__ == "__main__":
    unittest.main()
