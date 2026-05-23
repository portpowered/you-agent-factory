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

- `public/` is the one canonical directory for feature public exports.
- The default public entrypoint is `ui/src/features/<feature>/public/index.ts`.
- A feature that exposes more than one supported public seam may add explicitly named siblings under `public/`, such as `public/testing.ts` or `public/editor.ts`, but should not create a second public-export directory name.
- Public entrypoints must live under `public/` rather than at the feature root.
- App-level composition, cross-feature imports, and shared UI should depend on intentional `public/` modules instead of ad hoc deep imports.
- When a feature exposes multiple supported seams, use explicit `public/*` entrypoints rather than exporting every implementation file.
- Deep imports may exist temporarily inside the same feature during migrations, but maintained public imports should converge on the documented `public/` entrypoints for that feature.

Example import intent:

- Prefer `ui/src/features/header/public` over `ui/src/features/header/dashboard-header`.
- Prefer `ui/src/features/current-selection/public/provider-session` over a consumer guessing which internal `components/` or `hooks/` file is safe to import.

## Boundary Rules

- A module belongs under `hooks/` only if it is an actual React hook.
- Visualization helpers, contracts, geometry utilities, selectors, and other pure modules should live under the most descriptive approved directory instead of hiding at the feature root.
- Shared primitives belong under `ui/src/components/ui/` or another appropriate shared module rather than under one feature.
- Code used by multiple features should be promoted into an appropriate shared UI module instead of being copied between feature directories.
- When one feature starts collecting multiple distinct user-facing responsibilities, default toward splitting it into sibling features when that creates clearer behavior ownership than adding more internal buckets.

## Migration Notes

- This governance initiative standardizes structure only; it does not change backend contracts, generated API artifacts, or intended customer-visible behavior.
- Migrate one feature slice at a time and preserve existing loading, empty, error, and success states while moving modules.
- When a migration removes a feature-root `index.ts`, move its supported exports into `public/index.ts` in the same feature and update maintained consumers to import from that `public/` path instead of relying on the root barrel.
- If a previous root `index.ts` mixed public and internal exports, keep only the supported consumer-facing API in `public/` and move internal-only helpers into the most descriptive non-public directory.
- When a migration changes the location of a public module, update the owning `public/` entrypoint in the same change so downstream imports remain stable in behavior even if the import path changes.
- Temporary exceptions should be explicit, centrally tracked, and removed as each migrated slice eliminates its feature-root files.
