#!/usr/bin/env python3
"""Regression tests for setup-workspace worktree setup after safe sync."""

import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from preflight_test_support import write_packet


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"

EXPECTED_RESULT_KEYS = {
    "status",
    "worktree",
    "branch",
    "prd_path",
    "prd_md_path",
    "standing_rules_path",
    "reused",
    "preflight",
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
    return write_packet(repo_path, prd_name, include_md=include_md)


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
    (local_repo / ".git" / "info" / "exclude").write_text(
        "tasks/todo/\n.claude/\n",
        encoding="utf-8",
    )
    git(["fetch", "origin"], local_repo)

    return bare_remote


def setup_repo_with_local_main_ahead(local_repo, repo_root, ahead_commits=1):
    """Create a bare remote; local main carries unpushed commits ahead of it."""
    bare_remote = repo_root / "remote.git"
    bare_remote.mkdir()
    git(["init", "--bare", "-b", "main"], bare_remote)

    local_repo.mkdir()
    init_local_repo(local_repo)
    git(["remote", "add", "origin", str(bare_remote)], local_repo)
    git(["push", "-u", "origin", "main"], local_repo)

    for index in range(ahead_commits):
        ahead_file = local_repo / f"unpushed-{index}.txt"
        ahead_file.write_text("local main is ahead\n", encoding="utf-8")
        git(["add", ahead_file.name], local_repo)
        git(["commit", "-m", f"unpushed local commit {index}"], local_repo)

    git(["fetch", "origin"], local_repo)
    return bare_remote


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

    def test_unrelated_worktree_add_failure_is_not_treated_as_reuse(self):
        init_local_repo(self.repo_path)
        branch = "unrelated-worktree-failure"
        worktree_path = self.repo_path / "destination"
        worktree_path.mkdir()
        (worktree_path / ".git").write_text("gitdir: invalid\n", encoding="utf-8")
        marker = worktree_path / "operator-data.txt"
        marker.write_text("preserve\n", encoding="utf-8")
        expected_head = git(["rev-parse", "HEAD"], self.repo_path).stdout.strip()

        original_run_git = self.module.run_git

        def fail_worktree_add(*args, **kwargs):
            if args[:2] == ("worktree", "add"):
                raise RuntimeError(
                    "git worktree add failed (exit 128): permission denied"
                )
            return original_run_git(*args, **kwargs)

        with mock.patch.object(self.module, "run_git", side_effect=fail_worktree_add):
            with self.assertRaisesRegex(RuntimeError, "permission denied"):
                self.module.add_worktree_or_reuse(
                    self.repo_path,
                    worktree_path,
                    branch,
                    expected_head,
                    str(worktree_path),
                    branch,
                )

        self.assertEqual(marker.read_text(encoding="utf-8"), "preserve\n")

    def test_collision_reuse_requires_registered_requested_branch(self):
        init_local_repo(self.repo_path)
        other_branch = "other-destination-branch"
        worktree_path = self.repo_path / "destination"
        git(
            ["worktree", "add", "-b", other_branch, str(worktree_path), "main"],
            self.repo_path,
        )
        expected_head = git(["rev-parse", "main"], self.repo_path).stdout.strip()
        requested_branch = "requested-destination-branch"

        original_run_git = self.module.run_git

        def collide_once(*args, **kwargs):
            if args[:2] == ("worktree", "add"):
                raise RuntimeError(
                    f"git worktree add {worktree_path} failed (exit 128): "
                    f"'{requested_branch}' is already checked out at "
                    f"'{worktree_path}'"
                )
            return original_run_git(*args, **kwargs)

        with mock.patch.object(self.module, "run_git", side_effect=collide_once):
            with self.assertRaisesRegex(RuntimeError, "expected"):
                self.module.add_worktree_or_reuse(
                    self.repo_path,
                    worktree_path,
                    requested_branch,
                    expected_head,
                    str(worktree_path),
                    requested_branch,
                )

    def test_collision_reuse_requires_git_registration(self):
        init_local_repo(self.repo_path)
        branch = "unregistered-destination-branch"
        worktree_path = self.repo_path / "unregistered-destination"
        worktree_path.mkdir()
        (worktree_path / ".git").write_text("gitdir: invalid\n", encoding="utf-8")
        (worktree_path / "marker.txt").write_text("preserve\n", encoding="utf-8")
        expected_head = git(["rev-parse", "HEAD"], self.repo_path).stdout.strip()

        original_run_git = self.module.run_git

        def collide_once(*args, **kwargs):
            if args[:2] == ("worktree", "add"):
                raise RuntimeError(
                    f"git worktree add '{worktree_path}' failed (exit 128): "
                    f"'{worktree_path}' already exists"
                )
            return original_run_git(*args, **kwargs)

        with mock.patch.object(self.module, "run_git", side_effect=collide_once):
            with self.assertRaisesRegex(RuntimeError, "not Git-registered"):
                self.module.add_worktree_or_reuse(
                    self.repo_path,
                    worktree_path,
                    branch,
                    expected_head,
                    str(worktree_path),
                    branch,
                )

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

    def test_new_worktree_branches_from_origin_main_when_local_main_is_ahead(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_local_main_ahead(local_repo, self.repo_path, ahead_commits=2)

        origin_main_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()
        local_main_sha = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertNotEqual(local_main_sha, origin_main_sha)

        prd_name = "local-main-ahead-prd"
        packet_path, _ = write_prd(local_repo, prd_name)
        packet = json.loads(packet_path.read_text(encoding="utf-8"))
        packet["preflight"]["intendedMainline"]["commit"] = origin_main_sha
        packet_path.write_text(json.dumps(packet), encoding="utf-8")

        result = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)

        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        self.assertTrue(worktree_path.exists())

        worktree_head = git(["rev-parse", "HEAD"], worktree_path).stdout.strip()
        self.assertEqual(worktree_head, origin_main_sha)
        self.assertNotEqual(worktree_head, local_main_sha)
        for index in range(2):
            self.assertFalse((worktree_path / f"unpushed-{index}.txt").exists())
        ancestor_check = git(
            ["merge-base", "--is-ancestor", local_main_sha, worktree_head],
            worktree_path,
            check=False,
        )
        self.assertNotEqual(ancestor_check.returncode, 0)
        self.assertEqual(
            git(
                ["merge-base", "HEAD", "refs/remotes/origin/main"],
                worktree_path,
            ).stdout.strip(),
            origin_main_sha,
        )
        self.assertEqual(
            git(["rev-parse", f"refs/heads/{prd_name}"], local_repo).stdout.strip(),
            origin_main_sha,
        )

        # Local main must be untouched: the ahead commits stay on it.
        self.assertEqual(
            git(["rev-parse", "refs/heads/main"], local_repo).stdout.strip(),
            local_main_sha,
        )

    def test_resolve_worktree_start_point_falls_back_to_main_without_origin(self):
        init_local_repo(self.repo_path)
        self.assertEqual(
            self.module.resolve_worktree_start_point(None),
            "main",
        )

    def test_fetch_failure_with_stale_origin_main_uses_local_main(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_local_main_ahead(local_repo, self.repo_path, ahead_commits=2)

        stale_origin_main_sha = git(
            ["rev-parse", "refs/remotes/origin/main"],
            local_repo,
        ).stdout.strip()
        local_main_sha = git(
            ["rev-parse", "refs/heads/main"],
            local_repo,
        ).stdout.strip()
        self.assertNotEqual(local_main_sha, stale_origin_main_sha)

        missing_remote = self.repo_path / "missing-remote.git"
        git(["remote", "set-url", "origin", str(missing_remote)], local_repo)

        prd_name = "fetch-failure-stale-origin-prd"
        write_prd(local_repo, prd_name)

        result = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)

        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        worktree_head = git(["rev-parse", "HEAD"], worktree_path).stdout.strip()
        self.assertEqual(worktree_head, local_main_sha)
        self.assertNotEqual(worktree_head, stale_origin_main_sha)
        self.assertEqual(
            git(["rev-parse", f"refs/heads/{prd_name}"], local_repo).stdout.strip(),
            local_main_sha,
        )
        for index in range(2):
            self.assertTrue((worktree_path / f"unpushed-{index}.txt").exists())
        self.assertIn("fetch failed", result.stderr.lower())

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

    def test_reuses_valid_worktree_when_root_has_staged_changes(self):
        local_repo = self.repo_path / "local"
        setup_repo_with_origin_main_ahead(local_repo, self.repo_path)

        prd_name = "reuse-dirty-root-prd"
        write_prd(local_repo, prd_name)

        first = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        first_payload = json.loads(first.stdout)
        marker = Path(first_payload["worktree"]) / "reuse-marker.txt"
        marker.write_text("keep me\n", encoding="utf-8")

        staged_file = local_repo / "staged.txt"
        staged_file.write_text("staged change\n", encoding="utf-8")
        git(["add", "staged.txt"], local_repo)

        second = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertTrue(json.loads(second.stdout)["reused"])
        self.assertTrue(marker.exists())
        self.assertIn("A  staged.txt", git(["status", "--porcelain"], local_repo).stdout)

    def test_reused_worktree_stashes_local_changes_before_syncing_upstream(self):
        bare_remote = self.repo_path / "remote.git"
        bare_remote.mkdir()
        git(["init", "--bare", "-b", "main"], bare_remote)

        upstream = self.repo_path / "upstream"
        upstream.mkdir()
        init_local_repo(upstream)
        git(["remote", "add", "origin", str(bare_remote)], upstream)
        git(["push", "-u", "origin", "main"], upstream)

        local_repo = self.repo_path / "local"
        git(["clone", str(bare_remote), str(local_repo.name)], self.repo_path)

        prd_name = "dirty-worktree-sync-prd"
        write_prd(local_repo, prd_name)
        git(["checkout", "-b", prd_name], local_repo)
        (local_repo / "seed.txt").write_text("seed\n", encoding="utf-8")
        git(["add", "seed.txt"], local_repo)
        git(["commit", "-m", "seed branch"], local_repo)
        git(["push", "-u", "origin", prd_name], local_repo)
        git(["checkout", "main"], local_repo)

        first = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        worktree_path = Path(json.loads(first.stdout)["worktree"])

        dirty_file = worktree_path / "local-dirty.txt"
        dirty_file.write_text("keep me dirty\n", encoding="utf-8")

        git(["fetch", "origin"], upstream)
        git(["checkout", "-B", prd_name, f"origin/{prd_name}"], upstream)
        (upstream / "remote-only.txt").write_text("remote advance\n", encoding="utf-8")
        git(["add", "remote-only.txt"], upstream)
        git(["commit", "-m", "advance remote branch"], upstream)
        git(["push", "origin", prd_name], upstream)
        git(["fetch", "origin"], local_repo)

        second = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertIn("stashed local changes", second.stderr.lower())
        self.assertIn("?? local-dirty.txt", git(["status", "--porcelain"], worktree_path).stdout)
        self.assertEqual(
            dirty_file.read_text(encoding="utf-8"),
            "keep me dirty\n",
        )
        self.assertTrue((worktree_path / "remote-only.txt").exists())
        self.assertEqual(
            git(["rev-parse", "HEAD"], worktree_path).stdout.strip(),
            git(["rev-parse", f"origin/{prd_name}"], worktree_path).stdout.strip(),
        )

    def test_creates_worktree_when_branch_has_no_upstream(self):
        init_local_repo(self.repo_path)
        prd_name = "no-upstream-create-prd"
        write_prd(self.repo_path, prd_name)

        result = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)

        payload = json.loads(result.stdout)
        worktree_path = Path(payload["worktree"])
        self.assertTrue(worktree_path.exists())
        self.assertIsNone(self.module.branch_upstream_ref(worktree_path, prd_name))

    def test_reuses_worktree_when_branch_has_no_upstream(self):
        init_local_repo(self.repo_path)
        prd_name = "no-upstream-reuse-prd"
        write_prd(self.repo_path, prd_name)

        first = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        first_payload = json.loads(first.stdout)
        worktree_path = Path(first_payload["worktree"])
        self.assertIsNone(self.module.branch_upstream_ref(worktree_path, prd_name))
        marker = worktree_path / "reuse-marker.txt"
        marker.write_text("keep me\n", encoding="utf-8")

        second = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(second.returncode, 0, second.stderr)
        second_payload = json.loads(second.stdout)
        self.assertTrue(second_payload["reused"])
        self.assertTrue(marker.exists())
        self.assertIn("no upstream", second.stderr.lower())

    def test_keeps_local_worktree_state_when_branch_diverged_from_upstream(self):
        bare_remote = self.repo_path / "remote.git"
        bare_remote.mkdir()
        git(["init", "--bare", "-b", "main"], bare_remote)

        upstream = self.repo_path / "upstream"
        upstream.mkdir()
        init_local_repo(upstream)
        git(["remote", "add", "origin", str(bare_remote)], upstream)
        git(["push", "-u", "origin", "main"], upstream)

        local_repo = self.repo_path / "local"
        git(["clone", str(bare_remote), str(local_repo.name)], self.repo_path)

        prd_name = "unsafe-branch-update-prd"
        write_prd(local_repo, prd_name)
        git(["checkout", "-b", prd_name], local_repo)
        (local_repo / "seed.txt").write_text("seed\n", encoding="utf-8")
        git(["add", "seed.txt"], local_repo)
        git(["commit", "-m", "seed branch"], local_repo)
        git(["push", "-u", "origin", prd_name], local_repo)
        git(["checkout", "main"], local_repo)

        first = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(first.returncode, 0, first.stderr)
        worktree_path = Path(json.loads(first.stdout)["worktree"])

        (worktree_path / "local-only.txt").write_text("local commit\n", encoding="utf-8")
        git(["add", "local-only.txt"], worktree_path)
        git(["commit", "-m", "diverge locally"], worktree_path)

        git(["fetch", "origin"], upstream)
        git(["checkout", "-B", prd_name, f"origin/{prd_name}"], upstream)
        (upstream / "remote-only.txt").write_text("remote commit\n", encoding="utf-8")
        git(["add", "remote-only.txt"], upstream)
        git(["commit", "-m", "diverge on remote"], upstream)
        git(["push", "origin", prd_name], upstream)
        git(["fetch", "origin"], local_repo)

        local_tip = git(["rev-parse", "HEAD"], worktree_path).stdout.strip()
        dirty_file = worktree_path / "diverged-dirty.txt"
        dirty_file.write_text("keep me dirty\n", encoding="utf-8")

        second = run_setup_workspace(local_repo, prd_name)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertIn(
            "skipped (local branch diverged from upstream", second.stderr,
        )
        self.assertIn("stashed local changes", second.stderr.lower())
        self.assertNotIn("worktree branch update failed", second.stderr.lower())
        second_payload = json.loads(second.stdout)
        self.assertTrue(second_payload["reused"])

        # The unpushed local commits survive: HEAD still equals the pre-sync
        # local tip, and the worktree was not reset to the remote history.
        self.assertEqual(
            git(["rev-parse", "HEAD"], worktree_path).stdout.strip(),
            local_tip,
        )
        self.assertTrue((worktree_path / "local-only.txt").exists())
        self.assertFalse((worktree_path / "remote-only.txt").exists())

        # Stashed local changes are restored by the finally block.
        self.assertEqual(
            dirty_file.read_text(encoding="utf-8"),
            "keep me dirty\n",
        )
        self.assertIn(
            "?? diverged-dirty.txt",
            git(["status", "--porcelain"], worktree_path).stdout,
        )

        # Nothing is lost remotely either: the diverged remote commits remain
        # reachable on the fetched upstream ref for push-time reconciliation.
        remote_tip = git(
            ["rev-parse", f"origin/{prd_name}"], worktree_path,
        ).stdout.strip()
        self.assertNotEqual(remote_tip, local_tip)


if __name__ == "__main__":
    unittest.main()
