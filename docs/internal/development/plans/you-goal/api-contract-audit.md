# @you/goal API Contract Audit (P11)

Implementation-ready contract boundary for the `@you/goal` slice. This audit
maps each relevant surface to ownership, public vs internal scope, reuse vs
change posture, and follow-on verification expectations.

**Status:** in progress (story `you-goal-p11-api-contract-audit-001` complete)

**Last updated:** 2026-06-20 UTC

## Context

The `@you/goal` planning direction is intentionally narrow: reuse existing
public Factory Session invocation, lifecycle, and dispatch contracts; keep
response-stream progress internal to runtime/session models; and limit the
expected public OpenAPI delta to `Workstation.workPropagation`. This document is
the single reviewer-verifiable artifact maintainers should cite before widening
API scope in follow-on PRs.

Canonical public vocabulary: `docs/architecture/data-model.md` (`Factory`,
`Factory Session`, `Work`, `Work Request`, dispatch lifecycle, invocation
result). Invocation equivalence rules: `docs/architecture/invocation-contract.md`.

## Audit sections

| Section | Story | Status |
|---------|-------|--------|
| [Invocation and adapter reuse boundaries](#invocation-and-adapter-reuse-boundaries) | `you-goal-p11-api-contract-audit-001` | complete |
| Response streams vs lifecycle contracts | `you-goal-p11-api-contract-audit-002` | pending |
| Public OpenAPI delta and generated-client expectations | `you-goal-p11-api-contract-audit-003` | pending |

---

## Invocation and adapter reuse boundaries

`@you/goal` is a packaged named factory (`@you/goal`, project
`builtin-goal`) resolved through the same live-session invocation path as other
authored factories. Goal-mode work must not introduce a parallel invocation or
result transport.

### Boundary matrix

| Surface | Ownership | Public / internal | @you/goal posture | Follow-on verification |
|---------|-----------|-------------------|--------------------|------------------------|
| `POST /factory-sessions/{session_id}/invocations` | Shared live-session invocation API (`invokeFactorySessionBySessionId`) | **Public** | **Reuse as-is** — canonical API entrypoint for `@you/goal` invocations against an open live Factory Session | API regression in `tests/functional/runtime_api/api_session_invocation_test.go`; contract description in `api/openapi-main.yaml` |
| `InvocationRequest` / `InvocationResponse` | OpenAPI work schemas + `pkg/apisurface` projection | **Public** | **Reuse as-is** — text-first input via `WorkContent`; terminal status and `primaryResult` on `InvocationResponse` | OpenAPI contract tests in `pkg/api/contracttests/`; generated client sync via `make generate-api` only when these schemas change |
| `Factory.invocationReturn` | Factory configuration on the active session factory | **Public** | **Reuse as-is** — sole public selector for the final primary result; default `SUBMITTED_WORK_TERMINAL` when omitted | Factory validation before runtime; primary-result selection tests in `pkg/invocations/primary_result_test.go` |
| `you run --factory` / `you run --named @you/goal` | CLI transport adapter in `pkg/cli/run/` | **Public CLI** | **Reuse as-is** — must call the same shared resolver and primary-result selector as the API; no goal-specific transport contract | CLI tests in `pkg/cli/run/run_invocation_test.go`, `pkg/cli/root_run_factory_prompt_test.go`, and `pkg/cli/root_run_test.go` (`run --named @you/goal`) |
| Shared invocation resolver (`pkg/invocations`) | Backend-owned input and return-policy logic | **Internal shared contract** | **Reuse as-is** — `ResolveTextInput` / `ResolveAPITextInputContent` for input; `ResolvePrimaryResult` for return policy | Unit tests in `pkg/invocations/input_test.go` and `pkg/invocations/primary_result_test.go` |
| Goal-only result endpoint | — | — | **Out of scope** — this slice does **not** add `/goal/...`, `/results` variants, or any goal-named primary-result route | N/A — reject proposals that bypass `InvocationResponse.primaryResult` |

### Canonical public API entrypoint

`POST /factory-sessions/{session_id}/invocations` is the canonical public API
entrypoint for `@you/goal`. The route:

- Accepts one text-first `InvocationRequest` against an already-open live
  Factory Session (`session_id`, typically `~default` for CLI-started sessions).
- Submits work into the session runtime and blocks until terminal
  primary-result selection completes (subject to `timeoutMillis`).
- Returns `InvocationResponse` with `status` and, on success,
  `primaryResult` as `WorkContent`.

Durable JavaScript workflow execution routes (`POST /factory-sessions/async`,
`POST /factory-sessions/sync`, `GET /factory-sessions/{session_id}/results`)
are separate public surfaces for orchestrator-backed durable sessions. They are
**not** the `@you/goal` invocation entrypoint for this slice. Goal follow-on
work that needs one-shot live invocation against the packaged factory must use
the existing invocation route, not durable start/result routes.

**Authoritative references:**

- OpenAPI: `api/openapi-main.yaml` (`/factory-sessions/{session_id}/invocations`)
- Handler: `pkg/api/handlers_factory.go` → `FactoryService.InvokeFactorySession`
- Service: `pkg/service/model_catalog.go` (`InvokeFactorySession`,
  `resolveSessionInvocationInput`, session wait + primary-result projection)

### `Factory.invocationReturn` as the public result selector

`Factory.invocationReturn` on the active session factory remains the **only**
public configuration for selecting the invocation's final primary result:

| Policy | Behavior |
|--------|----------|
| *(omitted)* | `SUBMITTED_WORK_TERMINAL` — follow submitted work to terminal output and return that content as `InvocationResponse.primaryResult` |
| `EXPLICIT` | Select by configured `workTypeName`, `terminalState`, and optional `workName` within the invocation submit scope |

This slice does **not** add:

- A goal-only result endpoint or response wrapper.
- Per-invocation return-policy overrides on `InvocationRequest`.
- A separate public selector for streaming or partial progress (those belong to
  internal response-stream models; see story 002).

Primary-result selection is implemented once in `pkg/invocations/primary_result.go`
(`ResolvePrimaryResult`) and consumed by `pkg/service/model_catalog.go` after
the session invocation wait completes. Unresolved selection surfaces as
`InvocationResponse` status `FAILED` with code
`INVOCATION_PRIMARY_RESULT_UNRESOLVED` and no `primaryResult`, matching the
documented OpenAPI contract.

The built-in `@you/goal` factory payload (`pkg/config/layout.go`,
`BuiltInGoalFactoryJSON`) does not currently declare `invocationReturn`; runtime
behavior therefore uses the documented `SUBMITTED_WORK_TERMINAL` default until
an authored policy is added through normal factory configuration — not through
new API routes.

### CLI adapter reuse requirements

CLI transports must remain thin adapters over the shared invocation contract:

| CLI path | Adapter responsibility | Must not |
|----------|------------------------|----------|
| `you run --factory <factory.json> <text>` | Resolve positional/stdin text via `ResolveFactoryInvocationInput` → build `InvocationRequest` → `InvokeFactorySession` on `~default` | Invent separate conflict, empty-input, or primary-output rules |
| `you run --named @you/goal <text>` | Resolve named factory to the same live-session invocation flow as `--factory` | Add goal-specific flags, headers, or response shapes |
| Success output | Format `InvocationResponse.primaryResult` (e.g. text extraction in `writeInvocationSuccess`) | Return a goal-specific JSON envelope not derived from `InvocationResponse` |

**Shared resolver chain (CLI and API must converge here):**

1. **Input:** `pkg/invocations.ResolveTextInput` (CLI positional/stdin) and
   `pkg/invocations.ResolveAPITextInputContent` (API `WorkContent`) — same
   conflict and empty-input errors (`INVOCATION_INPUT_SOURCE_CONFLICT`,
   `INVOCATION_INPUT_EMPTY`).
2. **Submit + wait:** `FactoryService.InvokeFactorySession` with session
   `invocationReturn` from runtime factory config.
3. **Primary result:** `invocations.ResolvePrimaryResult` against selected-tick
   world state — same policy semantics for CLI and API callers.

Transport files: `pkg/cli/run/factory_invocation_input.go`,
`pkg/cli/run/run.go`. Equivalence requirement is normative in
`docs/architecture/invocation-contract.md`.

### Reviewer evidence for this boundary

Maintainers reviewing `@you/goal` invocation follow-on PRs should verify:

| Evidence | What it proves |
|----------|----------------|
| `docs/architecture/invocation-contract.md` | CLI/API equivalence and return-policy ownership are documented public contract |
| `pkg/invocations/input.go`, `pkg/invocations/primary_result.go` | Single backend-owned resolver and selector — no transport-specific forks |
| `pkg/service/model_catalog.go` (`InvokeFactorySession`, `resolveSessionInvocationInput`) | API path uses shared resolver; `invocationReturn` read from session factory config |
| `pkg/cli/run/factory_invocation_input.go` | CLI path uses `invocations.ResolveTextInput` and `InvokeFactorySession` |
| `pkg/invocations/input_test.go`, `pkg/invocations/primary_result_test.go` | Unit coverage for input conflicts and both return policies |
| `pkg/cli/run/run_invocation_test.go` | CLI request construction from positional/stdin sources |
| `tests/functional/runtime_api/api_session_invocation_test.go` | End-to-end API invocation returns `primaryResult`; explicit `invocationReturn` policy honored |
| `pkg/cli/root_run_test.go` (`run --named @you/goal`) | Named goal factory routes through standard invocation mode |

**Follow-on implementation PRs** that touch invocation behavior should extend
the evidence above (API functional tests, CLI run tests, and
`pkg/invocations` unit tests) rather than adding goal-specific routes or
adapters. Public OpenAPI or generated-client changes are **not** required for
invocation reuse in this slice.

---

## Response streams vs lifecycle contracts

*Pending story `you-goal-p11-api-contract-audit-002`.*

## Public OpenAPI delta and generated-client expectations

*Pending story `you-goal-p11-api-contract-audit-003`.*
