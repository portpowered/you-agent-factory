# Stream-Responses Final Audit (Batch 07 Gate)

Reviewer-visible final-audit evidence for the `you-goal-b07-stream-program-gate`
integration verification lane. This artifact maps each **Definition of Program
Completion** criterion from the stream-responses program (`stream-response-fix-plan.md`
§16, referenced by `prd.json` story 001) to **merged** code, generated artifacts,
docs, and/or passing verification commands on branch head `you-goal-b07-stream-program-gate`.

**Status:** intake complete — **blocking residuals recorded** (gate does not
soft-pass)

**Last updated:** 2026-07-12 UTC

**Audit story:** `you-goal-b07-stream-program-gate-001`

## Context

Batch 07 must not claim stream integration from staged plans alone. The
stream-responses program is a separate lane that was expected to land a public
`FactoryResponseEvent` contract, session-scoped response-event SSE, CLI/API
parity, and provider fidelity rules. This audit inspects **merged** artifacts on
the current branch head and records explicit residuals where the published
completion definition is not yet satisfied.

**Related maintainer artifacts:**

- `@you/goal` API boundary audit:
  `docs/internal/development/plans/you-goal/api-contract-audit.md` (response
  streams documented as **internal-only** for the P11 slice)
- Invocation/CLI response-stream wiring:
  `docs/internal/processes/invocation-relevant-files.md`
- API/process map for internal stream boundaries:
  `docs/internal/processes/api-relevant-files.md`

**Missing external plan sources (not in this worktree):**

- `docs/temp/projects/stream-responses/stream-response-fix-plan.md`
- `docs/temp/projects/you-goal-fixes/systematic-fix-plan.md` (S23)

Criteria below follow the story-001 acceptance list and `prd.md` FR-1 wording.

## Summary verdict

| Criterion | Verdict | Gate impact |
|-----------|---------|-------------|
| Public `FactoryResponseEvent` + session-scoped SSE | **Residual — not merged** | Blocks downstream API/CLI parity stories |
| CLI JSON NDJSON of **canonical** response events + shared invocation response | **Partial** — terminal `InvocationResponse` merged; progress records are CLI-private dialect | Blocks story 002/004 canonical-event claims |
| Session-ordered internal subscriptions with gaps | **Merged (internal)** | Satisfies internal retention/subscription slice only |
| Structured provider adapters + final-only honesty | **Partial** — normalization merged; explicit final-only fidelity contract not reviewer-proven | Narrow follow-up may be required in story 004 |
| Normalized failure model | **Partial** — invocation + stream terminal markers merged | Consumer parity still unproven across API SSE |
| Legacy fragment isolation / removal | **Merged** — internal terms isolated from public artifacts and durable history | Satisfies isolation slice |
| Quality gates | **Merged on cited commands** | See verification table |

**Overall gate posture:** internal `SessionResponseStream` infrastructure and
CLI `--output response-stream` attachment are merged and tested, but the
stream-responses program's **public canonical response-event contract**
(`FactoryResponseEvent` + session-scoped SSE) is **not** present in merged
OpenAPI or generated clients. Stories 002–004 must treat this as a blocking
prerequisite or record narrow integration corrections; this lane must not
invent a parallel stream program.

---

## Criterion 1 — Public `FactoryResponseEvent` + session-scoped SSE

**Completion definition:** A public OpenAPI `FactoryResponseEvent` (or
successor canonical response-event schema) and a session-scoped SSE route that
streams those events to API consumers.

### Merged evidence inspected

| Artifact | Finding |
|----------|---------|
| `api/openapi-main.yaml` | No `FactoryResponseEvent` schema; no `/factory-sessions/{session_id}/response-events` (or equivalent) route |
| `api/openapi.yaml` (bundled) | Same absence — public SSE remains `FactoryEvent` on `/factory-sessions/{session_id}/events` and compatibility `/events` |
| `pkg/api/generated/server.gen.go` | No generated `FactoryResponseEvent` type |
| `ui/src/api/generated/openapi.ts` | No generated `FactoryResponseEvent` type |
| `docs/internal/development/plans/you-goal/api-contract-audit.md` § Response streams are internal-only | Explicitly rejects public response-stream REST/SSE for the `@you/goal` slice |

### Residual (blocking)

- **R1.1:** Public `FactoryResponseEvent` schema is absent from authored OpenAPI
  and all generated Go/TypeScript clients.
- **R1.2:** Session-scoped response-event SSE is absent; only canonical
  `FactoryEvent` SSE exists (`GET /factory-sessions/{session_id}/events`).

**Suggested verification when landed:** `make generate-api`, `make api-smoke`,
and a new functional test against the response-event SSE route decoding to
`FactoryResponseEvent`.

---

## Criterion 2 — CLI JSON NDJSON of canonical events + shared invocation response

**Completion definition:** CLI `--json --output response-stream` emits NDJSON
lines that decode to the same canonical response-event records as the public
API contract, plus exactly one terminal shared `InvocationResponse` /
`primary_result` record.

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/cli/run/run_clean_invocation.go` | JSON record types: `progress`, `stream_gap`, `compaction`, `primary_result` |
| `docs/reference/config.md` § Response-stream stdout mode | Documents CLI-private NDJSON `recordType` vocabulary |
| `pkg/cli/run/run_config_test.go` | Unit tests for NDJSON ordering, gap/compaction records, and terminal `primary_result` wrapping `InvocationResponse` |
| `pkg/cli/run/run_wire_api_test.go` | Wire tests for JSON response-stream attachment and terminal `primary_result` |
| `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` | Real CLI smoke: human mode returns `primaryResult`; JSON mode ends with `primary_result` + `COMPLETED` |
| `api/components/schemas/api/InvocationResponse.yaml` + `pkg/apisurface` | Terminal record reuses shared public invocation envelope |

### Residual (blocking for canonical parity)

- **R2.1:** CLI `progress` / `stream_gap` / `compaction` records are a
  **CLI-private NDJSON dialect** (`recordType` + flattened fields), not
  `FactoryResponseEvent` (or successor) canonical response-event records.
- **R2.2:** Without criterion 1, CLI NDJSON cannot be proven equivalent to API
  SSE response events.

**Merged partial:** Terminal `primary_result` record shares the authoritative
`InvocationResponse` contract with primary-only mode (story 002 can build on
this).

---

## Criterion 3 — Session-ordered internal subscriptions with gaps

**Completion definition:** Internal consumers resume ordered session response
streams with explicit gap/compaction signaling when behind the retained window.

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/factorysessions/responsestream/types.go` | `Event.Sequence`, `EventKind`, `ReadResult.BehindRetainedWindow`, `CompactionSummary` |
| `pkg/factorysessions/responsestream/stream.go` | Monotonic sequencing, retention enforcement, gap detection |
| `pkg/factorysessions/responsestream/subscription.go` | Ordered `Subscribe` / `Next` with live wakeups |
| `pkg/factorysessions/responsestream/stream_test.go` | Monotonic sequence assignment, retention metadata, behind-window reads |
| `pkg/factorysessions/responsestream/subscription_test.go` | Subscriber ordering and completion behavior |
| `pkg/factorysessions/responsestream/publisher_test.go` | Publication + gap signaling under retention pressure |
| `pkg/factorysessions/service/stream.go` | `SubscribeSessionResponseStream`, dispatch-scoped subscriptions |
| `pkg/cli/run/invocation_observability.go` | CLI attachment consumes internal stream read results |

### Verdict

**Merged (internal scope).** This criterion is satisfied for the internal
`SessionResponseStream` model. It does **not** by itself satisfy public API or
canonical CLI NDJSON contracts.

---

## Criterion 4 — Structured provider adapters including final-only honesty

**Completion definition:** Provider subprocess output is normalized into
internal stream fragments with truthful fidelity (full native stream vs
final-only providers).

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/workers/provider/inference_progress.go` | Provider-boundary normalization (`PROGRESS_FRAGMENT`, `RESPONSE_FRAGMENT`, `STREAM_COMPLETED`, `STREAM_FAILED`); Codex SSE parsing with diagnostic classes |
| `pkg/workers/provider/cursor/stream.go` | Cursor stream normalization |
| `pkg/factorysessions/stream/fragment.go` | Fragment normalization into `responsestream.Event` |
| `pkg/service/factory_build.go` | Injects session-owned progress publisher into provider execution |
| `pkg/service/runtime_session_runtime_test.go` (`TestFactoryService_InferenceProgressPublisher_*`) | Internal fragments publish without mutating canonical `FactoryEvent` history; terminal completed/failed markers |

### Residual (narrow)

- **R4.1:** No merged maintainer-facing matrix documents which packaged
  providers are final-only vs full-stream for the integration surface; honesty
  is implied by adapter behavior and tests but not recorded as gate evidence.
- **R4.2:** Story 004 should add representative fixture proof that final-only
  providers do not emit false `TEXT_DELTA` progress.

---

## Criterion 5 — Normalized failure model

**Completion definition:** Stream and invocation failures surface through a
shared, stable failure vocabulary (CLI, API, internal stream).

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/cli/run/invocation_error.go` | Stable `InvocationError` codes for CLI non-success |
| `api/components/schemas/api/InvocationResponse.yaml` | Public terminal statuses and error codes |
| `pkg/factorysessions/responsestream/types.go` | `EventKindStreamFailed`, `EventTypeFailed` / `EventTypeCanceled` |
| `pkg/cli/run/run_clean_invocation.go` | Human `--- invocation outcome ---` section; JSON terminal `primary_result` failure envelope |
| `pkg/workers/provider/inference_progress.go` | `FailedFragment` terminal markers |
| `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` | Smoke path exercises successful terminal selection (failure parity deferred to story 002) |

### Residual

- **R5.1:** Without public response-event SSE, API stream failure parity is
  unproven for the integration surface.
- **R5.2:** Focused goal/subagent stream **failure** smokes are not yet cited
  in this gate package (story 002 scope).

---

## Criterion 6 — Legacy fragment isolation / removal

**Completion definition:** Internal response-stream vocabulary stays out of
public OpenAPI, generated clients, and durable `FactoryEvent` history.

### Merged evidence

| Evidence | What it proves |
|----------|----------------|
| `pkg/api/contracttests/generated_contract_common_test.go` (`assertTextOmitsInternalResponseStreamTerms`) | Public generated artifacts omit internal stream terms |
| `pkg/service/runtime_session_runtime_test.go` (`TestFactoryService_InferenceProgressPublisher_DoesNotEmitCanonicalFactoryEvents`) | Publishing internal fragments does not append canonical factory events |
| `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` (`TestNamedGoalResponseStream_DurableFactoryEventsOmitInternalStreamTerms`) | Durable goal invocation history omits internal stream identifiers |
| `docs/internal/development/plans/you-goal/api-contract-audit.md` | Public boundary policy: no new `FactoryEventType` stream chunk types |

### Verdict

**Merged.** Isolation is proven at artifact and durable-history boundaries.

---

## Criterion 7 — Quality gates

### Verification commands (2026-07-12 UTC)

All commands ran from worktree
`you-goal-b07-stream-program-gate` on Windows unless noted.

| Command | Result | Notes |
|---------|--------|-------|
| `go test ./pkg/factorysessions/responsestream/... ./pkg/cli/run/... -count=1 -short` | **pass** | Core internal stream + CLI attachment unit tests |
| `go test ./pkg/api/contracttests/ -count=1 -short` | **pass** | Public artifact boundary tests including internal-term omission |
| `go test ./pkg/service/ -run "InferenceProgressPublisher\|SessionResponseStream" -count=1 -short -timeout 120s` | **pass** | Service-layer internal stream integration |

### Recommended broader verification (not rerun in this iteration)

| Command | Purpose |
|---------|---------|
| `make test` | Short Go suite including functional smokes |
| `make verify-fast` | Dashboard typecheck + short UI/Go tests |
| `go test ./tests/functional/smoke/ -run TestNamedGoalResponseStream -count=1 -timeout 600s` | Real CLI goal response-stream smoke (slow; skipped under `-short`) |

---

## Blocking residual register (fail-open)

| ID | Residual | Blocks |
|----|----------|--------|
| R1.1 | No public `FactoryResponseEvent` schema in OpenAPI / generated clients | API response-event streaming (story 004) |
| R1.2 | No session-scoped response-event SSE route | API/CLI parity (story 004) |
| R2.1 | CLI NDJSON uses private `recordType` dialect, not canonical response events | Canonical stream claims for goal/subagent (stories 002–003) |
| R2.2 | API SSE parity cannot be exercised without R1 | Story 004 |
| R4.1 | Provider fidelity matrix not recorded as gate evidence | Story 004 provider parity section |
| R5.1 | API stream failure parity unproven | Story 004 |

**Gate rule:** Downstream stories must either close residuals with **narrow**
integration corrections or explicitly cite these IDs as exclusions. This audit
does **not** authorize re-implementing the stream-responses program inside the
Batch 07 gate lane.

---

## Ownership map (merged internal stack)

For reviewers tracing merged implementation without the external plan tree:

| Layer | Owner packages / docs |
|-------|----------------------|
| Internal stream model | `pkg/factorysessions/responsestream/` |
| Session gateway + subscriptions | `pkg/factorysessions/service/stream.go`, `pkg/factorysessions/stream/` |
| Provider normalization | `pkg/workers/provider/inference_progress.go`, `pkg/workers/provider/cursor/` |
| CLI attachment + NDJSON | `pkg/cli/run/run_clean_invocation.go`, `pkg/cli/run/invocation_observability.go` |
| Public boundary policy | `docs/internal/development/plans/you-goal/api-contract-audit.md` |
| Operator docs | `docs/reference/config.md` § Response-stream stdout mode |

---

## Next gate stories

| Story | Depends on |
|-------|------------|
| `you-goal-b07-stream-program-gate-002` | Prove `@you/goal` human/JSON streams against **canonical** contract or cite R2.1 |
| `you-goal-b07-stream-program-gate-003` | Same as 002 for `@you/subagent` |
| `you-goal-b07-stream-program-gate-004` | Close or explicitly record R1.*, R2.2, R4.1, R5.1 |
