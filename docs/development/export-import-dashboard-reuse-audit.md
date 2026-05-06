---
author: ralph agent
last modified: 2026, april, 25
doc-id: AGF-DEV-006
---

# Export And Import Dashboard Reuse Audit

This audit records the current Agent Factory PNG export and import dashboard surface before the refactor stories replace bespoke UI structure with the existing dashboard primitives, hooks, and typed API paths.

Use [Dashboard Mutation Flow Pattern](dashboard-mutation-flow-pattern.md) as the canonical follow-on implementation guide. This audit stays focused on the export/import-specific gap analysis that justified that shared pattern.

## Scope

- In scope: `libraries/agent-factory/ui` export and import dashboard surfaces, their local hooks, and the UI-owned contract helpers they currently depend on.
- Out of scope: backend named-factory semantics, replay behavior, runtime watcher behavior, or new customer-facing dashboard features.

## Reuse Candidates Reviewed

### Shared dashboard primitives

- `ui/src/components/dashboard/bento.tsx`
- `ui/src/components/dashboard/button.tsx`
- `ui/src/components/dashboard/widget-board.tsx`
- `ui/src/components/ui/dashboard-typography.ts`

### Existing widget and hook patterns

- `ui/src/features/submit-work/submit-work-card.tsx`
- `ui/src/features/submit-work/use-submit-work-widget.ts`
- `ui/src/features/workflow-activity/workflow-activity-bento-card.tsx`
- `ui/src/hooks/dashboard/useDashboardLayout.ts`

### Typed UI API paths already in use

- `ui/src/api/work/api.ts`
- `ui/src/api/named-factory/api.ts`
- `ui/src/features/import/use-factory-import-activation.ts`
- `ui/src/features/import/use-factory-import-preview.ts`
- `ui/src/features/import/use-factory-png-drop.ts`

## Audit Findings

| Current bespoke surface | Current owner | Reuse target | Follow-on story |
| --- | --- | --- | --- |
| Toolbar export CTA uses a local button class bundle and inline icon in `App.tsx`. | `ui/src/App.tsx` | Reuse `DashboardButton` tone and dashboard toolbar composition instead of another feature-local CTA shell. | US-002 |
| Export dialog owns its own backdrop, panel, close button, form controls, validation copy, and action buttons. | `ui/src/features/export/export-factory-dialog.tsx` | Reuse dashboard typography tokens plus `DashboardButton`; if a reusable dialog shell is needed, extract one instead of keeping another local panel class bundle. | US-002 |
| Import preview dialog inside the graph card owns another standalone overlay, metadata panel, status panel, and action row. | `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx` | Reuse the same dashboard dialog/action/status primitives chosen for export so import and export follow one mutation-dialog pattern. | US-002 |
| Import error and activation error feedback use feature-local alert shells. | `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx` | Reuse `EMPTY_STATE_CLASS` and the existing dashboard status typography, or extract one shared mutation-feedback panel if the empty-state shell is not sufficient. | US-002 |
| Export preparation reconstructs and canonicalizes the factory contract through a large local helper that scans timeline events and normalizes aliases. | `ui/src/features/export/current-factory-export.ts` | Move request/response and contract-shape ownership toward a typed UI helper/API boundary, matching the existing `submitWork` and `createNamedFactory` API paths instead of keeping ad hoc contract knowledge in UI composition. | US-003 |
| Import activation already routes the POST through a dedicated typed API wrapper and hook. | `ui/src/api/named-factory/api.ts`, `ui/src/features/import/use-factory-import-activation.ts` | Preserve this path as the canonical activation mutation seam; refactor composition around it instead of bypassing it. | Preserve in US-002 and US-003 |
| Import file reading and preview lifecycle already live in focused hooks instead of the graph card body. | `ui/src/features/import/*` | Preserve this hook split and use it as the canonical mutation-flow pattern for export orchestration work. | Preserve in US-002 and US-003 |
| Submit-work already demonstrates the intended dashboard mutation pattern: widget shell, focused hook, typed API wrapper, shared button, shared widget frame, and explicit status model. | `ui/src/features/submit-work/*` | Treat submit-work as the nearest existing dashboard mutation reference when refactoring export/import composition. | US-002, US-003, US-005 |

## Intentional Exceptions And Gaps

### No shared dashboard dialog primitive exists yet

The current dashboard package has reusable bento cards, widget frames, buttons, typography roles, and empty-state shells, but it does not yet have a canonical modal or dialog primitive. The refactor should not silently preserve two separate local dialog shells. It should either:

1. Reuse one extracted dashboard mutation-dialog shell for both export and import.
2. Keep one temporary local shell only if the audit is cited and the remaining gap is explicit.

### Export contract handling is thinner than import activation, but not yet shared

`submitWork` and named-factory activation already use typed UI API wrappers. Export preparation still reconstructs the current factory through local event scanning and alias normalization in UI feature code. That exception is intentional for the current shipped path, but it is the primary contract-drift target for `US-003`.

## Selected Reuse Path For Follow-On Stories

- Reuse `DashboardButton` for export and import primary and secondary actions.
- Reuse dashboard typography and widget-board status classes for feedback copy.
- Keep import file parsing, preview lifecycle, and activation orchestration in focused hooks.
- Model export orchestration after the `submit-work` split between presentational card/dialog code and a focused mutation/helper seam.
- Keep the import entry attached to the existing workflow graph surface instead of creating a parallel dashboard card or standalone page.

## Non-Goals Confirmed By Audit

- No backend contract redesign is required for this refactor map.
- No replay or event-stream redesign is required for this refactor map.
- No new customer-visible mutation capabilities are required beyond the existing export and import flows.
