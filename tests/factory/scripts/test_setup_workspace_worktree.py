#!/usr/bin/env python3
"""Regression tests for setup-workspace worktree setup after safe sync."""

import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"

EXPECTED_RESULT_KEYS = {
    "status",
    "worktree",
    "branch",
    "prd_path",
    "prd_md_path",
    "reused",
}


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


def write_prd(repo_path, prd_name, include_md=False):
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


class SetupWorkspaceWorktreeTest(unittest.TestCase):
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

    def test_prune_runs_after_sync_and_before_worktree_creation(self):
        init_local_repo(self.repo_path)
        prd_name = "prune-order-prd"
        write_prd(self.repo_path, prd_name)

        recorded, original_run_git = self.record_git_commands()
        try:
            self.module.sync_main(self.repo_path)
            self.module.prune_worktrees(self.repo_path)
            worktree_dir = (
                self.repo_path
                / ".claude"
                / "worktrees"
                / self.module.normalize_branch(prd_name)
            )
            self.module.create_or_reuse_worktree(
                self.repo_path, prd_name, worktree_dir,
            )
        finally:
            self.module.run_git = original_run_git

        prune_index = next(
            i for i, args in enumerate(recorded) if args[:2] == ("worktree", "prune")
        )
        add_index = next(
            i for i, args in enumerate(recorded) if args[:2] == ("worktree", "add")
        )
        self.assertLess(prune_index, add_index)

    def test_reuses_valid_worktree_with_expected_json_shape(self):
        init_local_repo(self.repo_path)
        prd_name = "reuse-worktree-prd"
        write_prd(self.repo_path, prd_name, include_md=True)

        first = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        first_payload = json.loads(first.stdout)
        self.assertEqual(set(first_payload), EXPECTED_RESULT_KEYS)
        self.assertFalse(first_payload["reused"])

        marker = Path(first_payload["worktree"]) / "reuse-marker.txt"
        marker.write_text("keep me\n", encoding="utf-8")

        second = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(second.returncode, 0, second.stderr)
        second_payload = json.loads(second.stdout)
        self.assertEqual(set(second_payload), EXPECTED_RESULT_KEYS)
        self.assertTrue(second_payload["reused"])
        self.assertEqual(second_payload["worktree"], first_payload["worktree"])
        self.assertEqual(second_payload["branch"], prd_name)
        self.assertTrue(marker.exists())

    def test_creates_new_worktree_from_main_when_branch_missing(self):
        init_local_repo(self.repo_path)
        prd_name = "new-branch-prd"
        write_prd(self.repo_path, prd_name)

        main_sha = git(["rev-parse", "main"], self.repo_path).stdout.strip()
        self.assertFalse(self.module.branch_exists_locally(self.repo_path, prd_name))
        self.assertFalse(self.module.branch_exists_on_remote(self.repo_path, prd_name))

        result = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)

        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        self.assertTrue(worktree_path.exists())
        branch_sha = git(
            ["rev-parse", prd_name],
            worktree_path,
        ).stdout.strip()
        self.assertEqual(branch_sha, main_sha)
        self.assertTrue(self.module.branch_exists_locally(self.repo_path, prd_name))

    def test_copies_prd_json_and_optional_markdown(self):
        init_local_repo(self.repo_path)
        prd_name = "copy-prd-prd"
        prd_json, prd_md = write_prd(self.repo_path, prd_name, include_md=True)

        result = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)

        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        self.assertEqual(
            (worktree_path / "prd.json").read_text(encoding="utf-8"),
            prd_json.read_text(encoding="utf-8"),
        )
        self.assertEqual(
            (worktree_path / "prd.md").read_text(encoding="utf-8"),
            prd_md.read_text(encoding="utf-8"),
        )
        self.assertEqual(payload["prd_path"], str(worktree_path / "prd.json"))
        self.assertEqual(payload["prd_md_path"], str(worktree_path / "prd.md"))

    def test_copies_prd_json_without_markdown_when_md_missing(self):
        init_local_repo(self.repo_path)
        prd_name = "json-only-prd"
        write_prd(self.repo_path, prd_name, include_md=False)

        result = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)

        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        self.assertTrue((worktree_path / "prd.json").exists())
        self.assertFalse((worktree_path / "prd.md").exists())
        self.assertIsNone(payload["prd_md_path"])


if __name__ == "__main__":
    unittest.main()
