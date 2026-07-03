# Component package documentation

Per-component usage notes, props tables, and examples belong in this
directory. Add one markdown file per component (or small component group) as
those surfaces are documented.

## What belongs here

- Import paths and category entrypoints for a component
- Props, variants, and accessibility notes
- Short usage examples that do not depend on dashboard routes or API clients

## What does not belong here

- Dashboard-only wiring, data fetching, or business workflow guides
- Generated docs registries, docs-site routes, or build-time documentation
  tooling

This docs shell is plain markdown kept under version control. Host applications
read it directly from the package; no docs-site implementation is required.

## Documented components

- [Typography roles](./typography-roles.md) — `Text`, `Heading`, `Label`,
  `Code`, dense text, truncation, and wrapping
- [Layout and display primitives](./layout-display-primitives.md) —
  `SurfacePanel`, `ActionRow`, `DescriptionList`, and responsive layout guidance
- [Form select primitives](./forms-select-primitives.md) — `Select`,
  `NativeSelect`, `EnumSelect`, keyboard behavior, and option contracts
- [Overlay and disclosure primitives](./overlays.md) — Dialog, Popover,
  Collapsible, and ScrollArea accessibility, labeling, focus, and overflow
  guidance.

## Getting started

See the package [README](../README.md) for install, CSS import setup, theming,
and consumer dependency boundaries. Link new component docs from that README or
from sibling files in this directory as the library grows.

## Feedback and code display

- [AlertPanel](./feedback-alert-panel.md) — semantic feedback variants, token
  mapping, accessibility, and Storybook references
- [Skeleton](./feedback-skeleton.md) — loading placeholders, busy regions, and
  empty vs loading feedback
- [CodePanel](./data-display-code-panel.md) — long-line and long-block
  containment, scrolling, and responsive layout expectations

## Widget layout recipes

- [Widget frame and layout recipes](./widget-frame-recipes.md) — import paths,
  state ownership, accessibility, Storybook references, and source-copy guidance
  for domain-free framed panels.
