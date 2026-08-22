#!/usr/bin/env python3
"""ci-wait.py — Gate a task until its PR's required checks are terminal.

Usage: python3 factory/scripts/ci-wait.py <lane-name>

Resolves the PR whose head branch equals the lane name, then waits until
every required check on that PR is terminal (pass, fail, cancelled, or
skipped — anything that is not pending/queued/in-progress). Verdicts are the
reviewer's job: this gate does NOT care whether checks passed, only that they
finished, so reviewer agent sessions never spend time or review-loop visits
watching CI.

Outcome contract (script workers signal via exit code only):
  exit 0 -> task moves to in-review (success output)
  exit 1 -> task moves to failed (failure output)

Because the runtime gives script workers no CONTINUE outcome, this script is
one-shot: it polls internally every POLL_INTERVAL_SECONDS and exits 0 either
when checks are terminal or when the internal DEADLINE_SECONDS budget (kept
well below the runtime's 2h hard worker-execution timeout) runs out. On a
deadline exit the reviewer will observe still-pending checks and end with a
hold, which routes the task back through this gate — a cheap, bounded
re-queue rather than a lane-killing failure.

Special cases:
  - No PR yet: the processor may have just pushed; retry for ~2 minutes,
    then exit 1 with a clear stderr message.
  - PR already MERGED (and no open PR for the branch): exit 0 immediately;
    the review workstation's merged-PR short-circuit handles the rest.
  - Only CLOSED PRs: exit 0 immediately; the reviewer decides what a closed,
    unmerged PR means for the lane.
  - No checks reported: tolerated for NO_CHECKS_GRACE_SECONDS (checks can
    lag a fresh push), then treated as terminal so the reviewer sees reality.

Stdlib-only; shells out to the `gh` CLI like setup-workspace.py does for git.
"""

import json
import subprocess
import sys
import time

POLL_INTERVAL_SECONDS = 120
DEADLINE_SECONDS = 100 * 60  # one-shot budget, well below the 2h hard timeout
GH_CALL_TIMEOUT_SECONDS = 120
PR_LOOKUP_ATTEMPTS = 5
PR_LOOKUP_INTERVAL_SECONDS = 30  # ~2 minutes of retries for a just-pushed PR
NO_CHECKS_GRACE_SECONDS = 10 * 60

NON_TERMINAL_BUCKETS = {"pending"}
NON_TERMINAL_STATES = {"PENDING", "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED", "EXPECTED"}


def log(message):
    """Log progress to stderr; stdout is reserved for the final JSON result."""
    print(message, file=sys.stderr, flush=True)


def run_gh(*args):
    """Run a gh command, returning the CompletedProcess. Never raises on rc."""
    return subprocess.run(
        ["gh", *args],
        capture_output=True,
        text=True,
        timeout=GH_CALL_TIMEOUT_SECONDS,
    )


def list_prs_for_head(branch):
    """Return the PR list for a head branch, or None when gh fails."""
    result = run_gh(
        "pr", "list",
        "--head", branch,
        "--state", "all",
        "--json", "number,state",
        "--limit", "20",
    )
    if result.returncode != 0:
        log(f"gh pr list failed (exit {result.returncode}): {result.stderr.strip()}")
        return None
    try:
        return json.loads(result.stdout or "[]")
    except json.JSONDecodeError as error:
        log(f"gh pr list returned unparseable JSON: {error}")
        return None


def resolve_pr(branch):
    """Resolve the lane's PR, retrying briefly because pushes race this gate.

    Returns a dict {number, state} for the PR to gate on, preferring an OPEN
    PR, then MERGED, then CLOSED. Exits 1 when no PR exists after retries.
    """
    for attempt in range(1, PR_LOOKUP_ATTEMPTS + 1):
        prs = list_prs_for_head(branch)
        if prs:
            for state in ("OPEN", "MERGED", "CLOSED"):
                matches = [pr for pr in prs if pr.get("state") == state]
                if matches:
                    return matches[0]
        if attempt < PR_LOOKUP_ATTEMPTS:
            log(
                f"no PR found for head branch {branch!r} "
                f"(attempt {attempt}/{PR_LOOKUP_ATTEMPTS}); retrying in "
                f"{PR_LOOKUP_INTERVAL_SECONDS}s"
            )
            time.sleep(PR_LOOKUP_INTERVAL_SECONDS)

    print(
        f"ci-wait: no PR exists with head branch {branch!r} after "
        f"{PR_LOOKUP_ATTEMPTS} lookups over ~2 minutes. The process "
        "workstation must open a PR named after the lane before the task can "
        "enter review.",
        file=sys.stderr,
    )
    sys.exit(1)


def fetch_checks(pr_number, required_only):
    """Return the parsed check list, or None when gh output is unusable.

    gh pr checks uses nonzero exits for pending (8) and failing (1) checks,
    so classification relies on parsed stdout, never the exit code.
    """
    args = ["pr", "checks", str(pr_number), "--json", "name,state,bucket"]
    if required_only:
        args.insert(3, "--required")
    try:
        result = run_gh(*args)
    except subprocess.TimeoutExpired:
        log("gh pr checks timed out; will retry")
        return None
    stdout = (result.stdout or "").strip()
    if not stdout:
        stderr = (result.stderr or "").strip()
        if stderr:
            log(f"gh pr checks produced no JSON (exit {result.returncode}): {stderr}")
        return None
    try:
        checks = json.loads(stdout)
    except json.JSONDecodeError as error:
        log(f"gh pr checks returned unparseable JSON: {error}")
        return None
    return checks if isinstance(checks, list) else None


def non_terminal_checks(checks):
    """Return the subset of checks that have not finished."""
    pending = []
    for check in checks:
        bucket = str(check.get("bucket", "")).lower()
        state = str(check.get("state", "")).upper()
        if bucket in NON_TERMINAL_BUCKETS or state in NON_TERMINAL_STATES:
            pending.append(check)
    return pending


def emit_result(**fields):
    print(json.dumps({"status": "ready", **fields}, indent=2))


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <lane-name>", file=sys.stderr)
        sys.exit(1)

    branch = sys.argv[1]
    pr = resolve_pr(branch)
    pr_number = pr.get("number")
    pr_state = pr.get("state")

    if pr_state == "MERGED":
        log(f"PR #{pr_number} for {branch!r} is already MERGED; releasing to review")
        emit_result(pr=pr_number, prState=pr_state, reason="pr-merged")
        return

    if pr_state == "CLOSED":
        log(f"only CLOSED PRs exist for {branch!r} (newest #{pr_number}); releasing to review")
        emit_result(pr=pr_number, prState=pr_state, reason="pr-closed")
        return

    deadline = time.monotonic() + DEADLINE_SECONDS
    no_checks_deadline = time.monotonic() + NO_CHECKS_GRACE_SECONDS

    while True:
        checks = fetch_checks(pr_number, required_only=True)
        if not checks:
            # No required checks reported: fall back to the full check list so
            # repos or heads without a required-check rollup still gate on
            # something real.
            checks = fetch_checks(pr_number, required_only=False)

        if checks:
            pending = non_terminal_checks(checks)
            if not pending:
                log(f"all {len(checks)} reported checks on PR #{pr_number} are terminal")
                emit_result(
                    pr=pr_number,
                    prState=pr_state,
                    reason="checks-terminal",
                    checks=len(checks),
                )
                return
            names = ", ".join(str(c.get("name", "?")) for c in pending[:5])
            log(f"PR #{pr_number}: {len(pending)} non-terminal check(s): {names}")
        elif time.monotonic() >= no_checks_deadline:
            log(
                f"PR #{pr_number} reports no checks after "
                f"{NO_CHECKS_GRACE_SECONDS // 60} minutes; releasing to review"
            )
            emit_result(pr=pr_number, prState=pr_state, reason="no-checks-reported")
            return
        else:
            log(f"PR #{pr_number}: no checks reported yet; waiting for CI to register")

        if time.monotonic() + POLL_INTERVAL_SECONDS >= deadline:
            # Continue-equivalent re-queue: exit 0 so the task reaches review;
            # the reviewer observes non-terminal checks and ends with a hold,
            # which routes the task straight back through this gate. Never
            # exit 1 here — a slow CI run is not a lane failure.
            log(
                f"internal deadline ({DEADLINE_SECONDS // 60} minutes) reached with "
                f"checks still pending on PR #{pr_number}; releasing to review for a "
                "hold-and-requeue"
            )
            emit_result(pr=pr_number, prState=pr_state, reason="deadline-requeue")
            return
        time.sleep(POLL_INTERVAL_SECONDS)


if __name__ == "__main__":
    main()
