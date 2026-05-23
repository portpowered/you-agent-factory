# Cleanup Idea: Deduplicate `/work` Content Validation

## Why this cleanup exists

The API currently validates work-content parts more than once on the same
public `/work` boundary.

Today:

- `pkg/api/handlers.go` runs raw JSON `content` validation in the decode path
  through `validateWorkContentField(...)` and `validatedRawWorkContentPart(...)`.
- `SubmitWork` then validates the same generated union again through
  `validateGeneratedWorkContentAtPath(...)`.
- `generatedWorkRequestToDomain(...)` repeats that same generated-union
  validation for `PUT /work/{request_id}` before converting the request into
  the domain work-request shape.

That leaves one boundary rule with overlapping owners in the same file. The
cleanup should remove the duplicate pass, not redesign the request model.

## Requested change

Make the API decode helpers the single owner of public `content` part
validation for `/work` requests and remove the redundant second-pass helper.

Keep this cleanup narrow:

- preserve current request and response behavior
- preserve current bad-request messages for invalid `content` shapes
- do not broaden this into content translation ownership or work-request schema
  redesign
- do not add a new validation framework or another forwarding helper
- prefer deleting duplicate validation helpers over wrapping them

Suggested shape:

- Keep `decodeSubmitWorkRequestBody(...)` and `decodeWorkRequestBody(...)`
  responsible for validating raw `content` payloads before handler logic runs.
- Reuse that one validation seam for both single-submit and batch-upsert paths.
- Remove `validateGeneratedWorkContentAtPath(...)` once the decode path fully
  owns the same observable validation outcomes.
- Keep domain conversion focused on translation, not on re-validating content
  parts that the boundary already accepted.

## Relevant files

- `pkg/api/handlers.go`
- `pkg/api/server_test.go`
- `pkg/api/openapi_contract_test.go`

## Acceptance criteria

- Public `/work` content-part validation no longer runs through overlapping
  decode-time and generated-union helper passes.
- `SubmitWork` and `UpsertWorkRequest` still reject malformed `content`
  payloads with the same user-visible bad-request messages.
- `generatedWorkRequestToDomain(...)` no longer owns duplicate validation that
  the decode boundary already performed.
- Coverage remains behavioral around API request outcomes rather than
  helper-topology assertions.
- Focused verification should include the API package tests for this surface,
  for example `go test ./pkg/api`.

## Review guidance

Review this as a duplication-removal cleanup on one public API boundary. The
main check is that invalid `content` payloads still fail the same way while the
second validation pass disappears.
