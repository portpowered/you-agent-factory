# PRD: Submit Work Header-Only Defaults

---
author: Codex
last modified: 2026, may, 30
status: draft
---

## Context

### Customer Ask

When a dashboard user submits work from the Submit work widget with only the required header fields filled in (work type and request name) and leaves the submission text area empty, submission fails. Add a test that proves header/type-only submission is currently rejected, then fix the flow so submission succeeds with an empty text payload.

### Problem

The dashboard Submit work widget builds structured submit requests through `buildStructuredSubmitItems`, which omits blank text items and therefore sends `items: []` when the user provides only `name` and `workTypeName`. The widget already has a unit test expecting that payload shape, but the backend rejects it:

- `validateSubmitWorkItemsField` returns `items must contain at least one item` for an empty `items` array.
- `submitWorkItemsToContent` returns the same class of error when converting structured items to work content.

As a result, operators who only want to enqueue a named request of a given work type see a failed submission even though the OpenAPI contract only requires `name` and `workTypeName`.

### Solution

Treat an empty structured `items` array as a valid header-only submission. Keep rejecting submissions whose `items` array contains only blank text or otherwise non-meaningful entries. Add explicit API tests for the header-only case, update backend validation/conversion in `pkg/api/submit_work_items.go`, and add functional smoke coverage so the dashboard path and HTTP API agree.

## Project Acceptance Criteria

- [ ] A submit-work request with `name`, `workTypeName`, and `items: []` is accepted by the API and returns `201 Created` with a trace identifier.
- [ ] The accepted header-only submission reaches the factory with the provided name and work type and with empty structured content (no phantom text parts).
- [ ] Submissions whose `items` array contains only whitespace text items remain rejected with a clear validation error.
- [ ] The dashboard Submit work widget can complete a header-only submission without showing a server error after the backend fix.
- [ ] Regression tests cover the rejection (before fix) and acceptance (after fix) behavior at the API layer, plus at least one functional smoke path.
- [ ] Quality gate: backend and frontend typecheck, lint, and relevant tests pass before merge.

## Goals

- [ ] Allow operators to submit work using only work type and request name when they do not need inline text or staged files.
- [ ] Align backend validation with the dashboard’s existing empty-items payload shape.
- [ ] Preserve validation for malformed or meaningless structured item payloads.
- [ ] Add direct test evidence so header-only submission cannot regress to `400 Bad Request`.

## User Stories

### US-001: Prove header-only structured submit-work is rejected today

**Description:** As a maintainer, I want an API test that submits only `name`, `workTypeName`, and an empty `items` array so the current rejection is documented before we change validation.

**Acceptance Criteria:**
- [ ] `POST /work` (or session-scoped equivalent handler test) with body `{"name":"<non-empty>","workTypeName":"<configured type>","items":[]}` returns `400 Bad Request`.
- [ ] The error message identifies structured `items` validation (for example `items must contain at least one item`).
- [ ] The test name and assertions describe header/type-only submission without text or file payload content.
- [ ] Typecheck passes
- [ ] Tests pass

### US-002: Accept header-only structured submit-work on the API

**Description:** As a dashboard operator, I want to submit work with only a work type and request name so I can enqueue named requests without typing optional details.

**Acceptance Criteria:**
- [ ] `POST /work` with `name`, `workTypeName`, and `items: []` returns `201 Created` and a non-empty trace identifier.
- [ ] The factory receives the submission with the provided `name` and `workTypeName` and with empty structured content (no text or file content parts).
- [ ] `items` arrays that contain only whitespace text items still return `400 Bad Request` with `items must contain at least one non-empty item`.
- [ ] Prior rejection tests for empty `items: []` are updated to expect success instead of failure.
- [ ] A functional runtime API smoke test submits header-only structured work and observes successful acceptance.
- [ ] Typecheck passes
- [ ] Tests pass

### US-003: Confirm the dashboard submit-work widget completes header-only submission

**Description:** As a dashboard user, I want the Submit work widget to succeed when I fill in work type and request name but leave the default text item blank.

**Acceptance Criteria:**
- [ ] With work type and request name filled and the default text item left empty, clicking Submit work issues one `POST` whose JSON body includes `items: []`, the chosen `name`, and `workTypeName`.
- [ ] When the API responds `201`, the widget shows the existing success status message with the returned trace identifier.
- [ ] No inline server error is shown for the header-only happy path.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- FR-1: Structured submit-work requests with `items: []` are valid when `name` and `workTypeName` are non-empty.
- FR-2: Empty `items` arrays produce empty structured work content at submission time (no synthetic text parts).
- FR-3: Structured `items` arrays that include entries but no meaningful text or staged file content remain invalid.
- FR-4: The dashboard continues to omit blank text draft items from the outbound `items` array.
- FR-5: Whitespace-only text items inside a non-empty `items` array remain rejected.

## Non-Goals

- Changing OpenAPI required fields or adding new submit-work request properties.
- Requiring users to enter text in the default text item.
- Allowing submissions with missing or blank `name` or `workTypeName`.
- Reworking file staging, multimodal item ordering, or submit-work card layout.
- Broad refactors of submit-work helpers unrelated to empty payload handling.

## High-Level Technical Design

**Ownership:** `pkg/api` owns structured submit-work validation and content conversion (`submit_work_items.go`, `server_submit_work_test.go`). The dashboard widget already builds empty `items` in `ui/src/features/submit-work/hooks/use-submit-work-widget-helpers.ts`.

**Validation change:** When `items` is present as an empty array, skip the “at least one item” and “at least one non-empty item” checks. When `items` is non-empty, keep existing per-item validation and meaningful-item checks.

**Conversion change:** `submitWorkItemsToContent` returns an empty content slice for `len(items) == 0` instead of an error.

**UI surface:** No payload-shape change expected; verify the existing widget test and success/error status handling still match backend behavior.

**Verification:** Handler tests in `pkg/api/server_submit_work_test.go`, functional smoke in `tests/functional/runtime_api/api_generated_smoke_test.go`, and widget tests in `ui/src/features/submit-work/components/submit-work-widget.test.tsx`.

## Supporting Technical and UX Considerations

- Distinguish `items: null`/omitted (legacy `content` path) from `items: []` (structured header-only path); only relax validation for the explicit empty array used by the dashboard.
- Keep error messages specific so operators can still tell the difference between “no items provided when items key is non-empty but meaningless” versus “valid header-only submission”.
- Success UX should continue to use existing trace-ID success copy; no new empty-state messaging is required.

## Success Metrics

- Header-only submissions from the dashboard complete without `400` responses.
- API and functional tests prevent reintroducing empty-items rejection.
- No increase in invalid blank-only multimodal submissions reaching the factory.

## Open Questions

None. The failure mode, payload shape, and fix boundary are clear.
