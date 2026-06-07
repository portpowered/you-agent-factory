# PRD: Cursor Provider Session Naming and Loading

## Introduction

Provider sessions produced by the Cursor-backed runtime are not loading reliably through the backend provider-session APIs. Current investigation shows two related problems:

1. backend provider-session loading can fail even when a real Cursor session exists on disk
2. internal and emitted metadata can refer to the provider as `agent` while the intended non-CLI provider name is `cursor`

The desired behavior is:

- all backend-facing provider-session behavior, metadata, and persisted/event-facing references use `cursor`
- the CLI command name remains `agent`
- existing behavior continues to work for callers and current flows while the backend becomes consistent and debuggable

This PRD covers backend-only work to validate and fix provider-session loading behavior and to remove non-CLI `agent` naming from the codebase.

## Goals

- Make Cursor-backed provider sessions load reliably through backend provider-session APIs when valid session storage exists.
- Make `cursor` the canonical provider name across backend codepaths, emitted provider-session metadata, and backend-facing contracts.
- Preserve CLI command compatibility for `agent`.
- Preserve compatibility for existing callers that still rely on current behavior during the transition.
- Add enough regression coverage and observability to prove the behavior and prevent reintroduction.

## User Stories

### US-001: Load Cursor provider sessions from actual storage
**Description:** As an operator using a running factory, I want the backend to resolve valid Cursor provider sessions from local storage so that provider-session detail requests return real session data instead of `NOT_FOUND`.

**Acceptance Criteria:**
- [ ] A backend provider-session detail request for a valid Cursor session ID succeeds when the session exists in the configured or discovered Cursor storage root.
- [ ] The backend searches the intended Cursor storage location on supported hosts, including the current local Cursor session storage layout used by the factory environment.
- [ ] When resolution fails, the backend records a reviewable diagnostic that identifies the provider, lookup kind, requested ID, and searched root or roots.
- [ ] A focused regression test proves successful loading for a real or fixture-backed Cursor session store path.
- [ ] A focused regression test proves the failure path reports a clear `NOT_FOUND` outcome when the session does not exist.

### US-002: Emit canonical provider-session metadata as `cursor`
**Description:** As a backend consumer of provider-session metadata, I want Cursor-backed runs and event-derived metadata to identify the provider as `cursor` so that downstream loading and interpretation use one stable canonical name.

**Acceptance Criteria:**
- [ ] Backend-emitted provider-session metadata for Cursor-backed runs uses `cursor` as the provider value.
- [ ] Non-CLI backend codepaths do not emit or persist `agent` as the provider name for Cursor-backed provider sessions.
- [ ] A regression test proves a Cursor-backed run or inference path emits provider-session metadata with provider `cursor`.
- [ ] Event-stream or persisted metadata parsing continues to recognize existing relevant data needed to load valid sessions during the transition.

### US-003: Preserve compatibility while retiring non-CLI `agent` naming
**Description:** As a maintainer, I want the backend to keep current workflows working while removing non-CLI `agent` naming from the code so that we can stabilize behavior without breaking operators.

**Acceptance Criteria:**
- [ ] The CLI command surface continues to use `agent`.
- [ ] Backend provider-session behavior continues to work for current callers without requiring transport or UI changes in this phase.
- [ ] Any required compatibility aliasing or translation is implemented inside the backend boundary and covered by regression tests.
- [ ] Non-CLI backend references that identify the Cursor provider use `cursor` after the change, with any remaining `agent` references limited to CLI-command semantics or clearly justified compatibility code.
- [ ] A repository search confirms no unintended non-CLI backend provider naming references to `agent` remain in the targeted scope after the change.

### US-004: Prove session behavior through backend regression coverage
**Description:** As a reviewer, I want focused backend tests around provider-session lookup and naming so that the change is trusted based on observable behavior rather than code movement.

**Acceptance Criteria:**
- [ ] Regression coverage exercises provider-session detail lookup for a valid Cursor session ID.
- [ ] Regression coverage exercises the canonical provider-name path with `cursor`.
- [ ] Regression coverage exercises any supported compatibility path needed to keep current callers working.
- [ ] Tests are focused on backend observable behavior rather than only internal helper topology.
- [ ] Relevant backend tests pass for the touched packages.

## Functional Requirements

1. `FR-1`: The backend must treat `cursor` as the canonical provider name for Cursor-backed provider-session behavior and metadata outside the CLI command surface.
2. `FR-2`: The backend must continue supporting the CLI command name `agent` without requiring a CLI rename in this phase.
3. `FR-3`: Provider-session detail lookup must resolve valid Cursor session IDs from the effective Cursor storage root used by the environment.
4. `FR-4`: The backend must have a defined storage-root strategy for Cursor session lookup on supported hosts, including explicit configuration and any supported default discovery behavior.
5. `FR-5`: When provider-session lookup fails, the backend must return the existing not-found behavior and emit useful backend diagnostics describing the lookup attempt.
6. `FR-6`: Cursor-backed provider-session metadata emitted by runtime or inference flows must use `cursor` as the provider value.
7. `FR-7`: If compatibility aliasing is needed for existing callers or persisted data, the backend must handle it internally without requiring frontend or transport changes in this phase.
8. `FR-8`: Backend tests must prove both the successful lookup path and the not-found path for Cursor provider sessions.
9. `FR-9`: Backend tests must prove canonical naming behavior and any compatibility translation behavior.
10. `FR-10`: This change must not expand into unrelated session-enumeration redesign, UI renaming, or transport-surface restructuring in this phase.

## Non-Goals

- No CLI command rename from `agent` to `cursor`.
- No frontend or UI contract changes in this phase.
- No broader provider-session enumeration product redesign beyond what is required to validate and load sessions correctly through existing backend behavior.
- No unrelated refactor of session orchestration, service composition, or API transport architecture.
- No change to user-facing transport shapes unless a backend compatibility fix requires a tightly scoped adjustment already supported by existing consumers.

## Design Considerations

- The backend should present a consistent mental model: the provider is `cursor`, while the CLI command used to invoke that provider remains `agent`.
- Compatibility behavior should be quiet and internal. Operators should not need to understand internal aliasing to get correct provider-session loading.
- Diagnostic logging should help answer why a provider session was not found without requiring ad hoc process inspection.

## Technical Considerations

- Current investigation indicates a likely storage-root discovery gap on macOS where valid Cursor session storage exists on disk but is not searched by the running backend unless explicitly configured.
- Current investigation also indicates a naming split where provider-session metadata can be emitted as `agent`; this should be normalized to `cursor` in non-CLI backend paths.
- Implementation should prefer a single canonical provider constant or normalization path for backend provider-session behavior rather than repeating string translation logic in multiple components.
- Because this phase is backend-only, any compatibility handling for existing values should occur within backend parsing, lookup, or emission boundaries.
- Regression coverage should target the concrete provider-session detail and metadata-emission surfaces that exposed the bug.

## Success Metrics

- A valid Cursor session ID present in local session storage can be loaded through the backend provider-session detail API in regression coverage and local verification.
- Backend-emitted provider-session metadata for Cursor-backed runs consistently reports provider `cursor`.
- No non-CLI backend provider naming references to `agent` remain in the targeted scope after implementation, except narrowly justified compatibility paths.
- Investigation of future lookup failures requires reading backend logs rather than reconstructing runtime state manually.

## Open Questions

- Which exact Cursor storage roots must be treated as supported defaults across platforms in this phase, and which must require explicit configuration?
- Is compatibility aliasing needed only for incoming provider-session lookups, or also for historical persisted/event-derived metadata already recorded as `agent`?
- Should backend diagnostics for provider-session lookup be plain logs only, or should they also surface through existing debug/status endpoints if available?
