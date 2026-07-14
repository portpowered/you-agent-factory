# Stream-Responses Final Audit (Batch 07 Gate)

Reviewer-visible final-audit evidence for the `you-goal-b07-stream-program-gate`
integration verification lane. This artifact maps each **Definition of Program
Completion** criterion from the stream-responses program (`stream-response-fix-plan.md`
§16, referenced by `prd.json` story 001) to **merged** code, generated artifacts,
docs, and/or passing verification commands on branch head `you-goal-b07-stream-program-gate`
after `origin/main` merged the stream-responses program (including
`stream-b09-remove-private-contract`).

**Status:** intake complete — **Definition of Program Completion satisfied on merged
public contract, canonical CLI delivery, and live API session response-event SSE
for goal/subagent integration fixtures.**

**Last updated:** 2026-07-14 UTC

**Audit story:** `you-goal-b07-stream-program-gate-001`

## Context

Batch 07 must not claim stream integration from staged plans alone. The
stream-responses program landed a public `FactoryResponseEvent` contract,
session-scoped response-event SSE, canonical CLI NDJSON, and provider fidelity
rules on `origin/main`. This audit inspects **merged** artifacts on the current
branch head and records any remaining integration residuals for the Batch 07 gate.

**Related maintainer artifacts:**

- `@you/goal` API boundary audit:
  `docs/internal/development/plans/you-goal/api-contract-audit.md`
- Goal/subagent stream integration evidence:
  `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`,
  `subagent-response-stream-integration.md`, `api-cli-response-stream-parity.md`
- Invocation/CLI response-stream wiring:
  `docs/internal/processes/invocation-relevant-files.md`
- API/process map:
  `docs/internal/processes/api-relevant-files.md`

## Summary verdict

| Criterion | Verdict | Gate impact |
|-----------|---------|-------------|
| Public `FactoryResponseEvent` + session-scoped SSE | **Merged** | OpenAPI + generated clients + `GET /factory-sessions/{session_id}/response-events` |
| CLI JSON NDJSON of canonical response events + shared invocation response | **Merged** | `response_event` + `invocation_result` NDJSON; retired private dialect rejected |
| Session-ordered internal subscriptions with gaps | **Merged (internal)** | `pkg/factory/sessions/responsestream/` + `responseeventstore/` |
| Structured provider adapters + final-only honesty | **Merged (fixture scope)** | Mock-worker goal/subagent smokes are truthful final-only |
| Normalized failure model | **Merged (terminal scope)** | Shared `InvocationResponse` terminal envelope |
| Legacy fragment isolation / removal | **Merged** | stream-b09 private-contract removal |
| Quality gates | **Merged on cited commands** | See verification table |

**Overall gate posture:** the stream-responses program's public canonical
response-event contract is **merged and proven** for CLI goal/subagent
response-stream modes and live API session SSE on the integration fixtures.
Story 004 closed the wire-compose publication gap and replaced synthetic SSE
round-trip smokes with live session SSE ↔ CLI NDJSON comparison.

---

## Criterion 1 — Public `FactoryResponseEvent` + session-scoped SSE

**Completion definition:** A public OpenAPI `FactoryResponseEvent` schema and a
session-scoped SSE route that streams those events to API consumers.

### Merged evidence inspected

| Artifact | Finding |
|----------|---------|
| `api/openapi-main.yaml` | `FactoryResponseEvent` schema; `GET /factory-sessions/{session_id}/response-events` with SSE semantics, `after_sequence`, `STREAM_GAP`, typed errors |
| `api/openapi.yaml` (bundled) | Same public contract |
| `pkg/transports/http/generated/server.gen.go` | Generated `FactoryResponseEvent` type and route handler registration |
| `ui/src/api/generated/openapi.ts` | Generated `FactoryResponseEvent` type |
| `pkg/transports/http/server_factory_sessions_test.go` | Live retained-then-live SSE delivery, stale cursor `STREAM_GAP`, typed expiry |
| `docs/reference/sessions.md` | Documents response-event stream lifecycle |

### Verdict

**Merged.** Public contract and live SSE handler are present on merged `origin/main`.

---

## Criterion 2 — CLI JSON NDJSON of canonical events + shared invocation response

**Completion definition:** CLI `--json --output response-stream` emits NDJSON
lines that decode to canonical `FactoryResponseEvent` records plus exactly one
terminal shared `InvocationResponse` / `invocation_result` record.

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/transports/cli/run/run_clean_invocation.go` | Canonical `response_event` + `invocation_result` NDJSON |
| `pkg/factory/sessions/responsestream/ndjsoncontract/` | Retired private `recordType` values rejected |
| `pkg/transports/cli/run/run_config_test.go`, `run_wire_api_test.go` | Unit/wire tests for canonical NDJSON |
| `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` | Real CLI goal smokes: validated `FactoryResponseEvent`, single terminal `InvocationResponse` |
| `tests/functional/smoke/cli_named_subagent_response_stream_smoke_test.go` | Same contract for `@you/subagent` |

### Verdict

**Merged.** Private CLI dialect removed on `origin/main`; goal/subagent smokes prove
canonical NDJSON and terminal parity.

---

## Criterion 3 — Session-ordered internal subscriptions with gaps

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/factory/sessions/responsestream/` | Monotonic sequencing, retention, gap detection |
| `pkg/factory/sessions/responseeventstore/` | Session-owned canonical event retention + subscriptions |
| `pkg/factory/sessions/service/stream.go` | Session gateway subscriptions |
| `pkg/transports/cli/run/invocation_observability.go` | CLI attachment to canonical store |

### Verdict

**Merged (internal + public store).**

---

## Criterion 4 — Structured provider adapters including final-only honesty

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/workers/provider/inference_progress.go` | Provider-boundary normalization + `CompletedFragment` |
| `pkg/factory/sessions/responsestream/fragmentmap/` | `STREAM_COMPLETED` → `RUN/COMPLETED` canonical mapping |
| Goal/subagent mock-worker smokes | Truthful final-only fidelity on integration fixtures |

### Verdict

**Merged for integration fixture scope.**

---

## Criterion 5 — Normalized failure model

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/transports/cli/run/invocation_error.go` | Stable CLI `InvocationError` codes |
| `api/components/schemas/api/InvocationResponse.yaml` | Public terminal statuses |
| Goal/subagent terminal parity smokes | API `InvocationResponse` matches CLI `invocation_result` |

### Verdict

**Merged for terminal outcome scope.**

---

## Criterion 6 — Legacy fragment isolation / removal

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| stream-b09 private-contract removal on `origin/main` | Retired private NDJSON dialect and legacy human prefixes |
| `pkg/transports/http/contracttests/` | Public artifacts omit internal stream terms |
| Goal/subagent smokes | Reject retired private `recordType` values and legacy human prefixes |

### Verdict

**Merged.**

---

## Criterion 7 — Quality gates

### Verification commands (2026-07-14 UTC)

| Command | Result | Notes |
|---------|--------|-------|
| `go test ./pkg/factory/sessions/responsestream/... ./pkg/transports/cli/run/... -count=1 -short` | **pass** | Canonical stream + CLI attachment |
| `go test ./pkg/transports/http/ -run FactoryResponseEvents -count=1 -short` | **pass** | Live SSE handler semantics |
| `go test ./tests/functional/smoke/ -run 'NamedGoalResponseStream|NamedSubagentResponseStream' -count=1` | **pass** | Goal/subagent canonical CLI smokes |
| `make test` | **pass** | Short suite on branch head |

---

## Residual register

No open Batch 07 gate residuals. Former R1/R2 public-contract gaps closed on
`origin/main` via stream-responses program merge; story 004 closed wire-compose
inference-progress publication for `InjectFactoryService`.

**Closed residuals:**

| ID | Former residual | Closed by |
|----|-----------------|-----------|
| R1.1 | No public `FactoryResponseEvent` | stream-responses OpenAPI + codegen on `origin/main` |
| R1.2 | No session response-event SSE | `GET /factory-sessions/{session_id}/response-events` |
| R2.1 | CLI private NDJSON dialect | stream-b09 canonical `response_event` NDJSON |
| R2.2 | API SSE parity unexercisable | Live goal/subagent SSE ↔ CLI NDJSON smokes + wire-compose publisher fix |
| R4-API-PUBLISH | Wire `InjectFactoryService` omitted inference-progress publishers | `NewRuntimeBuildService` now accepts sessions registry (2026-07-14) |

## Next gate stories

| Story | Depends on |
|-------|------------|
| `you-goal-b07-stream-program-gate-002` | **Closed** |
| `you-goal-b07-stream-program-gate-003` | **Closed** |
| `you-goal-b07-stream-program-gate-004` | **Closed** |
