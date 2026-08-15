#!/usr/bin/env python3
"""setup-workspace.py — Create or reuse a git worktree for a PRD.

Usage: python scripts/agents/setup-workspace.py <prd-name>

Reads tasks/todo/<prd-name>.json, uses <prd-name> as the branch/worktree name,
syncs main, creates or reuses a git worktree, copies the PRD (and optional .md)
into the worktree root, and prints a JSON result to stdout.

Exit 0 on success (stdout = JSON blob), exit 1 on failure (stderr = stage-specific error).
"""

import contextlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path


IMMUTABLE_OBJECT_ID = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$")
SNAPSHOT_REF_PREFIX = "refs/factory-snapshots/"
ROOT_SYNC_LOCK_FILENAME = "setup-workspace-root-sync.lock"
MAX_ANCESTOR_RESIDUE_PATHS = 20
_ROOT_SYNC_THREAD_LOCKS = {}
_ROOT_SYNC_THREAD_LOCKS_GUARD = threading.Lock()


def run_git(*args, cwd=None, check=True, env=None):
    """Run a git command, returning stdout. Raises on failure if check=True."""
    result = subprocess.run(
        ["git"] + list(args),
        cwd=cwd,
        capture_output=True,
        text=True,
        env=env,
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


def command_failure_details(result):
    """Return useful stderr/stdout text from a failed git command."""
    return (result.stderr or result.stdout).strip() or "no details available"


def require_git_output(result, description):
    """Return non-empty command output or raise a contextual error."""
    if result.returncode != 0:
        raise RuntimeError(f"{description}: {command_failure_details(result)}")
    output = result.stdout.strip()
    if not output:
        raise RuntimeError(f"{description}: git returned no object identifier")
    return output


def immutable_object_id(value):
    """Return True only for a complete SHA-1 or SHA-256 object identifier."""
    return bool(IMMUTABLE_OBJECT_ID.fullmatch(value))


def snapshot_ref_name(snapshot_id):
    """Return the private recovery ref for an immutable snapshot ID."""
    return f"{SNAPSHOT_REF_PREFIX}{snapshot_id}"


def anchor_snapshot(repo_path, snapshot_id):
    """Keep a captured snapshot reachable without touching the stash stack."""
    ref_name = snapshot_ref_name(snapshot_id)
    result = run_git(
        "update-ref", ref_name, snapshot_id, "",
        cwd=repo_path, check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"failed to anchor snapshot {snapshot_id} at {ref_name}: "
            f"{command_failure_details(result)}"
        )


def remove_snapshot_anchor(repo_path, snapshot_id, scope_label):
    """Delete only the expected private ref after a successful restore."""
    ref_name = snapshot_ref_name(snapshot_id)
    result = run_git(
        "update-ref", "-d", ref_name, snapshot_id,
        cwd=repo_path, check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"{scope_label} sync restored snapshot {snapshot_id}, but could "
            f"not remove its private recovery ref {ref_name}; the snapshot "
            f"remains anchored: {command_failure_details(result)}"
        )


def root_sync_lock_path(repo_path):
    """Return the persistent lock path shared by root-sync invocations."""
    common_dir = require_git_output(
        run_git(
            "rev-parse", "--git-common-dir", cwd=repo_path, check=False,
        ),
        "failed to resolve Git common directory for root sync lock",
    )
    common_path = Path(common_dir)
    if not common_path.is_absolute():
        common_path = Path(repo_path) / common_path
    return common_path.resolve() / ROOT_SYNC_LOCK_FILENAME


def root_sync_thread_lock(lock_path):
    """Return the in-process lock for one repository's root sync path."""
    lock_key = os.path.normcase(str(lock_path))
    with _ROOT_SYNC_THREAD_LOCKS_GUARD:
        return _ROOT_SYNC_THREAD_LOCKS.setdefault(lock_key, threading.Lock())


@contextlib.contextmanager
def root_sync_lock(repo_path):
    """Own root synchronization across threads and setup-workspace processes."""
    lock_path = root_sync_lock_path(repo_path)
    thread_lock = root_sync_thread_lock(lock_path)
    with thread_lock:
        lock_path.parent.mkdir(parents=True, exist_ok=True)
        with lock_path.open("a+b") as lock_file:
            lock_file.seek(0, os.SEEK_END)
            if lock_file.tell() == 0:
                lock_file.write(b"\0")
                lock_file.flush()

            if os.name == "nt":
                import msvcrt

                while True:
                    lock_file.seek(0)
                    try:
                        msvcrt.locking(lock_file.fileno(), msvcrt.LK_NBLCK, 1)
                        break
                    except OSError:
                        time.sleep(0.05)
            else:
                import fcntl

                fcntl.flock(lock_file.fileno(), fcntl.LOCK_EX)

            try:
                yield
            finally:
                if os.name == "nt":
                    import msvcrt

                    lock_file.seek(0)
                    msvcrt.locking(lock_file.fileno(), msvcrt.LK_UNLCK, 1)
                else:
                    import fcntl

                    fcntl.flock(lock_file.fileno(), fcntl.LOCK_UN)


def temporary_index_path():
    """Create a path for a temporary Git index without leaving an empty file."""
    file_descriptor, path = tempfile.mkstemp(prefix="setup-workspace-index-")
    os.close(file_descriptor)
    temporary_path = Path(path)
    temporary_path.unlink()
    return temporary_path


def git_index_path(repo_path):
    """Resolve the worktree-specific index path used by Git."""
    result = run_git("rev-parse", "--git-path", "index", cwd=repo_path)
    index_path = Path(result.stdout.strip())
    if not index_path.is_absolute():
        index_path = repo_path / index_path
    return index_path


def environment_with_index(index_path):
    """Return an environment that directs Git commands to index_path."""
    environment = os.environ.copy()
    environment["GIT_INDEX_FILE"] = str(index_path)
    return environment


def working_tree_tree(repo_path):
    """Write the tracked working-tree state to an immutable tree object."""
    temporary_path = temporary_index_path()
    try:
        source_index = git_index_path(repo_path)
        if source_index.exists():
            shutil.copy2(source_index, temporary_path)
        else:
            run_git(
                "read-tree", "HEAD", cwd=repo_path,
                env=environment_with_index(temporary_path),
            )

        environment = environment_with_index(temporary_path)
        run_git("add", "-u", cwd=repo_path, env=environment)
        return require_git_output(
            run_git("write-tree", cwd=repo_path, env=environment),
            "failed to capture the tracked working tree",
        )
    finally:
        temporary_path.unlink(missing_ok=True)


def untracked_paths(repo_path):
    """Return non-ignored untracked paths as Git pathspec arguments."""
    result = run_git(
        "ls-files", "--others", "--exclude-standard", "-z",
        cwd=repo_path,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"failed to enumerate untracked files: {command_failure_details(result)}"
        )
    return [path for path in result.stdout.split("\0") if path]


def untracked_tree(repo_path, paths):
    """Write the captured untracked files to an immutable tree object."""
    temporary_path = temporary_index_path()
    try:
        environment = environment_with_index(temporary_path)
        run_git("read-tree", "--empty", cwd=repo_path, env=environment)
        run_git("add", "--", *paths, cwd=repo_path, env=environment)
        return require_git_output(
            run_git("write-tree", cwd=repo_path, env=environment),
            "failed to capture untracked files",
        )
    finally:
        temporary_path.unlink(missing_ok=True)


def commit_tree(repo_path, tree, parents, message):
    """Create a dangling commit that owns one part of a local snapshot."""
    args = ["commit-tree", tree]
    for parent in parents:
        args.extend(["-p", parent])
    args.extend(["-m", message])
    object_id = require_git_output(
        run_git(*args, cwd=repo_path, check=False),
        f"failed to create snapshot object for {message}",
    )
    if not immutable_object_id(object_id):
        raise RuntimeError(
            f"failed to create snapshot object for {message}: "
            f"invalid object identifier {object_id!r}"
        )
    return object_id


def clear_local_changes(repo_path, snapshot_id):
    """Remove the captured state from the worktree before synchronization."""
    for args in (("reset", "--hard", "HEAD"), ("clean", "-fd")):
        result = run_git(*args, cwd=repo_path, check=False)
        if result.returncode != 0:
            raise RuntimeError(
                f"failed to clear local changes for snapshot {snapshot_id}: "
                f"{command_failure_details(result)}"
            )


def stash_local_changes(repo_path, label):
    """Capture local changes in a stack-independent commit snapshot.

    The snapshot has a working-tree commit, an index commit, and, when needed,
    an untracked-files commit as its third parent. It is never added to the
    shared refs/stash stack, so its complete object ID remains stable while
    other worktrees create ordinary stashes. A private recovery ref keeps the
    snapshot reachable until restoration succeeds.
    """
    if not working_tree_has_local_changes(repo_path):
        return None

    try:
        head = require_git_output(
            run_git("rev-parse", "HEAD", cwd=repo_path, check=False),
            "failed to resolve HEAD while capturing local changes",
        )
        index_tree = require_git_output(
            run_git("write-tree", cwd=repo_path, check=False),
            "failed to capture the staged index",
        )
        index_commit = commit_tree(
            repo_path,
            index_tree,
            [head],
            f"index for {label}",
        )
        tracked_tree = working_tree_tree(repo_path)
        paths = untracked_paths(repo_path)
        parents = [head, index_commit]
        if paths:
            untracked_commit = commit_tree(
                repo_path,
                untracked_tree(repo_path, paths),
                [],
                f"untracked files for {label}",
            )
            parents.append(untracked_commit)
        snapshot_id = commit_tree(repo_path, tracked_tree, parents, label)
        anchor_snapshot(repo_path, snapshot_id)
    except (OSError, RuntimeError) as error:
        raise RuntimeError(
            f"failed to capture local changes as snapshot: {error}"
        ) from error

    try:
        clear_local_changes(repo_path, snapshot_id)
    except RuntimeError as error:
        raise RuntimeError(
            f"failed to prepare synchronization with snapshot {snapshot_id}: {error}"
        ) from error

    return snapshot_id


def verify_snapshot(repo_path, snapshot_id, scope_label):
    """Verify that snapshot_id is a complete commit object before applying it."""
    if not immutable_object_id(snapshot_id):
        raise RuntimeError(
            f"{scope_label} sync could not verify snapshot {snapshot_id}: "
            "the identifier is not a complete immutable object ID"
        )

    result = run_git(
        "cat-file", "-e", f"{snapshot_id}^{{commit}}",
        cwd=repo_path,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"{scope_label} sync could not verify snapshot {snapshot_id}: "
            f"{command_failure_details(result)}"
        )


def restore_stashed_changes(repo_path, snapshot_id, scope_label):
    """Restore a snapshot by object ID and preserve its recovery ref on failure."""
    if snapshot_id is None:
        return

    verify_snapshot(repo_path, snapshot_id, scope_label)
    apply_result = run_git(
        "stash", "apply", "--index", snapshot_id,
        cwd=repo_path, check=False,
    )
    if apply_result.returncode != 0:
        fallback_result = run_git(
            "stash", "apply", snapshot_id,
            cwd=repo_path, check=False,
        )
        if fallback_result.returncode != 0:
            details = (fallback_result.stderr or fallback_result.stdout).strip()
            if not details:
                details = (apply_result.stderr or apply_result.stdout).strip()
            # A failed apply leaves a half-applied, possibly conflicted tree.
            # Unmerged index entries make every later snapshot capture fail,
            # which wedges all future workspace setups until a human cleans
            # the root checkout. Reset back to a clean tree; the snapshot
            # object still holds the full residue. `git clean` honors
            # .gitignore, so ignored operator files (docs/temp, tasks/todo)
            # are untouched.
            cleanup_details = []
            for args in (("reset", "--hard", "HEAD"), ("clean", "-fd")):
                cleanup_result = run_git(*args, cwd=repo_path, check=False)
                if cleanup_result.returncode != 0:
                    cleanup_details.append(command_failure_details(cleanup_result))
            if cleanup_details:
                details = f"{details}; cleanup also failed: {'; '.join(cleanup_details)}"
            raise RuntimeError(
                f"{scope_label} sync succeeded, but restoring snapshot "
                f"{snapshot_id} failed; the snapshot was preserved with the "
                f"unrestored changes: {details}"
            )

    remove_snapshot_anchor(repo_path, snapshot_id, scope_label)


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


def ancestor_commit_trees(repo_path, head_sha):
    """Return reachable ancestor commit/tree pairs without per-commit Git calls."""
    result = run_git(
        "log", "--format=%H%x00%T", "--no-decorate", head_sha,
        cwd=repo_path, check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"root main sync could not inspect ancestor trees of {head_sha}: "
            f"{command_failure_details(result)}"
        )

    commit_trees = []
    for line in result.stdout.splitlines():
        values = line.split("\0")
        if len(values) != 2 or not all(immutable_object_id(value) for value in values):
            raise RuntimeError(
                f"root main sync received an invalid ancestor commit/tree pair for "
                f"{head_sha}: {line!r}"
            )
        commit, tree = values
        if commit != head_sha:
            commit_trees.append((commit, tree))
    return commit_trees


def tree_for_revision(repo_path, revision, description):
    """Return an immutable tree ID for a Git revision."""
    return require_git_output(
        run_git("rev-parse", f"{revision}^{{tree}}", cwd=repo_path, check=False),
        description,
    )


def tracked_path_summary(repo_path, head_sha, ancestor_sha):
    """Return a bounded path-only summary of the current/ancestor difference."""
    result = run_git(
        "diff", "--no-ext-diff", "--no-renames", "--name-status",
        head_sha, ancestor_sha, cwd=repo_path, check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"root main sync could not summarize ancestor residue between "
            f"{head_sha} and {ancestor_sha}: {command_failure_details(result)}"
        )

    paths = [line for line in result.stdout.splitlines() if line]
    if not paths:
        return "tracked tree differs without a named path"
    if len(paths) > MAX_ANCESTOR_RESIDUE_PATHS:
        remaining = len(paths) - MAX_ANCESTOR_RESIDUE_PATHS
        paths = [*paths[:MAX_ANCESTOR_RESIDUE_PATHS], f"... ({remaining} more paths)"]
    return ", ".join(paths)


def verify_no_ancestor_residue(repo_path):
    """Reject tracked checkout state that exactly matches an ancestor tree."""
    head_sha = require_git_output(
        run_git("rev-parse", "HEAD", cwd=repo_path, check=False),
        "root main sync could not resolve current HEAD",
    )
    head_tree = tree_for_revision(
        repo_path, head_sha, "root main sync could not capture current HEAD tree",
    )
    index_tree = require_git_output(
        run_git("write-tree", cwd=repo_path, check=False),
        "root main sync could not capture the synchronized index tree",
    )
    worktree_tree = working_tree_tree(repo_path)

    local_states = []
    if index_tree != head_tree:
        local_states.append(("index", index_tree))
    if worktree_tree != head_tree:
        local_states.append(("working tree", worktree_tree))
    if not local_states:
        return

    ancestor_trees = ancestor_commit_trees(repo_path, head_sha)
    findings = []
    for state_name, state_tree in local_states:
        for ancestor_sha, ancestor_tree in ancestor_trees:
            if state_tree == ancestor_tree:
                paths = tracked_path_summary(repo_path, head_sha, ancestor_sha)
                findings.append(
                    f"{state_name} tree {state_tree} matches ancestor "
                    f"{ancestor_sha} (tree {ancestor_tree}); paths: {paths}"
                )
                break

    if findings:
        raise RuntimeError(
            "root main post-sync ancestor-residue guard rejected tracked "
            f"local state while HEAD is {head_sha}: {' | '.join(findings)}. "
            "The checkout was not reset or cleaned; preserve it for recovery."
        )


def verify_root_after_restore(repo_root, snapshot_id):
    """Run the root residue guard and preserve a snapshot when it rejects."""
    try:
        verify_no_ancestor_residue(repo_root)
    except RuntimeError as error:
        if snapshot_id is None:
            raise RuntimeError(
                f"{error}; no root snapshot was captured for automatic recovery"
            ) from error

        recovery_ref = snapshot_ref_name(snapshot_id)
        try:
            anchor_snapshot(repo_root, snapshot_id)
        except RuntimeError as anchor_error:
            raise RuntimeError(
                f"{error}; could not preserve recovery snapshot {recovery_ref}: "
                f"{anchor_error}"
            ) from error
        raise RuntimeError(
            f"{error}; recovery snapshot preserved at {recovery_ref}"
        ) from error


def sync_checked_out_main_with_stash(repo_root, remote_sha):
    """Own, sync, and restore a checked-out root ``main`` invocation."""
    with root_sync_lock(repo_root):
        return _sync_checked_out_main_with_stash(repo_root, remote_sha)


def _sync_checked_out_main_with_stash(repo_root, remote_sha):
    """Temporarily snapshot local changes, sync main, then restore them."""
    snapshot_id = stash_local_changes(
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
        restore_stashed_changes(repo_root, snapshot_id, "root main")

    verify_root_after_restore(repo_root, snapshot_id)

    if snapshot_id is None:
        return f"synced checked-out main to {remote_sha[:8]}"
    return (
        f"stashed local changes in snapshot {snapshot_id[:8]}, "
        f"synced checked-out main to {remote_sha[:8]}, then restored the snapshot"
    )


def sync_main(repo_root):
    """Own one complete root-main synchronization invocation."""
    with root_sync_lock(repo_root):
        return _sync_main(repo_root)


def _sync_main(repo_root):
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
        if current_branch(repo_root) == "main":
            verify_root_after_restore(repo_root, None)
        return "already up to date"

    if not can_fast_forward_main(repo_root, local_sha, remote_sha):
        return "skipped (local main is not a fast-forward behind origin/main)"

    if current_branch(repo_root) == "main":
        return _sync_checked_out_main_with_stash(repo_root, remote_sha)

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
    snapshot_id = stash_local_changes(
        worktree_path,
        f"setup-workspace worktree sync {branch}",
    )
    try:
        pull_result = run_git("pull", "--ff-only", cwd=worktree_path, check=False)
        if pull_result.returncode == 0:
            confirm_worktree_upstream_head(worktree_path, branch, upstream_ref)
            if snapshot_id is not None:
                return (
                    f"stashed local changes in snapshot {snapshot_id[:8]}, "
                    "fast-forwarded from upstream, then restored the snapshot"
                )
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
        if snapshot_id is None:
            return "fetch/reset --hard to upstream after pull --ff-only failed"
        return (
            f"stashed local changes in snapshot {snapshot_id[:8]}, then "
            "fetch/reset --hard to upstream after pull --ff-only failed"
        )
    finally:
        restore_stashed_changes(worktree_path, snapshot_id, f"worktree branch {branch}")


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


# Operator-authored standing rules that lane payloads reference by this exact
# repo-relative path. The file lives under the gitignored docs/temp tree, so a
# fresh worktree never contains it and the payload pointer would dangle. Copy it
# in alongside the PRD so every lane can actually read its own rules.
STANDING_RULES_RELPATH = Path("docs") / "temp" / "scale-program-rules.md"


def copy_standing_rules(repo_root, worktree_path):
    """Copy the operator standing-rules doc into the worktree, if it exists.

    Returns the destination path, or None when the source is absent. Never
    fatal: a missing or unreadable rules doc must not block workspace setup.
    """
    source = repo_root / STANDING_RULES_RELPATH
    if not source.is_file():
        return None

    dest = worktree_path / STANDING_RULES_RELPATH
    try:
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(str(source), str(dest))
    except OSError as e:
        print(f"Standing-rules copy skipped: {e}", file=sys.stderr)
        return None

    return dest


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

    # Copy the operator standing-rules doc referenced by lane payloads.
    dest_rules = copy_standing_rules(repo_root, worktree_dir)

    # Output result.
    result = {
        "status": "ready",
        "worktree": str(worktree_dir),
        "branch": branch,
        "prd_path": str(dest_json),
        "prd_md_path": str(dest_md) if dest_md else None,
        "standing_rules_path": str(dest_rules) if dest_rules else None,
        "reused": reused,
    }
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
