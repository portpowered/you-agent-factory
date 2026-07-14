# API/CLI Response-Stream Parity Evidence (Batch 07 Gate)

Reviewer-visible integration evidence for story `you-goal-b07-stream-program-gate-004`.
This artifact records what API and CLI response-stream surfaces prove on merged
`origin/main` after the stream-responses program landed public
`FactoryResponseEvent` schemas, session response-event SSE, production
publication, and canonical CLI NDJSON.

**Status:** gate evidence recorded — API/CLI canonical encoding parity and terminal
outcome parity proven for goal/subagent integration fixtures

**Last updated:** 2026-07-14 UTC

**Audit prerequisite:** `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`

**Goal stream prerequisite:** `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`

**Subagent stream prerequisite:** `docs/internal/development/plans/you-goal/subagent-response-stream-integration.md`

## Summary verdict

| Acceptance criterion | Verdict | Evidence |
|----------------------|---------|----------|
| CLI NDJSON `response_event.event` and API SSE `data:` decode to equivalent canonical `FactoryResponseEvent` values | **Merged** | Focused smokes marshal each CLI `response_event.event` through the API SSE frame decoder (`TestNamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON`, `TestNamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON`); live API SSE delivery covered by `pkg/transports/http/server_factory_sessions_test.go` and `pkg/transports/http/servertests/server_factory_session_orchestrator_test.go` |
| Provider parity shows truthful fidelity and equivalent terminal `InvocationResponse` across primary-only and stream modes | **Merged (terminal scope)** | Mock-worker fixtures are final-only; API `POST /factory-sessions/{session_id}/invocations` and CLI JSON `invocation_result` agree on terminal outcome (goal + subagent terminal parity smokes) |
| Narrow integration corrections only; no parallel stream reimplementation | **Merged** | No broad stream-program changes in this lane; parity proofs reuse canonical helpers from stories 002–003 |
| Final gate evidence package cites audit + focused integration tests | **Merged** | This document plus stories 001–003 artifacts and smoke suites listed below |

## Proven API/CLI parity (merged scope)

### Canonical response-event encoding parity

For representative `@you/goal` and `@you/subagent` JSON response-stream fixtures:

- CLI NDJSON `recordType=response_event` records embed validated public
  `FactoryResponseEvent` values (`responseevents.ValidateEvent`).
- Each embedded event round-trips through the API SSE frame contract (`id:` equals
  decimal `sequence`, `data:` is JSON `FactoryResponseEvent`) without semantic
  loss on kind, phase, or payload.
- Live API SSE publication and reconnect semantics remain covered by transport
  unit/servertests near `pkg/transports/http/handlers_events.go`.

### Terminal invocation outcome parity

For representative successful `@you/goal` and `@you/subagent` fixtures:

- API consumers receive the shared public `InvocationResponse` envelope from
  `POST /factory-sessions/{session_id}/invocations`.
- CLI consumers running `--json --output response-stream` receive exactly one
  terminal `invocation_result` NDJSON record wrapping the same `InvocationResponse`
  shape.
- Focused smoke tests prove API and CLI response-stream terminal outcomes agree
  for the same fixture inputs:

```bash
go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON|NamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON' -count=1
```

### Provider fidelity posture on the integration surface

Packaged goal/subagent smoke fixtures use mock workers that complete without live
provider streaming fragments. That is truthful **final-only** fidelity: stream mode
still ends with the authoritative primary result and does not invent synthetic
progress events beyond synthesized terminal `RUN`/`COMPLETED` observations emitted
by the canonical mapper when dispatch streams complete.

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
go test ./pkg/transports/cli/run/... -short -run ResponseStream
go test ./pkg/factory/sessions/responsestream/... -short
go test ./pkg/transports/http/ -short -run FactoryResponseEvents
go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream|NamedSubagentResponseStream|NamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON|NamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON' -count=1
```
