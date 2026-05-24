# Fix Storybook CI smoke-story ID drift

## Problem

The repo-owned Storybook verification entrypoint in `ui/scripts/run-storybook-ci.mjs`
hard-codes the smoke story ID
`you-agent-factory-dashboard-export-factory-dialog--ready`, but the current built
Storybook manifest no longer contains that ID. As a result, `bun run test-storybook`
fails during readiness with a `404` on `iframe.html` before it reaches the actual
Vitest Storybook plays or responsive checks.

## Why this matters

- This breaks the standard frontend browser-verification path for future UI work.
- Contributors can get false negatives even when the touched stories are healthy.
- The failure happens early, so it obscures real Storybook regressions behind a
  harness drift problem.

## Suggested direction

- Replace the hard-coded smoke story ID with one derived from a canonical tagged
  Storybook story that is expected to exist.
- Alternatively, make the readiness step validate against `index.json` plus a
  story ID resolved from the built manifest instead of a duplicated constant.
- Keep the responsive verification scripts aligned with the same source of truth
  so the browser harness cannot drift separately from the stories it exercises.

## Acceptance signal

- `bun run test-storybook` succeeds against the current `storybook-static` output
  without manual story-ID overrides.
- The harness fails with an actionable error if the chosen canonical story really
  disappears from the built manifest.
