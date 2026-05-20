# Manual QA

This document records the repo-local browser verification flow for dashboard UI changes and the latest execution evidence.

## Browser Verification Flow

Run these commands from the repository root unless the step says otherwise.

1. `cd ui && bun install`
2. `cd ui && bun run build-storybook`
3. `cd ui && bun run test-storybook`
4. `cd ui && bun run build`
5. `cd ui && bun run preview`

After the preview server starts, open `http://127.0.0.1:4173` in a browser and spot-check the changed dashboard flows at these viewport widths:

- Mobile: `390x844`
- Tablet: `768x1024`
- Desktop: `1440x900`

## Dashboard UI Checklist

Use this checklist for the shadcn primitive migration lane and similar dashboard-control changes.

- Submit work card: request name, request text, and work type stay labeled, keyboard-focusable, and preserve disabled and busy states.
- Export PNG flow: export trigger opens the dialog, validation text renders when input is invalid, export actions keep disabled and busy states, and a successful download leaves a visible success acknowledgment before dismissal.
- Import preview flow: preview image, dropped filename, cancel, activate, and close controls stay keyboard-reachable, and the dialog remains readable at mobile, tablet, and desktop viewport widths.
- Completed and failed work card: expand and collapse controls remain keyboard-operable and selected work rows still update the current-selection panel.
- Selected-work dispatch history: dispatch cards keep the work-facing title and `Started at` summary visible, omit request-count summary rows, and keep inference or script attempt details collapsed until expanded on both narrow and wide layouts.
- Trace drill-down card: selectable work-item controls still update the trace detail surface and dispatch grid.
- Work outcome chart: loading, empty, error, and ready states render explicitly, and sparse series do not appear as fabricated zero-value lines.
- Trace dispatch grid: shared table and skeleton states render without layout breakage on narrow and wide viewports.
- Dashboard header: icon-only branding keeps the accessible `Infinite You` name, the timeline slider cluster stays keyboard-operable, and the header toolbar remains unclipped at mobile, tablet, and desktop widths with desktop controls ordered brand -> slider -> stream status -> export action.
- Dashboard shared shell: the header and representative grid-card shell use the same computed border, radius, background, and shadow at mobile, tablet, and desktop widths while header and card controls keep their accessible names.

## Latest Evidence

Date: `2026-05-20`

- `cd ui && bun run build-storybook` passed.
- `cd ui && bun run test-storybook` passed in a browser-backed runner, including the selected-work dispatch history smoke story that proves compact summary fields, collapsed-by-default inference and script attempt disclosures, and successful expansion of both attempt paths.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6010 bun run test-storybook` passed in a browser-backed runner, including tagged header stories for icon-only branding, slider alignment, keyboard-driven return/export actions, and the dedicated responsive verification script.
- `cd ui && bun run build` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6011 node scripts/verify-import-export-storybook-responsive.mjs` passed against built Storybook `iframe.html` stories for `ExportFactoryDialog`, `DashboardImportPreviewDialog`, the dashboard header verification story, and the shared shell verification story, confirming mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) dialog/header/card bounds, visible controls, keyboard timeline interactions, desktop toolbar ordering, matching computed header/card shell styles, and no horizontal overflow in headless Chromium.
- `cd ui && bun run tsc` passed.
