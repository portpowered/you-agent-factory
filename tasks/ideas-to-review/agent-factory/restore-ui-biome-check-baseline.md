# Restore the repository-wide UI Biome check baseline

## Problem

`cd ui && bun run check` fails on the current `main` branch before feature
changes are applied. Biome 2.5.4 reports more than 800 formatting and import
organization errors across roughly 300 files, while the lint-only CI gate
remains green.

This makes any PRD that names `bun run check` as an acceptance gate impossible
to satisfy with a focused feature diff. It also causes repeated review
ambiguity: changed files can pass focused Biome checks and required CI while
the explicitly named repository command stays red.

## Suggested outcome

- Apply the repository-wide safe formatter/import fixes in a dedicated change
  whose only purpose is restoring the baseline.
- Verify the full UI unit, integration, browser, typecheck, build, and lint
  suites after the mechanical rewrite.
- Keep `bun run check` as a strict non-mutating whole-tree gate once the
  baseline is green.
- Add that exact command to required CI so the baseline cannot drift silently
  again.

## Current evidence

On 2026-07-23, `bunx biome check . --max-diagnostics=none` reported 815 errors
across 304 files in the feature worktree. The clean current `main` worktree
reported 816 errors from `bun run check`.
