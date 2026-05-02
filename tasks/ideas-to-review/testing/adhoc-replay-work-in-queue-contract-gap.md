# Adhoc Replay Work-In-Queue Contract Gap

## Why this should be reviewed

The last observable test left under `tests/functional_test/` is
`TestWorkInQueueScheduler_AdhocBatchReplayDispatchesMultipleItemsAndPrioritizesInitializedTraces`.
It is currently the only remaining non-long behavior blocking `US-009`, but the
test also fails when run directly today, even before any migration:

- `go test ./tests/functional_test -run TestWorkInQueueScheduler_AdhocBatchReplayDispatchesMultipleItemsAndPrioritizesInitializedTraces -count=1`

The failure shows the replay harness only dispatching one item on the first
tick instead of the asserted eight-item batch, so the remaining gap is no
longer just "move this file into a behavior package".

## Problem

The repository still has a replay-backed scheduler contract that is neither in
the decomposed package tree nor passing as written. Because the assertion is
currently stale against the runtime behavior, simply moving it into
`tests/functional/runtime_api` or the explicit long lane breaks quality checks
without actually clarifying the intended product contract.

## Proposed follow-up

- Decide whether replay-backed adhoc batching is still intended to dispatch
  multiple items per tick under the current harness and scheduler wiring.
- If the behavior is still intended, fix the production replay/runtime path and
  migrate the coverage into the owning behavior package with either default-lane
  or long-lane placement justified explicitly.
- If the behavior is no longer intended, replace the stale assertion with a
  narrower observable contract that matches the current replay-visible scheduler
  guarantee, then retire the legacy `tests/functional_test` seam cleanly.
