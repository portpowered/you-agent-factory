# OpenAPI Event Schema Modularization Data Model

This artifact records the shared-contract decisions for modularizing the Agent
Factory public event OpenAPI authoring flow without changing the published
event contract. It explains which files are the authored source of truth, which
artifact remains the downstream public contract, and how generators plus
regression tests prove the bundled schema surface did not drift.

## Change

- PRD, design, or issue: `prd.json` (`US-001` through `US-006`, branch
  `ralph/agent-factory-openapi-event-schema-modularization`)
- Owner: Codex branch
  `ralph/agent-factory-openapi-event-schema-modularization`
- Packages or subsystems: `libraries/agent-factory/api`,
  `libraries/agent-factory/pkg/api`, generated Go/UI API outputs, and Agent
  Factory contributor docs
- Canonical architecture document to update before completion: this file is the
  branch data-model construction artifact. Durable workflow rules live in
  `docs/processes/agent-factory-development.md`.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [ ] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| authored event schema tree | authoring contract | The split-source OpenAPI tree contributors edit for public Agent Factory event contract changes. | `libraries/agent-factory/api/openapi-main.yaml` plus `libraries/agent-factory/api/components/schemas/events/` | `libraries/agent-factory/api/openapi-main.yaml`, `libraries/agent-factory/README.md`, `libraries/agent-factory/docs/development/development.md` |
| bundled published event contract | public contract artifact | The checked-in bundled OpenAPI artifact that downstream generation, tests, and external review consume. | `libraries/agent-factory/api/openapi.yaml` | `libraries/agent-factory/Makefile`, `libraries/agent-factory/pkg/api/openapi_contract_test.go` |
| FactoryEvent envelope | public event schema | The public event message schema exposed to customers, replay, and generated models. | `libraries/agent-factory/api/components/schemas/events/FactoryEvent.yaml`, bundled as `#/components/schemas/FactoryEvent` | `libraries/agent-factory/api/components/schemas/events/FactoryEvent.yaml`, bundled contract guard in `libraries/agent-factory/pkg/api/openapi_contract_test.go` |
| supported event payload fragment | authored schema fragment | One split-source payload schema file for a supported public event type. | `libraries/agent-factory/api/components/schemas/events/payloads/*.yaml` | payload refs in `libraries/agent-factory/api/components/schemas/events/FactoryEvent.yaml` and `libraries/agent-factory/api/openapi-main.yaml` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| authored fragment ref | relative OpenAPI `$ref` such as `./components/schemas/events/FactoryEvent.yaml` | `api/openapi-main.yaml` and split event schema files | Redocly bundle step | `make bundle-api`, `make api-smoke` |
| bundled component schema ref | bundled OpenAPI ref such as `#/components/schemas/FactoryEvent` | `libraries/agent-factory/api/openapi.yaml` | Go/UI generators, contract tests, runtime readers of generated models | `libraries/agent-factory/pkg/api/openapi_contract_test.go`, `make generate-api` |
| supported public event payload refs | bundled refs such as `#/components/schemas/RunRequestEventPayload` through `#/components/schemas/RunResponseEventPayload` | bundled `FactoryEvent.payload.oneOf` | downstream generated models and bundled completeness guard | `bundledFactoryEventPayloadRefs` in `libraries/agent-factory/pkg/api/openapi_contract_test.go` |

## Boundary Lifecycle

| Layer | Owner | Allowed transition | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| authored split-source OpenAPI tree | `libraries/agent-factory/api/openapi-main.yaml` and `api/components/schemas/events/` | contributor edits fragments and bundle entrypoint | No | split-source refs in `libraries/agent-factory/api/openapi-main.yaml` |
| bundled published OpenAPI artifact | `libraries/agent-factory/api/openapi.yaml` | `make bundle-api` rewrites the published artifact from the authored tree | Yes | `libraries/agent-factory/Makefile`, clean-diff checks in `make api-smoke` |
| generated Go/UI contract outputs | generated outputs from bundled `api/openapi.yaml` | `make generate-api` regenerates consumers from the bundled artifact | Yes | `make api-smoke`, generated outputs checked by the second regenerate pass |
| bundled contract regression guard | `libraries/agent-factory/pkg/api/openapi_contract_test.go` | focused tests fail when schema names, refs, or payload coverage drift | Yes | `TestOpenAPIContract_BundledFactoryEventSchemasRemainComplete` and related assertions |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| Agent Factory OpenAPI authoring layout | `libraries/agent-factory/api` | `openapi-main.yaml`, split `components/schemas/events/` fragments, bundled `openapi.yaml` | none | contributors, bundle step, API generators, contract tests | `libraries/agent-factory/README.md`, `libraries/agent-factory/docs/development/development.md`, `docs/processes/agent-factory-development.md` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| authored source tree to bundled artifact | Agent Factory package-local OpenAPI source files | Redocly bundle workflow and checked-in published artifact | authored source -> bundled artifact | missing or miswired `$ref`s can drop schemas from the published contract | `make bundle-api`, `make api-smoke`, `libraries/agent-factory/api/openapi-main.yaml` |
| bundled artifact to generated models | bundled `libraries/agent-factory/api/openapi.yaml` | generated Go models, generated UI types, and runtime code that imports them | bundled artifact -> generated consumers | generation can drift if the bundled artifact changes or becomes non-idempotent | `make generate-api`, second-pass clean-diff assertion in `make api-smoke` |
| bundled artifact to focused contract review | bundled `FactoryEvent` schema surface | `libraries/agent-factory/pkg/api/openapi_contract_test.go` and reviewers | bundled artifact -> focused regression tests | dropped schema names, missing payload refs, or incorrect envelope refs must fail review | bundled completeness assertions in `libraries/agent-factory/pkg/api/openapi_contract_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  the bundled `libraries/agent-factory/api/openapi.yaml` remains the canonical
  downstream public contract consumed by generators, tests, and review.
- Package-local model selected: the split-source authored tree under
  `libraries/agent-factory/api/openapi-main.yaml` and
  `libraries/agent-factory/api/components/schemas/events/` is package-local
  authoring structure, not a second public contract.
- Reason: the work is structural. Contributors need smaller authored files for
  reviewability and merge-conflict control, but downstream consumers still need
  one stable published OpenAPI artifact and unchanged public schema names.
- Translation boundary: `make bundle-api` materializes the authored tree into
  `libraries/agent-factory/api/openapi.yaml`; `make generate-api` then reads
  only the bundled artifact.
- Review evidence: `libraries/agent-factory/Makefile`,
  `libraries/agent-factory/api/openapi-main.yaml`,
  `libraries/agent-factory/api/openapi.yaml`,
  `libraries/agent-factory/pkg/api/openapi_contract_test.go`, and
  `make api-smoke`.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| inline monolithic public event schema block | previous event schema block inside `libraries/agent-factory/api/openapi.yaml` / `openapi-main.yaml` | Unify into split-source fragments under `api/components/schemas/events/` | `libraries/agent-factory/api` | none |
| authored source tree versus bundled published artifact | `libraries/agent-factory/api/openapi-main.yaml` and `libraries/agent-factory/api/openapi.yaml` | justify | `libraries/agent-factory/api` | keep both, but with distinct roles documented and guarded by bundle plus regression tests |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Relevant interaction patterns: API contract and event or message contract.
  The authored split-source tree stays package-local to Agent Factory; the
  bundled `api/openapi.yaml` stays the one public contract that downstream
  generators and tests consume.
- Canonical owner note: the Agent Factory package owns both the authored source
  tree and the bundled published artifact. The distinction is authoring role
  versus published-contract role, not a split in ownership.
- Public-contract drift note: this modularization does not intentionally change
  the supported event envelope, event type enum, event context, or payload
  union. `make api-smoke` plus the bundled completeness guard prove the split
  source still bundles to the same supported public event contract.
- Approved exceptions: none.
