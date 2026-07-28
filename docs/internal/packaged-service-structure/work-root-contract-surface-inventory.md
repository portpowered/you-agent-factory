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
| `admission_contract.go` | Thin committed root contract (keep) | Admission slice typed failures (`ErrInvalidWorkRequest`, `ErrWorkRequestConflict`, `ErrWorkRequestRejected`) peers branch on via `errors.Is`. |
| `content_contract.go` | Thin committed root contract (keep) | Content materialization effect ports and `ContentMaterializer` narrow role published at the Work root boundary. |
| `content_materialization_public_seam_test.go` | Thin committed root contract (keep) | Root-contract characterization test for the published content materialization slice on `Service`. |
| `content_materialize_contract.go` | Thin committed root contract (keep) | Materialization slice typed failures (`ErrUnsafeContentURL`, `ErrContentURLInaccessible`). |
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
| `arguments.go` | Excess fold/consolidation debt | `work/internal` — invocation argument normalization, metadata projection, and signature binding logic (`invocation_return_policy` cluster). |
| `arguments_test.go` | Excess fold/consolidation debt | Co-located tests for `arguments.go`; move with invocation/return-policy fold. |
| `content_staging.go` | Excess fold/consolidation debt | `work/internal/services/content_staging` — staging filesystem effects, `ContentStagingService` wiring, and staging implementation helpers beyond thin `Service` slice types. |
| `content_url.go` | Excess fold/consolidation debt | `work/internal/services/content_materialization` — content URL validation and filesystem-to-URL mapping helpers. |
| `dependency_graph.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — batch dependency graph derivation (`lineage_graph_modules` cluster). |
| `dependency_graph_markdown.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — markdown dependency-graph rendering helpers. |
| `dependency_graph_markdown_test.go` | Excess fold/consolidation debt | Co-located tests for `dependency_graph_markdown.go`. |
| `dependency_graph_mermaid.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — mermaid dependency-graph rendering helpers. |
| `dependency_graph_mermaid_test.go` | Excess fold/consolidation debt | Co-located tests for `dependency_graph_mermaid.go`. |
| `dependency_graph_test.go` | Excess fold/consolidation debt | Co-located tests for `dependency_graph.go`. |
| `file_inputs.go` | Excess fold/consolidation debt | `work/internal` — path-backed request and payload file loaders for transport edges. |
| `file_inputs_test.go` | Excess fold/consolidation debt | Co-located tests for `file_inputs.go`. |
| `invocation_input_preparation.go` | Excess fold/consolidation debt | `work/internal` — `PrepareInvocationInput` implementation and argv/stdin normalization policy. |
| `invocation_input_preparation_test.go` | Excess fold/consolidation debt | Co-located tests for `invocation_input_preparation.go`. |
| `lineage.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — payload lineage projection types and replay-safe derivation helpers. |
| `primary_result.go` | Excess fold/consolidation debt | `work/internal` — primary-result selection policy, world-state evaluation, and return-policy resolution logic. |
| `primary_result_regression_test.go` | Excess fold/consolidation debt | Regression fixtures for `primary_result.go`; relocate with invocation/return-policy fold. |
| `primary_result_test.go` | Excess fold/consolidation debt | Co-located tests for `primary_result.go`. |
| `query_list.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — list query normalization, validation, and pagination preparation beyond thin `ListOptions` vocabulary. |
| `query_list_test.go` | Excess fold/consolidation debt | Co-located tests for `query_list.go`. |
| `query_select.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — in-memory Work selection and ordering policy used by state-access reads. |
| `query_select_test.go` | Excess fold/consolidation debt | Co-located tests for `query_select.go`. |
| `request_codec.go` | Excess fold/consolidation debt | `work/internal` — canonical Work Request JSON decode/preparation roles for batch transports. |
| `request_normalize.go` | Excess fold/consolidation debt | `work/internal` — `NormalizeWorkRequest`, single-target preparation, and admission normalization policy. |
| `request_normalize_test.go` | Excess fold/consolidation debt | Co-located tests for `request_normalize.go`. |
| `request_preparation.go` | Excess fold/consolidation debt | `work/internal` — `RequestPreparationService` admission policy before Factory Session submit. |
| `request_preparation_test.go` | Excess fold/consolidation debt | Co-located tests for `request_preparation.go`. |
| `request_submit_test.go` | Excess fold/consolidation debt | Admission submit integration tests that exercise request preparation through the root package; relocate with request fold. |
| `visualization.go` | Excess fold/consolidation debt | `work/internal/services/state_access` — batch-file dependency visualization operation binding (`lineage_graph_modules` cluster). |
| `visualization_test.go` | Excess fold/consolidation debt | Co-located tests for `visualization.go`. |

**Totals:** 45 root-level `.go` files — 15 thin committed root contract (keep), 30
excess fold/consolidation debt.

## Excess fold clusters

| Cluster | Destination | Root files |
| --- | --- | --- |
| `request_admission` | `work/internal` | `file_inputs.go`, `file_inputs_test.go`, `request_codec.go`, `request_normalize.go`, `request_normalize_test.go`, `request_preparation.go`, `request_preparation_test.go`, `request_submit_test.go` |
| `invocation_return_policy` | `work/internal` | `arguments.go`, `arguments_test.go`, `invocation_input_preparation.go`, `invocation_input_preparation_test.go`, `primary_result.go`, `primary_result_test.go`, `primary_result_regression_test.go` |
| `lineage_graph_modules` | `work/internal/services/state_access` | `dependency_graph.go`, `dependency_graph_test.go`, `dependency_graph_markdown.go`, `dependency_graph_markdown_test.go`, `dependency_graph_mermaid.go`, `dependency_graph_mermaid_test.go`, `lineage.go`, `visualization.go`, `visualization_test.go` |
| `state_access_query` | `work/internal/services/state_access` | `query_list.go`, `query_list_test.go`, `query_select.go`, `query_select_test.go` |
| `content_staging_impl` | `work/internal/services/content_staging` | `content_staging.go` |
| `content_materialization_impl` | `work/internal/services/content_materialization` | `content_url.go` |

## Out of scope for this note

- Nested packages under immediate child directories (classified in
  [`work-top-level-inventory.md`](work-top-level-inventory.md)).
- `packagetargetmanifestcheck` / `ownershipinventory` root-contract generator
  tables and JSON baseline regeneration (stories 003–006).
- Production package moves, folds, deletes, or `pkg/wire` edits.
