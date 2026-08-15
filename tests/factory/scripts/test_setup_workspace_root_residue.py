#!/usr/bin/env python3
"""Disposable-repository regressions for root setup-workspace synchronization.

The scenarios retain the observable ordering that reproduced the incident:
one invocation owns a temporarily cleared checked-out ``main`` while another
invocation attempts to synchronize the same root.  The fixed behavior keeps
the second invocation outside that capture-through-restore cycle and updates
the checked-out index and worktree along with ``main``.
"""

import contextlib
import importlib.util
import json
import subprocess
import tempfile
import threading
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"


def load_setup_workspace_module():
    spec = importlib.util.spec_from_file_location(
        "setup_workspace_root_residue", SCRIPT_PATH,
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


def create_remote_repository(root, remote_ahead):
    """Create a local main checkout at P and optionally push merge commit M."""
    root.mkdir(parents=True, exist_ok=True)
    remote = root / "remote.git"
    remote.mkdir()
    git(["init", "--bare", "-b", "main"], remote)

    upstream = root / "upstream"
    upstream.mkdir()
    init_repository(upstream)
    git(["remote", "add", "origin", str(remote)], upstream)
    git(["push", "-u", "origin", "main"], upstream)

    (upstream / "merged.txt").write_text("latest merge\n", encoding="utf-8")
    git(["add", "merged.txt"], upstream)
    git(["commit", "-m", "latest merge"], upstream)
    merge_sha = git(["rev-parse", "HEAD"], upstream).stdout.strip()
    if remote_ahead:
        git(["push", "origin", "main"], upstream)

    local = root / "local"
    git(["clone", str(remote), local.name], root)
    if remote_ahead:
        git(["reset", "--hard", "HEAD^"], local)
        git(["fetch", "origin"], local)

    return local, upstream, merge_sha


def tree_report(module, repo_path):
    """Return the observable checkout state used by each reproduction."""
    private_refs = {}
    refs = git(
        [
            "for-each-ref",
            "--format=%(refname) %(objectname)",
            "refs/factory-snapshots/",
        ],
        repo_path,
    ).stdout
    for line in refs.splitlines():
        ref_name, object_id = line.split(maxsplit=1)
        private_refs[ref_name] = object_id

    return {
        "head": git(["rev-parse", "HEAD"], repo_path).stdout.strip(),
        "main": git(["rev-parse", "refs/heads/main"], repo_path).stdout.strip(),
        "porcelain": git(["status", "--porcelain=v1"], repo_path).stdout,
        "index_tree": git(["write-tree"], repo_path).stdout.strip(),
        "worktree_tree": module.working_tree_tree(repo_path),
        "head_tree": git(["rev-parse", "HEAD:"], repo_path).stdout.strip(),
        "ancestor_tree": git(["rev-parse", "HEAD^:"], repo_path).stdout.strip(),
        "private_refs": private_refs,
    }


def report_message(label, report):
    return f"{label}: {json.dumps(report, sort_keys=True)}"


class SetupWorkspaceRootResidueTest(unittest.TestCase):
    def setUp(self):
        self.module = load_setup_workspace_module()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_root_state_matrix_reports_current_and_ancestor_identity(self):
        """Characterize clean, dirty, staged, and stale-anchor root starts."""
        scenarios = (
            ("clean", None),
            ("untracked-only", "untracked"),
            ("modified-tracked", "modified"),
            ("staged", "staged"),
            ("stale-private-ref", "stale"),
        )

        for label, initial_state in scenarios:
            with self.subTest(start=label):
                scenario_root = self.root / label
                scenario_root.mkdir()
                local, _upstream, remote_sha = create_remote_repository(
                    scenario_root, remote_ahead=True,
                )
                expected_private_refs = {}

                if initial_state == "untracked":
                    (local / "operator-untracked.txt").write_text(
                        "operator data\n", encoding="utf-8",
                    )
                elif initial_state == "modified":
                    (local / "README.md").write_text(
                        "operator edit\n", encoding="utf-8",
                    )
                elif initial_state == "staged":
                    (local / "operator-staged.txt").write_text(
                        "operator staged\n", encoding="utf-8",
                    )
                    git(["add", "operator-staged.txt"], local)
                elif initial_state == "stale":
                    (local / "stale-only.txt").write_text(
                        "recoverable stale snapshot\n", encoding="utf-8",
                    )
                    stale_snapshot = self.module.stash_local_changes(
                        local, "stale private snapshot",
                    )
                    expected_private_refs = {
                        self.module.snapshot_ref_name(stale_snapshot): stale_snapshot,
                    }

                outcome = self.module.sync_main(local)
                report = tree_report(self.module, local)
                self.assertEqual(
                    report["head"], remote_sha,
                    report_message(f"{label} outcome={outcome}", report),
                )
                self.assertEqual(
                    report["main"], remote_sha,
                    report_message(f"{label} outcome={outcome}", report),
                )
                self.assertEqual(
                    report["private_refs"], expected_private_refs,
                    report_message(f"{label} outcome={outcome}", report),
                )

                if label in {"clean", "stale-private-ref"}:
                    self.assertEqual(
                        report["index_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["worktree_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["porcelain"], "", report_message(label, report),
                    )
                    if label == "stale-private-ref":
                        self.assertNotIn(
                            "stale-only.txt", report["porcelain"],
                        )
                        self.assertFalse((local / "stale-only.txt").exists())
                elif label == "untracked-only":
                    self.assertEqual(
                        report["index_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["worktree_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["porcelain"], "?? operator-untracked.txt\n",
                        report_message(label, report),
                    )
                elif label == "modified-tracked":
                    self.assertEqual(
                        report["index_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertNotEqual(
                        report["worktree_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["porcelain"], " M README.md\n",
                        report_message(label, report),
                    )
                elif label == "staged":
                    self.assertNotEqual(
                        report["index_tree"], report["head_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["worktree_tree"], report["index_tree"],
                        report_message(label, report),
                    )
                    self.assertEqual(
                        report["porcelain"], "A  operator-staged.txt\n",
                        report_message(label, report),
                    )

    def test_remote_advance_between_snapshot_capture_and_restore_is_reported(self):
        """Prove snapshot restore across a remote change in a throwaway repo."""
        local, upstream, merge_sha = create_remote_repository(
            self.root / "remote-advance", remote_ahead=False,
        )
        (local / "README.md").write_text("operator edit\n", encoding="utf-8")
        (local / "operator-staged.txt").write_text(
            "operator staged\n", encoding="utf-8",
        )
        git(["add", "operator-staged.txt"], local)
        (local / "operator-untracked.txt").write_text(
            "operator data\n", encoding="utf-8",
        )

        original_run_git = self.module.run_git
        pushed = []

        def push_remote_before_restore(*args, **kwargs):
            if args[:2] == ("pull", "--ff-only") and not pushed:
                git(["push", "origin", "main"], upstream)
                pushed.append(True)
            return original_run_git(*args, **kwargs)

        self.module.run_git = push_remote_before_restore
        try:
            outcome = self.module.sync_checked_out_main_with_stash(local, merge_sha)
        finally:
            self.module.run_git = original_run_git

        report = tree_report(self.module, local)
        self.assertTrue(pushed, report_message(outcome, report))
        self.assertEqual(report["head"], merge_sha, report_message(outcome, report))
        self.assertEqual(report["main"], merge_sha, report_message(outcome, report))
        self.assertEqual(
            report["porcelain"],
            " M README.md\nA  operator-staged.txt\n?? operator-untracked.txt\n",
            report_message(outcome, report),
        )
        self.assertEqual(report["private_refs"], {}, report_message(outcome, report))
        self.assertNotEqual(report["head_tree"], report["index_tree"])
        self.assertNotEqual(report["index_tree"], report["worktree_tree"])

    def test_guard_rejects_ancestor_equivalent_index_and_worktree_without_repairing(self):
        """Reject a current HEAD whose tracked checkout exactly equals its parent."""
        local, _upstream, remote_sha = create_remote_repository(
            self.root / "guard-residue", remote_ahead=True,
        )
        git(["reset", "--hard", remote_sha], local)
        git(["read-tree", "--reset", "-u", "HEAD^"], local)

        with self.assertRaisesRegex(
            RuntimeError,
            r"root main post-sync ancestor-residue guard rejected tracked local state",
        ) as raised:
            self.module.verify_no_ancestor_residue(local)

        report = tree_report(self.module, local)
        self.assertEqual(report["head"], remote_sha, report_message("guard", report))
        self.assertEqual(report["main"], remote_sha, report_message("guard", report))
        self.assertEqual(
            report["index_tree"], report["ancestor_tree"],
            report_message("guard", report),
        )
        self.assertEqual(
            report["worktree_tree"], report["ancestor_tree"],
            report_message("guard", report),
        )
        self.assertIn("merged.txt", report["porcelain"])
        self.assertIn("merged.txt", str(raised.exception))
        self.assertIn(remote_sha, str(raised.exception))
        self.assertIn(report["head"], str(raised.exception))
        self.assertNotIn("latest merge", str(raised.exception))

    def test_guard_accepts_current_content_and_legitimate_non_ancestor_edit(self):
        """Current content and an ordinary edit do not look like old code."""
        local, _upstream, remote_sha = create_remote_repository(
            self.root / "guard-legitimate", remote_ahead=True,
        )
        git(["reset", "--hard", remote_sha], local)

        self.module.verify_no_ancestor_residue(local)

        (local / "README.md").write_text("operator edit\n", encoding="utf-8")
        self.module.verify_no_ancestor_residue(local)

        report = tree_report(self.module, local)
        self.assertEqual(report["head"], remote_sha, report_message("legitimate", report))
        self.assertEqual(report["index_tree"], report["head_tree"])
        self.assertNotEqual(report["worktree_tree"], report["head_tree"])
        self.assertEqual(report["porcelain"], " M README.md\n")

    def test_sync_guard_preserves_snapshot_anchor_when_restore_leaves_ancestor_residue(self):
        """Guard failure keeps the captured snapshot available for recovery."""
        local, _upstream, remote_sha = create_remote_repository(
            self.root / "guard-recovery", remote_ahead=True,
        )
        (local / "operator-untracked.txt").write_text(
            "operator data\n", encoding="utf-8",
        )

        original_restore = self.module.restore_stashed_changes
        captured_snapshot = []

        def induce_residue(repo_path, snapshot_id, scope_label):
            original_restore(repo_path, snapshot_id, scope_label)
            captured_snapshot.append(snapshot_id)
            git(["read-tree", "--reset", "-u", "HEAD^"], repo_path)

        self.module.restore_stashed_changes = induce_residue
        try:
            with self.assertRaisesRegex(
                RuntimeError,
                r"recovery snapshot preserved at refs/factory-snapshots/",
            ) as raised:
                self.module.sync_main(local)
        finally:
            self.module.restore_stashed_changes = original_restore

        self.assertEqual(len(captured_snapshot), 1)
        snapshot_id = captured_snapshot[0]
        report = tree_report(self.module, local)
        self.assertEqual(report["head"], remote_sha, report_message("recovery", report))
        self.assertEqual(report["main"], remote_sha, report_message("recovery", report))
        self.assertEqual(
            report["private_refs"],
            {self.module.snapshot_ref_name(snapshot_id): snapshot_id},
            report_message("recovery", report),
        )
        self.assertIn("merged.txt", report["porcelain"])
        self.assertIn(self.module.snapshot_ref_name(snapshot_id), str(raised.exception))

    def test_sync_guard_checks_residue_when_main_is_already_current(self):
        """A previous silent residue is rejected even when no fast-forward is needed."""
        local, _upstream, remote_sha = create_remote_repository(
            self.root / "guard-current", remote_ahead=True,
        )
        git(["reset", "--hard", remote_sha], local)
        git(["read-tree", "--reset", "-u", "HEAD^"], local)

        with self.assertRaisesRegex(
            RuntimeError,
            r"no root snapshot was captured for automatic recovery",
        ):
            self.module.sync_main(local)

        report = tree_report(self.module, local)
        self.assertEqual(report["head"], remote_sha, report_message("current", report))
        self.assertEqual(report["main"], remote_sha, report_message("current", report))
        self.assertIn("merged.txt", report["porcelain"])
        self.assertEqual(report["private_refs"], {})

    def test_setup_stops_before_lane_creation_on_ancestor_residue(self):
        """The integrated guard fails before the requested lane is prepared."""
        local, _upstream, remote_sha = create_remote_repository(
            self.root / "guard-setup", remote_ahead=True,
        )
        git(["reset", "--hard", remote_sha], local)
        git(["read-tree", "--reset", "-u", "HEAD^"], local)

        prd_name = "ancestor-residue-prd"
        tasks_dir = local / "tasks" / "todo"
        tasks_dir.mkdir(parents=True)
        (tasks_dir / f"{prd_name}.json").write_text(
            json.dumps({"branchName": prd_name}),
            encoding="utf-8",
        )

        result = subprocess.run(
            ["python3", str(SCRIPT_PATH), prd_name],
            cwd=local,
            capture_output=True,
            text=True,
            check=False,
        )

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn("Root sync failed:", result.stderr)
        self.assertIn("ancestor-residue guard", result.stderr)
        self.assertIn("merged.txt", result.stderr)
        self.assertFalse(
            (local / ".claude" / "worktrees" / prd_name).exists(),
            result.stderr,
        )
        report = tree_report(self.module, local)
        self.assertEqual(report["head"], remote_sha, report_message("setup", report))
        self.assertEqual(report["main"], remote_sha, report_message("setup", report))
        self.assertIn("merged.txt", report["porcelain"])

    def test_controlled_overlapping_syncs_serialize_root_sync_and_preserve_state(self):
        """A competing invocation waits for the owner's full restore cycle."""
        local, _upstream, remote_sha = create_remote_repository(
            self.root / "overlap", remote_ahead=True,
        )
        (local / "overlap-untracked.txt").write_text(
            "operator data\n", encoding="utf-8",
        )

        snapshot_captured = threading.Event()
        competitor_attempted = threading.Event()
        competitor_acquired = threading.Event()
        release_owner = threading.Event()
        owner_result = []
        competitor_result = []
        original_run_git = self.module.run_git
        original_root_sync_lock = self.module.root_sync_lock

        def coordinate_lock(repo_path):
            lock_context = original_root_sync_lock(repo_path)
            if threading.current_thread().name != "root-sync-competitor":
                return lock_context

            @contextlib.contextmanager
            def instrumented_lock():
                competitor_attempted.set()
                with lock_context:
                    competitor_acquired.set()
                    yield

            return instrumented_lock()

        def coordinate_owner(*args, **kwargs):
            if (
                threading.current_thread().name == "root-sync-owner"
                and args[:2] == ("pull", "--ff-only")
            ):
                snapshot_captured.set()
                # These bounded waits are only deadlock guards.  The events,
                # not elapsed time, determine the invocation ordering.
                if not competitor_attempted.wait(10):
                    raise RuntimeError("competitor did not attempt root sync")
                if competitor_acquired.is_set():
                    raise RuntimeError("competitor acquired root sync too early")
                if not release_owner.wait(10):
                    raise RuntimeError("owner release was not signaled")
            return original_run_git(*args, **kwargs)

        self.module.run_git = coordinate_owner
        self.module.root_sync_lock = coordinate_lock

        def owner_sync():
            try:
                owner_result.append(self.module.sync_main(local))
            except BaseException as error:  # report thread failures in test
                owner_result.append(error)

        def competitor_sync():
            try:
                competitor_result.append(self.module.sync_main(local))
            except BaseException as error:  # report thread failures in test
                competitor_result.append(error)

        owner = threading.Thread(target=owner_sync, name="root-sync-owner")
        competitor = threading.Thread(
            target=competitor_sync, name="root-sync-competitor",
        )
        owner.start()
        try:
            self.assertTrue(
                snapshot_captured.wait(10),
                "owner did not reach the capture/restore boundary",
            )
            competitor.start()
            self.assertTrue(
                competitor_attempted.wait(10),
                "competitor did not attempt to acquire root sync",
            )
            self.assertFalse(competitor_acquired.is_set())
            release_owner.set()
        finally:
            owner.join(10)
            competitor.join(10)
            self.module.run_git = original_run_git
            self.module.root_sync_lock = original_root_sync_lock

        self.assertFalse(owner.is_alive(), "owner sync did not finish")
        self.assertFalse(competitor.is_alive(), "competitor sync did not finish")
        self.assertEqual(len(owner_result), 1)
        if isinstance(owner_result[0], BaseException):
            raise owner_result[0]
        self.assertEqual(len(competitor_result), 1)
        if isinstance(competitor_result[0], BaseException):
            raise competitor_result[0]

        final_report = tree_report(self.module, local)
        self.assertEqual(final_report["head"], remote_sha, report_message("final", final_report))
        self.assertEqual(final_report["main"], remote_sha, report_message("final", final_report))
        self.assertEqual(
            final_report["index_tree"], final_report["head_tree"],
            report_message("final", final_report),
        )
        self.assertEqual(
            final_report["porcelain"],
            "?? overlap-untracked.txt\n",
            report_message("final", final_report),
        )
        self.assertEqual(final_report["private_refs"], {})


if __name__ == "__main__":
    unittest.main()
