# Website Feature Module Contract

This document defines the frontend feature-module contract for `ui/src/features/` during the website component cleanup.

## Goals

- Make feature structure predictable for maintainers.
- Keep feature public entrypoints explicit so app composition and shared UI do not rely on ad hoc deep imports.
- Separate React components, React hooks, state, messages, and non-React helpers by responsibility without changing customer-visible behavior.

## Standard Layout

Each feature should use the smallest layout that matches its responsibilities:

- `components/` for feature-scoped React components and stories or tests that primarily verify those components.
- `hooks/` for feature-scoped React hooks and their focused tests.
- `messages/` for feature-owned localized copy catalogs and lookup tests.
- `state/` for feature-owned Zustand stores or other client-state modules.
- clearly named feature-root modules for non-React helpers, contracts, or domain shaping that are not components or hooks.
- `index.ts` for the intentionally supported public feature entrypoint.

## EntryPoint Rules

- App-level composition and shared UI should import from a feature root `index.ts` when consuming the feature's public surface.
- When a feature exposes state or another sub-boundary intentionally, prefer a sub-boundary barrel such as `state/index.ts` instead of importing individual implementation files.
- Deep imports to leaf modules are acceptable inside the same feature while a migration is in progress, but cross-feature and app-level imports should converge on explicit barrels.
- Do not export every file by default. Re-export only the feature surface that other features, app shells, tests, or shared UI intentionally depend on.

## Classification Rules

- A module belongs under `hooks/` only if it is an actual React hook.
- Visualization helpers, contracts, geometry utilities, and other pure modules should keep descriptive names and remain outside `hooks/`.
- Shared primitives belong under `ui/src/components/ui/` instead of feature folders.
- Feature folders may depend on shared primitives, API clients, and shared hooks, but shared primitives must not depend on feature internals except through intentional feature entrypoints during this cleanup.

## Migration Notes

- This cleanup standardizes structure only; it does not change backend contracts, generated API artifacts, or visible product behavior.
- Migrate one feature group at a time and preserve existing loading, empty, error, and success behavior while moving files.
- When a migration changes the location of a public module, update the owning feature barrel in the same change so downstream imports remain stable.
