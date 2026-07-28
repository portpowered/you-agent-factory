# Work root `.go` contract-surface inventory (`pkg/services/work`)

Owner-local live inventory for **INV-WORK-TOPLEVEL** (`pss-inv-work-toplevel`). This
packet records evidence-backed classification only; it does **not** move, fold,
or delete any root-level `.go` files.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/work/*.go` (non-recursive root files only).

Companion directory inventory:
[`work-top-level-inventory.md`](work-top-level-inventory.md).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Thin committed root contract (keep)** | Intentional `package work` surface that cross-service peers may import directly: the singular `Service` interface, published slice request/result vocabulary, typed failure sentinels, and root-contract characterization tests. Expected to remain at the service root after CLN-WORK-CONTRACT-ROOTS, possibly slimmed. |
| **Excess fold/consolidation debt** | Root-level implementation, preparation, projection, or helper logic that belongs under `work/internal` or `work/internal/services/{content_staging,content_materialization,state_access}` once later CLN/DEL packets fold matching top-level debt directories and excess contract clusters. |

Private subservices already present under `work/internal/services/*` are
**not** proof that matching root `.go` surfaces are migrated. This inventory
treats live root files as debt until CLN-WORK-CONTRACT-ROOTS cutover packets
fold or delete them.

## Root-level `.go` file inventory

| File | Classification | Later target / rationale |
| --- | --- | --- |
| `admission_contract.go` | Thin committed root contract (keep) | Admission slice typed failures, request preparation interfaces, and thin delegators to `work/internal/requestadmission`. |
| `content_contract.go` | Thin committed root contract (keep) | Content materialization effect ports, `ContentMaterializer` narrow role, and published content URL vocabulary delegators for peers. |
| `content_materialization_public_seam_test.go` | Thin committed root contract (keep) | Root-contract characterization test for the published content materialization slice on `Service`. |
| `content_materialize_contract.go` | Thin committed root contract (keep) | Materialization slice typed failures (`ErrUnsafeContentURL`, `ErrContentURLInaccessible`). |
| `content_staging_contract.go` | Thin committed root contract (keep) | Content staging effect ports, `ContentStagingService` narrow role, staging request/result vocabulary, and typed staging failures. |
| `contracts.go` | Thin committed root contract (keep) | Canonical Work request, content-part, relation, invocation-config, and admission vocabulary shared across published slices. |
| `input.go` | Thin committed root contract (keep) | Invocation input vocabulary (`InputSourceLabel`, `ResolvedInput`, `TextInputSources`, `InputErrorCode`) consumed by peers without importing implementation packages. |
| `input_test.go` | Thin committed root contract (keep) | Co-located characterization tests for `input.go` root vocabulary. |
| `invocation_return_policy_contract.go` | Thin committed root contract (keep) | Invocation/return-policy slice typed failures (`ErrInvalidInvocationInput`, `ErrUnsupportedReturnPolicy`). |
| `read_contract.go` | Thin committed root contract (keep) | State-access slice detached projections (`ReadModel`, `ListResult`, `ErrWorkNotFound`, list defaults). |
| `recordings_import_boundary_test.go` | Thin committed root contract (keep) | CUT-WORK-REC boundary test proving Recordings imports stay on the published Work root seam. |
| `recordings_request_boundary_test.go` | Thin committed root contract (keep) | CUT-WORK-REC boundary test proving state-access Recordings queries construct through the Work root contract. |
| `service_contract.go` | Thin committed root contract (keep) | Singular `Service` interface plus `Runtime`, `RuntimeResolver`, and constructor port types for the sealed IMP-WORK root. |
| `service_root_contract_seal_test.go` | Thin committed root contract (keep) | Peer-shaped characterization consumer exercising every published `Service` slice through one root dependency. |
| `service_root_contract_test.go` | Thin committed root contract (keep) | Root-contract fake and per-slice success/failure characterization for admission, content, state access, and invocation slices. |
| `content_staging_public_seam_test.go` | Thin committed root contract (keep) | Root-contract characterization test for the published content staging slice on `Service`. |
| `legacy_packages_disposition_test.go` | Thin committed root contract (keep) | Post-inventory characterization test locking transitional public sibling retention and published-surface import boundaries after legacy package fold. |
| `service_import_boundary_test.go` | Thin committed root contract (keep) | Post-inventory import-boundary proof sealing production packages against transitional `work/service` imports. |
| `service_peer_bindings.go` | Thin committed root contract (keep) | Post-inventory peer-binding constructors projecting published `Service` slices (`MaterializationService`, `AdmissionContentService`, and related bindings). |
| `service_peer_bindings_test.go` | Thin committed root contract (keep) | Co-located characterization tests for `service_peer_bindings.go` peer constructors. |
| `wire_behavioral_proof_test.go` | Thin committed root contract (keep) | Post-inventory wire behavioral proof exercising admission, content, state-access, and invocation observables through `work/wire`. |
| `arguments.go` | Excess fold/consolidation debt | `work/internal` — invocation argument normalization, metadata projection, and signature binding logic (`invocation_return_policy` cluster). |
| `arguments_test.go` | Excess fold/consolidation debt | Co-located tests for `arguments.go`; move with invocation/return-policy fold. |
| `dependency_graph.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 004 to `work/internal/lineagegraph`; thin lineage/graph/visualization vocabulary and delegators remain on `lineage_contract.go`. |
| `dependency_graph_markdown.go` | Excess fold/consolidation debt | **Folded** with `dependency_graph.go` to `work/internal/lineagegraph`. |
| `dependency_graph_markdown_test.go` | Excess fold/consolidation debt | **Folded** with `dependency_graph.go` to `work/internal/lineagegraph`. |
| `dependency_graph_mermaid.go` | Excess fold/consolidation debt | **Folded** with `dependency_graph.go` to `work/internal/lineagegraph`. |
| `dependency_graph_mermaid_test.go` | Excess fold/consolidation debt | **Folded** with `dependency_graph.go` to `work/internal/lineagegraph`. |
| `dependency_graph_test.go` | Excess fold/consolidation debt | **Folded** with `dependency_graph.go` to `work/internal/lineagegraph`. |
| `file_inputs.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 005 to `work/internal/requestadmission`; thin file-loader delegators stay on `admission_contract.go`. |
| `file_inputs_test.go` | Excess fold/consolidation debt | **Folded** with `file_inputs.go` to `work/internal/requestadmission`. |
| `invocation_input_preparation.go` | Excess fold/consolidation debt | `work/internal` — `PrepareInvocationInput` implementation and argv/stdin normalization policy. |
| `invocation_input_preparation_test.go` | Excess fold/consolidation debt | Co-located tests for `invocation_input_preparation.go`. |
| `invocation_policy_service.go` | Excess fold/consolidation debt | `work/internal` — `NewInvocationPolicyService` invocation/return-policy slice constructor and inert `Service` implementation body (`invocation_return_policy` cluster). |
| `invocation_policy_service_test.go` | Excess fold/consolidation debt | Co-located tests for `invocation_policy_service.go`; relocate with invocation/return-policy fold. |
| `lineage.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 004 to `work/internal/lineagegraph`; payload-lineage projection types and delegators remain on `lineage_contract.go`. |
| `lineage_contract.go` | Thin committed root contract (keep) | Published lineage/graph/visualization vocabulary plus thin delegators to `work/internal/lineagegraph`. |
| `primary_result.go` | Excess fold/consolidation debt | `work/internal` — primary-result selection policy, world-state evaluation, and return-policy resolution logic. |
| `primary_result_regression_test.go` | Excess fold/consolidation debt | Regression fixtures for `primary_result.go`; relocate with invocation/return-policy fold. |
| `primary_result_test.go` | Excess fold/consolidation debt | Co-located tests for `primary_result.go`. |
| `query_list.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 003 to `work/internal/stateaccessquery`; thin list vocabulary and delegators remain on `read_contract.go`. |
| `query_list_test.go` | Excess fold/consolidation debt | **Folded** with `query_list.go` to `work/internal/stateaccessquery`. |
| `query_select.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 003 to `work/internal/stateaccessquery`; selection policy is private to state_access reads. |
| `query_select_test.go` | Excess fold/consolidation debt | **Folded** with `query_select.go` to `work/internal/stateaccessquery`. |
| `request_codec.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 005 to `work/internal/requestadmission`; canonical JSON decode/preparation delegators stay on `admission_contract.go`. |
| `request_normalize.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 005 to `work/internal/requestadmission`; `NormalizeWorkRequest` and related delegators stay on `admission_contract.go`. |
| `request_normalize_test.go` | Excess fold/consolidation debt | **Folded** with `request_normalize.go` to `work/internal/requestadmission`. |
| `request_preparation.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 005 to `work/internal/requestadmission`; `RequestPreparationService` delegators stay on `admission_contract.go`. |
| `request_preparation_test.go` | Excess fold/consolidation debt | **Folded** with `request_preparation.go` to `work/internal/requestadmission`. |
| `request_submit_test.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 005 to `work/internal/requestadmission`. |
| `visualization.go` | Excess fold/consolidation debt | **Folded** in CLN-WORK-CONTRACT-ROOTS story 004 to `work/internal/lineagegraph`; `NewVisualizationOperation` stays on `lineage_contract.go` as a thin delegator. |
| `visualization_test.go` | Excess fold/consolidation debt | **Folded** with `visualization.go` to `work/internal/lineagegraph`. |

**Totals:** 31 root-level `.go` files — 22 thin committed root contract (keep), 9
excess fold/consolidation debt.

`content_staging_impl` and `content_materialization_impl` folded in
CLN-WORK-CONTRACT-ROOTS story 002: staging contracts remain at
`content_staging_contract.go`, URL helpers live under
`work/internal/services/content_materialization/url.go`, and published URL
vocabulary stays on `content_contract.go` as thin delegators.

`state_access_query` folded in CLN-WORK-CONTRACT-ROOTS story 003: list/selection
implementation lives under `work/internal/stateaccessquery`, and thin list
vocabulary plus `NormalizeList` / `NewListRequestPreparation` delegators stay
on `read_contract.go`.

`lineage_graph_modules` folded in CLN-WORK-CONTRACT-ROOTS story 004:
lineage/graph/visualization implementation lives under
`work/internal/lineagegraph`, and thin published vocabulary plus delegators stay
on `lineage_contract.go`.

`request_admission` folded in CLN-WORK-CONTRACT-ROOTS story 005:
request admission implementation lives under `work/internal/requestadmission`, and
thin admission vocabulary plus delegators stay on `admission_contract.go`.

## Excess fold clusters

| Cluster | Destination | Root files |
| --- | --- | --- |
| `invocation_return_policy` | `work/internal` | `arguments.go`, `arguments_test.go`, `invocation_input_preparation.go`, `invocation_input_preparation_test.go`, `invocation_policy_service.go`, `invocation_policy_service_test.go`, `primary_result.go`, `primary_result_test.go`, `primary_result_regression_test.go` |

## Generator mirror

Committed generator tables mirror this inventory:

- `internal/ownershipinventory/work_root_contract.go` — `WorkThinRootContractFiles`,
  `WorkExcessRootContractFolds`, `ClassifyWorkRootContractFile`, and
  `WorkRootContractInventory`.
- `cmd/packagetargetmanifestcheck/work_root_contract.go` — package-target manifest
  mirror for the same thin retain vs excess fold partition.

## Out of scope for this note

- Nested packages under immediate child directories (classified in
  [`work-top-level-inventory.md`](work-top-level-inventory.md)).
- JSON baseline regeneration (story 006).
- Production package moves, folds, deletes, or `pkg/wire` edits.
