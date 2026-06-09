#!/usr/bin/env python3
"""Regression tests for setup-workspace sync_main quiet-skip behavior."""

import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


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


def git(args, cwd, check=True):
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=check,
        capture_output=True,
        text=True,
    )


def setup_repo_with_origin_main_ahead(local_repo, repo_root):
    """Create a bare remote ahead of local main; local repo on a feature branch."""
    bare_remote = repo_root / "remote.git"
    bare_remote.mkdir()
    git(["init", "--bare", "-b", "main"], bare_remote)

    upstream = repo_root / "upstream"
    upstream.mkdir()
    init_local_repo(upstream)
    git(["remote", "add", "origin", str(bare_remote)], upstream)
    git(["push", "-u", "origin", "main"], upstream)

    (upstream / "ahead.txt").write_text("origin is ahead\n", encoding="utf-8")
    git(["add", "ahead.txt"], upstream)
    git(["commit", "-m", "advance origin main"], upstream)
    git(["push", "origin", "main"], upstream)

    git(["clone", str(bare_remote), str(local_repo.name)], repo_root)
    git(["reset", "--hard", "HEAD~1"], local_repo)
    git(["checkout", "-b", "feature-branch"], local_repo)
    (local_repo / "dirty.txt").write_text("unstaged change\n", encoding="utf-8")
    git(["fetch", "origin"], local_repo)

    return bare_remote


class SetupWorkspaceSyncTest(unittest.TestCase):
    def setUp(self):
        self.module = load_setup_workspace_module()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name)

    def tearDown(self):
        self.temp_dir.cleanup()

    def record_git_commands(self):
        recorded = []
        original_run_git = self.module.run_git

        def tracking_run_git(*args, **kwargs):
            recorded.append(args)
            return original_run_git(*args, **kwargs)

        self.module.run_git = tracking_run_git
        return recorded, original_run_git

    def test_sync_main_skips_quietly_without_origin_remote(self):
        init_local_repo(self.repo_path)

        recorded, original_run_git = self.record_git_commands()
        try:
            self.module.sync_main(self.repo_path)
        finally:
            self.module.run_git = original_run_git

        self.assertFalse(any(args and args[0] == "pull" for args in recorded))
        self.assertFalse(any(args and args[0] == "fetch" for args in recorded))

    def test_sync_main_skips_quietly_without_origin_main(self):
        init_local_repo(self.repo_path)
        bare_remote = self.repo_path / "remote.git"
        bare_remote.mkdir()
        git(["init", "--bare", "-b", "develop"], bare_remote)
        git(["remote", "add", "origin", str(bare_remote)], self.repo_path)
        git(["fetch", "origin"], self.repo_path)

        recorded, original_run_git = self.record_git_commands()
        try:
            self.module.sync_main(self.repo_path)
        finally:
            self.module.run_git = original_run_git

        self.assertFalse(any(args and args[0] == "pull" for args in recorded))
        self.assertTrue(any(args[:2] == ("fetch", "origin") for args in recorded))
        self.assertFalse(self.module.origin_main_ref_exists(self.repo_path))

    def test_sync_main_does_not_fast_forward_after_failed_fetch(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)

        origin_main_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()
        local_main_sha_before = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertNotEqual(local_main_sha_before, origin_main_sha)
        self.assertTrue(self.module.origin_main_ref_exists(local_repo))

        missing_remote = self.repo_path / "missing-remote.git"
        git(["remote", "set-url", "origin", str(missing_remote)], local_repo)

        recorded, original_run_git = self.record_git_commands()
        try:
            self.module.sync_main(local_repo)
        finally:
            self.module.run_git = original_run_git

        local_main_sha_after = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertEqual(local_main_sha_after, local_main_sha_before)
        self.assertNotEqual(local_main_sha_after, origin_main_sha)
        self.assertFalse(
            any(
                args[:3] == ("update-ref", "refs/heads/main", origin_main_sha)
                for args in recorded
            )
        )

    def test_sync_main_fast_forwards_main_without_pull_on_dirty_tree(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)

        origin_main_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()
        local_main_sha_before = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertNotEqual(local_main_sha_before, origin_main_sha)

        recorded, original_run_git = self.record_git_commands()
        try:
            self.module.sync_main(local_repo)
        finally:
            self.module.run_git = original_run_git

        local_main_sha_after = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertEqual(local_main_sha_after, origin_main_sha)
        self.assertEqual(
            git(["branch", "--show-current"], local_repo).stdout.strip(),
            "feature-branch",
        )
        self.assertEqual(
            (local_repo / "dirty.txt").read_text(encoding="utf-8"),
            "unstaged change\n",
        )
        status = git(["status", "--porcelain"], local_repo).stdout
        self.assertIn("?? dirty.txt", status)

        self.assertFalse(any(args and args[0] == "pull" for args in recorded))
        self.assertTrue(any(args[:2] == ("fetch", "origin") for args in recorded))
        self.assertTrue(
            any(
                args[:3] == ("update-ref", "refs/heads/main", origin_main_sha)
                for args in recorded
            )
        )
        self.assertFalse(any(args and args[0] == "checkout" for args in recorded))

    def test_setup_workspace_exits_zero_through_sync_on_dirty_tree(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)

        prd_name = "dirty-tree-prd"
        tasks_dir = local_repo / "tasks" / "todo"
        tasks_dir.mkdir(parents=True)
        prd_json = tasks_dir / f"{prd_name}.json"
        prd_json.write_text(
            json.dumps({"branchName": prd_name}),
            encoding="utf-8",
        )

        result = subprocess.run(
            ["python3", str(SCRIPT_PATH), prd_name],
            cwd=local_repo,
            capture_output=True,
            text=True,
            check=False,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        origin_main_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()
        local_main_sha = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertEqual(local_main_sha, origin_main_sha)
        self.assertEqual(
            git(["branch", "--show-current"], local_repo).stdout.strip(),
            "feature-branch",
        )
        self.assertIn("?? dirty.txt", git(["status", "--porcelain"], local_repo).stdout)

    def test_setup_workspace_continues_after_sync_skip(self):
        init_local_repo(self.repo_path)
        prd_name = "test-workspace-prd"
        tasks_dir = self.repo_path / "tasks" / "todo"
        tasks_dir.mkdir(parents=True)
        prd_json = tasks_dir / f"{prd_name}.json"
        prd_json.write_text(
            json.dumps({"branchName": prd_name}),
            encoding="utf-8",
        )

        result = subprocess.run(
            ["python3", str(SCRIPT_PATH), prd_name],
            cwd=self.repo_path,
            capture_output=True,
            text=True,
            check=False,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "ready")
        worktree_path = Path(payload["worktree"])
        self.assertTrue(worktree_path.exists())
        self.assertTrue((worktree_path / "prd.json").exists())
        self.assertEqual(payload["branch"], prd_name)


if __name__ == "__main__":
    unittest.main()
