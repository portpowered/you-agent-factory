#!/usr/bin/env python3
"""setup-workspace.py — Create or reuse a git worktree for a PRD.

Usage: python scripts/agents/setup-workspace.py <prd-name>

Reads the exact tasks/todo/<prd-name>.json packet from the main checkout or a
Git-registered worktree, uses <prd-name> as the branch/worktree name, syncs
main, creates or reuses a git worktree, copies the PRD (and optional .md) into
the worktree root, and prints a JSON result to stdout.

Exit 0 on success (stdout = JSON blob), exit 1 on failure (stderr = stage-specific error).
"""

import contextlib
import hashlib
import json
import os
import re
import shutil
import stat
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
MAX_DIRTY_ROOT_SAMPLE_ENTRIES = 12
MAX_DIRTY_ROOT_ATTRIBUTION_PATHS = 5
MAX_DIRTY_ROOT_ATTRIBUTION_WORKTREES = 20
MAX_CANDIDATE_DIAGNOSTIC_PATHS = 8
MAX_CANDIDATE_DIAGNOSTIC_PATH_LENGTH = 240
MAX_STATUS_FAILURE_DETAILS = 512
MAX_FAILURE_DETAILS = 1024
MAX_DISPLAYED_STATUS_PATH_LENGTH = 240
FILE_DIGEST_CHUNK_SIZE = 1024 * 1024
PREFLIGHT_VERSION = "factory-preflight.v1"
SHA256_HEX = re.compile(r"^[0-9a-f]{64}$")
PREFLIGHT_KEYS = frozenset(
    {
        "version",
        "projectRoot",
        "projectIdentity",
        "contractRevision",
        "authority",
        "intendedMainline",
    }
)
AUTHORITY_FIELDS = ("sourcePlan", "request", "acceptance")
FILE_DESCRIPTOR_KEYS = frozenset({"path", "identity", "sha256"})
WINDOWS_RESERVED_PATH_COMPONENT = re.compile(
    r"(?<![a-z0-9])(?:nul|con|prn|aux|com[1-9]|lpt[1-9])"
    r"(?:\.[^\\/:*?\"<>|\r\n]*)?(?![a-z0-9])",
    re.IGNORECASE,
)
_ROOT_SYNC_THREAD_LOCKS = {}
_ROOT_SYNC_THREAD_LOCKS_GUARD = threading.Lock()
_MISSING_PATH = object()
_UNAVAILABLE_PATH = object()


class FileReadFailure(RuntimeError):
    """A bounded file-read failure with the last safe byte position."""

    def __init__(self, position):
        super().__init__("declared input could not be read")
        self.position = position


class PacketPreflightError(RuntimeError):
    """A stable, bounded diagnostic for a rejected v1 task packet."""

    def __init__(
        self,
        category,
        code,
        field,
        *,
        path=None,
        identity=None,
        expected=None,
        observed=None,
        required_commit=None,
        resolved_head=None,
        next_condition,
    ):
        super().__init__(code)
        self.category = category
        self.code = code
        self.field = field
        self.path = path
        self.identity = identity
        self.expected = expected
        self.observed = observed
        self.required_commit = required_commit
        self.resolved_head = resolved_head
        self.next_condition = next_condition

    def __str__(self):
        parts = [
            "input-preflight",
            f"category={self.category}",
            f"code={self.code}",
            f"field={self.field}",
        ]
        if self.path is not None:
            parts.append(f"path={safe_diagnostic_value(self.path)}")
        if self.identity is not None:
            parts.append(
                f"identity={safe_digest_identity(self.identity)}"
            )
        renderer = (
            safe_contract_value
            if self.code == "contract-mismatch"
            else safe_digest_value
        )
        if self.expected is not None:
            parts.append(f"expected={renderer(self.expected)}")
        if self.observed is not None:
            parts.append(f"observed={renderer(self.observed)}")
        if self.required_commit is not None:
            parts.append(
                f"requiredCommit={safe_commit_value(self.required_commit)}"
            )
        if self.resolved_head is not None:
            parts.append(
                f"resolvedCheckoutHead={safe_commit_value(self.resolved_head)}"
            )
        parts.append(f"next={safe_diagnostic_value(self.next_condition)}")
        return bounded_failure_details(" ".join(parts))


class DirtyRootError(RuntimeError):
    """A bounded, already-rendered diagnostic for an operator-owned root."""


class RootStatusError(RuntimeError):
    """A root status inspection failure that belongs to the setup preflight."""


class RootSyncResult(str):
    """Describe root synchronization and its optional fresh remote baseline."""

    def __new__(cls, message, fresh_origin_main_sha=None):
        result = super().__new__(cls, message)
        result.fresh_origin_main_sha = fresh_origin_main_sha
        return result


def raw_failure_details(error):
    """Return an exception's text without allowing rendering to raise again."""
    try:
        return str(error).strip()
    except Exception as render_error:  # noqa: BLE001 - defensive boundary
        return (
            f"{type(error).__name__} details could not be rendered: "
            f"{type(render_error).__name__}"
        )


def bounded_failure_details(value, limit=MAX_FAILURE_DETAILS):
    """Safely render and bound command or exception details for the terminal."""
    details = (
        raw_failure_details(value)
        if not isinstance(value, str)
        else value.strip()
    )
    rendered = []
    for character in details:
        codepoint = ord(character)
        if character == "\n":
            rendered.append("\n")
        elif character == "\t":
            rendered.append("\\t")
        elif character == "\r":
            rendered.append("\\r")
        elif (
            codepoint < 0x20
            or codepoint == 0x7F
            or 0xD800 <= codepoint <= 0xDFFF
        ):
            rendered.append(f"\\u{codepoint:04x}")
        else:
            rendered.append(character)

    result = "".join(rendered)
    if len(result) <= limit:
        return result
    omitted = len(result) - limit
    suffix = f"... ({omitted} more characters)"
    return f"{result[: max(0, limit - len(suffix))]}{suffix}"


def safe_diagnostic_value(value):
    """Render one bounded diagnostic value without exposing raw newlines."""
    return bounded_failure_details(
        json.dumps(str(value), ensure_ascii=True),
        MAX_CANDIDATE_DIAGNOSTIC_PATH_LENGTH,
    )


def safe_digest_value(value):
    """Render only bounded digest/status values in packet diagnostics."""
    if isinstance(value, str) and (
        value
        in {
            "missing",
            "unreadable",
            "invalid",
            "partial",
            "relative",
            "symlink",
            "non-regular",
            "unavailable",
        }
        or bool(SHA256_HEX.fullmatch(value))
    ):
        return safe_diagnostic_value(value)
    return safe_diagnostic_value("invalid")


def safe_contract_value(value):
    """Render bounded project/revision evidence without arbitrary packet text."""
    if (
        isinstance(value, str)
        and value
        and len(value) <= 120
        and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.:/-]*", value)
    ):
        return safe_diagnostic_value(value)
    return safe_diagnostic_value("invalid")


def safe_digest_identity(value):
    """Render a declared identity only when it has the expected safe shape."""
    if (
        isinstance(value, str)
        and value.startswith("sha256:")
        and bool(SHA256_HEX.fullmatch(value[len("sha256:") :]))
    ):
        return safe_diagnostic_value(value)
    return safe_diagnostic_value("invalid")


def safe_commit_value(value):
    """Render only complete immutable commit identities."""
    if value == "missing":
        return safe_diagnostic_value(value)
    if isinstance(value, str) and immutable_object_id(value):
        return safe_diagnostic_value(value)
    return safe_diagnostic_value("invalid")


def is_platform_invalid_path_failure(details):
    """Recognize Git and Windows messages for paths unavailable to the host."""
    lowered = details.casefold()
    if "\x00" in lowered or "\\u0000" in lowered:
        return True
    if "invalid path" in lowered or "invalid filename" in lowered:
        return True
    if "path" in lowered and "invalid" in lowered:
        return True

    has_reserved_component = bool(WINDOWS_RESERVED_PATH_COMPONENT.search(details))
    if not has_reserved_component:
        return False
    return any(
        marker in lowered
        for marker in (
            "not allowed",
            "not valid",
            "cannot create",
            "could not create",
            "checkout",
            "worktree",
            "windows",
            "win32",
        )
    )


def format_stage_failure(stage, error):
    """Render bounded, actionable failure text while preserving the stage."""
    raw_details = raw_failure_details(error)
    details = bounded_failure_details(raw_details)
    if not details:
        details = f"unexpected {type(error).__name__} without additional details"

    if is_platform_invalid_path_failure(raw_details):
        return (
            f"{stage}: Git rejected a Windows-reserved or otherwise "
            "platform-invalid path (for example, the literal NUL device name). "
            "Inspect the reported path, manually back up any needed content, "
            "then remove or rename that path before retrying; setup-workspace "
            f"does not modify it automatically. Details: {details}"
        )
    return f"{stage}: {details}"


def run_git(*args, cwd=None, check=True, env=None):
    """Run a git command, returning stdout. Raises on failure if check=True."""
    result = subprocess.run(
        ["git"] + list(args),
        cwd=cwd,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="surrogateescape",
        env=env,
    )
    if check and result.returncode != 0:
        raise RuntimeError(
            f"git {bounded_failure_details(' '.join(args))} failed "
            f"(exit {result.returncode}): "
            f"{command_failure_details(result)}"
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


def truncate_status_details(details):
    """Keep status-command failures useful without emitting unbounded output."""
    return bounded_failure_details(details, MAX_STATUS_FAILURE_DETAILS)


def parse_porcelain_status(output):
    """Parse NUL-delimited porcelain-v1 status entries safely."""
    entries = []
    tokens = output.split("\0")
    token_index = 0
    while token_index < len(tokens):
        token = tokens[token_index]
        token_index += 1
        if not token:
            continue
        if len(token) < 4 or token[2] != " ":
            raise RuntimeError(
                "git status returned malformed porcelain output"
            )

        status = token[:2]
        paths = [token[3:]]
        if "R" in status or "C" in status:
            if token_index >= len(tokens) or not tokens[token_index]:
                raise RuntimeError(
                    "git status returned an incomplete rename or copy entry"
                )
            paths.append(tokens[token_index])
            token_index += 1
        entries.append({"status": status, "paths": tuple(paths)})

    return entries


def repository_status_entries(repo_path):
    """Return non-ignored root status entries, or raise a bounded failure."""
    try:
        result = run_git(
            "status",
            "--porcelain=v1",
            "-z",
            "--untracked-files=all",
            cwd=repo_path,
            check=False,
        )
    except OSError as error:
        raise RuntimeError(f"could not run git status: {error}") from error

    if result.returncode != 0:
        details = truncate_status_details(command_failure_details(result))
        raise RuntimeError(
            "git status --porcelain=v1 failed "
            f"(exit {result.returncode}): {details}"
        )

    try:
        return parse_porcelain_status(result.stdout)
    except RuntimeError as error:
        raise RuntimeError(f"could not inspect repository status: {error}") from error


def status_entry_kind(entry):
    """Return the operator-facing category for one porcelain status entry."""
    return "untracked" if entry["status"] == "??" else "tracked"


def status_entry_sort_key(entry):
    """Return a stable sort key independent of Git's presentation order."""
    kind_order = 0 if status_entry_kind(entry) == "tracked" else 1
    return kind_order, entry["paths"], entry["status"]


def status_entry_path(entry):
    """Render one status path as a safely escaped, bounded display value."""
    paths = entry["paths"]
    if "R" in entry["status"] or "C" in entry["status"]:
        paths = tuple(reversed(paths))

    rendered_paths = []
    for path in paths:
        if len(path) > MAX_DISPLAYED_STATUS_PATH_LENGTH:
            path = (
                path[: MAX_DISPLAYED_STATUS_PATH_LENGTH - 1]
                + "…"
            )
        rendered_paths.append(json.dumps(path, ensure_ascii=True))
    return " -> ".join(rendered_paths)


def dirty_root_sample(entries):
    """Select a stable bounded sample while retaining both status categories."""
    grouped = {
        "tracked": sorted(
            (entry for entry in entries if status_entry_kind(entry) == "tracked"),
            key=status_entry_sort_key,
        ),
        "untracked": sorted(
            (entry for entry in entries if status_entry_kind(entry) == "untracked"),
            key=status_entry_sort_key,
        ),
    }

    sample = [group[0] for group in grouped.values() if group]
    remaining = [
        entry
        for entry in entries
        if all(entry is not selected for selected in sample)
    ]
    remaining.sort(key=status_entry_sort_key)
    sample.extend(remaining[: max(0, MAX_DIRTY_ROOT_SAMPLE_ENTRIES - len(sample))])
    sample.sort(key=status_entry_sort_key)
    return sample


def path_content_digest(path):
    """Return a bounded-memory digest for a file, or a path-state sentinel."""
    try:
        if not path.exists():
            return _MISSING_PATH
        if not path.is_file():
            return _UNAVAILABLE_PATH

        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for chunk in iter(
                lambda: stream.read(FILE_DIGEST_CHUNK_SIZE),
                b"",
            ):
                digest.update(chunk)
        return digest.digest()
    except (OSError, ValueError):
        return _UNAVAILABLE_PATH


def git_revision_path_digest(repo_path, revision, relative_path):
    """Return a digest for a revision path without mutating the repository."""
    try:
        result = run_git(
            "show",
            f"{revision}:{relative_path}",
            cwd=repo_path,
            check=False,
        )
    except Exception:  # noqa: BLE001 - attribution must never replace refusal
        return _UNAVAILABLE_PATH

    if result.returncode != 0:
        return _MISSING_PATH
    try:
        contents = result.stdout.encode("utf-8", "surrogateescape")
    except (AttributeError, UnicodeEncodeError):
        return _UNAVAILABLE_PATH
    return hashlib.sha256(contents).digest()


def dirty_root_attribution_fingerprints(repo_path, entries):
    """Capture at most five root changes relative to the fetched baseline."""
    try:
        if not origin_main_ref_exists(repo_path):
            return []

        fingerprints = []
        for entry in dirty_root_sample(entries)[:MAX_DIRTY_ROOT_ATTRIBUTION_PATHS]:
            paths = entry.get("paths", ())
            if not paths:
                continue
            relative_path = paths[0]
            baseline = git_revision_path_digest(
                repo_path,
                "refs/remotes/origin/main",
                relative_path,
            )
            if baseline is _UNAVAILABLE_PATH:
                continue

            current = path_content_digest(repo_path / relative_path)
            if current is _UNAVAILABLE_PATH or current == baseline:
                continue
            fingerprints.append((relative_path, baseline, current))
        return fingerprints
    except Exception:  # noqa: BLE001 - attribution is best effort only
        return []


def sibling_worktree_candidates(repo_path):
    """Return a deterministic, bounded list of sibling directories."""
    worktrees_dir = repo_path / ".claude" / "worktrees"
    try:
        children = sorted(
            worktrees_dir.iterdir(),
            key=lambda path: os.fsencode(path.name),
        )
    except (OSError, UnicodeError, ValueError):
        return []

    candidates = []
    for candidate in children:
        try:
            if not candidate.is_dir():
                continue
        except OSError:
            continue
        candidates.append(candidate)
        if len(candidates) >= MAX_DIRTY_ROOT_ATTRIBUTION_WORKTREES:
            break
    return candidates


def is_valid_sibling_worktree(candidate):
    """Check one sibling independently so stale entries cannot abort refusal."""
    try:
        result = run_git(
            "rev-parse",
            "--show-toplevel",
            cwd=candidate,
            check=False,
        )
        if result.returncode != 0 or not result.stdout.strip():
            return False
        return Path(result.stdout.strip()).resolve() == candidate.resolve()
    except Exception:  # noqa: BLE001 - one broken candidate is non-fatal
        return False


def dirty_root_sibling_attribution(repo_path, entries):
    """Find bounded sibling worktrees carrying the same sampled root changes."""
    fingerprints = dirty_root_attribution_fingerprints(repo_path, entries)
    if not fingerprints:
        return []

    matches = []
    for candidate in sibling_worktree_candidates(repo_path):
        if not is_valid_sibling_worktree(candidate):
            continue

        matching_paths = []
        for relative_path, baseline, current in fingerprints:
            sibling_current = path_content_digest(candidate / relative_path)
            if sibling_current is _UNAVAILABLE_PATH:
                continue
            if sibling_current == current and sibling_current != baseline:
                matching_paths.append(relative_path)
        if matching_paths:
            matches.append((candidate.name, matching_paths))

    if not matches:
        return []

    lines = [
        "likely sibling worktree matches (same changes relative to origin/main):",
    ]
    for candidate_name, matching_paths in matches:
        rendered_paths = ", ".join(
            json.dumps(path, ensure_ascii=True)
            for path in matching_paths
        )
        lines.append(
            "  sibling worktree "
            f"{json.dumps(candidate_name, ensure_ascii=True)} "
            f"matches path(s): {rendered_paths}"
        )
    return lines


def dirty_root_diagnostic(repo_path, entries):
    """Describe dirty-root evidence and safe manual recovery guidance."""
    tracked_count = sum(
        status_entry_kind(entry) == "tracked" for entry in entries
    )
    untracked_count = len(entries) - tracked_count
    sample = dirty_root_sample(entries)
    omitted_count = len(entries) - len(sample)

    lines = [
        f"repository root is dirty: {repo_path}",
        "status counts: "
        f"total entries={len(entries)}; "
        f"tracked changes={tracked_count}; "
        f"untracked files={untracked_count}",
        "workspace setup stopped before root synchronization, snapshot "
        "capture, worktree preparation, pruning, or PRD copy",
        f"status sample (up to {MAX_DIRTY_ROOT_SAMPLE_ENTRIES} entries):",
    ]

    for kind in ("tracked", "untracked"):
        lines.append(f"  {kind}:")
        category_sample = [
            entry for entry in sample if status_entry_kind(entry) == kind
        ]
        if not category_sample:
            lines.append("    (none)")
            continue
        for entry in category_sample:
            lines.append(
                f"    {entry['status']} {status_entry_path(entry)}"
            )

    if omitted_count:
        lines.append(
            f"  {omitted_count} additional path(s) omitted from the sample"
        )
    lines.append(
        "Inspect the repository root manually, then commit the changes or "
        "back them up and restore them manually before retrying."
    )
    lines.extend(dirty_root_sibling_attribution(repo_path, entries))
    return "\n".join(lines)


def root_status_for_setup(repo_root):
    """Inspect root dirt after fetch while preserving the preflight failure stage."""
    try:
        return repository_status_entries(repo_root)
    except Exception as error:  # noqa: BLE001 - preserve the CLI stage boundary
        raise RootStatusError(str(error)) from error


def command_failure_details(result):
    """Return useful stderr/stdout text from a failed git command."""
    return (
        bounded_failure_details(result.stderr or result.stdout)
        or "no details available"
    )


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
            details = command_failure_details(fallback_result)
            if details == "no details available":
                details = command_failure_details(apply_result)
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


def stream_file_sha256(path):
    """Hash a declared input in fixed-size chunks and report read position."""
    digest = hashlib.sha256()
    position = 0
    try:
        with Path(path).open("rb") as stream:
            while True:
                chunk = stream.read(FILE_DIGEST_CHUNK_SIZE)
                if not chunk:
                    return digest.hexdigest()
                digest.update(chunk)
                position += len(chunk)
    except (OSError, ValueError, UnicodeError) as error:
        raise FileReadFailure(position) from error


def preflight_next_condition(code):
    """Return the stable operator action for one preflight failure code."""
    conditions = {
        "missing-input": "supply the declared immutable input",
        "malformed-digest": "issue a packet with a valid sha256 digest",
        "malformed-identity": "issue a packet with a sha256 identity",
        "identity-mismatch": "align identity with the declared sha256",
        "digest-mismatch": "restore the declared source bytes or issue a new packet",
        "input-path": "declare an absolute regular file without a symlink",
        "input-read": "restore readable source bytes and retry with a fresh packet",
        "contract-mismatch": "align the project identity and contract revision",
        "unknown-field": "remove the unknown v1 field and issue a fresh packet",
        "malformed-commit": "issue a packet with a complete immutable mainline commit",
        "missing-mainline": "prepare an isolated checkout containing the required commit",
        "non-ancestor": "prepare the implementation checkout from the required mainline",
        "mainline-check": "prepare a readable implementation checkout and retry",
    }
    return conditions.get(code, "correct the packet and retry with a fresh identity")


def packet_preflight_error(category, code, field, **details):
    """Construct one bounded packet diagnostic with a stable next condition."""
    return PacketPreflightError(
        category,
        code,
        field,
        next_condition=preflight_next_condition(code),
        **details,
    )


def require_preflight_string(value, field, category="contract"):
    """Require a non-empty string in the packet envelope."""
    if not isinstance(value, str) or not value.strip():
        raise packet_preflight_error(category, "missing-input", field)
    return value


def reject_unknown_keys(value, allowed, field, category="contract"):
    """Reject unknown keys without rendering their values or packet contents."""
    if not isinstance(value, dict):
        raise packet_preflight_error(category, "missing-input", field)
    unknown = sorted(set(value) - set(allowed))
    if unknown:
        raise packet_preflight_error(
            category,
            "unknown-field",
            f"{field}.{unknown[0]}",
        )


def validate_digest_identity(descriptor, field, category):
    """Validate the bounded digest and identity pair for one file."""
    identity = descriptor.get("identity")
    sha256 = descriptor.get("sha256")
    if not isinstance(identity, str):
        raise packet_preflight_error(
            category,
            "missing-input",
            f"{field}.identity",
        )
    if not isinstance(sha256, str):
        raise packet_preflight_error(
            category,
            "missing-input",
            f"{field}.sha256",
            identity=identity,
        )
    if not identity.startswith("sha256:"):
        raise packet_preflight_error(
            category,
            "malformed-identity",
            f"{field}.identity",
            identity=identity,
        )
    if not SHA256_HEX.fullmatch(sha256):
        raise packet_preflight_error(
            category,
            "malformed-digest",
            f"{field}.sha256",
            identity=identity,
        )
    identity_digest = identity[len("sha256:") :]
    if not SHA256_HEX.fullmatch(identity_digest):
        raise packet_preflight_error(
            category,
            "malformed-identity",
            f"{field}.identity",
            identity=identity,
            expected=sha256,
        )
    if identity_digest != sha256:
        raise packet_preflight_error(
            category,
            "identity-mismatch",
            field,
            identity=identity,
            expected=sha256,
            observed=identity_digest,
        )
    return identity, sha256


def validate_declared_file_descriptor(descriptor, field, category):
    """Validate and hash one immutable regular-file descriptor."""
    reject_unknown_keys(descriptor, FILE_DESCRIPTOR_KEYS, field, category)
    if not isinstance(descriptor, dict):
        raise packet_preflight_error(category, "missing-input", field)

    raw_path = descriptor.get("path")
    if not isinstance(raw_path, str) or not raw_path:
        raise packet_preflight_error(category, "missing-input", f"{field}.path")
    identity, expected_digest = validate_digest_identity(
        descriptor, field, category,
    )
    if not os.path.isabs(raw_path):
        raise packet_preflight_error(
            category,
            "input-path",
            f"{field}.path",
            path=raw_path,
            identity=identity,
            expected=expected_digest,
            observed="relative",
        )

    path = Path(raw_path)
    try:
        if any(parent.is_symlink() for parent in (path, *path.parents)):
            raise packet_preflight_error(
                category,
                "input-path",
                field,
                path=raw_path,
                identity=identity,
                expected=expected_digest,
                observed="symlink",
            )
        if not path.exists():
            raise packet_preflight_error(
                category,
                "missing-input",
                field,
                path=raw_path,
                identity=identity,
                expected=expected_digest,
                observed="missing",
            )
        if not stat.S_ISREG(path.stat().st_mode):
            raise packet_preflight_error(
                category,
                "input-path",
                field,
                path=raw_path,
                identity=identity,
                expected=expected_digest,
                observed="non-regular",
            )
    except PacketPreflightError:
        raise
    except (OSError, ValueError, UnicodeError) as error:
        raise packet_preflight_error(
            category,
            "input-path",
            field,
            path=raw_path,
            identity=identity,
            expected=expected_digest,
            observed="unavailable",
        ) from error

    try:
        observed_digest = stream_file_sha256(path)
    except FileReadFailure as error:
        raise packet_preflight_error(
            category,
            "input-read",
            field,
            path=raw_path,
            identity=identity,
            expected=expected_digest,
            observed="partial" if error.position else "unreadable",
        ) from error

    if observed_digest != expected_digest:
        raise packet_preflight_error(
            category,
            "digest-mismatch",
            field,
            path=raw_path,
            identity=identity,
            expected=expected_digest,
            observed=observed_digest,
        )
    return {
        "field": field,
        "path": raw_path,
        "identity": identity,
        "expectedSha256": expected_digest,
        "observedSha256": observed_digest,
    }


def validate_preflight_contract(prd):
    """Validate the strict packet envelope before reading declared files."""
    if not isinstance(prd, dict):
        raise packet_preflight_error("contract", "missing-input", "packet")

    project = require_preflight_string(prd.get("project"), "project")
    preflight = prd.get("preflight")
    reject_unknown_keys(preflight, PREFLIGHT_KEYS, "preflight")
    if preflight.get("version") != PREFLIGHT_VERSION:
        raise packet_preflight_error("contract", "contract-mismatch", "preflight.version")

    project_root = require_preflight_string(
        preflight.get("projectRoot"), "preflight.projectRoot",
    )
    if not os.path.isabs(project_root):
        raise packet_preflight_error(
            "contract",
            "input-path",
            "preflight.projectRoot",
            path=project_root,
            observed="relative",
        )

    project_identity = require_preflight_string(
        preflight.get("projectIdentity"), "preflight.projectIdentity",
    )
    if project != project_identity:
        raise packet_preflight_error(
            "contract",
            "contract-mismatch",
            "project",
            expected=project_identity,
            observed=project,
        )

    contract_revision = require_preflight_string(
        preflight.get("contractRevision"), "preflight.contractRevision",
    )
    supplied_revision = prd.get("contractRevision")
    if supplied_revision is not None and supplied_revision != contract_revision:
        raise packet_preflight_error(
            "contract",
            "contract-mismatch",
            "contractRevision",
            expected=contract_revision,
            observed=supplied_revision,
        )

    authority = preflight.get("authority")
    reject_unknown_keys(authority, AUTHORITY_FIELDS, "preflight.authority", "authority-input")
    for field in AUTHORITY_FIELDS:
        if field not in authority:
            raise packet_preflight_error(
                "authority-input",
                "missing-input",
                f"preflight.authority.{field}",
            )

    intended_mainline = preflight.get("intendedMainline")
    reject_unknown_keys(
        intended_mainline,
        {"commit"},
        "preflight.intendedMainline",
        "contract",
    )
    required_commit = intended_mainline.get("commit")
    if not isinstance(required_commit, str):
        raise packet_preflight_error(
            "contract",
            "missing-input",
            "preflight.intendedMainline.commit",
        )
    if not immutable_object_id(required_commit):
        raise packet_preflight_error(
            "contract",
            "malformed-commit",
            "preflight.intendedMainline.commit",
            required_commit=required_commit,
        )

    for field in ("build", "fixtures", "publicDocs"):
        if field not in prd:
            raise packet_preflight_error("contract", "missing-input", field)
    build = prd.get("build")
    fixtures = prd.get("fixtures")
    public_docs = prd.get("publicDocs")
    if build is not None and not isinstance(build, dict):
        raise packet_preflight_error("artifact-input", "missing-input", "build")
    if not isinstance(fixtures, list):
        raise packet_preflight_error("artifact-input", "missing-input", "fixtures")
    if not isinstance(public_docs, list):
        raise packet_preflight_error(
            "artifact-input", "missing-input", "publicDocs",
        )

    return {
        "projectIdentity": project_identity,
        "contractRevision": contract_revision,
        "projectRoot": project_root,
        "authority": authority,
        "intendedMainline": intended_mainline,
        "build": build,
        "fixtures": fixtures,
        "publicDocs": public_docs,
    }


def validate_preflight_files(envelope):
    """Stream every authority and declared artifact input in stable order."""
    verified_files = []
    for field in AUTHORITY_FIELDS:
        verified_files.append(
            validate_declared_file_descriptor(
                envelope["authority"][field],
                f"preflight.authority.{field}",
                "authority-input",
            )
        )

    if envelope["build"] is not None:
        verified_files.append(
            validate_declared_file_descriptor(
                envelope["build"], "build", "artifact-input",
            )
        )
    for collection_name in ("fixtures", "publicDocs"):
        for index, descriptor in enumerate(envelope[collection_name]):
            verified_files.append(
                validate_declared_file_descriptor(
                    descriptor,
                    f"{collection_name}[{index}]",
                    "artifact-input",
                )
            )
    return verified_files


def resolve_checkout_head(reference_path):
    """Resolve a checkout head without updating any refs or worktree state."""
    result = run_git("rev-parse", "HEAD", cwd=reference_path, check=False)
    head = result.stdout.strip()
    if result.returncode != 0 or not immutable_object_id(head):
        return None
    return head


def validate_intended_mainline(
    reference_path, intended_mainline, *, resolved_head=None,
):
    """Require the immutable mainline commit to be present and ancestral."""
    required_commit = intended_mainline["commit"]
    if resolved_head is None:
        resolved_head = resolve_checkout_head(reference_path)
    object_result = run_git(
        "cat-file", "-e", f"{required_commit}^{{commit}}",
        cwd=reference_path,
        check=False,
    )
    if object_result.returncode != 0:
        raise packet_preflight_error(
            "mainline",
            "missing-mainline",
            "preflight.intendedMainline.commit",
            required_commit=required_commit,
            resolved_head=resolved_head or "missing",
        )
    if resolved_head is None:
        raise packet_preflight_error(
            "mainline",
            "mainline-check",
            "preflight.intendedMainline.commit",
            required_commit=required_commit,
            resolved_head="missing",
        )

    ancestor_result = run_git(
        "merge-base", "--is-ancestor", required_commit, resolved_head,
        cwd=reference_path,
        check=False,
    )
    if ancestor_result.returncode == 1:
        raise packet_preflight_error(
            "mainline",
            "non-ancestor",
            "preflight.intendedMainline.commit",
            required_commit=required_commit,
            resolved_head=resolved_head,
        )
    if ancestor_result.returncode != 0:
        raise packet_preflight_error(
            "mainline",
            "mainline-check",
            "preflight.intendedMainline.commit",
            required_commit=required_commit,
            resolved_head=resolved_head,
        )
    return {
        "requiredCommit": required_commit,
        "resolvedCheckoutHead": resolved_head,
    }


def validate_packet_preflight(
    repo_root,
    candidate,
    prd_name,
    *,
    reference_path=None,
    resolved_head=None,
):
    """Admit one packet without mutating Git, the root, or a destination."""
    packet_path = candidate["prd_json_path"]
    prd = read_prd(packet_path)
    if not isinstance(prd, dict):
        raise packet_preflight_error("contract", "missing-input", "packet")
    if prd.get("branchName") != prd_name:
        raise ValueError(
            "PRD branchName mismatch: expected "
            f"{safe_identity_display(prd_name)}, observed "
            f"{safe_identity_display(prd.get('branchName'))}"
        )

    try:
        packet_sha256 = stream_file_sha256(packet_path)
    except FileReadFailure as error:
        raise packet_preflight_error(
            "authority-input",
            "input-read",
            "packet",
            path=packet_path,
            observed="partial" if error.position else "unreadable",
        ) from error

    envelope = validate_preflight_contract(prd)
    verified_files = validate_preflight_files(envelope)
    if reference_path is None:
        reference_path = (
            candidate["worktree_path"]
            if not candidate["is_root"]
            else repo_root
        )
    intended_mainline = validate_intended_mainline(
        reference_path,
        envelope["intendedMainline"],
        resolved_head=resolved_head,
    )
    return {
        "version": PREFLIGHT_VERSION,
        "status": "verified",
        "packet": {
            "path": str(packet_path),
            "identity": f"sha256:{packet_sha256}",
            "sha256": packet_sha256,
        },
        "projectIdentity": envelope["projectIdentity"],
        "contractRevision": envelope["contractRevision"],
        "verifiedFiles": verified_files,
        "intendedMainline": intended_mainline,
    }


def resolve_initial_preflight_reference(
    repo_root, candidate, worktree_path, branch,
):
    """Use the packet checkout for the pre-sync, read-only admission check."""
    if worktree_path.exists() and worktree_is_valid(worktree_path):
        head = validate_registered_worktree(
            repo_root,
            worktree_path,
            branch,
        )
        return worktree_path, head

    reference_path = (
        candidate["worktree_path"] if not candidate["is_root"] else repo_root
    )
    return reference_path, resolve_checkout_head(reference_path)


def normalized_absolute_path(path):
    """Return a stable case-normalized absolute path for identity checks."""
    try:
        return os.path.normcase(str(Path(path).resolve()))
    except (OSError, RuntimeError, UnicodeError, ValueError):
        return os.path.normcase(os.path.abspath(os.path.normpath(str(path))))


def display_path(path):
    """Render a bounded, escaped local path for an operator diagnostic."""
    return bounded_failure_details(
        json.dumps(str(path), ensure_ascii=True),
        MAX_CANDIDATE_DIAGNOSTIC_PATH_LENGTH,
    )


def absolute_worktree_path(raw_path, repo_root):
    """Resolve one Git-reported worktree path without inspecting its children."""
    path = Path(raw_path)
    if not path.is_absolute():
        path = Path(repo_root) / path
    try:
        return path.resolve()
    except (OSError, RuntimeError, UnicodeError, ValueError):
        return Path(os.path.abspath(os.path.normpath(str(path))))


def canonical_packet_file_path(packet_path, worktree_path, label):
    """Validate and canonicalize one packet file before it can be consumed."""
    try:
        lexical_exists = os.path.lexists(packet_path)
    except (OSError, UnicodeError, ValueError) as error:
        raise RuntimeError(
            f"could not inspect {label.lower()} candidate "
            f"{display_path(packet_path)}: {raw_failure_details(error)}"
        ) from error
    if not lexical_exists:
        return None

    try:
        canonical_root = Path(worktree_path).resolve(strict=True)
        canonical_path = Path(packet_path).resolve(strict=True)
    except FileNotFoundError as error:
        raise RuntimeError(
            f"{label} candidate {display_path(packet_path)} could not be "
            "resolved because it is missing or contains a broken link"
        ) from error
    except (OSError, RuntimeError, UnicodeError, ValueError) as error:
        raise RuntimeError(
            f"could not resolve {label.lower()} candidate "
            f"{display_path(packet_path)}: {raw_failure_details(error)}"
        ) from error

    try:
        canonical_path.relative_to(canonical_root)
    except ValueError as error:
        raise RuntimeError(
            f"{label} candidate {display_path(packet_path)} is ineligible: "
            "its resolved path is outside the registered worktree "
            f"{display_path(canonical_root)}"
        ) from error

    try:
        is_regular_file = stat.S_ISREG(canonical_path.stat().st_mode)
    except (OSError, UnicodeError, ValueError) as error:
        raise RuntimeError(
            f"could not inspect {label.lower()} candidate "
            f"{display_path(packet_path)}: {raw_failure_details(error)}"
        ) from error
    if not is_regular_file:
        raise RuntimeError(
            f"{label} candidate {display_path(packet_path)} is ineligible: "
            "it is not a regular file"
        )

    return canonical_path


def parse_worktree_inventory(output, repo_root):
    """Parse Git's porcelain worktree inventory into bounded records."""
    blocks = []
    current = []
    for line in output.splitlines():
        if line:
            current.append(line)
        elif current:
            blocks.append(current)
            current = []
    if current:
        blocks.append(current)

    records = []
    for block in blocks:
        if not block or not block[0].startswith("worktree "):
            raise RuntimeError("git worktree inventory returned malformed output")
        raw_path = block[0][len("worktree "):]
        if not raw_path:
            raise RuntimeError("git worktree inventory returned an empty path")

        record = {
            "path": absolute_worktree_path(raw_path, repo_root),
            "branch_ref": None,
            "detached": False,
            "locked": False,
            "prunable": False,
        }
        for line in block[1:]:
            if line == "detached":
                record["detached"] = True
            elif line == "locked" or line.startswith("locked "):
                record["locked"] = True
            elif line == "prunable" or line.startswith("prunable "):
                record["prunable"] = True
            elif line.startswith("branch "):
                record["branch_ref"] = line[len("branch "):]
        records.append(record)

    if not records:
        raise RuntimeError("git worktree inventory returned no worktrees")
    return records


def list_registered_worktrees(repo_root):
    """Read Git's registered worktrees exactly once for packet discovery."""
    result = run_git(
        "worktree", "list", "--porcelain", cwd=repo_root, check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            "git worktree inventory failed "
            f"(exit {result.returncode}): {command_failure_details(result)}"
        )
    return parse_worktree_inventory(result.stdout, repo_root)


def resolve_revision_head(repo_path, revision, description):
    """Resolve one immutable Git revision without changing repository state."""
    result = run_git("rev-parse", "--verify", revision, cwd=repo_path, check=False)
    head = result.stdout.strip()
    if result.returncode != 0 or not immutable_object_id(head):
        raise RuntimeError(
            f"{description} could not resolve {safe_identity_display(revision)}: "
            f"{command_failure_details(result)}"
        )
    return head


def registered_worktree_record(repo_root, worktree_path):
    """Return the Git registration for one exact worktree path."""
    path_key = normalized_absolute_path(worktree_path)
    for record in list_registered_worktrees(repo_root):
        if normalized_absolute_path(record["path"]) == path_key:
            return record
    raise RuntimeError(
        "destination worktree "
        f"{display_path(worktree_path)} is not Git-registered"
    )


def validate_registered_worktree(
    repo_root, worktree_path, branch, *, expected_head=None,
):
    """Require a destination to be attached to the requested branch and head."""
    record = registered_worktree_record(repo_root, worktree_path)
    expected_ref = f"refs/heads/{branch}"
    if record["locked"]:
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} is locked"
        )
    if record["prunable"]:
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} is prunable"
        )
    if record["detached"] or record["branch_ref"] != expected_ref:
        observed = "detached" if record["detached"] else record["branch_ref"]
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} has branch "
            f"{safe_identity_display(observed or 'missing')}; expected "
            f"{safe_identity_display(expected_ref)}"
        )
    if not (Path(worktree_path) / ".git").exists():
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} has no .git file"
        )

    head = resolve_checkout_head(worktree_path)
    if head is None:
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} has no readable HEAD"
        )
    branch_head = resolve_revision_head(
        worktree_path,
        expected_ref,
        "destination worktree branch",
    )
    if branch_head != head:
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} HEAD "
            f"{safe_commit_value(head)} does not match branch "
            f"{safe_commit_value(branch_head)}"
        )
    if expected_head is not None and head != expected_head:
        raise RuntimeError(
            f"destination worktree {display_path(worktree_path)} resolved head "
            f"{safe_commit_value(head)}; expected {safe_commit_value(expected_head)}"
        )
    return head


def packet_candidates(repo_root, prd_name, records):
    """Find exact packet paths under each unique registered worktree root."""
    root_key = normalized_absolute_path(repo_root)
    roots = [*records]
    if not any(normalized_absolute_path(record["path"]) == root_key for record in roots):
        roots.append(
            {
                "path": Path(repo_root),
                "branch_ref": None,
                "detached": False,
                "locked": False,
                "prunable": False,
            }
        )

    candidates = []
    seen_roots = set()
    for record in roots:
        worktree_path = record["path"]
        root_identity = normalized_absolute_path(worktree_path)
        if root_identity in seen_roots:
            continue
        seen_roots.add(root_identity)

        packet_path = worktree_path / "tasks" / "todo" / f"{prd_name}.json"
        canonical_path = canonical_packet_file_path(
            packet_path, worktree_path, "PRD JSON",
        )
        if canonical_path is not None:
            candidates.append(
                {
                    "prd_json_path": canonical_path,
                    "prd_json_candidate_path": packet_path,
                    "worktree_path": worktree_path,
                    "record": record,
                    "is_root": root_identity == root_key,
                }
            )
    return candidates


def candidate_path_details(candidates):
    """Render a bounded list of conflicting exact packet paths."""
    paths = [
        display_path(
            candidate.get("prd_json_candidate_path", candidate["prd_json_path"]),
        )
        for candidate in candidates
    ]
    if len(paths) > MAX_CANDIDATE_DIAGNOSTIC_PATHS:
        omitted = len(paths) - MAX_CANDIDATE_DIAGNOSTIC_PATHS
        paths = [*paths[:MAX_CANDIDATE_DIAGNOSTIC_PATHS], f"... ({omitted} more paths)"]
    return ", ".join(paths)


def safe_identity_display(value):
    """Render a bounded packet identity without exposing packet contents."""
    return bounded_failure_details(
        json.dumps(value, ensure_ascii=True),
        MAX_CANDIDATE_DIAGNOSTIC_PATH_LENGTH,
    )


def validate_nested_packet_freshness(repo_root, candidate, prd_name):
    """Refuse attached packets with no positive evidence of a live lane."""
    record = candidate["record"]
    packet_path = candidate["prd_json_path"]
    worktree_path = candidate["worktree_path"]
    expected_ref = f"refs/heads/{prd_name}"

    if record["locked"]:
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} is ineligible: "
            "its registered worktree is locked"
        )
    if record["prunable"]:
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} is ineligible: "
            "its registered worktree is prunable"
        )
    if record["detached"] or record["branch_ref"] != expected_ref:
        observed_ref = "detached" if record["detached"] else record["branch_ref"]
        observed = safe_identity_display(observed_ref) if observed_ref else "missing"
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} is ineligible: "
            f"registered worktree branch is {observed}; expected {expected_ref}"
        )
    if not (worktree_path / ".git").exists():
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} is ineligible: "
            "registered worktree path is unavailable"
        )

    has_remote_head = branch_exists_on_remote(repo_root, prd_name)
    has_origin_main = origin_main_ref_exists(repo_root)
    baseline_ref = "refs/remotes/origin/main"
    baseline_label = "origin/main"
    if not has_origin_main and local_main_ref_exists(repo_root):
        baseline_ref = "refs/heads/main"
        baseline_label = "main"
    elif not has_origin_main and has_remote_head:
        return
    elif not has_origin_main:
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} cannot be correlated "
            "to a live lane: no local main baseline or remote head is "
            "available"
        )

    ahead_result = run_git(
        "rev-list", "--count", f"{baseline_ref}..{expected_ref}",
        cwd=repo_root, check=False,
    )
    if ahead_result.returncode != 0:
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} freshness check failed: "
            f"{command_failure_details(ahead_result)}"
        )
    ahead_text = ahead_result.stdout.strip()
    try:
        commits_ahead = int(ahead_text)
    except ValueError as error:
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} freshness check "
            f"returned an invalid commit count {safe_identity_display(ahead_text)}"
        ) from error

    if commits_ahead == 0 and not has_remote_head:
        raise RuntimeError(
            f"PRD candidate {display_path(packet_path)} appears abandoned: "
            f"the attached branch has 0 commits ahead of {baseline_label} "
            "and no remote head"
        )


def select_prd_candidate(repo_root, prd_name, records):
    """Select and validate one exact packet before any setup mutation."""
    candidates = packet_candidates(repo_root, prd_name, records)
    if not candidates:
        raise RuntimeError(
            f"PRD not found: exact tasks/todo/{prd_name}.json was absent from "
            "the main checkout and all Git-registered worktrees"
        )
    if len(candidates) > 1:
        raise RuntimeError(
            "ambiguous PRD: multiple exact candidates were found: "
            f"{candidate_path_details(candidates)}"
        )

    candidate = candidates[0]
    prd = read_prd(candidate["prd_json_path"])
    if not isinstance(prd, dict):
        raise ValueError("PRD must be a JSON object")

    packet_branch = prd.get("branchName")
    if not isinstance(packet_branch, str):
        raise ValueError("PRD branchName must be a string")
    if packet_branch != prd_name:
        raise ValueError(
            "PRD branchName mismatch: expected "
            f"{safe_identity_display(prd_name)}, observed "
            f"{safe_identity_display(packet_branch)}"
        )

    if not candidate["is_root"]:
        validate_nested_packet_freshness(repo_root, candidate, prd_name)

    packet_dir = candidate["prd_json_candidate_path"].parent
    prd_md_candidate_path = packet_dir / f"{prd_name}.md"
    prd_md_path = canonical_packet_file_path(
        prd_md_candidate_path,
        candidate["worktree_path"],
        "PRD Markdown",
    )
    candidate["prd_md_path"] = prd_md_path
    return candidate


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


def resolve_remote_main_sha(repo_root, fetch_succeeded):
    """Resolve origin/main only when the current fetch succeeded."""
    if not fetch_succeeded:
        return None
    sha = remote_main_sha(repo_root)
    if sha is None or not immutable_object_id(sha):
        return None
    return sha


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
    so clean-root checkouts can continue workspace setup from local state.
    Dirty roots skip root synchronization after fetch when a local or remote
    main ref can provide a safe start point for the requested lane.
    Returns a string-compatible result carrying any fresh origin/main SHA.
    """
    fresh_origin_main_sha = None

    def result(message):
        return RootSyncResult(message, fresh_origin_main_sha)

    if not has_origin_remote(repo_root):
        entries = root_status_for_setup(repo_root)
        if local_main_ref_exists(repo_root):
            if entries:
                return result(
                    "skipped (dirty root; using local main without root sync)"
                )
            return result("skipped (no origin remote)")
        if entries:
            raise DirtyRootError(dirty_root_diagnostic(repo_root, entries))
        raise RuntimeError(
            "no origin remote and refs/heads/main is missing"
        )

    fetch_result = run_git("fetch", "origin", cwd=repo_root, check=False)
    fetch_succeeded = fetch_result.returncode == 0
    if not fetch_succeeded:
        if local_main_ref_exists(repo_root):
            return result(
                "skipped (fetch failed: "
                f"{command_failure_details(fetch_result)})"
            )
        if not origin_main_ref_exists(repo_root):
            entries = root_status_for_setup(repo_root)
            if entries:
                raise DirtyRootError(dirty_root_diagnostic(repo_root, entries))
            raise RuntimeError(
                "fetch failed and refs/heads/main is missing: "
                f"{command_failure_details(fetch_result)}"
            )

    remote_sha = resolve_remote_main_sha(repo_root, fetch_succeeded)
    fresh_origin_main_sha = remote_sha
    entries = root_status_for_setup(repo_root)
    if entries:
        if remote_sha is not None:
            return result(
                "skipped (dirty root; using origin/main "
                f"{remote_sha[:8]} without root sync)"
            )
        if local_main_ref_exists(repo_root):
            return result(
                "skipped (dirty root; using local main without root sync)"
            )
        raise DirtyRootError(dirty_root_diagnostic(repo_root, entries))

    if remote_sha is None:
        if local_main_ref_exists(repo_root):
            return result("skipped (origin has no main branch)")
        raise RuntimeError(
            "origin has no main branch and refs/heads/main is missing"
        )

    if not local_main_ref_exists(repo_root):
        run_git("update-ref", "refs/heads/main", remote_sha, cwd=repo_root)
        return result(f"created refs/heads/main at {remote_sha[:8]}")

    local_sha = run_git("rev-parse", "refs/heads/main", cwd=repo_root).stdout.strip()
    if local_sha == remote_sha:
        if current_branch(repo_root) == "main":
            verify_root_after_restore(repo_root, None)
        return result("already up to date")

    if not can_fast_forward_main(repo_root, local_sha, remote_sha):
        return result(
            "skipped (local main is not a fast-forward behind origin/main)"
        )

    if current_branch(repo_root) == "main":
        return result(_sync_checked_out_main_with_stash(repo_root, remote_sha))

    run_git("update-ref", "refs/heads/main", remote_sha, cwd=repo_root)
    return result(
        f"fast-forwarded refs/heads/main to {remote_sha[:8]} "
        "(fetch-only; did not run git pull)"
    )


def resolve_worktree_start_point(fresh_origin_main_sha):
    """Resolve the start point for brand-new lane branches.

    Use only the immutable SHA captured by the current successful fetch. A
    missing SHA means setup is using its existing local-main fallback; a stale
    remote-tracking ref must never become a new lane's baseline.
    """
    if fresh_origin_main_sha and immutable_object_id(fresh_origin_main_sha):
        return fresh_origin_main_sha
    return "main"


def resolve_worktree_preflight_reference(
    repo_root, branch, worktree_path, fresh_origin_main_sha=None,
):
    """Resolve the exact checkout or immutable start point used for admission."""
    if worktree_path.exists() and worktree_is_valid(worktree_path):
        head = validate_registered_worktree(repo_root, worktree_path, branch)
        return worktree_path, head

    if branch_exists_locally(repo_root, branch):
        return repo_root, resolve_revision_head(
            repo_root,
            f"refs/heads/{branch}",
            "local destination branch",
        )
    if branch_exists_on_remote(repo_root, branch):
        return repo_root, resolve_revision_head(
            repo_root,
            f"refs/remotes/origin/{branch}",
            "remote destination branch",
        )

    start_point = resolve_worktree_start_point(fresh_origin_main_sha)
    if start_point == "main" and not local_main_ref_exists(repo_root):
        # Preserve the existing root-sync failure when no valid creation
        # baseline exists; setup will fail before it can create a destination.
        return repo_root, resolve_checkout_head(repo_root)
    return repo_root, resolve_revision_head(
        repo_root,
        start_point,
        "new destination start point",
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


def expected_worktree_collision(error, worktree_path, branch):
    """Recognize only Git's expected same-destination collision diagnostics."""
    details = raw_failure_details(error).casefold().replace("\\", "/")
    path = str(worktree_path).casefold().replace("\\", "/")
    if "git worktree add" not in details or path not in details:
        return False
    branch = str(branch).casefold()
    quoted_paths = (f"'{path}'", f'"{path}"')
    quoted_branches = (f"'{branch}'", f'"{branch}"')
    for quoted_path in quoted_paths:
        if f"{quoted_path} already exists" in details:
            return True
        for quoted_branch in quoted_branches:
            if any(
                f"{quoted_branch} {marker} {quoted_path}" in details
                for marker in (
                    "is already checked out at",
                    "is already used by worktree at",
                )
            ):
                return True
    return False


def add_worktree_or_reuse(
    repo_root, worktree_path, branch, expected_head, *git_args,
):
    """Converge only after a verified same-destination Git collision."""
    try:
        run_git("worktree", "add", *git_args, cwd=repo_root)
    except RuntimeError as error:
        if not expected_worktree_collision(error, worktree_path, branch):
            raise
        validate_registered_worktree(
            repo_root,
            worktree_path,
            branch,
            expected_head=expected_head,
        )
        print(
            "Worktree preparation: reused destination admitted concurrently",
            file=sys.stderr,
        )
        return True
    validate_registered_worktree(
        repo_root,
        worktree_path,
        branch,
        expected_head=expected_head,
    )
    return False


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

    No-upstream, missing-remote-branch, and diverged-from-upstream conditions
    are all non-fatal skips: a diverged local branch keeps its worktree state
    as-is (remote commits stay on origin and reconcile at push time), because
    resetting would destroy unpushed local commits and raising would kill the
    lane. Returns a human-readable outcome string for logging.
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
            if snapshot_id is not None:
                return (
                    f"stashed local changes in snapshot {snapshot_id[:8]}, "
                    "then skipped (local branch diverged from upstream; "
                    "keeping local worktree state)"
                )
            return (
                "skipped (local branch diverged from upstream; "
                "keeping local worktree state)"
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


def create_or_reuse_worktree(
    repo_root, branch, worktree_path, fresh_origin_main_sha=None,
):
    """Create a new worktree or reuse an existing one. Returns reused flag."""
    if worktree_path.exists() and worktree_is_valid(worktree_path):
        validate_registered_worktree(repo_root, worktree_path, branch)
        sync_outcome = sync_reused_worktree_branch(repo_root, worktree_path, branch)
        print(f"Worktree branch sync: {sync_outcome}", file=sys.stderr)
        validate_registered_worktree(repo_root, worktree_path, branch)
        return True

    # Remove stale path if it exists but is invalid.
    if worktree_path.exists():
        shutil.rmtree(worktree_path)

    # Create new worktree.
    worktree_path.parent.mkdir(parents=True, exist_ok=True)

    if branch_exists_locally(repo_root, branch):
        expected_head = resolve_revision_head(
            repo_root,
            f"refs/heads/{branch}",
            "local destination branch",
        )
        return add_worktree_or_reuse(
            repo_root,
            worktree_path,
            branch,
            expected_head,
            str(worktree_path),
            branch,
        )
    elif branch_exists_on_remote(repo_root, branch):
        expected_head = resolve_revision_head(
            repo_root,
            f"refs/remotes/origin/{branch}",
            "remote destination branch",
        )
        return add_worktree_or_reuse(
            repo_root,
            worktree_path,
            branch,
            expected_head,
            "--track",
            "-b",
            branch,
            str(worktree_path),
            f"origin/{branch}",
        )
    else:
        start_point = resolve_worktree_start_point(fresh_origin_main_sha)
        expected_head = resolve_revision_head(
            repo_root,
            start_point,
            "new destination start point",
        )
        return add_worktree_or_reuse(
            repo_root,
            worktree_path,
            branch,
            expected_head,
            "-b",
            branch,
            str(worktree_path),
            start_point,
        )


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
    except Exception as e:  # noqa: BLE001 - this optional copy must stay non-fatal
        print(
            format_stage_failure("Standing-rules copy skipped", e),
            file=sys.stderr,
        )
        return None

    return dest


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <prd-name>", file=sys.stderr)
        sys.exit(1)

    prd_name = sys.argv[1]

    if not prd_name:
        print("PRD name must not be empty", file=sys.stderr)
        sys.exit(1)

    try:
        repo_root = get_repo_root()
    except Exception as e:  # noqa: BLE001 - CLI boundary must classify all failures
        print(format_stage_failure("Failed to discover repo root", e), file=sys.stderr)
        sys.exit(1)

    # Discover and validate the packet before any synchronization or worktree
    # mutation. Git's registered inventory is the only supported non-root source.
    try:
        records = list_registered_worktrees(repo_root)
        selected_candidate = select_prd_candidate(repo_root, prd_name, records)
    except Exception as e:  # noqa: BLE001 - CLI boundary must classify all failures
        print(format_stage_failure("Failed to read PRD", e), file=sys.stderr)
        sys.exit(1)

    branch = f"{prd_name}"
    prd_json_path = selected_candidate["prd_json_path"]
    prd_md_path = selected_candidate["prd_md_path"]
    if selected_candidate["is_root"]:
        worktree_dir = repo_root / ".claude" / "worktrees" / normalize_branch(branch)
    else:
        worktree_dir = selected_candidate["worktree_path"]

    try:
        reference_path, resolved_head = resolve_initial_preflight_reference(
            repo_root,
            selected_candidate,
            worktree_dir,
            branch,
        )
        preflight_result = validate_packet_preflight(
            repo_root,
            selected_candidate,
            prd_name,
            reference_path=reference_path,
            resolved_head=resolved_head,
        )
    except Exception as e:  # noqa: BLE001 - CLI boundary must classify all failures
        print(format_stage_failure("Failed to read PRD", e), file=sys.stderr)
        sys.exit(1)

    # Sync main and prune worktrees.
    try:
        sync_result = sync_main(repo_root)
        sync_outcome = str(sync_result)
        fresh_origin_main_sha = getattr(
            sync_result, "fresh_origin_main_sha", None,
        )
        print(f"Root sync: {sync_outcome}", file=sys.stderr)
    except (DirtyRootError, RootStatusError) as e:
        print(f"Root cleanliness check failed: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:  # noqa: BLE001 - CLI boundary must classify all failures
        print(format_stage_failure("Root sync failed", e), file=sys.stderr)
        sys.exit(1)

    # Root synchronization is coordinated separately from packet discovery.
    # Re-read all declared inputs and the reference head before pruning or
    # creating/reusing a destination so concurrent admissions converge on the
    # same verified packet rather than trusting a stale first read.
    try:
        reference_path, resolved_head = resolve_worktree_preflight_reference(
            repo_root,
            branch,
            worktree_dir,
            fresh_origin_main_sha,
        )
        preflight_result = validate_packet_preflight(
            repo_root,
            selected_candidate,
            prd_name,
            reference_path=reference_path,
            resolved_head=resolved_head,
        )
    except Exception as e:  # noqa: BLE001 - CLI boundary must classify all failures
        print(format_stage_failure("Failed to read PRD", e), file=sys.stderr)
        sys.exit(1)

    try:
        prune_worktrees(repo_root)
    except Exception as e:  # noqa: BLE001 - preserve the existing stage boundary
        print(format_stage_failure("Root sync failed", e), file=sys.stderr)
        sys.exit(1)

    # Create or reuse worktree, then verify the actual prepared checkout before
    # copying the packet or reporting readiness. The lock covers both steps so
    # another admission cannot sync the destination between verification and
    # handoff.
    failure_stage = "Worktree preparation failed"
    try:
        with root_sync_lock(repo_root):
            reused = create_or_reuse_worktree(
                repo_root,
                branch,
                worktree_dir,
                fresh_origin_main_sha,
            )
            final_head = validate_registered_worktree(
                repo_root, worktree_dir, branch,
            )
            preflight_result = validate_packet_preflight(
                repo_root,
                selected_candidate,
                prd_name,
                reference_path=worktree_dir,
                resolved_head=final_head,
            )

            failure_stage = "PRD copy failed"
            dest_json, dest_md = copy_prd_files(
                prd_json_path, prd_md_path, worktree_dir,
            )
            dest_rules = copy_standing_rules(repo_root, worktree_dir)
    except Exception as e:  # noqa: BLE001 - CLI boundary must classify all failures
        print(
            format_stage_failure(failure_stage, e),
            file=sys.stderr,
        )
        sys.exit(1)

    # Output result.
    result = {
        "status": "ready",
        "worktree": str(worktree_dir),
        "branch": branch,
        "prd_path": str(dest_json),
        "prd_md_path": str(dest_md) if dest_md else None,
        "standing_rules_path": str(dest_rules) if dest_rules else None,
        "reused": reused,
        "preflight": preflight_result,
    }
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
