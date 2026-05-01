# Process Review Loop Contract

This note records the current checked-in mismatch in the repository-maintainer
workflow and defines the canonical contract that follow-up runtime and prompt
stories must implement.

## Current Mismatch

The checked-in maintainer loop currently uses rejection for two different
meanings:

1. `factory/workers/processor/AGENTS.md` sets `stopToken: "<COMPLETE>"`, so
   `pkg/workers/agent.go` classifies any successful model response without
   `<COMPLETE>` as `REJECTED`.
2. `factory/workstations/process/AGENTS.md` instructs the executor to respond
   with `<CONTINUE>` when the current PRD still has unfinished work after the
   current iteration.
3. `factory/factory.json` routes `process.onRejection` back to `task:init`, so
   ordinary partial progress uses the same rejection arc that the checked-in
   `review` workstation uses for true send-back behavior.
4. `tests/functional_test/testdata/adhoc-recording-batch-event-log.json`
   preserves the mismatch in replay evidence: repeated `process` dispatches
   record `outcome: "REJECTED"` with `output: "<CONTINUE>\n"`, while actual
   review send-back records `outcome: "REJECTED"` with `output: "<REJECTED>\n"`.

The result is that replay and history surfaces cannot distinguish normal
story-by-story executor iteration from an actual review rejection.

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
  worker outcome mappers that currently collapse missing stop tokens into
  `REJECTED`.
- Workstation config and public authoring contract:
  `factory/factory.json`, workstation config structs and mapping in `pkg/`,
  and the workstation authoring docs that currently describe only
  `outputs`, `onRejection`, and `onFailure`.
- Checked-in maintainer prompts:
  `factory/workers/processor/AGENTS.md`,
  `factory/workstations/process/AGENTS.md`, and
  `factory/workstations/review/AGENTS.md`.
- Replay, history, and behavioral proof:
  checked-in replay or fixture evidence such as
  `tests/functional_test/testdata/adhoc-recording-batch-event-log.json` plus
  focused tests that assert the observable routing and outcome history.

## Story Boundaries

- `US-001` defines the contract and the exact surfaces that must change.
- `US-002` should introduce a first-class continue outcome and routing path in
  the runtime and config layers.
- `US-003` should align the checked-in prompts and scaffolded workflow text to
  the runtime contract.
- `US-004` should add focused behavioral coverage for continue, rejection, and
  loop-breaker behavior.
- `US-005` should remove stale maintainer-facing wording that still describes
  ordinary partial progress as rejection.
