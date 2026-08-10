#!/usr/bin/env python3
"""Create or reuse one safe task worktree for @you/full-flow."""
import json
import re
import subprocess
import sys
import time
from pathlib import Path

SAFE_TASK = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$")
SAFE_BRANCH = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,79}$")
GIT_CONFIG_LOCK_RETRIES = 20
GIT_CONFIG_LOCK_RETRY_DELAY_SECONDS = 0.05

def run(root, *args):
    result = subprocess.run(["git", *args], cwd=root, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    return result.stdout.strip()


def git_config_lock_path(root):
    result = subprocess.run(
        ["git", "rev-parse", "--git-path", "config.lock"],
        cwd=root,
        text=True,
        capture_output=True,
    )
    raw_path = result.stdout.strip()
    if result.returncode == 0 and raw_path:
        path = Path(raw_path)
        return path if path.is_absolute() else root / path
    return root / ".git" / "config.lock"


def persist_longpaths(root):
    last_error = ""
    for _ in range(GIT_CONFIG_LOCK_RETRIES):
        result = subprocess.run(
            ["git", "config", "core.longpaths", "true"],
            cwd=root,
            text=True,
            capture_output=True,
        )
        if result.returncode == 0:
            return
        last_error = result.stderr.strip() or result.stdout.strip()
        # Parallel task setup processes can briefly contend on .git/config.lock.
        # Give the lock owner a bounded opportunity to finish before retrying.
        time.sleep(GIT_CONFIG_LOCK_RETRY_DELAY_SECONDS)
    lock_path = git_config_lock_path(root)
    if lock_path.exists():
        raise RuntimeError(
            "git config serialization contention: "
            f"resource={lock_path} owner_liveness=indeterminate; "
            "verify no Git or task-worktree setup process is still using the "
            f"repository, then remove only {lock_path} and retry. "
            f"last_git_error={last_error}"
        )
    raise RuntimeError(last_error or "could not persist core.longpaths")

def main():
    if len(sys.argv) != 3 or not SAFE_TASK.fullmatch(sys.argv[1]) or not SAFE_BRANCH.fullmatch(sys.argv[2]) or ".." in sys.argv[1] or ".." in sys.argv[2]:
        raise RuntimeError("safe task and base branch names are required")
    task, base = sys.argv[1], sys.argv[2]
    root = Path(run(None, "rev-parse", "--show-toplevel"))
    run(root, "rev-parse", "--verify", base)
    target = root / ".claude" / "worktrees" / task
    if not (target / ".git").exists():
        target.parent.mkdir(parents=True, exist_ok=True)
        exists = subprocess.run(["git", "show-ref", "--verify", "--quiet", f"refs/heads/{task}"], cwd=root).returncode == 0
        args = ["worktree", "add", str(target), task] if exists else ["worktree", "add", "-b", task, str(target), base]
        # Packaged invocations may run with an isolated HOME and therefore
        # cannot rely on a customer's global Git-for-Windows configuration.
        run(root, "-c", "core.longpaths=true", *args)
    persist_longpaths(root)
    print(json.dumps({"status":"ready","task":task,"base":base,"worktree":str(target)}))

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"task worktree setup failed: {error}", file=sys.stderr)
        raise SystemExit(1)
