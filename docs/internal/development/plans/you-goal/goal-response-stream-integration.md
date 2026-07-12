# @you/goal Response-Stream Integration Evidence (Batch 07 Gate)

Reviewer-visible integration evidence for story `you-goal-b07-stream-program-gate-002`.
This artifact proves named `@you/goal` one-shot `--output response-stream` human and
JSON modes consume the merged internal response-stream renderer contract and end with
the shared authoritative `InvocationResponse` terminal outcome.

**Status:** CLI-private stream proofs recorded; gate remains incomplete pending R1/R2 (no public `FactoryResponseEvent`)

**Last updated:** 2026-07-12 UTC

**Audit prerequisite:** `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`
records blocking residuals for public `FactoryResponseEvent` + session-scoped SSE.
Story 002 therefore proves the **merged CLI stream contract** (internal
`SessionResponseStream` vocabulary mapped through `pkg/cli/run/run_clean_invocation.go`)
without soft-passing public canonical parity.

## Summary verdict

| Acceptance criterion | Verdict | Evidence |
|----------------------|---------|----------|
| Human response-stream emits shared CLI progress/observation lines and terminal primary result | **Merged (CLI scope)** | `run_clean_invocation.go` human renderer; smoke `TestNamedGoalResponseStream_HumanModeUsesCanonicalProgressPrefixNotLegacyDialect` |
| JSON response-stream emits NDJSON shared CLI `recordType` stream records and exactly one terminal `primary_result` | **Merged (CLI scope)** | `run_clean_invocation.go` JSON renderer; smoke `TestNamedGoalResponseStream_JSONModeEmitsExactlyOnePrimaryResultRecord`, `TestNamedGoalResponseStream_JSONModeUsesCanonicalCLIStreamRecordVocabulary` |
| Primary-only and response-stream modes agree on terminal invocation outcome | **Merged** | smoke `TestNamedGoalResponseStream_PrimaryOnlyAndResponseStreamAgreeOnTerminalOutcome` |
| Focused goal stream integration/smoke tests | **Merged** | `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` |

## Shared CLI stream vocabulary (merged — not public `FactoryResponseEvent`)

Human mode:

- Progress/observation lines use the `[you:progress] ` prefix.
- Successful invocations end with the same plain-text primary result as primary-only mode.
- Failed terminal outcomes use `--- invocation outcome ---` (not emitted on successful goal smoke fixture).

JSON mode (`--json --output response-stream`):

- NDJSON `recordType` values: `progress`, `stream_gap`, `compaction`, `primary_result`.
- `progress` records expose internal stream `kind`, `eventType`, `sequence`, optional `dispatchId`, and `payload`.
- Exactly one terminal `primary_result` record wraps the shared public `InvocationResponse` envelope.

## Verification commands

```bash
go test ./pkg/cli/run/... -short -run ResponseStream
go test ./tests/functional/smoke/ -run NamedGoalResponseStream -count=1
```

## Residual exclusions carried from story 001

- **R1 / R2:** CLI progress NDJSON remains a CLI-private dialect until public
  `FactoryResponseEvent` lands; story 004 owns API/CLI canonical parity.
- Mock-worker goal smoke fixtures may complete without live progress fragments;
  vocabulary tests still prove absence of legacy provider fragment dialect on stdout.
