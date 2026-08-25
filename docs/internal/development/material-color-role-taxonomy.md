# Material-inspired color role taxonomy

---
doc-id: DEV-MATERIAL-COLOR-ROLES
---

Canonical reference for the dashboard UI Material-style color roles introduced in the material color role migration PRD.

## Source of truth

- **Role tokens (long-term API):** `ui/packages/components/src/styles/color-role-tokens.css` — use Tailwind utilities such as `bg-primary`, `text-on-surface`, `border-outline`.
- **Product `--color-af-*` keys:** Role-backed mappings live in `ui/packages/components/src/styles/color-role-tokens.css`; foundation palette keys and unrelated presentation aliases remain in `ui/src/styles.css`. The chart, edge, graph-control/focus, and overlay follow-up has completed: consumed aliases resolve through Material roles, while six zero-consumer aliases were retired.
- **Factory palette keys:** `af-foundation-*` in `ui/src/styles.css` — baseline for Factory Dark; palette presets in `ui/src/styles/color-palette-presets.css` override these keys at runtime (US-008).
- **Rollout & regression (US-010):** [material-color-role-migration-rollout.md](./material-color-role-migration-rollout.md) — phased rollout, regression matrix, and completed alias removal.

## Role families

### Neutral (surfaces and content on surfaces)

| Role | Tailwind utility examples | Intent |
| --- | --- | --- |
| `background` | `bg-background` | App/page backdrop |
| `surface` | `bg-surface` | Default component surface |
| `surface-container-low` | `bg-surface-container-low` | Lowest elevation container |
| `surface-container` | `bg-surface-container` | Standard elevated panel |
| `surface-container-high` | `bg-surface-container-high` | Raised cards, toolbars |
| `surface-container-highest` | `bg-surface-container-highest` | Highest emphasis containers |
| `on-surface` | `text-on-surface` | Primary text/icons on surfaces |
| `on-surface-variant` | `text-on-surface-variant` | Secondary/muted text |
| `outline` | `border-outline` | Default borders and dividers |
| `outline-variant` | `border-outline-variant` | Subtle borders |

### Accent (brand emphasis — not status)

Each accent family exposes `role`, `on-role`, `role-container`, and `on-role-container` tokens (`primary` → `--color-primary`, `--color-on-primary`, `--color-primary-container`, `--color-on-primary-container`).

| Family | Hue family | Notes |
| --- | --- | --- |
| `primary` | Yellow (`#f5c76f`) | Brand accent; most visually prominent accent |
| `secondary` | Cyan (`af-foundation-secondary-accent`) | Supporting accent; calmer than semantic `info` |
| `tertiary` | Violet (`af-foundation-tertiary-accent`) | Supporting accent; calmer than legacy `af-foundation-worker` chrome |

**Do not** use `warning` for ordinary brand emphasis. **Do not** use semantic greens/blues/reds for non-status UI chrome.

### Semantic (status only)

Separate from accent roles. Use only when communicating outcome or state meaning.

| Family | Maps from legacy | Use for |
| --- | --- | --- |
| `success` | `af-success` / foundation success | Positive completion, live/healthy state |
| `warning` | `af-warning` / foundation warning | Caution, attention-needed (not brand accent) |
| `error` | `af-danger` / foundation danger | Destructive actions, failures |
| `info` | `af-info` | Informational callouts (not default secondary emphasis) |

Each semantic family includes `on-*` and `*-container` / `on-*-container` pairs matching accent structure.

### Shared primitives — neutral surfaces (US-005)

Shared primitives in `ui/src/components/ui/` use role utilities directly for neutral chrome:

| Transitional `af-*` | Role utility | Typical use |
| --- | --- | --- |
| `af-background` | `bg-background` | Page/shell backdrop (fixtures) |
| `af-surface-subtle` | `bg-surface-container-low` | Tables, charts, empty states, selected list rows |
| `af-surface-raised` | `bg-surface-container-high` | Cards, inputs, dialogs, popovers |
| `af-border` | `border-outline` | Default borders and row dividers |
| `af-border-strong` | `border-outline-variant` | Stronger borders, selected outlines |
| `af-text` | `text-on-surface` | Primary text on surfaces |
| `af-text-muted` | `text-on-surface-variant` | Secondary text |

Dashboard typography classes (`af-dashboard-*` in `styles.css`) map to the Material scale and text color roles documented in [material-typography-role-taxonomy.md](./material-typography-role-taxonomy.md). Visual review: Storybook `Agent Factory/UI/Theme Role Migration Overview` (consolidated), `Typography Role Hierarchy`, and `Color Role Neutral Surfaces`.

### Shared primitives (US-004)

| Component | Accent / brand emphasis | Semantic tones |
| --- | --- | --- |
| `Button` (`tone="default"`) | `af-accent` → `primary` | `destructive` → `error` only |
| `DashboardStatusPill` | `active` → primary accent container | `success`, `warning`, `info`, `danger` for state meaning |
| `StandardListSelectionItem` | `accent` → primary accent surface | `success` / `danger` for outcome rows; selected rows stay neutral |

Do not use `warning` for draft/pending copy, `info` for brand row emphasis, or `success` for generic highlights.

## Implementation notes

1. **Yellow primary** — `primary` stays tied to `af-foundation-accent` so Factory Dark identity is preserved.
2. **Secondary and tertiary saturation (US-003)** — `secondary` / `tertiary` roles use `af-foundation-secondary-accent` and `af-foundation-tertiary-accent` (calmer than `af-foundation-info` / `af-foundation-worker`). Semantic `info` and chart/info chrome keep the vibrant `af-foundation-info` family. Visual review: Storybook `Agent Factory/UI/Color Role Accent Contrast`.
3. **Palette switching (US-008)** — Five predefined palettes (`factory-dark`, `factory-light`, `material-baseline`, `slate`, `olive`) override `af-foundation-*` keys via `data-color-palette` on `:root` (`ui/src/styles/color-palette-presets.css`). The dashboard header palette dropdown (`DashboardPaletteMenu`) persists the selection in `sessionStorage` for the current browser session. Role token names stay stable across palettes; yellow `#f5c76f` remains the primary brand accent.
4. **Migration order** — Taxonomy (this doc) → shared primitives → feature surfaces → alias layer removal (complete). See [material-color-role-migration-rollout.md](./material-color-role-migration-rollout.md) for regression tests, Storybook fixtures, and maintenance gates.

## Supported `--color-af-*` product keys

Role-backed product keys (for example `af-text`, `af-surface`, `af-accent`, semantic surfaces, chart series, graph edges, graph controls, focus, and overlays) are defined in `ui/packages/components/src/styles/color-role-tokens.css` and resolve to Material roles. Foundation keys (`af-foundation-*`) and unrelated presentation aliases remain in `ui/src/styles.css`. The retired zero-consumer keys are `--color-af-chart-grid-line`, `--color-af-chart-selection-fill`, `--color-af-chart-selection-stroke`, `--color-af-chart-cursor`, `--color-af-chart-active-dot-stroke`, and `--color-af-overlay-subtle`.

Wiring contract: `ui/src/styles/theme-role-regression.component.test.ts` resolves compiled role-backed variables, and `ui/src/features/work-outcome/components/work-chart/work-chart-color-role-behavior.component.test.tsx` verifies the four rendered chart series use palette-backed aliases.

## CSS variable reference

Neutral and accent/semantic roles register in Tailwind v4 `@theme` as `--color-<role-name>`, enabling utilities such as `bg-primary`, `text-on-surface`, and `border-outline-variant`.

### Feature and graph surfaces (US-009)

Dashboard feature modules under `ui/src/features/` consume role utilities directly for neutral chrome and accent emphasis. Semantic borders, overlays, shadows, chart series, and graph edges use their role-backed `af-*` aliases where consumed; the six zero-consumer aliases listed above were retired.

| Area | Role usage |
| --- | --- |
| Header, session tabs, palette menu | `bg-surface-container-*`, `border-outline`, `text-on-surface`, `border-primary` for selected menu rows |
| Flowchart / factory graph nodes | `bg-surface`, `border-outline`, accent `border-primary` / `bg-primary-container`; semantic borders for phase/state meaning |
| Trace drilldown graph nodes | Same pattern; `info` / `success` / `warning` / `error` containers for dispatch and outcome chrome |
| Charts (`work-outcome`) | Series colors stay on `af-chart-*` keys; grid/label utilities use `stroke-outline-variant`, `fill-on-surface-subtle` |
| Mutation dialog | `gap-layout-block`; role surfaces for shell and message panels |

Enforcement contract: `ui/src/features/feature-surface-color-roles.test.ts` — feature surface color roles must pass this test; fix violations with targeted manual edits per [material-color-role-migration-rollout.md](./material-color-role-migration-rollout.md) cleanup steps.

## Related layout system (US-007)

Layout spacing roles and shared primitives are documented in `material-layout-role-taxonomy.md`. Storybook: `Agent Factory/UI/Layout Role Primitives`.
