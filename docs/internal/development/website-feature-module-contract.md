# Website Feature Module Contract

This document defines the repository-owned governance contract for `ui/src/features/`.

## Goals

- Make feature structure predictable for maintainers and reviewers.
- Keep public versus internal ownership explicit without relying on feature-root files.
- Preserve behavior-owned feature boundaries so oversized features do not become generic buckets.
- Promote code shared across multiple features into shared UI modules instead of copying feature-local helpers.

## Root Contract

Every direct child of `ui/src/features/` is a feature root. A feature root:

- may contain directories only.
- may not contain files directly at the feature root.
- may not use `index.ts` as a special exception. `index.ts` is also prohibited at the feature root.

This applies to every file type, including source files, tests, stories, markdown notes, and helper modules.

## Standard Ownership Buckets

Each feature should use the smallest directory layout that matches its responsibilities. Approved internal ownership buckets include:

- `components/` for feature-scoped React components plus tests or stories that primarily verify those components.
- `hooks/` for actual React hooks and their focused tests.
- `messages/` for feature-owned localized copy catalogs and lookup tests.
- `state/` for feature-owned Zustand stores or other client-state modules.
- `selectors/` for feature-owned selection or derivation helpers when those selectors are a real boundary.
- `lib/` for pure helper modules that are genuinely feature-local but do not fit a more precise bucket.

When a more specific name communicates ownership better than a generic helper bucket, prefer a well-named domain directory such as `editing/`, `graph/`, `chart/`, or `replay/`.

## Public Surface Rules

- Public entrypoints must live under an approved subdirectory rather than at the feature root.
- App-level composition, cross-feature imports, and shared UI should depend on intentional public-surface modules instead of ad hoc deep imports.
- When a feature exposes multiple supported seams, use explicit sub-boundary entrypoints rather than exporting every implementation file.
- Deep imports may exist temporarily inside the same feature during migrations, but maintained public imports should converge on the documented public-surface directories for that feature.

## Boundary Rules

- A module belongs under `hooks/` only if it is an actual React hook.
- Visualization helpers, contracts, geometry utilities, selectors, and other pure modules should live under the most descriptive approved directory instead of hiding at the feature root.
- Shared primitives belong under `ui/src/components/ui/` or another appropriate shared module rather than under one feature.
- Code used by multiple features should be promoted into an appropriate shared UI module instead of being copied between feature directories.
- When one feature starts collecting multiple distinct user-facing responsibilities, default toward splitting it into sibling features when that creates clearer behavior ownership than adding more internal buckets.

## Migration Notes

- This governance initiative standardizes structure only; it does not change backend contracts, generated API artifacts, or intended customer-visible behavior.
- Migrate one feature slice at a time and preserve existing loading, empty, error, and success states while moving modules.
- When a migration changes the location of a public module, update the owning public entrypoint in the same change so downstream imports remain stable.
- Temporary exceptions should be explicit, centrally tracked, and removed as each migrated slice eliminates its feature-root files.
