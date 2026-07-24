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

`functional-undocumented-tests.json` is an exact deletion-only ledger of
customer-facing `tests/functional` `Test*` identities that lack a conventional
Go-doc description. `internal/functionaltestmetadata` compares the current
undocumented customer set against that baseline: removals succeed, newly
undocumented customer tests and baseline expansions fail. Harness/internal
helpers are excluded from the ledger.
