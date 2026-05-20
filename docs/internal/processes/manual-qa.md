# Manual QA

This document records the repo-local browser verification flow for dashboard UI changes and the latest execution evidence.

## Browser Verification Flow

Run these commands from the repository root unless the step says otherwise.

1. `cd ui && bun install`
2. `cd ui && bun run build-storybook`
3. `cd ui && bun run test-storybook`
4. `cd ui && bun run storybook:responsive-check`
5. `cd ui && bun run build`
6. `cd ui && bun run preview`

`storybook:responsive-check` is self-contained against `storybook-static`: it reuses an already-running Storybook server on `127.0.0.1:6008` when present, or starts and stops a temporary static server automatically after `build-storybook`.

After the preview server starts, open `http://127.0.0.1:4173` in a browser and spot-check the changed dashboard flows at these viewport widths:

- Mobile: `390x844`
- Tablet: `768x1024`
- Desktop: `1440x900`

## Dashboard UI Checklist

Use this checklist for the shadcn primitive migration lane and similar dashboard-control changes.

- Submit work card: request name, request text, and work type stay labeled, keyboard-focusable, and preserve disabled and busy states.
- Factory graph add flow: work type creation clearly asks for the first ordered state, work state creation clearly asks for its parent work type and state type, and field-level validation stays visible after failed submits.
- Factory graph save/discard flow: pending graph changes expose explicit save and discard actions, save opens a confirmation summary of created/deleted entities and changed edges, stale drafts show a newer-topology warning, and active work keeps save visibly blocked with a clear explanation.
- Factory graph connect flow: workstation transition anchors stay labeled as Success, Continue, Failure, and Reject, compatible state anchors remain keyboard-reachable in connect mode, and incompatible anchor pairings surface an explicit recovery notice instead of silently failing.
- Factory graph edge-delete flow: removing a route or resource/worker edge requires an explicit confirmation that names the affected route or availability link, and pending added or removed edges stay visually distinct before save.
- Factory graph removal flow: the delete tool exposes a destructive confirmation that names the entity and impact summary, ineligible removals show an explicit warning notice, and pending removals stay visibly marked in the graph before save.
- Export PNG flow: export trigger opens the dialog, validation text renders when input is invalid, export actions keep disabled and busy states, and a successful download leaves a visible success acknowledgment before dismissal.
- Import preview flow: preview image, dropped filename, cancel, activate, and close controls stay keyboard-reachable, and the dialog remains readable at mobile, tablet, and desktop viewport widths.
- Completed and failed work card: expand and collapse controls remain keyboard-operable and selected work rows still update the current-selection panel.
- Selected-work dispatch history: dispatch cards keep the work-facing title and `Started at` summary visible, omit request-count summary rows, and keep inference or script attempt details collapsed until expanded on both narrow and wide layouts.
- Trace drill-down card: selectable work-item controls still update the trace detail surface and dispatch grid.
- Work outcome chart: loading, empty, error, and ready states render explicitly, and sparse series do not appear as fabricated zero-value lines.
- Trace dispatch grid: shared table and skeleton states render without layout breakage on narrow and wide viewports.
- Dashboard header: icon-only branding keeps the accessible `Infinite You` name, the timeline slider cluster stays keyboard-operable, and the header toolbar remains unclipped at mobile, tablet, and desktop widths with desktop controls ordered brand -> slider -> stream status -> export action.
- Dashboard shared shell: the header and representative grid-card shell use the same computed border, radius, background, and shadow at mobile, tablet, and desktop widths while header and card controls keep their accessible names.
- Current-selection prompt editor: the prompt variable help toggle stays keyboard-operable, available-variable examples remain readable in-context, invalid template references render inline squiggle diagnostics, and the editor stays unclipped at mobile, tablet, and desktop widths.

## Latest Evidence

Date: `2026-05-19`

- `cd ui && bun run build-storybook` passed.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-controls.stories.tsx src/features/factory-graph-editor/factory-graph-editor-flow.stories.tsx` passed in a browser-backed runner, covering destructive confirmation, blocked-removal notice copy, and pending-removal graph rendering for the factory graph editor removal flow.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-controls.stories.tsx` passed in a browser-backed runner, covering pending save/discard actions, save confirmation summary copy, active-work blocking, and stale-definition warning states for the factory graph editor.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-flow.stories.tsx` passed in a browser-backed runner, covering distinct pending-added and pending-removed edge styling in the focused factory graph editor flow story.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-flow.stories.tsx` passed in a browser-backed runner, covering labeled Success/Continue/Failure/Reject workstation anchors and keyboard-reachable connection-source selection in the focused graph-editor flow story.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-add-dialog.stories.tsx` passed in a browser-backed runner, covering the differentiated work-type and work-state add dialogs plus field-level work-state validation copy.
- `cd ui && bun run build` passed.
- `cd ui && bun run storybook:responsive-check` passed against built Storybook `iframe.html` stories for `ExportFactoryDialog`, `DashboardImportPreviewDialog`, and the focused `DashboardHeader` verification story, confirming mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) dialog/header bounds, visible controls, keyboard timeline interactions, desktop toolbar ordering, and no horizontal overflow in headless Chromium.
- `make test` passed from the repository root while the refreshed Storybook wrapper waited for the built index to restabilize before launching the responsive browser check.

Date: `2026-05-20`

- `cd ui && bun run build-storybook` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6013 bun x vitest run --config vitest.storybook.config.ts --project=storybook src/App.workstation-requests.stories.tsx` passed in a browser-backed runner, confirming current-selection request and response timestamps render as browser-local date/time text instead of raw UTC ISO strings, while the no-response and errored workstation-request stories keep explicit empty and error messaging visible.
- `cd ui && bun run test-storybook` passed in a browser-backed runner, including the selected-work dispatch history smoke story that proves compact summary fields, collapsed-by-default inference and script attempt disclosures, and successful expansion of both attempt paths.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6012 bun run test-storybook` passed in a browser-backed runner, including the submit-work `StableActionAlignment` story that proves the primary button keeps the same measured right edge across ready, submitting, success, error, and wrapped validation states.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6010 bun run test-storybook` passed in a browser-backed runner, including tagged header stories for icon-only branding, slider alignment, keyboard-driven return/export actions, and the dedicated responsive verification script.
- `cd ui && bun run build` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6011 node scripts/verify-import-export-storybook-responsive.mjs` passed against built Storybook `iframe.html` stories for `ExportFactoryDialog`, `DashboardImportPreviewDialog`, the dashboard header verification story, and the shared shell verification story, confirming mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) dialog/header/card bounds, visible controls, keyboard timeline interactions, desktop toolbar ordering, matching computed header/card shell styles, and no horizontal overflow in headless Chromium.
- `cd ui && bun run tsc` passed.
- `cd ui && bunx vitest run src/features/current-selection/current-selection-widget.save.test.tsx scripts/verify-import-export-storybook-responsive.schedule.test.mjs scripts/verify-import-export-storybook-responsive.test.mjs scripts/dashboard-shell-storybook-responsive.test.mjs` passed.
- `cd ui && bun run build-storybook` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6012 bun run test-storybook` passed, including the new current-selection prompt hinting Storybook interaction and responsive browser verification for prompt help, inline squiggle diagnostics, keyboard interaction, and mobile/tablet/desktop overflow checks.
