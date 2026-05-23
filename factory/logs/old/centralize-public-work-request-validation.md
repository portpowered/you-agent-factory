# Cleanup Idea: Centralize Public Work-Request Validation

## Why this cleanup exists

The repository has two public work-ingest paths that converge into the same
normalized domain request shape, but they still keep overlapping boundary
validation logic in separate places.

Today:

- `pkg/api/handlers.go` decodes API request bodies and then manually rescans raw
  JSON to reject retired aliases and conflicting chaining-trace fields.
- `pkg/factory/work_request_json.go` performs similar validation for checked-in
  and file-backed work-request JSON before normalization.
- Both paths eventually converge into the same canonical factory-owned
  work-request normalization flow.

This is exactly the kind of duplication the maintainer workflow should remove:
parallel legacy-handling and validation structure around one public boundary.

## Requested change

Collapse the duplicated public work-request validation into one canonical
factory-owned validator/parser and reuse it from the API boundary.

Keep this cleanup narrow:

- do not change customer-visible work-request behavior
- do not introduce a second abstraction layer or generic validation framework
- do not broaden this into unrelated request-shape cleanup
- do not change the normalized domain request model
- do not replace behavioral tests with source-layout or helper-location tests

Suggested shape:

- Move retired alias rejection and conflicting trace-field validation behind one
  factory-owned helper that validates canonical batch JSON.
- Reuse that helper from the API batch decoding path instead of keeping a second
  raw-field scan in `pkg/api/handlers.go`.
- Reuse the same field-level validation for the single-item submit path where it
  currently repeats the same retired-field and trace-conflict checks.
- Preserve current behavior:
  - reject retired aliases such as `work_type_id`
  - keep the current `traceId` and `currentChainingTraceId` normalization
    behavior
  - preserve current content and payload conflict validation
  - preserve current API error semantics as closely as practical

## Relevant files

- `pkg/api/handlers.go`
- `pkg/factory/work_request_json.go`
- `pkg/factory/work_request.go`
- `pkg/factory/work_request_trace.go`
- `pkg/api/server_test.go`
- `pkg/factory/work_request_test.go`

## Acceptance criteria

- The retired alias rejection and conflicting trace-field validation no longer
  live in two separate public boundary implementations.
- API submission and file-backed work-request parsing both flow through one
  canonical validation owner before normalization.
- Observable behavior stays the same for supported request shapes, including
  trace normalization and content or payload validation.
- Existing negative coverage for retired aliases and conflicting trace fields
  remains behavioral and still proves the public surfaces reject the same bad
  inputs.
- Run the focused backend tests for this surface, for example:
  `go test ./pkg/api ./pkg/factory`, or the repository's current equivalent.

## Review guidance

Review this change by confirming the duplicated validation paths disappeared and
that the same public bad-input cases are still rejected through API and
file-backed work-request entrypoints. The regression risk is accidental drift in
error behavior or trace normalization while collapsing the overlapping logic.
