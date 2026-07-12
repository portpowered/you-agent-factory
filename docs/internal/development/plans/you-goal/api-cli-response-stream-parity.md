# API/CLI Response-Stream Parity Evidence (Batch 07 Gate)

Reviewer-visible integration evidence for story `you-goal-b07-stream-program-gate-004`.
This artifact records what API and CLI response-stream surfaces prove on merged
branch head, which upstream stream-responses residuals remain explicitly excluded,
and where narrow integration corrections were not required in this lane.

**Status:** gate evidence complete with documented upstream exclusions

**Last updated:** 2026-07-12 UTC

**Audit prerequisite:** `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`

**Goal stream prerequisite:** `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`

**Subagent stream prerequisite:** `docs/internal/development/plans/you-goal/subagent-response-stream-integration.md`

## Summary verdict

| Acceptance criterion | Verdict | Evidence |
|----------------------|---------|----------|
| CLI NDJSON and API session response-event SSE decode to equivalent canonical `FactoryResponseEvent` values | **Excluded — upstream residual** | R1/R2 from stream-responses final audit: no public `FactoryResponseEvent` schema and no session-scoped response-event SSE route in merged OpenAPI |
| Provider parity shows truthful fidelity and equivalent terminal `InvocationResponse` across primary-only and stream modes | **Merged (terminal scope)** | Mock-worker fixtures are final-only; API `POST /factory-sessions/{session_id}/invocations` and CLI JSON `primary_result` agree on terminal outcome (goal + subagent parity smokes below) |
| Narrow integration corrections only; no parallel stream reimplementation | **Merged** | No code changes required beyond focused parity proofs; internal stream vocabulary stays isolated from public artifacts |
| Final gate evidence package cites audit + focused integration tests | **Merged** | This document plus stories 001–003 artifacts and smoke suites listed below |

## Proven API/CLI parity (merged scope)

### Terminal invocation outcome parity

For representative successful `@you/goal` and `@you/subagent` fixtures:

- API consumers receive the shared public `InvocationResponse` envelope from
  `POST /factory-sessions/{session_id}/invocations`.
- CLI consumers running `--json --output response-stream` receive exactly one
  terminal `primary_result` NDJSON record wrapping the same `InvocationResponse`
  shape.
- Focused smoke tests prove API and CLI response-stream terminal outcomes agree
  for the same fixture inputs:

```bash
go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal' -count=1
```

### Internal stream boundary and public artifact isolation

Merged evidence from stories 001–003 still applies:

- Internal `SessionResponseStream` events are consumed by the CLI renderer in
  `pkg/cli/run/run_clean_invocation.go` and are not projected into durable
  `FactoryEvent` history.
- Generated public API artifacts omit internal response-stream vocabulary
  (`pkg/api/contracttests/generated_contract_common_test.go`,
  `assertTextOmitsInternalResponseStreamTerms`).
- Canonical `FactoryEvent` SSE (`GET /factory-sessions/{session_id}/events`)
  remains the only public session event stream; it does not carry internal
  response-stream fragments.

### Provider fidelity posture on the integration surface

Packaged goal/subagent smoke fixtures use mock workers that complete without live
provider streaming fragments. That is truthful **final-only** fidelity: stream mode
still ends with the authoritative primary result and does not invent synthetic
progress events.

## Explicit exclusions (upstream stream-responses program)

These residuals block **canonical** API/CLI response-event parity and are owned by
the separate stream-responses program, not this integration gate lane:

| Residual | Impact on story 004 |
|----------|---------------------|
| **R1** — no public `FactoryResponseEvent` in OpenAPI or generated clients | Cannot decode CLI progress NDJSON and API SSE to a shared canonical response-event schema |
| **R2** — no session-scoped response-event SSE route | Cannot compare per-event stream payloads across API and CLI beyond terminal `InvocationResponse` |

Until R1/R2 land, CLI `progress` / `stream_gap` / `compaction` NDJSON remains a
CLI-private dialect documented in `docs/reference/config.md`. This gate records
that exclusion explicitly rather than soft-passing canonical event parity.

## Final gate evidence package

| Artifact | Story |
|----------|-------|
| `docs/internal/development/plans/you-goal/stream-responses-final-audit.md` | 001 |
| `docs/internal/development/plans/you-goal/goal-response-stream-integration.md` | 002 |
| `docs/internal/development/plans/you-goal/subagent-response-stream-integration.md` | 003 |
| `docs/internal/development/plans/you-goal/api-cli-response-stream-parity.md` | 004 (this document) |
| `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` | 002 |
| `tests/functional/smoke/cli_named_subagent_response_stream_smoke_test.go` | 003 |
| `tests/functional/smoke/cli_named_response_stream_api_parity_smoke_test.go` | 004 |

## Verification commands

```bash
go test ./pkg/cli/run/... -short -run ResponseStream
go test ./pkg/factorysessions/responsestream/... -short
go test ./pkg/api/contracttests/ -short -run 'ResponseStream|InternalResponseStream'
go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream|NamedSubagentResponseStream|NamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal' -count=1
```
