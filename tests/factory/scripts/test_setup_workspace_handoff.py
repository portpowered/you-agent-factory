"""Local-real coverage for exact packet handoff and pre-mutation refusal."""

import importlib.util
import copy
import io
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path
from unittest import mock

from preflight_test_support import write_packet as write_v1_packet


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
    if not (worktree_path / ".git").exists():
        packet_dir = worktree_path / "tasks" / "todo"
        packet_dir.mkdir(parents=True, exist_ok=True)
        packet = payload if payload is not None else {"branchName": prd_name}
        packet_path = packet_dir / f"{prd_name}.json"
        packet_path.write_text(json.dumps(packet), encoding="utf-8")
        if markdown is not None:
            packet_path.with_suffix(".md").write_text(markdown, encoding="utf-8")
        return packet_path
    packet_path, _ = write_v1_packet(
        worktree_path,
        prd_name,
        payload,
        include_md=markdown is not None,
    )
    if markdown is not None:
        packet_path.with_suffix(".md").write_text(markdown, encoding="utf-8")
    return packet_path


def create_directory_link(link_path, target_path):
    """Create a junction on Windows or a directory symlink elsewhere."""
    if os.name == "nt":
        result = subprocess.run(
            [
                "cmd.exe", "/d", "/c", "mklink", "/J",
                str(link_path), str(target_path),
            ],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            details = result.stderr.strip() or result.stdout.strip()
            raise OSError(
                f"mklink /J failed with exit {result.returncode}: {details}"
            )
        return
    link_path.symlink_to(target_path, target_is_directory=True)


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


def run_setup_workspace(repo_path, prd_name, *args):
    return subprocess.run(
        [sys.executable, str(SCRIPT_PATH), prd_name, *args],
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

    def test_null_recovery_creates_ordinary_workspace_with_exact_packet(self):
        init_repository(self.repo_path)
        prd_name = "null-recovery-fresh"
        source = write_packet(self.repo_path, prd_name, {
            "branchName": prd_name, "context": {"recovery": None},
        }, markdown="# ordinary fresh lane\n")
        packet_bytes = source.read_bytes()
        markdown_bytes = source.with_suffix(".md").read_bytes()

        result = run_setup_workspace(self.repo_path, prd_name, "--recovery-worktree", "")

        self.assertEqual(result.returncode, 0, result.stderr)
        output = json.loads(result.stdout)
        destination = self.repo_path / ".claude/worktrees" / prd_name
        self.assertEqual(set(output), {"status", "worktree", "branch", "prd_path",
                                     "prd_md_path", "standing_rules_path", "reused"})
        self.assertEqual(output["status"], "ready")
        self.assertFalse(output["reused"])
        self.assertEqual(output["branch"], prd_name)
        self.assertEqual(Path(output["worktree"]).resolve(), destination.resolve())
        self.assertEqual(Path(output["prd_path"]), destination / "prd.json")
        self.assertEqual(Path(output["prd_md_path"]), destination / "prd.md")
        self.assertEqual(Path(output["prd_path"]).read_bytes(), packet_bytes)
        self.assertEqual(Path(output["prd_md_path"]).read_bytes(), markdown_bytes)
        self.assertEqual(git(["branch", "--show-current"], destination).stdout.strip(), prd_name)
        self.assertEqual(source.read_bytes(), packet_bytes)

    def test_tagged_null_recovery_refuses_before_setup_or_adoption_mutations(self):
        init_repository(self.repo_path)
        prd_name = "null-recovery-tagged"
        _, retained = create_nested_worktree(self.repo_path, "retained")
        sentinel = retained / "sentinel.txt"
        sentinel.write_bytes(b"preserve retained bytes\x00\r\n")
        git(["add", sentinel.name], retained)
        packet = write_packet(self.repo_path, prd_name, {
            "branchName": prd_name, "context": {"recovery": None},
        })
        packet_bytes = packet.read_bytes()
        before = repository_snapshot(self.repo_path, (retained,))
        root_index = (self.repo_path / ".git/index").read_bytes()
        retained_index = git(["diff", "--cached", "--binary"], retained).stdout

        result = run_setup_workspace(self.repo_path, prd_name, "--recovery-worktree",
                                     ".claude/worktrees/fixture")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD: recovery-worktree tag requires context.recovery", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (retained,)), before)
        self.assertEqual((self.repo_path / ".git/index").read_bytes(), root_index)
        self.assertEqual(git(["diff", "--cached", "--binary"], retained).stdout, retained_index)
        self.assertEqual(packet.read_bytes(), packet_bytes)
        self.assertEqual(sentinel.read_bytes(), b"preserve retained bytes\x00\r\n")
        self.assertFalse((self.repo_path / ".claude/worktrees" / prd_name).exists())

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

    def test_handoff_rejects_packet_junction_escape_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "junction-escape-prd"
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        outside_dir = Path(tempfile.mkdtemp(prefix="setup-workspace-outside-"))
        self.addCleanup(shutil.rmtree, outside_dir, ignore_errors=True)
        outside_packet = write_packet(
            outside_dir,
            prd_name,
            {"branchName": prd_name, "payload": "must not escape"},
        )
        try:
            create_directory_link(nested_path / "tasks", outside_dir / "tasks")
        except (OSError, NotImplementedError) as error:
            self.skipTest(f"directory junction/symlink unavailable: {error}")
        before = repository_snapshot(self.repo_path, (nested_path,))
        outside_bytes = outside_packet.read_bytes()

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertIn("outside the registered worktree", result.stderr)
        self.assertNotIn("must not escape", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (nested_path,)), before)
        self.assertEqual(outside_packet.read_bytes(), outside_bytes)
        self.assertFalse((nested_path / "prd.json").exists())

    def test_handoff_rejects_non_regular_packet_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "directory-packet-prd"
        packet_dir = self.repo_path / "tasks" / "todo" / f"{prd_name}.json"
        packet_dir.mkdir(parents=True, exist_ok=True)
        before = repository_snapshot(self.repo_path)

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertIn("not a regular file", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path), before)

    def test_handoff_rejects_markdown_junction_escape_before_mutation(self):
        init_repository(self.repo_path)
        prd_name = "markdown-junction-escape-prd"
        _, nested_path = create_nested_worktree(self.repo_path, prd_name)
        write_packet(nested_path, prd_name, {"branchName": prd_name})
        outside_dir = Path(tempfile.mkdtemp(prefix="setup-workspace-md-outside-"))
        self.addCleanup(shutil.rmtree, outside_dir, ignore_errors=True)
        try:
            create_directory_link(
                nested_path / "tasks" / "todo" / f"{prd_name}.md",
                outside_dir,
            )
        except (OSError, NotImplementedError) as error:
            self.skipTest(f"directory junction/symlink unavailable: {error}")
        before = repository_snapshot(self.repo_path, (nested_path,))

        result = run_setup_workspace(self.repo_path, prd_name)

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Failed to read PRD:", result.stderr)
        self.assertIn("PRD Markdown candidate", result.stderr)
        self.assertIn("outside the registered worktree", result.stderr)
        self.assertNotIn("Root sync:", result.stderr)
        self.assertEqual(repository_snapshot(self.repo_path, (nested_path,)), before)
        self.assertFalse((nested_path / "prd.json").exists())

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


def recovery_packet(worktree=".claude/worktrees/lane", head="a" * 40):
    return {"context": {"recovery": {
        "originalSessionId": "11111111-1111-4111-8111-111111111111",
        "originalLaneWorkId": "original-idea", "predecessorWorkId": "original-idea", "attempt": 1,
        "diagnosis": {"classification": "visit_cap_with_progress", "evidence": ["worker-session:old"],
                      "blocker": "Complete adoption proof", "correction": "Finish the same PR"},
        "workspace": {"branch": "lane", "worktree": worktree,
                      "prUrl": "https://github.com/example/repository/pull/99", "headSha": head},
    }}}


class RecoveryPacketValidationTest(unittest.TestCase):
    """Unit proof of one packet validator with no Git or external processes."""

    def setUp(self):
        self.module = load_setup_workspace_module()
        self.path = ".claude/worktrees/fixture"
        self.packet = recovery_packet(self.path)

    def test_missing_or_null_recovery_without_tag_returns_none(self):
        for packet in ({}, {"context": {}}, {"context": {"recovery": None}}):
            with self.subTest(packet=packet):
                self.assertIsNone(self.module.validate_recovery_packet(packet, ""))

    def test_missing_or_null_recovery_with_tag_requires_packet(self):
        for packet in ({}, {"context": {}}, {"context": {"recovery": None}}):
            with self.subTest(packet=packet), self.assertRaisesRegex(
                ValueError, "^recovery-worktree tag requires context.recovery$",
            ):
                self.module.validate_recovery_packet(packet, self.path)

    def test_non_null_non_object_recovery_still_refuses(self):
        for value in ("invalid", "", [], 0, False):
            with self.subTest(value=value), self.assertRaisesRegex(
                ValueError, "^context.recovery must be an object$",
            ):
                self.module.validate_recovery_packet({"context": {"recovery": value}}, "")

    def test_invalid_context_and_empty_recovery_object_still_refuse(self):
        with self.assertRaisesRegex(ValueError, "^PRD context must be an object$"):
            self.module.validate_recovery_packet({"context": None}, "")
        with self.assertRaisesRegex(ValueError, "^recovery requires originalSessionId$"):
            self.module.validate_recovery_packet({"context": {"recovery": {}}}, "")

    def test_ordinary_and_fresh_recovery_preserve_name_derived_setup(self):
        self.assertIsNone(self.module.validate_recovery_packet({}, ""))
        for attempt in (1, 2):
            packet = copy.deepcopy(self.packet)
            packet["context"]["recovery"].update(attempt=attempt, workspace=None)
            self.assertEqual(self.module.validate_recovery_packet(packet, "")["attempt"], attempt)
        with self.assertRaisesRegex(ValueError, "fresh recovery"):
            self.module.validate_recovery_packet(packet, self.path)

    def test_valid_retained_packet_is_forwarded_without_rewriting(self):
        for classification in ("visit_cap_with_progress", "breaker_one_blocker", "deterministic_failure"):
            packet = copy.deepcopy(self.packet)
            packet["context"]["recovery"]["diagnosis"]["classification"] = classification
            self.assertIs(self.module.validate_recovery_packet(packet, self.path), packet["context"]["recovery"])

    def test_absolute_escaping_and_unnormalized_tags_refuse(self):
        for path in ("C:/repo/.claude/worktrees/lane", "/repo/.claude/worktrees/lane",
                     "C:lane", "\\\\host\\repo\\lane", ".claude/worktrees/../lane",
                     ".claude/worktrees/lane/", ".claude//worktrees/lane",
                     "./.claude/worktrees/lane", ".claude\\worktrees\\lane"):
            packet = copy.deepcopy(self.packet)
            packet["context"]["recovery"]["workspace"]["worktree"] = path
            with self.subTest(path=path), self.assertRaisesRegex(ValueError, "repo-relative"):
                self.module.validate_recovery_packet(packet, path)

    def test_missing_lineage_diagnosis_and_bad_attempts_refuse(self):
        paths = [("originalSessionId",), ("originalLaneWorkId",), ("predecessorWorkId",),
                 ("workspace",), ("diagnosis", "classification"), ("diagnosis", "blocker"),
                 ("diagnosis", "correction"), ("diagnosis", "evidence")]
        for path in paths:
            packet = copy.deepcopy(self.packet)
            value = packet["context"]["recovery"]
            for key in path[:-1]:
                value = value[key]
            del value[path[-1]]
            with self.subTest(path=path), self.assertRaises(ValueError):
                self.module.validate_recovery_packet(packet, self.path)
        for attempt in (0, 3, True, False, "1", 1.0, None):
            packet = copy.deepcopy(self.packet)
            packet["context"]["recovery"]["attempt"] = attempt
            with self.subTest(attempt=attempt), self.assertRaisesRegex(ValueError, "integer 1 or 2"):
                self.module.validate_recovery_packet(packet, self.path)

    def test_missing_tag_invalid_workspace_or_unknown_diagnosis_refuse(self):
        for field, value in (("branch", ""), ("worktree", "relative/lane"), ("prUrl", None),
                             ("headSha", "a" * 39), ("headSha", "not-a-commit")):
            packet = copy.deepcopy(self.packet)
            packet["context"]["recovery"]["workspace"][field] = value
            with self.subTest(field=field, value=value), self.assertRaises(ValueError):
                self.module.validate_recovery_packet(packet, self.path)
        with self.assertRaisesRegex(ValueError, "exactly match"):
            self.module.validate_recovery_packet(self.packet, "")
        with self.assertRaisesRegex(ValueError, "context.recovery"):
            self.module.validate_recovery_packet({}, self.path)
        self.packet["context"]["recovery"]["diagnosis"]["classification"] = "unknown"
        with self.assertRaisesRegex(ValueError, "classification"):
            self.module.validate_recovery_packet(self.packet, self.path)


class RecoveryOwnershipValidationTest(unittest.TestCase):
    """Unit proof of ownership/PR checks with controlled CLI and Git results."""

    def setUp(self):
        self.module = load_setup_workspace_module()
        self.workspace = recovery_packet()["context"]["recovery"]["workspace"]
        self.session = "11111111-1111-4111-8111-111111111111"
        self.successor = {"name": "lane-r2", "workId": "new", "state": {"type": "INITIAL"}}

    def test_paged_inventory_refuses_active_or_parked_owner(self):
        for state in ("init", "awaiting-answer", "awaiting-ci"):
            owner = {"name": "lane", "workId": "old", "state": {"type": "PROCESSING", "name": state}}
            pages = [{"sessions": [{"id": self.session}]},
                     {"results": [self.successor], "paginationContext": {"nextToken": "page2"}},
                     {"results": [owner], "paginationContext": {}}]
            with self.subTest(state=state), mock.patch.object(self.module, "recovery_command_json", side_effect=pages) as command:
                with self.assertRaisesRegex(ValueError, "active owner"):
                    self.module.validate_recovery_ownership(Path.cwd(), "lane-r2", self.workspace)
                self.assertIn("page2", command.call_args.args[0])

    def test_terminal_predecessor_and_same_successor_allow_repeat(self):
        works = [self.successor, {"name": "lane", "state": {"type": "FAILED"}},
                 {"name": "healthy", "state": {"type": "PROCESSING"}},
                 {"name": "older-r2", "tags": {"recovery-worktree": self.workspace["worktree"]},
                  "state": {"type": "TERMINAL"}}]
        with mock.patch.object(self.module, "recovery_command_json", side_effect=[
            {"sessions": [{"id": self.session}]}, {"results": works, "paginationContext": {}},
        ]):
            self.module.validate_recovery_ownership(Path.cwd(), "lane-r2", self.workspace)

    def test_unverifiable_or_duplicate_session_ownership_refuses(self):
        cases = [
            [{"sessions": []}],
            [{"sessions": [{"id": self.session}]}, {"results": [], "paginationContext": {}}],
            [{"sessions": [{"id": self.session}]}, {"results": []}],
            [{"sessions": [{"id": self.session}, {"id": "22222222-2222-4222-8222-222222222222"}]},
             {"results": [self.successor], "paginationContext": {}},
             {"results": [self.successor], "paginationContext": {}}],
        ]
        for pages in cases:
            with self.subTest(pages=pages), mock.patch.object(self.module, "recovery_command_json", side_effect=pages):
                with self.assertRaises(ValueError):
                    self.module.validate_recovery_ownership(Path.cwd(), "lane-r2", self.workspace)

    def test_pr_mismatch_closed_merged_or_missing_head_refuses_without_git(self):
        pr = {"url": self.workspace["prUrl"], "state": "OPEN", "headRefName": "lane",
              "headRefOid": "a" * 40, "isCrossRepository": False}
        for field, value in (("state", "CLOSED"), ("state", "MERGED"), ("headRefName", "other"),
                             ("headRefOid", ""), ("isCrossRepository", True)):
            invalid = dict(pr, **{field: value})
            with self.subTest(field=field, value=value), mock.patch.object(self.module, "recovery_command_json", side_effect=[
                {"url": "https://github.com/example/repository"}, invalid,
            ]), mock.patch.object(self.module.subprocess, "run") as command:
                with self.assertRaisesRegex(ValueError, "OPEN"):
                    self.module.validate_recovery_pr(Path.cwd(), self.workspace)
                command.assert_not_called()

    def test_external_check_timeout_or_missing_executable_never_falls_back(self):
        for error in (FileNotFoundError("gh unavailable"), subprocess.TimeoutExpired("gh", 15)):
            with self.subTest(error=error), mock.patch.object(self.module.subprocess, "run", side_effect=error):
                with self.assertRaises(type(error)):
                    self.module.recovery_command_json(["gh", "repo", "view"], Path.cwd())


class RecoveryWorkspacePreservationTest(unittest.TestCase):
    """I1: real local Git/files, controlled read-only gh and live ownership CLI.

    Two setup visits serialize because they adopt the same customer directory.
    This proof makes no claim about compiled Factory routing or live judgment.
    """

    def test_I1_adoption_repeats_without_resetting_commits_dirty_files_or_scaffold(self):
        module = load_setup_workspace_module()
        with tempfile.TemporaryDirectory(prefix="retained-recovery-") as directory:
            repo = Path(directory)
            init_repository(repo)
            retained = repo / ".claude" / "worktrees" / "lane"
            retained.parent.mkdir(parents=True)
            git(["worktree", "add", "-b", "lane", str(retained), "main"], repo)
            remote_head = git(["rev-parse", "HEAD"], retained).stdout.strip()
            (retained / "change.txt").write_bytes(b"committed improvement\n")
            git(["add", "change.txt"], retained)
            git(["commit", "-m", "useful unpublished work"], retained)
            head = git(["rev-parse", "HEAD"], retained).stdout.strip()
            saved = {"change.txt": b"dirty improvement\n", "untracked.txt": b"untracked\x00bytes",
                     "prd.json": b'{"project":"old"}', "progress.txt": b"old progress\r\n"}
            for name, data in saved.items():
                (retained / name).write_bytes(data)
            git(["add", "change.txt"], retained)
            (retained / "change.txt").write_bytes(b"unstaged improvement\n")
            saved["change.txt"] = b"unstaged improvement\n"
            before = git(["status", "--porcelain=v1", "-z"], retained).stdout
            index_before = git(["diff", "--cached", "--binary"], retained).stdout
            packet = recovery_packet(".claude/worktrees/lane", head)
            packet["branchName"] = "lane-r2"
            source = write_packet(repo, "lane-r2", packet, markdown="# retained slice\n")
            # Invalid tags cannot change any retained file or Git identity.
            snapshot = repository_snapshot(repo, (retained,))
            for invalid in (str(retained), ".claude/worktrees/../lane"):
                invalid_packet = copy.deepcopy(packet)
                invalid_packet["context"]["recovery"]["workspace"]["worktree"] = invalid
                with self.subTest(tag=invalid), self.assertRaisesRegex(ValueError, "repo-relative"):
                    module.validate_recovery_packet(invalid_packet, invalid)
                self.assertEqual(repository_snapshot(repo, (retained,)), snapshot)
            def external(command, cwd):
                if command[:3] == ["gh", "repo", "view"]:
                    return {"url": "https://github.com/example/repository"}
                if command[:3] == ["gh", "pr", "view"]:
                    return {"url": packet["context"]["recovery"]["workspace"]["prUrl"], "state": "OPEN",
                            "headRefName": "lane", "headRefOid": remote_head, "isCrossRepository": False}
                if "session" in command:
                    return {"sessions": [{"id": "11111111-1111-4111-8111-111111111111"}]}
                return {"results": [{"name": "lane", "state": {"type": "FAILED"}},
                                    {"name": "lane-r2", "state": {"type": "INITIAL"}}], "paginationContext": {}}
            for visit in range(2):
                with self.subTest(visit=visit), mock.patch.object(module, "get_repo_root", return_value=repo), \
                     mock.patch.object(module, "recovery_command_json", side_effect=external), \
                     mock.patch.object(module, "sync_main") as sync, mock.patch.object(module, "prune_worktrees") as prune, \
                     mock.patch.object(sys, "argv", ["setup-workspace.py", "lane-r2", "--recovery-worktree", ".claude/worktrees/lane"]):
                    stdout, stderr = io.StringIO(), io.StringIO()
                    with redirect_stdout(stdout), redirect_stderr(stderr):
                        module.main()
                    result = json.loads(stdout.getvalue())
                    self.assertEqual(result["branch"], "lane")
                    self.assertEqual(Path(result["worktree"]), retained)
                    self.assertEqual(Path(result["prd_path"]), retained / "tasks/todo/lane-r2.json")
                    self.assertTrue(result["reused"])
                    sync.assert_not_called()
                    prune.assert_not_called()
                for name, data in saved.items():
                    self.assertEqual((retained / name).read_bytes(), data)
                self.assertEqual(git(["rev-parse", "HEAD"], retained).stdout.strip(), head)
                self.assertEqual(git(["status", "--porcelain=v1", "-z"], retained).stdout, before)
                self.assertEqual(git(["diff", "--cached", "--binary"], retained).stdout, index_before)
                self.assertEqual(Path(result["prd_path"]).read_bytes(), source.read_bytes())
            # A closed retained PR must refuse with no receipt or file/ref effects.
            def closed_pr(command, cwd):
                data = external(command, cwd)
                if command[:3] == ["gh", "pr", "view"]:
                    data["state"] = "CLOSED"
                return data
            snapshot = repository_snapshot(repo, (retained,))
            with mock.patch.object(module, "get_repo_root", return_value=repo), \
                 mock.patch.object(module, "recovery_command_json", side_effect=closed_pr), \
                 mock.patch.object(sys, "argv", ["setup-workspace.py", "lane-r2", "--recovery-worktree", ".claude/worktrees/lane"]):
                stdout, stderr = io.StringIO(), io.StringIO()
                with redirect_stdout(stdout), redirect_stderr(stderr), self.assertRaises(SystemExit) as refusal:
                    module.main()
                self.assertEqual(refusal.exception.code, 1)
                self.assertEqual(stdout.getvalue(), "")
                self.assertIn("OPEN", stderr.getvalue())
            self.assertEqual(repository_snapshot(repo, (retained,)), snapshot)
            for name, data in saved.items():
                self.assertEqual((retained / name).read_bytes(), data)
            # A changed destination refuses before copying any new packet.
            Path(result["prd_path"]).write_bytes(b"different successor")
            inventory = module.list_registered_worktrees(repo)
            with self.assertRaisesRegex(RuntimeError, "ambiguous PRD"):
                module.select_prd_candidate(repo, "lane-r2", inventory, ".claude/worktrees/lane")


class RecoveryCheckoutValidationTest(unittest.TestCase):
    """Unit proof of registration validation with controlled Git identities."""

    def test_locked_detached_mismatched_and_wrong_head_refuse(self):
        module = load_setup_workspace_module()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            (path / ".git").write_text("fixture", encoding="utf-8")
            record = {"path": path, "branch_ref": "refs/heads/lane", "detached": False,
                      "locked": False, "prunable": False}
            for field, value in (("locked", True), ("prunable", True), ("detached", True),
                                 ("branch_ref", "refs/heads/other")):
                with self.subTest(field=field), mock.patch.object(module, "registered_worktree_record", return_value=dict(record, **{field: value})), \
                     mock.patch.object(module, "resolve_checkout_head") as head:
                    with self.assertRaises(RuntimeError):
                        module.validate_registered_worktree(path, path, "lane", expected_head="a" * 40)
                    head.assert_not_called()
            with mock.patch.object(module, "registered_worktree_record", return_value=record), \
                 mock.patch.object(module, "resolve_checkout_head", return_value="b" * 40), \
                 mock.patch.object(module, "resolve_revision_head", return_value="b" * 40):
                with self.assertRaisesRegex(RuntimeError, "expected"):
                    module.validate_registered_worktree(path, path, "lane", expected_head="a" * 40)

    def test_escaped_and_same_name_worktree_refuse_before_git(self):
        module = load_setup_workspace_module()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            managed = root / ".claude/worktrees"
            managed.mkdir(parents=True)
            workspace = recovery_packet(str(root))["context"]["recovery"]["workspace"]
            with mock.patch.object(module, "validate_registered_worktree") as validate:
                with self.assertRaisesRegex(ValueError, "managed"):
                    module.recovery_destination(root, workspace, "lane-r2")
                workspace["worktree"] = str(managed)
                with self.assertRaisesRegex(ValueError, "managed"):
                    module.recovery_destination(root, workspace, "lane-r2")
                retained = managed / "lane"
                retained.mkdir()
                workspace["worktree"] = ".claude/worktrees/lane"
                with self.assertRaisesRegex(ValueError, "new successor name"):
                    module.recovery_destination(root, workspace, "lane")
                validate.assert_not_called()


if __name__ == "__main__":
    unittest.main()
