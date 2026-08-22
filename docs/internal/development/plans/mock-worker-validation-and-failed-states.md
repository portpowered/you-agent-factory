# Mock Worker Validation and Failed-State Fidelity

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-MOCK-001
status: proposed
---

# problem statement

The mock-worker harness silently falls through to default-accept when its selector matches nothing, so a misconfigured harness reports success — and it cannot reproduce most of the failure modes the real runtime produces, so no test can assert them.

## customer ask

Mock workers must be a trustworthy substrate for integration testing: a selector that matches nothing must say so, and a mock worker must be able to fail in every way a real worker fails.

## solution

Make no-match an explicit, loud configuration error rather than a silent fall-through, then extend the mock contract to cover the runtime's actual failure taxonomy so integration tests can assert each terminal state by construction.

# original document

`docs/internal/development/plans/ci-integration-test-matrix.md`. That tier is happy-path
only and carries no failure rows, so this plan's failure taxonomy is exercised in the
**functional** tier. Two connections to the integration tier remain, and both are load-
bearing: MOCK-1/MOCK-2 decide whether the mock workers backing integration tests I1, I2
and I3 actually bind their inputs, and MOCK-3's routed-to-failed-state mode is the
acceptance evidence for integration Story 0.

## Why this comes before the integration tier

A harness that cannot fail is worse than no harness, because it manufactures confidence.
The adversarial evaluation of v0.0.8 reported that the `workInputs` selector never
matched anything, so **all three shipped example configs silently fell through to
default-accept while reporting success**. If that holds, every test written against mock
workers today is green by construction, in the same way that
`TestRootProcessCompiledBinaryModeMatrix` is green by construction.

This is the same failure shape as accepting a non-empty result as a valid one: the system
distinguishes "matched and accepted" from "matched nothing, so accepted anyway" and then
discards the distinction. The two outcomes are indistinguishable to the caller, and the
uninteresting one is the default.

**The selector claim is not yet confirmed in the current tree.** The machinery is present
— `pkg/services/workers/mock_workers_contracts.go:42` declares
`WorkInputs []MockWorkInputSelector`, and
`pkg/services/workers/internal/interface/` carries an input index, a config decoder, and
baseline inventories for `workId`, `workType`, `state`, `inputName`, `traceId`,
`channel`, and `payloadHash`. Whether a selector actually *binds at runtime* cannot be
settled by reading code; it needs a run. Story MOCK-1 exists to settle it either way, and
its result is a finding regardless of which way it lands.

## Story MOCK-1 — Settle the selector question (characterization)

Before changing anything, pin the current behavior with a test that distinguishes the
three outcomes the caller currently cannot tell apart:

1. a selector that matches an input, and the matched input binds
2. a selector that matches nothing, and the worker accepts anyway
3. no selector configured, and the worker accepts by default

Run each of the three shipped example configs and record which outcome each produces.

Acceptance: a characterization test that passes against today's behavior and records the
verdict in its assertions, so the subsequent change is a visible, reviewable diff rather
than a claim. If outcome 2 does not occur, say so plainly — the evaluation finding is
then refuted, and MOCK-2 narrows to a guard rather than a fix.

## Story MOCK-2 — No-match is an error, not a fall-through

A configured selector that matches no input must fail the run with a diagnostic naming
the worker, the selector, the fields it matched on, and what was actually available.

Rules:

- A selector that matches nothing is a **configuration error**, surfaced at run start
  where possible, not at dispatch time.
- Default-accept remains the behavior when **no selector is configured**. That case is
  legitimate and common; it is only the configured-but-unmatched case that is a lie.
- The three shipped example configs must be corrected as part of this story, and each
  becomes a functional-tier case. If a selector silently falls through, integration tests
  I1–I3 are green by construction, because mock workers are what they run on — that is the
  path by which this defect would silently disable the new tier.

Acceptance: a config whose selector matches nothing exits non-zero with the selector and
the available input names in the diagnostic; a config with no selector still accepts; a
config whose selector matches binds the selected input, observable in the dispatch record.

## Story MOCK-3 — Failure taxonomy

The functional tier needs mock workers that fail the way real workers fail.
Today the contract carries `MockWorkerRejectConfig` with `Stderr` and `ExitCode`
(exercised in `pkg/transports/cli/run/run_config_test.go:419`), which covers one mode.
The runtime produces several, and each drives different downstream behavior.

| Mode | What the mock must do | What it lets a test assert |
|---|---|---|
| Reject with output | Non-zero exit, stderr, no result | The failure detail reaches `work show`, and batch mode exits non-zero |
| Route to a failed state | Complete normally but emit an output routing the token to a failed place | The engine-completed-but-work-failed case — the exact shape that makes batch exit 0 today |
| Hard timeout | Exceed the configured worker duration bound | Timeout classification is distinct from crash, and the bound is enforced |
| Crash mid-stream | Exit after partial output | Partial output is not mistaken for a result; no half-written state |
| Oversized output | Exceed `MaxOutputBytesPerWorker` | The bound is enforced and the diagnostic names it, rather than the record being silently discarded |
| Consecutive failures | Fail deterministically on N successive attempts | The circuit breaker trips at exactly three and names the transition |
| Slow but successful | Complete after a bounded delay | Concurrency limits are observable; capacity 1/2/4 rows are meaningful |

Each mode is one field or one enum value on the mock worker config, declaratively
selected. None of them requires a real provider.

Acceptance: for each mode, a factory using it produces the documented terminal state and
the documented exit code, asserted from a compiled binary. The routed-to-failed-state
mode is the acceptance evidence for integration Story 0 (truthful batch exit codes) and
must be written to fail against today's behavior.

## Story MOCK-4 — Failed-state observability

A test that can only see the process exit code cannot tell a circuit breaker from a
timeout. Each failure mode above must be distinguishable through the read surfaces the
integration tier uses.

Acceptance: for every mode in MOCK-3, `work show` reports a failure reason that names the
mode, `worker-sessions list --work-id` reports a lifecycle classification distinguishing
it from the others, and `_last_output` does not contradict the terminal state. This
directly closes the adversarial finding that `work show` on a FAILED item returns no
reason and that its `_last_output` tag can contradict the FAILED state.

## Story MOCK-5 — Selector coverage across the declared field set

The input index declares seven selector fields: `workId`, `workType`, `state`,
`inputName`, `traceId`, `channel`, `payloadHash`. A field that is declared but never
matched is a trap for the next author.

Acceptance: one integration row per field, proving each selects and each rejects; any
field that cannot be made to match is removed from the contract in this story rather than
left declared. A declared-but-dead selector field is not a smaller problem than a broken
one — it is the same problem with a longer fuse.

## Delivery order

MOCK-1 → MOCK-2 → MOCK-3 → MOCK-4 → MOCK-5. Each merges on its own.

MOCK-1 and MOCK-2 block integration tests I1, I2 and I3 — not because those tests assert
selector behavior, but because they run on mock workers and a silent fall-through would
make them unfalsifiable. MOCK-3, MOCK-4 and MOCK-5 land in the functional tier and block
nothing in the integration tier, except that MOCK-3's routed-to-failed-state mode is
Story 0's acceptance evidence.

MOCK-3's routed-to-failed-state mode has a cross-dependency worth naming: it is written
as a failing test *before* integration Story 0 lands, and Story 0's acceptance is that
this test goes green. Neither lane should wait on the other — the test lands red-and-
skipped with a pointer to Story 0, and Story 0 removes the skip.

## Non-goals

- **No new provider adapters.** Mock workers exist so tests need no provider at all.
- **No mock-worker support for real model behavior** — token accounting, streaming
  semantics, or provider-specific output shapes. Those belong to provider characterization
  tests, and conflating them would make the mock harness a second implementation of the
  thing it is supposed to stand in for.
- **No changes to real worker execution.** This plan touches the mock path and the read
  surfaces that report failure; it does not alter how real workers run.

## Verification

`make test`, plus targeted tests near `pkg/services/workers/` and the integration tier
from `ci-integration-test-matrix.md`. MOCK-2 changes behavior for existing mock configs,
so `make test-functional` is warranted for that story specifically.

## Delivery loop

Implementation finishes when its final head is pushed, the PR is open, CI has started,
and blocking review feedback is addressed. Review owns terminal-and-passing CI, conflict
resolution, and merge. CI-run evidence goes in a PR comment, never a commit.
