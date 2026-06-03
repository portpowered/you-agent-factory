# Material-inspired typography and text-color roles

---
doc-id: DEV-MATERIAL-TYPOGRAPHY-ROLES
---

Canonical reference for the dashboard Material 3 typography scale and text color roles (US-006).

## Source of truth

- **Typography scale:** `ui/src/styles/typography-role-tokens.css` — `--text-<family>-<variant>` tokens and Tailwind utilities such as `text-body-medium`, `type-title-large`.
- **Composed utilities:** `ui/src/styles/typography-role-utilities.css` — `type-*` classes pair scale + weight/tracking defaults.
- **Text color roles:** `ui/src/styles/text-color-role-tokens.css` — `text-on-surface`, `text-on-inverse`, `text-code`, etc.
- **Dashboard product mappings:** `af-dashboard-*` classes in `ui/src/styles.css` and constants in `ui/src/components/ui/dashboard-typography.ts`.
- **Color fill roles:** `ui/src/styles/color-role-tokens.css` — accent/semantic containers use matching `on-*-container` text utilities.

Visual review: Storybook `Agent Factory/UI/Typography Role Hierarchy`.

## Material typography families

Intent follows [Material 3 applying type](https://m3.material.io/styles/typography/applying-type):

| Family | Use for | Product variants in use |
| --- | --- | --- |
| `display` | Short hero or high-emphasis numeric moments | large, medium, small |
| `headline` | Prominent short-form headings | large, medium, small |
| `title` | Page, section, and subsection wayfinding | large, medium, small |
| `body` | Long-form reading text | large, medium, small |
| `label` | Interactive control text and compact annotations | large, medium, small |

### Product extension: `code`

Monospace copy is **not** a Material family. It is documented as a deliberate extension:

- Tokens: `--text-code-medium`, `--text-code-small`
- Utilities: `type-code-medium`, `type-code-small`
- Color: `text-code` (`--color-code` → foundation code ink)

Use `code` roles for transcripts, payloads, and CLI output — not for ordinary headings.

## Dashboard class map

| Dashboard class | Material scale | Text color role |
| --- | --- | --- |
| `af-dashboard-page-heading` | display / medium | `on-surface` |
| `af-dashboard-section-heading` | title / large | `on-surface` |
| `af-dashboard-body-text` | body / medium | `on-surface-variant` |
| `af-dashboard-supporting-text` | body / small | `on-surface-variant` |
| `af-dashboard-supporting-label` | label / medium | `on-surface-subtle` |
| `af-dashboard-body-code` | code / medium | `code` |
| `af-dashboard-supporting-code` | code / small | `code` |
| `af-dashboard-widget-subtitle` | display / small (mono) | `on-surface-variant` |

Prefer exported constants from `dashboard-typography.ts` (`DASHBOARD_PAGE_HEADING_CLASS`, etc.) in TSX instead of string literals.

## Text color roles

| Role | Utility | Intent |
| --- | --- | --- |
| `on-surface` | `text-on-surface` | Primary text on neutral surfaces |
| `on-surface-variant` | `text-on-surface-variant` | Secondary body and metadata |
| `on-surface-subtle` | `text-on-surface-subtle` | Tertiary labels and chart ticks |
| `on-surface-disabled` | `text-on-surface-disabled` | Disabled controls and placeholders |
| `on-inverse` | `text-on-inverse` | Text on saturated accent/semantic fills |
| `code` | `text-code` | Monospace body copy |

Accent and semantic **container** fills use the existing `on-primary-container`, `on-success-container`, etc. from `color-role-tokens.css`.

### Transitional `af-*` text aliases

| Transitional token | Role source |
| --- | --- |
| `af-text` | `on-surface` |
| `af-text-muted` | `on-surface-variant` |
| `af-text-subtle` | `on-surface-subtle` |
| `af-text-disabled` | `on-surface-disabled` |
| `af-text-inverse` | `on-inverse` |
| `af-code-ink` | `code` |

## Implementation notes

1. Import order in `styles.css`: color roles → text color roles → typography tokens → typography utilities → aliases.
2. New dashboard copy should use `DASHBOARD_*_CLASS` constants, or pair `type-*` / `text-<scale>-<variant>` with `text-on-*` utilities. Tailwind `@apply` in `styles.css` uses scale utilities directly because component classes cannot be nested in `@apply`.
3. Feature surfaces still using raw `text-af-*` migrate in US-009; aliases preserve rendering until then.
