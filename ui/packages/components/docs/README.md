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

## Getting started

See the package [README](../README.md) for install, CSS import setup, theming,
and consumer dependency boundaries. Link new component docs from that README or
from sibling files in this directory as the library grows.

## Component guides

- [Charts](./charts.md) — `ChartContainer`, tooltip and legend content, state
  panels, config types, and host-owned data boundaries
