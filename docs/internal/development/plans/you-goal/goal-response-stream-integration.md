# @you/goal Response-Stream Integration Evidence (Batch 07 Gate)

Reviewer-visible integration evidence for story `you-goal-b07-stream-program-gate-002`.
This artifact proves named `@you/goal` one-shot `--output response-stream` human and
JSON modes consume the merged canonical response-stream contract and end with the
shared authoritative `InvocationResponse` terminal outcome.

**Status:** Canonical goal response-stream proofs recorded against merged `origin/main`
(stream-b09 private-contract removal and public `FactoryResponseEvent` delivery).

**Last updated:** 2026-07-14 UTC

**Audit prerequisite:** `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`
records the original blocking residuals. The stream-responses program has since merged
public OpenAPI schemas, session response-event SSE, production publication, and
canonical CLI NDJSON (`response_event` + `invocation_result`) on `origin/main`.

## Summary verdict

| Acceptance criterion | Verdict | Evidence |
|----------------------|---------|----------|
| Human response-stream emits canonical progress/observation events and terminal primary result | **Merged** | `pkg/transports/cli/run/invocation_observability.go` human renderer; smoke `TestNamedGoalResponseStream_HumanModeUsesCanonicalHumanFormatNotLegacyDialect` |
| JSON response-stream emits NDJSON canonical `FactoryResponseEvent` records and exactly one terminal `invocation_result` | **Merged** | `pkg/transports/cli/run/run_clean_invocation.go` JSON renderer; smoke `TestNamedGoalResponseStream_JSONModeEmitsInvocationResultRecord`, `TestNamedGoalResponseStream_JSONModeEmitsExactlyOneInvocationResultRecord`, `TestNamedGoalResponseStream_JSONModeUsesCanonicalCLIStreamRecordVocabulary` |
| Primary-only and response-stream modes agree on terminal invocation outcome | **Merged** | smoke `TestNamedGoalResponseStream_PrimaryOnlyAndResponseStreamAgreeOnTerminalOutcome` |
| Focused goal stream integration/smoke tests | **Merged** | `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` |

## Canonical CLI stream vocabulary (merged)

Human mode:

- Progress/observation lines use canonical `FactoryResponseEvent` formatting (`progress:`, `reasoning:`, `tool:`, `stream gap:` prefixes).
- Successful invocations end with the same plain-text primary result as primary-only mode.
- Retired private dialect terms (`[you:progress] `, `PROGRESS_FRAGMENT`, private `recordType` values) are absent.

JSON mode (`--json --output response-stream`):

- NDJSON `recordType` values: `response_event`, `invocation_result`.
- `response_event` records wrap validated public `FactoryResponseEvent` values.
- Exactly one terminal `invocation_result` record wraps the shared public `InvocationResponse` envelope.
- Retired private `recordType` values (`progress`, `stream_gap`, `compaction`, `primary_result`) are rejected by `ndjsoncontract`.

## Verification commands

```bash
go test ./pkg/transports/cli/run/... -short -run ResponseStream
go test ./tests/functional/smoke/ -run NamedGoalResponseStream -count=1
```

## Upstream dependency closure

- Public `FactoryResponseEvent` schemas, session response-event SSE, production publication, and canonical CLI NDJSON merged via stream-responses program tranches on `origin/main` (including `stream-b09-remove-private-contract` #1129).
- Mock-worker goal smoke fixtures may complete without live progress fragments; vocabulary tests still prove absence of legacy provider fragment dialect on stdout.
