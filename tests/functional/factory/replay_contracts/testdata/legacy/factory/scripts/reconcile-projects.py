#!/usr/bin/env python3
"""Reconcile waiting Project Leads without creating overlapping cycles.

The script is intended for a SCRIPT_RUN workstation.  It reads the selected
Factory Session through the public ``you`` CLI, then makes only deliberate,
idempotent moves of stranded existing ``project`` Work from ``waiting`` to
``init``.  Blocked Projects are inspect-only. It never submits a new Project
or cycle.

Usage:
    python3 factory/scripts/reconcile-projects.py --session SESSION_ID

The worker workstation should pass ``--session {{ .Context.SessionID }}`` and
may override ``--server`` when the Factory host is not on the local default
port.  ``--dry-run`` is useful for local probes and tests.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
from typing import Any, Callable, Iterable, Mapping, Optional


DEFAULT_SERVER = "http://127.0.0.1:7437"
CLI_TIMEOUT_SECONDS = 30
CHILD_WORK_TYPES = frozenset({"idea", "plan", "task", "review", "validation"})
ACTIVE_WORKER_SESSION_STATES = frozenset({"RESERVED", "STARTING", "RUNNING", "PAUSED"})
NONTERMINAL_STATE_TYPES = frozenset({"INITIAL", "PROCESSING"})
REQUEST_ID_LIMIT = 180


class ReconcileError(RuntimeError):
    """A public CLI read or move failed before reconciliation could finish."""


CommandRunner = Callable[[list[str]], subprocess.CompletedProcess[str]]


def _work_id(work: Mapping[str, Any]) -> str:
    return _string(work.get("workId") or work.get("id"))


def _work_name(work: Mapping[str, Any]) -> str:
    return _string(work.get("name"))


def _work_type(work: Mapping[str, Any]) -> str:
    return _string(work.get("workTypeName") or work.get("workType"))


def _state(work: Mapping[str, Any]) -> tuple[str, str]:
    state = work.get("state")
    if not isinstance(state, Mapping):
        return "", ""
    return _string(state.get("name")), _string(state.get("type")).upper()


def _string(value: Any) -> str:
    return value.strip() if isinstance(value, str) else ""


def _payload_object(work: Mapping[str, Any]) -> Mapping[str, Any]:
    payload = work.get("payload")
    if isinstance(payload, Mapping):
        return payload
    if isinstance(payload, str):
        try:
            decoded = json.loads(payload)
        except json.JSONDecodeError:
            return {}
        return decoded if isinstance(decoded, Mapping) else {}
    return {}


def _run_command(command: list[str]) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            command,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=CLI_TIMEOUT_SECONDS,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ReconcileError(f"could not execute {' '.join(command[:2])}: {error}") from error


def _cli_json(
    runner: CommandRunner,
    server: str,
    *arguments: str,
) -> Any:
    command = ["you", "--server", server, "--json", *arguments]
    result = runner(command)
    if result.returncode != 0:
        details = (result.stderr or result.stdout or "").strip()
        if len(details) > 400:
            details = details[:400] + "..."
        raise ReconcileError(
            f"{' '.join(command)} failed with exit {result.returncode}"
            + (f": {details}" if details else "")
        )
    try:
        return json.loads(result.stdout or "")
    except json.JSONDecodeError as error:
        raise ReconcileError(
            f"{' '.join(command)} returned invalid JSON"
        ) from error


def _session_is_running(session: Any) -> bool:
    if not isinstance(session, Mapping):
        raise ReconcileError("session show returned a non-object JSON value")
    runtime = session.get("runtime")
    if not isinstance(runtime, Mapping):
        raise ReconcileError("session show omitted runtime")
    progress = runtime.get("progress")
    if not isinstance(progress, Mapping):
        raise ReconcileError("session show omitted runtime.progress")
    # Factory state is the authoritative scheduler lifecycle.  Runtime status
    # is IDLE while a healthy server has no dispatch in flight, so it must not
    # be used as the sole liveness test.
    return _string(progress.get("factoryState")).upper() == "RUNNING"


def _work_results(response: Any) -> list[Mapping[str, Any]]:
    if not isinstance(response, Mapping):
        raise ReconcileError("work list returned a non-object JSON value")
    results = response.get("results")
    if results is None:
        return []
    if not isinstance(results, list) or any(not isinstance(item, Mapping) for item in results):
        raise ReconcileError("work list returned an invalid results array")
    return list(results)


def _worker_session_results(response: Any) -> list[Mapping[str, Any]]:
    if not isinstance(response, Mapping):
        raise ReconcileError("Worker Session list returned a non-object JSON value")
    sessions = response.get("sessions")
    if sessions is None:
        return []
    if not isinstance(sessions, list) or any(not isinstance(item, Mapping) for item in sessions):
        raise ReconcileError("Worker Session list returned an invalid sessions array")
    return list(sessions)


def _session_has_active_work(
    sessions: Iterable[Mapping[str, Any]], work_id: str, session_id: str
) -> bool:
    for session in sessions:
        observed_session_id = _string(session.get("factorySessionId"))
        if observed_session_id and observed_session_id != session_id:
            continue
        if _string(session.get("state")).upper() not in ACTIVE_WORKER_SESSION_STATES:
            continue
        ids = set()
        candidate = session.get("workId")
        if isinstance(candidate, str):
            ids.add(candidate)
        candidates = session.get("workIds")
        if isinstance(candidates, list):
            ids.update(item for item in candidates if isinstance(item, str))
        # The Work-scoped endpoint may omit nullable Work identity fields. An
        # active observation with no identity is still potentially this lead;
        # fail closed rather than dispatching concurrently.
        if not ids:
            return True
        if work_id in ids:
            return True
    return False


def _belongs_to_project(work: Mapping[str, Any], project: Mapping[str, Any]) -> bool:
    name = _work_name(work)
    project_name = _work_name(project)
    if not name or not project_name or _work_type(work) not in CHILD_WORK_TYPES:
        return False

    project_payload = _payload_object(project)
    child_payload = _payload_object(work)
    project_root = _string(project_payload.get("projectRoot"))
    if _string(child_payload.get("project")) == project_name:
        return True
    if project_root and _string(child_payload.get("projectRoot")) == project_root:
        return True

    # Project Lead's documented naming contract is PROJECT-cNN-... for ideas;
    # plan/task/review/validation Work keeps that authored name through the
    # delivery graph.
    return name.startswith(project_name + "-c")


def _project_children(
    works: Iterable[Mapping[str, Any]], project: Mapping[str, Any]
) -> list[Mapping[str, Any]]:
    return [work for work in works if _belongs_to_project(work, project)]


def _same_name_cycles(
    works: Iterable[Mapping[str, Any]], project_name: str
) -> list[Mapping[str, Any]]:
    return [
        work
        for work in works
        if _work_type(work) == "project-cycle" and _work_name(work) == project_name
    ]


def _observation_view(work: Mapping[str, Any]) -> dict[str, Any]:
    """Keep the request fingerprint bounded to public, state-bearing fields."""
    failure = work.get("failureDetail")
    if isinstance(failure, Mapping):
        failure = {
            "reason": _string(failure.get("reason")),
            "message": _string(failure.get("message"))[:512],
        }
    else:
        failure = None
    return {
        "workId": _work_id(work),
        "name": _work_name(work),
        "workTypeName": _work_type(work),
        "state": work.get("state"),
        "currentChainingTraceId": work.get("currentChainingTraceId"),
        "confirmationState": work.get("confirmationState"),
        "failureDetail": failure,
    }


def _observation_revision(
    project: Mapping[str, Any],
    children: Iterable[Mapping[str, Any]],
    cycles: Iterable[Mapping[str, Any]],
    reason: str,
) -> str:
    view = {
        "project": _observation_view(project),
        "children": sorted(
            (_observation_view(child) for child in children),
            key=lambda item: (item["workId"], item["name"]),
        ),
        "cycles": sorted(
            (_observation_view(cycle) for cycle in cycles),
            key=lambda item: (item["workId"], item["name"]),
        ),
        "reason": reason,
    }
    encoded = json.dumps(view, sort_keys=True, separators=(",", ":"), default=str)
    return hashlib.sha256(encoded.encode("utf-8")).hexdigest()[:20]


def _request_id(work_id: str, observation_revision: str) -> str:
    value = re.sub(r"[^A-Za-z0-9_.-]+", "-", work_id).strip("-.")
    value = value or "unknown-project"
    return ("project-reconcile-" + value + "-" + observation_revision)[:REQUEST_ID_LIMIT]


def _move_project(
    runner: CommandRunner,
    server: str,
    session_id: str,
    project: Mapping[str, Any],
    observation_revision: str,
) -> None:
    work_id = _work_id(project)
    if not work_id:
        raise ReconcileError(f"project { _work_name(project)!r } has no workId")
    _cli_json(
        runner,
        server,
        "work",
        "move",
        work_id,
        "init",
        "--session",
        session_id,
        "--request-id",
        _request_id(work_id, observation_revision),
    )


def reconcile(
    *,
    server: str,
    session_id: str,
    dry_run: bool = False,
    runner: CommandRunner = _run_command,
) -> dict[str, Any]:
    """Inspect one session and return a deterministic reconciliation summary."""
    session_id = session_id.strip()
    server = server.strip()
    if not session_id:
        raise ReconcileError("session id is required")
    if not server:
        raise ReconcileError("server is required")
    session = _cli_json(runner, server, "session", "show", session_id)
    if not _session_is_running(session):
        return {
            "sessionId": session_id,
            "server": server,
            "status": "skipped",
            "reason": "factory-not-running",
            "moved": [],
            "skipped": [],
        }

    works = _work_results(
        _cli_json(
            runner,
            server,
            "work",
            "list",
            "--session",
            session_id,
        )
    )
    projects = [work for work in works if _work_type(work) == "project"]
    grouped: dict[str, list[Mapping[str, Any]]] = {}
    for project in projects:
        grouped.setdefault(_work_name(project), []).append(project)

    moved: list[dict[str, Any]] = []
    skipped: list[dict[str, str]] = []
    for project_name in sorted(grouped):
        candidates = grouped[project_name]
        if not project_name:
            skipped.append({"reason": "project-without-name"})
            continue
        if len(candidates) != 1:
            skipped.append({"name": project_name, "reason": "ambiguous-project-name"})
            continue
        project = candidates[0]
        state_name, _ = _state(project)
        if state_name not in {"waiting", "blocked"}:
            continue
        if state_name == "blocked":
            skipped.append({"name": project_name, "reason": "blocked-inspect-only"})
            continue

        children = _project_children(works, project)
        project_id = _work_id(project)
        if not project_id:
            skipped.append({"name": project_name, "reason": "project-without-work-id"})
            continue

        cycles = _same_name_cycles(works, project_name)
        # A cycle of any state is left to the authored graph. Moving the
        # project while that cycle remains visible can race its completion
        # transition and create overlapping same-name cycles.
        if cycles:
            reason = "cycle-in-progress" if any(
                _state(cycle)[1] in NONTERMINAL_STATE_TYPES for cycle in cycles
            ) else "cycle-transition-pending"
            skipped.append({"name": project_name, "reason": reason})
            continue

        # With no visible cycle, a waiting lead is structurally stranded. This
        # is the public-state staleness signal; no filesystem clock is used.
        reason = "missing-cycle"

        worker_sessions = _worker_session_results(
            _cli_json(
                runner,
                server,
                "worker-sessions",
                "list",
                "--work-id",
                project_id,
                "--session",
                session_id,
            )
        )
        if _session_has_active_work(worker_sessions, project_id, session_id):
            skipped.append({"name": project_name, "reason": "project-lead-active"})
            continue

        observation_revision = _observation_revision(
            project, children, cycles, reason
        )
        unfinished_children = [
            _work_id(child)
            for child in children
            if _state(child)[1] in NONTERMINAL_STATE_TYPES
        ]
        detail: dict[str, Any] = {
            "name": project_name,
            "workId": project_id,
            "reason": reason,
            "observationRevision": observation_revision,
        }
        if unfinished_children:
            detail["unfinishedChildren"] = unfinished_children
        if dry_run:
            moved.append(detail)
            continue
        _move_project(runner, server, session_id, project, observation_revision)
        moved.append(detail)

    return {
        "sessionId": session_id,
        "server": server,
        "status": "dry-run" if dry_run else "completed",
        "moved": moved,
        "skipped": skipped,
    }


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--server", default=DEFAULT_SERVER)
    parser.add_argument("--session", required=True)
    parser.add_argument("--dry-run", action="store_true")
    return parser


def main(argv: Optional[list[str]] = None) -> int:
    args = _parser().parse_args(argv)
    try:
        result = reconcile(
            server=args.server,
            session_id=args.session,
            dry_run=args.dry_run,
        )
    except ReconcileError as error:
        print(f"project reconciliation failed: {error}", file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
