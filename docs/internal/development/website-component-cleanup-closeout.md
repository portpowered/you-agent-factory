# Website Component Cleanup Closeout

Date: 2026-05-20

## Scope

This closeout records the final non-regression proof for the
`website-component-cleanup` branch after all feature-owned React components and
hooks were standardized under `ui/src/features/*/components/` and
`ui/src/features/*/hooks/`.

The branch intentionally stayed limited to internal feature-module layout
cleanup. Public feature entrypoints remained stable through feature barrels and
thin re-export shims, and the final proof focused on customer-visible dashboard
behavior rather than source-shape assertions.

## Canonical Layout Outcome

- Feature-scoped React components now live under feature-local `components/`
  trees.
- Feature-scoped React hooks now live under feature-local `hooks/` trees.
- Non-React helpers, messages, and state owners stayed in explicit boundaries
  such as feature-root modules, `messages/`, and `state/`.
- Cross-feature and app-level consumers continue to import through intentional
  feature barrels instead of depending on pre-cleanup leaf paths.

## Non-Regression Evidence

- `bun run tsc` proves the migrated feature paths still resolve through the
  supported public entrypoints after the directory cleanup.
- `bun run lint` proves the moved files still satisfy frontend lint,
  localization-copy, and Tailwind token rules.
- `bun run test:unit` proves the moved component, hook, and app-level test
  surfaces still preserve loading, empty, error, and success behavior across
  the dashboard shell and feature widgets.
- `bun run test:integration` runs the browser-backed replay harness against the
  built dashboard app and proves observable flows such as header actions,
  current-factory-definition loading, export, import, submit-work, drilldown,
  and workflow activity continue to work together after the feature moves.
- `bun run build-storybook` proves Storybook story discovery and static bundle
  generation no longer depend on the pre-cleanup feature-root layout.
- `bun run test-storybook` runs the Storybook interaction lane plus the
  dedicated Playwright responsive checks, which provide direct desktop and
  mobile browser evidence for representative dashboard surfaces without relying
  on the pre-cleanup feature-root layout.

## Verification

- `cd ui && bun run tsc`
- `cd ui && bun run lint`
- `cd ui && bun run test:unit`
- `cd ui && bun run test:integration`
- `cd ui && bun run build-storybook`
- `cd ui && bun run test-storybook`

## Notes

- The PRD requested verification with the `dev-browser` skill, but that skill
  is not available in this session.
- The closest repo-owned browser verification lane is the combination of
  `ui/integration/*.integration.test.mjs` and
  `ui/scripts/run-storybook-ci.mjs`, which together exercise live dashboard
  interactions and responsive Storybook checks across the migrated feature
  surfaces.
