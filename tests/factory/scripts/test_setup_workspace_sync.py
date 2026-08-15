#!/usr/bin/env python3
"""Regression tests for setup-workspace sync_main quiet-skip behavior."""

import contextlib
import io
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


def stash_list(repo_path):
    return git(["stash", "list", "--format=%H"], repo_path).stdout.splitlines()


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

    def test_sync_main_does_not_fast_forward_from_stale_origin_main_after_remote_deleted(self):
        local_repo = self.repo_path / "local"
        bare_remote = setup_repo_with_origin_main_ahead(local_repo, self.repo_path)

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

        git(["update-ref", "-d", "refs/heads/main"], bare_remote)
        plain_fetch = git(["fetch", "origin"], local_repo, check=False)
        self.assertEqual(plain_fetch.returncode, 0)
        self.assertTrue(self.module.origin_main_ref_exists(local_repo))

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

    def test_sync_main_stashes_and_restores_dirty_checked_out_main(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)
        git(["checkout", "main"], local_repo)

        dirty_file = local_repo / "f.txt"
        dirty_file.write_text("dirty content\n", encoding="utf-8")
        status_before = git(["status", "--porcelain"], local_repo).stdout
        head_before = git(["rev-parse", "HEAD"], local_repo).stdout.strip()
        branch_before = git(["branch", "--show-current"], local_repo).stdout.strip()
        local_main_sha_before = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        origin_main_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()
        self.assertNotEqual(local_main_sha_before, origin_main_sha)

        recorded, original_run_git = self.record_git_commands()
        try:
            self.module.sync_main(local_repo)
        finally:
            self.module.run_git = original_run_git

        self.assertEqual(
            git(["rev-parse", "refs/heads/main"], local_repo).stdout.strip(),
            origin_main_sha,
        )
        self.assertEqual(git(["rev-parse", "HEAD"], local_repo).stdout.strip(), origin_main_sha)
        self.assertEqual(
            git(["branch", "--show-current"], local_repo).stdout.strip(),
            branch_before,
        )
        self.assertEqual(git(["status", "--porcelain"], local_repo).stdout, status_before)
        self.assertEqual(
            dirty_file.read_text(encoding="utf-8"),
            "dirty content\n",
        )
        self.assertFalse(
            any(
                args[:3] == ("update-ref", "refs/heads/main", origin_main_sha)
                for args in recorded
            )
        )
        self.assertFalse(any(args[:2] == ("stash", "push") for args in recorded))
        self.assertTrue(any(args[:2] == ("stash", "apply") for args in recorded))
        self.assertFalse(any(args[:2] == ("stash", "drop") for args in recorded))
        self.assertTrue(any(args and args[0] == "commit-tree" for args in recorded))
        self.assertFalse(
            any("stash@{" in str(argument) for args in recorded for argument in args)
        )
        self.assertTrue(any(args[:2] == ("pull", "--ff-only") for args in recorded))
        self.assertEqual(head_before, local_main_sha_before)

    def test_restores_owned_snapshot_when_another_stash_is_created_before_restore(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)
        git(["checkout", "main"], local_repo)

        (local_repo / "pre-existing.txt").write_text("pre-existing\n", encoding="utf-8")
        git(
            ["stash", "push", "--include-untracked", "--message", "pre-existing"],
            local_repo,
        )
        stack_before = stash_list(local_repo)

        (local_repo / "README.md").write_text("owned unstaged\n", encoding="utf-8")
        (local_repo / "owned-staged.txt").write_text("owned staged\n", encoding="utf-8")
        git(["add", "owned-staged.txt"], local_repo)
        (local_repo / "owned-untracked.txt").write_text(
            "owned untracked\n",
            encoding="utf-8",
        )
        remote_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()

        original_restore = self.module.restore_stashed_changes
        interfering_stash = []

        def create_interfering_stash(repo_path, snapshot_id, scope_label):
            (repo_path / "interfering-only.txt").write_text(
                "foreign stash\n",
                encoding="utf-8",
            )
            git(
                [
                    "stash",
                    "push",
                    "--include-untracked",
                    "--message",
                    "interfering stash",
                ],
                repo_path,
            )
            interfering_stash.append(git(["rev-parse", "refs/stash"], repo_path).stdout.strip())
            return original_restore(repo_path, snapshot_id, scope_label)

        self.module.restore_stashed_changes = create_interfering_stash
        try:
            outcome = self.module.sync_checked_out_main_with_stash(local_repo, remote_sha)
        finally:
            self.module.restore_stashed_changes = original_restore

        self.assertIn("restored", outcome)
        self.assertEqual(len(interfering_stash), 1)
        self.assertEqual(stash_list(local_repo), [interfering_stash[0], *stack_before])
        self.assertEqual(
            git(["diff", "--cached", "--name-status"], local_repo).stdout,
            "A\towned-staged.txt\n",
        )
        self.assertEqual(
            git(["diff", "--name-status"], local_repo).stdout,
            "M\tREADME.md\n",
        )
        self.assertTrue((local_repo / "owned-untracked.txt").exists())
        self.assertFalse((local_repo / "interfering-only.txt").exists())

        restored_foreign = git(
            ["stash", "apply", interfering_stash[0]],
            local_repo,
        )
        self.assertEqual(restored_foreign.returncode, 0, restored_foreign.stderr)
        self.assertEqual(
            (local_repo / "interfering-only.txt").read_text(encoding="utf-8"),
            "foreign stash\n",
        )
        self.assertEqual(stash_list(local_repo), [interfering_stash[0], *stack_before])

    def test_setup_exits_nonzero_when_owned_snapshot_is_missing_before_restore(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)
        git(["checkout", "main"], local_repo)

        (local_repo / "pre-existing.txt").write_text(
            "pre-existing\n",
            encoding="utf-8",
        )
        git(
            [
                "stash",
                "push",
                "--include-untracked",
                "--message",
                "pre-existing",
            ],
            local_repo,
        )
        stack_before = stash_list(local_repo)

        tasks_dir = local_repo / "tasks" / "todo"
        tasks_dir.mkdir(parents=True)
        (local_repo / ".git" / "info" / "exclude").write_text(
            "tasks/\n",
            encoding="utf-8",
        )
        prd_name = "missing-snapshot-prd"
        (tasks_dir / f"{prd_name}.json").write_text(
            json.dumps({"branchName": prd_name}),
            encoding="utf-8",
        )

        (local_repo / "README.md").write_text("owned unstaged\n", encoding="utf-8")
        staged_file = local_repo / "owned-staged.txt"
        staged_file.write_text("owned staged\n", encoding="utf-8")
        git(["add", "owned-staged.txt"], local_repo)
        (local_repo / "owned-untracked.txt").write_text(
            "owned untracked\n",
            encoding="utf-8",
        )

        original_root = self.module.get_repo_root
        original_restore = self.module.restore_stashed_changes
        original_argv = self.module.sys.argv
        captured_snapshot = []
        interfering_stash = []

        def invalidate_snapshot(repo_path, snapshot_id, scope_label):
            captured_snapshot.append(snapshot_id)
            (repo_path / "interfering-only.txt").write_text(
                "foreign stash\n",
                encoding="utf-8",
            )
            git(
                [
                    "stash",
                    "push",
                    "--include-untracked",
                    "--message",
                    "interfering stash",
                ],
                repo_path,
            )
            interfering_stash.append(
                git(["rev-parse", "refs/stash"], repo_path).stdout.strip()
            )
            git(["prune", "--expire=now"], repo_path)
            snapshot_check = git(
                ["cat-file", "-e", f"{snapshot_id}^{{commit}}"],
                repo_path,
                check=False,
            )
            self.assertNotEqual(snapshot_check.returncode, 0)
            return original_restore(repo_path, snapshot_id, scope_label)

        self.module.get_repo_root = lambda: local_repo
        self.module.restore_stashed_changes = invalidate_snapshot
        self.module.sys.argv = ["setup-workspace.py", prd_name]
        stderr = io.StringIO()
        try:
            with contextlib.redirect_stderr(stderr):
                with self.assertRaises(SystemExit) as exit_context:
                    self.module.main()
        finally:
            self.module.get_repo_root = original_root
            self.module.restore_stashed_changes = original_restore
            self.module.sys.argv = original_argv

        self.assertEqual(exit_context.exception.code, 1)
        self.assertEqual(len(captured_snapshot), 1)
        self.assertEqual(len(interfering_stash), 1)
        snapshot_id = captured_snapshot[0]
        self.assertIn(
            f"Root sync failed: root main sync could not verify snapshot {snapshot_id}",
            stderr.getvalue(),
        )
        self.assertEqual(stash_list(local_repo), [interfering_stash[0], *stack_before])
        self.assertFalse((local_repo / "interfering-only.txt").exists())

        restored_foreign = git(
            ["stash", "apply", interfering_stash[0]],
            local_repo,
        )
        self.assertEqual(restored_foreign.returncode, 0, restored_foreign.stderr)
        self.assertEqual(
            (local_repo / "interfering-only.txt").read_text(encoding="utf-8"),
            "foreign stash\n",
        )
        self.assertEqual(stash_list(local_repo), [interfering_stash[0], *stack_before])

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

    def test_setup_workspace_creates_worktree_with_staged_root_changes(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)

        staged_file = local_repo / "staged.txt"
        staged_file.write_text("staged change\n", encoding="utf-8")
        git(["add", "staged.txt"], local_repo)

        prd_name = "staged-root-prd"
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
        self.assertIn("A  staged.txt", git(["status", "--porcelain"], local_repo).stdout)
        self.assertIn("?? dirty.txt", git(["status", "--porcelain"], local_repo).stdout)
        self.assertEqual(
            staged_file.read_text(encoding="utf-8"),
            "staged change\n",
        )
        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        self.assertTrue(worktree_path.exists())
        self.assertTrue((worktree_path / "prd.json").exists())
        self.assertIn("Root sync:", result.stderr)

    def test_setup_workspace_reports_stashed_sync_on_dirty_main_checkout(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)
        git(["checkout", "main"], local_repo)

        dirty_file = local_repo / "dirty-main.txt"
        dirty_file.write_text("dirty on main\n", encoding="utf-8")

        prd_name = "dirty-main-prd"
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
        self.assertIn("Root sync:", result.stderr)
        self.assertIn("stashed local changes", result.stderr.lower())
        self.assertIn("?? dirty-main.txt", git(["status", "--porcelain"], local_repo).stdout)
        self.assertEqual(
            git(["rev-parse", "refs/heads/main"], local_repo).stdout.strip(),
            git(["rev-parse", "refs/remotes/origin/main"], local_repo).stdout.strip(),
        )

    def test_restore_stashed_changes_falls_back_when_index_apply_conflicts(self):
        recorded = []
        original_run_git = self.module.run_git
        snapshot_id = "a" * 40

        def fake_run_git(*args, **kwargs):
            recorded.append(args)
            if args == ("cat-file", "-e", f"{snapshot_id}^{{commit}}"):
                return subprocess.CompletedProcess(
                    ["git", *args],
                    0,
                    stdout="",
                    stderr="",
                )
            if args == ("stash", "apply", "--index", snapshot_id):
                return subprocess.CompletedProcess(
                    ["git", *args],
                    1,
                    stdout="",
                    stderr=(
                        "error: conflicts in index. "
                        "Try without --index."
                    ),
                )
            if args == ("stash", "apply", snapshot_id):
                return subprocess.CompletedProcess(
                    ["git", *args],
                    0,
                    stdout="applied\n",
                    stderr="",
                )
            return original_run_git(*args, **kwargs)

        self.module.run_git = fake_run_git
        try:
            self.module.restore_stashed_changes(
                self.repo_path,
                snapshot_id,
                "root main",
            )
        finally:
            self.module.run_git = original_run_git

        self.assertEqual(recorded[0], ("cat-file", "-e", f"{snapshot_id}^{{commit}}"))
        self.assertEqual(recorded[1], ("stash", "apply", "--index", snapshot_id))
        self.assertEqual(recorded[2], ("stash", "apply", snapshot_id))


if __name__ == "__main__":
    unittest.main()
