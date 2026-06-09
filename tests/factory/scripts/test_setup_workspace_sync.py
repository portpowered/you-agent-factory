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


def git(args, cwd):
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
    )


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
