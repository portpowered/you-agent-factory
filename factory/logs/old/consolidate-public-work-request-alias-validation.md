# Cleanup Idea: Consolidate Public Work-Request Alias Validation

## Why this cleanup exists

The repository still keeps the same public work-request alias and trace-field
validation in two active production boundaries.

Today:

- `pkg/factory/work_request_json.go` validates canonical batch JSON for retired
  public aliases such as `work_type_id`, retired `target_state`, and
  conflicting `currentChainingTraceId` versus `traceId` aliases before
  normalization.
- `pkg/api/handlers.go` separately re-reads raw request JSON and applies the
  same alias and trace-conflict policy for `POST /work` and batch-shaped
  request handling through local helpers.
- Both paths are guarding the same public workflow contract, but ownership is
  still split between API-local raw-field scans and factory-owned canonical
  work-request parsing.

This is still live duplication after the earlier public work-request cleanup.
The repository now has one canonical factory-owned parser, but the API
boundary still carries a second copy of the same retired-alias and
trace-conflict behavior.

## Requested change

Collapse retired public work-request alias validation and trace-alias conflict
validation onto one canonical factory-owned raw-JSON helper path and reuse it
from the API boundary.

Keep this cleanup narrow:

- preserve supported public submit behavior
- do not broaden this into work-request model redesign
- do not add a generic validation framework
- keep API-only checks such as content-part validation and submit-name handling
  local to the API boundary
- prefer deleting API-local duplicate alias helpers instead of wrapping them

Suggested shape:

- Add one canonical factory-owned validator in
  `pkg/factory/work_request_json.go` for:
  - retired aliases `work_type_id` and `target_state`
  - conflicting `currentChainingTraceId`, `traceId`,
    `current_chaining_trace_id`, and `trace_id` combinations
- Reuse that validator from `ParseCanonicalWorkRequestJSON(...)`.
- Reuse the same validator from the API decode paths in
  `pkg/api/handlers.go` instead of keeping separate
  `rejectPublicBatchWorkAliases(...)` and
  `rejectConflictingChainingTraceFields(...)` ownership there.
- Preserve current API-facing error wording as closely as practical, including
  indexed prefixes such as `works[0].`.

## Relevant files

- `pkg/api/handlers.go`
- `pkg/factory/work_request_json.go`
- `pkg/factory/work_request_trace.go`
- `pkg/api/server_test.go`
- `pkg/factory/work_request_test.go`
- `pkg/cli/run/run.go`
- `pkg/listeners/filewatcher.go`
- `pkg/service/factory.go`

## Acceptance criteria

- Retired public work-request alias rejection and trace-alias conflict
  validation no longer live in separate API-local and factory-local raw-JSON
  implementations.
- API submit handling and non-HTTP canonical work-request parsing both reuse
  one factory-owned validator for `work_type_id`, `target_state`, and
  conflicting trace-field aliases.
- Observable behavior stays the same for supported request shapes, including
  API bad-request semantics and canonical work-request parsing behavior.
- Behavioral regression coverage still proves:
  - unary submit rejects `work_type_id`
  - batch submit rejects `works[n].work_type_id`
  - batch submit rejects `target_state`
  - conflicting trace aliases are rejected consistently
- Focused backend verification remains behavioral, for example:
  `go test ./pkg/api ./pkg/factory ./pkg/cli/run ./pkg/listeners ./pkg/service`

## Review guidance

Review this as a redundant-legacy-handling cleanup. The main thing to verify is
that the API boundary no longer owns a second copy of the retired-alias and
trace-conflict policy, while all public submit entrypoints still reject the
same unsupported field combinations.
