# Workers root `.go` contract-surface inventory (`pkg/services/workers`)

Owner-local live inventory for **INV-WRK-TOPLEVEL** (`pss-inv-wrk-toplevel`). This
packet records evidence-backed classification only; it does **not** move, fold,
or delete any root-level `.go` files.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/workers/*.go` (non-recursive root files only).

**Baseline before CLN-WRK-CONTRACT-ROOTS moves:** **41** root-level `.go` files
(34 inventoried at INV-WRK-TOPLEVEL capture plus 7 post-inventory additions).

**After runners fold (story 002):** **38** root-level `.go` files (22 keep + 16 move).

**After workstations fold (story 003):** **31** root-level `.go` files (26 keep + 4 move-to-workers/internal).

**After workers/internal fold (story 004):** **29** root-level `.go` files (29 keep; move targets complete).

**After import retarget (story 005):** **32** root-level `.go` files (30 keep + 1 temporary documented keep impl + 1 impl test).

**After cutover delivery proof (story 006):** **33** root-level `.go` files (31 keep + 1 temporary documented keep impl + 1 delivery seal test). Pre-cutover baseline remains **41**.

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
| `env_diagnostics.go` | *(folded)* | Moved to `internal/services/workstations/envdiagnostics` during story 003. |
| `env_diagnostics_test.go` | *(folded)* | Moved with `env_diagnostics.go` during story 003. |
| `execution_context.go` | keep | Worker execution environment (`Context`, project/session defaults). |
| `execution_contracts.go` | keep | Canonical inference/script/model/agent event and request vocabulary at the Workers root boundary. |
| `execution_requests.go` | keep | Runner capability and workstation execution request vocabulary published for selection and dispatch planning. |
| `execution_requests_test.go` | keep | Root-contract characterization tests for execution request clone helpers retained after runner-registry test split. |
| `execution_tokens.go` | keep | Worker-facing dispatch token/color view shared across execution paths. |
| `executor_test_helpers_test.go` | *(folded)* | Moved to `internal/testhelpers/workstation_executor.go` during story 004. |
| `failure.go` | keep | Normalized provider failure type (`ProviderError`) at the public boundary. |
| `inference_failure.go` | *(folded)* | Root forwards to `internal/services/workstations/inferencefailure` during story 003. |
| `inference_failure_test.go` | *(folded)* | Moved to internal inferencefailure during story 003. |
| `interfaces.go` | keep | Primary Workers root contracts (`Service`, hosted poller ports, provider identity, docs loader, and related peer-facing interfaces). |
| `invocation_executor_test.go` | *(folded)* | Moved to `internal/services/workstations/invocation` during story 003. |
| `legacy_fold_boundary_test.go` | keep | Post-inventory characterization test locking transitional public sibling retention and published-surface import boundaries after legacy package fold. |
| `mock_workers.go` | *(folded)* | Implementation moved to `internal/interface/mock_workers_config.go`; root `mock_workers_contracts.go` aliases during story 004. |
| `mock_workers_config_test.go` | *(folded)* | Moved to `internal/interface/mock_workers_config_test.go` during story 004. |
| `mock_workers_contracts.go` | keep | Thin root aliases for mock-worker config types and loader entrypoints. |
| `model_invocation.go` | *(folded)* | Workstation pool contracts split to `workstation_contracts.go`; implementation under workstations during story 003. |
| `opencode_agent_contract_test.go` | *(folded)* | Moved to `internal/services/runners/runner` with runner policy during story 002. |
| `progress_observations.go` | keep | Provider-neutral progress fragments accepted by Factory Session response streams. |
| `prompt_template_contracts.go` | keep | Prompt template contract types retained at root; implementation under `internal/services/workstations/prompting`. |
| `prompt_templates.go` | *(folded)* | Split to `prompt_template_contracts.go` + internal prompting during story 003. |
| `provider_port.go` | keep | `Provider` inference port explicitly documented for cross-service consumers (for example Recordings replay). |
| `provider_port_test.go` | keep | Root-contract characterization test for `provider_port.go`. |
| `response_drafts.go` | keep | Provider response draft shapes and publication-boundary helpers. |
| `response_draft_validation.go` | *(folded)* | Root `validate_draft.go` forwards to internal draftvalidation during story 003. |
| `runner_policy_contracts.go` | keep | Published runner policy, built-in metadata, and selection helpers at the thin root boundary. |
| `runner_policy.go` | *(folded)* | Moved to `internal/services/runners/runner/policy.go` during story 002. |
| `runner_registry.go` | *(folded)* | Moved to `internal/services/runners/runner/registry.go` during story 002. |
| `runner_registry_test.go` | *(folded)* | Runner-specific tests moved to `internal/services/runners/runner`; clone tests retained at root in `execution_requests_test.go`. |
| `runtime_service.go` | keep | `RuntimeService` opening contract for Factory Runtime assembly. |
| `safe_diagnostics.go` | keep | Canonical safe diagnostics types for history, replay, and projections. |
| `safe_diagnostics_codec.go` | *(folded)* | Codec moved to `internal/diagnostics/codec.go`; root `safe_diagnostics_forward.go` forwards during story 004. |
| `safe_diagnostics_forward.go` | keep | Thin root forwarders for safe diagnostics projection and event codec entrypoints. |
| `service_import_boundary_test.go` | keep | Post-inventory import-boundary proof sealing production packages against transitional `workers/service` imports and moved `workers/internal` helpers. |
| `service_root_contract_seal_test.go` | keep | Post-cutover delivery seal proving thin-root inventory closure, pre→post file-count reduction, and peer-shaped `Service` compile-time surface. |
| `sessions_consumer_boundary_test.go` | keep | Post-inventory boundary test proving Factory Sessions can name Workers root contracts without importing internal packages. |
| `sessions_consumer_contracts.go` | keep | Sessions-facing `PTYAllocator`/`ProviderRegistry` type aliases and interfaces so peers name the Workers root instead of `agypty` or `provider/registry`. |
| `template_fields.go` | keep | `TemplateFieldResolver` contract without exposing prompting implementation packages. |
| `template_fields_root_test.go` | keep | Root-contract characterization tests for template field resolution through published Workers types. |
| `template_fields_test.go` | *(folded)* | Relocated to root as `template_fields_root_test.go` during story 003. |
| `token_lineage.go` | *(folded)* | Token lineage inlined in `execution_tokens.go` during story 003. |
| `validate_draft.go` | keep | Root forwarder for draft validation at the publication boundary. |
| `worker_vocabulary_boundary_test.go` | keep | Post-inventory boundary test proving peer consumers import Workers root vocabulary instead of Factory Definitions contracts. |
| `worker_vocabulary_contract.go` | keep | Package-level documentation anchor for Workers-owned peer execution and diagnostics vocabulary. |
| `workstation_contracts.go` | keep | Workstation pool lifecycle contracts, runtime-build vocabulary, and `Service` workstation methods at the thin root. |
| `workstation_pool_boundary_contracts.go` | keep | Published pool-boundary contracts and adapters for Factory Runtime dispatch planning through the thin root. |
| `workstation_pool_boundary_impl.go` | temporary documented keep | Pool-boundary implementation colocated at root to avoid workers↔poolboundary import cycles until DEL-WRK can relocate it behind a cycle-free bridge. |
| `workstation_pool_boundary_impl_test.go` | keep | Root-contract characterization test for pool-boundary binding capacity defaults. |
| `workstation_pool_boundary.go` | *(folded)* | Moved to `internal/services/workstations/poolboundary` during story 003; peers use published root contracts after story 005 retarget. |
| `workstation_pool_boundary_test.go` | *(folded)* | Moved to internal poolboundary during story 003. |
| `workstation_result_contract_test.go` | keep | Root-contract characterization test for `WorkstationResult` round-trip through published Workers root types. |

## Keep vs move summary (after story 006 cutover seal)

| Classification | Count | Target after CLN-WRK-CONTRACT-ROOTS |
| --- | ---: | --- |
| **keep** | 31 | Remain at `pkg/services/workers/` as thin root contracts |
| **move-to-runners** | 0 | Folded under `pkg/services/workers/internal/services/runners/runner` (story 002) |
| **move-to-workstations** | 0 | Folded under `pkg/services/workers/internal/services/workstations` (story 003) |
| **move-to-runtime_assembly** | 0 | — |
| **move-to-workers/internal** | 0 | Folded under `pkg/services/workers/internal` (story 004) |
| **temporary documented keep** | 1 | `workstation_pool_boundary_impl.go` until DEL-WRK cycle-free relocation |
| **Total** | **33** | 31 keep + 1 temporary documented keep + 1 delivery seal test |

## Keep vs move summary (after story 005)

| Classification | Count | Target after CLN-WRK-CONTRACT-ROOTS |
| --- | ---: | --- |
| **keep** | 30 | Remain at `pkg/services/workers/` as thin root contracts |
| **move-to-runners** | 0 | Folded under `pkg/services/workers/internal/services/runners/runner` (story 002) |
| **move-to-workstations** | 0 | Folded under `pkg/services/workers/internal/services/workstations` (story 003) |
| **move-to-runtime_assembly** | 0 | — |
| **move-to-workers/internal** | 0 | Folded under `pkg/services/workers/internal` (story 004) |
| **temporary documented keep** | 1 | `workstation_pool_boundary_impl.go` until DEL-WRK cycle-free relocation |
| **Total** | **31** | 30 keep + 1 temporary documented keep |

## Keep vs move summary (after story 004)

| Classification | Count | Target after CLN-WRK-CONTRACT-ROOTS |
| --- | ---: | --- |
| **keep** | 29 | Remain at `pkg/services/workers/` as thin root contracts |
| **move-to-runners** | 0 | Folded under `pkg/services/workers/internal/services/runners/runner` (story 002) |
| **move-to-workstations** | 0 | Folded under `pkg/services/workers/internal/services/workstations` (story 003) |
| **move-to-runtime_assembly** | 0 | — |
| **move-to-workers/internal** | 0 | Folded under `pkg/services/workers/internal` (story 004) |
| **temporary documented keep** | 0 | — |
| **Total** | **29** | 29 keep |

## Keep vs move summary (baseline before moves)

| Classification | Count | Target after CLN-WRK-CONTRACT-ROOTS |
| --- | ---: | --- |
| **keep** | 26 | Remain at `pkg/services/workers/` as thin root contracts |
| **move-to-runners** | 0 | Folded under `pkg/services/workers/internal/services/runners/runner` (story 002) |
| **move-to-workstations** | 0 | Folded under `pkg/services/workers/internal/services/workstations` (story 003) |
| **move-to-runtime_assembly** | 0 | — |
| **move-to-workers/internal** | 4 | `pkg/services/workers/internal` (3 delete-after-fold) |
| **temporary documented keep** | 0 | — |
| **Total** | **31** | 26 keep + 4 move |

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
