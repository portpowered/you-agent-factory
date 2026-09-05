#!/usr/bin/env python3
"""Regression tests for safe setup from a dirty repository root."""

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

from preflight_test_support import write_packet


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
    write_packet(repo_path, prd_name)


def create_remote_operator(base_path):
    """Create an operator clone whose remote can advance independently."""
    remote_path = base_path / "remote.git"
    remote_path.mkdir()
    git(["init", "--bare", "-b", "main"], remote_path)

    upstream_path = base_path / "upstream"
    upstream_path.mkdir()
    init_repository(upstream_path)
    git(["remote", "add", "origin", str(remote_path)], upstream_path)
    git(["push", "-u", "origin", "main"], upstream_path)

    operator_path = base_path / "operator"
    git(["clone", str(remote_path), operator_path.name], base_path)
    git(
        ["config", "user.email", "setup-workspace-test@example.com"],
        operator_path,
    )
    git(
        ["config", "user.name", "Setup Workspace Test"],
        operator_path,
    )
    (operator_path / ".git" / "info" / "exclude").write_text(
        "tasks/todo/\n.claude/\n",
        encoding="utf-8",
    )
    return operator_path, upstream_path


def create_residual_refusal_repository(base_path):
    """Create a root with stale origin/main but no safe local or remote main."""
    remote_path = base_path / "residual-remote.git"
    remote_path.mkdir()
    git(["init", "--bare", "-b", "main"], remote_path)

    upstream_path = base_path / "residual-upstream"
    upstream_path.mkdir()
    init_repository(upstream_path)
    git(["remote", "add", "origin", str(remote_path)], upstream_path)
    git(["push", "-u", "origin", "main"], upstream_path)

    operator_path = base_path / "residual-operator"
    git(["clone", str(remote_path), operator_path.name], base_path)
    (operator_path / ".git" / "info" / "exclude").write_text(
        "tasks/todo/\n.claude/\n",
        encoding="utf-8",
    )

    git(["checkout", "--detach", "HEAD"], operator_path)
    git(["update-ref", "-d", "refs/heads/main"], operator_path)
    git(["update-ref", "-d", "refs/heads/main"], remote_path)
    # A normal fetch retains the stale remote-tracking ref without --prune.
    git(["fetch", "origin"], operator_path)
    return operator_path


def add_sibling_worktree(repo_path, name):
    worktree_path = repo_path / ".claude" / "worktrees" / name
    worktree_path.parent.mkdir(parents=True, exist_ok=True)
    git(
        [
            "worktree",
            "add",
            "-b",
            f"{name}-branch",
            str(worktree_path),
            "refs/remotes/origin/main",
        ],
        repo_path,
    )
    return worktree_path


def run_setup_workspace(repo_path, prd_name, env=None):
    environment = os.environ.copy()
    if env:
        environment.update(env)
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), prd_name],
        cwd=repo_path,
        capture_output=True,
        text=True,
        check=False,
        env=environment,
    )


class SetupWorkspaceDirtyRootTest(unittest.TestCase):
    def setUp(self):
        self.module = load_setup_workspace_module()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name)

    def tearDown(self):
        self.temp_dir.cleanup()

    def run_safe_case(self, mutate_root):
        init_repository(self.repo_path)
        prd_name = "dirty-root-prd"
        write_prd(self.repo_path, prd_name)
        mutate_root()

        head_before = git(["rev-parse", "HEAD"], self.repo_path).stdout.strip()
        branch_before = git(
            ["branch", "--show-current"], self.repo_path,
        ).stdout.strip()
        main_before = git(
            ["rev-parse", "refs/heads/main"], self.repo_path,
        ).stdout.strip()
        status_before = git(["status", "--porcelain=v1"], self.repo_path).stdout
        index_before = git(["ls-files", "--stage"], self.repo_path).stdout
        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "ready")
        self.assertTrue(Path(payload["worktree"]).exists())
        self.assertEqual(
            git(["rev-parse", "HEAD"], self.repo_path).stdout.strip(),
            head_before,
        )
        self.assertEqual(
            git(["status", "--porcelain=v1"], self.repo_path).stdout,
            status_before,
        )
        self.assertEqual(
            git(["ls-files", "--stage"], self.repo_path).stdout,
            index_before,
        )
        self.assertEqual(
            git(["branch", "--show-current"], self.repo_path).stdout.strip(),
            branch_before,
        )
        self.assertEqual(
            git(["rev-parse", "refs/heads/main"], self.repo_path).stdout.strip(),
            main_before,
        )
        return result

    def test_allows_unstaged_tracked_change_without_root_mutation(self):
        def mutate():
            (self.repo_path / "README.md").write_text(
                "operator edit\n", encoding="utf-8",
            )

        self.run_safe_case(mutate)

    def test_allows_staged_tracked_change_without_root_mutation(self):
        def mutate():
            staged_path = self.repo_path / "staged.txt"
            staged_path.write_text("staged\n", encoding="utf-8")
            git(["add", staged_path.name], self.repo_path)

        self.run_safe_case(mutate)

    def test_all_dirty_root_change_types_remain_eligible(self):
        def mutate():
            (self.repo_path / "README.md").write_text(
                "operator edit\n", encoding="utf-8",
            )
            (self.repo_path / "untracked.txt").write_text(
                "operator data\n", encoding="utf-8",
            )

        self.run_safe_case(mutate)

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

    def test_unusual_path_does_not_block_safe_setup(self):
        init_repository(self.repo_path)
        prd_name = "unusual-path-prd"
        write_prd(self.repo_path, prd_name)
        unusual_name = "operator [path] café.txt"
        (self.repo_path / unusual_name).write_text(
            "operator data\n", encoding="utf-8",
        )

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.repo_path / unusual_name).exists())

    def test_many_dirty_paths_do_not_block_safe_setup(self):
        init_repository(self.repo_path)
        prd_name = "bounded-sample-prd"
        write_prd(self.repo_path, prd_name)
        for index in range(self.module.MAX_DIRTY_ROOT_SAMPLE_ENTRIES + 5):
            (self.repo_path / f"operator-{index:02d}.txt").write_text(
                "operator data\n", encoding="utf-8",
            )

        result = run_setup_workspace(self.repo_path, prd_name)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "ready")

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
        prune_worktrees.assert_not_called()
        create_worktree.assert_not_called()

    def test_subprocess_setup_uses_fresh_origin_main_and_preserves_dirty_root(self):
        operator_path, upstream_path = create_remote_operator(self.repo_path)
        prd_name = "dirty-root-remote-prd"
        write_prd(operator_path, prd_name)

        (upstream_path / "remote-only.txt").write_text(
            "fresh remote start\n", encoding="utf-8",
        )
        git(["add", "remote-only.txt"], upstream_path)
        git(["commit", "-m", "advance remote main"], upstream_path)
        git(["push", "origin", "main"], upstream_path)
        remote_sha = git(["rev-parse", "HEAD"], upstream_path).stdout.strip()

        (operator_path / "README.md").write_text(
            "operator tracked edit\n", encoding="utf-8",
        )
        staged_path = operator_path / "operator-staged.txt"
        staged_path.write_text("operator staged\n", encoding="utf-8")
        git(["add", staged_path.name], operator_path)
        untracked_path = operator_path / "operator-untracked.txt"
        untracked_path.write_text("operator untracked\n", encoding="utf-8")

        before = {
            "head": git(["rev-parse", "HEAD"], operator_path).stdout.strip(),
            "branch": git(
                ["branch", "--show-current"], operator_path,
            ).stdout.strip(),
            "main": git(
                ["rev-parse", "refs/heads/main"], operator_path,
            ).stdout.strip(),
            "status": git(
                ["status", "--porcelain=v1"], operator_path,
            ).stdout,
            "index": git(["ls-files", "--stage"], operator_path).stdout,
        }
        trace_path = self.repo_path / "git-trace.json"
        result = run_setup_workspace(
            operator_path,
            prd_name,
            env={"GIT_TRACE2_EVENT": str(trace_path)},
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["status"], "ready")
        worktree_path = Path(payload["worktree"])
        self.assertEqual(
            git(["rev-parse", "HEAD"], worktree_path).stdout.strip(),
            remote_sha,
        )
        self.assertEqual(
            git(
                ["rev-parse", f"refs/heads/{prd_name}"],
                worktree_path,
            ).stdout.strip(),
            remote_sha,
        )
        self.assertEqual(
            git(["rev-parse", "HEAD"], operator_path).stdout.strip(),
            before["head"],
        )
        self.assertEqual(
            git(["branch", "--show-current"], operator_path).stdout.strip(),
            before["branch"],
        )
        self.assertEqual(
            git(["rev-parse", "refs/heads/main"], operator_path).stdout.strip(),
            before["main"],
        )
        self.assertEqual(
            git(["status", "--porcelain=v1"], operator_path).stdout,
            before["status"],
        )
        self.assertEqual(
            git(["ls-files", "--stage"], operator_path).stdout,
            before["index"],
        )
        self.assertEqual(
            staged_path.read_text(encoding="utf-8"),
            "operator staged\n",
        )
        self.assertEqual(
            untracked_path.read_text(encoding="utf-8"),
            "operator untracked\n",
        )

        trace_events = [
            json.loads(line)
            for line in trace_path.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
        child_commands = [
            event.get("child_argv", [])
            for event in trace_events
            if event.get("event") == "child_start"
        ]
        self.assertTrue(child_commands)
        self.assertFalse(
            any(
                command[:1] == ["stash"]
                or command[:2] == ["git", "stash"]
                for command in child_commands
            ),
        )

    def test_subprocess_dirty_root_refusal_survives_fetch_failure_without_main_refs(
        self,
    ):
        operator_path, _upstream_path = create_remote_operator(self.repo_path)
        prd_name = "dirty-root-fetch-failure-prd"
        write_prd(operator_path, prd_name)

        git(["checkout", "--detach", "HEAD"], operator_path)
        git(["update-ref", "-d", "refs/heads/main"], operator_path)
        git(["update-ref", "-d", "refs/remotes/origin/main"], operator_path)
        git(
            ["remote", "set-url", "origin", str(self.repo_path / "missing.git")],
            operator_path,
        )
        (operator_path / "README.md").write_text(
            "operator tracked edit\n", encoding="utf-8",
        )
        untracked_path = operator_path / "operator-untracked.txt"
        untracked_path.write_text("operator untracked\n", encoding="utf-8")

        entries = self.module.repository_status_entries(operator_path)
        expected_core = self.module.dirty_root_diagnostic(operator_path, entries)
        result = run_setup_workspace(operator_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(result.stdout, "")
        expected_lines = expected_core.splitlines()
        self.assertEqual(
            result.stderr.splitlines(),
            [
                "Root cleanliness check failed: "
                + expected_lines[0],
                *expected_lines[1:],
            ],
        )
        self.assertIn("repository root is dirty", result.stderr)
        self.assertNotIn(
            "fetch failed and refs/heads/main is missing",
            result.stderr,
        )

    def test_residual_refusal_attributes_matching_sibling_without_changing_core_text(self):
        operator_path = create_residual_refusal_repository(self.repo_path)
        prd_name = "residual-attribution-prd"
        write_prd(operator_path, prd_name)
        sibling_path = add_sibling_worktree(operator_path, "matching-sibling")

        (operator_path / "README.md").write_text(
            "same operator edit\n", encoding="utf-8",
        )
        (sibling_path / "README.md").write_text(
            "same operator edit\n", encoding="utf-8",
        )
        unusual_name = "operator [path] café.txt"
        (operator_path / unusual_name).write_text(
            "same untracked edit\n", encoding="utf-8",
        )
        (sibling_path / unusual_name).write_text(
            "same untracked edit\n", encoding="utf-8",
        )

        entries = self.module.repository_status_entries(operator_path)
        with mock.patch.object(
            self.module,
            "dirty_root_sibling_attribution",
            return_value=[],
        ):
            expected_core = self.module.dirty_root_diagnostic(
                operator_path,
                entries,
            )

        trace_path = self.repo_path / "residual-attribution-trace.json"
        result = run_setup_workspace(
            operator_path,
            prd_name,
            env={"GIT_TRACE2_EVENT": str(trace_path)},
        )

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertEqual(
            result.stderr.splitlines()[: len(expected_core.splitlines())],
            [
                "Root cleanliness check failed: "
                + expected_core.splitlines()[0],
                *expected_core.splitlines()[1:],
            ],
        )
        self.assertIn(
            'likely sibling worktree matches (same changes relative to origin/main):',
            result.stderr,
        )
        self.assertIn(
            'sibling worktree "matching-sibling" matches path(s): "README.md", '
            '"operator [path] caf\\u00e9.txt"',
            result.stderr,
        )
        self.assertIn("repository root is dirty", result.stderr)
        self.assertIn("Inspect the repository root manually", result.stderr)

        trace_events = [
            json.loads(line)
            for line in trace_path.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
        child_commands = [
            event.get("child_argv", [])
            for event in trace_events
            if event.get("event") == "child_start"
        ]
        self.assertFalse(
            any(
                command[:1] == ["stash"]
                or command[:2] == ["git", "stash"]
                for command in child_commands
            ),
        )

    def test_residual_refusal_omits_attribution_for_nonmatching_sibling(self):
        operator_path = create_residual_refusal_repository(self.repo_path)
        prd_name = "residual-negative-prd"
        write_prd(operator_path, prd_name)
        sibling_path = add_sibling_worktree(operator_path, "different-sibling")

        (operator_path / "README.md").write_text(
            "operator edit\n", encoding="utf-8",
        )
        (sibling_path / "README.md").write_text(
            "different edit\n", encoding="utf-8",
        )

        result = run_setup_workspace(operator_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn("Root cleanliness check failed", result.stderr)
        self.assertIn("repository root is dirty", result.stderr)
        self.assertNotIn("likely sibling worktree matches", result.stderr)
        self.assertNotIn("different-sibling", result.stderr)

    def test_residual_attribution_caps_paths_and_sibling_candidates(self):
        operator_path = create_residual_refusal_repository(self.repo_path)
        worktrees_dir = operator_path / ".claude" / "worktrees"
        worktrees_dir.mkdir(parents=True, exist_ok=True)
        for index in range(
            self.module.MAX_DIRTY_ROOT_ATTRIBUTION_WORKTREES + 5
        ):
            (worktrees_dir / f"candidate-{index:02d}").mkdir()

        for index in range(self.module.MAX_DIRTY_ROOT_ATTRIBUTION_PATHS + 5):
            (operator_path / f"operator-{index:02d}.txt").write_text(
                f"operator-{index}\n", encoding="utf-8",
            )
        entries = self.module.repository_status_entries(operator_path)

        recorded = []
        original_run_git = self.module.run_git

        def recording_run_git(*args, **kwargs):
            recorded.append((args, kwargs.get("cwd")))
            return original_run_git(*args, **kwargs)

        self.module.run_git = recording_run_git
        try:
            attribution = self.module.dirty_root_sibling_attribution(
                operator_path,
                entries,
            )
        finally:
            self.module.run_git = original_run_git

        self.assertEqual(attribution, [])
        baseline_commands = [
            args for args, _cwd in recorded if args[:1] == ("show",)
        ]
        candidate_commands = [
            args
            for args, cwd in recorded
            if args[:2] == ("rev-parse", "--show-toplevel")
            and Path(cwd).parent == worktrees_dir
        ]
        self.assertLessEqual(
            len(baseline_commands),
            self.module.MAX_DIRTY_ROOT_ATTRIBUTION_PATHS,
        )
        self.assertLessEqual(
            len(candidate_commands),
            self.module.MAX_DIRTY_ROOT_ATTRIBUTION_WORKTREES,
        )

    def test_residual_refusal_survives_missing_unusual_and_stale_paths(self):
        operator_path = create_residual_refusal_repository(self.repo_path)
        prd_name = "residual-malformed-prd"
        write_prd(operator_path, prd_name)
        (operator_path / "README.md").unlink()
        unusual_name = "operator [path] café.txt"
        (operator_path / unusual_name).write_text(
            "operator data\n", encoding="utf-8",
        )

        worktrees_dir = operator_path / ".claude" / "worktrees"
        invalid_path = worktrees_dir / "invalid-directory"
        invalid_path.mkdir(parents=True)
        stale_path = worktrees_dir / "stale-worktree"
        stale_path.mkdir()
        (stale_path / ".git").write_text(
            "gitdir: missing-worktree-metadata\n",
            encoding="utf-8",
        )

        result = run_setup_workspace(operator_path, prd_name)

        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(result.stdout, "")
        self.assertIn("Root cleanliness check failed", result.stderr)
        self.assertIn("repository root is dirty", result.stderr)
        self.assertIn("README.md", result.stderr)
        self.assertIn(
            json.dumps(unusual_name, ensure_ascii=True),
            result.stderr,
        )
        self.assertNotIn("likely sibling worktree matches", result.stderr)
        self.assertNotIn("Traceback", result.stderr)


if __name__ == "__main__":
    unittest.main()
