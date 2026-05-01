# Consolidate handwritten public boundary structs with generated OpenAPI models

## Why

This branch exposed a repeatable drift path: `make generate-api` correctly updated
`pkg/api/generated/server.gen.go` and `ui/src/api/generated/openapi.ts`, but
watched-file batch ingestion and generated worker-output parsing still relied on
handwritten JSON-tagged structs in `pkg/interfaces/factory_runtime.go`.

That duplication let the public API switch to camelCase while file-based
ingestion and worker-generated batch payloads still expected snake_case, which
caused runtime failures outside the HTTP handler tests.

## Proposed direction

- Inventory every handwritten struct that still serializes or deserializes a
  public OpenAPI-owned payload outside `pkg/api/generated`.
- Decide whether each surface should:
  - consume the generated model directly, or
  - stay handwritten but gain a contract guard that proves its JSON tags match
    the generated/public schema.
- Start with the factory request batch and generated submission batch payloads,
  because they are consumed by the HTTP API, watched-file preseed path, and
  worker-generated follow-up batch path.

## Expected benefit

- One public field-name migration should not require rediscovering drift in
  HTTP handlers, watched-file ingestion, replay fixtures, and worker-generated
  request parsing separately.
- Future OpenAPI cleanup work can rely on explicit guardrails instead of
  implicit tag synchronization across multiple packages.
