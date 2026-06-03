# Material-inspired layout spacing and primitives

---
doc-id: DEV-MATERIAL-LAYOUT-ROLES
---

Canonical reference for dashboard layout spacing roles and shared layout primitives (US-007).

## Source of truth

- **Spacing scale:** `ui/src/styles/layout-role-tokens.css` — `--spacing-layout-*` tokens and utilities such as `gap-layout-section`, `p-layout-inset-dialog`.
- **Product constants:** `ui/src/components/ui/dashboard-layout.ts` — `LAYOUT_*_CLASS` strings and `DASHBOARD_LAYOUT_CONTRACT`.
- **Composable wrappers:** `ui/src/components/ui/layout-primitives.tsx` — `SectionStack`, `CardContentStack`, `PageHeaderLayout`, `ToolbarRowLayout`, `FormGroupLayout`, `DialogBodyLayout`.
- **Shared dialog shell:** `ui/src/components/ui/dialog.tsx` — `DialogContent`, `DialogHeader`, `DialogFooter` use layout role gaps.

Visual review: Storybook `Agent Factory/UI/Layout Role Primitives`.

## Spacing roles (4px grid)

| Role | Utility | Size | Typical use |
| --- | --- | --- | --- |
| `tight` | `gap-layout-tight` | 8px | Toolbar chips, label-to-control stacks |
| `element` | `gap-layout-element` | 12px | Card interior rows, dialog footers |
| `group` | `gap-layout-group` | 16px | Dialog body sections, header action offset |
| `block` | `gap-layout-block` | 20px | Feature mutation dialogs (until US-009 migration) |
| `section` | `gap-layout-section` | 24px | Section stacks between widgets |
| `page` | `gap-layout-page` | 32px | Page-level vertical rhythm |

Inset roles: `p-layout-inset-card`, `p-layout-inset-card-relaxed`, `p-layout-inset-dialog`.

## Layout primitives

| Primitive | Constant / component | When to use |
| --- | --- | --- |
| Section stack | `SectionStack` / `LAYOUT_SECTION_STACK_CLASS` | Vertical groups of cards or page regions |
| Card content | `CardContentStack` / `LAYOUT_CARD_CONTENT_STACK_CLASS` | Interior copy, lists, and nested blocks inside a panel |
| Page header | `PageHeaderLayout` (`heading` + optional `actions`) / `LAYOUT_PAGE_HEADER_CLASS` | Title + trailing actions |
| Toolbar row | `ToolbarRowLayout` / `LAYOUT_TOOLBAR_ROW_CLASS` | Dense icon/action clusters (see also `DashboardActionRow`) |
| Form group | `FormGroupLayout` / `LAYOUT_FORM_GROUP_CLASS` | Label, input, helper text |
| Dialog body | `DialogBodyLayout` / `LAYOUT_DIALOG_BODY_CLASS` | Main dialog content between header and footer |

Prefer exported constants from `dashboard-layout.ts` in TSX instead of reintroducing ad hoc `gap-*` clusters when a primitive already expresses the intent.

## Utility-only spacing (not behind primitives)

Keep raw Tailwind spacing utilities when:

- A one-off offset is required (`mt-1`, `pb-2.5` on chart chrome).
- Graph or canvas overlays need pixel-tuned positioning.
- A third-party component exposes spacing only via its own class API.

Migrate recurring clusters (three or more copies of the same `grid gap-*` recipe) to layout primitives during feature work (US-009).

## Feature migration notes

- `DashboardActionRow` remains the status/action split row; new toolbars without statuses may use `ToolbarRowLayout`.
- `mutation-dialog.tsx` keeps local `gap-5` until US-009; map to `gap-layout-block` when that surface migrates.
- `widget-frame.tsx` detail list spacing (`[&_dl]:gap-3`) aligns with `gap-layout-element` semantically; migrate with feature cards in US-009.
