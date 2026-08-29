# C11 workers invoke/continue tracked plan replacement

## Authority and conflict resolution

This document is the tracked source-plan replacement and authority record for
`functional-test-optimization-c11-workers-invoke-continue`. The PRD originally
named the temporary plan `docs/temp/functional-test-optimization.md`, but that
operator-held path is intentionally gitignored and is absent from this
checkout, `origin/main`, and every inspected local/ref history. The temporary
plan is not reconstructed here.

The operator disposition on [PR #2416](https://github.com/portpowered/you-agent-factory/pull/2416#issuecomment-5461024854)
authorizes a tracked ledger replacement under planning-standards §11 and
confirms the source-plan revision `functional-test-optimization-v2`. This
document applies that authority to this lane and records the exact replacement
scope used by the PRD. The assertion ledger remains the executable observation
record; this document is the source-plan traceability record, not a silent
substitute.

| Original PRD reference | Tracked replacement section | Resolution |
| --- | --- | --- |
| `Scope 2 — workers/inference contention context` | [Worker-specific concurrency](#worker-specific-concurrency) | Retained only for the manager/concurrent-stream and targeted interruption witnesses. |
| `Scope 10 — Full-inventory migration sweep` | [Migration outcome](#migration-outcome) | All 14 eligible rows and nested public assertions remain mapped one-for-one. |
| `Scopes 4 and 5` | [Executable spine and isolation](#executable-spine-and-isolation) | One package-owned root-built process, explicit sessions, and immutable pre-start routes are required. |
| `Functional test-case discipline` | [Functional-test constraints](#functional-test-constraints) | The declared root/CLI/edge/no-sleep construction rules remain binding. |
| `Acceptance Criteria 2 and 3` | [Acceptance and verification](#acceptance-and-verification) | Topology, typed outcomes, no fallback, and next-scenario recovery remain required. |

## Lane outcome

The lane migrates the 14 eligible Worker invoke/continue rows from 20
application-graph constructions to one package-scoped `root.BuildProcess` /
`Process.Execute` process. Every scenario registers its provider effect route
before process start and opens a unique non-default explicit Factory Session.
The route is selected only by the scenario WorkDir; scenario state is not
looked up or registered after start.

The retained behavior includes invoke/continue identity and lineage, provider
session references, Work and Factory Event observations, transcript and replay
ordering, typed validation/failure/control outcomes, exact provider-call and
cancellation counts, idempotency/conflict behavior, concurrent stream
isolation, targeted interruption, and teardown before reuse. The unsupported
continuation witness uses the production Providers wire with a route-specific
catalog capability override and the existing `ProviderCommandRunner` edge.

## Executable spine and isolation

- Construction is through `support.BuildProcessWithContext`, which reaches
  `root.BuildProcess`; scenarios execute through `Process.Execute`.
- Ordinary customer flows use public CLI operations. Remote-control and stream
  cases use injected local HTTP endpoints because those are API-owned remote
  boundaries.
- External provider effects use the pre-registered route and
  `ProviderCommandRunner`/controlled streaming runners through `edges.Edges`.
  No `MockWorkers` shortcut or remote/paid provider is in scope.
- Synchronization uses provider barriers, context cancellation, and bounded
  observation of owned streams. No sleep or timeout inflation is part of the
  migration.
- Cleanup observes balanced Factory Session open/close/delete, stream and
  provider-call drain, route removal, process close, listener/port shutdown,
  and owned-root removal.

## Worker-specific concurrency

The manager rows remain in this lane and run on the same package process while
retaining separate explicit sessions, repositories, Work IDs, Worker Session
IDs, Provider Session IDs, event topics, transcripts, and provider calls. The
interruption witness cancels only A, resumes A's successor through the exact
recorded provider session, and verifies B survives and replays completely.

## Migration outcome

The authoritative row-to-witness mapping is
`tests/functional/workers/invoke_continue/assertion_parity_ledger.md`. It
contains all 14 rows, including the two manager rows, and explicitly identifies
the three post-migration characterization additions for empty input,
deterministic cancellation, and duplicate continuation. No pre-migration
assertion is deleted or weakened.

## Functional-test constraints

The implementation follows the repository-wide backend testing standard and
the functional-test construction preferences: real root composition,
`Process.Execute`, public CLI-first ordinary flows, external effects only at
the edge boundary, command-runner provider substitution, no mock-worker mode,
and deterministic observation rather than default sleeps or padded waits.

## Acceptance and verification

The required local-real evidence is the focused shared-process witness, the
manager/interruption/cleanup determinism selectors, the applicable race
selector, and one package-wide
`go test ./tests/functional/workers/invoke_continue -count=1` run. The
clean-room validation also audits `origin/main...HEAD`, the owned diff, one
package process/start, balanced explicit sessions, immutable routes, typed
failure/no-fallback behavior, and the final package direction. Remote provider
availability and terminal CI remain outside this local replacement record;
review owns terminal CI and merge after the final head is handed off.
