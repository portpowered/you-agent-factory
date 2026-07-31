# Recordings root `.go` contract-surface inventory (`pkg/services/recordings`)

Owner-local live inventory for **INV-REC-TOPLEVEL** (`pss-inv-rec-toplevel`). This
packet records the post-convergence root contract and its evidence-backed
classification.

**Inventory captured:** 2026-07-31 UTC from the live tree at
`pkg/services/recordings/*.go` (non-recursive root files only).

Companion directory inventory:
[`recordings-top-level-inventory.md`](recordings-top-level-inventory.md).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Thin committed root contract (keep)** | Intentional `package recordings` surface that cross-service peers may import directly: the singular `Service` interface, published slice request/result vocabulary, typed failure sentinels, metadata helpers, and root-contract characterization tests. Expected to remain at the service root after CLN-REC-CONTRACT-ROOTS, possibly slimmed. |
| **Excess fold/consolidation debt** | Root-level implementation, preparation, projection, replay, dispatch, or lifecycle helper logic that belongs under `recordings/internal/services/{canonical_ledger,projection_query,replay,recording_lifecycle,artifacts_export}` once later CLN/DEL packets fold matching top-level debt directories and excess contract clusters. |

Private subservices and common implementation packages under
`recordings/internal/*` own the implementation details behind this root seam.

## Root-level `.go` file inventory

| File | Classification | Later target / rationale |
| --- | --- | --- |
| `contracts.go` | Thin committed root contract (keep) | Singular `Service` interface plus aliases for transport-neutral event, projection, replay, lifecycle, artifact, metadata, and portable-recording vocabulary. The implementation declarations live under `recordings/internal/contracts`. |
| `contracts_test.go` | Thin committed root contract (keep) | Co-located characterization tests for published `Service` slice contracts. |
| `root_surface_test.go` | Thin committed root contract (keep) | Regression proof for one root `Service` interface, no exported root functions, and only canonical child directories. |
| `runtime_request_boundary_test.go` | Thin committed root contract (keep) | Runtime request-boundary test proving runtime dispatch constructs through the Recordings root contract. |
| `service_root_contract_fake_test.go` | Thin committed root contract (keep) | Root-contract fake harness for per-slice characterization through one root dependency. |
| `service_root_contract_invariants_test.go` | Thin committed root contract (keep) | Root-contract invariant characterization for append order, subscribe cursors, and projection outcomes. |
| `service_root_contract_lifecycle_test.go` | Thin committed root contract (keep) | Root-contract characterization for recording lifecycle bind/start/stop/finalize paths. |
| `service_root_contract_replay_test.go` | Thin committed root contract (keep) | Root-contract characterization for replay load/plan/execution success and typed failures. |
| `service_root_contract_seam_test.go` | Thin committed root contract (keep) | Peer-shaped characterization consumer exercising every published `Service` slice through one root dependency. |
| `workers_root_boundary_test.go` | Thin committed root contract (keep) | Workers import-boundary test proving worker execution stays on the published Recordings root seam. |

**Totals:** 10 root-level `.go` files — all thin committed root contract (keep);
no excess fold/consolidation debt remains at the public root after the Recordings
convergence seal.

## Folded clusters (CLN-REC-CONTRACT-ROOTS)

| Cluster | Destination | Folded files (no longer at public root) |
| --- | --- | --- |
| `contract vocabulary` | `recordings/internal/contracts` | `metadata.go`, `portable_recording.go`, `portable_recording_build.go`, `portable_recording_validate.go`, and the former root contract declarations |
| `canonical event wrappers` | `recordings/internal/events` | Former `canonical_ledger/events` implementation and tests |
| `projection wrappers` | `recordings/internal/projections` | Former `projection_query/projections` implementation and tests |
| `replay wrappers` | `recordings/internal/replay` | Former `replay/replay` implementation and tests |
| `artifact wrappers` | `recordings/internal/artifacts` | Former `artifacts_export/artifacts` implementation and tests |
| `live recording target` | `recordings/internal/services/recording_lifecycle/internal/service` | Private implementation and wire-only constructor |

Peer-needed Factory Event, world-state, dispatch, workstation-request, replay,
live-recording-target, and portable-recording vocabulary remains importable from
the thin Recordings root (`contracts.go`); private implementation packages own
the folded homes and characterization tests.

## Excess fold clusters

None. CLN-REC-CONTRACT-ROOTS sealed the public Recordings root to the committed
thin contract set.

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
- Further JSON baseline regeneration for later CLN/DEL packets.
- Future production package moves, folds, deletes, or `pkg/wire` edits.
