# Workers top-level inventory (`pkg/services/workers`)

Owner-local live inventory for **INV-WRK-TOPLEVEL** (`pss-inv-wrk-toplevel`). This
packet records evidence-backed classification only; it does not move, fold, or
delete packages.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/workers/` (immediate child directories only).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Canonical** | Allowed direct service-root directories per packaged-structure rules (`wire`, `internal`, and `transports` when present). |
| **Providers-extraction source** | Legacy Workers-hosted surface slated for move into `pkg/services/providers/**` (already encoded for `provider/**`, `agypty`, `cliprovider`). |
| **Workers transitional debt** | Unexpected public top-level directory that must later move under `workers/internal` or `workers/internal/services/{runners,runtime_assembly,workstations}`; not canonical and not a Providers extraction source. |

No `transports/` directory is present under `pkg/services/workers` at inventory
time. This inventory does not invent transport adapters.

## Immediate child directory inventory

| Directory | Classification | Notes |
| --- | --- | --- |
| `agypty` | Providers-extraction source | Move target: `providers/internal/services/execution` (PTY provider adapter). |
| `cliprovider` | Providers-extraction source | Move target: `providers/internal/services/execution` (CLI provider adapter). |
| `construction` | Workers transitional debt | Runtime-assembly construction helpers; target subservice `runtime_assembly`. |
| `diagnostics` | Workers transitional debt | Worker diagnostics surface; target `workers/internal` (private diagnostics helper). |
| `execution` | Workers transitional debt | Workstation execution slice; target subservice `workstations`. |
| `executor` | Workers transitional debt | Workstation executor slice; target subservice `workstations`. |
| `interface` | Workers transitional debt | Legacy interface/helper surface; target `workers/internal`. |
| `internal` | Canonical | Private implementation tree (`internal/services/{runners,runtime_assembly,workstations}` already present). |
| `invocation` | Workers transitional debt | Invocation-time worker behavior; target subservice `workstations`. |
| `process` | Workers transitional debt | Process runner slice; target subservice `runners`. |
| `prompting` | Workers transitional debt | Prompting/workstation helpers; target subservice `workstations`. |
| `provider` | Providers-extraction source | Move target: `providers/internal/services/execution` (`provider/registry` → catalog). |
| `provider_test` | Providers-extraction source | Test support for provider extraction sources; move with Providers tree. |
| `runner` | Workers transitional debt | Runner slice; target subservice `runners`. |
| `service` | Workers transitional debt | Legacy `service/` implementation package; target `workers/internal`. |
| `services` | Workers transitional debt | Public sibling `services/` container (non-canonical); nested slices remap under `internal/services/*`. |
| `skippermissions` | Workers transitional debt | Permission-skip workstation helper; target subservice `workstations`. |
| `wire` | Canonical | Service-local Wire construction bridge. |
| `worktree` | Workers transitional debt | Worktree/workstation helper; target subservice `workstations`. |

**Totals:** 19 immediate child directories — 2 canonical, 5 Providers-extraction
sources, 12 Workers transitional debt, 0 `transports`.

## Out of scope for this note

- Root-level `.go` contract surfaces (see companion inventory in story 002).
- `packagetargetmanifestcheck` / `ownershipinventory` remap rows and JSON baseline
  regeneration (stories 003–005).
- Production package moves, folds, deletes, or `pkg/wire` edits.
