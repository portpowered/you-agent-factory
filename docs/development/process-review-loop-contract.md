# Process Review Loop Contract

This note records the canonical continue-versus-rejection contract for the
checked-in repository-maintainer workflow and the repository surfaces that must
stay aligned with it.

## Status

The checked-in maintainer loop now uses an explicit continue path for ordinary
partial executor progress. `process` `<CONTINUE>` responses route through
`onContinue` and record `CONTINUE`, while true review send-back remains
`REJECTED` and continues to route through `review.onRejection`.

This replaced an older mismatch where `<CONTINUE>` fell through the rejection
path and made replay/history evidence look like rejection churn. Historical
notes or fixtures may still mention that older behavior as context, but the
repository's active workflow contract is the continue-versus-rejection split
described below.

## Canonical Contract

The repository-level contract for the checked-in maintainer workflow is:

- `<COMPLETE>` means the current execution pass is ready to advance from
  `process` into `review`.
- `<CONTINUE>` means the executor made ordinary partial progress and the same
  task should continue iterating without being classified as rejection.
- True rejection is reserved for review send-back behavior, where the reviewer
  determined that the code or evidence is not ready and the task must return to
  the executor with rejection semantics intact.

In other words, "more work remains on this task" and "review rejected this
attempt" are distinct outcomes and must stay distinct in both routing and
history.

## Required Alignment Surfaces

The following surfaces must agree on the contract above:

- Runtime outcome classification:
  `pkg/interfaces/work_execution.go`, `pkg/workers/agent.go`, and any other
  worker outcome mappers that classify workstation responses into accepted,
  continued, rejected, or failed outcomes.
- Workstation config and public authoring contract:
  `factory/factory.json`, workstation config structs and mapping in `pkg/`,
  and the workstation authoring docs that describe `outputs`, `onContinue`,
  `onRejection`, and `onFailure`.
- Checked-in maintainer prompts:
  `factory/workers/processor/AGENTS.md`,
  `factory/workstations/process/AGENTS.md`, and
  `factory/workstations/review/AGENTS.md`.
- Replay, history, and behavioral proof:
  checked-in replay or fixture evidence such as
  `tests/functional_test/testdata/adhoc-recording-batch-event-log.json` plus
  focused tests that assert the observable routing and outcome history.

## Closeout Scope

This cleanup is complete only when all of the following stay true together:

- `process` `<CONTINUE>` traffic records as `CONTINUE`, not `REJECTED`.
- The checked-in `process` loop uses `onContinue` for ordinary executor
  iteration and reserves rejection for true negative outcomes.
- The checked-in `review` loop still uses rejection for send-back behavior and
  keeps loop-breaker safeguards based on actual rejection accumulation.
- Maintainer-facing prompts, docs, and focused behavioral tests all describe
  the same contract without calling ordinary partial progress rejection.
