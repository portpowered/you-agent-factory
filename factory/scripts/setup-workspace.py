#!/usr/bin/env python3
"""setup-workspace.py — Create or reuse a git worktree for a PRD.

Usage: python scripts/agents/setup-workspace.py <prd-name>

Reads tasks/todo/<prd-name>.json, uses <prd-name> as the branch/worktree name,
syncs main, creates or reuses a git worktree, copies the PRD (and optional .md)
into the worktree root, and prints a JSON result to stdout.

Exit 0 on success (stdout = JSON blob), exit 1 on failure (stderr = stage-specific error).
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


def current_branch(repo_path):
    """Return the currently checked-out branch name, or empty when detached."""
    return run_git(
        "branch", "--show-current", cwd=repo_path, check=False,
    ).stdout.strip()


def working_tree_has_local_changes(repo_path):
    """True when the working tree has staged, unstaged, or untracked changes."""
    status = run_git("status", "--porcelain", cwd=repo_path, check=False)
    return bool(status.stdout.strip())


def stash_local_changes(repo_path, label):
    """Stash local changes and return the top stash ref, or None when clean."""
    if not working_tree_has_local_changes(repo_path):
        return None

    result = run_git(
        "stash", "push", "--include-untracked", "--message", label,
        cwd=repo_path, check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            "failed to stash local changes: "
            f"{(result.stderr or result.stdout).strip()}"
        )

    return "stash@{0}"


def restore_stashed_changes(repo_path, stash_ref, scope_label):
    """Restore a stashed worktree and keep the stash entry on failure."""
    if stash_ref is None:
        return

    apply_result = run_git(
        "stash", "apply", "--index", stash_ref,
        cwd=repo_path, check=False,
    )
    if apply_result.returncode != 0:
        fallback_result = run_git(
            "stash", "apply", stash_ref,
            cwd=repo_path, check=False,
        )
        if fallback_result.returncode != 0:
            details = (fallback_result.stderr or fallback_result.stdout).strip()
            if not details:
                details = (apply_result.stderr or apply_result.stdout).strip()
            raise RuntimeError(
                f"{scope_label} sync succeeded, but restoring stashed changes failed; "
                f"{stash_ref} was preserved: {details}"
            )

    drop_result = run_git("stash", "drop", stash_ref, cwd=repo_path, check=False)
    if drop_result.returncode != 0:
        raise RuntimeError(
            f"{scope_label} sync restored local changes, but failed to drop "
            f"{stash_ref}: {(drop_result.stderr or drop_result.stdout).strip()}"
        )


def read_prd(prd_path):
    """Read and parse a PRD JSON file. Returns the parsed dict."""
    with open(prd_path, "r", encoding="utf-8") as f:
        return json.load(f)


def has_origin_remote(repo_root):
    """Return True when an origin remote is configured."""
    result = run_git("remote", "get-url", "origin", cwd=repo_root, check=False)
    return result.returncode == 0


def origin_main_ref_exists(repo_root):
    """Return True when refs/remotes/origin/main exists locally."""
    result = run_git(
        "rev-parse", "--verify", "refs/remotes/origin/main",
        cwd=repo_root, check=False,
    )
    return result.returncode == 0


def local_main_ref_exists(repo_root):
    """Return True when refs/heads/main exists locally."""
    result = run_git(
        "rev-parse", "--verify", "refs/heads/main",
        cwd=repo_root, check=False,
    )
    return result.returncode == 0


def remote_main_sha(repo_root):
    """Return origin/main sha from ls-remote, or None when missing/unreachable."""
    result = run_git(
        "ls-remote", "--exit-code", "origin", "refs/heads/main",
        cwd=repo_root, check=False,
    )
    if result.returncode != 0 or not result.stdout.strip():
        return None
    return result.stdout.strip().split()[0]


def stale_origin_main_sha(repo_root):
    """Return local origin/main sha when the remote-tracking ref exists."""
    if not origin_main_ref_exists(repo_root):
        return None
    result = run_git(
        "rev-parse", "refs/remotes/origin/main",
        cwd=repo_root, check=False,
    )
    if result.returncode != 0:
        return None
    return result.stdout.strip()


def resolve_remote_main_sha(repo_root, fetch_succeeded):
    """Resolve the best available origin/main sha for root sync."""
    if fetch_succeeded:
        return remote_main_sha(repo_root)
    return stale_origin_main_sha(repo_root)


def can_fast_forward_main(repo_root, local_sha, remote_sha):
    """True when remote_sha is a strict fast-forward of local_sha."""
    if local_sha == remote_sha:
        return False
    merge_base = run_git(
        "merge-base", local_sha, remote_sha,
        cwd=repo_root, check=False,
    )
    return merge_base.returncode == 0 and merge_base.stdout.strip() == local_sha


def confirm_ref_matches(repo_path, ref_name, expected_sha):
    """Raise when ref_name does not resolve to expected_sha."""
    resolved_sha = run_git("rev-parse", ref_name, cwd=repo_path).stdout.strip()
    if resolved_sha != expected_sha:
        raise RuntimeError(
            f"{ref_name} resolved to {resolved_sha[:8]}, expected {expected_sha[:8]}"
        )


def sync_checked_out_main_with_stash(repo_root, remote_sha):
    """Temporarily stash local changes, sync main, then restore the stash."""
    stash_ref = stash_local_changes(
        repo_root,
        "setup-workspace root sync",
    )
    try:
        pull_result = run_git("pull", "--ff-only", cwd=repo_root, check=False)
        if pull_result.returncode != 0:
            run_git("fetch", "origin", cwd=repo_root)
            run_git("reset", "--hard", remote_sha, cwd=repo_root)
        confirm_ref_matches(repo_root, "HEAD", remote_sha)
        confirm_ref_matches(repo_root, "refs/heads/main", remote_sha)
    finally:
        restore_stashed_changes(repo_root, stash_ref, "root main")

    return (
        f"stashed local changes, synced checked-out main to {remote_sha[:8]}, "
        "then restored the stash"
    )


def sync_main(repo_root):
    """Best-effort root main sync without disturbing the working tree.

    Uses fetch plus refs/heads/main fast-forward when safe instead of git pull,
    so dirty-root checkouts can continue workspace setup from local state.
    Returns a human-readable outcome string for logging.
    """
    if not has_origin_remote(repo_root):
        if local_main_ref_exists(repo_root):
            return "skipped (no origin remote)"
        raise RuntimeError(
            "no origin remote and refs/heads/main is missing"
        )

    fetch_result = run_git("fetch", "origin", cwd=repo_root, check=False)
    fetch_succeeded = fetch_result.returncode == 0
    if not fetch_succeeded:
        if local_main_ref_exists(repo_root):
            return f"skipped (fetch failed: {fetch_result.stderr.strip()})"
        if not origin_main_ref_exists(repo_root):
            raise RuntimeError(
                "fetch failed and refs/heads/main is missing: "
                f"{fetch_result.stderr.strip()}"
            )

    remote_sha = resolve_remote_main_sha(repo_root, fetch_succeeded)
    if remote_sha is None:
        if local_main_ref_exists(repo_root):
            return "skipped (origin has no main branch)"
        raise RuntimeError(
            "origin has no main branch and refs/heads/main is missing"
        )

    if not local_main_ref_exists(repo_root):
        run_git("update-ref", "refs/heads/main", remote_sha, cwd=repo_root)
        return f"created refs/heads/main at {remote_sha[:8]}"

    local_sha = run_git("rev-parse", "refs/heads/main", cwd=repo_root).stdout.strip()
    if local_sha == remote_sha:
        return "already up to date"

    if not can_fast_forward_main(repo_root, local_sha, remote_sha):
        return "skipped (local main is not a fast-forward behind origin/main)"

    if current_branch(repo_root) == "main" and working_tree_has_local_changes(repo_root):
        return sync_checked_out_main_with_stash(repo_root, remote_sha)

    run_git("update-ref", "refs/heads/main", remote_sha, cwd=repo_root)
    return (
        f"fast-forwarded refs/heads/main to {remote_sha[:8]} "
        "(fetch-only; did not run git pull)"
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


def branch_upstream_ref(git_dir, branch):
    """Return upstream ref for branch, or None when no upstream is configured."""
    result = run_git(
        "rev-parse", "--abbrev-ref", f"{branch}@{{upstream}}",
        cwd=git_dir, check=False,
    )
    if result.returncode != 0:
        return None
    upstream = result.stdout.strip()
    return upstream or None


def confirm_worktree_upstream_head(worktree_path, branch, upstream_ref):
    """Raise when branch or HEAD does not match the resolved upstream sha."""
    upstream_sha = run_git("rev-parse", upstream_ref, cwd=worktree_path).stdout.strip()
    confirm_ref_matches(worktree_path, "HEAD", upstream_sha)
    confirm_ref_matches(worktree_path, f"refs/heads/{branch}", upstream_sha)


def sync_reused_worktree_branch(repo_root, worktree_path, branch):
    """Checkout branch in a reused worktree and fast-forward when safe.

    No-upstream and missing-remote-branch conditions are non-fatal. Unsafe
    fast-forward failures raise RuntimeError for worktree-preparation reporting.
    Returns a human-readable outcome string for logging.
    """
    run_git("checkout", branch, cwd=worktree_path)

    if branch_upstream_ref(worktree_path, branch) is None:
        return "skipped (no upstream configured)"

    if not branch_exists_on_remote(repo_root, branch):
        return "skipped (branch has no origin ref)"

    upstream_ref = branch_upstream_ref(worktree_path, branch)
    stash_ref = stash_local_changes(
        worktree_path,
        f"setup-workspace worktree sync {branch}",
    )
    try:
        pull_result = run_git("pull", "--ff-only", cwd=worktree_path, check=False)
        if pull_result.returncode == 0:
            confirm_worktree_upstream_head(worktree_path, branch, upstream_ref)
            if stash_ref is not None:
                return "stashed local changes, fast-forwarded from upstream, then restored the stash"
            return "fast-forwarded from upstream"

        stderr = pull_result.stderr.strip()
        lowered = stderr.lower()
        if "no tracking information" in lowered:
            return "skipped (no upstream configured)"

        run_git("fetch", "origin", cwd=worktree_path)
        local_sha = run_git("rev-parse", f"refs/heads/{branch}", cwd=worktree_path).stdout.strip()
        upstream_sha = run_git("rev-parse", upstream_ref, cwd=worktree_path).stdout.strip()
        if not can_fast_forward_main(worktree_path, local_sha, upstream_sha):
            raise RuntimeError(
                f"worktree branch update failed for {branch}: {stderr}"
            )

        run_git("reset", "--hard", upstream_ref, cwd=worktree_path)
        confirm_worktree_upstream_head(worktree_path, branch, upstream_ref)
        return (
            "stashed local changes, then fetch/reset --hard to upstream "
            "after pull --ff-only failed"
        )
    finally:
        restore_stashed_changes(worktree_path, stash_ref, f"worktree branch {branch}")


def create_or_reuse_worktree(repo_root, branch, worktree_path):
    """Create a new worktree or reuse an existing one. Returns reused flag."""
    if worktree_path.exists() and worktree_is_valid(worktree_path):
        sync_outcome = sync_reused_worktree_branch(repo_root, worktree_path, branch)
        print(f"Worktree branch sync: {sync_outcome}", file=sys.stderr)
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

    # Read the PRD to catch malformed input; the branch name is the work item name.
    try:
        read_prd(prd_json_path)
    except (json.JSONDecodeError, OSError) as e:
        print(f"Failed to read PRD: {e}", file=sys.stderr)
        sys.exit(1)

    branch = f"{prd_name}"
    if not branch:
        print("PRD name must not be empty", file=sys.stderr)
        sys.exit(1)

    # Sync main and prune worktrees.
    try:
        sync_outcome = sync_main(repo_root)
        print(f"Root sync: {sync_outcome}", file=sys.stderr)
        prune_worktrees(repo_root)
    except RuntimeError as e:
        print(f"Root sync failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Create or reuse worktree.
    worktree_dir = repo_root / ".claude" / "worktrees" / normalize_branch(branch)
    try:
        reused = create_or_reuse_worktree(repo_root, branch, worktree_dir)
    except RuntimeError as e:
        print(f"Worktree preparation failed: {e}", file=sys.stderr)
        sys.exit(1)

    # Copy PRD files into worktree.
    try:
        dest_json, dest_md = copy_prd_files(prd_json_path, prd_md_path, worktree_dir)
    except OSError as e:
        print(f"PRD copy failed: {e}", file=sys.stderr)
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
