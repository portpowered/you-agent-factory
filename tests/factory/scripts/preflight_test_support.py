"""Disposable v1 packet fixtures shared by setup-workspace tests."""

import hashlib
import json
import subprocess
from pathlib import Path


def git(args, cwd, check=True):
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=check,
        capture_output=True,
        text=True,
    )


def current_head(repo_path):
    return git(["rev-parse", "HEAD"], repo_path).stdout.strip()


def _ignore_operator_packet(repo_path):
    common_dir = Path(git(["rev-parse", "--git-common-dir"], repo_path).stdout.strip())
    if not common_dir.is_absolute():
        common_dir = repo_path / common_dir
    exclude_path = common_dir.resolve() / "info" / "exclude"
    existing = exclude_path.read_text(encoding="utf-8") if exclude_path.exists() else ""
    entries = set(existing.splitlines())
    entries.update(("tasks/todo/", ".claude/"))
    exclude_path.parent.mkdir(parents=True, exist_ok=True)
    exclude_path.write_text("\n".join(sorted(entries)) + "\n", encoding="utf-8")


def _descriptor(path):
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    return {
        "path": str(path),
        "identity": f"sha256:{digest}",
        "sha256": digest,
    }


def valid_packet(repo_path, prd_name, *, fixtures=None, public_docs=None):
    """Create authority files and return a complete valid setup packet."""
    fixture_root = repo_path.parent / f"{repo_path.name}-{prd_name}-authority"
    fixture_root.mkdir(parents=True, exist_ok=True)
    authority = {}
    for field, contents in (
        ("sourcePlan", f"source plan for {prd_name}\n"),
        ("request", f"request for {prd_name}\n"),
        ("acceptance", f"acceptance for {prd_name}\n"),
    ):
        path = fixture_root / f"{field}.md"
        path.write_text(contents, encoding="utf-8")
        authority[field] = _descriptor(path)

    return {
        "branchName": prd_name,
        "project": "setup-workspace-test",
        "preflight": {
            "version": "factory-preflight.v1",
            "projectRoot": str(fixture_root),
            "projectIdentity": "setup-workspace-test",
            "contractRevision": "setup-workspace-test-v1",
            "authority": authority,
            "intendedMainline": {"commit": current_head(repo_path)},
        },
        "build": None,
        "fixtures": list(fixtures or []),
        "publicDocs": list(public_docs or []),
    }


def write_packet(repo_path, prd_name, payload=None, *, include_md=False):
    """Write a packet, enriching only packets with the expected branch name."""
    _ignore_operator_packet(repo_path)
    packet = valid_packet(repo_path, prd_name) if payload is None else payload
    if isinstance(payload, dict) and payload.get("branchName") == prd_name:
        packet = valid_packet(repo_path, prd_name)
        packet.update(payload)
    tasks_dir = repo_path / "tasks" / "todo"
    tasks_dir.mkdir(parents=True, exist_ok=True)
    packet_path = tasks_dir / f"{prd_name}.json"
    packet_path.write_text(json.dumps(packet), encoding="utf-8")
    markdown_path = None
    if include_md:
        markdown_path = tasks_dir / f"{prd_name}.md"
        markdown_path.write_text(f"# {prd_name}\n", encoding="utf-8")
    return packet_path, markdown_path


def add_file_descriptor(repo_path, prd_name, collection, name, contents):
    """Create an ignored declared artifact and return its packet descriptor."""
    fixture_root = repo_path.parent / f"{repo_path.name}-{prd_name}-artifacts"
    fixture_root.mkdir(parents=True, exist_ok=True)
    path = fixture_root / name
    path.write_bytes(contents)
    return _descriptor(path)
