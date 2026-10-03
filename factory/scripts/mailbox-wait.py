#!/usr/bin/env python3
"""mailbox-wait.py - park an asking lane on the operator mailbox without a slot.

TEMPORARY (AM-T0 stopgap): removed by AM-T11 together with the
`task:awaiting-answer` / `idea:awaiting-answer` states, the `mailbox-wait` /
`mailbox-wait-idea` workstations and the `mailbox-waiter` worker.

Usage:
  python3 factory/scripts/mailbox-wait.py <lane-name> <work-type> <continue-feedback>

The `process` and `plan` workstations route every CONTINUE through an
`awaiting-answer` state that this SCRIPT_RUN poller consumes. It holds no
`executor-slot`, exactly like `ci-wait`, so a parked lane costs no agent slot
and no process/review visit while it waits.

A CONTINUE parks only when its feedback starts with PARK_MARKER and the lane's
request file exists in the operator mailbox. Every other CONTINUE passes
through at once so the authored behavior is unchanged:

  task, ordinary CONTINUE               -> exit 0 (task returns to init)
  idea, ordinary CONTINUE               -> exit 1 (legacy route: reporting-failed)

For a marked CONTINUE with a request file present:

  an undelivered response appears       -> exit 0 (work returns to init)
  only an already-delivered response,
  not older than the request            -> task: exit 0 (init, no wait);
                                           idea: exit 1 (reporting-failed)
  wait window (60 min from the request
  file's mtime) runs out                -> exit 0 (init; the prompt's stated
                                           no-answer path is taken)
  window had already run out on entry   -> task: exit 0 (init, no wait);
  (the same unanswered request parked     idea: exit 1 (reporting-failed, so a
  twice)                                  planner cannot loop forever)

A marked CONTINUE without a request file has nothing to wait on: a task returns
to init and an idea takes the legacy reporting-failed route.

Delivered response versions (mtime) are recorded under `waits/<lane>.delivered`
in the mailbox so a lane that re-parks on an answer it already received cannot
loop. Script workers signal through the exit code only; stdout becomes the work's
last output and stderr carries diagnostics. The mailbox directory is
`<main checkout>/docs/temp/operator-mailbox`, where the main checkout is the
parent of `git rev-parse --git-common-dir`. `YOU_OPERATOR_MAILBOX_DIR`
overrides it (tests).
"""

import os
import subprocess
import sys
import time
from pathlib import Path

PARK_MARKER = "AWAITING_OPERATOR_ANSWER"
POLL_INTERVAL_SECONDS = 30
WAIT_WINDOW_SECONDS = 60 * 60
WORK_TYPES = {"task", "idea"}


def env_seconds(name, default):
    value = os.environ.get(name, "").strip()
    if not value:
        return default
    try:
        parsed = float(value)
    except ValueError:
        return default
    return parsed if parsed >= 0 else default


def mailbox_dir():
    override = os.environ.get("YOU_OPERATOR_MAILBOX_DIR", "").strip()
    if override:
        return Path(override)
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
            capture_output=True,
            text=True,
            check=False,
            timeout=30,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    common = result.stdout.strip()
    if result.returncode != 0 or not common:
        return None
    return Path(common).parent / "docs" / "temp" / "operator-mailbox"


def is_safe_lane(lane):
    return bool(lane) and lane not in {".", ".."} and not any(c in lane for c in "/\\:")


def response_stamp(response):
    try:
        return str(response.stat().st_mtime_ns)
    except OSError:
        return ""


def already_delivered(delivered, response):
    try:
        recorded = delivered.read_text(encoding="utf-8").strip()
    except OSError:
        return False
    return bool(recorded) and recorded == response_stamp(response)


def deliver(delivered, response):
    """Return the work to init and remember which response version it carried."""
    try:
        delivered.parent.mkdir(parents=True, exist_ok=True)
        delivered.write_text(response_stamp(response) + "\n", encoding="utf-8")
    except OSError as error:
        print(f"mailbox-wait: could not record delivery: {error}", file=sys.stderr)
    print(f"mailbox-wait: response present at {response}; read it first")
    return 0


def pass_through(work_type, reason):
    """Leave the park without waiting: task -> init, idea -> legacy failure route."""
    if work_type == "task":
        print(f"mailbox-wait: {reason}; returning the task to init")
        return 0
    print(
        f"mailbox-wait: {reason}; keeping the planner's legacy CONTINUE route "
        "to reporting-failed",
        file=sys.stderr,
    )
    return 1


def run(argv, now=time.time, sleep=time.sleep):
    if len(argv) < 3:
        print(
            "usage: mailbox-wait.py <lane-name> <work-type> [continue-feedback]",
            file=sys.stderr,
        )
        return 1
    lane = argv[1].strip()
    work_type = argv[2].strip()
    feedback = argv[3] if len(argv) > 3 else ""
    if work_type not in WORK_TYPES:
        print(f"mailbox-wait: unsupported work type {work_type!r}", file=sys.stderr)
        return 1
    if not feedback.lstrip().startswith(PARK_MARKER):
        return pass_through(work_type, "ordinary CONTINUE (no mailbox marker)")
    if not is_safe_lane(lane):
        return pass_through(work_type, f"unsafe lane name {lane!r}")

    mailbox = mailbox_dir()
    if mailbox is None:
        return pass_through(work_type, "operator mailbox directory not found")
    request = mailbox / "requests" / f"{lane}.md"
    response = mailbox / "responses" / f"{lane}.md"

    delivered = mailbox / "waits" / f"{lane}.delivered"

    def fresh_response():
        return response.exists() and not already_delivered(delivered, response)

    if fresh_response():
        return deliver(delivered, response)
    if not request.exists():
        return pass_through(work_type, f"marked CONTINUE but no request file at {request}")
    try:
        request_mtime = request.stat().st_mtime
    except OSError:
        return pass_through(work_type, f"request file vanished at {request}")
    if response.exists() and request_mtime <= response.stat().st_mtime:
        return pass_through(
            work_type,
            f"response {response} was already delivered to an earlier visit; "
            "act on it instead of parking again",
        )

    window = env_seconds("MAILBOX_WAIT_WINDOW_SECONDS", WAIT_WINDOW_SECONDS)
    poll = env_seconds("MAILBOX_WAIT_POLL_SECONDS", POLL_INTERVAL_SECONDS)
    deadline = request_mtime + window
    if now() >= deadline:
        return pass_through(
            work_type,
            f"no answer: request {request} already waited its full window; "
            "take the stated no-answer path",
        )

    print(f"mailbox-wait: parked on {request}", file=sys.stderr, flush=True)
    while True:
        if fresh_response():
            return deliver(delivered, response)
        remaining = deadline - now()
        if remaining <= 0:
            print(
                f"mailbox-wait: no answer within the wait window for {request}; "
                "take the stated no-answer path"
            )
            return 0
        sleep(min(poll, remaining))


def main():
    return run(sys.argv)


if __name__ == "__main__":
    sys.exit(main())
