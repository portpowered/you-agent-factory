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

`unfinished-package-moves.json` is the single ledger of unfinished Packaged
Service Structure migration intent. Each row names a `packagePath` under `pkg/`
that still has to move, together with its `destination` bucket, its `successor`
path, and — where a cutover packet closes it — a `deletionCondition`. A package
that simply stays where it already sits has **no row**: its owner is derived
from the tree by `ownershipinventory.OwnerForPackage`, so adding or removing a
package inside an existing service requires no edit here. The surviving check
runs the other way: a row naming a `packagePath` that is absent from the live
tree is stale and fails. The ledger only shrinks. Landing a move deletes its
row, and when `moves` is empty the file is deleted together with its loaders and
checks. Both `ownership-inventory-check` and `package-target-manifest-check`
read this one file, so there is no second destination catalog to keep in sync.

`package-target-test-only-baseline.json` is the exact deletion-only companion
for the package-target checker. It records only test-only source observations
for open move rows, with the source class included in each identity. A
test-only source never establishes production package liveness; new observations
and stale accepted entries both fail until the exact edge is reviewed.

`ownership-inventory.json` is the PSS-F01 frozen ownership inventory. It no
longer enumerates packages — that moved to `unfinished-package-moves.json`
above. It freezes the closed destination vocabulary, the Process Edges
architecture exception, the structures.md seed services and additional current
roots, a cross-service edge table that classifies each distinct-owner production
import as command, query, event, protocol composition, construction, lifecycle,
or external effect, named-owner confirmations for Providers, Provider Sessions,
Operator Settings, System Bootstrap, Factory Visualization, and Recordings with
reviewed nested-subservice maps (no alternate top-level owners or further
discovery), and a misplaced-guard burn-down for standards/allowlists/package
guards/baselines/diagnostics that still assign provider inference or hosted
polling to Workers (replacement owners Providers or Automations). Process Edges
edges are marked as the architecture exception and restricted to construction or
external effect. Regenerate with `go run ./cmd/ownershipinventoryfreeze` and
prove with `go test ./internal/ownershipinventory` or
`make ownership-inventory-check`.

Owner and nested-subservice rationale cards (authority, state store, lifecycle,
consumers, transaction boundary, failure recovery), large responsibility
clusters, public CLI/HTTP/MCP/replay/visualization and behavior-test surface
ownership, and constructor/datastore/lifecycle-role/protocol-adapter ownership
are **not** baselines. Nothing counts or ratchets them, and requiring a row per
service made adding a service a registration exercise. They are published as
design intent at
[`docs/architecture/service-ownership-rationale.md`](../../architecture/service-ownership-rationale.md),
along with the destination-vocabulary rationale and the deferred FND-06 Edges
narrowing that used to sit in the retired
`docs/internal/packaged-service-structure/package-target-manifest.json`.

The initial path-lease freeze published from that inventory lives at
`docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json`.
It reuses FND-10 `pss-path-lease-packet-manifest/v1` mechanics (`internal/psslease`)
to assign exclusive changed-path leases for the ownership-inventory packet
(`PSS-F01`) and the first PSS-F02 owner-boundary checker slice, rejects
overlapping active leases, and refuses CLI-manifest / provider-conductor
portfolio holds. Regenerate with the same freeze command; prove with
`go test ./internal/ownershipinventory ./internal/psslease` or
`make ownership-inventory-check`. The combined verification gate
(`ownershipinventory.VerifyFreeze`) proves completeness, stable sort order,
edge classifications, named-owner coverage, Process Edges exception presence,
and non-overlapping active leases together. That check is part of `make lint`.

`functional-undocumented-tests.json` is an exact deletion-only ledger of
customer-facing `tests/functional` `Test*` identities that lack a conventional
Go-doc description. `internal/functionaltestmetadata` compares the current
undocumented customer set against that baseline: removals succeed, newly
undocumented customer tests and baseline expansions fail. Harness/internal
helpers are excluded from the ledger.
