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
    exclude_path = repo_path / ".git" / "info" / "exclude"
    existing_excludes = (
        exclude_path.read_text(encoding="utf-8")
        if exclude_path.exists()
        else ""
    )
    missing_excludes = [
        entry
        for entry in ("tasks/todo/", ".claude/")
        if entry not in existing_excludes.splitlines()
    ]
    if missing_excludes:
        exclude_path.write_text(
            existing_excludes.rstrip("\n")
            + "\n"
            + "\n".join(missing_excludes)
            + "\n",
            encoding="utf-8",
        )

    tasks_dir = repo_path / "tasks" / "todo"
    tasks_dir.mkdir(parents=True)
    prd_json = tasks_dir / f"{prd_name}.json"
    prd_json.write_text(
        json.dumps({"branchName": prd_name}),
        encoding="utf-8",
    )
    prd_md = None
    if include_md:
        prd_md = tasks_dir / f"{prd_name}.md"
        prd_md.write_text(f"# {prd_name}\n", encoding="utf-8")
    return prd_json, prd_md


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
            raise RuntimeError(
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

        prd_json.write_text(
            json.dumps({"branchName": prd_name, "updated": True}),
            encoding="utf-8",
        )

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
