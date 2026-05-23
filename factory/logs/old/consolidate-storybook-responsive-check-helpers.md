# Cleanup Idea: Consolidate Storybook Responsive Check Helpers

## Why this cleanup exists

The recent dashboard and localization work expanded the Storybook responsive
verification lane. That is good coverage, but the helper code is starting to
split into overlapping script-local implementations:

- `ui/scripts/verify-import-export-storybook-responsive.mjs` owns shared
  Playwright assertions such as `expectVisible`, `expectNoHorizontalOverflow`,
  `waitForStoryRegion`, timeout constants, and viewport tolerance handling.
- `ui/scripts/dashboard-shell-storybook-responsive.mjs` duplicates the same
  visibility, region, horizontal-overflow, timeout, and tolerance helpers for
  one dashboard-shell assertion.
- `ui/scripts/verify-localized-widget-storybook-responsive.mjs` already accepts
  helper functions from the main responsive script instead of duplicating them.

This leaves two patterns for the same responsive-test support code in the same
script folder. As more dashboard stories are added, that split will make timeout
changes, tolerance changes, and locator behavior drift across checks.

## Requested change

Create one small shared Storybook responsive helper module and use it from the
existing responsive verification scripts.

Keep the cleanup narrow:

- do not change the Storybook story IDs being checked
- do not change the set of viewport sizes
- do not change the visible UI assertions or accessibility queries
- do not change the `storybook:responsive-check` package script behavior
- do not broaden this into a Storybook runner redesign or UI component changes

Suggested shape:

- Add a helper module under `ui/scripts/`, for example
  `storybook-responsive-helpers.mjs`.
- Move common helpers and constants there:
  - `STORY_RENDER_TIMEOUT_MS`
  - `OVERFLOW_TOLERANCE_PX`
  - `expectVisible`
  - `expectNoHorizontalOverflow`
  - `waitForStoryRegion`
  - any small shared viewport-bound helpers if they are reused cleanly
- Import those helpers from
  `verify-import-export-storybook-responsive.mjs` and
  `dashboard-shell-storybook-responsive.mjs`.
- Keep dashboard-shell-specific style comparison logic in
  `dashboard-shell-storybook-responsive.mjs`.
- Keep import/export-specific dialog assertions in
  `verify-import-export-storybook-responsive.mjs`.

## Relevant files

- `ui/scripts/verify-import-export-storybook-responsive.mjs`
- `ui/scripts/verify-import-export-storybook-responsive.test.mjs`
- `ui/scripts/dashboard-shell-storybook-responsive.mjs`
- `ui/scripts/dashboard-shell-storybook-responsive.test.mjs`
- `ui/scripts/verify-localized-widget-storybook-responsive.mjs`
- `ui/package.json`

## Acceptance criteria

- There is one reusable implementation of the common Storybook responsive
  helper assertions and constants.
- `verify-import-export-storybook-responsive.mjs` and
  `dashboard-shell-storybook-responsive.mjs` no longer maintain duplicate
  `expectVisible`, `expectNoHorizontalOverflow`, region-waiting, timeout, or
  horizontal-overflow tolerance logic.
- The dashboard-shell verification still checks the header controls, bento card
  controls, matching shell styles, and no horizontal overflow.
- The import/export and localized widget responsive checks keep their current
  observable behavior and viewport coverage.
- Existing script tests are adjusted to cover the shared helper module behavior
  without replacing browser-visible assertions with file-layout or inventory
  tests.
- `bun run test:unit -- scripts` or the repository's equivalent targeted
  script-test command passes.

## Review guidance

Prefer behavioral script-helper tests that exercise the exported helpers with
fake Playwright locators and pages. The important regression risk is changing
what the responsive lane observes at runtime, not whether a helper lives in a
specific file.
