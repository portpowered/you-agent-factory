# C09 Work-watch contention

## Source-plan authority and conflict resolution

The task packet names `docs/temp/functional-test-optimization.md` as its
`context.sourcePlan`. That path is a gitignored temporary pointer and is not
available in this checkout. The read-only authority checks performed on
2026-08-28 were:

- `Test-Path docs/temp/functional-test-optimization.md` -> `False`;
- `git ls-tree -r origin/main -- docs/temp/functional-test-optimization.md` ->
  no entry; and
- `git log --all --full-history --oneline --
  docs/temp/functional-test-optimization.md` -> no entry.

This tracked c09 artifact is the explicit reviewable replacement authority and
evidence record for the two retained source-plan references. The PRD remains
the task packet's scope and acceptance authority; this resolution does not
change it. `Scope 7 — winning raised-parallelism configuration must remain
green` is preserved as the raised `jobs=8` PR-CI gate owned by review, without
a local wall-clock threshold. `Functional test-case discipline` is preserved
by the complete CASE-WW-001 through CASE-WW-017 matrix and the existing or
not-applicable ownership recorded below. No source-plan requirement is
removed, weakened, or reinterpreted, and no production, support, baseline,
workflow, sibling, or generated surface is added. The absent temporary pointer
is not used as a silent substitute or as a reason to broaden this lane.

## Pre-change CI diagnostic

- Run: `33220840795`
- Job: `99014418744` (`Backend Functional Coverage`)
- Head: `63937e9878f2d1a1b1d86964c201e24a62d2d1d0`
- Functional runner configuration: `jobs=8` on a four-logical-CPU runner
- Exact selector: `TestWorkWatchFollowsStateTransitionsUntilTerminal`
- Package elapsed: `3.336s`
- Test elapsed: `1.300s`
- Artifact: `functional-test-diagnostics` (`9705215279`)
- Run URL: https://github.com/portpowered/you-agent-factory/actions/runs/33220840795

The exact raw diagnostic retained by the CI diagnostics artifact was:

```text
watch_test.go:89: Process.Execute(work watch) error = CLI_COMMAND_FAILED: command failed: work watch stream for session "~default": reduce Work watch event "factory-event/work-state-change/batch-request-644648ee-ddd2-4bfb-95fe-8f3b5d426f50-...
```

The retained event prefix is:

```text
factory-event/work-state-change/batch-request-644648ee-ddd2-4bfb-95fe-8f3b5d426f50-...
```

The reducer suffix is unresolved because the CI artifact had already truncated
the diagnostic at that prefix. The artifact does not recover the omitted
event identity, sequence, or reducer detail.

## Pre-change test ordering

Before this correction, `watch_test.go` performed the following operations:

1. Line 73 started the finite watch asynchronously with `support.StartProcessCommand`.
2. Lines 74–88 immediately executed the `processing` move and then the
   `complete` move through the separate move process.
3. Line 89 awaited the watch process only after both moves had been submitted.

That ordering did not observe attachment of the exact public Factory Event
stream before publishing transitions. Under package contention, the watch could
therefore reduce a transition before its retained/live stream boundary had been
deterministically established.

## Local characterization

Before implementation, the focused non-reproduction was recorded as:

```text
go test -count=25 -run '^TestWorkWatchFollowsStateTransitionsUntilTerminal$' -timeout=10m ./tests/functional/work/watch
```

It passed all 25 repeats in `80.658s`. This did not disprove the raised-
concurrency CI failure or recover the unresolved reducer suffix; it only
characterized that the existing race was not reproduced by the local run.

## Implementation evidence

The package-local gate is a reverse proxy over the existing functional API
server. It signals only after the exact `GET /factory-sessions/~default/events`
response is `200 OK`, has `text/event-stream` content type, and carries the
retained-count header. The finite watcher is given the gate URL, and the two
public `work move` commands continue to use the real API server. This keeps the
root-built Process/CLI/HTTP/Factory Event path intact while making attachment
observable before either transition is published.

The focused and lifecycle procedures produced these results:

```text
go test -count=20 -run '^TestWorkWatchFollowsStateTransitionsUntilTerminal$' -timeout=10m ./tests/functional/work/watch
ok github.com/portpowered/infinite-you/tests/functional/work/watch 52.115s

go test -count=1 -run '^TestWorkWatchControlledLifecycleCases$' -timeout=10m -v ./tests/functional/work/watch
--- PASS: TestWorkWatchControlledLifecycleCases (2.11s)
ok github.com/portpowered/infinite-you/tests/functional/work/watch 2.177s

go test -race -count=1 -run '^(TestWorkWatchFollowsStateTransitionsUntilTerminal|TestWorkWatchRecordedProductionRetryLedger)$' -timeout=10m ./tests/functional/work/watch
ok github.com/portpowered/infinite-you/tests/functional/work/watch 11.950s

go test -race -count=1 -run '^TestWorkWatchControlledLifecycleCases$' -timeout=10m ./tests/functional/work/watch
ok github.com/portpowered/infinite-you/tests/functional/work/watch 12.198s

go test -count=1 -timeout=10m ./tests/functional/work/watch
ok github.com/portpowered/infinite-you/tests/functional/work/watch 8.875s
```

The functional test runner emitted `PASS`/`ok` for each procedure. On this
shared four-logical-CPU host, the Go driver remained resident after emitting
the result; the bounded driver processes were stopped after the result was
captured. This is host/process contention, not a test failure, and no local
wall-clock threshold is used as an acceptance condition.

The witnesses now cover CASE-WW-001 through CASE-WW-012 and CASE-WW-016:

- CASE-WW-001 uses the real submit/watch/two-move spine and the observable
  public stream gate.
- CASE-WW-002 through CASE-WW-007 and CASE-WW-009 through CASE-WW-012 use
  root-built public CLI processes against a controlled HTTP SSE boundary,
  asserting complete NDJSON, duplicate/conflict behavior, cancellation and
  deadline classification, cursor recovery, and stream cleanup.
- CASE-WW-008 and CASE-WW-017 use the checked-in redacted production retry
  ledger; replayed Work/Event identity, terminal output, structured result,
  and later follow transitions remain asserted.
- CASE-WW-016 uses an explicit publish/delivery signal before cancellation;
  the supplemental race run exercises the new controlled-stream
  synchronization directly.

The implementation changes only the watch functional package and this evidence
document. No production, generated, support, sibling, baseline, workflow, or
customer-contract files changed. No sleep, polling loop, or blanket timeout
increase was added. The exact-head clean-room execution and final
validation-loopback are recorded below; raised `jobs=8` CI, PR review, and
merge remain review-owned edges.

## Validation loopback report

### Environment and artifact

- Commit/build identifier: `e3f8eca809d4eaca79e860dd887485050630154f` (exact
  implementation/test head; this report refresh is documentation-only).
- Environment and configuration: detached clean worktree at the exact
  implementation/test head on Windows amd64; Go `go1.25.0`; no PRD or progress
  scaffolding was present in the clean worktree.
- Customer entry point: `root.BuildProcess` plus `Process.Execute`, using the
  public `you work watch` CLI contract.
- Real and substituted dependencies: production root/CLI/HTTP/Work/Factory
  Event path was exercised; controlled HTTP SSE servers were used only for the
  lifecycle fault-injection boundary; the checked-in redacted retry ledger was
  used for persisted replay.
- Cost/call budget used: free local validation; no paid provider or remote
  dependency calls.

The exact-head package proof was:

```text
go test -count=1 -timeout=10m ./tests/functional/work/watch
```

It exited with code `0` and emitted:

```text
ok  github.com/portpowered/infinite-you/tests/functional/work/watch  7.581s
```

The clean worktree had no tracked changes, and no package-matching process
remained after the command completed. The report was refreshed after this
proof against the same executable head; the refresh changes documentation
only.

### Project criteria

| Criterion | PASS/FAIL/BLOCKED | Dependency fidelity / applicability | Evidence | Unproven edge |
| --- | --- | --- | --- | --- |
| GATE-CI-DIAG | PASS | remote_real artifact; applicable | The pre-change run, job, head, selector, timings, exact retained diagnostic/event prefix, ordering, 25-repeat characterization, and unresolved suffix are recorded above before implementation evidence. | The artifact cannot recover the truncated reducer suffix. |
| Observable attachment and executable spine | PASS | local_real; applicable | `TestWorkWatchFollowsStateTransitionsUntilTerminal` starts a root-built watcher through the gate, waits for the exact public SSE response, then submits both public moves through the real server. | Raised-concurrency CI interaction remains review-owned. |
| Unchanged watch/Event contract and generated surfaces | PASS | local_real/read-only diff; applicable | Functional assertions cover `you.work.watch.v1`, canonical event identity/order, terminal output, cancellation, diagnostics, and replay; the delivered diff has no production or generated changes. | Full-suite contract interaction is not established locally. |
| No prohibited synchronization or scope growth | PASS | local_real/read-only diff; applicable | Changed paths are limited to the Work-watch functional package and this evidence document; no sleeps, polling loops, timeout padding, support, baseline, workflow, sibling, or generated edits were added. | None within the delivered scope. |
| CASE-WW-011/012 cleanup and primary-error retention | PASS | local_real; applicable | Success, cancellation, deadline, conflict, disconnect, and replay witnesses wait on explicit close/done signals and assert that the original diagnostic is retained. | Process-driver residency on this shared host is recorded as ENV-001 below. |
| GATE-FOCUSED | PASS | local_real | `go test -count=20 -run '^TestWorkWatchFollowsStateTransitionsUntilTerminal$' -timeout=10m ./tests/functional/work/watch` passed in `52.115s`; lifecycle witnesses also passed. | Not a whole-package or jobs=8 proof. |
| GATE-RACE | PASS | local_real | Named race command passed in `11.950s`; the controlled lifecycle race passed in `12.198s`. | No claim about unexercised races. |
| GATE-PACKAGE | PASS | local_real clean-room result | The implementation/test-head package rerun passed in `7.082s`. A detached clean worktree at exact implementation/test head `e3f8eca809d4eaca79e860dd887485050630154f` emitted `ok github.com/portpowered/infinite-you/tests/functional/work/watch 7.581s` with exit code `0`, no test failure output, and no matching clean-room processes after cleanup. | The package result does not prove full-suite `jobs=8` CI or terminal review state. |
| GATE-LOOPBACK | PASS | local_real; applicable | This report was produced after the detached clean-worktree package proof at exact head `e3f8eca809d4eaca79e860dd887485050630154f`; tracked-tree `git diff --check` is run after the report refresh. | Review-owned CI and merge remain outside loopback. |
| GATE-PR-CI | BLOCKED | remote_real PR runner; review-owned | The exact-head loopback was captured before the final report-refresh push, so raised `jobs=8` CI has not yet measured the final head. | Push the final head, start authoritative Backend Functional Coverage, and let review drive it terminal. |
| Paid validation and sensitive evidence | PASS | none/controlled; applicable | No paid validation was used; the retained diagnostic is truncated and contains no secrets or raw payloads. | None. |
| Accessibility and localization | PASS | not applicable | No UI or customer copy changed. | None. |

### Case matrix

| Case | PASS/FAIL/BLOCKED | Dependency fidelity / applicability | Evidence | Unproven edge |
| --- | --- | --- | --- | --- |
| CASE-WW-001 | PASS | local_real | Focused and package runs exercise submit, observable attachment, two public moves, exactly two ordered terminal NDJSON lines, empty stderr, and finite completion. | Raised `jobs=8` CI. |
| CASE-WW-002 | PASS | local_real with controlled SSE boundary | Controlled empty stream cancellation asserts empty stdout, existing cancellation classification, and stream close. | None within the case. |
| CASE-WW-003 | PASS | local_real with controlled SSE boundary | Retained metadata/request/transition history is delivered before a signal-gated live terminal transition; output order and identity are asserted. | None within the case. |
| CASE-WW-004 | PASS | local_real with controlled SSE boundary | An exact duplicate event is delivered and the watcher emits one transition without an error. | None within the case. |
| CASE-WW-005 | PASS | local_real with controlled SSE boundary and redacted ledger | Conflicting same-sequence data returns the existing non-increasing canonical sequence diagnostic, emits no false success, and closes safely. | The original historical CI suffix remains unavailable. |
| CASE-WW-006 | PASS | local_real with controlled SSE boundary | Follow mode consumes retained history, is canceled by context, emits only complete lines, preserves the existing cancellation behavior, and closes. | None within the case. |
| CASE-WW-007 | PASS | local_real with controlled SSE boundary | An established stream stalls under a bounded scenario deadline; no partial stdout is emitted, the deadline error is retained, and the stream closes. | None within the case. |
| CASE-WW-008 | PASS | local_real with checked-in redacted replay | Finite watch drains terminal retained history and exits without live traffic. | None within the case. |
| CASE-WW-009 | PASS | local_real with controlled SSE boundary | A delivered non-terminal transition is retained across disconnect and the bounded recovery path starts without discarding accepted output/cursor. | Remote raised-concurrency scheduling. |
| CASE-WW-010 | PASS | local_real with controlled SSE boundary | Reconnect observes the accepted cursor query, avoids duplicate output, delivers the remaining terminal transitions, and completes once. | Remote raised-concurrency scheduling. |
| CASE-WW-011 | PASS | local_real | Success, terminal, cancellation, and recovery cases wait for explicit stream/process completion and assert cleanup signals. | Shared-host outer Go-driver residency is ENV-001. |
| CASE-WW-012 | PASS | local_real | Conflict, deadline, and disconnect failure cases assert closure and safe diagnostics without masking the primary error. | Shared-host outer Go-driver residency is ENV-001. |
| CASE-WW-013 | PASS | existing package ownership | Existing empty-session validation ownership is retained; this correction does not add a duplicate witness. | This lane does not reassign or broaden that unit-level proof. |
| CASE-WW-014 | PASS | not applicable | The loopback server has no authorization boundary, so no artificial authorization assertion was added. | None; not applicable by plan. |
| CASE-WW-015 | PASS | not applicable | No maximum Work cohort or stream capacity changed; retained-prefix and production-ledger cases remain the bounded representative scale proof. | None; not applicable by plan. |
| CASE-WW-016 | PASS | local_real with controlled SSE boundary and race detector | Delivery, publication, and cancellation are signal-gated; the supplemental controlled race run passed. | No claim about every possible scheduler interleaving. |
| CASE-WW-017 | PASS | local_real with checked-in redacted replay | Replay asserts stable Work/Event/request-response identities, ordering, structured result, terminal completion, and later follow transitions without mutating the fixture. | None within the case. |

### Customer journey

1. From a detached clean worktree at exact implementation/test head
   `e3f8eca809d4eaca79e860dd887485050630154f`, run `go test -count=1
   -timeout=10m ./tests/functional/work/watch`. The package emitted
   `ok github.com/portpowered/infinite-you/tests/functional/work/watch 7.581s`
   with exit code `0`;
   all package test cases therefore completed without a failure result.
2. The test package's public journey remains root-built Process construction,
   public CLI invocation, production HTTP/Factory Event behavior, and the
   unchanged Work-watch NDJSON contract. The controlled stream cases use
   observable response, publication, delivery, and close signals.
3. The tracked delivered tree passed `git diff --check` after this report
   refresh; the clean worktree was removed after its result and no clean-room
   process matching the package remained.

### Cross-task integration and usability

- Documentation discoverability: the lane evidence is in the canonical
  development document `docs/internal/development/functional-test-optimization/c09-work-watch-contention.md`.
- Permission and error behavior: the loopback server is loopback-only and has
  no authorization boundary; existing cancellation, deadline, conflict, and
  safe-diagnostic behavior remains asserted.
- Persistence/reload behavior: the redacted production retry ledger exercises
  retained replay and later follow transitions without fixture mutation.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: explicit attachment, retained-history, publication,
  delivery, close, command-done, and context-deadline signals provide bounded
  diagnostics without sleeps or polling.

### Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| ENV-001 | non-blocking environmental | Run the package command through the shared Windows host runner. | The outer Go driver exits shortly after the package reports its result. | The package emitted `ok`; no package-matching clean-room process remained after the exact-head run. The shared host's occasional outer-driver residency remains recorded from the implementation runs, but it did not alter this clean-room exit code or package result. | Exact-head package output above; the implementation-run residency observation is retained without treating it as a package-owned leak. |

### Verdict

PASS

The loopback runtime proof is passing at the package-test fidelity declared by
the task. `GATE-PR-CI`, terminal CI, review, conflict resolution, and merge are
explicitly not claimed here and remain review-owned handoff edges.

### Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: None for loopback; `GATE-PR-CI` is a
  review-owned delivery gate rather than a loopback failure.
- Root-cause evidence or remaining uncertainty: the historical reducer suffix
  is unavailable because the source artifact was truncated; raised-concurrency
  CI has not yet run on this head.
- Smallest recommended correction/prerequisite: open the requested PR and
  start the authoritative Backend Functional Coverage check at `jobs=8`; do
  not change the package based on ENV-001 unless review identifies a
  reproducible package-owned leak.
- Dependencies and retest scope: review-owned PR CI, then any concrete
  blocking feedback; no additional local re-run is required before handoff.
