# Typography roles

Domain-free typography primitives for headings, body copy, labels, and inline
code. Use these components when you need consistent text hierarchy, dense
metadata rows, and predictable truncation or wrapping without dashboard routes,
providers, API clients, or generated OpenAPI types.

The package ships four primitives:

| Component | Role |
| --- | --- |
| `Heading` | Page and section titles with semantic heading levels. |
| `Text` | Body, supporting, muted, caption, and dense copy variants. |
| `Label` | Uppercase field labels and description-list term labels. |
| `Code` | Inline monospace identifiers at body or supporting sizes. |

These components are **presentation typography**. They apply token-backed role
classes and optional overflow behavior. They do not fetch data, localize strings,
or own application state.

## Required setup

Import package styles once in your host application before rendering typography:

```css
@import "@you-agent-factory/components/styles.css";
```

Typography roles depend on package font tokens (`--font-display`, `--font-sans`,
`--font-mono`) and semantic color roles such as `text-on-surface` and
`text-on-surface-subtle`. They do not require dashboard `styles.css`, dashboard
providers, generated OpenAPI types, React Query, or Zustand.

## Import paths

Prefer the primitives category entrypoint for explicit category boundaries:

```ts
import {
  Code,
  Heading,
  Label,
  Text,
  type CodeProps,
  type HeadingProps,
  type LabelProps,
  type TextProps,
  type TextVariant,
} from "@you-agent-factory/components/primitives";
```

The same exports are also available from the package root:

```ts
import { Code, Heading, Label, Text } from "@you-agent-factory/components";
```

## Typography role hierarchy

Each primitive maps to a domain-free `af-*` role class defined in package CSS.
Prefer the React components over applying raw `af-*` classes in host markup so
overflow props and semantic element defaults stay consistent.

### Headings

| `level` | Default element | Role class | When to use |
| --- | --- | --- | --- |
| `page` | `h1` | `af-page-heading` | Top-level page titles. |
| `section` | `h3` | `af-section-heading` | Section titles inside panels or forms. |

Override the rendered element with `as` when the visual role and document
outline need different semantics:

```tsx
import { Heading } from "@you-agent-factory/components";

<Heading level="section" as="h2">
  Section title supplied by the host
</Heading>
```

### Text variants

| `variant` | Role class | When to use |
| --- | --- | --- |
| `body` (default) | `af-body-text` | Primary readable copy. |
| `supporting` | `af-supporting-text` | Secondary metadata below primary content. |
| `muted` | `af-muted-text` | De-emphasized supporting copy. |
| `caption` | `af-caption-text` | Compact captions and footnotes. |
| `dense` | `af-dense-body-text` | Tight metadata rows in tables or description lists. |

```tsx
import { Text } from "@you-agent-factory/components";

<Text>Primary body copy supplied by the host</Text>
<Text variant="muted">Muted secondary metadata</Text>
<Text variant="dense">Dense row value</Text>
```

### Labels and code

- `Label` applies `af-supporting-label` for uppercase field and term labels.
  Use `as="dt"` inside description lists.
- `Code` applies `af-body-code` or `af-supporting-code` for inline identifiers.
  Use `size="supporting"` for compact metadata.

```tsx
import { Code, Label } from "@you-agent-factory/components";

<Label htmlFor="resource-name">Resource name</Label>
<Code size="supporting">example-resource-id</Code>
```

## Dense text

Use `Text variant="dense"` when scan-friendly metadata needs tighter line height
without switching to caption sizing. Dense rows pair well with `DescriptionList`
and compact panel layouts.

The dense role uses `text-body-small` with `leading-tight` and
`text-on-surface-variant`. It is intended for host-supplied values such as
timestamps, revision numbers, and status summaries—not for long paragraphs.

## Truncation and long content

`Text`, `Heading`, and `Label` accept `truncate` and `wrap` props for overflow
behavior. Do not mix both on the same element.

| Prop | Role class | Behavior |
| --- | --- | --- |
| `truncate` | `af-text-truncate` | Single-line ellipsis inside a width-constrained parent. |
| `wrap` | `af-text-wrap` | Multi-line wrapping with `min-w-0` and `overflow-wrap: anywhere`. |

Truncation requires a bounded-width ancestor. Wrap long values inside narrow
containers to avoid horizontal page overflow:

```tsx
import { Label, Text } from "@you-agent-factory/components";

<div className="max-w-xs">
  <Label truncate>{longHostLabel}</Label>
  <Text wrap>{longHostValue}</Text>
</div>
```

Choose `truncate` for toolbar labels, table headers, and single-line metadata.
Choose `wrap` for multi-line descriptions, identifiers, and messages that must
remain fully readable.

## Host application responsibilities

The component package owns presentation: role classes, semantic defaults, and
overflow utilities backed by package tokens.

Host applications own:

| Concern | Owner |
| --- | --- |
| Visible headings, labels, body copy, and code content | Host |
| When to use muted, caption, or dense variants in product context | Host |
| `htmlFor`, `id`, and accessible naming for labels | Host |
| `dateTime` on time elements and heading level in the document outline | Host |
| Truncation versus wrapping for specific data values | Host |
| Loading, empty, error, and success copy around typography | Host |
| Localization and string formatting | Host |

Pass copy and accessibility attributes into typography primitives as props. Do
not expect the package to read from your routers, stores, or API layers.

## Storybook examples

Package Storybook stories demonstrate role hierarchy and overflow behavior with
package imports only:

- `Primitives/Typography` — body, supporting, muted, caption, dense, heading,
  label, code, truncated, and wrapped examples.
- `Primitives/Typography/Mobile typography roles` and
  `Primitives/Typography/Desktop typography roles` — responsive readability
  checks at narrow and wide viewport sizes.

Run `bun run storybook` from `ui/packages/components` or inspect the static
build with `bun run build-storybook`.
