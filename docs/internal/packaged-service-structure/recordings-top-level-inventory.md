# Recordings top-level inventory (`pkg/services/recordings`)

Owner-local live inventory for **INV-REC-TOPLEVEL** (`pss-inv-rec-toplevel`). This
packet records the post-convergence direct-child classification.

**Inventory captured:** 2026-07-31 UTC from the live tree at
`pkg/services/recordings/` (immediate child directories only).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Canonical** | Allowed direct service-root directories per packaged-structure rules (`wire`, `internal`, and `transports` when present). |
| **Recordings transitional debt** | Unexpected public top-level directory that must later move under `recordings/internal` or `recordings/internal/services/{canonical_ledger,projection_query,replay,recording_lifecycle,artifacts_export}`; not canonical. |

## Immediate child directory inventory

| Directory | Classification | Notes |
| --- | --- | --- |
| `internal` | Canonical | Private implementation tree (`internal/services/*` subservices already present). |
| `transports` | Canonical | Service-local HTTP/CLI/MCP protocol adapters. |
| `wire` | Canonical | Service-local Wire construction bridge. |

**Totals:** 3 immediate child directories — all canonical; no transitional public
Recordings siblings remain.

## Generator mirror

Committed generator tables mirror this inventory:

- `internal/ownershipinventory/recordings_top_level.go` — `RecordingsTopLevelExpectedRetain`,
  `RecordingsTopLevelUnexpected`, `ClassifyRecordingsTopLevelChild`, and
  `RecordingsTopLevelInventory`.
- `cmd/packagetargetmanifestcheck/recordings_top_level.go` — package-target manifest
  mirror for the same canonical vs unexpected partition.

## Out of scope for this note

- Root-level `.go` contract surfaces (see companion inventory in
  [`recordings-root-contract-surface-inventory.md`](recordings-root-contract-surface-inventory.md)).
- Future `packagetargetmanifestcheck` / `ownershipinventory` remap rows and JSON
  baseline regeneration.
- Future package moves, folds, deletes, or `pkg/wire` edits.
