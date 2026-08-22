# Bun unit lane: workspace package source resolution

## Problem

Bun-native unit migrations that transitively import
`@you-agent-factory/factory-graph` or `@you-agent-factory/components/*` fail
under `bun test` even when the same modules load under Vitest
`dashboard-unit`.

Observed failure while migrating
`trace-relation-factory-graph-flow.test.ts`:

```text
error: Cannot find module './graph-edge' from
'.../ui/packages/components/dist/graphs/index.d.ts'
```

Vitest resolves these packages through dashboard/tsconfig source aliases.
Bun follows package `exports` into `dist`, and for `components/graphs` it can
land on the types entry (`index.d.ts`) whose relative imports do not resolve
as runtime modules. Building `packages/components` does not clear the failure.

This will repeatedly block Bun unit cohort migrations that touch factory-graph
projection/React Flow helpers, not just this leased file.

## Proposed direction

Owned by the Bun unit foundation / UI test lane, not by individual cohort
PRs:

1. Give the Bun unit runner the same source-package resolution strategy Vitest
   already uses for `@you-agent-factory/*` (preload, bunfig paths, or an
   explicit resolver), or
2. Document a durable Vitest-retention rule for unit files whose import graph
   requires those packages until that resolver exists.

Do not ask cohort migrations to add broad `mock.module` shims that weaken
coverage or invent per-file package fakes.

## Evidence

- Cohort: `BUN-UNIT-features-trace-drilldown-02`
- Retained file: `ui/src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.test.ts`
- Timing/evidence note:
  `docs/internal/development/plans/ui-test-latency/bun-unit-features-trace-drilldown-02-timing.md`
