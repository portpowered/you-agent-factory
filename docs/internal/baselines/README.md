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
Process Edges exception, or deletion/move successor. It also freezes owner and
nested-subservice rationale cards (authority, state/store, lifecycle, consumers,
transaction boundary, failure/recovery), large responsibility clusters, a
cross-service edge table that classifies each distinct-owner production import
as command, query, event, protocol composition, construction, lifecycle, or
external effect, named-owner confirmations for Providers, Provider Sessions,
Operator Settings, System Bootstrap, Factory Visualization, and Recordings with
reviewed nested-subservice maps (no alternate top-level owners or further
discovery), a misplaced-guard burn-down for standards/allowlists/package
guards/baselines/diagnostics that still assign provider inference or hosted
polling to Workers (replacement owners Providers or Automations), public
CLI/HTTP/MCP/replay/visualization and behavior-test surfaces mapped to durable
owners, and constructor/datastore/lifecycle-role/protocol-adapter ownership
rows. Process Edges edges are marked as the architecture exception and
restricted to construction or external effect. When
`package-target-manifest.json` (FND-01) is present, validators reuse that seed
for package rows instead of inventing a second destination catalog. Regenerate
with `go run ./cmd/ownershipinventoryfreeze` and prove with
`go test ./internal/ownershipinventory`.
