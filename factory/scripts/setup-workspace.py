#!/usr/bin/env python3
"""setup-workspace.py — Create or reuse a git worktree for a PRD.

Usage: python scripts/agents/setup-workspace.py <prd-name>

Reads the PRD from tasks/todo/<prd-name>.json, extracts the branchName,
syncs main, creates or reuses a git worktree, copies the PRD (and optional
.md) into the worktree root, and prints a JSON result to stdout.

Exit 0 on success (stdout = JSON blob), exit 1 on failure (stderr = error).
"""

import json

import shutil
import subprocess
import sys
from pathlib import Path


def run_git(*args, cwd=None, check=True):
    """Run a git command, returning stdout. Raises on failure if check=True."""
    result = subprocess.run(
        ["git"] + list(args),
        cwd=cwd,
        capture_output=True,
        text=True,
    )
    if check and result.returncode != 0:
        raise RuntimeError(
            f"git {' '.join(args)} failed (exit {result.returncode}): {result.stderr.strip()}"
        )
    return result


def get_repo_root():
    """Discover the repository root via git."""
    result = run_git("rev-parse", "--show-toplevel")
    return Path(result.stdout.strip())


def read_prd(prd_path):
    """Read and parse a PRD JSON file. Returns the parsed dict."""
    with open(prd_path, "r", encoding="utf-8") as f:
        return json.load(f)


def sync_main(repo_root):
    """Run git pull unless the repo has no upstream configured."""
    result = run_git("pull", cwd=repo_root, check=False)
    if result.returncode == 0:
        return

    stderr = result.stderr.lower()
    if "there is no tracking information for the current branch" in stderr:
        return

    raise RuntimeError(
        f"git pull failed (exit {result.returncode}): {result.stderr.strip()}"
    )


def prune_worktrees(repo_root):
    """Prune stale worktree entries."""
    run_git("worktree", "prune", cwd=repo_root)


def normalize_branch(branch_name):
    """Convert branch name to a filesystem-safe directory name."""
    return branch_name.replace("/", "-")


def worktree_is_valid(worktree_path):
    """Check if an existing worktree path is valid and has content."""
    git_file = worktree_path / ".git"
    if not git_file.exists():
        return False
    # Check for non-.git content.
    entries = [e for e in worktree_path.iterdir() if e.name != ".git"]
    return len(entries) > 0


def branch_exists_locally(repo_root, branch):
    """Check if a branch exists as a local ref."""
    result = run_git(
        "rev-parse", "--verify", f"refs/heads/{branch}",
        cwd=repo_root, check=False,
    )
    return result.returncode == 0


def branch_exists_on_remote(repo_root, branch):
    """Check if a branch exists on origin."""
    result = run_git(
        "rev-parse", "--verify", f"refs/remotes/origin/{branch}",
        cwd=repo_root, check=False,
    )
    return result.returncode == 0


def create_or_reuse_worktree(repo_root, branch, worktree_path):
    """Create a new worktree or reuse an existing one. Returns reused flag."""
    if worktree_path.exists() and worktree_is_valid(worktree_path):
        # Reuse: checkout branch and pull latest.
        run_git("-C", str(worktree_path), "checkout", branch, cwd=repo_root)
        if branch_exists_on_remote(repo_root, branch):
            run_git(
                "-C", str(worktree_path), "pull", "--ff-only",
                cwd=repo_root, check=False,
            )
        return True

    # Remove stale path if it exists but is invalid.
    if worktree_path.exists():
        shutil.rmtree(worktree_path)

    # Create new worktree.
    worktree_path.parent.mkdir(parents=True, exist_ok=True)

    if branch_exists_locally(repo_root, branch):
        run_git(
            "worktree", "add", str(worktree_path), branch,
            cwd=repo_root,
        )
    elif branch_exists_on_remote(repo_root, branch):
        run_git(
            "worktree", "add", "--track", "-b", branch,
            str(worktree_path), f"origin/{branch}",
            cwd=repo_root,
        )
    else:
        run_git(
            "worktree", "add", "-b", branch, str(worktree_path), "main",
            cwd=repo_root,
        )

    return False


def copy_prd_files(prd_json_path, prd_md_path, worktree_path):
    """Copy PRD files into the worktree root."""
    dest_json = worktree_path / "prd.json"
    shutil.copy2(str(prd_json_path), str(dest_json))

    dest_md = None
    if prd_md_path and prd_md_path.exists():
        dest_md = worktree_path / "prd.md"
        shutil.copy2(str(prd_md_path), str(dest_md))

    return dest_json, dest_md


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <prd-name>", file=sys.stderr)
        sys.exit(1)

    prd_name = sys.argv[1]

    try:
        repo_root = get_repo_root()
    except RuntimeError as e:
        print(f"Failed to discover repo root: {e}", file=sys.stderr)
        sys.exit(1)

    # Locate PRD files.
    prd_json_path = repo_root / "tasks" / "todo" / f"{prd_name}.json"
    if not prd_json_path.exists():
        print(f"PRD not found: {prd_json_path}", file=sys.stderr)
        sys.exit(1)

    prd_md_path = repo_root / "tasks" / "todo" / f"{prd_name}.md"
    if not prd_md_path.exists():
        prd_md_path = None

    # Read PRD and extract branch name.
    try:
        prd = read_prd(prd_json_path)
    except (json.JSONDecodeError, OSError) as e:
        print(f"Failed to read PRD: {e}", file=sys.stderr)
        sys.exit(1)

    branch = f"{prd_name}"
    if not branch:
        print("PRD missing 'branchName' field", file=sys.stderr)
        sys.exit(1)

    # Sync main and prune worktrees.
    try:
        sync_main(repo_root)
        prune_worktrees(repo_root)
    except RuntimeError as e:
        print(f"Git sync failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Create or reuse worktree.
    worktree_dir = repo_root / ".claude" / "worktrees" / normalize_branch(branch)
    try:
        reused = create_or_reuse_worktree(repo_root, branch, worktree_dir)
    except RuntimeError as e:
        print(f"Worktree setup failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Copy PRD files into worktree.
    try:
        dest_json, dest_md = copy_prd_files(prd_json_path, prd_md_path, worktree_dir)
    except OSError as e:
        print(f"Failed to copy PRD files: {e}", file=sys.stderr)
        sys.exit(1)

    # Output result.
    result = {
        "status": "ready",
        "worktree": str(worktree_dir),
        "branch": branch,
        "prd_path": str(dest_json),
        "prd_md_path": str(dest_md) if dest_md else None,
        "reused": reused,
    }
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
