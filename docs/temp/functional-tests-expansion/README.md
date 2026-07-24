# Functional Tests Expansion

This directory contains the implementation plan for rationalizing and expanding
the repository's customer-level functional tests.

## Documents

- [`plan.md`](plan.md) defines the goals, domain layout, generated
  visualization, golden-file contract, sequencing, and quality gates.
- [`test-file-checklist.md`](test-file-checklist.md) is the granular backlog.
  Each checkbox names one expected test file and is intended to be assignable
  to one agent without requiring ownership of a broad epic.
- [`migration-ledger.md`](migration-ledger.md) is the **Wave 0 migration
  authority**. It maps every current customer functional scenario to one
  checklist destination cell or an approved wrong-layer rationale, preserves
  short/long membership and specialty Make bindings, and names deletion-only
  batches for later move work. Planning-only: it does not move tests.
- [`migration-ledger-inventory.json`](migration-ledger-inventory.json) is the
  machine-readable companion to the ledger Inventory section (same required
  row fields). Use it for later batch tooling; keep it in sync with the
  Markdown inventory when mapping stories update rows. `runtime_api` rows are
  fully mapped into named `runtime_api-delete-*` deletion-only batches
  (FND-007-003). `smoke` and `workflow` rows are fully mapped into named
  `smoke-delete-*` and `workflow-delete-*` split batches (FND-007-004).
  `guards_batch`, `bootstrap_portability`, and `replay_contracts` rows are fully
  mapped into named `*-delete-*` split batches (FND-007-005). Remaining
  non-catch-all packages (`acceptance`, `cli`, `config_init`, `models`,
  `operator_settings`, `providers`, `sessionparity`, `work`) are fully mapped
  with `deletion_only_batch` = `n/a` (FND-007-006).
- [`provider-session-goldens.md`](provider-session-goldens.md) defines how
  sanitized provider execution output becomes checked-in golden test input and
  expected public metadata.

## Intended shape

Functional tests mirror the product and code, not transport-first feature
families:

```text
workers/<script|inference|mock>/...
orchestration/<javascript|petri>/...
workstations/<execution|cron|repeater|poller|watcher>/...
transport/<cli|http|mcp>/...
work/<submission|relationships|routing|recovery|visualization>/...
sessions/<lifecycle|controls|execution|restart>/...
factory/<definitions|packaged|current>/...
```

A customer looking for worker variants starts under `workers/`. Transport owns
only transport mechanics. Cross-domain scenarios are exceptions owned by the
primary domain.

## Priority

Implementation order is deliberate:

1. Wave 0 foundations (layout, metadata, viz, goldens, migration ledger)
2. Wave 1: `transport`, `workers`, `orchestration`, `workstations`
3. Wave 2: `work`, `sessions`, `factory`, `provider_sessions`, `events`
4. Wave 3: `models`, `guards`, `resources`, `observability`, `product`,
   `resilience`

## Work-cell rule

Every unchecked test-file item in `test-file-checklist.md` is one work cell.
An agent assigned a cell owns:

- the named test file;
- narrowly coupled fixtures or helpers;
- the smallest product correction revealed by the test;
- a customer-readable Go doc comment for every top-level `Test*`;
- focused verification for that file;
- any catalog metadata generated from the source.

An agent does not own adjacent cleanup, a shared refactor, or another cell
unless the checklist explicitly declares the dependency. This shape allows
dozens of agents to work concurrently after the shared harness cells land.
