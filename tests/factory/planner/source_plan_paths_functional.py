#!/usr/bin/env python3
"""Source-plan path contract harness and actual planner probe.

The pure helpers in this module make the planner prompt's filesystem contract
executable without a provider.  The command-line probe runs the real plan
workstation when a configured planner is available.
"""

from __future__ import annotations

import argparse
from contextlib import contextmanager
import json
import ntpath
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Iterable, Iterator, Mapping


WINDOWS_DRIVE_ABSOLUTE = re.compile(r"^[A-Za-z]:[\\/]")
SOURCE_PLAN_MARKER = re.compile(r"^Source plan: `([^`]+)`$", re.MULTILINE)
_CONSUMER_CWD_LOCK = threading.Lock()


class SourcePlanError(ValueError):
    """A source-plan input cannot be read under the planner contract."""


class PacketValidationError(ValueError):
    """Generated planner artifacts do not form a usable packet."""


class PlannerCancelled(RuntimeError):
    """The planner was cancelled before it completed a packet."""


@dataclass(frozen=True)
class SourcePlanResolution:
    raw_source_plan: str
    persisted_source_plan: str
    content: bytes


@dataclass(frozen=True)
class PacketPaths:
    json_path: Path
    markdown_path: Path


def is_windows_drive_absolute(value: str) -> bool:
    """Recognize both slash spellings of a Windows drive-letter path."""

    return bool(WINDOWS_DRIVE_ABSOLUTE.match(value))


def _is_windows_path(value: str) -> bool:
    return is_windows_drive_absolute(value) or "\\" in value


def _normalized_path(value: str) -> str:
    if _is_windows_path(value):
        return ntpath.normcase(ntpath.normpath(value))
    return os.path.normcase(os.path.abspath(value))


def _is_within(path: str, root: str) -> bool:
    path_norm = _normalized_path(path)
    root_norm = _normalized_path(root)
    try:
        if _is_windows_path(path_norm) or _is_windows_path(root_norm):
            return ntpath.commonpath([path_norm, root_norm]) == root_norm
        return os.path.commonpath([path_norm, root_norm]) == root_norm
    except ValueError:
        return False


def _default_regular_file(path: str) -> bool:
    return Path(path).is_file()


def _default_read_file(path: str) -> bytes:
    return Path(path).read_bytes()


def _absolute_relative_path(repo_root: str, relative_value: str) -> str:
    if _is_windows_path(repo_root):
        return ntpath.normpath(ntpath.join(repo_root, relative_value))
    return str((Path(repo_root) / Path(relative_value)).resolve())


def resolve_source_plan(
    raw_source_plan: str | None,
    repo_root: str | Path,
    *,
    authorized_roots: Iterable[str | Path] | None = None,
    is_regular_file: Callable[[str], bool] | None = None,
    read_file: Callable[[str], bytes] | None = None,
) -> SourcePlanResolution | None:
    """Resolve, authorize, and fully read one source-plan input.

    ``is_regular_file`` and ``read_file`` are injectable only so the local
    matrix can exercise permission and provider boundaries deterministically.
    The actual planner probe uses the default local filesystem functions.
    """

    if raw_source_plan is None:
        return None
    if not isinstance(raw_source_plan, str):
        raise SourcePlanError("sourcePlan must be a string or null")
    if raw_source_plan.strip() == "":
        raise SourcePlanError("sourcePlan must not be empty")
    if raw_source_plan != raw_source_plan.strip():
        raise SourcePlanError("sourcePlan must not have surrounding whitespace")

    root = str(repo_root)
    roots = [str(value) for value in (authorized_roots or [root])]
    if is_windows_drive_absolute(raw_source_plan):
        persisted = raw_source_plan
    else:
        persisted = _absolute_relative_path(root, raw_source_plan)

    if not any(_is_within(persisted, root_value) for root_value in roots):
        raise SourcePlanError(f"sourcePlan is outside an authorized workspace: {raw_source_plan}")

    regular_file = is_regular_file or _default_regular_file
    if not regular_file(persisted):
        raise SourcePlanError(f"sourcePlan is not a readable regular file: {persisted}")

    reader = read_file or _default_read_file
    try:
        content = reader(persisted)
    except (OSError, PermissionError) as error:
        raise SourcePlanError(f"sourcePlan read failed for {persisted}: {error}") from error
    if not isinstance(content, bytes):
        raise SourcePlanError("sourcePlan reader did not return complete bytes")
    return SourcePlanResolution(raw_source_plan, persisted, content)


def _check_cancelled(cancel_event: threading.Event | None) -> None:
    if cancel_event is not None and cancel_event.is_set():
        raise PlannerCancelled("planner cancelled before packet completion")


def write_packet(
    output_dir: str | Path,
    resolution: SourcePlanResolution | None,
    *,
    name: str = "source-plan-path-fixture",
) -> PacketPaths:
    """Write a minimal pair of planner artifacts with an exact path trace."""

    destination = Path(output_dir)
    destination.mkdir(parents=True, exist_ok=True)
    context: dict[str, object] = {
        "sourcePlan": (
            resolution.persisted_source_plan if resolution is not None else None
        )
    }
    if resolution is not None:
        context["sourcePlanResolution"] = {
            "rawSourcePlan": resolution.raw_source_plan,
            "persistedSourcePlan": resolution.persisted_source_plan,
        }
    payload = {
        "project": name,
        "description": "Source-plan path contract fixture",
        "context": context,
        "acceptanceCriteria": ["The source plan remains readable from a consumer worktree."],
        "userStories": [],
    }
    json_path = destination / f"{name}.json"
    markdown_path = destination / f"{name}.md"
    json_path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    marker_value = (
        resolution.persisted_source_plan if resolution is not None else "null"
    )
    markdown_path.write_text(
        f"# Source-plan path fixture\n\nSource plan: `{marker_value}`\n",
        encoding="utf-8",
    )
    return PacketPaths(json_path, markdown_path)


def plan_and_write(
    output_dir: str | Path,
    raw_source_plan: str | None,
    repo_root: str | Path,
    *,
    cancel_event: threading.Event | None = None,
    **resolve_options: object,
) -> tuple[SourcePlanResolution | None, PacketPaths]:
    """Complete resolution before writing either artifact."""

    _check_cancelled(cancel_event)
    resolution = resolve_source_plan(raw_source_plan, repo_root, **resolve_options)
    _check_cancelled(cancel_event)
    return resolution, write_packet(output_dir, resolution)


def _read_consumer_bytes(
    persisted_source_plan: str,
    consumer_cwd: str | Path | None,
    *,
    read_file: Callable[[str], bytes] | None = None,
) -> bytes:
    if consumer_cwd is None:
        if read_file is not None:
            return read_file(persisted_source_plan)
        return Path(persisted_source_plan).read_bytes()

    with _CONSUMER_CWD_LOCK:
        previous_cwd = Path.cwd()
        try:
            os.chdir(consumer_cwd)
            if read_file is not None:
                return read_file(persisted_source_plan)
            return Path(persisted_source_plan).read_bytes()
        finally:
            os.chdir(previous_cwd)


def validate_packet(
    packet: PacketPaths,
    *,
    expected_raw_source_plan: str | None,
    expected_persisted_source_plan: str | None,
    expected_content: bytes | None,
    consumer_cwd: str | Path | None = None,
    read_file: Callable[[str], bytes] | None = None,
) -> Mapping[str, object]:
    """Validate exact decoded values, paired artifacts, and consumer bytes."""

    if not packet.json_path.is_file() or not packet.markdown_path.is_file():
        raise PacketValidationError(
            f"packet is incomplete: json={packet.json_path.is_file()} "
            f"markdown={packet.markdown_path.is_file()}"
        )
    try:
        document = json.loads(packet.json_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise PacketValidationError(f"JSON artifact cannot be read: {error}") from error
    context = document.get("context")
    if not isinstance(context, dict):
        raise PacketValidationError("JSON artifact has no object context")

    persisted = context.get("sourcePlan")
    if persisted != expected_persisted_source_plan:
        raise PacketValidationError(
            f"artifact sourcePlan values differ: json={persisted!r} "
            f"expected={expected_persisted_source_plan!r}"
        )
    if expected_persisted_source_plan is None:
        if "sourcePlanResolution" in context:
            raise PacketValidationError("no-plan packet contains a stale source-plan trace")
        if expected_content is not None:
            raise PacketValidationError("no-plan packet unexpectedly has source bytes")
        return document

    trace = context.get("sourcePlanResolution")
    if not isinstance(trace, dict):
        raise PacketValidationError("named source plan has no resolution trace")
    if trace.get("rawSourcePlan") != expected_raw_source_plan:
        raise PacketValidationError(
            f"raw sourcePlan trace differs: {trace.get('rawSourcePlan')!r} "
            f"expected={expected_raw_source_plan!r}"
        )
    if trace.get("persistedSourcePlan") != expected_persisted_source_plan:
        raise PacketValidationError(
            f"persisted sourcePlan trace differs: {trace.get('persistedSourcePlan')!r} "
            f"expected={expected_persisted_source_plan!r}"
        )

    markers = SOURCE_PLAN_MARKER.findall(
        packet.markdown_path.read_text(encoding="utf-8")
    )
    if markers != [expected_persisted_source_plan]:
        raise PacketValidationError(
            f"Markdown sourcePlan values differ: {markers!r} "
            f"expected={[expected_persisted_source_plan]!r}"
        )
    if expected_content is not None:
        actual_content = _read_consumer_bytes(
            expected_persisted_source_plan,
            consumer_cwd,
            read_file=read_file,
        )
        if actual_content != expected_content:
            raise PacketValidationError("consumer read returned different source bytes")
    return document


def require_planner_success(result: subprocess.CompletedProcess[str]) -> None:
    if result.returncode != 0:
        detail = (result.stderr or result.stdout or "planner returned no diagnostic").strip()
        raise SourcePlanError(
            f"planner provider failed with exit {result.returncode}: {detail}"
        )


def repository_root() -> Path:
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        check=True,
        capture_output=True,
        text=True,
    )
    return Path(result.stdout.strip()).resolve()


@contextmanager
def temporary_git_worktree(root: Path) -> Iterator[Path]:
    """Yield a real detached worktree and remove only that temporary worktree."""

    parent = Path(tempfile.mkdtemp(prefix="source-plan-consumer-"))
    worktree = parent / "consumer"
    try:
        result = subprocess.run(
            ["git", "worktree", "add", "--detach", str(worktree), "HEAD"],
            cwd=root,
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            detail = (result.stderr or result.stdout or "git worktree add failed").strip()
            raise RuntimeError(f"consumer worktree creation failed: {detail}")
        yield worktree
    finally:
        subprocess.run(
            ["git", "worktree", "remove", "--force", str(worktree)],
            cwd=root,
            capture_output=True,
            text=True,
            check=False,
        )
        shutil.rmtree(parent, ignore_errors=True)


def _minimal_factory(source_factory_dir: Path, planner_prompt: str) -> None:
    """Create a one-workstation factory that stops after plan completion."""

    (source_factory_dir / "workstations" / "plan").mkdir(parents=True)
    (source_factory_dir / "workstations" / "plan" / "AGENTS.md").write_text(
        planner_prompt,
        encoding="utf-8",
    )
    factory = {
        "name": "source-plan-functional-fixture",
        "version": {"logical": "1", "physical": "2026-08-30T00:00:00Z"},
        "workTypes": [
            {
                "id": "plan",
                "name": "plan",
                "states": [
                    {"id": "init", "name": "init", "type": "INITIAL"},
                    {"id": "complete", "name": "complete", "type": "TERMINAL"},
                    {"id": "failed", "name": "failed", "type": "FAILED"},
                ],
            }
        ],
        "workers": [
            {
                "id": "planner",
                "name": "planner",
                "type": "AGENT_WORKER",
                "executorProvider": "SCRIPT_WRAP",
                "modelProvider": "codex",
                "model": "gpt-5.6-sol",
                "reasoningEffort": "medium",
                "skipPermissions": True,
                "stopToken": "<COMPLETE>",
            }
        ],
        "workstations": [
            {
                "id": "plan",
                "name": "plan",
                "type": "AGENT_RUN",
                "worker": "planner",
                "inputs": [{"workType": "plan", "state": "init"}],
                "outputs": [{"workType": "plan", "state": "complete"}],
                "onFailure": [{"workType": "plan", "state": "failed"}],
            }
        ],
    }
    (source_factory_dir / "factory.json").write_text(
        json.dumps(factory, indent=2) + "\n", encoding="utf-8"
    )


def _run_actual_case(
    root: Path,
    planner_prompt: str,
    case: str,
) -> Mapping[str, object]:
    if os.name != "nt":
        raise RuntimeError("actual Windows path probe requires a Windows planner host")

    fixture_name = f"source-plan-functional-{uuid.uuid4().hex}"
    source_relative = Path("docs") / "temp" / f"{fixture_name}.md"
    source_path = root / source_relative
    content = f"# Unique source-plan fixture {fixture_name}\n\nbytes={uuid.uuid4().hex}\n".encode()
    source_path.parent.mkdir(parents=True, exist_ok=True)
    source_path.write_bytes(content)

    batch_path = Path(tempfile.mkdtemp(prefix="source-plan-batch-")) / "batch.json"
    factory_dir = batch_path.parent / "factory"
    output_dir = root / "tasks" / "todo"
    output_dir.mkdir(parents=True, exist_ok=True)
    batch_path.parent.mkdir(parents=True, exist_ok=True)
    raw_path = str(source_path)
    if case == "windows-forward":
        raw_path = raw_path.replace("\\", "/")
    elif case == "relative":
        raw_path = source_relative.as_posix()
    batch = {
        "requestId": fixture_name,
        "type": "FACTORY_REQUEST_BATCH",
        "works": [
            {
                "name": fixture_name,
                "workTypeName": "plan",
                "state": "init",
                "payload": {
                    "sourcePlan": raw_path,
                    "request": (
                        "Read the named source plan and create the smallest valid plan. "
                        "Follow the source-plan input contract exactly."
                    ),
                },
            }
        ],
        "relations": [],
    }
    batch_path.write_text(json.dumps(batch, indent=2) + "\n", encoding="utf-8")
    _minimal_factory(factory_dir, planner_prompt)
    try:
        result = subprocess.run(
            [
                "you",
                "run",
                "--dir",
                str(factory_dir),
                "--work",
                str(batch_path),
                "--no-record",
                "--quiet",
            ],
            cwd=root,
            capture_output=True,
            text=True,
            timeout=300,
            check=False,
        )
        require_planner_success(result)
        packet = PacketPaths(
            output_dir / f"{fixture_name}.json",
            output_dir / f"{fixture_name}.md",
        )
        expected_values = (
            {
                str(source_path.resolve()),
                str(source_path.resolve()).replace("\\", "/"),
            }
            if case == "relative"
            else {raw_path}
        )
        document = json.loads(packet.json_path.read_text(encoding="utf-8"))
        actual_persisted = document["context"]["sourcePlan"]
        if actual_persisted not in expected_values:
            raise PacketValidationError(
                f"relative sourcePlan is not an absolute root path: {actual_persisted!r}"
            )
        with temporary_git_worktree(root) as consumer_worktree:
            validate_packet(
                packet,
                expected_raw_source_plan=raw_path,
                expected_persisted_source_plan=actual_persisted,
                expected_content=content,
                consumer_cwd=consumer_worktree,
            )
        return {
            "case": case,
            "rawSourcePlan": raw_path,
            "persistedSourcePlan": actual_persisted,
            "fixtureBytes": len(content),
            "packet": str(packet.json_path),
        }
    except subprocess.TimeoutExpired as error:
        raise SourcePlanError(
            f"planner provider timed out after {error.timeout} seconds"
        ) from error
    finally:
        for path in (
            output_dir / f"{fixture_name}.json",
            output_dir / f"{fixture_name}.md",
            source_path,
        ):
            path.unlink(missing_ok=True)
        shutil.rmtree(batch_path.parent, ignore_errors=True)


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--phase", choices=("pre-change", "post-change"), required=True)
    parser.add_argument(
        "--case",
        action="append",
        choices=("windows-backslash", "windows-forward", "relative"),
        required=True,
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    if args.phase == "pre-change" and args.case != ["windows-backslash"]:
        raise SystemExit("pre-change accepts exactly --case windows-backslash")
    if args.phase == "post-change" and set(args.case) != {
        "windows-backslash",
        "windows-forward",
        "relative",
    }:
        raise SystemExit(
            "post-change requires windows-backslash, windows-forward, and relative"
        )
    root = repository_root()
    prompt = (root / "factory" / "workstations" / "plan" / "AGENTS.md").read_text(
        encoding="utf-8"
    )
    results = [_run_actual_case(root, prompt, case) for case in args.case]
    print(json.dumps({"phase": args.phase, "results": results}, indent=2))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (RuntimeError, SourcePlanError, PacketValidationError) as error:
        print(f"source-plan functional probe failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
