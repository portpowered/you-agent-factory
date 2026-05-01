# Functional Default Runtime Target Gap

## Why this should be reviewed

The functional decomposition PRD established the right package structure and
an explicit long lane, but the default lane is still above the original
`10s` target. Closeout verification on 2026-05-01 measured
`make test-functional` at `51.608s` in this Windows worktree, and an earlier
warm run still measured `32.691s`.

## Problem

The default non-long lane is no longer a single mixed bucket, but several
behavior packages still carry enough runtime that contributors will not get the
intended fast-feedback loop. The structural migration alone did not solve the
remaining runtime hotspots.

## Proposed follow-up

- Profile `make test-functional` package-by-package on the agreed baseline
  environment so the repo has a stable, reviewable runtime budget.
- Identify the slowest default-lane scenarios in `guards_batch`, `workflow`,
  `runtime_api`, and `smoke`, then either trim redundant setup or move any
  truly broad scenarios into the opt-in long lane.
- Decide whether the repository should enforce a per-package or total runtime
  budget in CI once the agreed baseline environment is defined.
