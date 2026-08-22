#!/usr/bin/env python3
"""Regression tests for refusing a dirty repository root during setup."""

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
from types import SimpleNamespace
from unittest import mock


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"


def load_setup_workspace_module():
    spec = importlib.util.spec_from_file_location(
        "setup_workspace_dirty_root", SCRIPT_PATH,
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def git(args, cwd, check=True):
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=check,
        capture_output=True,
        text=True,
    )


def init_repository(repo_path):
    git(["init", "-b", "main"], repo_path)
    git(
        ["config", "user.email", "setup-workspace-test@example.com"],
        repo_path,
    )
    git(["config", "user.name", "Setup Workspace Test"], repo_path)
    (repo_path / "README.md").write_text("base\n", encoding="utf-8")
    git(["add", "README.md"], repo_path)
    git(["commit", "-m", "base"], repo_path)

    exclude_path = repo_path / ".git" / "info" / "exclude"
    exclude_path.write_text(
        "tasks/todo/\nignored/\n.claude/\n",
        encoding="utf-8",
    )


def write_prd(repo_path, prd_name):
    tasks_dir = repo_path / "tasks" / "todo"
    tasks_dir.mkdir(parents=True)
    prd_path = tasks_dir / f"{prd_name}.json"
    prd_path.write_text(
        json.dumps({"branchName": prd_name}),
        encoding="utf-8",
    )


def run_setup_workspace(repo_path, prd_name):
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), prd_name],
        cwd=repo_path,
        capture_output=True,
        text=True,
        check=False,
    )


class SetupWorkspaceDirtyRootTest(unittest.TestCase):
    def setUp(self):
        self.module = load_setup_workspace_module()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name)

    def tearDown(self):
        self.temp_dir.cleanup()

    def run_refusal_case(self, mutate_root):
        init_repository(self.repo_path)
        prd_name = "dirty-root-prd"
        write_prd(self.repo_path, prd_name)
        mutate_root()

        head_before = git(["rev-parse", "HEAD"], self.repo_path).stdout
        status_before = git(["status", "--porcelain=v1"], self.repo_path).stdout
        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertIn("repository root is dirty", result.stderr.lower())
        self.assertIn(str(self.repo_path), result.stderr)
        self.assertIn("total entries=", result.stderr)
        self.assertIn("tracked changes=", result.stderr)
        self.assertIn("untracked files=", result.stderr)
        self.assertIn("Inspect the repository root manually", result.stderr)
        self.assertIn("commit the changes", result.stderr)
        self.assertIn("back them up and restore them manually", result.stderr)
        self.assertEqual(
            git(["rev-parse", "HEAD"], self.repo_path).stdout,
            head_before,
        )
        self.assertEqual(
            git(["status", "--porcelain=v1"], self.repo_path).stdout,
            status_before,
        )
        self.assertFalse(
            (self.repo_path / ".claude" / "worktrees" / prd_name).exists()
        )

    def test_refuses_unstaged_tracked_change_before_mutation(self):
        def mutate():
            (self.repo_path / "README.md").write_text(
                "operator edit\n", encoding="utf-8",
            )

        self.run_refusal_case(mutate)

    def test_refuses_staged_tracked_change_before_mutation(self):
        def mutate():
            staged_path = self.repo_path / "staged.txt"
            staged_path.write_text("staged\n", encoding="utf-8")
            git(["add", staged_path.name], self.repo_path)

        self.run_refusal_case(mutate)

    def test_refuses_mixed_tracked_and_untracked_changes_with_separate_counts(self):
        def mutate():
            (self.repo_path / "README.md").write_text(
                "operator edit\n", encoding="utf-8",
            )
            (self.repo_path / "untracked.txt").write_text(
                "operator data\n", encoding="utf-8",
            )

        self.run_refusal_case(mutate)
        result = run_setup_workspace(self.repo_path, "dirty-root-prd")
        self.assertIn("tracked changes=1", result.stderr)
        self.assertIn("untracked files=1", result.stderr)

    def test_ignored_only_root_remains_eligible(self):
        init_repository(self.repo_path)
        prd_name = "ignored-root-prd"
        write_prd(self.repo_path, prd_name)
        ignored_path = self.repo_path / "ignored" / "build-output.bin"
        ignored_path.parent.mkdir()
        ignored_path.write_bytes(b"ignored\0artifact")

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "ready")
        self.assertTrue(ignored_path.exists())

    def test_unusual_path_is_safely_escaped(self):
        init_repository(self.repo_path)
        prd_name = "unusual-path-prd"
        write_prd(self.repo_path, prd_name)
        unusual_name = "operator [path] café.txt"
        (self.repo_path / unusual_name).write_text(
            "operator data\n", encoding="utf-8",
        )

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn(
            json.dumps(unusual_name, ensure_ascii=True),
            result.stderr,
        )
        self.assertNotIn(unusual_name, result.stderr)

    def test_sample_is_bounded_deterministic_and_reports_omitted_paths(self):
        init_repository(self.repo_path)
        prd_name = "bounded-sample-prd"
        write_prd(self.repo_path, prd_name)
        for index in range(self.module.MAX_DIRTY_ROOT_SAMPLE_ENTRIES + 5):
            (self.repo_path / f"operator-{index:02d}.txt").write_text(
                "operator data\n", encoding="utf-8",
            )

        first = run_setup_workspace(self.repo_path, prd_name)
        second = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(first.returncode, 1, first.stdout)
        self.assertEqual(second.returncode, 1, second.stdout)
        self.assertEqual(first.stderr, second.stderr)
        sample_entries = [
            line for line in first.stderr.splitlines() if line.startswith("    ?? ")
        ]
        self.assertEqual(
            len(sample_entries), self.module.MAX_DIRTY_ROOT_SAMPLE_ENTRIES,
        )
        self.assertIn("5 additional path(s) omitted", first.stderr)

    def test_status_command_failure_stops_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "status-failure-prd"
        write_prd(self.repo_path, prd_name)
        original_run_git = self.module.run_git

        def failing_status(*args, **kwargs):
            if args[:1] == ("status",):
                return SimpleNamespace(
                    returncode=128,
                    stdout="",
                    stderr="simulated status failure\n",
                )
            return original_run_git(*args, **kwargs)

        stderr = io.StringIO()
        original_cwd = os.getcwd()
        os.chdir(self.repo_path)
        try:
            with mock.patch.object(self.module, "run_git", side_effect=failing_status):
                with mock.patch.object(self.module, "sync_main") as sync_main:
                    with mock.patch.object(
                        self.module, "prune_worktrees",
                    ) as prune_worktrees:
                        with mock.patch.object(
                            self.module, "create_or_reuse_worktree",
                        ) as create_worktree:
                            with mock.patch.object(
                                sys, "argv", ["setup-workspace.py", prd_name],
                            ):
                                with redirect_stderr(stderr):
                                    with self.assertRaises(SystemExit) as raised:
                                        self.module.main()
        finally:
            os.chdir(original_cwd)

        self.assertEqual(raised.exception.code, 1)
        self.assertIn("Root cleanliness check failed", stderr.getvalue())
        self.assertIn("git status --porcelain=v1 failed", stderr.getvalue())
        self.assertIn("simulated status failure", stderr.getvalue())
        sync_main.assert_not_called()
        prune_worktrees.assert_not_called()
        create_worktree.assert_not_called()

    def test_dirty_root_stops_before_mutation_capable_stages(self):
        init_repository(self.repo_path)
        prd_name = "dirty-stage-order-prd"
        write_prd(self.repo_path, prd_name)
        (self.repo_path / "README.md").write_text(
            "operator edit\n", encoding="utf-8",
        )

        stderr = io.StringIO()
        original_cwd = os.getcwd()
        os.chdir(self.repo_path)
        try:
            with mock.patch.object(self.module, "sync_main") as sync_main, \
                    mock.patch.object(
                        self.module, "prune_worktrees",
                    ) as prune_worktrees, \
                    mock.patch.object(
                        self.module, "create_or_reuse_worktree",
                    ) as create_worktree, \
                    mock.patch.object(
                        self.module, "copy_prd_files",
                    ) as copy_prd_files:
                with mock.patch.object(
                    sys, "argv", ["setup-workspace.py", prd_name],
                ):
                    with redirect_stderr(stderr):
                        with self.assertRaises(SystemExit) as raised:
                            self.module.main()
        finally:
            os.chdir(original_cwd)

        self.assertEqual(raised.exception.code, 1)
        self.assertIn("repository root is dirty", stderr.getvalue().lower())
        sync_main.assert_not_called()
        prune_worktrees.assert_not_called()
        create_worktree.assert_not_called()
        copy_prd_files.assert_not_called()


if __name__ == "__main__":
    unittest.main()
