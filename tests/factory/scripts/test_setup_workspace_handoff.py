"""Local-real coverage for exact packet handoff and pre-mutation refusal."""

import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from unittest import mock


REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"


def load_setup_workspace_module():
    spec = importlib.util.spec_from_file_location(
        "setup_workspace_handoff", SCRIPT_PATH,
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
    configure_ignores(repo_path)


def configure_ignores(repo_path):
    exclude_path = repo_path / ".git" / "info" / "exclude"
    exclude_path.write_text("tasks/todo/\n.claude/\n", encoding="utf-8")


def create_nested_worktree(repo_path, prd_name, seed=True, branch=None):
    parent_path = repo_path / ".claude" / "worktrees" / "parent"
    parent_path.parent.mkdir(parents=True, exist_ok=True)
    parent_branch = "parent-worktree"
    git(
        ["worktree", "add", "-b", parent_branch, str(parent_path), "main"],
        repo_path,
    )
    if seed:
        seed_path = parent_path / "parent-seed.txt"
        seed_path.write_text("parent branch is active\n", encoding="utf-8")
        git(["add", seed_path.name], parent_path)
        git(["commit", "-m", "start parent lane"], parent_path)

    nested_path = parent_path / ".claude" / "worktrees" / prd_name
    nested_path.parent.mkdir(parents=True, exist_ok=True)
    attached_branch = branch or prd_name
    git(
        [
            "worktree",
            "add",
            "-b",
            attached_branch,
            str(nested_path),
            parent_branch,
        ],
        repo_path,
    )
    return parent_path, nested_path


def write_packet(worktree_path, prd_name, payload=None, markdown=None):
    packet_dir = worktree_path / "tasks" / "todo"
    packet_dir.mkdir(parents=True, exist_ok=True)
    packet = payload if payload is not None else {"branchName": prd_name}
    packet_path = packet_dir / f"{prd_name}.json"
    packet_path.write_text(json.dumps(packet), encoding="utf-8")
    if markdown is not None:
        (packet_dir / f"{prd_name}.md").write_text(markdown, encoding="utf-8")
    return packet_path


def create_remote_clone(base_path):
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
    configure_ignores(operator_path)
    return operator_path


def run_setup_workspace(repo_path, prd_name):
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), prd_name],
        cwd=repo_path,
        capture_output=True,
        text=True,
        check=False,
    )


def repository_snapshot(repo_path, nested_paths=()):
    snapshot = {
        "head": git(["rev-parse", "HEAD"], repo_path).stdout.strip(),
        "main": git(
            ["rev-parse", "--verify", "refs/heads/main"],
            repo_path,
            check=False,
        ).stdout.strip(),
        "refs": git(["show-ref"], repo_path, check=False).stdout,
        "inventory": git(
            ["worktree", "list", "--porcelain"], repo_path,
        ).stdout,
        "status": git(
            ["status", "--porcelain=v1", "--untracked-files=all", "--ignored"],
            repo_path,
        ).stdout,
    }
    for index, nested_path in enumerate(nested_paths):
        snapshot[f"nested-{index}-head"] = git(
            ["rev-parse", "HEAD"], nested_path,
        ).stdout.strip()
        snapshot[f"nested-{index}-branch"] = git(
            ["branch", "--show-current"], nested_path,
        ).stdout.strip()
        snapshot[f"nested-{index}-status"] = git(
            ["status", "--porcelain=v1", "--untracked-files=all", "--ignored"],
            nested_path,
        ).stdout
    return snapshot


class SetupWorkspaceHandoffTest(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_handoff_reuses_exact_nested_packet_and_preserves_content(self):
        init_repository(self.repo_path)
        prd_name = "nested-handoff-prd"
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        packet_path = write_packet(
            nested_path,
            prd_name,
            {"branchName": prd_name, "payload": "nested packet"},
            markdown="# nested packet\n",
        )
        sentinel = nested_path / "sentinel.txt"
        sentinel.write_text("preserve me\n", encoding="utf-8")
        before = repository_snapshot(self.repo_path, (nested_path,))
        nested_head = before["nested-0-head"]

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        self.assertEqual(output["status"], "ready")
        self.assertTrue(output["reused"])
        self.assertEqual(
            Path(output["worktree"]).resolve(), nested_path.resolve(),
        )
        self.assertEqual(
            Path(output["prd_path"]).resolve(),
            (nested_path / "prd.json").resolve(),
        )
        self.assertEqual(
            Path(output["prd_md_path"]).resolve(),
            (nested_path / "prd.md").resolve(),
        )
        self.assertEqual(
            (nested_path / "prd.json").read_bytes(), packet_path.read_bytes(),
        )
        self.assertEqual(
            (nested_path / "prd.md").read_text(encoding="utf-8"),
            "# nested packet\n",
        )
        self.assertEqual(sentinel.read_text(encoding="utf-8"), "preserve me\n")
        self.assertEqual(
            git(["rev-parse", "HEAD"], nested_path).stdout.strip(),
            nested_head,
        )
        self.assertEqual(
            git(["branch", "--show-current"], nested_path).stdout.strip(),
            prd_name,
        )
        self.assertFalse(
            (self.repo_path / "tasks" / "todo" / f"{prd_name}.json").exists()
        )

    def test_handoff_reuses_nested_packet_without_markdown(self):
        init_repository(self.repo_path)
        prd_name = "nested-handoff-json-only"
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        write_packet(nested_path, prd_name, {"branchName": prd_name})
        sentinel = nested_path / "json-only-sentinel.txt"
        sentinel.write_text("keep\n", encoding="utf-8")

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        self.assertTrue(output["reused"])
        self.assertEqual(output["prd_md_path"], None)
        self.assertEqual(sentinel.read_text(encoding="utf-8"), "keep\n")

    def test_handoff_allows_a_local_nested_branch_with_positive_activity(self):
        init_repository(self.repo_path)
        prd_name = "nested-local-active-prd"
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        write_packet(nested_path, prd_name)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            Path(json.loads(result.stdout)["worktree"]).resolve(),
            nested_path.resolve(),
        )

    def test_handoff_refuses_root_and_nested_duplicate_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "duplicate-root-nested-prd"
        root_packet = write_packet(self.repo_path, prd_name)
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        nested_packet = write_packet(nested_path, prd_name)
        before = repository_snapshot(self.repo_path, (nested_path,))

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertIn("ambiguous PRD", result.stderr)
        self.assertIn(json.dumps(str(root_packet)), result.stderr)
        self.assertIn(json.dumps(str(nested_packet)), result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (nested_path,)), before)

    def test_handoff_refuses_multiple_nested_duplicates_deterministically(self):
        init_repository(self.repo_path)
        prd_name = "duplicate-nested-prd"
        first_parent, first_nested = create_nested_worktree(self.repo_path, prd_name)
        first_packet = write_packet(first_nested, prd_name)
        second_path = self.repo_path / ".claude" / "worktrees" / "second"
        git(
            ["worktree", "add", "-b", "second-worktree", str(second_path), "main"],
            self.repo_path,
        )
        second_nested = second_path / ".claude" / "worktrees" / prd_name
        second_nested.parent.mkdir(parents=True, exist_ok=True)
        git(
            ["worktree", "add", "-b", "second-prd", str(second_nested), "main"],
            self.repo_path,
        )
        second_packet = write_packet(second_nested, prd_name)
        before = repository_snapshot(self.repo_path, (first_nested, second_nested))

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("ambiguous PRD", result.stderr)
        self.assertIn(json.dumps(str(first_packet)), result.stderr)
        self.assertIn(json.dumps(str(second_packet)), result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(
            repository_snapshot(self.repo_path, (first_nested, second_nested)),
            before,
        )
        self.assertTrue(first_parent.exists())

    def test_handoff_refuses_wrong_attached_branch_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "wrong-attached-branch-prd"
        _, nested_path = create_nested_worktree(
            self.repo_path, prd_name, branch="different-branch",
        )
        write_packet(nested_path, prd_name, {"branchName": prd_name})
        before = repository_snapshot(self.repo_path, (nested_path,))

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("ineligible", result.stderr)
        self.assertIn("different-branch", result.stderr)
        self.assertIn(f"expected refs/heads/{prd_name}", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (nested_path,)), before)

    def test_handoff_refuses_locked_attached_worktree_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "locked-attached-prd"
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        write_packet(nested_path, prd_name)
        git(["worktree", "lock", str(nested_path)], self.repo_path)
        before = repository_snapshot(self.repo_path, (nested_path,))

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("ineligible", result.stderr)
        self.assertIn("locked", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (nested_path,)), before)

    def test_handoff_refuses_detached_packet_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "detached-packet-prd"
        nested_path = self.repo_path / ".claude" / "worktrees" / "detached"
        nested_path.parent.mkdir(parents=True, exist_ok=True)
        git(["worktree", "add", "--detach", str(nested_path), "main"], self.repo_path)
        write_packet(nested_path, prd_name)
        before = repository_snapshot(self.repo_path, (nested_path,))

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("ineligible", result.stderr)
        self.assertIn("detached", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (nested_path,)), before)

    def test_handoff_rejects_malformed_packet_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "malformed-handoff-prd"
        packet = self.repo_path / "tasks" / "todo" / f"{prd_name}.json"
        packet.parent.mkdir(parents=True, exist_ok=True)
        packet.write_text("{malformed packet secret", encoding="utf-8")
        before = repository_snapshot(self.repo_path)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertNotIn("malformed packet secret", result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path), before)

    def test_handoff_rejects_non_object_packet_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "shape-handoff-prd"
        write_packet(self.repo_path, prd_name, [prd_name, "secret"])
        before = repository_snapshot(self.repo_path)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertIn("PRD must be a JSON object", result.stderr)
        self.assertNotIn("secret", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path), before)

    def test_handoff_rejects_missing_branch_identity_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "missing-identity-handoff-prd"
        write_packet(self.repo_path, prd_name, {"description": "secret"})
        before = repository_snapshot(self.repo_path)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertIn("branchName must be a string", result.stderr)
        self.assertNotIn("secret", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path), before)

    def test_handoff_rejects_mismatched_branch_identity_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "mismatched-identity-handoff-prd"
        write_packet(
            self.repo_path,
            prd_name,
            {"branchName": "different-work", "payload": "secret"},
        )
        before = repository_snapshot(self.repo_path)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertIn("branchName mismatch", result.stderr)
        self.assertIn("different-work", result.stderr)
        self.assertNotIn("secret", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path), before)

    def test_handoff_missing_packet_fails_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "missing-handoff-prd"
        before = repository_snapshot(self.repo_path)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertIn("PRD not found", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path), before)

    def test_handoff_rejects_stale_same_name_branch_before_root_sync(self):
        operator_path = create_remote_clone(self.repo_path)
        prd_name = "stale-name-collision-prd"
        _, nested_path = create_nested_worktree(
            operator_path, prd_name, seed=False,
        )
        write_packet(
            nested_path,
            prd_name,
            {"branchName": prd_name, "payload": "earlier program"},
        )
        sentinel = nested_path / "stale-sentinel.txt"
        sentinel.write_text("do not adopt\n", encoding="utf-8")
        before = repository_snapshot(operator_path, (nested_path,))

        result = run_setup_workspace(operator_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("appears abandoned", result.stderr)
        self.assertIn("0 commits ahead of origin/main", result.stderr)
        self.assertIn("no remote head", result.stderr)
        self.assertNotIn("earlier program", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(operator_path, (nested_path,)), before)
        self.assertEqual(sentinel.read_text(encoding="utf-8"), "do not adopt\n")
        self.assertFalse((nested_path / "prd.json").exists())

    def test_handoff_inventory_failure_stops_before_sync(self):
        init_repository(self.repo_path)
        prd_name = "inventory-failure-handoff-prd"
        write_packet(self.repo_path, prd_name)
        module = load_setup_workspace_module()
        stdout = io.StringIO()
        stderr = io.StringIO()
        original_cwd = os.getcwd()
        os.chdir(self.repo_path)
        try:
            with mock.patch.object(
                module,
                "list_registered_worktrees",
                side_effect=RuntimeError("simulated inventory failure"),
            ), mock.patch.object(module, "sync_main") as sync_main:
                with mock.patch.object(sys, "argv", ["setup-workspace.py", prd_name]):
                    with redirect_stdout(stdout), redirect_stderr(stderr):
                        with self.assertRaises(SystemExit) as raised:
                            module.main()
        finally:
            os.chdir(original_cwd)

        self.assertEqual(raised.exception.code, 1)
        self.assertEqual(stdout.getvalue(), "")
        self.assertIn("simulated inventory failure", stderr.getvalue())
        sync_main.assert_not_called()


if __name__ == "__main__":
    unittest.main()
