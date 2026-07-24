# Repository baselines

This directory owns repository-wide quality-gate baselines, budgets, coverage
minimums, and historical baseline snapshots.

Keep baselines here when a repository-level command or CI lane consumes them.
Keep package-owned golden files and contract fixtures beside their tests under
`testdata/baseline`, and keep executable performance budgets beside the code
they govern.

Baseline changes require review of the current findings. Prefer removing stale
entries and lowering accepted debt over expanding a baseline.

`backend-package-file-count.json` is an exact deletion-only ratchet. The package
file-count gate rejects new oversized packages, count increases, and entries
that were not lowered or removed when the corresponding package shrank.

For Packaged Service Structure FND-12, the maintainer-runnable public behavior
baseline suite map (CLI, HTTP, MCP, replay, visualization activation) lives in
[`fnd-12-public-behavior-baseline-suite-map.md`](./fnd-12-public-behavior-baseline-suite-map.md).
That map names focused Make/`go test` entry points and marks success vs
typed-failure coverage; it does not own PR #1262 CLI-manifest baselines.
