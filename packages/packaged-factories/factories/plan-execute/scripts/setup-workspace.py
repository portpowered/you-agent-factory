#!/usr/bin/env python3
"""Prepare the isolated PRD worktree used by @you/plan-execute."""
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

SAFE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,79}$")

def git(root, *args):
    result = subprocess.run(["git", *args], cwd=root, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    return result.stdout.strip()

def main():
    if len(sys.argv) not in (2, 3) or not SAFE_NAME.fullmatch(sys.argv[1]) or ".." in sys.argv[1]:
        raise RuntimeError("a safe PRD/worktree name is required")
    name = sys.argv[1]
    branch = sys.argv[2].strip() if len(sys.argv) == 3 else ""
    if not branch:
        branch = name
    if not SAFE_NAME.fullmatch(branch) or ".." in branch:
        raise RuntimeError("a safe branch name is required")
    root = Path(git(None, "rev-parse", "--show-toplevel"))
    source = root / "tasks" / "todo" / f"{name}.json"
    markdown = source.with_suffix(".md")
    if not source.is_file() or not markdown.is_file():
        raise RuntimeError(f"matching Markdown and JSON PRDs are required for {name}")
    document = json.loads(source.read_text(encoding="utf-8"))
    if not isinstance(document.get("stories"), list) or not document["stories"]:
        raise RuntimeError("PRD JSON must contain non-empty stories")
    worktree = root / ".claude" / "worktrees" / name.replace("/", "-")
    branch_exists = subprocess.run(["git", "show-ref", "--verify", "--quiet", f"refs/heads/{branch}"], cwd=root).returncode == 0
    if not (worktree / ".git").exists():
        worktree.parent.mkdir(parents=True, exist_ok=True)
        args = ["worktree", "add", str(worktree), branch] if branch_exists else ["worktree", "add", "-b", branch, str(worktree), "HEAD"]
        git(root, *args)
    shutil.copy2(source, worktree / "prd.json")
    shutil.copy2(markdown, worktree / "prd.md")
    (worktree / "progress.txt").touch(exist_ok=True)
    print(json.dumps({"status":"ready","branch":branch,"worktree":str(worktree)}))

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"workspace setup failed: {error}", file=sys.stderr)
        raise SystemExit(1)
