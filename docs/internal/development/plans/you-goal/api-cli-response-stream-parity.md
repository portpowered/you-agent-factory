# API/CLI Response-Stream Parity Evidence (Batch 07 Gate)

Reviewer-visible integration evidence for story `you-goal-b07-stream-program-gate-004`.
This artifact records what API and CLI response-stream surfaces prove on merged
`origin/main` after the stream-responses program landed public
`FactoryResponseEvent` schemas, session response-event SSE, production
publication, and canonical CLI NDJSON.

**Status:** gate evidence recorded — live API session response-event SSE and CLI
NDJSON canonical semantics proven for goal/subagent integration fixtures

**Last updated:** 2026-07-14 UTC

**Audit prerequisite:** `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`

**Goal stream prerequisite:** `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`

**Subagent stream prerequisite:** `docs/internal/development/plans/you-goal/subagent-response-stream-integration.md`

## Summary verdict

| Acceptance criterion | Verdict | Evidence |
|----------------------|---------|----------|
| CLI NDJSON `response_event.event` and live API SSE `data:` decode to equivalent canonical `FactoryResponseEvent` values | **Merged** | `TestNamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON` and `TestNamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON` subscribe to `GET /factory-sessions/~default/response-events` during API invocation, then compare decoded SSE events to CLI `response_event` NDJSON semantics (kind/phase/payload fingerprints) |
| Provider parity shows truthful fidelity and equivalent terminal `InvocationResponse` across primary-only and stream modes | **Merged (terminal scope)** | Mock-worker fixtures are final-only; API `POST /factory-sessions/{session_id}/invocations` and CLI JSON `invocation_result` agree on terminal outcome (goal + subagent terminal parity smokes) |
| Narrow integration corrections only; no parallel stream reimplementation | **Merged** | Wire compose now wires inference-progress publication into `InjectFactoryService` runtime builds (matching `BuildFactoryService`); focused smokes only |
| Final gate evidence package cites audit + focused integration tests | **Merged** | This document plus stories 001–003 artifacts and smoke suites listed below |

## Proven API/CLI parity (merged scope)

### Live canonical response-event parity

For representative `@you/goal` and `@you/subagent` JSON response-stream fixtures:

- A functional API server loads the packaged factory and mock-worker topology.
- A goroutine opens `GET /factory-sessions/~default/response-events` before `POST /factory-sessions/~default/invocations` completes.
- Decoded live SSE `FactoryResponseEvent` records validate through `responseevents.ValidateEvent` and match CLI `response_event` kind/phase/payload semantics for the integration fixture.
- API SSE frame contract (`id:` equals decimal `sequence`, `data:` is JSON `FactoryResponseEvent`) is asserted for each live API event.

**Fidelity note:** the API text-invocation fixture on a pre-started service-mode server may emit fewer terminal `RUN/COMPLETED` observations than a fresh bootstrap CLI `--named` invocation because dispatch breadth differs; parity is proven on the events both surfaces actually publish (truthful final-only), not on byte-identical dispatch IDs.

### Terminal invocation outcome parity

For representative successful `@you/goal` and `@you/subagent` fixtures:

- API consumers receive the shared public `InvocationResponse` envelope from
  `POST /factory-sessions/{session_id}/invocations`.
- CLI consumers running `--json --output response-stream` receive exactly one
  terminal `invocation_result` NDJSON record wrapping the same `InvocationResponse`
  shape.
- Focused smoke tests prove API and CLI response-stream terminal outcomes agree
  for the same fixture inputs.

```bash
go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON|NamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON' -count=1
```

### Provider fidelity posture on the integration surface

Packaged goal/subagent smoke fixtures use mock workers that complete without live
provider streaming fragments. That is truthful **final-only** fidelity: stream mode
still ends with the authoritative primary result and does not invent synthetic
progress events beyond synthesized terminal `RUN`/`COMPLETED` observations emitted
by the canonical mapper when dispatch streams complete.

## Integration correction (story 004)

`compose.InjectFactoryService` previously built runtime bundles without session-scoped
inference-progress publishers (`NewRuntimeBuildService` passed `nil` factories).
That prevented wire-composed API servers from publishing canonical response events
into the session store. Wire now passes the sessions registry into
`NewRuntimeBuildService`, matching `BuildFactoryService` / `NewFactoryServiceCollaborators`.

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
go test ./cmd/factory/compose/ -run TestInjectFactoryService_MatchesBuildFactoryServiceCollaborators -count=1
go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream|NamedSubagentResponseStream|NamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal|NamedGoalResponseStream_APISSEMatchesCLIResponseEventNDJSON|NamedSubagentResponseStream_APISSEMatchesCLIResponseEventNDJSON' -count=1
```
