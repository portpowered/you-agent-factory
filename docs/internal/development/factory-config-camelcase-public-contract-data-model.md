# Factory Config CamelCase Public Contract Data Model

This artifact records the current public Agent Factory factory-config naming
surface before the camelCase contract cleanup. It exists to make the migration
target explicit across the OpenAPI-owned schema, generated models, canonical
serialization, docs/examples, and compatibility tests.

## Change

- PRD, design, or issue: `prd.json` for Factory Config CamelCase Public Contract
- Owner: Agent Factory maintainers
- Packages or subsystems: `api/openapi.yaml`, `pkg/api/generated`, `pkg/config`,
  `pkg/replay`, `docs`, `examples`, `tests/functional_test`
- Canonical package responsibility artifact: `api/openapi.yaml` owns the public
  factory-config contract; `pkg/config` owns boundary normalization and
  canonical config serialization; `pkg/replay` owns generated Factory
  serialization and hydration.
- Canonical package interaction artifact: generated `pkg/api/generated.Factory`
  is the shared contract used by API, replay, and dashboard consumers; runtime
  code translates at explicit mapper boundaries.

## Trigger Check

- [x] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API or generated schema
- [x] Package-local model another package must interpret

## Boundary Rules

- `env` and `metadata` are free-form maps. Their property names are part of the
  public contract, but their nested keys are caller-owned and are intentionally
  excluded from naming cleanup inventory.
- This audit covers the public factory-config contract rooted at generated
  `Factory`, not unrelated work-submit, status, or event-context fields that
  live outside the factory-config schema family.
- Canonical config serialization already emits camelCase through
  `config.MarshalCanonicalFactoryConfig`; this inventory tracks the remaining
  snake_case fields still advertised in OpenAPI and generated models.

## Current Inventory

| Field family | Current public names | Canonical camelCase target | Affected surfaces |
| --- | --- | --- | --- |
| Factory top-level config | `factory_dir`, `source_directory`, `workflow_id`, `input_types`, `work_types`, `exhaustion_rules` | `factoryDir`, `sourceDirectory`, `workflowId`, `inputTypes`, `workTypes`, `exhaustionRules` | OpenAPI `Factory`, generated `factoryapi.Factory`, run-started/replay config payloads, docs/examples |
| Worker config | `model_provider`, `session_id`, `stop_token`, `skip_permissions` | `modelProvider`, `sessionId`, `stopToken`, `skipPermissions` | OpenAPI `Worker`, generated `factoryapi.Worker`, inline worker config examples, compatibility loader |
| Workstation runtime/topology config | `prompt_file`, `prompt_template`, `output_schema`, `on_rejection`, `on_failure`, `resource_usage`, `stop_words`, `working_directory` | `promptFile`, `promptTemplate`, `outputSchema`, `onRejection`, `onFailure`, `resources`, `stopWords`, `workingDirectory` | OpenAPI `Workstation`, generated `factoryapi.Workstation`, canonical config output, docs/examples, replay serialization |
| Workstation cron config | `trigger_at_start`, `expiry_window` | `triggerAtStart`, `expiryWindow` | OpenAPI `WorkstationCron`, generated `factoryapi.WorkstationCron`, docs/examples, compatibility loader |
| Workstation limits | `max_retries`, `max_execution_time` | `maxRetries`, `maxExecutionTime` | OpenAPI `WorkstationLimits`, generated `factoryapi.WorkstationLimits`, AGENTS/frontmatter mapping docs |
| Per-input and workstation guards | `work_type`, `parent_input`, `spawned_by`, `max_visits` | `workType`, `parentInput`, `spawnedBy`, `maxVisits` | OpenAPI `WorkstationIO`, `InputGuard`, `WorkstationGuard`; generated models; fan-in docs/examples; compatibility loader |
| Exhaustion rules | `watch_workstation`, `max_visits` | `watchWorkstation`, `maxVisits` | OpenAPI `ExhaustionRule`, generated models, factory examples, compatibility loader |

## Active Customer-Facing Surfaces To Update In Later Stories

- `libraries/agent-factory/api/openapi.yaml`
- `libraries/agent-factory/pkg/api/generated/server.gen.go`
- `libraries/agent-factory/ui/src/api/generated/openapi.ts`
- `libraries/agent-factory/docs/work.md`
- `libraries/agent-factory/docs/authoring-agents-md.md`
- `libraries/agent-factory/examples/**/factory.json`

## Compatibility And Serialization Notes

- Boundary normalization already accepts camelCase and kebab-case aliases in
  `pkg/config/openapi_factory.go` and `pkg/config/openapi_factory_test.go`.
- Canonical serialization already prefers camelCase in
  `pkg/config/factory_config_mapping_test.go` and
  `pkg/config/openapi_factory_test.go`.
- Later migration stories should remove snake_case from OpenAPI and generated
  models first, then narrow the boundary compatibility surface to explicit
  migration-only aliases.

## Verification

- Focused audit test:

```bash
cd libraries/agent-factory
go test ./pkg/api -run "TestFactoryConfigContract(Audit|Guard)" -count=1
```

- Mapper and canonical serialization checks:

```bash
cd libraries/agent-factory
go test ./pkg/config -run "TestFactoryConfigFromOpenAPIJSON|TestFactoryConfigMapper" -count=1
```
