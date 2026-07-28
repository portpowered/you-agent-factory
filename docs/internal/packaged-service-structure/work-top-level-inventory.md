# Work top-level inventory (`pkg/services/work`)

Owner-local live inventory for **INV-WORK-TOPLEVEL** (`pss-inv-work-toplevel`). This
packet records evidence-backed classification only; it does not move, fold, or
delete packages.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/work/` (immediate child directories only) after DEL-WORK transitional
public deletion (`service/`, `stateaccessrecordings/`).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Canonical** | Allowed direct service-root directories per packaged-structure rules (`wire`, `internal`, and `transports`). |
| **Work transitional debt** | Unexpected public top-level directory that must later move under `work/internal` or `work/internal/services/{content_staging,content_materialization,state_access}`; not canonical and must not retain at the owner root. |

Private subservices already present under `work/internal/services/*` are
**not** proof that matching public siblings are migrated. This inventory treats
live public siblings as debt until CLN/DEL cutover packets remove them.

## Immediate child directory inventory

| Directory | Classification | Disposition | Destination |
| --- | --- | --- | --- |
| `internal` | Canonical | retain | `work` (private implementation tree; subservices `content_staging`, `content_materialization`, and `state_access` already exist) |
| `testdata` | Work transitional debt | move | `work/internal` (test-only fixtures; not a canonical retain-at-root exception) |
| `transports` | Canonical | retain | `work` |
| `wire` | Canonical | retain | `work` |

**Deleted after DEL-WORK:** `service/` and `stateaccessrecordings/` transitional
public paths were removed in DEL-WORK story 002; they no longer appear on disk or
in the committed owner top-level inventory.

**Totals:** 4 immediate child directories — 3 canonical, 1 Work transitional debt.

## Generator mirror

The committed generator tables in `internal/ownershipinventory/owner_top_level.go`
and `cmd/packagetargetmanifestcheck/owner_top_level.go` mirror this inventory:

- **Expected retain:** `internal`, `transports`, `wire`
- **Unexpected move siblings:** `testdata`

Move destinations align with `workMoveRules` / `nestedOwnerMoveRules` for `work`:
`testdata` → `work/internal`. Mapping rules are confirmed in
`internal/ownershipinventory/work_mapping.go`, `cmd/packagetargetmanifestcheck/owners.go`
(`nestedOwnerMoveRules["work"]`), and locked by
`TestWorkTopLevelUnexpectedMoveDestinationsMatchInventory` /
`TestWorkTopLevelUnexpectedCoveredByMoveRules`.

## Related inventory

- Root-level `.go` contract surfaces:
  [`work-root-contract-surface-inventory.md`](work-root-contract-surface-inventory.md).

## Out of scope for this note
- Excess root `.go` contract fold encoding and baseline regeneration (stories 004–006).
- Production package moves, folds, deletes, or `pkg/wire` edits.
