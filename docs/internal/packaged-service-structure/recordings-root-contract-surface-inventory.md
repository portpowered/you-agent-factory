# Recordings root `.go` contract-surface inventory (`pkg/services/recordings`)

Owner-local live inventory for **INV-REC-TOPLEVEL** (`pss-inv-rec-toplevel`). This
packet records evidence-backed classification only; it does **not** move, fold,
or delete any root-level `.go` files.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/recordings/*.go` (non-recursive root files only).

Companion directory inventory:
[`recordings-top-level-inventory.md`](recordings-top-level-inventory.md).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Thin committed root contract (keep)** | Intentional `package recordings` surface that cross-service peers may import directly: the singular `Service` interface, published slice request/result vocabulary, typed failure sentinels, metadata helpers, and root-contract characterization tests. Expected to remain at the service root after CLN-REC-CONTRACT-ROOTS, possibly slimmed. |
| **Excess fold/consolidation debt** | Root-level implementation, preparation, projection, replay, dispatch, or lifecycle helper logic that belongs under `recordings/internal/services/{canonical_ledger,projection_query,replay,recording_lifecycle,artifacts_export}` once later CLN/DEL packets fold matching top-level debt directories and excess contract clusters. |

Private subservices already present under `recordings/internal/services/*` are
**not** proof that matching root `.go` surfaces are migrated. This inventory
treats live root files as debt until CLN-REC-CONTRACT-ROOTS cutover packets
fold or delete them.

## Root-level `.go` file inventory

| File | Classification | Later target / rationale |
| --- | --- | --- |
| `contracts.go` | Thin committed root contract (keep) | Canonical `Service` interface, append/subscribe, projection/query, lifecycle, replay, and artifact-export slice vocabulary plus typed failure sentinels peers branch on via `errors.Is`. |
| `contracts_test.go` | Thin committed root contract (keep) | Co-located characterization tests for published `Service` slice contracts. |
| `metadata.go` | Thin committed root contract (keep) | Factory metadata mismatch warnings for replay-safe inspection at the public boundary. |
| `runtime_import_boundary_test.go` | Thin committed root contract (keep) | Runtime import-boundary test proving Factory Runtime peers stay on the published Recordings root seam. |
| `runtime_request_boundary_test.go` | Thin committed root contract (keep) | Runtime request-boundary test proving runtime dispatch constructs through the Recordings root contract. |
| `service_import_boundary_test.go` | Thin committed root contract (keep) | Service import-boundary test proving cross-service peers import only the thin Recordings root package. |
| `service_root_contract_fake_test.go` | Thin committed root contract (keep) | Root-contract fake harness for per-slice characterization through one root dependency. |
| `service_root_contract_invariants_test.go` | Thin committed root contract (keep) | Root-contract invariant characterization for append order, subscribe cursors, and projection outcomes. |
| `service_root_contract_lifecycle_test.go` | Thin committed root contract (keep) | Root-contract characterization for recording lifecycle bind/start/stop/finalize paths. |
| `service_root_contract_replay_test.go` | Thin committed root contract (keep) | Root-contract characterization for replay load/plan/execution success and typed failures. |
| `service_root_contract_seam_test.go` | Thin committed root contract (keep) | Peer-shaped characterization consumer exercising every published `Service` slice through one root dependency. |
| `wire_peer_import_boundary_test.go` | Thin committed root contract (keep) | Wire import-boundary test proving `recordings/wire` peers stay on the published root seam. |
| `workers_root_boundary_test.go` | Thin committed root contract (keep) | Workers import-boundary test proving worker execution stays on the published Recordings root seam. |
| `canonical_event_contract_test.go` | Excess fold/consolidation debt | `canonical_ledger` — canonical append acceptance characterization; move with `event` cluster. |
| `event_contract.go` | Excess fold/consolidation debt | `canonical_ledger` — event vocabulary helpers and append validation beyond thin `Service` slice types (`event` cluster). |
| `event_contract_test.go` | Excess fold/consolidation debt | Co-located tests for `event_contract.go`. |
| `event_vocabulary_boundary_test.go` | Excess fold/consolidation debt | Event vocabulary boundary characterization; move with `event` cluster. |
| `events_import_boundary_test.go` | Excess fold/consolidation debt | Events import-boundary test; relocate with `event` cluster fold. |
| `world_state_contract.go` | Excess fold/consolidation debt | `projection_query` — world-state reconstruction helpers beyond thin projection vocabulary (`world_state` cluster). |
| `world_state_contract_test.go` | Excess fold/consolidation debt | Co-located tests for `world_state_contract.go`. |
| `projections_import_boundary_test.go` | Excess fold/consolidation debt | Projections import-boundary test; relocate with `world_state` cluster fold. |
| `artifacts_import_boundary_test.go` | Excess fold/consolidation debt | `artifacts_export` — artifacts import-boundary test (`artifacts` cluster). |
| `replay_contract.go` | Excess fold/consolidation debt | `replay` — replay plan/load vocabulary helpers beyond thin replay slice types (`replay` cluster). |
| `replay_import_boundary_test.go` | Excess fold/consolidation debt | Replay import-boundary test; relocate with `replay` cluster fold. |
| `dispatch_contract.go` | Excess fold/consolidation debt | `projection_query` — dispatch projection helpers (`dispatch` cluster). |
| `workstation_requests.go` | Excess fold/consolidation debt | `projection_query` — workstation-request projection helpers (`workstation_request` cluster). |
| `workstation_requests_content_assert_test.go` | Excess fold/consolidation debt | Co-located content-assert tests for workstation-request projection helpers. |
| `workstation_requests_test.go` | Excess fold/consolidation debt | Co-located tests for `workstation_requests.go`. |
| `live_recording_target.go` | Excess fold/consolidation debt | `recording_lifecycle` — live recording target path/naming planning (`live_recording_target` cluster). |
| `live_recording_target_test.go` | Excess fold/consolidation debt | Co-located tests for `live_recording_target.go`. |

**Totals:** 30 root-level `.go` files — 13 thin committed root contract (keep), 17
excess fold/consolidation debt.

## Excess fold clusters

| Cluster | Destination | Root files |
| --- | --- | --- |
| `event` | `recordings/internal/services/canonical_ledger` | `canonical_event_contract_test.go`, `event_contract.go`, `event_contract_test.go`, `event_vocabulary_boundary_test.go`, `events_import_boundary_test.go` |
| `world_state` | `recordings/internal/services/projection_query` | `world_state_contract.go`, `world_state_contract_test.go`, `projections_import_boundary_test.go` |
| `artifacts` | `recordings/internal/services/artifacts_export` | `artifacts_import_boundary_test.go` |
| `replay` | `recordings/internal/services/replay` | `replay_contract.go`, `replay_import_boundary_test.go` |
| `dispatch` | `recordings/internal/services/projection_query` | `dispatch_contract.go` |
| `workstation_request` | `recordings/internal/services/projection_query` | `workstation_requests.go`, `workstation_requests_content_assert_test.go`, `workstation_requests_test.go` |
| `live_recording_target` | `recordings/internal/services/recording_lifecycle` | `live_recording_target.go`, `live_recording_target_test.go` |

## Generator mirror

Committed generator tables mirror this inventory:

- `internal/ownershipinventory/recordings_root_contract.go` — `RecordingsThinRootContractFiles`,
  `RecordingsExcessRootContractFolds`, `ClassifyRecordingsRootContractFile`, and
  `RecordingsRootContractInventory`.
- `cmd/packagetargetmanifestcheck/recordings_root_contract.go` — package-target manifest
  mirror for the same thin retain vs excess fold partition.

## Out of scope for this note

- Nested packages under immediate child directories (classified in
  [`recordings-top-level-inventory.md`](recordings-top-level-inventory.md)).
- JSON baseline regeneration (later CLN/DEL packets).
- Production package moves, folds, deletes, or `pkg/wire` edits.
