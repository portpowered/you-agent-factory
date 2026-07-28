# Workers root `.go` contract-surface inventory (`pkg/services/workers`)

Owner-local live inventory for **INV-WRK-TOPLEVEL** (`pss-inv-wrk-toplevel`). This
packet records evidence-backed classification only; it does **not** move, fold,
or delete any root-level `.go` files.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/workers/*.go` (non-recursive root files only).

**Baseline before CLN-WRK-CONTRACT-ROOTS moves:** **41** root-level `.go` files
(34 inventoried at INV-WRK-TOPLEVEL capture plus 7 post-inventory additions).

Companion directory inventory:
[`workers-top-level-inventory.md`](workers-top-level-inventory.md).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **keep** | Thin committed root contract: intentional `package workers` surface that cross-service peers may import directly (interfaces, request/result vocabulary, safe diagnostics types, publication-boundary shapes, and root-contract characterization or import-boundary tests). Expected to remain at the service root after CLN-WRK-CONTRACT-ROOTS, possibly slimmed. |
| **move-to-runners** | Root-level runner policy, registry, or runner-selection implementation and co-located tests that belong under `workers/internal/services/runners`. |
| **move-to-workstations** | Root-level workstation pool, diagnostics projection, inference-failure, model-invocation, prompting, draft-validation, token-lineage, or integration-test glue that belongs under `workers/internal/services/workstations`. |
| **move-to-runtime_assembly** | Root-level runtime-assembly glue that belongs under `workers/internal/services/runtime_assembly`. *(No live root files use this classification at baseline.)* |
| **move-to-workers/internal** | Root-level diagnostics codecs, mock-worker config loaders, executor integration helpers, and other non-subservice helpers (including delete-after-fold transitional surfaces) that belong under `workers/internal`. |
| **temporary documented keep** | Explicitly approved temporary root exception with documented rationale. *(No live root files use this classification at baseline.)* |

Private subservices already present under `workers/internal/services/*` are
**not** proof that matching root `.go` surfaces are migrated. This inventory
treats live root files as debt until CLN-WRK-CONTRACT-ROOTS cutover packets
fold or delete them.

## Root-level `.go` file inventory

| File | Classification | Later target / rationale |
| --- | --- | --- |
| `command.go` | keep | Subprocess execution port (`CommandRunner`, `CommandRequest`, `CommandResult`). |
| `env_diagnostics.go` | move-to-workstations | Command-environment classification and safe diagnostic projection helpers. |
| `env_diagnostics_test.go` | move-to-workstations | Co-located tests for `env_diagnostics.go`; move with diagnostics projection logic. |
| `execution_context.go` | keep | Worker execution environment (`Context`, project/session defaults). |
| `execution_contracts.go` | keep | Canonical inference/script/model/agent event and request vocabulary at the Workers root boundary. |
| `execution_requests.go` | keep | Runner capability and workstation execution request vocabulary published for selection and dispatch planning. |
| `execution_tokens.go` | keep | Worker-facing dispatch token/color view shared across execution paths. |
| `executor_test_helpers_test.go` | move-to-workers/internal | Root-level executor integration helpers; delete once `executor/` folds under `workstations`. |
| `failure.go` | keep | Normalized provider failure type (`ProviderError`) at the public boundary. |
| `inference_failure.go` | move-to-workstations | Inference failure classification and customer-safe messaging logic. |
| `inference_failure_test.go` | move-to-workstations | Co-located tests for `inference_failure.go`. |
| `interfaces.go` | keep | Primary Workers root contracts (`Service`, hosted poller ports, provider identity, docs loader, and related peer-facing interfaces). |
| `invocation_executor_test.go` | move-to-workstations | Integration tests that reach `invocation/` and `executor/` through the root package; relocate with workstation fold. |
| `legacy_fold_boundary_test.go` | keep | Post-inventory characterization test locking transitional public sibling retention and published-surface import boundaries after legacy package fold. |
| `mock_workers.go` | move-to-workers/internal | Legacy JSON mock-worker config loader; delete after `interface/` fold replaces it with OpenAPI-generated schema. |
| `mock_workers_config_test.go` | move-to-workers/internal | Co-located tests for `mock_workers.go`; remove with mock-worker schema fold. |
| `model_invocation.go` | move-to-workstations | Workstation pool lifecycle errors, requests/results, and `ModelInvoker` wiring belong with workstation execution, not a thin root contract. |
| `opencode_agent_contract_test.go` | move-to-runners | Runner-selection contract tests for OpenCode agent policy; move with `runner_policy.go`. |
| `progress_observations.go` | keep | Provider-neutral progress fragments accepted by Factory Session response streams. |
| `prompt_templates.go` | move-to-workstations | Prompting template contract/diagnostic types; fold with `prompting/` debt. |
| `provider_port.go` | keep | `Provider` inference port explicitly documented for cross-service consumers (for example Recordings replay). |
| `provider_port_test.go` | keep | Root-contract characterization test for `provider_port.go`. |
| `response_drafts.go` | keep | Provider response draft shapes and publication-boundary helpers. |
| `response_draft_validation.go` | move-to-workstations | Draft validation logic ahead of publication; fold with inference/publication path. |
| `runner_policy.go` | move-to-runners | Built-in runner metadata and `ResolveRunnerSelection` policy implementation. |
| `runner_registry.go` | move-to-runners | Built-in runner prerequisite validation and availability reporting. |
| `runner_registry_test.go` | move-to-runners | Co-located tests for `runner_registry.go`. |
| `runtime_service.go` | keep | `RuntimeService` opening contract for Factory Runtime assembly. |
| `safe_diagnostics.go` | keep | Canonical safe diagnostics types for history, replay, and projections. |
| `safe_diagnostics_codec.go` | move-to-workers/internal | Projection/codec logic between worker and safe diagnostics shapes. |
| `service_import_boundary_test.go` | keep | Post-inventory import-boundary proof sealing production packages against transitional `workers/service` imports. |
| `sessions_consumer_boundary_test.go` | keep | Post-inventory boundary test proving Factory Sessions can name Workers root contracts without importing internal packages. |
| `sessions_consumer_contracts.go` | keep | Sessions-facing `PTYAllocator`/`ProviderRegistry` type aliases and interfaces so peers name the Workers root instead of `agypty` or `provider/registry`. |
| `template_fields.go` | keep | `TemplateFieldResolver` contract without exposing prompting implementation packages. |
| `template_fields_test.go` | move-to-workstations | Integration tests that assemble prompting/executor paths; relocate with workstation fold. |
| `token_lineage.go` | move-to-workstations | Chaining-trace derivation helpers over execution tokens. |
| `worker_vocabulary_boundary_test.go` | keep | Post-inventory boundary test proving peer consumers import Workers root vocabulary instead of Factory Definitions contracts. |
| `worker_vocabulary_contract.go` | keep | Package-level documentation anchor for Workers-owned peer execution and diagnostics vocabulary. |
| `workstation_pool_boundary.go` | move-to-workstations | Pool boundary implementation; retain only slim runtime-facing interfaces at root during later fold. |
| `workstation_pool_boundary_test.go` | move-to-workstations | Co-located tests for `workstation_pool_boundary.go`. |
| `workstation_result_contract_test.go` | keep | Root-contract characterization test for `WorkstationResult` round-trip through published Workers root types. |

## Keep vs move summary (baseline before moves)

| Classification | Count | Target after CLN-WRK-CONTRACT-ROOTS |
| --- | ---: | --- |
| **keep** | 21 | Remain at `pkg/services/workers/` as thin root contracts |
| **move-to-runners** | 4 | `pkg/services/workers/internal/services/runners` |
| **move-to-workstations** | 12 | `pkg/services/workers/internal/services/workstations` |
| **move-to-runtime_assembly** | 0 | — |
| **move-to-workers/internal** | 4 | `pkg/services/workers/internal` (3 delete-after-fold) |
| **temporary documented keep** | 0 | — |
| **Total** | **41** | 21 keep + 20 move |

Keep-set files are limited to interfaces, request/result/value types, documented
errors/constants, approved pure contract helpers, and root-contract or
import-boundary characterization tests. None of the 21 keep files host concrete
service state, IO, registries, or workflow implementations.

## Post-inventory additions (since INV-WRK-TOPLEVEL capture)

Seven root files were added after the original 34-file capture. All are classified
above with the same legend:

| File | Classification | Rationale |
| --- | --- | --- |
| `sessions_consumer_contracts.go` | keep | Sessions-facing root type aliases and `ProviderRegistry` interface. |
| `sessions_consumer_boundary_test.go` | keep | Compile-time proof Sessions factories can name Workers root contracts. |
| `worker_vocabulary_contract.go` | keep | Documentation anchor for Workers-owned peer vocabulary. |
| `worker_vocabulary_boundary_test.go` | keep | Import-boundary proof for peer vocabulary consumers. |
| `workstation_result_contract_test.go` | keep | Behavioral characterization of published `WorkstationResult` types. |
| `service_import_boundary_test.go` | keep | Production import seal against transitional `workers/service` shim. |
| `legacy_fold_boundary_test.go` | keep | Legacy public-package fold disposition and import-boundary lock. |

## Generator mirror

Committed generator tables mirror this inventory:

- `internal/ownershipinventory/workers_root_contract.go` — `WorkersThinRootContractFiles`,
  `WorkersRootContractMoveTargets`, `ClassifyWorkersRootContractFile`, and
  `WorkersRootContractInventory`.
- `cmd/packagetargetmanifestcheck/workers_root_contract.go` — package-target manifest
  mirror for the same thin retain vs root-move partition.

## Out of scope for this note

- Nested packages under immediate child directories (classified in
  [`workers-top-level-inventory.md`](workers-top-level-inventory.md)).
- `packagetargetmanifestcheck` / `ownershipinventory` remap rows and JSON baseline
  regeneration beyond the root-contract mirror tables above.
- Production package moves, folds, deletes, or `pkg/wire` edits.
