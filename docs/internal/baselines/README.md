# Repository baselines

This directory owns repository-wide quality-gate baselines, budgets, coverage
minimums, and historical baseline snapshots.

Keep baselines here when a repository-level command or CI lane consumes them.
Keep package-owned golden files and contract fixtures beside their tests under
`testdata/baseline`, and keep executable performance budgets beside the code
they govern.

Baseline changes require review of the current findings. Prefer removing stale
entries and lowering accepted debt over expanding a baseline.

## Required-comparison inventory

This is the source-backed provenance inventory for the required repository-state
comparisons. It was audited at commit
`153213a92b132b9e4cd7cacc9aa458a30821325b`. One row represents one committed
comparison file; shared consumers do not collapse rows. The audit began with a
20-file candidate lead, then reconciled the required workflow, Make, checker,
script, and package-test paths in both directions. It resolved to 28 active
repository-state comparison files: 18 in this directory, five Packaged Service
Structure live inventories, three contract or transport identity inventories,
one root boundary inventory, and one UI style inventory. The `Class` column is
the exact join key to the maintenance classification below.

The evidence identity for each row is the SHA-256 of the committed comparison
content at that audit commit. The implementation source and command columns are
the read-only provenance edge; they do not authorize an update. Story 002
extends these same rows with the ratchet or snapshot class, safe maintenance
procedure, and truthful generator status.

### Repository quality and coverage comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `docs/internal/baselines/backend-exemption-budget.json` | `make lint` → `backend-size`, `pkg-maint` | `internal/exemptionbudget/budget.go:Reconcile`; `cmd/backendsizecheck`; `cmd/pkgmaintcheck` | Backend quality gates | Directive identity and exemption-budget row | `make backend-size`; `make pkg-maint` | R-01 | `sha256:a195de5452b8bfa5cf7e1b1504e5cca65b30d6d10b79ca71f06d0711a4490ce6` |
| `docs/internal/baselines/backend-package-file-count.json` | `make lint` → `pkg-file-count` | `cmd/pkgfilecountcheck/main.go` | Backend package-shape gate | Package path and tracked Go file count | `make pkg-file-count` | R-02 | `sha256:c87c5eed895cfbab2a9923849f1678851f0f0853b98cca930bc3d2fc75c18cc3` |
| `docs/internal/baselines/deadcode-baseline.txt` | `make lint` → `deadcode` | `cmd/deadcodecheck/main.go` | Backend dead-code gate | Normalized unreachable-symbol identity | `make deadcode` | S-01 | `sha256:a620f0fae6462f0c36e95d12963bef8a791f0294f9c417421f19b78169c39651` |
| `docs/internal/baselines/frontend-deadcode-baseline.json` | `make lint` → `ui-deadcode` | `ui/scripts/check-deadcode-baseline.ts` | Dashboard dead-code gate | Normalized Knip issue identity | `make ui-deadcode` | R-03 | `sha256:988791a647d158530d962cf9b6f03b187f381b97ed78d10bcad1439b3d6d2e5b` |
| `docs/internal/baselines/functional-undocumented-tests.json` | `make verify-tests` → `test-maintenance` | `internal/functionaltestmetadata/baseline_repo_test.go:TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests` | Functional test metadata | Relative test file and `Test*` name | `go test ./internal/functionaltestmetadata -run TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests -count=1` | R-04 | `sha256:b78da501e4a36a1b497be1f2a5f8988ac50706db38d0c0ace17b7eb5d553c8b6` |
| `docs/internal/baselines/go-functional-coverage-package-minimums.json` | `make functional-test-viz` → backend functional coverage | `cmd/functionaltestviz/main.go:packageManifestPath`; `Makefile:GO_FUNCTIONAL_COVERAGE_MANIFEST` | Functional coverage gate | Go import package and minimum statement-coverage percentage | `make functional-test-viz` | R-05 | `sha256:79e77099a74fe8065b9af69a53622303196353b77ef55d373def3f52adb12dc5` |
| `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `make test-unit-coverage` → backend unit coverage | `cmd/unitcoverage/main.go`; `Makefile:GO_UNIT_COVERAGE_MANIFEST` | Unit coverage gate | Go import package and minimum statement-coverage percentage | `make test-unit-coverage` | R-06 | `sha256:ee1faa58bad1f07903868c3eb791184ea299c10250cfddb5478adfbecb299eb3` |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `make test-unit-latency-budget` | `cmd/unitlanebudget/main.go`; `cmd/unitlanebudget/budget.go` | Unit-lane performance gate | Three unit-lane wall samples plus package and test inventories | `make test-unit-latency-budget` | S-02 | `sha256:7416c3fd88fd8c216d6466c4cf1ca9d66d8b797e375cd4eb6578003df348b6d3` |
| `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | `make lint` → `ui-lint` | `ui/scripts/check-hardcoded-ui-copy.ts`; `ui/package.json:check:localized-copy` | Dashboard localization gate | Source location and literal finding | `cd ui && bun run check:localized-copy` | R-07 | `sha256:bb6ee7e94bc96d013f164dc81004471112e457fa8a22d11ab17c1804982c609a` |
| `docs/internal/baselines/package-structure-baseline.json` | `make lint` → `pkg-structure` | `cmd/pkgstructurecheck/main.go` | Packaged Service Structure | Package path and exact recorded structure finding | `make pkg-structure` | R-08 | `sha256:7485752286f3dd1f9afcf6e2deb9bc26fab28dcbc2696773d5283ca5e1f5bfc5` |
| `docs/internal/baselines/package-target-test-only-baseline.json` | `make lint` → `package-target-manifest-check` | `cmd/packagetargetmanifestcheck/manifest.go` | Package-target migration gate | Open-move package path and test-only source identity | `make package-target-manifest-check` | R-09 | `sha256:64c98e7f5ee3b25d74bda79ee50a571b1b3a21e985946bc34688b330df5af40a` |
| `docs/internal/baselines/petri-public-surface-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/petri_public_surface.go` | Runtime public-boundary gate | File, symbol, kind, and migration identity | `make pkg-boundary` | R-10 | `sha256:07f84f9ce316d81ac4d80f13ec8435853274b3481136a8eb1db14e8f464599cc` |
| `docs/internal/baselines/service-construction-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/service_baselines.go` | Service construction boundary | Source file, import path, symbol, and class | `make pkg-boundary` | R-11 | `sha256:a6c401bb41481149a3576bb7b29aa42f4999de122c88b70520d3e848de259059` |
| `docs/internal/baselines/service-cycle-ceiling.json` | `make lint` → `service-cycle-check` | `cmd/servicecyclecheck/report.go` | Service dependency graph | Minimum feedback-arc-set weight of the service graph | `make service-cycle-check` | R-12 | `sha256:410eebddbff280ff10c346699cf4585877f5a752dd184901d24ef1595d1f764d` |
| `docs/internal/baselines/test-service-import-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/service_baselines.go` | Test service-boundary gate | Test source file, concrete import, and target service | `make pkg-boundary` | R-13 | `sha256:336548d5046747b3a823faf4cdca89648d67a3896ed3a5fd076f66131e9cc49a` |
| `docs/internal/baselines/transport-behavior-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/transport_behavior.go` | Transport boundary gate | Transport source, imported service, and behavior edge | `make pkg-boundary` | R-14 | `sha256:ce7868b58936d0cd6cd635a2ac9ca404baf007069573e395c77466393eeaa29e` |
| `docs/internal/baselines/unfinished-package-moves.json` | `make lint` → `ownership-inventory-check`, `package-target-manifest-check` | `internal/ownershipinventory/moves.go`; `cmd/packagetargetmanifestcheck/manifest.go` | Packaged Service Structure migration | Live `pkg/` package path and successor move row | `make ownership-inventory-check`; `make package-target-manifest-check` | R-15 | `sha256:21bb2cfc079413494f5d10b093b6f775fefa019385e6b4ec1ef95366e938f26f` |

### Ownership and live tree comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ownership-boundary-baseline.json` | `make lint` → `pkg-boundary` | `cmd/ownershipboundarycheck/main.go` | Root service-boundary gate | Boundary finding key and occurrence count | `make pkg-boundary` | R-16 | `sha256:b4705381e2f0dba798e15fd4e83c2ebd93792d0f4a73ba120549e934f70071e8` |
| `docs/internal/baselines/ownership-inventory.json` | `make lint` → `ownership-inventory-check` | `internal/ownershipinventory/load.go`; `internal/ownershipinventory/gate.go`; `cmd/ownershipinventorycheck/main.go` | PSS-F01 ownership inventory | Package path, destination mapping, named owner, and guard row | `make ownership-inventory-check` | S-03 | `sha256:c1456242093d400a743d63124270278ca086a0dc018f6844dc61cb188c687083` |
| `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | `make lint` → `ownership-inventory-check` | `internal/ownershipinventory/path_lease_freeze.go`; `internal/ownershipinventory/gate.go` | PSS-F01 path-lease freeze | Packet ID, exclusive path, and active-lease overlap | `make ownership-inventory-check` | S-04 | `sha256:48d54c6e0d5171f44737083a1a9f69821a6d70831789d711afab9d83e12da3eb` |
| `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/operator_settings_root_go.go` | Operator Settings ownership | Root `.go` filename and contract/fold classification | `go test ./internal/ownershipinventory -count=1` | S-05 | `sha256:7785c2c312f143ba5bceaa254b15c30e1068da0cc1f12dbf6379596ebd1a0890` |
| `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/operator_settings_top_level.go` | Operator Settings ownership | Immediate child directory and classification | `go test ./internal/ownershipinventory -count=1` | S-06 | `sha256:ff3fd338c5f22a1d51beb201bab752e0885c386c0c44031d7feb22e866521a20` |
| `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/provider_sessions_root_go.go` | Provider Sessions ownership | Root `.go` filename and contract/fold classification | `go test ./internal/ownershipinventory -count=1` | S-07 | `sha256:7a1fdbe169ec18d77c38d5f55489ca1398ebf2b71c1833cae062bf3cd747ae75` |
| `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/provider_sessions_top_level.go` | Provider Sessions ownership | Immediate child directory and classification | `go test ./internal/ownershipinventory -count=1` | S-08 | `sha256:d541d6aaf9ca5da509ac1982119531e2c2c49b1ec791d4662073d5d6f4b7c58a` |

### CLI, MCP, and UI identity comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `contracts/testdata/baseline/cli-commands.json` | `make test-contract` | `pkg/transports/cli/commandidentity/baseline_fixture_test.go`; `pkg/transports/cli/cliinputs/walk_production_test.go` | CLI command identity | Production command path and stable identity candidate | `go test ./pkg/transports/cli/commandidentity -run TestWalk_ProductionInventoryMatchesCommittedBaseline -count=1` | S-09 | `sha256:1011ba95129d30a02c542d074af96fe023332be2f9af1b7fb932c80fa9952974` |
| `contracts/testdata/baseline/cli-command-inputs.json` | `make test-contract` | `pkg/transports/cli/cliinputs/walk_production_test.go` | CLI input identity | Command path, argument/flag position, name, and relationship | `go test ./pkg/transports/cli/cliinputs -run TestWalk_ProductionInventoryMatchesCommittedBaseline -count=1` | S-10 | `sha256:09331beded46c5557024f571916c3cdaa532a3ef9db732ca8aec544b2b3ff20d` |
| `contracts/testdata/baseline/mcp-tools.json` | Backend unit package tests in the required unit lane | `pkg/services/factory_sessions/transports/mcp/registry.go`; `pkg/services/factory_sessions/transports/mcp/registry_test.go` | MCP tool identity | Tool name, ID candidate, description, input schema, and handler registration | `go test ./pkg/services/factory_sessions/transports/mcp -run 'TestBaselineFixtureMatchesProjectedInventory|TestBaselineFixtureMatchesDiscoverToolsRegistry' -count=1` | S-11 | `sha256:98eff642b8b6bf442de1e150e13bdb89a6beab05c1fec3a24aa270690e9ffb33` |
| `ui/src/styles/palette-contrast-baseline.ts` | `make ui-component-test` → frontend component CI | `ui/src/styles/palette-contrast-ratchet.component.test.ts` | Dashboard styles | Palette, foreground token, fill token, and measured contrast ratio | `make ui-component-test` | R-17 | `sha256:e82d55905b1b523d8a5e2878bf9e94a2874ab55e6f3e0f1d9b93fcea5591f1cd` |

## Maintenance classification

Every active inventory row above has exactly one class ID. The `R-*` rows are
manual ratchets: they record an intentional one-way quality or migration
commitment, so a current finding must be reviewed before the canonical file is
changed. The `S-*` rows are snapshots: they are deterministic projections of a
specific repository observation and may be refreshed only by the named
generator from the named evidence.

The status terms below are deliberately narrow. **Merged** means that the
writer or generator is present on the current implementation branch. The
protected-main status is recorded separately so that a merged writer is not
mistaken for an unattended update path. There are no **Pending** or **Absent**
rows in this revision: PR #2347 has merged as
`fee3da73388514cfb5975307d2cd1e07b345cd84`, and the remaining S-* writers are
present on this branch. A verification test or an unrelated artifact generator
must not be relabelled as a maintenance mechanism.

### Manual ratchets

| Class | Comparison file | Owner and comparison unit | Safe update procedure | Generator status and one-way rule |
| --- | --- | --- | --- | --- |
| R-01 | `docs/internal/baselines/backend-exemption-budget.json` | Backend quality gates — directive identity and exemption-budget row | Run `make backend-size` and `make pkg-maint`. Remove a resolved directive; add an entry only for a reviewed exception with an owner and removal reason. | Manual; no canonical writer. Automatic re-export would accept newly exempted debt. |
| R-02 | `docs/internal/baselines/backend-package-file-count.json` | Backend package-shape gate — package path and tracked Go file count | Run `make pkg-file-count`. When a package shrinks, lower or remove its row in the same reviewed change; never raise a limit or add a row to hide growth. | Manual deletion-only ratchet; `cmd/pkgfilecountcheck` has no baseline writer. |
| R-03 | `docs/internal/baselines/frontend-deadcode-baseline.json` | Dashboard dead-code gate — normalized Knip issue identity | Run `make ui-deadcode`. Fix or remove the finding first; use the script's `--update-baseline` writer only for an explicit, reviewed intentional exception. | Manual ratchet; unattended `--update-baseline` would accept new dead code. |
| R-04 | `docs/internal/baselines/functional-undocumented-tests.json` | Functional test metadata — relative test file and `Test*` name | Run `go test ./internal/functionaltestmetadata -run TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests -count=1`. Document a test or remove a resolved row; never add newly undocumented customer tests to the baseline. | Manual deletion-only ratchet; the comparison test has no canonical writer. |
| R-05 | `docs/internal/baselines/go-functional-coverage-package-minimums.json` | Functional coverage gate — Go import package and minimum statement-coverage percentage | Run `make functional-test-viz`. If a quality-floor change is approved, generate a candidate with `go run ./cmd/gocoveragecheck -suite functional -generate-manifest <candidate>`, inspect it, and manually apply only the approved change. | Candidate generation only; never unattended-replace the manifest because current coverage could lower a quality floor. |
| R-06 | `docs/internal/baselines/go-unit-coverage-package-minimums.json` | Unit coverage gate — Go import package and minimum statement-coverage percentage | Run `make test-unit-coverage`. If a quality-floor change is approved, generate a candidate with `go run ./cmd/gocoveragecheck -suite unit -generate-manifest <candidate>`, inspect it, and manually apply only the approved change. | Candidate generation only; never unattended-replace the manifest because current coverage could lower a quality floor. |
| R-07 | `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | Dashboard localization gate — source location and literal finding | Run `cd ui && bun run check:localized-copy`. Move the copy to the catalog; update the baseline only for a reviewed intentional exception, and never use the writer unattended. | Manual ratchet; `--write-baseline` would accept new user-facing hardcoded copy. |
| R-08 | `docs/internal/baselines/package-structure-baseline.json` | Packaged Service Structure — package path and exact recorded structure finding | Run `make pkg-structure`. Resolve the finding, then remove only the resolved entry in a reviewed change; do not add or retain a row to make a new violation pass. | Manual deletion-only ratchet; `-create-baseline` is bootstrap-only and refuses to overwrite an existing file. |
| R-09 | `docs/internal/baselines/package-target-test-only-baseline.json` | Package-target migration gate — open-move package path and test-only source identity | Run `make package-target-manifest-check`. Resolve the move or test-only edge, then remove its exact row; do not add a new observation to make the migration check pass. | Manual deletion-only ratchet; `-create-test-only-baseline` refuses to overwrite an existing file. |
| R-10 | `docs/internal/baselines/petri-public-surface-baseline.json` | Runtime public-boundary gate — file, symbol, kind, and migration identity | Run `make pkg-boundary`. Resolve a public-surface migration finding, then remove only its reviewed row; never add a new public-surface debt row. | Manual deletion-only ratchet; the `-create-petri-public-surface-baseline` path refuses an existing baseline. |
| R-11 | `docs/internal/baselines/service-construction-baseline.json` | Service construction boundary — source file, import path, symbol, and class | Run `make pkg-boundary`. Complete the construction migration, then remove the resolved row in the same reviewed change; do not repin it to accept new construction debt. | Manual deletion-only ratchet; no canonical generator exists for this file. |
| R-12 | `docs/internal/baselines/service-cycle-ceiling.json` | Service dependency graph — minimum feedback-arc-set weight | Run `make service-cycle-check`. Change the ceiling only when the graph change is intentional and reviewed: an increase hides a regression, while a decrease records an unclaimed improvement. | Manual bidirectional ratchet; the checker rejects both unreviewed increases and decreases. |
| R-13 | `docs/internal/baselines/test-service-import-baseline.json` | Test service-boundary gate — test source file, concrete import, and target service | Run `make pkg-boundary`. Move or remove the reviewed test-service edge, then delete its exact row; do not add a new edge to the baseline. | Manual deletion-only ratchet; `-create-test-service-import-baseline` refuses to overwrite an existing file. |
| R-14 | `docs/internal/baselines/transport-behavior-baseline.json` | Transport boundary gate — transport source, imported service, and behavior edge | Run `make pkg-boundary`. Resolve the reviewed transport edge, then delete its exact row; do not add a new behavior edge to the baseline. | Manual deletion-only ratchet; `-create-transport-behavior-baseline` refuses to overwrite an existing file. |
| R-15 | `docs/internal/baselines/unfinished-package-moves.json` | Packaged Service Structure migration — live `pkg/` package path and successor move row | Run `make ownership-inventory-check` and `make package-target-manifest-check`. Landing a move deletes its row; when no moves remain, delete the empty ledger with its consumers. | Manual shrink-only ledger; no generator exists because package ownership is derived from the live tree. |
| R-16 | `ownership-boundary-baseline.json` | Root service-boundary gate — boundary finding key and occurrence count | Run `make pkg-boundary`. Resolve the boundary finding, then remove or lower only the corresponding reviewed entry; never raise accepted boundary debt. | Manual deletion-only ratchet; `-create-baseline` refuses to overwrite an existing file. |
| R-17 | `ui/src/styles/palette-contrast-baseline.ts` | Dashboard styles — palette, foreground token, fill token, and measured contrast ratio | Run `make ui-component-test`. Improve the measured contrast, then lower or remove the debt entry; never raise the recorded debt to make a regression pass. | Manual ratchet; the component test has no baseline writer, and automatic recapture could record a regression. |

### Self-maintained snapshots

| Class | Comparison file | Owner and comparison unit | Deterministic generation command | Generator status and safe use |
| --- | --- | --- | --- | --- |
| S-01 | `docs/internal/baselines/deadcode-baseline.txt` | Backend dead-code gate — normalized unreachable-symbol identity | `make regenerate-shared-ci-baselines BASELINE_REGEN_ROOT=. UNIT_LATENCY_BUDGET="$UNIT_BUDGET_BASELINE" UNIT_LATENCY_SAMPLES=".artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json" BASELINE_REGEN_DEADCODE_REPORT="$DEADCODE_REPORT_PATH"` | Writer merged on main via PR #2347, merge commit `fee3da73388514cfb5975307d2cd1e07b345cd84`, through `cmd/unitlanebudget/regenerate.go:regenerateSharedBaselines`. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` after validating the delivered normalized dead-code report and stages only the allowlisted path; `make deadcode` remains verification only. |
| S-02 | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | Unit-lane performance gate — three unit-lane wall samples plus package and test inventories | `make regenerate-shared-ci-baselines BASELINE_REGEN_ROOT=. UNIT_LATENCY_BUDGET="$UNIT_BUDGET_BASELINE" UNIT_LATENCY_SAMPLES=".artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json" BASELINE_REGEN_DEADCODE_REPORT="$DEADCODE_REPORT_PATH"` | Writer merged on main via PR #2347, merge commit `fee3da73388514cfb5975307d2cd1e07b345cd84`, through `cmd/unitlanebudget/regenerate.go:regenerateSharedBaselines`. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` after validating three complete hosted `github-actions`/`ubuntu-24.04` samples and stages only the allowlisted path; local timing runs are not substitutes. |
| S-03 | `docs/internal/baselines/ownership-inventory.json` | PSS-F01 ownership inventory — package path, destination mapping, named owner, and guard row | `go run ./cmd/ownershipinventoryfreeze` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The command deterministically writes S-03 together with S-04 through S-08; prove the result with `make ownership-inventory-check`. |
| S-04 | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | PSS-F01 path-lease freeze — packet ID, exclusive path, and active-lease overlap | `go run ./cmd/ownershipinventoryfreeze` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The command deterministically writes S-04 together with S-03 and S-05 through S-08; inspect the packet and lease changes and prove them with `make ownership-inventory-check`. |
| S-05 | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | Operator Settings ownership — root `.go` filename and contract/fold classification | `go run ./cmd/ownershipinventoryfreeze` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The command deterministically writes S-05 together with S-03, S-04, and S-06 through S-08; `go test ./internal/ownershipinventory -count=1` remains verification. |
| S-06 | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | Operator Settings ownership — immediate child directory and classification | `go run ./cmd/ownershipinventoryfreeze` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The command deterministically writes S-06 together with S-03 through S-05, S-07, and S-08; `go test ./internal/ownershipinventory -count=1` remains verification. |
| S-07 | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | Provider Sessions ownership — root `.go` filename and contract/fold classification | `go run ./cmd/ownershipinventoryfreeze` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The command deterministically writes S-07 together with S-03 through S-06 and S-08; `go test ./internal/ownershipinventory -count=1` remains verification. |
| S-08 | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | Provider Sessions ownership — immediate child directory and classification | `go run ./cmd/ownershipinventoryfreeze` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The command deterministically writes S-08 together with S-03 through S-07; `go test ./internal/ownershipinventory -count=1` remains verification. |
| S-09 | `contracts/testdata/baseline/cli-commands.json` | CLI command identity — production command path and stable identity candidate | `UPDATE_CLI_BASELINES=1 go test ./pkg/transports/cli/commandidentity -run '^TestWriteProductionInventoryBaseline$' -count=1` | Writer merged on the current implementation branch. Protected-main invokes this explicit update-mode test through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The test-owned writer is deliberate and review-gated; inspect the exact JSON diff before retaining it. |
| S-10 | `contracts/testdata/baseline/cli-command-inputs.json` | CLI input identity — command path, argument/flag position, name, and relationship | `UPDATE_CLI_BASELINES=1 go test ./pkg/transports/cli/cliinputs -run '^TestWriteProductionInputsInventoryBaseline$' -count=1` | Writer merged on the current implementation branch. Protected-main invokes this explicit update-mode test through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. The test-owned writer is deliberate and review-gated; inspect the exact JSON diff before retaining it. |
| S-11 | `contracts/testdata/baseline/mcp-tools.json` | MCP tool identity — tool name, ID candidate, description, input schema, and handler registration | `go run ./cmd/mcptoolinventorygen -root .` | Writer merged on the current implementation branch. Protected-main invokes it through `Makefile:regenerate-shared-ci-baselines` and stages the output only when it remains in the eleven-path allowlist. `mcpdiscoverygen` writes different discovery artifacts and is not this snapshot's writer; the focused baseline tests remain comparison verification. |

The classification intentionally separates backend dead-code from frontend
dead-code: S-01 has a self-maintaining writer on main through merged PR #2347,
while R-03's writer flag would accept new unused-code findings into a quality
gate. The coverage manifests are also ratchets: their generator flags produce
candidates, but unattended replacement could lower a package floor. The
remaining S-* writers are now merged on this implementation branch and are
invoked by the same protected-main target; this status update does not alter
any R-* procedure or required check.

### Unit identity reconciliation

The unit-lane budget contains two deliberately different inventory units that
must remain visible. The checked-in comparison file has **444 packages and
18,156 current test identities** at the audit commit. The accepted historical
sample evidence records **444 packages and 18,122 tests** in each of its three
captures. The latter is not silently substituted for the former: it is a
historical timing distribution, while the budget's `testInventory` is the
current final-mode identity set.

| Evidence source | Reproduction command | Unit and observed value | SHA-256 |
| --- | --- | --- | --- |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `make test-unit-latency-budget` | Current final-mode reference: 444 packages, 18,156 test identities | `7416c3fd88fd8c216d6466c4cf1ca9d66d8b797e375cd4eb6578003df348b6d3` |
| `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-summary.md` and its three linked `baseline-make-run-*.v2.json` captures | `go run ./cmd/unitlanebudget -mode baseline -samples docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-2.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-3.v2.json` | Historical baseline distribution: 444 packages, 18,122 tests per capture; the three wall samples are 222.006s, 239.612s, and 258.271s | Summary `801e62cbff17729f7c256309f058fc961ed0959a321de86e3783933049d43a93`; captures `ba7e1364ed5c88d66071d4cac4b2bf027571044ef7d159b16d25435f7fc95d8a`, `d30fdc0215d50a14c0a4cef65b234fde68a680e4015e37b5d9a463c9f361723f`, `e4288d9085e19ea3e7f8a87e0ad67ca52b38a255e0bc1e1a569ad59fbd008d98` |

The mapping is therefore **same package unit, different test-inventory
revision**: the historical captures support the accepted timing distribution;
the current budget preserves the full current identity set. Neither count is a
claim about the other, and future maintenance must identify which unit changed.

## Exclusion ledger

The reverse file-to-consumer pass also checked nearby files that contain
`baseline`, `inventory`, or `comparison` in their path. The following are
explicitly excluded because they are historical evidence, schemas, static
policy/compatibility documents, or behavioral fixtures rather than committed
repository-state comparisons. No excluded file is an unclassified inventory
row.

| Candidate path | Observed source or consumer | Evidence-backed exclusion |
| --- | --- | --- |
| `docs/internal/baselines/README.md`; `docs/internal/baselines/first-exemption-burn-down-baseline.md`; `docs/internal/baselines/fnd-12-public-behavior-baseline-suite-map.md`; `docs/internal/baselines/go-functional-coverage-variance-c74b3e27f.annotations.json`; `docs/internal/baselines/go-functional-coverage-variance-c74b3e27f.md`; `docs/internal/baselines/maturity-introspection-pr1006-recovery-baseline.md` | Documentation and archived evidence; only the FND-12 map is named by its Make aggregator | Catalog prose, historical PR evidence, a variance report/annotation, and a suite map are not comparison inputs consumed against current repository state. |
| `docs/internal/baselines/go-coverage-package-baseline.txt`; `docs/internal/baselines/go-functional-coverage-package-baseline.txt` | `cmd/gocoveragecheck/main.go` defaults for explicit `-package-baseline` invocations | Both are legacy compatibility inputs. Required unit and functional coverage targets pass `-package-manifest`, which makes `legacyPackageGateEnabled` false; no required workflow or Make path consumes either file as its active comparison. |
| `docs/internal/baselines/go-unit-lane-latency-budget.schema.json` | `cmd/unitlanebudget/budget.go` | JSON schema validation input only; the live sample comparison consumes `go-unit-lane-latency-budget.v1.json` and generated sample artifacts. |
| `docs/internal/projects/packaged-service-structure/path-lease-packet-manifest.json` | `internal/psslease/validate_test.go` | Static packet schema/catalog and lease-mechanics fixture. The required ownership check consumes the PSS-F01 `ownership-path-lease-freeze.json`, not this historical catalog as a live repository-state comparison. |
| `contracts/testdata/baseline/api-compatibility-surfaces.json` | `contracts/compatibility_inventory_coverage_test.go` and `contracts/api/deprecated.json` | Authored compatibility-policy inventory compared with another authored compatibility document; it does not scan live source or repository structure. |
| `contracts/testdata/baseline/mcp-result-policy.json` | `pkg/services/factory_sessions/transports/mcp/inventory_boundary_test.go` | Success/error envelope and representative tools/call behavior fixture. Its projection is protocol behavior, not repository-state inventory; the MCP identity inventory is separately included above. |
| `contracts/testdata/baseline/rest-operations.json` | No current required consumer found; current functional HTTP tests derive directly from `api/openapi.yaml` through `internal/contractinventory` | Retained historical REST inventory with no active required comparison consumer at this commit. |
| `pkg/transports/cli/baseline/testdata/command_tree.txt`; `docs_help.txt`; `docs_topic_index.txt`; `intentional_changes.json`; `intentional_changes.md`; `root_help.txt`; `run_flags.txt`; `run_help.txt` | `pkg/transports/cli/baseline` tests and `make fnd-12-*-behavior-baselines` | Customer-visible help, output, and intentional-change behavior fixtures. They are expressly the FND-12 behavioral suite, not the CLI command/input identity inventories included above. |
| `pkg/services/operator_settings/internal/services/document/identityinventory/testdata/baseline/system-config-input-index.json`; `pkg/services/operator_settings/testdata/baseline/operator-config-input-index.json` | Operator Settings contract and parity tests project documented input cases and loader outcomes | Static loader/input behavior matrices, not a scan of live repository files or package structure. |
| `pkg/services/workers/internal/interface/testdata/baseline/mock-workers-input-index.json`; `pkg/services/workers/internal/interface/testdata/baseline/mock-workers-topology.json` | `pkg/services/workers/internal/interface` inventory tests | Static mock-worker loader, schema, and topology behavior matrices; they do not compare a committed repository inventory with live source state. |
| `cmd/gocoveragecheck/testdata/empty-package-baseline.txt`; `internal/contractinventory/baseline_test.go`; `api/components/schemas/factory-world/FactoryWorldRunnerBaselineCapability.yaml` | Focused unit tests or OpenAPI schema authoring | Test-only fixture, extractor stability test source, and schema capability respectively; none is a required live repository-state comparison file. |
| `docs/internal/development/acp-baselines.md`; `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-*.json`; `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-summary.md`; archived UI/test-latency and factory-session baseline reports | Optional ACP capture commands and archived optimization/test-split documentation | Runbooks and historical performance/test evidence. They are not active required comparison inputs; raw capture output is intentionally not committed. |
| `ui/src/features/factory-session-detail/components/panel-behavior/factory-session-detail-panel.baseline.test.tsx`; `ui/src/features/factory-session-detail/components/test-support/factory-session-detail-panel.baseline-fixtures.ts`; source-only `*failure_baseline*` and `*baseline_fixture_test.go` files outside the inventory rows above | Focused UI and backend behavior tests | Behavior scenarios and test source names, not standalone committed repository-state comparison artifacts. |

The audit also observed optional `pkgboundarycheck` loader paths for
`service-cross-import-baseline.json`, `support-service-import-baseline.json`,
`test-behavior-boundary-baseline.json`, `production-default-selection-baseline.json`,
and `initializer-behavior-baseline.json` at the repository root. None exists in
the committed tree at this audit commit, so none is a comparison file or an
inventory row; their absence is recorded rather than inferred as a populated
zero-debt baseline.

Audit result at the named commit: 28 check-to-file entries reconciled with 28
file-to-check entries, zero duplicate rows, zero unclassified active comparison
files, and no unreadable or ambiguous active consumer. The two ledgers are
represented by the inventory and this exclusion ledger; future comparison files
must be added to exactly one inventory row before their check can be treated as
reconciled.

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
