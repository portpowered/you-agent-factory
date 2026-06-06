## Summary

The repo-wide `ui/scripts/check-inline-component-class-usage.mjs` analysis is expensive enough to block routine UI iterations. In this worktree it continued consuming CPU for more than four minutes on a clean isolated run, which makes `bun run lint` unreliable as a fast mergeability gate for small current-selection changes.

## Why This Matters

- The script runs as part of the default UI lint workflow, so slow analysis affects nearly every frontend task.
- Long-running repo-wide AST scans make it harder to distinguish real lint regressions from tooling latency.
- Expensive checks encourage partial local verification and longer feedback loops, which is the opposite of what this workflow wants.

## Proposed Direction

- Profile `check-inline-component-class-usage.mjs` to identify the dominant hot path.
- Add incremental or cached file discovery/parsing where practical.
- Consider narrowing the default lint scope to changed files locally while preserving full-repo enforcement in CI, if the rule cannot be made fast enough.

## Reproduction Notes

- Command: `bun run check:inline-component-class-usage`
- Observed on: `2026-06-06`
- Behavior: the process stayed active and CPU-bound for more than four minutes in `/ui` with no result output.
