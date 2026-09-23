"""Local-real regressions for workspace admission from canonical Git state."""

import json
import subprocess
import tempfile
import unittest
from pathlib import Path

from preflight_test_support import write_packet

REPO_ROOT = Path(__file__).resolve().parents[3]
SCRIPT_PATH = REPO_ROOT / "factory" / "scripts" / "setup-workspace.py"


def git(args, cwd):
    return subprocess.run(
        ["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
    )


def init_repository(repo_path):
    git(["init", "-b", "main"], repo_path)
    git(["config", "user.email", "workspace-admission@example.com"], repo_path)
    git(["config", "user.name", "Workspace Admission"], repo_path)
    (repo_path / "README.md").write_text("fixture\n", encoding="utf-8")
    git(["add", "README.md"], repo_path)
    git(["commit", "-m", "initial"], repo_path)


def run_setup(repo_path, name):
    return subprocess.run(
        ["python", str(SCRIPT_PATH), name],
        cwd=repo_path,
        capture_output=True,
        text=True,
        check=False,
    )


class WorkspaceAdmissionTest(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_path = Path(self.temp_dir.name)
        init_repository(self.repo_path)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_packet_without_preflight_or_declared_branch_is_admitted(self):
        name = "derived-work-identity"
        write_packet(
            self.repo_path,
            name,
            {"project": "embed", "description": "use canonical Work identity"},
        )

        result = run_setup(self.repo_path, name)

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["branch"], name)
        self.assertNotIn("preflight", payload)
        self.assertEqual(
            git(["branch", "--show-current"], Path(payload["worktree"])).stdout.strip(),
            name,
        )

    def test_incomplete_changed_path_advice_does_not_block_discovered_path(self):
        name = "expand-impact-after-planning"
        write_packet(
            self.repo_path,
            name,
            {
                "project": "localai",
                "changedPathLease": ["docs/planned.md"],
                "description": "implementation may discover required paths",
            },
        )
        first = run_setup(self.repo_path, name)
        self.assertEqual(first.returncode, 0, first.stderr)
        worktree = Path(json.loads(first.stdout)["worktree"])

        planned = worktree / "docs" / "planned.md"
        discovered = worktree / "pkg" / "publication" / "gpu.go"
        planned.parent.mkdir(parents=True, exist_ok=True)
        discovered.parent.mkdir(parents=True, exist_ok=True)
        planned.write_text("planned\n", encoding="utf-8")
        discovered.write_text("package publication\n", encoding="utf-8")
        git(["add", "docs/planned.md", "pkg/publication/gpu.go"], worktree)
        git(["commit", "-m", "include discovered publication path"], worktree)

        second = run_setup(self.repo_path, name)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertTrue(json.loads(second.stdout)["reused"])
        self.assertTrue(discovered.is_file())

    def test_main_registered_destination_is_adopted_as_isolated_lane(self):
        name = "recover-main-registration"
        git(["checkout", "-b", "operator"], self.repo_path)
        destination = self.repo_path / ".claude" / "worktrees" / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        git(["worktree", "add", str(destination), "main"], self.repo_path)
        write_packet(destination, name, {"project": "embed"})

        result = run_setup(self.repo_path, name)

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertTrue(payload["reused"])
        self.assertEqual(Path(payload["worktree"]).resolve(), destination.resolve())
        self.assertEqual(
            git(["branch", "--show-current"], destination).stdout.strip(), name,
        )
        self.assertTrue((destination / "prd.json").is_file())

    def test_stale_main_destination_uses_fresh_origin_without_moving_main(self):
        name = "recover-stale-main-registration"
        local_main = git(["rev-parse", "main"], self.repo_path).stdout.strip()
        with tempfile.TemporaryDirectory() as remote_directory:
            remote_root = Path(remote_directory)
            bare = remote_root / "remote.git"
            git(["init", "--bare", "-b", "main", str(bare)], remote_root)
            git(["remote", "add", "origin", str(bare)], self.repo_path)
            git(["push", "-u", "origin", "main"], self.repo_path)

            git(["checkout", "-b", "operator"], self.repo_path)
            destination = self.repo_path / ".claude" / "worktrees" / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            git(["worktree", "add", str(destination), "main"], self.repo_path)
            write_packet(destination, name, {"project": "embed"})

            upstream = remote_root / "upstream"
            git(["clone", str(bare), str(upstream)], remote_root)
            git(["config", "user.email", "upstream@example.com"], upstream)
            git(["config", "user.name", "Upstream"], upstream)
            (upstream / "ahead.txt").write_text("fresh origin\n", encoding="utf-8")
            git(["add", "ahead.txt"], upstream)
            git(["commit", "-m", "advance origin"], upstream)
            git(["push", "origin", "main"], upstream)
            remote_main = git(["rev-parse", "HEAD"], upstream).stdout.strip()

            result = run_setup(self.repo_path, name)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            git(["rev-parse", "refs/heads/main"], self.repo_path).stdout.strip(),
            local_main,
        )
        self.assertEqual(git(["rev-parse", "HEAD"], destination).stdout.strip(), remote_main)
        self.assertEqual(
            git(["branch", "--show-current"], destination).stdout.strip(), name,
        )
        self.assertTrue((destination / "ahead.txt").is_file())


if __name__ == "__main__":
    unittest.main()
