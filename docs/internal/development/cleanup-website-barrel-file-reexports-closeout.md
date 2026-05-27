# Cleanup Website Barrel File Re-exports Closeout

Date: 2026-05-27

## Scope

This closeout records the final verification pass for the
`cleanup-website-barrel-file-reexports` branch after redundant website
feature-barrel re-exports were retired.

The branch intentionally stayed limited to frontend import seams under `ui/`
and one maintainer-process note. It did not change backend code, OpenAPI
contracts, generated artifacts, routing, design tokens, or browser-visible UI
behavior.

## Retired Re-export Seams

| Seam | Retired public or index exports | Canonical owner for maintained consumers |
| --- | --- | --- |
| `ui/src/features/current-selection/public/index.ts` | `components/current-selection-cards` | Direct component modules under `current-selection/components/` |
| `ui/src/features/header/public/index.ts` | `components/dashboard-session-tabs`, `components/tick-slider-control` | Direct component modules under `header/components/` |
| `ui/src/features/workflow-activity/public/index.ts` | `components/dashboard-flow-axis-legend`, `components/react-flow-current-activity-card`, `components/workflow-activity-bento-card` | Direct component modules under `workflow-activity/components/` |
| `ui/src/features/timeline/state/index.ts` | Deleted one-line state barrel for timeline debug/store exports | `timeline/state/factoryTimelineStore` and the owning state module |
| `ui/src/features/terminal-work/messages/index.ts` | Deleted one-line message barrel | `terminal-work/messages/terminal-work` |
| `ui/src/features/current-factory-definition/public/index.ts` | `lib/workstation-behavior`, `lib/workstation-editable-values` | Direct helper modules under `current-factory-definition/lib/` |
| `ui/src/features/flowchart/public/index.ts` | `lib/layout`, `lib/workstation-semantics` | Direct helper modules under `flowchart/lib/` |
| `ui/src/features/import/public/index.ts` | `lib/factory-png-import` | `import/lib/factory-png-import` |
| `ui/src/features/provider-session-detail/public/index.ts` | `lib/provider-session-ref` | `provider-session-detail/lib/provider-session-ref` |

## Supported Public Exports

The surviving public barrels continue to expose the intentional cross-feature
entrypoints that maintained consumers compile through:

- `current-selection/public`: `CurrentSelectionWidget`, selection types,
  current-selection hooks, and selected provider-session state.
- `header/public`: dashboard export dialog, dashboard header, and dashboard
  status panel.
- `workflow-activity/public`: mutation dialog, workflow activity widget, and
  the current-activity import controller hook.
- `current-factory-definition/public`: current-factory-definition hook and
  public current-factory-definition API types.
- `flowchart/public`: activity node components, semantic graph icon, and
  workstation icon metadata.
- `import/public`: import preview dialog and import activation, preview, and
  PNG-drop hooks.
- `provider-session-detail/public`: provider session widget.

## Behavior Preservation

The implementation only changes TypeScript import paths and removes redundant
forwarding modules. Store state shape, selector behavior, message content,
timeline replay helpers, PNG import parsing, export/import dialog behavior,
provider-session rendering, workflow activity cards, dashboard header behavior,
and current-selection rendering remain owned by the same direct modules as
before.

No browser-visible loading, empty, error, success, accessibility, keyboard, or
responsive behavior changed in this batch. Direct browser verification was
therefore not required; the reviewer evidence is the focused compile, lint, and
component/app test surface for the affected import seams.

## Verification

Run from `ui/`:

- `bun run tsc`
- `bun run lint`
- `bun run test:unit -- --run src/i18n/messages.test.ts src/features/terminal-work/components/terminal-work-card.test.tsx src/features/terminal-work/components/terminal-work-widget.test.tsx src/App.follow-up-trace.test.tsx src/App.replay-workstation-requests.test.tsx src/features/trace-drilldown/components/trace-grid-card.replay.test.tsx src/features/import/lib/factory-png-import.test.ts src/features/import/hooks/use-factory-png-drop.test.tsx src/features/current-factory-definition/lib/workstation-editable-values.test.ts src/features/provider-session-detail/lib/provider-session-ref.test.ts src/features/workflow-activity/components/react-flow-current-activity-card-import.test.tsx src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx src/features/bento/components/dashboard-bento.test.tsx`

## Notes

- No compatibility shim, alias, replacement barrel, or alternate public
  re-export path was added for the retired exports.
- The verification avoids repository-wide barrel inventory tests; it uses the
  affected maintained consumers and behavior-level component or app tests
  instead.
