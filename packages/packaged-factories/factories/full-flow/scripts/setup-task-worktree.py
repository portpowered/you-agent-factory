#!/usr/bin/env python3
"""Create or reuse one safe task worktree for @you/full-flow."""
import json
import re
import subprocess
import sys
from pathlib import Path

SAFE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,79}$")

def run(root, *args):
    result = subprocess.run(["git", *args], cwd=root, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    return result.stdout.strip()

def main():
    if len(sys.argv) != 3 or not SAFE.fullmatch(sys.argv[1]) or not SAFE.fullmatch(sys.argv[2]) or ".." in sys.argv[1] or ".." in sys.argv[2]:
        raise RuntimeError("safe task and base branch names are required")
    task, base = sys.argv[1], sys.argv[2]
    root = Path(run(None, "rev-parse", "--show-toplevel"))
    run(root, "rev-parse", "--verify", base)
    target = root / ".claude" / "worktrees" / task.replace("/", "-")
    if not (target / ".git").exists():
        target.parent.mkdir(parents=True, exist_ok=True)
        exists = subprocess.run(["git", "show-ref", "--verify", "--quiet", f"refs/heads/{task}"], cwd=root).returncode == 0
        args = ["worktree", "add", str(target), task] if exists else ["worktree", "add", "-b", task, str(target), base]
        run(root, *args)
    print(json.dumps({"status":"ready","task":task,"base":base,"worktree":str(target)}))

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"task worktree setup failed: {error}", file=sys.stderr)
        raise SystemExit(1)
