---
author: ralph agent
last modified: 2026, april, 25
doc-id: AGF-DEV-007
---

# Dashboard Mutation Flow Pattern

This document is the canonical reuse guide for Agent Factory dashboard mutations such as submit-work, export, and import. Use it when a dashboard feature needs a customer-triggered mutation, confirmation surface, or inline status feedback without introducing feature-local shells or contract parsing.

## Use This Pattern When

- The feature starts from an existing dashboard card, toolbar action, or graph-surface entry point.
- The customer action triggers a typed API mutation, file-derived mutation, or another request that can succeed, fail, or require confirmation.
- The feature needs dashboard-consistent cards, dialogs, buttons, or feedback states.

## Keep A Different Pattern When

- The work is display-only and does not perform a customer-triggered mutation.
- The flow needs a broader page or wizard structure that cannot fit the existing dashboard card-plus-dialog composition.
- The backend contract or workflow semantics are still unsettled enough that a typed UI helper would be premature.

## Required Reuse Path

### 1. Start from the nearest existing dashboard composition

- Reuse the existing dashboard card, toolbar, graph card, or widget shell instead of adding a parallel page or bespoke panel tree.
- Treat `ui/src/features/submit-work` as the nearest reference when the mutation starts from a dashboard widget.
- Treat `docs/development/export-import-dashboard-reuse-audit.md` as the bounded audit reference for export and import follow-on work.

### 2. Keep shell and mutation logic split

- Keep the presentational component focused on fields, preview metadata, and action wiring.
- Move mutation lifecycle state, cancellation, and request orchestration into a focused hook when the flow needs more than trivial local state.
- Keep file parsing or preview lifecycle in feature hooks instead of pushing those responsibilities back into dashboard shell components.

### 3. Reuse the shared dashboard primitives

- Use `ui/src/components/dashboard/button.tsx` for customer-facing actions instead of feature-local CTA classes.
- Use `ui/src/components/dashboard/mutation-dialog.tsx` when the mutation needs modal confirmation, preview, or final acknowledgment.
- Keep reusable dialog accessibility wiring instance-safe; shared mutation dialogs should derive `aria-labelledby` and `aria-describedby` ids per render instead of hard-coding DOM ids that can collide when two mutation surfaces coexist.
- Use dashboard typography helpers and existing bento or widget shells before adding local `text-[...]`, spacing, or container bundles.
- Use `DashboardMessagePanel` for inline mutation success, error, and empty-or-status feedback instead of rebuilding alert shells.

### 4. Route contract handling through typed UI helpers

- Put request construction, response normalization, and generated-contract validation in `ui/src/api/...` helpers when more than one component or hook depends on that boundary.
- Keep feature components and hooks thin callers over those typed helpers.
- Do not duplicate contract-shape knowledge in dialog components, card components, or local formatting utilities when the same boundary already exists in the typed UI API layer.

### 5. Verify the flow at the dashboard seam

- Add focused App-level, component-level, or hook-level coverage that proves at least one success path and one retryable or failure path.
- Prefer the real typed helper path plus fetch mocks when the goal is to prove request or response handling.
- Rebuild committed dashboard assets when source changes affect the embedded UI shell.

## Canonical Building Blocks

| Need | Preferred reuse target | Notes |
| --- | --- | --- |
| Widget or toolbar action | `DashboardButton` plus existing dashboard card or toolbar composition | Keep loading labels and disabled states at the feature boundary. |
| Confirmation or preview modal | `ui/src/components/dashboard/mutation-dialog.tsx` | Export and import should share this shell unless the audit documents a temporary exception. |
| Inline feedback | `DashboardMessagePanel` | Reuse dashboard status treatment for success, error, and retryable feedback. |
| Mutation state and orchestration | Feature-local hook under `ui/src/features/<feature>/` | Keep feature-specific parsing or preview ownership here. |
| Request or response boundary | Typed UI API helper under `ui/src/api/` | Shared contract normalization belongs here, not in feature components. |
| Regression proof | Focused Vitest or App-level dashboard smoke | Prove the typed helper path, not just isolated markup. |

## Current Reference Implementations

- `ui/src/features/submit-work/` shows the widget-plus-hook split for a dashboard mutation.
- `ui/src/features/export/` shows dialog composition over shared dashboard controls and shared factory-definition normalization.
- `ui/src/features/import/` shows hook-owned PNG parsing and preview lifecycle paired with the shared mutation dialog.

## Related Docs

- [Agent Factory Development Guide](development.md)
- [Export And Import Dashboard Reuse Audit](export-import-dashboard-reuse-audit.md)
- [Agent Factory Development Process](../../../docs/processes/agent-factory-development.md)
