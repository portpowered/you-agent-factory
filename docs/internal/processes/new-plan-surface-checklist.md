# New Plan Surface Checklist

Use this checklist when writing a PRD, `prd.json`, task breakdown, or implementation plan. It complements `docs/internal/standards/code/planning-standards.md` by making sure every plan deliberately considers the repository surfaces that often need coordinated work.

The answer for each surface can be "not applicable", but the plan should make that decision visible when the surface is plausibly related to the feature.

## Required Planning Pass

- Customer behavior: define the user-visible or operator-visible outcome, the current gap, and the success condition.
- Backend runtime: decide whether the engine, scheduler, worker execution, persistence, replay, resource, timeout, cancellation, or concurrency behavior changes.
- API contract: decide whether OpenAPI routes, schemas, enums, error responses, generated server code, generated client code, or compatibility behavior changes.
- Event stream: decide whether canonical factory events, event payloads, event ordering, replay artifacts, or event-derived projections change.
- CLI: decide whether users need command-line controls, JSON output, stable error messages, pagination, filtering, or automation-friendly flags.
- Website: decide whether dashboard widgets, current-selection panels, forms, state hooks, accessibility, responsive behavior, loading, empty, error, and success states change.
- Config and authoring: decide whether `factory.json`, split `AGENTS.md` files, runtime lookup, docs examples, validation rules, or migration guidance change.
- Observability: decide whether logs, safe diagnostics, dashboard projections, provider or script diagnostics, metrics, or operator summaries change.
- Security and permissions: decide whether the work introduces sensitive content, external callbacks, idempotency keys, authorization expectations, filesystem/process access, or audit requirements.
- Failure modes: decide how invalid input, stale state, duplicate requests, partial completion, process restart, timeout, cancellation, provider/script failure, and retry behavior should work.
- Generated artifacts: decide which generated Go, TypeScript, OpenAPI, story, fixture, or replay artifacts must be regenerated or intentionally left unchanged.
- Tests and evidence: name the unit, integration, functional, contract, UI, storybook, accessibility, race, stress, replay, or manual QA evidence that will prove the change.
- Documentation: decide whether user docs, reference docs, internal architecture notes, process docs, examples, or release notes need updates.
- Rollout and compatibility: decide whether existing factories, saved dashboard layout, persisted events, replay files, CLI scripts, or old API clients need compatibility handling.

## Story Checklist

Each story or implementation lane should state:

- The behavior it delivers or the enabling reason it exists.
- The affected surfaces from the required planning pass.
- The acceptance criteria that prove the behavior.
- The test or verification evidence expected before closeout.
- The generated artifacts, docs, or examples that must be updated.
- The important non-goals so implementation does not expand into unrelated cleanup.

## Common Surface Bundles

API-backed dashboard feature:

- API contract
- Event stream or dashboard projection
- Website state and component behavior
- CLI support if operators need the same action outside the browser
- Contract tests, API tests, UI tests, and responsive/manual QA

Runtime execution feature:

- Backend runtime
- Config and authoring
- Event stream
- API or CLI controls
- Observability
- Failure modes, restart behavior, and concurrency evidence

CLI control feature:

- API contract or service method
- CLI command UX and JSON output
- Error mapping and idempotency behavior
- Functional or package-level CLI tests
- Docs examples

Public contract feature:

- OpenAPI source
- Generated Go and TypeScript artifacts
- Backend handlers and service boundaries
- Frontend typed API wrappers
- Contract drift tests and compatibility notes

## Closeout Questions

- Did the plan explain why any plausible surface is not needed?
- Did the plan include the smallest set of vertically sliced stories that still proves the behavior?
- Did the plan name the first contract or runtime decision that downstream work depends on?
- Did the plan include enough test evidence for the highest-risk surface?
- Did the plan avoid vague "update UI", "add API", or "write tests" tasks without observable acceptance criteria?
