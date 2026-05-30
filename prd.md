# PRD: CLI Submit Response Contract v3 (`you submit`)

## Introduction

`you submit` returns HTTP `201` and a trace id, but operators and agents cannot tell which **work id**, **name**, or **work type** were created without immediately running `you work list`. That gap caused duplicate ingress attempts and missed verification during docs PRD submission exercises.

This project makes successful `you submit` output actionable for humans and scripts, extends the `POST /work` response contract so `workId` is available at submit time, and documents the submit → verify loop in agent-facing docs. Field names (`workId`, `name`, `workTypeName`, `traceId`, `sessionId`, `endpointPath`) are the naming source of truth for unary submit and align with [`prd-cli-submit-batch.md`](../prd-cli-submit-batch.md) batch output.

## Context

| | |
|---|---|
| **Customer ask** | Implement [`prd-cli-submit-response-contract.md`](../prd-cli-submit-response-contract.md): actionable success output and `--json` `workId` fields for `you submit`. |
| **Problem** | Success stdout is only `Submitted work: <traceId>\n`; `--json` emits `factoryapi.SubmitWorkResponse` with `traceId` only. Agents cannot run `you work show <work-id>` or filter by name without a follow-up list call. |
| **Solution** | Return `workId`, `name`, and `workTypeName` from `POST /work` (and session-scoped submit), render human and JSON CLI confirmation including CLI routing metadata, improve failure messages, and document the verify loop in `you docs agents` and `you docs work`. |

## Project-level acceptance criteria

- [ ] On HTTP `201`, human-mode `you submit` prints submitted `name`, `workTypeName`, `traceId`, and `workId` when the API returns it, plus a one-line next-step hint (`you work show <work-id>` or `you work list --name <name>`).
- [ ] On HTTP `201` with global `--json`, stdout is one JSON object with at minimum: `workId`, `name`, `workTypeName`, `traceId`, `sessionId`, `endpointPath` (CLI-derived session and path fields populated from the request context).
- [ ] Non-`201` responses exit non-zero with status and a bounded API error summary; no success confirmation line on stdout.
- [ ] OpenAPI `SubmitWorkResponse` and generated server/CLI types include the new API fields; contract tests cover the schema.
- [ ] Agent docs describe the submit → wait → verify loop using the new output fields.
- [ ] Quality gate: backend and CLI typecheck, lint, and relevant tests pass before merge.

## Goals

- Human-mode success tells the operator what was submitted and what command to run next.
- `--json` success is a single parseable confirmation object scripts can use without listing work.
- API returns stable `workId` for unary submit using the same id generation rules as batch normalization (`batch-<requestId>-<name>` when caller omits explicit `workId`).
- Errors distinguish HTTP/API rejection from transport unreachable; include existing work id in error text when the API returns it on conflict.
- Preserve existing verbose diagnostics on stderr without logging full payload bodies.

## User Stories

### cli-submit-response-contract-v3-001: API returns work identifiers on submit

**Description:** As an API client, I want `POST /work` (and session-scoped submit) to return `workId`, `name`, and `workTypeName` with `traceId` so callers know which work row was accepted without listing.

**Acceptance Criteria:**

- [ ] OpenAPI `SubmitWorkResponse` includes `workId`, `name`, and `workTypeName` (camelCase) alongside required `traceId`; regenerate `pkg/api/generated` artifacts.
- [ ] `POST /work` and `POST /factory-sessions/{sessionId}/work` success bodies populate all four fields for a valid single-work submit.
- [ ] `workId` matches the normalized submit id (`batch-<requestId>-<name>` when the request does not specify an explicit work id).
- [ ] Idempotent resubmit with the same `requestId` returns the same `workId` and `traceId` with HTTP `201` (existing engine behavior).
- [ ] API unit test asserts response JSON shape from a mocked factory submit.
- [ ] Typecheck passes
- [ ] Tests pass

### cli-submit-response-contract-v3-002: Human success output and verify hint

**Description:** As an operator, I want `you submit` to print what was created and a suggested follow-up command so I can verify without guessing filters.

**Acceptance Criteria:**

- [ ] On HTTP `201`, stdout includes submitted `name`, `workTypeName`, `traceId`, and `workId` when present in the API response.
- [ ] Prints exactly one follow-up hint line: `you work show <work-id>` when `workId` is non-empty, otherwise `you work list --name <name>`.
- [ ] Does not print the full HTTP response body or request payload on stdout.
- [ ] Verbose diagnostics remain on stderr only (unchanged policy).
- [ ] CLI test with httptest server asserts human stdout contains name, work type, trace, work id, and hint when API returns enriched response.
- [ ] Typecheck passes
- [ ] Tests pass

### cli-submit-response-contract-v3-003: JSON success confirmation object

**Description:** As a script author, I want `you --json submit` to emit one object with identifiers and routing metadata so automation can verify work without parsing human text.

**Acceptance Criteria:**

- [ ] On HTTP `201` with global `--json`, stdout is one JSON object with fields: `workId`, `name`, `workTypeName`, `traceId`, `sessionId`, `endpointPath`.
- [ ] `sessionId` reflects the effective session label from `--session` (including default-session alias behavior via `sessionpath`).
- [ ] `endpointPath` is the scoped submit path used for the request (e.g. `/work` or `/factory-sessions/<id>/work`).
- [ ] Exit code is `0` on success.
- [ ] CLI test unmarshals stdout and asserts all required keys and values against a mocked `201` response.
- [ ] Typecheck passes
- [ ] Tests pass

### cli-submit-response-contract-v3-004: Clear failure output without false success

**Description:** As an agent, I want duplicate or invalid submit failures to be obvious so I do not treat a rejection as success or re-submit blindly.

**Acceptance Criteria:**

- [ ] Non-`201` responses exit non-zero; stdout contains no success confirmation line.
- [ ] Error message includes HTTP status and bounded `ErrorResponse.message` when the body parses as API error JSON.
- [ ] When the error body includes a work identifier field, error text mentions it (e.g. existing `workId` on conflict payloads).
- [ ] Transport failures (`factory not reachable`) remain distinct from API rejection messages.
- [ ] CLI tests cover at least `400` with `ErrorResponse` and unreachable server cases.
- [ ] Typecheck passes
- [ ] Tests pass

### cli-submit-response-contract-v3-005: Document submit verify loop for agents

**Description:** As an agent author, I want `you docs agents` and `you docs work` to describe how to confirm a submit using the new output so the wave-2 playbook is accurate.

**Acceptance Criteria:**

- [ ] `you docs agents` includes a submit → wait → verify subsection referencing `you submit` success fields and `you work show` / `you work list --name`.
- [ ] `you docs work` documents `POST /work` success response fields (`workId`, `name`, `workTypeName`, `traceId`) and CLI `--json` confirmation fields (`sessionId`, `endpointPath`).
- [ ] Docs use camelCase field names matching OpenAPI and CLI JSON output.
- [ ] `pkg/cli/docs` tests that assert topic content still pass after updates.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional requirements

- FR-1: Extend `SubmitWorkResponse` in OpenAPI with `workId`, `name`, `workTypeName`.
- FR-2: Populate submit handler responses from normalized submit metadata (request id, submitted name, work type, trace id, resolved work id).
- FR-3: Human `you submit` success formatting and follow-up hint as specified in US-002.
- FR-4: `you --json submit` emits the confirmation object in US-003 (API fields plus CLI `sessionId` and `endpointPath`).
- FR-5: Failure paths in US-004; no success line on stderr or stdout for failures.
- FR-6: Align field naming with batch submit PRD for shared `works[]` identifiers where both surfaces ship together.

## Non-goals

- Changing submit HTTP routes, session model, or request body shape for unary submit.
- Batch submit via `you submit` (remains `you submit batch` / API / `you run --work`) — see [`prd-cli-submit-batch.md`](../prd-cli-submit-batch.md).
- Unary payload input modes (stdin, positional) — see [`prd-cli-submit-unary-input.md`](../prd-cli-submit-unary-input.md).
- `you work list` filters or `you work show` — see [`prd-cli-work-inspection.md`](../prd-cli-work-inspection.md).
- Dashboard UI submit confirmation changes.

## High-level technical design

1. **API layer (`pkg/api`, `api/openapi.yaml`)**  
   Extend `SubmitWorkResponse`. After `SubmitWorkRequest` / `SubmitWorkRequestForSession` succeeds, map normalized work metadata into the response. `WorkRequestSubmitResult` may need optional work identity fields if the handler cannot read them from the request alone; prefer values from normalization to avoid drift.

2. **CLI layer (`pkg/cli/submit`)**  
   Parse enriched `SubmitWorkResponse`. Human mode: format a short multi-field summary plus hint. JSON mode: encode a CLI confirmation struct (API fields + `sessionId` + `endpointPath` from `SubmitConfig`). Keep diagnostics in `clidiag` on stderr.

3. **Contracts**  
   Regenerate `pkg/api/generated`. Update `pkg/api/contracttests` for `SubmitWorkResponse` properties. Coordinate with batch PRD if `UpsertWorkRequestResponse.works[]` ships in the same wave.

4. **Verification**  
   - API: `pkg/api/server_submit_work_test.go` (or adjacent) for response body.  
   - CLI: `pkg/cli/submit/submit_test.go` httptest cases for human, JSON, and error paths.  
   - Docs: `pkg/cli/docs/docs_test.go` content assertions.

## Supporting technical considerations

- **Package ownership:** API response shaping in `pkg/api/handlers.go`; CLI rendering in `pkg/cli/submit`; OpenAPI in `api/components/schemas/api/SubmitWorkResponse.yaml`.
- **Default session:** Use existing `sessionpath.ScopedPath` and `clidiag.SessionLabel` for consistent `sessionId` in JSON output.
- **Interim fallback:** If API work is split across PRs, CLI may print trace + name and hint `you work list --name` only when `workId` is absent; v3 assumes API story 001 ships first.
- **Coordination:** [`prd-cli-work-inspection.md`](../prd-cli-work-inspection.md) supplies `you work show` and `--name` filter for the verify loop; [`prd-docs-agents-consolidation-wave2.md`](../prd-docs-agents-consolidation-wave2.md) references this contract.

## Success metrics

- An agent can copy `workId` from `you --json submit` and run `you work show <work-id>` without an intermediate list in the common case.
- Zero false-positive success lines on failed submit in CLI tests.
- No regression in submit verbose stderr behavior (payload content still not logged).

## Open questions

None — work id generation follows existing batch normalization; session and endpoint metadata come from CLI config.
