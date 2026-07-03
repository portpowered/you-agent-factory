# Widget frame and layout recipes

Domain-free widget frame shells and layout recipes for dashboard-like panels.
Use these components when you need framed content regions with consistent
typography, state treatments, disclosure controls, and responsive layout helpers
without importing dashboard features, bento grids, API clients, or app state.

## Required setup

Import package styles once in your host application before rendering recipes:

```css
@import "@you-agent-factory/components/styles.css";
```

Recipes depend on package role tokens (`--color-surface`, `--shadow-af-card`,
typography roles) and Tailwind utility classes compiled from those tokens. They
do not require dashboard `styles.css`, dashboard providers, generated OpenAPI
types, React Query, Zustand, or app localization context.

## Import paths

Prefer the recipes category entrypoint for tree-shaking and explicit category
boundaries:

```ts
import {
  WidgetFrame,
  WidgetSubtitle,
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateTitle,
  WidgetEmptyStateText,
  WidgetLoadingState,
  WidgetErrorState,
  WidgetSuccessState,
  WidgetFrameDisclosure,
  WidgetFrameDisclosureTrigger,
  WidgetFrameDisclosurePanel,
  widgetFrameDetailCardClass,
} from "@you-agent-factory/components/recipes";
```

The same exports are also available from the package root:

```ts
import { WidgetFrame } from "@you-agent-factory/components";
```

## Host ownership

The package owns presentation markup, layout classes, and accessibility
semantics for frame shells. Host applications own everything domain-specific:

| Concern | Owner |
| --- | --- |
| User-visible copy (titles, messages, labels) | Host |
| Data fetching, caching, and error mapping | Host |
| Domain models (factory, work, session, provider) | Host |
| Collapsed/expanded boolean state | Host |
| Header and body action callbacks | Host |
| Bento grid composition, drag/resize, widget selection | Dashboard feature code |
| Durable cross-route or persisted UI state | Host |

Pass copy and state into recipe components as props and children. Do not expect
recipes to read from your routers, stores, or API layers.

## Composition overview

A typical framed panel stacks:

1. `WidgetFrame` — card shell with title, optional `headerAction`, and scrollable body.
2. Content primitives — `WidgetSubtitle`, `WidgetDetailCopy`, or arbitrary host children.
3. State shells — `WidgetLoadingState`, `WidgetErrorState`, `WidgetSuccessState`, or `WidgetEmptyState`.
4. Optional disclosure — `WidgetFrameDisclosure` + controlled trigger/panel pair.

```tsx
import { useState } from "react";
import {
  WidgetDetailCopy,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetFrame,
  WidgetFrameDisclosure,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
  WidgetLoadingState,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";

export function ExamplePanel({
  loading,
  title,
}: {
  loading: boolean;
  title: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const panelID = "example-panel-details";

  return (
    <WidgetFrame
      headerAction={
        <button type="button" onClick={() => undefined}>
          Refresh
        </button>
      }
      title={title}
    >
      <WidgetSubtitle>42 host-provided items</WidgetSubtitle>
      {loading ? (
        <WidgetLoadingState>
          <WidgetEmptyStateTitle>Loading content</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>Host-provided loading message.</WidgetEmptyStateText>
        </WidgetLoadingState>
      ) : (
        <WidgetDetailCopy>Host-provided detail copy.</WidgetDetailCopy>
      )}
      <WidgetFrameDisclosure>
        <WidgetFrameDisclosureTrigger
          controlsID={panelID}
          expanded={expanded}
          onExpandedChange={setExpanded}
        >
          {expanded ? "Collapse details" : "Expand details"}
        </WidgetFrameDisclosureTrigger>
        <WidgetFrameDisclosurePanel expanded={expanded} id={panelID}>
          <WidgetDetailCopy>Host-provided disclosure content.</WidgetDetailCopy>
        </WidgetFrameDisclosurePanel>
      </WidgetFrameDisclosure>
    </WidgetFrame>
  );
}
```

## Key props and layout helpers

### `WidgetFrame`

| Prop | Purpose |
| --- | --- |
| `title` | Required accessible name (`aria-label` on the frame article). |
| `children` | Host content region inside the body grid. |
| `headerAction` | Optional header tools slot (buttons, menus). A spacer preserves header height when omitted. |
| `wide` | Applies minimum body height for wide dashboard columns. |
| `bodyScroll` | Enables scrollable body (`overflow-auto`). Defaults to `true`. |
| `className` / `bodyClassName` | Host layout overrides. Apply `widgetFrameDetailCardClass` via `className` when rendering description lists. |

Layout constants exported for host composition:

- `widgetFrameDetailCardClass` — description-list spacing inside framed detail cards.
- `WIDGET_FRAME_MIN_WIDTH_CLASS` — `min-w-0` shrink guard for grid parents.
- `WIDGET_FRAME_WIDE_BODY_CLASS` — minimum body height for wide layouts.
- `WIDGET_FRAME_RESPONSIVE_SHELL_CLASS` — full-width responsive shell helper.

### State shells

| Component | Semantics | Host provides |
| --- | --- | --- |
| `WidgetLoadingState` | `role="status"`, `aria-busy="true"` | Loading copy via children; optional `placeholder` or `showDefaultPlaceholder={false}`. |
| `WidgetErrorState` | `role="alert"` | Error title and message via children. |
| `WidgetSuccessState` | `role="status"` | Success title and message via children. |
| `WidgetEmptyState` | Neutral dashed panel | Empty title/text via `WidgetEmptyStateTitle` / `WidgetEmptyStateText` or arbitrary children. |

State shells never invent domain-specific text. All user-visible strings come from
the host.

### Disclosure (collapsed / expanded)

Use controlled state from the host:

- `WidgetFrameDisclosureTrigger` requires `controlsID`, `expanded`, and optional `onExpandedChange`.
- `WidgetFrameDisclosurePanel` requires matching `id` and `expanded`.
- Triggers expose `aria-expanded`, `aria-controls`, keyboard focus rings, and `type="button"`.
- Panels use the `hidden` attribute when collapsed.

Icon-only triggers must pass `aria-label`. Labeled triggers use visible button text.

### Responsive layout helpers

For Storybook demos and host layout tests:

- `widgetFrameStoryShellStyle(maxWidth)` — bounded shell wrapper style object.
- `widgetFrameHasNoHorizontalOverflow(element)` — pure helper for overflow checks.
- `WIDGET_FRAME_STORY_SHELL_DATA_ATTR` — `data-widget-frame-story-shell` marker for test shells.

## Accessibility expectations

- Frame title renders as a level-3 heading inside a landmark `article` labeled with the same title text.
- Loading, error, and success regions use appropriate live-region roles; do not nest conflicting roles.
- Disclosure triggers are keyboard operable buttons with visible `focus-visible` outlines.
- Header actions supplied by the host must include accessible names (`aria-label` or visible text).
- Host copy inside alerts and status regions should use heading + body structure for screen-reader scanability.

## Storybook visual reference

Package Storybook lives under `Recipes/WidgetFrame`. Stories use package imports
and package token decorators only — no dashboard providers.

| Story | Storybook id | Demonstrates |
| --- | --- | --- |
| Success content | `recipes-widgetframe--success-content` | Default framed content |
| Empty state | `recipes-widgetframe--empty-state` | Host-provided empty copy |
| Loading state | `recipes-widgetframe--loading-state` | `WidgetLoadingState` + placeholder |
| Error state | `recipes-widgetframe--error-state` | `WidgetErrorState` alert treatment |
| Success state | `recipes-widgetframe--success-state` | `WidgetSuccessState` treatment |
| Collapsed disclosure | `recipes-widgetframe--collapsed-disclosure` | Controlled collapse + expand interaction |
| Expanded disclosure | `recipes-widgetframe--expanded-disclosure` | Expanded panel visibility |
| Responsive compact | `recipes-widgetframe--responsive-compact` | 360px bounded shell |
| Responsive medium | `recipes-widgetframe--responsive-medium` | 768px bounded shell |
| Responsive wide | `recipes-widgetframe--responsive-wide` | 1280px bounded shell |

Run package Storybook locally:

```bash
cd ui/packages/components
bun run storybook
```

Browser verification for documented stories:

```bash
cd ui/packages/components
bun run verify:widget-frame-docs
```

Responsive overflow checks (compact + wide at mobile and desktop viewports):

```bash
cd ui/packages/components
bun run verify:widget-frame-responsive
```

## Allowed dependencies

Recipe source may import:

- Package utilities (`cn` from `@you-agent-factory/components/utilities`)
- Package token CSS via the host `styles.css` import
- React and `react-dom` peer dependencies

Recipe source must **not** import:

- Dashboard feature modules, routes, or bento grid (`react-grid-layout`)
- Generated OpenAPI clients or dashboard API adapters
- React Query, Zustand, Monaco, Sonner, or dashboard i18n/session providers
- Factory/work/session/provider domain types

`check:package-boundary` enforces these rules in CI.

## Source-copy guidance

Teams may copy recipe files into a host repository to customize markup while
keeping the same token foundation:

1. Import `@you-agent-factory/components/styles.css` in the host app.
2. Copy only the recipe files you need from `src/recipes/`.
3. Keep copied code on the package side of your boundary — do not pull dashboard
   modules into copied recipe files.
4. Retain host ownership of copy, data fetching, disclosure state, and action handlers.

**No source-copy CLI or generator ships with this package.** Copy-and-adapt
workflows are manual and maintained by the host application.

## Dashboard integration note

The dashboard keeps bento composition (`AgentBentoCard`, grid layout, widget
picker) in feature code. Dashboard widgets import recipe content primitives from
`@you-agent-factory/components/recipes` while `DashboardWidgetFrame` remains a
dashboard-owned wrapper that applies bento-specific chrome.

When adopting recipes outside the dashboard, compose `WidgetFrame` directly and
supply your own header actions, state, and slot content.
