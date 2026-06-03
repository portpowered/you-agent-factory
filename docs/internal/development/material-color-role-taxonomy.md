# Material-inspired color role taxonomy

---
doc-id: DEV-MATERIAL-COLOR-ROLES
---

Canonical reference for the dashboard UI Material-style color roles introduced in the material color role migration PRD.

## Source of truth

- **Role tokens (long-term API):** `ui/src/styles/color-role-tokens.css` — use Tailwind utilities such as `bg-primary`, `text-on-surface`, `border-outline`.
- **Transitional `af-*` aliases:** `ui/src/styles/color-role-aliases.css` — maps widely used `af-*` product tokens to roles so existing UI keeps rendering while components migrate.
- **Factory palette keys:** `af-foundation-*` in `ui/src/styles.css` — baseline for Factory Dark until palette switching (US-008).
- **Cleanup (US-010):** remove `color-role-aliases.css` and obsolete `af-*` after consumers use role utilities directly.

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

## Transitional `af-*` alias map (US-002)

| Transitional token | Role source of truth | Notes |
| --- | --- | --- |
| `af-background` | `background` | Page/shell backdrop |
| `af-surface` | `surface` | Default component surface |
| `af-surface-subtle` | `surface-container-low` | Low-emphasis panels |
| `af-surface-raised` | `surface-container-high` | Raised cards, graph controls |
| `af-border` | `outline` | Default borders |
| `af-border-strong` | `outline-variant` | Stronger borders |
| `af-text` | `on-surface` | Primary text |
| `af-text-muted` | `on-surface-variant` | Secondary text |
| `af-accent` | `primary` | Brand emphasis |
| `af-accent-hover` | `on-primary-container` | Strong accent ink |
| `af-accent-surface` | `primary-container` | Accent fill |
| `af-accent-border` | `primary` | Accent stroke |
| `af-on-accent` | `on-primary` | Text/icons on accent |
| `af-success` / `af-success-surface` | `success` / `success-container` | Status only |
| `af-warning` / `af-warning-surface` | `warning` / `warning-container` | Status only |
| `af-danger` / `af-danger-surface` | `error` / `error-container` | Status only |
| `af-info` / `af-info-surface` | `info` / `info-container` | Status only |
| `af-worker` / `af-worker-surface` | `tertiary` / `tertiary-container` | Supporting accent |

Tokens without a direct role yet (for example `af-text-subtle`, `af-overlay`, semantic `*-border` opacities, chart keys) remain defined in `ui/src/styles.css` until a later story adds roles or consumers migrate.

## CSS variable reference

Neutral and accent/semantic roles register in Tailwind v4 `@theme` as `--color-<role-name>`, enabling utilities such as `bg-primary`, `text-on-surface`, and `border-outline-variant`.
