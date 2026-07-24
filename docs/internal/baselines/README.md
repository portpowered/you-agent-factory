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

`ownership-inventory.json` is the PSS-F01 frozen package-destination inventory.
It maps every production `pkg` package to one committed owner, approved family,
Process Edges exception, or deletion/move successor. When
`package-target-manifest.json` (FND-01) is present, validators reuse that seed
instead of inventing a second destination catalog. Regenerate with
`go run ./cmd/ownershipinventoryfreeze` and prove with
`go test ./internal/ownershipinventory`.
