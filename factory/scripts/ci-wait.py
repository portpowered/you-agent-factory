#!/usr/bin/env python3
"""ci-wait.py — Gate a task until its PR's observed checks are terminal.

Usage: python3 factory/scripts/ci-wait.py <lane-name>

Resolves the PR whose head branch equals the lane name, then waits until the
complete current-head check set observed by review is terminal (pass, fail,
cancelled, or skipped). Verdicts are the reviewer's job: this gate does NOT
care whether checks passed, only that they finished, so reviewer agent sessions
never spend time or review-loop visits watching CI.

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
  - No PR yet: successful lookups that return no matching PR may race a
    processor push; retry for ~2 minutes, then exit 1 with a clear stderr
    message.
  - GitHub lookup unavailable: retry a bounded infrastructure budget with
    backoff, then exit 0 for the review hold-and-requeue path.
  - PR already MERGED (and no open PR for the branch): exit 0 immediately;
    the review workstation's merged-PR short-circuit handles the rest.
  - Only CLOSED PRs: exit 0 immediately; the reviewer decides what a closed,
    unmerged PR means for the lane.
  - No checks reported: tolerated for NO_CHECKS_GRACE_SECONDS (checks can
    lag a fresh push), then released with an explicit no-checks reason so the
    reviewer sees reality. It is never treated as checks-terminal.

Stdlib-only; shells out to the `gh` CLI like setup-workspace.py does for git.
"""

import json
import subprocess
import sys
import time
from dataclasses import dataclass
from enum import Enum

POLL_INTERVAL_SECONDS = 120
DEADLINE_SECONDS = 100 * 60  # one-shot budget, well below the 2h hard timeout
GH_CALL_TIMEOUT_SECONDS = 120
PR_LOOKUP_ATTEMPTS = 5
PR_LOOKUP_INTERVAL_SECONDS = 30  # ~2 minutes of retries for a just-pushed PR
PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS = 3
PR_LOOKUP_INFRASTRUCTURE_BACKOFF_SECONDS = 5
PR_LOOKUP_INFRASTRUCTURE_MAX_BACKOFF_SECONDS = 60
NO_CHECKS_GRACE_SECONDS = 10 * 60
PR_VIEW_JSON_FIELDS = "number,state,headRefOid,statusCheckRollup"
PR_CHECKS_JSON_FIELDS = "name,state,bucket,link,workflow,startedAt,completedAt"
CONVERGENCE_OBSERVATIONS = 2

NON_TERMINAL_BUCKETS = {"pending"}
NON_TERMINAL_STATES = {"PENDING", "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED", "EXPECTED"}
TERMINAL_STATES = {
    "ACTION_REQUIRED",
    "CANCELLED",
    "ERROR",
    "FAILURE",
    "NEUTRAL",
    "SKIPPED",
    "STALE",
    "STARTUP_FAILURE",
    "SUCCESS",
    "TIMED_OUT",
}
KNOWN_CHECK_BUCKETS = {"cancel", "fail", "pass", "pending", "skipping"}
PR_STATE_PREFERENCE = ("OPEN", "MERGED", "CLOSED")


class PRLookupStatus(Enum):
    """Classification of a GitHub PR lookup response."""

    FOUND = "found"
    NOT_FOUND = "not-found"
    INFRASTRUCTURE_FAILURE = "infrastructure-failure"


@dataclass(frozen=True)
class PRLookupResult:
    """A typed result that keeps an empty response distinct from a failure."""

    status: PRLookupStatus
    prs: tuple = ()


class JSONReadStatus(Enum):
    """Classification of one bounded JSON response from gh."""

    OK = "ok"
    EMPTY = "empty"
    MALFORMED = "malformed"
    UNAVAILABLE = "unavailable"


@dataclass(frozen=True)
class JSONRead:
    """A parsed gh response with an explicit failure classification."""

    status: JSONReadStatus
    value: object = None


class SnapshotStatus(Enum):
    """Classification of the reconciled current-head observation."""

    VALID = "valid"
    EMPTY = "empty"
    UNCERTAIN = "uncertain"


@dataclass(frozen=True)
class CurrentHeadSnapshot:
    """One bounded, same-head observation of the review-visible check set."""

    status: SnapshotStatus
    reason: str
    head_ref_oid: str = ""
    checks: tuple = ()
    observed_heads: tuple = ()

    def fingerprint(self):
        """Return the stable evidence used for convergence across snapshots."""
        return (
            self.head_ref_oid,
            tuple(
                (
                    check["identity"],
                    check["state"],
                    check["bucket"],
                    check.get("link"),
                    check.get("workflow"),
                )
                for check in self.checks
            ),
        )


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
    """Return a typed result for the lane's GitHub PR lookup."""
    try:
        result = run_gh(
            "pr", "list",
            "--head", branch,
            "--state", "all",
            "--json", "number,state",
            "--limit", "20",
        )
    except subprocess.TimeoutExpired:
        log("gh pr list timed out; treating lookup as an infrastructure failure")
        return PRLookupResult(PRLookupStatus.INFRASTRUCTURE_FAILURE)
    except OSError:
        log("gh pr list could not be executed; treating lookup as an infrastructure failure")
        return PRLookupResult(PRLookupStatus.INFRASTRUCTURE_FAILURE)

    if result.returncode != 0:
        log(
            f"gh pr list failed (exit {result.returncode}); "
            "treating lookup as an infrastructure failure"
        )
        return PRLookupResult(PRLookupStatus.INFRASTRUCTURE_FAILURE)

    stdout = (result.stdout or "").strip()
    if not stdout:
        log(
            "gh pr list returned no output; treating lookup as an infrastructure "
            "failure"
        )
        return PRLookupResult(PRLookupStatus.INFRASTRUCTURE_FAILURE)

    try:
        prs = json.loads(stdout)
    except (TypeError, json.JSONDecodeError):
        log(
            "gh pr list returned unparseable JSON; treating lookup as an "
            "infrastructure failure"
        )
        return PRLookupResult(PRLookupStatus.INFRASTRUCTURE_FAILURE)

    if not isinstance(prs, list) or any(
        not isinstance(pr, dict)
        or not isinstance(pr.get("number"), int)
        or pr.get("state") not in PR_STATE_PREFERENCE
        for pr in prs
    ):
        log(
            "gh pr list returned unusable JSON; treating lookup as an infrastructure "
            "failure"
        )
        return PRLookupResult(PRLookupStatus.INFRASTRUCTURE_FAILURE)

    status = PRLookupStatus.FOUND if prs else PRLookupStatus.NOT_FOUND
    return PRLookupResult(status, tuple(prs))


def infrastructure_backoff_seconds(attempt):
    """Return bounded exponential backoff for an infrastructure retry."""
    return min(
        PR_LOOKUP_INFRASTRUCTURE_BACKOFF_SECONDS * (2 ** (attempt - 1)),
        PR_LOOKUP_INFRASTRUCTURE_MAX_BACKOFF_SECONDS,
    )


def release_for_infrastructure_requeue(branch, attempts):
    """Emit the successful result that routes dependency failure to review."""
    log(
        f"ci-wait: GitHub PR lookup infrastructure retry budget exhausted "
        f"for head branch {branch!r} after {attempts} failures; releasing to "
        "review for hold-and-requeue"
    )
    emit_result(
        branch=branch,
        reason="pr-lookup-infrastructure-requeue",
        lookup=PRLookupStatus.INFRASTRUCTURE_FAILURE.value,
        infrastructureAttempts=attempts,
    )


def resolve_pr(branch):
    """Resolve the lane's PR with separate absence and infrastructure budgets.

    Returns a dict {number, state} for the PR to gate on, preferring an OPEN
    PR, then MERGED, then CLOSED. Exits 1 when successful empty lookups exhaust
    the missing-PR budget. Returns None after an infrastructure budget is
    exhausted, having emitted the exit-0 requeue result.
    """
    successful_empty_lookups = 0
    infrastructure_failures = 0
    while successful_empty_lookups < PR_LOOKUP_ATTEMPTS:
        lookup = list_prs_for_head(branch)
        if lookup.status == PRLookupStatus.INFRASTRUCTURE_FAILURE:
            infrastructure_failures += 1
            if infrastructure_failures >= PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS:
                release_for_infrastructure_requeue(branch, infrastructure_failures)
                return None

            backoff = infrastructure_backoff_seconds(infrastructure_failures)
            log(
                f"ci-wait: GitHub PR lookup infrastructure failure for head branch "
                f"{branch!r} (attempt {infrastructure_failures}/"
                f"{PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS}); retrying in {backoff}s"
            )
            time.sleep(backoff)
            continue

        # A successful response means the dependency recovered. Do not let
        # earlier transport failures consume the separate infrastructure
        # budget or change the meaning of this response.
        infrastructure_failures = 0

        if lookup.status == PRLookupStatus.NOT_FOUND:
            successful_empty_lookups += 1
            log(
                f"successful PR lookup found no matching PR for head branch "
                f"{branch!r} (successful empty lookup "
                f"{successful_empty_lookups}/{PR_LOOKUP_ATTEMPTS})"
            )
            if successful_empty_lookups < PR_LOOKUP_ATTEMPTS:
                log(
                    f"ci-wait: retrying successful empty PR lookup for head branch "
                    f"{branch!r} in {PR_LOOKUP_INTERVAL_SECONDS}s"
                )
                time.sleep(PR_LOOKUP_INTERVAL_SECONDS)
            continue

        for state in PR_STATE_PREFERENCE:
            matches = [pr for pr in lookup.prs if pr.get("state") == state]
            if matches:
                return matches[0]

        # Valid gh output currently has only OPEN, MERGED, or CLOSED states,
        # so reaching this point would indicate a future response shape that
        # cannot be selected safely. Treat it like an infrastructure failure
        # without spending a successful-not-found attempt.
        infrastructure_failures += 1
        log(
            f"ci-wait: GitHub PR lookup returned no selectable PR for head branch "
            f"{branch!r}; treating response as an infrastructure failure"
        )
        if infrastructure_failures >= PR_LOOKUP_INFRASTRUCTURE_ATTEMPTS:
            release_for_infrastructure_requeue(branch, infrastructure_failures)
            return None
        backoff = infrastructure_backoff_seconds(infrastructure_failures)
        time.sleep(backoff)

    print(
        f"ci-wait: successful PR lookups found no PR for head branch "
        f"{branch!r} after {successful_empty_lookups} successful lookups. "
        "The process workstation must open a PR named after the lane before "
        "the task can enter review.",
        file=sys.stderr,
    )
    sys.exit(1)


def read_gh_json(label, *args):
    """Read one JSON response without leaking dependency stderr or payloads."""
    try:
        result = run_gh(*args)
    except subprocess.TimeoutExpired:
        log(f"{label} timed out; treating the observation as unavailable")
        return JSONRead(JSONReadStatus.UNAVAILABLE)
    except OSError:
        log(f"{label} could not be executed; treating the observation as unavailable")
        return JSONRead(JSONReadStatus.UNAVAILABLE)

    stdout = (result.stdout or "").strip()
    if not stdout:
        status = JSONReadStatus.UNAVAILABLE if result.returncode else JSONReadStatus.EMPTY
        log(
            f"{label} returned no JSON (exit {result.returncode}); "
            f"treating the observation as {status.value}"
        )
        return JSONRead(status)

    try:
        value = json.loads(stdout)
    except (TypeError, json.JSONDecodeError) as error:
        position = error.pos if hasattr(error, "pos") else "?"
        log(f"{label} returned unparseable JSON at position {position}")
        return JSONRead(JSONReadStatus.MALFORMED)

    # gh pr checks uses nonzero exits for pending (8) and failing (1) checks.
    # A schema-valid stdout response remains useful regardless of that signal.
    return JSONRead(JSONReadStatus.OK, value)


def fetch_pr_view(pr_number):
    """Read the PR head and status-check rollup for one bounded observation."""
    return read_gh_json(
        "gh pr view",
        "pr",
        "view",
        str(pr_number),
        "--json",
        PR_VIEW_JSON_FIELDS,
    )


def fetch_checks(pr_number):
    """Read the full review-observed check list; required-only is not used."""
    return read_gh_json(
        "gh pr checks",
        "pr",
        "checks",
        str(pr_number),
        "--json",
        PR_CHECKS_JSON_FIELDS,
    )


def _non_empty_text(value):
    """Return a stripped string or None for an absent/invalid text field."""
    if not isinstance(value, str):
        return None
    value = value.strip()
    return value or None


def _check_kind(check, source):
    """Return the stable GitHub check type used in an identity."""
    explicit_kind = check.get("__typename") or check.get("type") or check.get("kind")
    if explicit_kind is not None:
        if explicit_kind in {"CheckRun", "StatusContext"}:
            return explicit_kind
        return None
    if _non_empty_text(check.get("context")):
        return "StatusContext"
    if source == "checks" or _non_empty_text(check.get("name")):
        return "CheckRun"
    return None


def _check_state(check):
    """Normalize gh check-run/status states into a terminality vocabulary."""
    raw_state = check.get("state")
    if raw_state is None:
        raw_state = check.get("status")
    state = _non_empty_text(raw_state)
    conclusion = _non_empty_text(check.get("conclusion"))
    if state:
        state = state.upper().replace("-", "_")
    if conclusion:
        conclusion = conclusion.upper().replace("-", "_")

    if state in {"COMPLETED", "COMPLETE", "DONE"}:
        state = conclusion
    elif state is None:
        state = conclusion

    if state == "CANCELED":
        state = "CANCELLED"
    return state


def _canonical_bucket(state):
    """Map a known state to the conservative review bucket."""
    if state in NON_TERMINAL_STATES:
        return "pending"
    if state == "SUCCESS":
        return "pass"
    if state in {"NEUTRAL", "SKIPPED"}:
        return "skipping"
    return "fail"


def _normalize_check(check, source):
    """Return one identity-complete check row or a bounded rejection reason."""
    if not isinstance(check, dict):
        return None, "malformed-check-row"

    kind = _check_kind(check, source)
    if kind is None:
        return None, "unknown-check-type"
    name = _non_empty_text(check.get("name")) or _non_empty_text(check.get("context"))
    if name is None:
        return None, "unknown-check-name"

    link = None
    for field in ("link", "detailsUrl", "targetUrl", "url", "htmlUrl"):
        link = _non_empty_text(check.get(field))
        if link:
            break
    workflow = _non_empty_text(check.get("workflow")) or _non_empty_text(
        check.get("workflowName")
    )
    stable_ref = link
    if stable_ref is None and workflow is not None:
        stable_ref = f"workflow:{workflow}"
    if stable_ref is None and kind == "StatusContext":
        stable_ref = f"context:{name}"
    if stable_ref is None:
        return None, "unknown-check-identity"

    state = _check_state(check)
    if state not in NON_TERMINAL_STATES and state not in TERMINAL_STATES:
        return None, "unknown-check-state"
    bucket = _canonical_bucket(state)
    supplied_bucket = check.get("bucket")
    if supplied_bucket is not None:
        supplied_bucket = _non_empty_text(supplied_bucket)
        if supplied_bucket is None or supplied_bucket.lower() not in KNOWN_CHECK_BUCKETS:
            return None, "unknown-check-bucket"
        supplied_bucket = supplied_bucket.lower()
        if supplied_bucket != bucket and not (
            state == "CANCELLED" and supplied_bucket == "cancel"
        ):
            return None, "check-state-bucket-mismatch"

    identity = f"{kind}|{name}|{stable_ref}"
    return (
        {
            "identity": identity,
            "name": name,
            "workflow": workflow,
            "link": link,
            "state": state,
            "bucket": bucket,
        },
        None,
    )


def _normalize_check_list(checks, source):
    """Normalize a non-empty check list and reject duplicate/unknown rows."""
    if not isinstance(checks, list):
        return None, "malformed-check-list"
    normalized = {}
    for check in checks:
        normalized_check, reason = _normalize_check(check, source)
        if normalized_check is None:
            return None, reason
        identity = normalized_check["identity"]
        if identity in normalized:
            return None, "duplicate-check-identity"
        normalized[identity] = normalized_check
    return normalized, None


def _merge_check_maps(left, right, require_same_keys=False):
    """Reconcile two normalized sources without hiding state changes."""
    if require_same_keys and set(left) != set(right):
        return None, "check-set-changed-during-observation"

    merged = {}
    for identity in sorted(set(left) | set(right)):
        left_check = left.get(identity)
        right_check = right.get(identity)
        if left_check is not None and right_check is not None:
            if (
                left_check["state"] != right_check["state"]
                or left_check["bucket"] != right_check["bucket"]
            ):
                return None, "check-state-mismatch"
            check = dict(left_check)
            if check["link"] is None:
                check["link"] = right_check["link"]
            if check["workflow"] is None:
                check["workflow"] = right_check["workflow"]
            merged[identity] = check
        else:
            merged[identity] = dict(left_check or right_check)
    return merged, None


def _view_parts(read, pr_number):
    """Validate and unpack one PR view response."""
    if read.status != JSONReadStatus.OK:
        return None, f"view-{read.status.value}"
    if not isinstance(read.value, dict):
        return None, "malformed-pr-view"
    if read.value.get("number") != pr_number:
        return None, "pr-number-mismatch"
    if _non_empty_text(read.value.get("state")) != "OPEN":
        return None, "pr-state-changed"
    head_ref_oid = _non_empty_text(read.value.get("headRefOid"))
    if head_ref_oid is None:
        return None, "unknown-head"
    rollup = read.value.get("statusCheckRollup")
    if not isinstance(rollup, list):
        return None, "malformed-status-check-rollup"
    return (head_ref_oid, rollup), None


def _head_hint(read):
    """Extract a safe head hint for a bounded diagnostic."""
    if read.status == JSONReadStatus.OK and isinstance(read.value, dict):
        return _non_empty_text(read.value.get("headRefOid")) or ""
    return ""


def _observed_heads(*heads):
    """Keep ordered, non-empty head identities without duplicates."""
    return tuple(dict.fromkeys(head for head in heads if head))


def observe_current_head(pr_number):
    """Take one bounded before/checks/after observation of the PR."""
    before_read = fetch_pr_view(pr_number)
    checks_read = fetch_checks(pr_number)
    after_read = fetch_pr_view(pr_number)

    before, before_reason = _view_parts(before_read, pr_number)
    after, after_reason = _view_parts(after_read, pr_number)
    before_head = before[0] if before else _head_hint(before_read)
    after_head = after[0] if after else _head_hint(after_read)
    heads = _observed_heads(before_head, after_head)
    head_ref_oid = after_head or before_head

    if before_reason is not None:
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            f"before-{before_reason}",
            head_ref_oid,
            observed_heads=heads,
        )
    if after_reason is not None:
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            f"after-{after_reason}",
            head_ref_oid,
            observed_heads=heads,
        )
    if before_head != after_head:
        log(
            f"PR #{pr_number} head changed during checks observation: "
            f"{before_head} -> {after_head}"
        )
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            "head-changed-during-observation",
            after_head,
            observed_heads=heads,
        )

    checks = checks_read.value if checks_read.status == JSONReadStatus.OK else None
    if checks_read.status != JSONReadStatus.OK:
        if not before[1] and not after[1] and checks_read.status == JSONReadStatus.EMPTY:
            return CurrentHeadSnapshot(
                SnapshotStatus.EMPTY,
                "empty-current-head-check-set",
                head_ref_oid,
                observed_heads=heads,
            )
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            f"checks-{checks_read.status.value}",
            head_ref_oid,
            observed_heads=heads,
        )
    if not isinstance(checks, list):
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            "malformed-check-list",
            head_ref_oid,
            observed_heads=heads,
        )
    if not before[1] or not after[1] or not checks:
        if not before[1] and not after[1] and not checks:
            return CurrentHeadSnapshot(
                SnapshotStatus.EMPTY,
                "empty-current-head-check-set",
                head_ref_oid,
                observed_heads=heads,
            )
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            "incomplete-current-head-check-set",
            head_ref_oid,
            observed_heads=heads,
        )

    before_map, reason = _normalize_check_list(before[1], "rollup")
    if reason is not None:
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            f"before-{reason}",
            head_ref_oid,
            observed_heads=heads,
        )
    after_map, reason = _normalize_check_list(after[1], "rollup")
    if reason is not None:
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            f"after-{reason}",
            head_ref_oid,
            checks=tuple(before_map.values()),
            observed_heads=heads,
        )
    checks_map, reason = _normalize_check_list(checks, "checks")
    if reason is not None:
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            f"checks-{reason}",
            head_ref_oid,
            checks=tuple(sorted(before_map.values(), key=lambda check: check["identity"])),
            observed_heads=heads,
        )

    rollup_map, reason = _merge_check_maps(
        before_map,
        after_map,
        require_same_keys=True,
    )
    if reason is not None:
        diagnostic_checks = before_map
        if reason == "check-set-changed-during-observation":
            # The union is diagnostic only: the differing snapshots remain
            # uncertain and cannot be used as terminal evidence. Including
            # both sides makes a delayed registration visible to review.
            diagnostic_checks, _ = _merge_check_maps(before_map, after_map)
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            reason,
            head_ref_oid,
            checks=tuple(
                sorted(
                    diagnostic_checks.values(), key=lambda check: check["identity"]
                )
            ),
            observed_heads=heads,
        )
    reconciled, reason = _merge_check_maps(rollup_map, checks_map)
    if reason is not None:
        return CurrentHeadSnapshot(
            SnapshotStatus.UNCERTAIN,
            reason,
            head_ref_oid,
            checks=tuple(sorted(reconciled.values(), key=lambda check: check["identity"]))
            if reconciled
            else tuple(sorted(rollup_map.values(), key=lambda check: check["identity"])),
            observed_heads=heads,
        )

    return CurrentHeadSnapshot(
        SnapshotStatus.VALID,
        "stable-current-head-observation",
        head_ref_oid,
        checks=tuple(reconciled[identity] for identity in sorted(reconciled)),
        observed_heads=heads,
    )


def non_terminal_checks(checks):
    """Return the subset of normalized checks that have not finished."""
    pending = []
    for check in checks:
        if not isinstance(check, dict):
            pending.append(check)
            continue
        bucket = str(check.get("bucket", "")).lower()
        state = str(check.get("state", "")).upper()
        if bucket in NON_TERMINAL_BUCKETS or state in NON_TERMINAL_STATES:
            pending.append(check)
    return pending


def emit_result(**fields):
    print(json.dumps({"status": "ready", **fields}, indent=2))


def snapshot_fields(snapshot):
    """Return additive, review-visible evidence for a snapshot."""
    fields = {
        "checks": len(snapshot.checks),
        "headRefOid": snapshot.head_ref_oid or None,
        "checkIdentities": list(snapshot.checks),
    }
    pending = non_terminal_checks(snapshot.checks)
    if pending:
        fields["pendingChecks"] = pending
    return fields


def snapshot_uncertainty(snapshot, reason=None):
    """Return bounded uncertainty without treating it as terminal evidence."""
    uncertainty = {"reason": reason or snapshot.reason}
    if snapshot.head_ref_oid:
        uncertainty["headRefOid"] = snapshot.head_ref_oid
    if snapshot.observed_heads:
        uncertainty["observedHeads"] = list(snapshot.observed_heads)
    return uncertainty


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <lane-name>", file=sys.stderr)
        sys.exit(1)

    branch = sys.argv[1]
    pr = resolve_pr(branch)
    if pr is None:
        return
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
    candidate_fingerprint = None
    convergence_count = 0
    while True:
        snapshot = observe_current_head(pr_number)
        now = time.monotonic()
        pending = non_terminal_checks(snapshot.checks)

        if snapshot.status == SnapshotStatus.VALID:
            if pending:
                candidate_fingerprint = None
                convergence_count = 0
                names = ", ".join(
                    f"{check.get('name', '?')}="
                    f"{check.get('state', '?')} ({check.get('identity', '?')})"
                    for check in pending[:5]
                )
                log(
                    f"PR #{pr_number} current head {snapshot.head_ref_oid}: "
                    f"{len(pending)} non-terminal check(s): {names}"
                )
            else:
                fingerprint = snapshot.fingerprint()
                if fingerprint == candidate_fingerprint:
                    convergence_count += 1
                else:
                    candidate_fingerprint = fingerprint
                    convergence_count = 1
                if convergence_count >= CONVERGENCE_OBSERVATIONS:
                    log(
                        f"all {len(snapshot.checks)} checks on current head "
                        f"{snapshot.head_ref_oid} are terminal after "
                        f"{convergence_count} stable observations"
                    )
                    emit_result(
                        pr=pr_number,
                        prState=pr_state,
                        reason="checks-terminal",
                        **snapshot_fields(snapshot),
                        uncertainty=None,
                    )
                    return
                log(
                    f"PR #{pr_number} current head {snapshot.head_ref_oid}: "
                    f"terminal candidate observed; awaiting same-head convergence "
                    f"({convergence_count}/{CONVERGENCE_OBSERVATIONS})"
                )
        elif snapshot.status == SnapshotStatus.EMPTY:
            candidate_fingerprint = None
            convergence_count = 0
            if snapshot.reason == "empty-current-head-check-set" and now >= no_checks_deadline:
                log(
                    f"PR #{pr_number} current head {snapshot.head_ref_oid or 'unknown'} "
                    f"reports no checks after {NO_CHECKS_GRACE_SECONDS // 60} minutes; "
                    "releasing with explicit no-check uncertainty"
                )
                emit_result(
                    pr=pr_number,
                    prState=pr_state,
                    reason="no-checks-reported",
                    **snapshot_fields(snapshot),
                    uncertainty=snapshot_uncertainty(snapshot),
                )
                return
            log(
                f"PR #{pr_number} current head {snapshot.head_ref_oid or 'unknown'}: "
                "no complete check set reported yet; waiting for CI to register"
            )
        else:
            candidate_fingerprint = None
            convergence_count = 0
            log(
                f"PR #{pr_number} current-head observation uncertain "
                f"({snapshot.reason}) at {snapshot.head_ref_oid or 'unknown'}; retrying"
            )

        if now + POLL_INTERVAL_SECONDS >= deadline:
            # Continue-equivalent re-queue: exit 0 so the task reaches review;
            # the reviewer observes the bounded evidence and can hold/requeue.
            # The convergence comparison above, not this polling interval, is
            # the completeness proof.
            if snapshot.status == SnapshotStatus.VALID and pending:
                deadline_reason = "non-terminal-checks"
            elif snapshot.status == SnapshotStatus.VALID:
                deadline_reason = "terminal-candidate-not-converged"
            else:
                deadline_reason = snapshot.reason
            log(
                f"internal deadline ({DEADLINE_SECONDS // 60} minutes) reached for "
                f"PR #{pr_number}; releasing to review for hold-and-requeue "
                f"with bounded uncertainty {deadline_reason}"
            )
            emit_result(
                pr=pr_number,
                prState=pr_state,
                reason="deadline-requeue",
                **snapshot_fields(snapshot),
                uncertainty=snapshot_uncertainty(snapshot, deadline_reason),
            )
            return
        time.sleep(POLL_INTERVAL_SECONDS)


if __name__ == "__main__":
    main()
