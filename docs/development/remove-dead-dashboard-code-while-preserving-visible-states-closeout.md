# Remove Dead Dashboard Code While Preserving Visible States Closeout

Date: 2026-05-05

## Scope

This closeout records the `US-004` dashboard-lane cleanup completed in the May
2026 dead-code batch.

The change removed duplicate dashboard typography and formatter re-export shims
that had become shadow-only owners over the canonical shared UI modules. The
surviving dashboard, selection, trace, submit-work, import/export, and Storybook
surfaces now import the canonical dashboard typography and formatter contracts
directly.

## Canonical Owners

| Behavior lane | Canonical surviving owner | Removed or collapsed shadow owner |
| --- | --- | --- |
| Shared dashboard typography contract | `ui/src/components/ui/dashboard-typography.ts` | `ui/src/components/dashboard/typography.ts` and feature-local `typography.ts` re-export shims under `terminal-work`, `trace-drilldown`, and `work-outcome` |
| Shared dashboard formatter helpers | `ui/src/components/ui/formatters.ts` | `ui/src/components/dashboard/formatters.ts` |
| Shared dashboard place-label helpers | `ui/src/components/ui/place-labels.ts` | `ui/src/components/dashboard/place-labels.ts` |

## Behavior Preservation

- Dashboard loading and error shell states still render through
  `DashboardStatusPanel` while the canonical stream and timeline state owners
  remain unchanged.
- Header status, current-selection cards, trace drill-down cards, submit-work
  controls, import/export dialogs, and trend cards still use the same typed
  dashboard typography classes after the import collapse.
- Storybook stories and app tests now point at the same canonical typography
  owner instead of relying on the removed dashboard barrel exports.

## Verification

- `cd ui && bun run tsc`
- `cd ui && bun run test`
- `cd ui && bun run build`
- `cd ui && bun run replay:coverage:check`
- `cd ui && bun run build-storybook`

## Notes

- The requested `dev-browser` skill is not available in this session, so the
  closest repo-owned browser verification was the Storybook lane.
- `bun run test-storybook` still fails in this Windows/Bun environment after the
  cleanup-specific import regressions were fixed. The remaining failure is a
  Storybook Vitest runtime issue (`react` default-export resolution inside
  `@storybook/addon-vitest`), not a dashboard assertion failure introduced by
  this lane.
