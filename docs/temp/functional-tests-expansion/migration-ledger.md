# Existing Functional Test Migration Ledger

**Status:** planning-only (Wave 0 / FND-007)  
**Authority:** this file is the Wave 0 migration authority for source→destination
mapping of current customer functional scenarios.

## Planning-only notice

This ledger is a **planning artifact**. It does **not** move, rename, split, or
delete functional test files. Later move batches consume the row mappings and
named deletion-only batch ids recorded here. Makefile specialty-target
retargets also belong to those later batches; this ledger only records current
bindings and intended post-move package/path targets.

Destination topology remains owned by
[`test-file-checklist.md`](test-file-checklist.md). Ownership rules remain
owned by [`plan.md`](plan.md).

## Row schema

Every customer scenario row uses the same required fields. Later mapping
stories append rows; they do not invent alternate column sets.

| Field | Required | Meaning |
| --- | --- | --- |
| `source_path` | yes | Current `_test.go` path relative to repo root |
| `package` | yes | Current Go package import path under `tests/functional/...` |
| `scenario` | yes | Top-level `Test*` function name |
| `lane` | yes | `short` or `functionallong` (from `//go:build` / package convention) |
| `destination` | yes* | Exact checklist cell path from `test-file-checklist.md`, **or** an approved wrong-layer rationale |
| `catch_all` | yes | Named catch-all owner if any: `runtime_api`, `smoke`, `workflow`, `guards_batch`, `bootstrap_portability`, `replay_contracts`, or `none` |
| `specialty_targets` | yes | Make target names that currently select this scenario or its package; use `none` when unbound |
| `deletion_only_batch` | yes* | Named batch id for later independent move work when applicable; use `n/a` when not part of a deletion-only batch |

\* During schema publication, inventory-only rows may leave `destination` and
`deletion_only_batch` as `TBD` until the matching mapping story fills them.
After FND-007 completes, no customer row may remain `TBD`.

### Destination values

Use exactly one of:

1. **Checklist cell path** — a path that already appears as a checkbox cell in
   `test-file-checklist.md` (do not invent destinations).
2. **Wrong-layer rationale** — `wrong-layer: <layer> — <why>` where `<layer>`
   names the correct proof layer (for example `unit`, `package-integration`,
   `stress/race`, `ui-browser`, `contract-smoke-outside-functional`) and `<why>`
   states why the scenario must not remain a customer functional owner. When
   replacement evidence already exists elsewhere, name that owner.

### Catch-all owners

| Owner | Later handling |
| --- | --- |
| `runtime_api` | Deletion-only batches until package ownership reaches zero |
| `smoke` | Split by durable domain owner; featureless package must reach zero |
| `workflow` | Split by durable domain owner; featureless package must reach zero |
| `guards_batch` | Split (example durable owners: `guards`, `resources`, `resilience`) |
| `bootstrap_portability` | Split (example durable owner: `factory/portability`) |
| `replay_contracts` | Split (example durable owner: `events/replay`) |
| `none` | Already under a durable or remaining non-catch-all package; still map |

### Lane membership

| Value | Rule |
| --- | --- |
| `functionallong` | Source file has `//go:build functionallong` (or equivalent long-only constraint) |
| `short` | Default / `!functionallong` / no long-only constraint |

### Markdown row template

Use this table shape in every inventory section:

```markdown
| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/<pkg>/<file>_test.go | you-agent-factory/tests/functional/<pkg> | TestExample | short | tests/functional/<domain>/.../<file>_test.go | none | none | n/a |
```

### Machine-readable companion (optional)

A JSON/YAML companion in this directory may mirror the same fields for later
batch tooling. If present, the Markdown ledger remains the human review
authority; the companion must not diverge from these required fields.

## Document sections (filled by later stories)

| Section | Owning story | Purpose |
| --- | --- | --- |
| [Inventory](#inventory) | FND-007-002 | Complete customer `Test*` inventory + exclusion list |
| [runtime_api deletion-only batches](#runtime_api-deletion-only-batches) | FND-007-003 | Map + batch every `runtime_api` scenario |
| [smoke and workflow split plans](#smoke-and-workflow-split-plans) | FND-007-004 | Explicit move/split plans |
| [guards_batch, bootstrap_portability, and replay_contracts split plans](#guards_batch-bootstrap_portability-and-replay_contracts-split-plans) | FND-007-005 | Explicit move/split plans |
| [Remaining packages and wrong-layer approvals](#remaining-packages-and-wrong-layer-approvals) | FND-007-006 | Map non-catch-all packages |
| [Specialty Make target bindings](#specialty-make-target-bindings) | FND-007-003…007 | Current vs intended post-move bindings |
| [Deletion-only batch index](#deletion-only-batch-index) | FND-007-003…007 | Ordered batch ids for later move work |
| [Completeness audit](#completeness-audit) | FND-007-007 | Zero-unmapped proof against live tree + checklist |

---

## Inventory

_Status: empty — filled by FND-007-002._

### Inventory summary

| Measure | Count |
| --- | ---: |
| Customer top-level `Test*` scenarios | TBD |
| Helper-only / non-customer harness files excluded | TBD |
| `tests/functional/internal/**` excluded | TBD |

### Customer scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | _TBD_ | _TBD_ | TBD |

### Non-customer harness exclusions

List helper-only files and `tests/functional/internal/**` harnesses explicitly
with rationale. Do not silently omit them from completeness accounting.

| path | rationale |
| --- | --- |
| _TBD_ | _TBD_ |

---

## runtime_api deletion-only batches

_Status: empty — filled by FND-007-003._

### Move/split plan

`runtime_api` is deletion-only debt. Every current customer scenario maps to
exactly one checklist destination or approved wrong-layer rationale, then is
grouped into named deletion-only batch ids that later work can execute
independently until package ownership reaches zero.

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | runtime_api | _TBD_ | TBD |

---

## smoke and workflow split plans

_Status: empty — filled by FND-007-004._

### smoke plan

_TBD: which scenarios move together, which split across domains, which become
deletion-only once coverage exists elsewhere._

### workflow plan

_TBD: which scenarios move together, which split across domains, which become
deletion-only once coverage exists elsewhere._

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | smoke\|workflow | _TBD_ | TBD |

---

## guards_batch, bootstrap_portability, and replay_contracts split plans

_Status: empty — filled by FND-007-005._

### guards_batch plan

_TBD: durable domain owners (for example `guards`, `resources`, `resilience`)._

### bootstrap_portability plan

_TBD: durable domain owners (for example `factory/portability`)._

### replay_contracts plan

_TBD: durable domain owners (for example `events/replay`)._

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | guards_batch\|bootstrap_portability\|replay_contracts | _TBD_ | TBD |

---

## Remaining packages and wrong-layer approvals

_Status: empty — filled by FND-007-006._

Covers every remaining customer scenario outside the six named catch-alls
(including `cli/**`, `providers/**`, `acceptance`, `sessionparity`, `work/**`,
`models/**`, `operator_settings/**`, `config_init`, and any other live
packages).

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | none | _TBD_ | n/a |

### Approved wrong-layer cases

| scenario | wrong-layer rationale | replacement evidence owner |
| --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ |

---

## Specialty Make target bindings

_Status: skeleton — filled as catch-all and completeness stories record bindings._

Each specialty Make target that currently selects a functional package or
`-run` pattern records its current binding and intended post-move
package/path binding. Do not change Makefile behavior in FND-007 unless a tiny
documentation comment is required to point at this ledger.

| Make target | Current binding | Intended post-move binding | Notes |
| --- | --- | --- | --- |
| `api-smoke` | TBD | TBD | Includes runtime_api generated-API smoke selector today |
| `docs-reference-smoke` | TBD | TBD | Includes `tests/functional/smoke` docs command selector today |
| `cron-time-work-smoke` | TBD | TBD | Includes runtime_api cron selector today |
| `current-factory-watcher-switch-smoke` | TBD | TBD | Includes bootstrap_portability selector today |
| `release-surface-smoke` / artifact closeout functional selectors | TBD | TBD | Includes runtime_api and replay_contracts selectors via closeout |
| `long-tests-managed-runtime` / related long selectors | TBD | TBD | Record actual Makefile bindings; preserve coverage intent |
| `pr-inference-approval` | TBD | TBD | runtime_api long-tag selector today |

---

## Deletion-only batch index

_Status: empty — filled by FND-007-003…007._

Ordered list of named deletion-only batches that later move work can consume
without inventing destinations. Prefer independent, reviewable batch sizes.

| batch_id | source catch-all | scenario count | destination domains | status |
| --- | --- | ---: | --- | --- |
| _TBD_ | _TBD_ | TBD | _TBD_ | planned |

---

## Completeness audit

_Status: empty — filled by FND-007-007._

| Check | Result |
| --- | --- |
| Unmapped customer scenarios vs fresh `tests/functional` inventory | TBD |
| Destination paths exist in `test-file-checklist.md` or approved wrong-layer | TBD |
| Short/long membership preserved on every row | TBD |
| Specialty Make targets fully accounted for | TBD |
| Deletion-only batch index covers runtime_api + featureless catch-alls | TBD |
| `make pkg-structure` | TBD |
