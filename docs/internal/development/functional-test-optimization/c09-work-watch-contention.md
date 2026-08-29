# C09 Work-watch contention

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
increase was added. Clean-room execution, final validation-loopback, raised
`jobs=8` CI, PR review, and merge remain unproven edges for the next story.
