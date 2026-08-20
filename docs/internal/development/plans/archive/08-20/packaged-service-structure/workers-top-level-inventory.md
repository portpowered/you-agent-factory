# Workers top-level inventory (`pkg/services/workers`)

Owner-local live inventory for **INV-WRK-TOPLEVEL** (`pss-inv-wrk-toplevel`). This
packet records the evidence-backed immediate child directory shape after the
Providers extraction; package-target and ownership ledgers are generated from
the live tree separately.

**Inventory captured:** 2026-07-31 UTC from the live tree at
`pkg/services/workers/` (immediate child directories only).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Canonical** | Allowed direct service-root directories per packaged-structure rules (`wire`, `internal`, and `transports` when present). |
| **Providers-extraction source** | No live Workers top-level directory remains in this classification; provider execution and catalog packages now live under `pkg/services/providers/**`. |
| **Workers transitional debt** | No live Workers top-level directory remains in this classification; transitional implementation packages are private under `workers/internal/**`. |

No `transports/` directory is present under `pkg/services/workers` at inventory
time. This inventory does not invent transport adapters.

## Immediate child directory inventory

| Directory | Classification | Notes |
| --- | --- | --- |
| `internal` | Canonical | Private Workers implementation tree, including `internal/services/{runners,runtime_assembly,workstations}`. |
| `wire` | Canonical | Service-local Wire construction bridge. |

**Totals:** 2 immediate child directories — 2 canonical, 0 Providers-extraction
sources, 0 Workers transitional debt directories, 0 `transports`.

## Out of scope for this note

- Root-level `.go` contract surfaces (see the companion inventory in
  [`workers-root-contract-surface-inventory.md`](workers-root-contract-surface-inventory.md)).
- Service-root contract sealing and internal subservice normalization.
- Shared `pkg/wire` and protocol-composition cutovers.
