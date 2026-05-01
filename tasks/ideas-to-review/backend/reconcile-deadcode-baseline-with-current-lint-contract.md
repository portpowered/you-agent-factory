# Reconcile Deadcode Baseline With Current Lint Contract

## Problem

Repository-root `make lint` currently fails even on a doc-only lane because
`cmd/deadcodecheck` reports 17 unreachable helpers while
`docs/development/deadcode-baseline.txt` is empty. That means future lanes can
trip the same unrelated failure before they even touch the code that owns those
helpers.

## Why This Matters

- It creates noisy CI and local validation failures that are unrelated to the
  current story.
- It makes narrow review lanes look dirty or incomplete even when their scoped
  code is fine.
- It encourages accidental baseline churn in unrelated PRs.

## Proposed Follow-Up

- Audit the current unreachable helper list and decide which functions should be
  deleted versus intentionally baselined.
- Update `docs/development/deadcode-baseline.txt` only after that audit so the
  baseline reflects real accepted debt rather than an empty placeholder.
- Add a narrow workflow note describing when it is acceptable to update the
  deadcode baseline versus when authors should delete the reported code.
