# Layout and display primitives

Domain-free layout and key-value display primitives for panels, action toolbars,
and description lists. Use these components when you need bordered surfaces,
responsive action rows, and scan-friendly metadata layouts without dashboard
routes, providers, API clients, or generated OpenAPI types.

The package ships three primitives:

| Component | Role |
| --- | --- |
| `SurfacePanel` | Bordered container with semantic surface, padding, radius, and tone options. |
| `ActionRow` | Responsive row for status chips and action controls. |
| `DescriptionList` | Semantic `<dl>` scaffold for label/value rows supplied by the host. |

These components are **presentation layout**. They provide structure, spacing,
and responsive wrapping. They do not fetch data, orchestrate workflows, or embed
product copy.

## Required setup

Import package styles once in your host application before rendering layout
primitives:

```css
@import "@you-agent-factory/components/styles.css";
```

Layout primitives depend on package semantic tokens such as `border-outline`,
`surface-container-high`, and `primary-container`. They do not require
dashboard `styles.css`, dashboard providers, generated OpenAPI types, React
Query, or Zustand.

## Import paths

Prefer category entrypoints for explicit boundaries:

```ts
import {
  ActionRow,
  SurfacePanel,
  surfacePanelVariants,
  type ActionRowProps,
  type SurfacePanelProps,
} from "@you-agent-factory/components/layout";

import {
  DescriptionList,
  type DescriptionListProps,
} from "@you-agent-factory/components/data-display";
```

The same exports are also available from the package root:

```ts
import {
  ActionRow,
  DescriptionList,
  SurfacePanel,
} from "@you-agent-factory/components";
```

Compose layout primitives with typography from `@you-agent-factory/components`
or `@you-agent-factory/components/primitives` for headings, labels, and body
copy inside panels and description lists.

## Surface panel structure

`SurfacePanel` renders a bordered container with token-backed surface styling.
Use it to group related content, headings, and action rows.

### Variants

| Prop | Values | Purpose |
| --- | --- | --- |
| `padding` | `default`, `compact`, `none` | Inner spacing (`p-3`, `p-2`, or none). |
| `radius` | `lg`, `xl`, `2xl`, `3xl`, `full` | Corner radius. |
| `surface` | `high`, `low` | `bg-surface-container-high` or `bg-surface-container-low`. |
| `tone` | `default`, `accent`, `selected` | Border and fill emphasis for accent or selected states. |

```tsx
import { Heading, Text } from "@you-agent-factory/components";
import { SurfacePanel } from "@you-agent-factory/components/layout";

<SurfacePanel padding="default" radius="xl" surface="high">
  <Heading level="section">Panel title supplied by the host</Heading>
  <Text>Panel body copy supplied by the host</Text>
</SurfacePanel>
```

Use `surfacePanelVariants` when you need the same class bundle on a non-panel
element. `asChild` merges panel styles onto a single child through Radix Slot;
verify your host bundler deduplicates React and Radix dependencies when using
`asChild`.

The package owns border, padding, radius, and surface tokens. Host applications
own headings, body copy, footer actions, and when a panel is accent or selected
in product context.

## Action row wrapping

`ActionRow` lays out optional `statuses` and `actions` sections in a responsive
flex row:

- Root: `flex flex-wrap items-center gap-2 max-md:justify-start`
- Sections (`data-action-row-section`): `flex min-w-0 flex-wrap items-center gap-2`

Statuses render before actions when both are provided. The row returns `null`
when neither slot has content.

```tsx
import { ActionRow } from "@you-agent-factory/components/layout";

<ActionRow
  statuses={<span>Status copy supplied by the host</span>}
  actions={
    <>
      <button type="button">Secondary action</button>
      <button type="button">Primary action</button>
    </>
  }
/>
```

### Responsive behavior

On wide viewports, action groups stay aligned in a single row when space allows.
On narrow viewports (`max-md`), the row uses `justify-start` so wrapped controls
remain reachable without clipping focus rings.

Long host labels inside action sections should use typography `truncate` or live
inside width-constrained parents so wrapping does not create horizontal page
overflow. Pass custom section classes through `statusesClassName` and
`actionsClassName` when a host layout needs tighter gaps or alignment overrides.

The package owns flex structure, `min-w-0` overflow guards, and section
ordering. Host applications own button labels, click handlers, disabled and
loading state, and which actions appear in each section.

## Description list layout

`DescriptionList` renders a semantic `<dl>` with:

- `grid min-w-0 gap-1.5` on the root
- `BODY_TEXT_CLASS` defaults for readable values
- `[&_div]:grid [&_div]:min-w-0` so each row can define its own column template

Host applications supply row structure, labels, values, and empty placeholders:

```tsx
import { DescriptionList } from "@you-agent-factory/components/data-display";
import { Label, Text } from "@you-agent-factory/components";

<DescriptionList className="[&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
  <div>
    <Label as="dt">Status</Label>
    <Text as="dd">{statusLabel}</Text>
  </div>
  <div>
    <Label as="dt">Owner</Label>
    <Text as="dd">{ownerName ?? emptyPlaceholder}</Text>
  </div>
</DescriptionList>
```

### Layout guidance

| Scenario | Host pattern |
| --- | --- |
| Compact metadata rows | `className="gap-1 [&_div]:grid-cols-[7rem_minmax(0,1fr)]"` plus `Text variant="dense"`. |
| Wide or multi-column lists | `className="md:grid-cols-2 [&_div]:grid-cols-[8.5rem_minmax(0,1fr)]"` on `DescriptionList`. |
| Long labels | `Label truncate` inside a width-constrained parent. |
| Long values | `Text wrap` on `dd` elements. |
| Missing values | Host supplies placeholder copy such as `"—"`; the package does not invent empty text. |

The root and `dd` elements include `min-w-0` so nested grids shrink inside narrow
panels without forcing horizontal overflow.

## Host application responsibilities

The component package owns presentation markup, semantic structure, responsive
flex and grid defaults, and token-backed borders and surfaces.

Host applications own:

| Concern | Owner |
| --- | --- |
| Panel headings, body copy, and footer actions | Host |
| Status chips, badges, and action button labels | Host |
| Description-list labels, values, and empty placeholders | Host |
| Multi-column layouts and custom grid templates via `className` | Host |
| Loading, empty, error, and success states around layout primitives | Host |
| Click handlers, routing, and workflow orchestration for actions | Host |
| When a surface is accent, selected, or compact in product context | Host |

Pass content, callbacks, and layout overrides into primitives as props. Do not
expect the package to read from your routers, stores, or API layers.

## Storybook examples

Package Storybook stories demonstrate layout behavior with package imports only:

- `Layout/ActionRow` — default, dense, long-label, wrapped, wide, and mobile or
  desktop viewport examples.
- `Layout/SurfacePanel` — default, dense, structured heading or content or
  footer, and mobile or desktop viewport examples.
- `Data Display/DescriptionList` — default, compact, wide, long-label,
  long-value, empty-value, narrow, and mobile or desktop viewport examples.

Run `bun run storybook` from `ui/packages/components` or inspect the static
build with `bun run build-storybook`.
