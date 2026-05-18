---
author: ralph agent
last modified: 2026, may, 18
doc-id: AGF-DEV-007
---

# Website Localizability Migration Inventory

This inventory records the React website copy surfaces that need localizability coverage for English `en` and Simplified Mandarin Chinese for China `zh-CN`. It follows the internationalization guidance in [General Website Standards](../standards/code/general-website-standards.md).

## Scope Boundary

Product-authored copy is in scope when the website owns the words shown to users or assistive technology. That includes visible text, aria labels, title text, button labels, form labels, empty states, error states, validation messages, dialog copy, table labels, chart labels, tooltips, status text, and metadata.

Data values are out of scope for translation. API-provided IDs, dispatch IDs, trace IDs, workstation names, place names, user-authored factory names, generated OpenAPI code, test fixture names, operational event payload values, and developer diagnostics should remain unchanged when the locale changes. Product-owned labels that introduce those values should still be localized.

## Current Catalog Coverage

| Surface | Current owner | Catalog status | Notes |
| --- | --- | --- | --- |
| Locale policy and shared helpers | `ui/src/i18n/` | Catalog infrastructure exists | `locales.ts`, `messages.ts`, `formatters.ts`, and `app-locale.tsx` own supported locale resolution, the session-over-URL-over-browser source order, required-locale catalog validation, fallback behavior, and shared `Intl` formatting helpers. |
| Dashboard shell and header controls | `ui/src/features/header/messages/header-controls.ts` | Required-locale catalog coverage exists | Brand lockup text, top-level dashboard loading/error states, header summary labels, stream status accessible names, timeline slider copy, tick status templates, timeline controls, and the session-scoped header language switcher all resolve through the header catalog. Dashboard shell coverage now includes English and Mandarin component tests plus Storybook browser verification for both localized header controls and live app-level locale propagation without a full page reload. |
| Export dialog | `ui/src/features/export/messages/export-dialog.ts` | Required-locale catalog coverage exists | Export dialog title, description, form labels, helper text, validation copy, action labels, loading status, success copy, trigger label, and accessible close label resolve through a feature catalog, with component and Storybook browser coverage proving English and `zh-CN` dialog readability. |
| Import preview dialog | `ui/src/features/import/messages/import-preview-dialog.ts` | Required-locale catalog coverage exists | Import preview title, description template, labels, activation actions, activation error title, mapped activation errors, preview alt text, and accessible close label resolve through a feature catalog, with component and Storybook browser coverage proving English and `zh-CN` dialog readability. |
| Workflow activity widget shell | `ui/src/features/workflow-activity/messages/` | Required-locale catalog coverage exists | Graph legend labels, icon labels, import/drop overlay copy, validation errors, dismiss/reset labels, preview dialog shell copy, graph shell eyebrow/title, empty topology state, viewport region label, and widget title resolve through workflow-activity catalogs with English and `zh-CN` component coverage. |
| Current selection details | `ui/src/features/current-selection/messages/` and `ui/src/features/current-selection/current-selection-locale.tsx` | Required-locale catalog coverage exists | Shell headings, trace guidance, undo/redo labels, execution details, dispatch history, terminal summary, workstation detail headings, statuses, and fallback copy have feature-local catalogs and a feature-local provider. The dashboard now forwards locale into this seam, so selected-work and workstation cards resolve the localized shell in the main app. |
| Terminal work widget | `ui/src/features/terminal-work/messages/terminal-work.ts` | Required-locale catalog coverage exists | Terminal work card title, legend, row titles, icon labels, disclosure labels, empty states, item-count labels, and session-summary fallbacks resolve through a feature catalog and are now wired through the dashboard locale seam. |
| Work totals, work outcome, trace drilldown, and submit work widgets | `ui/src/features/{work-totals,work-outcome,trace-drilldown,submit-work}/messages/` | Required-locale catalog coverage exists | Widget titles, count labels, chart labels, trend copy, trace region labels, empty/loading/error states, form labels, placeholders, validation copy, action labels, and success/error states resolve through feature-owned English and `zh-CN` catalogs with focused component coverage plus Storybook browser verification at mobile, tablet, and desktop widths. |

## Migration Targets

| Feature | User-facing copy surfaces to migrate or verify | Current examples |
| --- | --- | --- |
| Dashboard shell | Visible shell loading/error text, header copy, brand lockup text, stream status, timeline controls, tick controls, dashboard summary labels, primary dashboard actions, accessible names, and metadata where present. | `ui/src/features/dashboard/dashboard-screen.tsx`, `ui/src/features/header/`, and `ui/src/i18n/app-locale.tsx` now resolve shell-owned copy through feature catalogs and an app-level locale provider. Static metadata, `ui/index.html`, and `ui/fallback_dist/` remain follow-on work. |
| Import/export workflows | Import preview dialog copy, activation error copy, loading states, validation states, action labels, accessible names, export filename helper text, success states, error states, and trigger labels. | `ui/src/features/import/`, `ui/src/features/export/`, `ui/src/features/header/dashboard-export-dialog.tsx`, `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx`. |
| Work totals and terminal work | Widget titles, count labels, legends, empty states, loading states, error states, table labels, disclosure labels, tooltip or title text, and accessible names. Counts should use shared locale-aware formatters or localized message functions. | `ui/src/features/work-totals/`, `ui/src/features/terminal-work/`. Work totals and terminal work now route through localized widget copy; future review should focus on any new data-visualization surfaces rather than launch-locale browser proof for the current cards. |
| Work outcome charts | Chart titles, card titles, axis labels, legend labels, time-range labels, empty/loading/error states, failure summaries, trend card labels, tooltip/title text, and chart accessible names. Chart data values and API-derived workstation or work names should remain data. | `ui/src/features/work-outcome/`. Widget-owned chart and trend copy now resolve through feature catalogs and are browser-verified in `zh-CN` across mobile, tablet, and desktop widths. |
| Workflow activity and flowchart | Activity widget titles, graph labels, legend labels, icon accessible names, import overlay states, graph empty/error/loading states, node category labels, edge labels, and dialog copy. Workstation names, place names, and event payload text should remain data values. | `ui/src/features/workflow-activity/`, `ui/src/features/flowchart/`. Graph shell copy is now catalog-backed; flowchart node-internal labels should still be reviewed if new product-authored labels are introduced. |
| Current selection, trace, and drilldown | Current-selection headings, selected-work detail labels, trace drilldown titles, table headers, empty/error states, status labels, action labels, region labels, and accessible names. Trace IDs, dispatch IDs, request IDs, provider session IDs, prompts, response bodies, and workstation names should remain data. | `ui/src/features/current-selection/`, `ui/src/features/trace-drilldown/`. Current-selection and trace-drilldown copy now resolve through feature catalogs while IDs and authored work names remain unchanged, and the trace drilldown card now contains wide tables and React Flow visualizations inside the card without forcing page-level horizontal overflow at mobile widths. |
| Submit work | Card title, form labels, placeholder or helper copy, validation messages, action labels, loading/success/error states, and accessible names. User-authored submitted text should remain data. | `ui/src/features/submit-work/`. Submit-work shell and validation copy are now catalog-backed, browser-verified in `zh-CN`, and submitted request text remains data. |
| Shared dashboard primitives | Reusable labels owned by shared UI primitives, shared empty-state copy if present, dialog primitive copy if extracted, and icon accessible names that are not feature-specific. | `ui/src/components/ui/`, `ui/src/components/dashboard/`. |
| App metadata and fallback shell | Document title, static fallback shell copy, embedded dashboard fallback metadata, and any non-generated static UI copy that ships outside React components. | `ui/index.html`, `ui/fallback_dist/`. |

## Exclusions

- Generated OpenAPI artifacts under `ui/src/api/generated/` remain generated and should not receive handwritten localization edits.
- API response fields, event payload strings, factory/workstation/place names, dispatch IDs, trace IDs, and user-authored request or response text remain data values.
- Test fixture names and Storybook scenario names do not need product translation unless a story intentionally demonstrates localized UI.
- Developer-only diagnostics, console messages, and test helper names are not product copy unless they are rendered to users.

## Follow-On Review Notes

- Prefer feature-owned message packages under `ui/src/features/<feature>/messages/` for migrated product copy.
- Use `ui/src/i18n/formatters.ts` for date, time, number, count, percent, list, and relative-time formatting.
- Use full localized message functions or templates for dynamic labels instead of concatenating translated fragments around data values.
- Add rendered component or functional coverage for non-default locale behavior when migrating a user-facing surface.
