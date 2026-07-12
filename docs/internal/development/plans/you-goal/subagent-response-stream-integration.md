# @you/subagent Response-Stream Integration Evidence (Batch 07 Gate)

Reviewer-visible integration evidence for story `you-goal-b07-stream-program-gate-003`.
This artifact proves named `@you/subagent` one-shot `--output response-stream` human and
JSON modes consume the same merged internal response-stream renderer contract as
`@you/goal` and end with the shared authoritative `InvocationResponse` terminal outcome.

**Status:** integration proofs recorded against merged branch head

**Last updated:** 2026-07-12 UTC

**Audit prerequisite:** `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`
records blocking residuals for public `FactoryResponseEvent` + session-scoped SSE.
Story 003 therefore proves the **merged CLI stream contract** (shared
`humanResponseStreamRenderer` / `jsonResponseStreamRenderer` in
`pkg/cli/run/run_clean_invocation.go`) without soft-passing public canonical parity.

**Goal parity reference:** `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`

## Summary verdict

| Acceptance criterion | Verdict | Evidence |
|----------------------|---------|----------|
| Human and JSON response-stream modes use the same canonical CLI stream vocabulary as `@you/goal` | **Merged** | Shared renderers in `run_clean_invocation.go`; smoke `TestNamedSubagentResponseStream_HumanModeUsesCanonicalProgressPrefixNotLegacyDialect`, `TestNamedSubagentResponseStream_JSONModeUsesCanonicalCLIStreamRecordVocabulary` |
| JSON response-stream emits NDJSON canonical records and exactly one terminal `primary_result` | **Merged** | smoke `TestNamedSubagentResponseStream_JSONModeEmitsExactlyOnePrimaryResultRecord`, `TestNamedSubagentResponseStream_JSONModeEmitsPrimaryResultRecord` |
| Successful subagent stream runs return exactly one authoritative primary response | **Merged** | smoke `TestNamedSubagentResponseStream_RealCLICompletesWithPrimaryResult` |
| Primary-only and response-stream modes agree on terminal invocation outcome | **Merged** | smoke `TestNamedSubagentResponseStream_PrimaryOnlyAndResponseStreamAgreeOnTerminalOutcome` |
| No subagent-only alternate response-stream renderer or record types | **Merged** | No `subagent` references under response-stream renderer code; subagent reuses shared `InvocationOutputResponseStream` path from `pkg/cli/run/invocation_observability.go` |
| Focused subagent stream integration/smoke tests | **Merged** | `tests/functional/smoke/cli_named_subagent_response_stream_smoke_test.go` |

## Shared renderer path (merged)

`@you/subagent` does not introduce a parallel stream stack:

- Output mode selection uses the same `InvocationOutputResponseStream` constant and
  `newResponseStreamRenderer` factory as `@you/goal`.
- Internal `SessionResponseStream` attachment runs through `startResponseStreamAttachment`
  in `pkg/cli/run/invocation_observability.go`.
- Human progress uses `[you:progress] `; JSON NDJSON uses `progress`, `stream_gap`,
  `compaction`, and `primary_result` record types.
- Terminal `primary_result` wraps the shared public `InvocationResponse` envelope.

## Verification commands

```bash
go test ./pkg/cli/run/... -short -run ResponseStream
go test ./tests/functional/smoke/ -run NamedSubagentResponseStream -count=1
```

## Residual exclusions carried from story 001

- **R1 / R2:** CLI progress NDJSON remains a CLI-private dialect until public
  `FactoryResponseEvent` lands; story 004 owns API/CLI canonical parity.
- Mock-worker subagent smoke fixtures may complete without live progress fragments;
  vocabulary tests still prove absence of legacy provider fragment dialect on stdout.
