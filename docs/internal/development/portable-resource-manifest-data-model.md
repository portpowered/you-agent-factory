# Portable Resource Manifest Data Model

This artifact records the shared-model decisions for the Agent Factory portable
resource manifest contract. It keeps the public OpenAPI schema, generated
models, config mapping, validation, and customer-facing docs aligned on one
canonical portability surface.

## Change

- PRD, design, or issue: `prd.json` for
  `agent-factory-portable-resource-manifest`
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory API, config, replay, CLI, and docs reviewers
- Packages or subsystems: `libraries/agent-factory/api`,
  `pkg/api/generated`, `ui/src/api/generated`, `pkg/config`,
  `pkg/interfaces`, portability docs, and process guidance
- Canonical process document to update before completion:
  `docs/processes/agent-factory-development.md`

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| `supportingFiles` | public top-level config field | Optional portability-only manifest carried by the public `factory.json` contract. | `api/openapi.yaml` and generated artifacts derived from it | `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, `ui/src/api/generated/openapi.ts`, `pkg/config/factory_config_mapping_test.go` |
| `requiredTools` | public manifest collection | Declarative host-tool dependencies that validation can probe on `PATH` without install or embed behavior. | OpenAPI `FactoryResourceManifest` schema plus config validation boundary | `api/openapi.yaml`, `pkg/config/config_validator.go`, `pkg/config/runtime_config.go` |
| `bundledFiles` | public manifest collection | Portability-bundle members that carry inline file content plus canonical factory-relative target paths for supported scripts, docs, and root helpers. | OpenAPI `FactoryResourceManifest` schema plus config validation boundary | `api/openapi.yaml`, `pkg/config/config_validator.go`, `pkg/config/runtime_config.go`, `pkg/config/layout.go` |
| `factory/scripts/`, `factory/docs/`, and `Makefile` | documentation and validation convention | Canonical default portable roots for bundled script and documentation assets plus the supported root-helper allowlist for this slice. | Config validator and canonical customer docs | `pkg/config/config_validator.go`, `docs/reference/config.md`, `docs/work.md`, `docs/authoring-workflows.md` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `supportingFiles` | camelCase JSON/OpenAPI field name | config authors and generated public clients | generated Go models, generated UI models, config mapper, docs | `api/openapi.yaml`, `pkg/config/factory_config_mapping.go`, `pkg/config/factory_config_mapping_test.go` |
| `requiredTools[].command` | PATH lookup token string | config authors | config validator, runtime loader, preflight callers | `pkg/config/config_validator_test.go`, `pkg/config/runtime_config_test.go` |
| `bundledFiles[].targetPath` | forward-slash factory-relative path | config authors | config validator, runtime loader, portability-boundary preservation paths | `pkg/config/config_validator_test.go`, `pkg/config/runtime_config_test.go`, `pkg/cli/config/config_test.go` |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public `Factory.supportingFiles` schema | `api/openapi.yaml` and generated models | optional top-level `supportingFiles` with `requiredTools` and `bundledFiles` | omitted manifest preserves existing factory behavior | generated Go/UI models, config mapper, docs/examples | `api/openapi.yaml`, `pkg/config/factory_config_mapping_test.go`, `docs/reference/config.md`, `docs/work.md` |
| Public `requiredTools[]` entry | OpenAPI manifest schema plus config validator | `name`, `command`; optional `purpose`, `versionArgs` | validation-only; no install side effects | config validator, runtime loader, customer docs | `api/openapi.yaml`, `pkg/config/config_validator.go`, `docs/work.md` |
| Public `bundledFiles[]` entry | OpenAPI manifest schema plus config validator | `type`, `targetPath`, `content.encoding`, `content.inline` | flatten auto-collects the supported allowlist, `SCRIPT` targets stay under `factory/scripts/`, `DOC` under `factory/docs/`, `ROOT_HELPER` on the supported root-helper allowlist, `utf-8` content in v1 | config validator, runtime loader, flatten/expand preservation, customer docs | `api/openapi.yaml`, `pkg/config/config_validator.go`, `pkg/config/layout.go`, `docs/reference/config.md`, `docs/authoring-workflows.md` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| OpenAPI-owned portability contract | `api/openapi.yaml` | generated Go models, generated UI models, config mapper, docs, tests | OpenAPI source owns public field names and the generated/public contract; downstream code and docs must not invent aliases or parallel naming. | Renaming fields locally, documenting alias-only variants, or omitting generated-field coverage is a public-contract regression. | generated artifacts, mapper tests, docs, `pkg/api/factory_config_contract_audit_test.go` |
| Config-owned validation and load boundary | `pkg/config` | CLI config paths, runtime loader, portability callers, tests | Validators and load helpers own manifest semantics and failure paths; callers consume results instead of rechecking tool/path rules independently. Flatten collects the supported bundle allowlist, while expand/load materialize bundled files only after the full target set validates. | Missing PATH tools, blank commands, wrong bundle roots, unsupported root helpers, absolute paths, and dot-segment escapes fail with canonical `supportingFiles...` paths. | `pkg/config/config_validator.go`, `pkg/config/runtime_config.go`, focused tests |
| Customer-doc portability guidance | package docs | authors and reviewers | `docs/reference/config.md` and `docs/work.md` own the canonical manifest contract; README and workflow docs summarize and link back instead of restating a separate contract. | Duplicated or stale manifest wording across entrypoints is a docs-drift regression. | `docs/README.md`, `docs/reference/README.md`, `docs/reference/config.md`, `docs/work.md`, `docs/authoring-workflows.md` |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Interop structs or config models that intentionally differ from the canonical
  model: internal `interfaces.FactoryConfig` and related runtime config structs
  remain package-local and are translated only at explicit config boundaries.
- Approved exceptions: none in this slice.
