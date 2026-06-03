# Material-inspired color role taxonomy

---
doc-id: DEV-MATERIAL-COLOR-ROLES
---

Canonical reference for the dashboard UI Material-style color roles introduced in the material color role migration PRD.

## Source of truth

- Token definitions: `ui/src/styles/color-role-tokens.css` (imported from `ui/src/styles.css`)
- Factory Dark baseline values derive from `af-foundation-*` palette keys in `ui/src/styles.css`
- Transitional `af-*` product tokens remain until alias migration (PRD US-002) and cleanup (US-010)

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
| `secondary` | Cyan (`af-foundation-info`) | Supporting accent; moderate saturation (rebalanced in US-003) |
| `tertiary` | Violet (`af-foundation-worker`) | Supporting accent; moderate saturation (rebalanced in US-003) |

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

## Implementation notes

1. **Yellow primary** — `primary` stays tied to `af-foundation-accent` so Factory Dark identity is preserved.
2. **Secondary and tertiary saturation** — Initial tokens mirror current foundation info/worker hues. US-003 lowers vibrancy while keeping hue families distinguishable from neutrals and from each other.
3. **Palette switching (US-008)** — Future palette sets override the same role keys; role names stay stable across Factory Dark, Factory Light, Material Baseline, Slate, and Olive.
4. **Migration order** — Taxonomy (this doc) → `af-*` aliases → shared primitives → feature surfaces → alias cleanup.

## CSS variable reference

Neutral and accent/semantic roles register in Tailwind v4 `@theme` as `--color-<role-name>`, enabling utilities such as `bg-primary`, `text-on-surface`, and `border-outline-variant`.
