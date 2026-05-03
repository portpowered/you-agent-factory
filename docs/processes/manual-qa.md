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
- Completed and failed work card: expand and collapse controls remain keyboard-operable and selected work rows still update the current-selection panel.
- Trace drill-down card: selectable work-item controls still update the trace detail surface and dispatch grid.
- Work outcome chart: loading, empty, error, and ready states render explicitly, and sparse series do not appear as fabricated zero-value lines.
- Trace dispatch grid: shared table and skeleton states render without layout breakage on narrow and wide viewports.

## Latest Evidence

Date: `2026-05-01`

- `cd ui && bun run build-storybook` passed.
- `cd ui && bun run test-storybook` passed in a browser-backed runner.
- `cd ui && bun run build` passed.
- `cd ui && bun run preview` served the migrated dashboard locally for viewport spot checks.
