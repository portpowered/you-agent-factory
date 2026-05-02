# Named Factory API Contract Data Model

This artifact records the public-contract decisions for the Agent Factory
named-factory sharing seam. It explains the canonical authored payload, how
the HTTP and PNG export or import boundaries reuse that payload, and which
documents own the durable contract rules.

## Change

- PRD, design, or issue: `prd.json` (`US-004`, branch
  `ralph/agent-factory-export-import-authoring-contract-roundtrip`)
- Owner: Codex branch
  `ralph/agent-factory-export-import-authoring-contract-roundtrip`
- Packages or subsystems: `libraries/agent-factory/api`,
  `libraries/agent-factory/pkg/api`, `libraries/agent-factory/pkg/service`,
  `libraries/agent-factory/ui`, `libraries/agent-factory/docs/development/`,
  and `docs/processes/agent-factory-development.md`
- Canonical contract and process docs updated by this change:
  `libraries/agent-factory/docs/development/development.md` links here for the
  package-local sharing boundary, and
  `docs/processes/agent-factory-development.md` keeps the reusable process
  rules that future stories should follow.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [x] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| named factory | public API resource | One customer-named factory definition paired with the canonical flattened `Factory` payload. | `libraries/agent-factory/api/openapi-main.yaml` `NamedFactory` schema | bundled as `#/components/schemas/NamedFactory`, guarded in `pkg/api/openapi_contract_test.go` |
| factory name | public identifier | Customer-facing identifier used to address one stored named factory, with `UNDEFINED` reserved for current-factory readback while the default root runtime is active. | `libraries/agent-factory/api/openapi-main.yaml` `FactoryName` schema | bundled as `#/components/schemas/FactoryName` |
| current factory | public runtime view | The active named factory resolved from the durable `.current-factory` pointer, or the active default root runtime when no pointer exists yet. | `libraries/agent-factory/api/openapi-main.yaml` route contract | handler and service regression coverage in `pkg/api/server_test.go` and `pkg/service/factory_test.go` |
| sharing payload | public authored contract | The exact authored `NamedFactory` payload that must survive export and import without dashboard-side reshaping. | `NamedFactory` schema plus the UI named-factory API wrapper | export/import roundtrip coverage in `ui/src/features/import/use-factory-import-activation.test.tsx` |
| PNG envelope | browser transport wrapper | The PNG metadata wrapper that adds `schemaVersion` to the canonical `NamedFactory` payload and nothing else. | `ui/src/features/export/factory-png-export.ts` and `ui/src/features/import/factory-png-import.ts` `PortOSFactoryPngEnvelope` | focused PNG export/import tests under `ui/src/features/export/` and `ui/src/features/import/` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `/factory` | HTTP `POST` route | Agent Factory REST API | UI import flows, backend clients | `TestOpenAPIContract_ContainsCoveredJSONOperations` and named-factory server tests |
| `/factory/~current` | HTTP `GET` route | Agent Factory REST API | UI reloads and backend readers | `TestOpenAPIContract_ContainsCoveredJSONOperations` and named-factory server tests |
| `schemaVersion` | PNG metadata string field | browser export envelope | browser import reader | PNG metadata tests in `factory-png-export.test.ts` and `factory-png-import.test.ts` |
| `INVALID_FACTORY_NAME` | machine-readable error code | named-factory validation response | clients branch on invalid-name failures without parsing prose | `ErrorResponse.code` enum plus response-example guard |
| `FACTORY_ALREADY_EXISTS` | machine-readable error code | named-factory conflict response | clients branch on duplicate-name failures | `ErrorResponse.code` enum plus response-example guard |
| `INVALID_FACTORY` | machine-readable error code | named-factory validation response | clients distinguish invalid payloads from name conflicts | `ErrorResponse.code` enum plus response-example guard |
| `FACTORY_NOT_IDLE` | machine-readable error code | named-factory conflict response | clients distinguish runtime-state conflicts from validation failures | `ErrorResponse.code` enum plus response-example guard |

## Configuration Shapes

| Shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| `NamedFactory` | `libraries/agent-factory/api/openapi-main.yaml` | `name`, `factory` | none | bundled contract reviewers, generated Go/UI artifacts, handler boundary | `pkg/api/openapi_contract_test.go` named-factory schema guard |
| `FactoryName` | `libraries/agent-factory/api/openapi-main.yaml` | canonical lowercase-hyphen customer names plus reserved `UNDEFINED` current-factory readback | none | generated Go/UI artifacts, named-factory handler validation, current-factory readers | `pkg/api/openapi_contract_test.go` factory-name schema guard |
| `Factory` | existing public config schema | canonical flattened factory payload | existing optional-field defaults | nested inside `NamedFactory.factory` | `NamedFactory.factory -> #/components/schemas/Factory` |
| `PortOSFactoryPngEnvelope` | `libraries/agent-factory/ui` sharing boundary | `schemaVersion`, `name`, `factory` | `schemaVersion` fixed to the current PNG format version | browser export, browser import, dashboard activation preview | `factory-png-export.test.ts`, `factory-png-import.test.ts` |
| `ErrorResponse` with named-factory codes | shared API error schema | `message`, `code` | none | all JSON clients | `ErrorResponse.code` enum plus route-specific response examples |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| authored named-factory contract | `api/openapi-main.yaml` | bundled `api/openapi.yaml` | authored source -> bundled artifact | missing route refs, schema refs, or example wiring drops the published contract | `make generate-api`, bundled contract guards |
| service-owned API runtime seam | `pkg/service.FactoryService` | `pkg/api.Server` | service -> API boundary | API reads pinned to startup runtime after activation | `pkg/api/server_test.go` activated-runtime submission regression |
| current-factory export seam | `pkg/api.Server` `GET /factory/~current` | `ui/src/api/named-factory/api.ts` and export dialog hooks | API -> generated UI types -> dashboard export flow | event-history reconstruction or private DTOs drift from authored config | `current-factory-export.test.ts` and `App.test.tsx` |
| import activation seam | `ui/src/features/import/factory-png-import.ts` | `ui/src/features/import/use-factory-import-activation.ts` then `POST /factory` | PNG reader -> dashboard hook -> API | dashboard-only field renames or reshaping diverge from API contract | `use-factory-import-activation.test.tsx` |

## Canonical Sharing Boundary

The canonical sharing payload is the generated OpenAPI `NamedFactory` schema:

```json
{
  "name": "Factory Name",
  "factory": {
    "...": "canonical authored Factory payload"
  }
}
```

Every public sharing boundary reuses that payload:

1. `GET /factory/~current` returns the authored `NamedFactory` payload for the
   current factory. When the active runtime is still the default root runtime
   and no durable pointer exists yet, the response uses `name: "UNDEFINED"` as
   the reserved canonical identifier.
2. Browser PNG export embeds that same payload inside
   `PortOSFactoryPngEnvelope`, which adds only `schemaVersion`.
3. Browser PNG import reads `PortOSFactoryPngEnvelope`, validates
   `schemaVersion`, normalizes the legacy `v1` `factoryName` field to the
   canonical `name` field when needed, and passes the exact embedded
   `NamedFactory` to `POST /factory`.

The dashboard must not rebuild sharing payloads from `/events`, runtime-only
views, or export-specific field aliases such as `factoryName`. The public
contract boundary stays authoring-first: runtime projections may support
inspection, but they do not define the export or import payload.

`UNDEFINED` is reserved for current-factory readback only. `POST /factory`
continues to require a customer-safe named-factory identifier and should reject
attempts to activate a named factory with the reserved default-runtime name.

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  `NamedFactory` is the one public request and response shape for
  `POST /factory`, `GET /factory/~current`, and the PNG sharing payload inside
  `PortOSFactoryPngEnvelope`.
- Shared runtime seam selected: `pkg/apisurface.APISurface` is the API server's owner
  for current runtime reads and named-factory activation so `/work`, `/status`,
  `/events`, and `/factory/~current` all observe the same swapped runtime
  pointer.
- Reason: the persistence and activation stories already made `FactoryService`
  the owner of the durable current-factory pointer and runtime swap. The
  export/import contract stories extend that same seam so the browser reuses
  authored API payloads instead of event-derived runtime projections.
- Review evidence: `libraries/agent-factory/api/openapi-main.yaml`,
  `libraries/agent-factory/pkg/api/server.go`,
  `libraries/agent-factory/pkg/api/server_test.go`, and
  `libraries/agent-factory/pkg/service/factory.go`, plus
  `libraries/agent-factory/ui/src/features/export/factory-png-export.ts`,
  `libraries/agent-factory/ui/src/features/import/factory-png-import.ts`, and
  `libraries/agent-factory/ui/src/features/import/use-factory-import-activation.test.tsx`.
