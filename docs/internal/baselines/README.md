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

This is the source-backed provenance inventory for required repository-state comparisons.
It was audited from a clean detached checkout of `origin/main` at commit
`022888528c6b3ae6192a9936db235bd4102fe597` on 2026-08-28.
The checkout had an empty `git status --porcelain`.
One row represents one committed comparison file.
Shared consumers do not collapse rows.
The source-first and reverse passes resolved to 28 active repository-state comparison files.

The set contains 18 files in this directory, five Packaged Service Structure live inventories,
and three contract or transport identity inventories.
It also contains one root boundary inventory and one UI style inventory.
The `Class` column is the exact join key to the maintenance classification below.

The evidence identity for each row is the SHA-256 of the committed comparison
content at that audit commit. The implementation source and command columns are
the read-only provenance edge. They do not authorize an update.

### Reproducible audit procedure

Run the audit from a clean detached checkout.

1. Fetch `origin/main` and create a detached worktree at the fetched revision.
2. Record `git rev-parse HEAD` and `git status --porcelain` before inspection.
3. Start with required jobs in `.github/workflows/ci.yml`.
4. Follow each job through Make or direct commands into its checker, script, or
   package-test source.
5. Record literal comparison paths, generator functions, and verification
   commands from those sources.
6. Reverse-trace every recorded path with
   `rg -n -F -- "$comparisonPath" --glob '!docs/internal/baselines/README.md'`.
7. Sort and deduplicate both ledgers, then compare their exact path sets.
8. Run each documented read-only consumer at the pinned revision.
9. Recompute each committed path's raw SHA-256 and scalar content counts.
10. Inspect hosted-only claims from the matching protected-main workflow run.

The independent path-name sweep is only a completeness witness. This command
returned 665 tracked candidates at the audit revision:

```text
git ls-files | rg -i 'baseline|inventory|comparison|budget|floor|minimum|manifest|freeze|golden|snapshot|allowlist|whitelist'
```

The source graph, rather than this keyword list, determines the active set.

| Source-first edge | Active comparison rows reached |
| --- | --- |
| `.github/workflows/ci.yml` `Backend Lint` → `make lint` → `LINT_TARGETS` | R-01, R-02, R-03, R-07 through R-16, S-01, S-03, and S-04 |
| `.github/workflows/ci.yml` `Backend Coverage` → `make test-unit-coverage` and `make functional-test-viz` | R-05 and R-06 |
| `.github/workflows/ci.yml` `Backend Unit Latency` → three hosted samples → `make test-unit-latency-budget` | S-02 |
| `.github/workflows/ci.yml` `Frontend Component` → `make ui-component-test` | R-17 |
| `make ci-verify-tests` → `make verify-tests` → `test-maintenance` | R-04, S-05, S-06, S-07, and S-08 |
| `make test-contract` and required backend unit package tests | S-09, S-10, and S-11 |
| `.github/workflows/regenerate-shared-ci-baselines.yml` → `make regenerate-shared-ci-baselines` | S-01 through S-11, subject to the shared allowlist |

The required S-04 witness starts at its ownership consumer. The path is
`make ownership-inventory-check` → `cmd/ownershipinventorycheck/main.go` →
`internal/ownershipinventory/gate.go` →
`internal/ownershipinventory/path_lease_freeze.go:PathLeaseFreezeRelativePath`
→ `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json`.
This source edge reaches S-04 even though its filename contains none of
`baseline`, `inventory`, or `comparison`.

### GATE-AUDIT bidirectional ledgers

The following two ledgers were produced independently. They are sorted by
repository path and show the same 28-path set.

| # | Source-to-file ledger | File-to-source ledger |
| ---: | --- | --- |
| 1 | `contracts/testdata/baseline/cli-command-inputs.json` | `contracts/testdata/baseline/cli-command-inputs.json` |
| 2 | `contracts/testdata/baseline/cli-commands.json` | `contracts/testdata/baseline/cli-commands.json` |
| 3 | `contracts/testdata/baseline/mcp-tools.json` | `contracts/testdata/baseline/mcp-tools.json` |
| 4 | `docs/internal/baselines/backend-exemption-budget.json` | `docs/internal/baselines/backend-exemption-budget.json` |
| 5 | `docs/internal/baselines/backend-package-file-count.json` | `docs/internal/baselines/backend-package-file-count.json` |
| 6 | `docs/internal/baselines/deadcode-baseline.txt` | `docs/internal/baselines/deadcode-baseline.txt` |
| 7 | `docs/internal/baselines/frontend-deadcode-baseline.json` | `docs/internal/baselines/frontend-deadcode-baseline.json` |
| 8 | `docs/internal/baselines/functional-undocumented-tests.json` | `docs/internal/baselines/functional-undocumented-tests.json` |
| 9 | `docs/internal/baselines/go-functional-coverage-package-minimums.json` | `docs/internal/baselines/go-functional-coverage-package-minimums.json` |
| 10 | `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `docs/internal/baselines/go-unit-coverage-package-minimums.json` |
| 11 | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` |
| 12 | `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` |
| 13 | `docs/internal/baselines/ownership-inventory.json` | `docs/internal/baselines/ownership-inventory.json` |
| 14 | `docs/internal/baselines/package-structure-baseline.json` | `docs/internal/baselines/package-structure-baseline.json` |
| 15 | `docs/internal/baselines/package-target-test-only-baseline.json` | `docs/internal/baselines/package-target-test-only-baseline.json` |
| 16 | `docs/internal/baselines/petri-public-surface-baseline.json` | `docs/internal/baselines/petri-public-surface-baseline.json` |
| 17 | `docs/internal/baselines/service-construction-baseline.json` | `docs/internal/baselines/service-construction-baseline.json` |
| 18 | `docs/internal/baselines/service-cycle-ceiling.json` | `docs/internal/baselines/service-cycle-ceiling.json` |
| 19 | `docs/internal/baselines/test-service-import-baseline.json` | `docs/internal/baselines/test-service-import-baseline.json` |
| 20 | `docs/internal/baselines/transport-behavior-baseline.json` | `docs/internal/baselines/transport-behavior-baseline.json` |
| 21 | `docs/internal/baselines/unfinished-package-moves.json` | `docs/internal/baselines/unfinished-package-moves.json` |
| 22 | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` |
| 23 | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` |
| 24 | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` |
| 25 | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` |
| 26 | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` |
| 27 | `ownership-boundary-baseline.json` | `ownership-boundary-baseline.json` |
| 28 | `ui/src/styles/palette-contrast-baseline.ts` | `ui/src/styles/palette-contrast-baseline.ts` |

GATE-AUDIT result: both ledgers contain 28 entries.
Both ledgers have zero duplicate rows and zero unclassified active comparison files.
Their set difference is empty at audit SHA `022888528c6b3ae6192a9936db235bd4102fe597`.
The consumer blocker in GATE-BLOCKER does not change this set result.

### Repository quality and coverage comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `docs/internal/baselines/backend-exemption-budget.json` | `make lint` → `backend-size`, `pkg-maint` | `internal/exemptionbudget/budget.go:Reconcile`, `cmd/backendsizecheck`, `cmd/pkgmaintcheck` | Backend quality gates | Directive identity and exemption-budget row | `make backend-size`, `make pkg-maint` | R-01 | `sha256:e3f3e6b60a34ad3f48cd9e497e405f0b63a6e663abc39d6e36bfc75f8d7bd8cc` |
| `docs/internal/baselines/backend-package-file-count.json` | `make lint` → `pkg-file-count` | `cmd/pkgfilecountcheck/main.go` | Backend package-shape gate | Package path and tracked Go file count | `make pkg-file-count` | R-02 | `sha256:c87c5eed895cfbab2a9923849f1678851f0f0853b98cca930bc3d2fc75c18cc3` |
| `docs/internal/baselines/deadcode-baseline.txt` | `make lint` → `deadcode` | `cmd/deadcodecheck/main.go` | Backend dead-code gate | Normalized unreachable-symbol identity | `make deadcode` | S-01 | `sha256:f31645c911b22d76e5a121e0da0c47d5549de16045e1d803e0003a254afdfe13` |
| `docs/internal/baselines/frontend-deadcode-baseline.json` | `make lint` → `ui-deadcode` | `ui/scripts/check-deadcode-baseline.ts` | Dashboard dead-code gate | Normalized Knip issue identity | `make ui-deadcode` | R-03 | `sha256:988791a647d158530d962cf9b6f03b187f381b97ed78d10bcad1439b3d6d2e5b` |
| `docs/internal/baselines/functional-undocumented-tests.json` | `make verify-tests` → `test-maintenance` | `internal/functionaltestmetadata/baseline_repo_test.go:TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests` | Functional test metadata | Relative test file and `Test*` name | `go test ./internal/functionaltestmetadata -run TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests -count=1` | R-04 | `sha256:b78da501e4a36a1b497be1f2a5f8988ac50706db38d0c0ace17b7eb5d553c8b6` |
| `docs/internal/baselines/go-functional-coverage-package-minimums.json` | `make functional-test-viz` → backend functional coverage | `cmd/functionaltestviz/main.go:packageManifestPath`, `Makefile:GO_FUNCTIONAL_COVERAGE_MANIFEST` | Functional coverage gate | Go import package and minimum statement-coverage percentage | `make functional-test-viz` | R-05 | `sha256:79e77099a74fe8065b9af69a53622303196353b77ef55d373def3f52adb12dc5` |
| `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `make test-unit-coverage` → backend unit coverage | `cmd/unitcoverage/main.go`, `Makefile:GO_UNIT_COVERAGE_MANIFEST` | Unit coverage gate | Go import package and minimum statement-coverage percentage | `make test-unit-coverage` | R-06 | `sha256:ee1faa58bad1f07903868c3eb791184ea299c10250cfddb5478adfbecb299eb3` |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `make test-unit-latency-budget` | `cmd/unitlanebudget/main.go`, `cmd/unitlanebudget/budget.go` | Unit-lane performance gate | Three unit-lane wall samples plus package and test inventories | `make test-unit-latency-budget` | S-02 | `sha256:b5f46eb81459d7d97f9958a174a195c4676b29e39f25933e65c76bdf8cc39711` |
| `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | `make lint` → `ui-lint` | `ui/scripts/check-hardcoded-ui-copy.ts`, `ui/package.json:check:localized-copy` | Dashboard localization gate | Source location and literal finding | `cd ui && bun run check:localized-copy` | R-07 | `sha256:bb6ee7e94bc96d013f164dc81004471112e457fa8a22d11ab17c1804982c609a` |
| `docs/internal/baselines/package-structure-baseline.json` | `make lint` → `pkg-structure` | `cmd/pkgstructurecheck/main.go` | Packaged Service Structure | Package path and exact recorded structure finding | `make pkg-structure` | R-08 | `sha256:a75bbca7df83dda6c422e8ff0b0892dc7d23b0a1ff96f03415677da9f4787a7e` |
| `docs/internal/baselines/package-target-test-only-baseline.json` | `make lint` → `package-target-manifest-check` | `cmd/packagetargetmanifestcheck/manifest.go` | Package-target migration gate | Open-move package path and test-only source identity | `make package-target-manifest-check` | R-09 | `sha256:64c98e7f5ee3b25d74bda79ee50a571b1b3a21e985946bc34688b330df5af40a` |
| `docs/internal/baselines/petri-public-surface-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/petri_public_surface.go` | Runtime public-boundary gate | File, symbol, kind, and migration identity | `make pkg-boundary` | R-10 | `sha256:07f84f9ce316d81ac4d80f13ec8435853274b3481136a8eb1db14e8f464599cc` |
| `docs/internal/baselines/service-construction-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/service_baselines.go` | Service construction boundary | Source file, import path, symbol, and class | `make pkg-boundary` | R-11 | `sha256:a6c401bb41481149a3576bb7b29aa42f4999de122c88b70520d3e848de259059` |
| `docs/internal/baselines/service-cycle-ceiling.json` | `make lint` → `service-cycle-check` | `cmd/servicecyclecheck/report.go` | Service dependency graph | Minimum feedback-arc-set weight of the service graph | `make service-cycle-check` | R-12 | `sha256:410eebddbff280ff10c346699cf4585877f5a752dd184901d24ef1595d1f764d` |
| `docs/internal/baselines/test-service-import-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/service_baselines.go` | Test service-boundary gate | Test source file, concrete import, and target service | `make pkg-boundary` | R-13 | `sha256:336548d5046747b3a823faf4cdca89648d67a3896ed3a5fd076f66131e9cc49a` |
| `docs/internal/baselines/transport-behavior-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/transport_behavior.go` | Transport boundary gate | Transport source, imported service, and behavior edge | `make pkg-boundary` | R-14 | `sha256:ce7868b58936d0cd6cd635a2ac9ca404baf007069573e395c77466393eeaa29e` |
| `docs/internal/baselines/unfinished-package-moves.json` | `make lint` → `ownership-inventory-check`, `package-target-manifest-check` | `internal/ownershipinventory/moves.go`, `cmd/packagetargetmanifestcheck/manifest.go` | Packaged Service Structure migration | Live `pkg/` package path and successor move row | `make ownership-inventory-check`, `make package-target-manifest-check` | R-15 | `sha256:21bb2cfc079413494f5d10b093b6f775fefa019385e6b4ec1ef95366e938f26f` |

### Ownership and live tree comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ownership-boundary-baseline.json` | `make lint` → `pkg-boundary` | `cmd/ownershipboundarycheck/main.go` | Root service-boundary gate | Boundary finding key and occurrence count | `make pkg-boundary` | R-16 | `sha256:b4705381e2f0dba798e15fd4e83c2ebd93792d0f4a73ba120549e934f70071e8` |
| `docs/internal/baselines/ownership-inventory.json` | `make lint` → `ownership-inventory-check` | `internal/ownershipinventory/load.go`, `internal/ownershipinventory/gate.go`, `cmd/ownershipinventorycheck/main.go` | PSS-F01 ownership inventory | Package path, destination mapping, named owner, and guard row | `make ownership-inventory-check` | S-03 | `sha256:c1456242093d400a743d63124270278ca086a0dc018f6844dc61cb188c687083` |
| `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | `make lint` → `ownership-inventory-check` | `internal/ownershipinventory/path_lease_freeze.go`, `internal/ownershipinventory/gate.go` | PSS-F01 path-lease freeze | Packet ID, exclusive path, and active-lease overlap | `make ownership-inventory-check` | S-04 | `sha256:48d54c6e0d5171f44737083a1a9f69821a6d70831789d711afab9d83e12da3eb` |
| `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/operator_settings_root_go.go` | Operator Settings ownership | Root `.go` filename and contract/fold classification | `go test ./internal/ownershipinventory -count=1` | S-05 | `sha256:7785c2c312f143ba5bceaa254b15c30e1068da0cc1f12dbf6379596ebd1a0890` |
| `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/operator_settings_top_level.go` | Operator Settings ownership | Immediate child directory and classification | `go test ./internal/ownershipinventory -count=1` | S-06 | `sha256:ff3fd338c5f22a1d51beb201bab752e0885c386c0c44031d7feb22e866521a20` |
| `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/provider_sessions_root_go.go` | Provider Sessions ownership | Root `.go` filename and contract/fold classification | `go test ./internal/ownershipinventory -count=1` | S-07 | `sha256:7a1fdbe169ec18d77c38d5f55489ca1398ebf2b71c1833cae062bf3cd747ae75` |
| `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | `make verify-tests` → `test-maintenance` | `internal/ownershipinventory/provider_sessions_top_level.go` | Provider Sessions ownership | Immediate child directory and classification | `go test ./internal/ownershipinventory -count=1` | S-08 | `sha256:d541d6aaf9ca5da509ac1982119531e2c2c49b1ec791d4662073d5d6f4b7c58a` |

### CLI, MCP, and UI identity comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `contracts/testdata/baseline/cli-commands.json` | `make test-contract` | `pkg/transports/cli/commandidentity/baseline_fixture_test.go`, `pkg/transports/cli/cliinputs/walk_production_test.go` | CLI command identity | Production command path and stable identity candidate | `go test ./pkg/transports/cli/commandidentity -run TestWalk_ProductionInventoryMatchesCommittedBaseline -count=1` | S-09 | `sha256:1bcd6b2e28272d01701a34e5082e773122656ebd644b205d8591183902ba858f` |
| `contracts/testdata/baseline/cli-command-inputs.json` | `make test-contract` | `pkg/transports/cli/cliinputs/walk_production_test.go` | CLI input identity | Command path, argument/flag position, name, and relationship | `go test ./pkg/transports/cli/cliinputs -run TestWalk_ProductionInventoryMatchesCommittedBaseline -count=1` | S-10 | `sha256:09331beded46c5557024f571916c3cdaa532a3ef9db732ca8aec544b2b3ff20d` |
| `contracts/testdata/baseline/mcp-tools.json` | Backend unit package tests in the required unit lane | `pkg/services/factory_sessions/transports/mcp/registry.go`, `pkg/services/factory_sessions/transports/mcp/registry_test.go` | MCP tool identity | Tool name, ID candidate, description, input schema, and handler registration | `go test ./pkg/services/factory_sessions/transports/mcp -run 'TestBaselineFixtureMatchesProjectedInventory|TestBaselineFixtureMatchesDiscoverToolsRegistry' -count=1` | S-11 | `sha256:98eff642b8b6bf442de1e150e13bdb89a6beab05c1fec3a24aa270690e9ffb33` |
| `ui/src/styles/palette-contrast-baseline.ts` | `make ui-component-test` → frontend component CI | `ui/src/styles/palette-contrast-ratchet.component.test.ts` | Dashboard styles | Palette, foreground token, fill token, and measured contrast ratio | `make ui-component-test` | R-17 | `sha256:e82d55905b1b523d8a5e2878bf9e94a2874ab55e6f3e0f1d9b93fcea5591f1cd` |

### Revision-pinned scalar observations

These values were parsed from committed files at the audit SHA. They are
content observations, not permission to rewrite a ratchet or snapshot.

| Class | Comparison file | Current scalar observation at `022888528c6b3ae6192a9936db235bd4102fe597` |
| --- | --- | --- |
| R-01 | `backend-exemption-budget.json` | 457 exemption-budget entries |
| R-02 | `backend-package-file-count.json` | 61 package-file-count entries |
| S-01 | `deadcode-baseline.txt` | 3,074 normalized findings |
| R-03 | `frontend-deadcode-baseline.json` | 30 accepted Knip issues |
| R-04 | `functional-undocumented-tests.json` | 330 committed undocumented-test identities |
| R-05 | `go-functional-coverage-package-minimums.json` | 357 packages and 29 floor holds |
| R-06 | `go-unit-coverage-package-minimums.json` | 403 packages and 33 floor holds |
| S-02 | `go-unit-lane-latency-budget.v1.json` | 444 packages and 18,223 final-mode test identities |
| R-07 | `hardcoded-ui-copy-baseline.txt` | 2 accepted literal findings |
| R-08 | `package-structure-baseline.json` | 532 structure findings |
| R-09 | `package-target-test-only-baseline.json` | 31 test-only migration findings |
| R-10 | `petri-public-surface-baseline.json` | 93 public-surface findings |
| R-11 | `service-construction-baseline.json` | 4 construction findings |
| R-12 | `service-cycle-ceiling.json` | Feedback-arc ceiling 13 |
| R-13 | `test-service-import-baseline.json` | 1 test-service import finding |
| R-14 | `transport-behavior-baseline.json` | 1 transport behavior finding |
| R-15 | `unfinished-package-moves.json` | 37 open move rows |
| R-16 | `ownership-boundary-baseline.json` | 26 boundary findings |
| S-03 | `ownership-inventory.json` | 6 named-owner confirmations and 0 misplaced guards |
| S-04 | `ownership-path-lease-freeze.json` | 2 path-lease packets |
| S-05 | `operator-settings-root-go-inventory.json` | 19 root Go files |
| S-06 | `operator-settings-top-level-inventory.json` | 4 top-level children |
| S-07 | `provider-sessions-root-go-inventory.json` | 2 root Go files |
| S-08 | `provider-sessions-top-level-inventory.json` | 3 top-level children |
| S-09 | `cli-commands.json` | 69 production commands |
| S-10 | `cli-command-inputs.json` | 37 arguments, 538 flags, and 11 relationships |
| S-11 | `mcp-tools.json` | 10 registered tools |
| R-17 | `palette-contrast-baseline.ts` | 8 measured palette entries |

## Maintenance classification

Every active inventory row above has exactly one class ID.
The `R-*` rows are manual ratchets recording an intentional one-way quality or migration commitment.
Review a current finding before changing the canonical file.
The `S-*` rows are snapshots.
They are deterministic projections of a specific repository observation.
Refresh them only with the named generator and named evidence.

The status terms below are deliberately narrow. **Merged** means that the
writer or generator is present in the audited `origin/main` revision. The
protected-main status is recorded separately so that a merged writer is not
mistaken for an unattended update path. PR #2347 merged as
`fee3da73388514cfb5975307d2cd1e07b345cd84`. The remaining S-* writers are
source-present at the same pinned revision. A verification test or an unrelated
artifact generator must not be relabelled as a maintenance mechanism.

### Manual ratchets

| Class | Comparison file | Owner and comparison unit | Safe update procedure | Generator status and one-way rule |
| --- | --- | --- | --- | --- |
| R-01 | `docs/internal/baselines/backend-exemption-budget.json` | Backend quality gates — directive identity and exemption-budget row | Run `make backend-size` and `make pkg-maint`. Remove a resolved directive. Add an entry only for a reviewed exception with an owner and removal reason. | Manual. No canonical writer. Automatic re-export would accept newly exempted debt. |
| R-02 | `docs/internal/baselines/backend-package-file-count.json` | Backend package-shape gate — package path and tracked Go file count | Run `make pkg-file-count`. When a package shrinks, lower or remove its row in the same reviewed change. Never raise a limit or add a row to hide growth. | Manual deletion-only ratchet. `cmd/pkgfilecountcheck` has no baseline writer. |
| R-03 | `docs/internal/baselines/frontend-deadcode-baseline.json` | Dashboard dead-code gate — normalized Knip issue identity | Run `make ui-deadcode`. Fix or remove the finding first. Use the script's `--update-baseline` writer only for an explicit, reviewed intentional exception. | Manual ratchet. Unattended `--update-baseline` would accept new dead code. |
| R-04 | `docs/internal/baselines/functional-undocumented-tests.json` | Functional test metadata — relative test file and `Test*` name | Run `go test ./internal/functionaltestmetadata -run TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests -count=1`. Document a test or remove a resolved row. Never add newly undocumented customer tests to the baseline. | Manual deletion-only ratchet. The comparison test has no canonical writer. |
| R-05 | `docs/internal/baselines/go-functional-coverage-package-minimums.json` | Functional coverage gate — Go import package and minimum statement-coverage percentage | Run `make functional-test-viz`. If a quality-floor change is approved, generate a candidate with `go run ./cmd/gocoveragecheck -suite functional -generate-manifest <candidate>`. Inspect it and manually apply only the approved change. | Candidate generation only. Never unattended-replace the manifest because current coverage could lower a quality floor. |
| R-06 | `docs/internal/baselines/go-unit-coverage-package-minimums.json` | Unit coverage gate — Go import package and minimum statement-coverage percentage | Run `make test-unit-coverage`. If a quality-floor change is approved, generate a candidate with `go run ./cmd/gocoveragecheck -suite unit -generate-manifest <candidate>`. Inspect it and manually apply only the approved change. | Candidate generation only. Never unattended-replace the manifest because current coverage could lower a quality floor. |
| R-07 | `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | Dashboard localization gate — source location and literal finding | Run `cd ui && bun run check:localized-copy`. Move the copy to the catalog. Update the baseline only for a reviewed intentional exception. Never use the writer unattended. | Manual ratchet. `--write-baseline` would accept new user-facing hardcoded copy. |
| R-08 | `docs/internal/baselines/package-structure-baseline.json` | Packaged Service Structure — package path and exact recorded structure finding | Run `make pkg-structure`. Resolve the finding, then remove only the resolved entry in a reviewed change. Do not add or retain a row to make a new violation pass. | Manual deletion-only ratchet. `-create-baseline` is bootstrap-only and refuses to overwrite an existing file. |
| R-09 | `docs/internal/baselines/package-target-test-only-baseline.json` | Package-target migration gate — open-move package path and test-only source identity | Run `make package-target-manifest-check`. Resolve the move or test-only edge, then remove its exact row. Do not add a new observation to make the migration check pass. | Manual deletion-only ratchet. `-create-test-only-baseline` refuses to overwrite an existing file. |
| R-10 | `docs/internal/baselines/petri-public-surface-baseline.json` | Runtime public-boundary gate — file, symbol, kind, and migration identity | Run `make pkg-boundary`. Resolve a public-surface migration finding, then remove only its reviewed row. Never add a new public-surface debt row. | Manual deletion-only ratchet. The `-create-petri-public-surface-baseline` path refuses an existing baseline. |
| R-11 | `docs/internal/baselines/service-construction-baseline.json` | Service construction boundary — source file, import path, symbol, and class | Run `make pkg-boundary`. Complete the construction migration, then remove the resolved row in the same reviewed change. Do not repin it to accept new construction debt. | Manual deletion-only ratchet. No canonical generator exists for this file. |
| R-12 | `docs/internal/baselines/service-cycle-ceiling.json` | Service dependency graph — minimum feedback-arc-set weight | Run `make service-cycle-check`. Change the ceiling only when the graph change is intentional and reviewed. An increase hides a regression. A decrease records an unclaimed improvement. | Manual bidirectional ratchet. The checker rejects both unreviewed increases and decreases. |
| R-13 | `docs/internal/baselines/test-service-import-baseline.json` | Test service-boundary gate — test source file, concrete import, and target service | Run `make pkg-boundary`. Move or remove the reviewed test-service edge, then delete its exact row. Do not add a new edge to the baseline. | Manual deletion-only ratchet. `-create-test-service-import-baseline` refuses to overwrite an existing file. |
| R-14 | `docs/internal/baselines/transport-behavior-baseline.json` | Transport boundary gate — transport source, imported service, and behavior edge | Run `make pkg-boundary`. Resolve the reviewed transport edge, then delete its exact row. Do not add a new behavior edge to the baseline. | Manual deletion-only ratchet. `-create-transport-behavior-baseline` refuses to overwrite an existing file. |
| R-15 | `docs/internal/baselines/unfinished-package-moves.json` | Packaged Service Structure migration — live `pkg/` package path and successor move row | Run `make ownership-inventory-check` and `make package-target-manifest-check`. Landing a move deletes its row. When no moves remain, delete the empty ledger with its consumers. | Manual shrink-only ledger. No generator exists because package ownership is derived from the live tree. |
| R-16 | `ownership-boundary-baseline.json` | Root service-boundary gate — boundary finding key and occurrence count | Run `make pkg-boundary`. Resolve the boundary finding, then remove or lower only the corresponding reviewed entry. Never raise accepted boundary debt. | Manual deletion-only ratchet. `-create-baseline` refuses to overwrite an existing file. |
| R-17 | `ui/src/styles/palette-contrast-baseline.ts` | Dashboard styles — palette, foreground token, fill token, and measured contrast ratio | Run `make ui-component-test`. Improve the measured contrast, then lower or remove the debt entry. Never raise the recorded debt to make a regression pass. | Manual ratchet. The component test has no baseline writer, and automatic recapture could record a regression. |

### Self-maintained snapshots

| Class | Comparison file | Owner and comparison unit | Deterministic generation command | Generator status and safe use |
| --- | --- | --- | --- | --- |
| S-01 | `docs/internal/baselines/deadcode-baseline.txt` | Backend dead-code gate — normalized unreachable-symbol identity | `make regenerate-shared-ci-baselines BASELINE_REGEN_ROOT=. UNIT_LATENCY_BUDGET="$UNIT_BUDGET_BASELINE" UNIT_LATENCY_SAMPLES=".artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json" BASELINE_REGEN_DEADCODE_REPORT="$DEADCODE_REPORT_PATH"` | Merged on main through PR #2347, merge commit `fee3da73388514cfb5975307d2cd1e07b345cd84`, using `cmd/unitlanebudget/regenerate.go:regenerateSharedBaselines`. The protected workflow validates the delivered normalized dead-code report, writes the eleven-path allowlist, and leaves `make deadcode` as verification only. |
| S-02 | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | Unit-lane performance gate — three unit-lane wall samples plus package and test inventories | `make regenerate-shared-ci-baselines BASELINE_REGEN_ROOT=. UNIT_LATENCY_BUDGET="$UNIT_BUDGET_BASELINE" UNIT_LATENCY_SAMPLES=".artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json" BASELINE_REGEN_DEADCODE_REPORT="$DEADCODE_REPORT_PATH"` | Merged on main through PR #2347, merge commit `fee3da73388514cfb5975307d2cd1e07b345cd84`, using `cmd/unitlanebudget/regenerate.go:regenerateSharedBaselines`. The protected workflow requires three complete hosted `github-actions`/`ubuntu-24.04` samples. Local timing runs are diagnostic only. |
| S-03 | `docs/internal/baselines/ownership-inventory.json` | PSS-F01 ownership inventory — package path, destination mapping, named owner, and guard row | `go run ./cmd/ownershipinventoryfreeze` | Merged writer present at the audit revision: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.WriteInventory`. The protected workflow invokes it through `Makefile:regenerate-shared-ci-baselines` and accepts the output only in the eleven-path allowlist. It writes S-03 with S-04 through S-08. |
| S-04 | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | PSS-F01 path-lease freeze — packet ID, exclusive path, and active-lease overlap | `go run ./cmd/ownershipinventoryfreeze` | Merged writer present at the audit revision: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.WritePathLeaseFreeze`. The protected workflow invokes it through the same eleven-path allowlist. It writes S-04 with S-03 and S-05 through S-08. Verify with `make ownership-inventory-check`. |
| S-05 | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | Operator Settings ownership — root `.go` filename and contract/fold classification | `go run ./cmd/ownershipinventoryfreeze` | Merged writer present at the audit revision: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.WriteSnapshotCandidates` for this output. The protected workflow invokes it through the same eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-06 | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | Operator Settings ownership — immediate child directory and classification | `go run ./cmd/ownershipinventoryfreeze` | Merged writer present at the audit revision: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.WriteSnapshotCandidates` for this output. The protected workflow invokes it through the same eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-07 | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | Provider Sessions ownership — root `.go` filename and contract/fold classification | `go run ./cmd/ownershipinventoryfreeze` | Merged writer present at the audit revision: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.WriteSnapshotCandidates` for this output. The protected workflow invokes it through the same eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-08 | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | Provider Sessions ownership — immediate child directory and classification | `go run ./cmd/ownershipinventoryfreeze` | Merged writer present at the audit revision: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.WriteSnapshotCandidates` for this output. The protected workflow invokes it through the same eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-09 | `contracts/testdata/baseline/cli-commands.json` | CLI command identity — production command path and stable identity candidate | `UPDATE_CLI_BASELINES=1 go test ./pkg/transports/cli/commandidentity -run '^TestWriteProductionInventoryBaseline$' -count=1` | Merged writer present at the audit revision: `pkg/transports/cli/commandidentity/baseline_fixture_test.go:TestWriteProductionInventoryBaseline`. The protected workflow invokes this explicit update-mode test through `Makefile:regenerate-shared-ci-baselines` and accepts only the eleven-path allowlist. Inspect the exact JSON diff before retaining it. |
| S-10 | `contracts/testdata/baseline/cli-command-inputs.json` | CLI input identity — command path, argument/flag position, name, and relationship | `UPDATE_CLI_BASELINES=1 go test ./pkg/transports/cli/cliinputs -run '^TestWriteProductionInputsInventoryBaseline$' -count=1` | Merged writer present at the audit revision: `pkg/transports/cli/cliinputs/walk_production_test.go:TestWriteProductionInputsInventoryBaseline`. The protected workflow invokes this explicit update-mode test through `Makefile:regenerate-shared-ci-baselines` and accepts only the eleven-path allowlist. Inspect the exact JSON diff before retaining it. |
| S-11 | `contracts/testdata/baseline/mcp-tools.json` | MCP tool identity — tool name, ID candidate, description, input schema, and handler registration | `go run ./cmd/mcptoolinventorygen -root .` | Merged writer present at the audit revision: `cmd/mcptoolinventorygen/main.go:run` calls `factorysession.GenerateToolInventoryJSON`. The protected workflow invokes it through `Makefile:regenerate-shared-ci-baselines` and accepts only the eleven-path allowlist. `mcpdiscoverygen` writes a different artifact and is not this writer. |

The protected-main path is `.github/workflows/regenerate-shared-ci-baselines.yml` →
`make regenerate-shared-ci-baselines` →
`scripts/ci/shared-baseline-regeneration-workflow.mjs`.
Its exact eleven-path allowlist contains S-01 through S-06 and S-07 through S-11.
The workflow checks out the delivered main revision.
It requires hosted unit-latency and normalized dead-code evidence.
It validates scope and opens a no-change result when no output diff exists.

S-01 and S-02 consume those hosted artifacts directly.
S-03 through S-11 are source projections generated in the same protected path.

The classification separates backend dead-code from frontend dead-code.
S-01 has a self-maintaining writer on main through merged PR #2347.
R-03's writer flag would accept new unused-code findings into a quality gate.
The coverage manifests are also ratchets.
Their generator flags produce candidates.
Unattended replacement could lower a package floor.

All snapshot writers named above are present at the audited main revision.
The same protected-main target invokes them.

This catalog does not alter any ratchet procedure, comparison file, or required check.

### GATE-CONSUMERS

The local-real audit used the documented commands in a disposable Windows checkout at the audit SHA.
The documented quality-gate commands passed with exit 0.
They included `make backend-size`, `make pkg-maint`, and `make pkg-file-count`.
They also included `make pkg-boundary`, `make pkg-structure`, and `make service-cycle-check`.

They included `make package-target-manifest-check`, `make ownership-inventory-check`,
and `go test ./internal/ownershipinventory -count=1`.
The focused S-09 and S-10 identity tests passed.
The focused S-11 MCP identity tests passed.

After `make ui-deps`, `make ui-deadcode` matched 30 accepted issues.
The full `make ui-component-test` run executed 1,079 tests with zero test failures.
It exited 1 because Windows wall time was 217.06 seconds against the 150.00-second budget.
This local runner limit does not authorize an R-17 edit.

The R-07 command `bun run check:localized-copy` also exited 0.

Four consumer observations remain explicit.
`make deadcode` returned exit 2 because the Windows report had 3,072 findings against the committed 3,074-line Linux snapshot.
The two extra snapshot identities and Unix-to-Windows file substitutions are local platform diagnostics.
They do not authorize a snapshot edit.

`make test-unit-latency-budget` returned exit 2 because the hosted `.artifacts/unit-latency/run-1.v2.json` input was absent.
No local timing run substitutes for that hosted artifact.
The full 75-minute functional coverage target was not rerun locally.
Its hosted job is recorded in GATE-HOSTED.

`go test ./internal/functionaltestmetadata -run
^TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests$ -count=1`
returned exit 1. It reports 330 committed undocumented-test identities and
296 identities discovered from current source. This is a current-main
comparison failure, not permission to shrink the ratchet in this audit.

`make test-contract` returned exit 1 at the same clean SHA. The focused S-09
and S-10 identity comparisons passed, but
`contracts/session_command_family_test.go:116` reported
`you.session.list --scope default = all, want live`. This is a separate
current-main contract defect. It is recorded without changing a fixture or
generated contract.

### GATE-HOSTED

The protected-main source path requires the hosted unit-latency artifacts and
normalized dead-code report before the shared generator can run. The matching
source CI observation was run 33228272233 for audit SHA
`022888528c6b3ae6192a9936db235bd4102fe597`.
The run URL is
https://github.com/portpowered/you-agent-factory/actions/runs/33228272233.
`gh run view 33228272233 --json databaseId,headSha,status,conclusion,url,jobs`
reported `success`. Backend Unit Latency retained three hosted samples and
enforced the latency budget. Backend Lint uploaded the normalized dead-code
evidence, and Verification Policy completed successfully.

The matching regeneration run was 33228682483 at the same main SHA.
The run URL is
https://github.com/portpowered/you-agent-factory/actions/runs/33228682483.
Its log recorded successful downloads of both hosted artifacts, the canonical
`make regenerate-shared-ci-baselines` path, allowlist validation, and
`SHARED_BASELINE_RECONCILIATION action=merge-requested publish=true`.
The generated candidate changed only
`docs/internal/baselines/go-unit-lane-latency-budget.v1.json`.
GATE-HOSTED passes for this audit revision. Local timing remains diagnostic.

### GATE-BLOCKER

| ID | Owning gate and owner | Reproduction and observed result | Smallest separate correction |
| --- | --- | --- | --- |
| C11-R04-001 | `GATE-CONSUMERS`, `internal/functionaltestmetadata` maintainers | At audit SHA `022888528c6b3ae6192a9936db235bd4102fe597`, run `go test ./internal/functionaltestmetadata -run ^TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests$ -count=1`. The checker returned exit 1 with 330 committed identities and 296 discovered identities. | Review the 34-entry set difference, document any still-undocumented test, and manually remove only resolved baseline rows. Rerun the checker in a separately owned correction. This lane makes no baseline edit. |
| C11-CONTRACT-002 | `GATE-CONSUMERS`, CLI contract owner | At audit SHA `022888528c6b3ae6192a9936db235bd4102fe597`, run `make test-contract`. The command returned exit 1 at `contracts/session_command_family_test.go:116`: `you.session.list --scope default = all, want live`. Focused S-09 and S-10 comparisons still passed. | Confirm the intended default at the contract and production-command sources, then correct the owning contract surface and rerun `make test-contract`. This lane makes no fixture or generated-file edit. |

These blockers remain assigned to separate correction lanes. The local dead-code,
component-runner, and missing latency-artifact results are environment
limitations. They are not combined with either current-main blocker and do not
justify changing a comparison file.

### GATE-DOC-REVIEW

The documentation checklist passed for this revision. The intended tracked
diff is one file, all 28 rows have one class, and protected literals retain
their exact paths and commands. Current and historical values are labeled,
manual ratchets retain shrink or repin review, and snapshot writers name their
protected path and allowlist. No typed surface changed.

### GATE-SCOPE-SECURITY-ROLLBACK

The audit evidence contains no credentials, tokens, payloads, or private
runtime output. The authorized implementation surface is only this README.
Reverting this one file restores the prior catalog and does not require a
generated-file, schema, baseline, or code rollback.

### GATE-LOOPBACK and GATE-PR-CI

GATE-LOOPBACK result: PASS for the final documentation content at delivered head
`b905584c77172e7fa23ffaa611a2988e2111ca20`. A second fresh detached checkout at
that head had empty `git status --porcelain`. The source-first traversal and
reverse row parser each produced 28 unique paths. Their set difference was
empty, with zero duplicate paths, zero duplicate classes, zero missing files,
and zero SHA mismatches. The S-04 witness reached `PathLeaseFreezeRelativePath`
from `make ownership-inventory-check` through `VerifyFreeze`. Recomputed scalar
values matched the catalog at audit SHA
`022888528c6b3ae6192a9936db235bd4102fe597`.

The loopback used the report shape from
`factory/docs/standards/validation-loopback-template.md`. Its project criteria
were:

| Criterion | Status | Evidence | Unproven edge |
| --- | --- | --- | --- |
| C1 source/file reconciliation | PASS | 28 unique paths matched in both directions | Future source drift |
| C2 S-04 and row provenance | PASS | Ownership consumer reached `PathLeaseFreezeRelativePath`, and 28 rows had source metadata | Future consumer changes |
| C3 revision-pinned hashes and scalars | PASS | All 28 raw SHA-256 values and scalar observations matched | Future artifact changes |
| C4 ratchet procedures | PASS | R-* rows retained deliberate shrink or manual-repin procedures | Semantic review of future procedures |
| C5 hosted writer evidence | PASS | Source run 33228272233 and regeneration run 33228682483 both matched audit SHA and succeeded | Future protected-main runs |
| C6 defect handling | PASS | R-04 and contract defects were reproduced and left unchanged | Separately owned corrections |
| C7 performance scope | PASS | Hosted latency remained authoritative, while local timing and UI wall time remained diagnostic | Future hosted samples |
| C8 security, scope, and rollback | PASS | Clean status, one README diff, no secrets, and one-file revert path | Review-time merge state |
| C9 documentation review | PASS | README and prose checks passed with no typed-surface change | Reviewer interpretation |
| C10 PR CI | BLOCKED | No PR existed before the implementation handoff | Required CI on final pushed head |
| C11 explicit status report | PASS | This table records PASS, FAIL, or BLOCKED for every criterion | None for this report |
| C12 implementation handoff | BLOCKED | Final push and PR creation remained after loopback capture | Review-owned terminal CI and merge |

The unchanged source was exercised in the preceding clean worktree at parent head
`8137e86303c00039ddc0e94728a31088d132d466`. `make ui-component-test` executed
1,079 tests with zero test failures and exited 1 because Windows wall time was
217.06 seconds against the 150.00-second budget. The final head changes only
this README, so the result remains a local runner limitation.

GATE-PR-CI is the implementation handoff gate. It becomes satisfied when the
final head is pushed, a PR is open, and required CI has started. Review owns
terminal CI, merge conflicts, and merge after that handoff.

### Unit identity reconciliation

The unit-lane budget contains two deliberately different inventory units that
must remain visible. The checked-in comparison file has **444 packages and
18,223 current test identities** at the audit commit. Its internal
`reference.baseCommit` is `177ebdd07a176863221f11410ab84fd075f1eb80`, the
historical source revision used for the accepted timing sample. It is not the
catalog audit SHA. The accepted historical sample evidence records **444
packages and 18,122 tests** in each of its three captures. The latter is not
silently substituted for the former: it is a historical timing distribution,
while the budget's `testInventory` is the current final-mode identity set.

| Evidence source | Reproduction command | Unit and observed value | SHA-256 |
| --- | --- | --- | --- |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `make test-unit-latency-budget` | Current final-mode reference at the audit SHA: 444 packages, 18,223 test identities. Artifact `reference.baseCommit` is historical `177ebdd07a176863221f11410ab84fd075f1eb80` | `b5f46eb81459d7d97f9958a174a195c4676b29e39f25933e65c76bdf8cc39711` |
| `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-summary.md` and its three linked `baseline-make-run-*.v2.json` captures | `go run ./cmd/unitlanebudget -mode baseline -samples docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-2.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-3.v2.json` | Historical baseline distribution: 444 packages, 18,122 tests per capture. The three wall samples are 222.006s, 239.612s, and 258.271s | Summary `801e62cbff17729f7c256309f058fc961ed0959a321de86e3783933049d43a93`, captures `ba7e1364ed5c88d66071d4cac4b2bf027571044ef7d159b16d25435f7fc95d8a`, `d30fdc0215d50a14c0a4cef65b234fde68a680e4015e37b5d9a463c9f361723f`, `e4288d9085e19ea3e7f8a87e0ad67ca52b38a255e0bc1e1a569ad59fbd008d98` |

The mapping is therefore **same package unit, different test-inventory
revision**. The historical captures support the accepted timing distribution.
The current budget preserves the full current identity set. Neither count is a
claim about the other, and future maintenance must identify which unit changed.

## Exclusion ledger

The reverse file-to-consumer pass also checked the 665 tracked paths returned
by the independent sweep for `baseline`, `inventory`, `comparison`, `budget`,
`floor`, `minimum`, `manifest`, `freeze`, `golden`, `snapshot`, `allowlist`, or
`whitelist`. The following are explicitly excluded because they are historical
evidence, schemas, static policy or compatibility documents, or behavioral
fixtures rather than committed repository-state comparisons. No excluded file
is an unclassified inventory row.

| Candidate path | Observed source or consumer | Evidence-backed exclusion |
| --- | --- | --- |
| `docs/internal/baselines/README.md`, `docs/internal/baselines/first-exemption-burn-down-baseline.md`, `docs/internal/baselines/fnd-12-public-behavior-baseline-suite-map.md`, `docs/internal/baselines/go-functional-coverage-variance-c74b3e27f.annotations.json`, `docs/internal/baselines/go-functional-coverage-variance-c74b3e27f.md`, `docs/internal/baselines/maturity-introspection-pr1006-recovery-baseline.md` | Documentation and archived evidence. Only the FND-12 map is named by its Make aggregator. | Catalog prose, historical PR evidence, a variance report/annotation, and a suite map are not comparison inputs consumed against current repository state. |
| `docs/internal/baselines/go-coverage-package-baseline.txt`, `docs/internal/baselines/go-functional-coverage-package-baseline.txt` | `cmd/gocoveragecheck/main.go` defaults for explicit `-package-baseline` invocations | Both are legacy compatibility inputs. Required unit and functional coverage targets pass `-package-manifest`, which makes `legacyPackageGateEnabled` false. No required workflow or Make path consumes either file as its active comparison. |
| `docs/internal/baselines/go-unit-lane-latency-budget.schema.json` | `cmd/unitlanebudget/budget.go` | JSON schema validation input only. The live sample comparison consumes `go-unit-lane-latency-budget.v1.json` and generated sample artifacts. |
| `docs/internal/projects/packaged-service-structure/path-lease-packet-manifest.json` | `internal/psslease/validate_test.go` | Static packet schema/catalog and lease-mechanics fixture. The required ownership check consumes the PSS-F01 `ownership-path-lease-freeze.json`, not this historical catalog as a live repository-state comparison. |
| `contracts/testdata/baseline/api-compatibility-surfaces.json` | `contracts/compatibility_inventory_coverage_test.go` and `contracts/api/deprecated.json` | Authored compatibility-policy inventory compared with another authored compatibility document. It does not scan live source or repository structure. |
| `contracts/testdata/baseline/mcp-result-policy.json` | `pkg/services/factory_sessions/transports/mcp/inventory_boundary_test.go` | Success/error envelope and representative tools/call behavior fixture. Its projection is protocol behavior, not repository-state inventory. The MCP identity inventory is separately included above. |
| `contracts/testdata/baseline/rest-operations.json` | No current required consumer found. Current functional HTTP tests derive directly from `api/openapi.yaml` through `internal/contractinventory` | Retained historical REST inventory with no active required comparison consumer at this commit. |
| `pkg/transports/cli/baseline/testdata/command_tree.txt`, `docs_help.txt`, `docs_topic_index.txt`, `intentional_changes.json`, `intentional_changes.md`, `root_help.txt`, `run_flags.txt`, `run_help.txt` | `pkg/transports/cli/baseline` tests and `make fnd-12-*-behavior-baselines` | Customer-visible help, output, and intentional-change behavior fixtures. They are expressly the FND-12 behavioral suite, not the CLI command/input identity inventories included above. |
| `pkg/services/operator_settings/internal/services/document/identityinventory/testdata/baseline/system-config-input-index.json`, `pkg/services/operator_settings/testdata/baseline/operator-config-input-index.json` | Operator Settings contract and parity tests project documented input cases and loader outcomes | Static loader/input behavior matrices, not a scan of live repository files or package structure. |
| `pkg/services/workers/internal/interface/testdata/baseline/mock-workers-input-index.json`, `pkg/services/workers/internal/interface/testdata/baseline/mock-workers-topology.json` | `pkg/services/workers/internal/interface` inventory tests | Static mock-worker loader, schema, and topology behavior matrices. They do not compare a committed repository inventory with live source state. |
| `cmd/gocoveragecheck/testdata/empty-package-baseline.txt`, `internal/contractinventory/baseline_test.go`, `api/components/schemas/factory-world/FactoryWorldRunnerBaselineCapability.yaml` | Focused unit tests or OpenAPI schema authoring | Test-only fixture, extractor stability test source, and schema capability respectively. None is a required live repository-state comparison file. |
| `docs/internal/development/acp-baselines.md`, `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-*.json`, `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-summary.md`, archived UI/test-latency and factory-session baseline reports | Optional ACP capture commands and archived optimization/test-split documentation | Runbooks and historical performance/test evidence. They are not active required comparison inputs. Raw capture output is intentionally not committed. |
| `ui/src/features/factory-session-detail/components/panel-behavior/factory-session-detail-panel.baseline.test.tsx`, `ui/src/features/factory-session-detail/components/test-support/factory-session-detail-panel.baseline-fixtures.ts`, source-only `*failure_baseline*` and `*baseline_fixture_test.go` files outside the inventory rows above | Focused UI and backend behavior tests | Behavior scenarios and test source names, not standalone committed repository-state comparison artifacts. |

The audit also observed optional `pkgboundarycheck` loader paths for
`service-cross-import-baseline.json`, `support-service-import-baseline.json`,
`test-behavior-boundary-baseline.json`, `production-default-selection-baseline.json`,
and `initializer-behavior-baseline.json` at the repository root. None exists in
the committed tree at this audit commit, so none is a comparison file or an
inventory row. Their absence is recorded rather than inferred as a populated
zero-debt baseline.

GATE-LOOPBACK must repeat the same exclusion review from a fresh delivered checkout.
At the named audit commit, both ledgers contained 28 entries.
They had zero duplicate rows and zero unclassified active comparison files.
No active consumer was unreadable or ambiguous.
Future comparison files must be added to exactly one inventory row before their check is treated as reconciled.

`backend-package-file-count.json` is an exact deletion-only ratchet. The package
file-count gate rejects new oversized packages, count increases, and entries
that were not lowered or removed when the corresponding package shrank.

For Packaged Service Structure FND-12, the maintainer-runnable public behavior
baseline suite map (CLI, HTTP, MCP, replay, visualization activation) lives in
[`fnd-12-public-behavior-baseline-suite-map.md`](./fnd-12-public-behavior-baseline-suite-map.md).
That map names focused Make/`go test` entry points and marks success vs
typed-failure coverage. It does not own PR #1262 CLI-manifest baselines.

`unfinished-package-moves.json` is the single ledger of unfinished Packaged Service Structure migration intent.
Each row names a `packagePath` under `pkg/` that still has to move.
Each row also names its `destination`, `successor`, and optional `deletionCondition`.
A package that stays where it already sits has **no row**.
Its owner is derived from the tree by `ownershipinventory.OwnerForPackage`.
Adding or removing a package inside an existing service requires no edit here.

The surviving check runs the other way.
A row naming an absent `packagePath` is stale and fails.
The ledger only shrinks.
Landing a move deletes its row.
When `moves` is empty, the file is deleted with its loaders and checks.

Both `ownership-inventory-check` and `package-target-manifest-check` read this file.
No second destination catalog is needed.

`package-target-test-only-baseline.json` is the exact deletion-only companion
for the package-target checker. It records only test-only source observations
for open move rows, with the source class included in each identity. A
test-only source never establishes production package liveness. New observations
and stale accepted entries both fail until the exact edge is reviewed.

`ownership-inventory.json` is the PSS-F01 frozen ownership inventory.
It no longer enumerates packages.
That responsibility moved to `unfinished-package-moves.json` above.
It freezes the closed destination vocabulary and the Process Edges exception.
It also freezes the `structures.md` seed services and additional current roots.
A cross-service edge table classifies distinct-owner production imports.

Classes include command, query, event, protocol composition, construction, lifecycle, and external effect.
Named-owner confirmations cover Providers, Provider Sessions, Operator Settings, System Bootstrap, Factory Visualization, and Recordings.
Reviewed nested-subservice maps allow no alternate top-level owners or further discovery.
The misplaced-guard burn-down covers standards, allowlists, package guards, baselines, and diagnostics.
Those guards still assign provider inference or hosted polling to Workers.
Replacement owners are Providers or Automations.

Process Edges remain restricted to construction or external effect.

Regenerate with `go run ./cmd/ownershipinventoryfreeze`.
Prove with `go test ./internal/ownershipinventory` or `make ownership-inventory-check`.

Owner and nested-subservice rationale cards cover authority, state store, lifecycle, consumers, transaction boundary, and failure recovery.
Large responsibility clusters are **not** baselines.
Public CLI, HTTP, MCP, replay, visualization, and behavior-test surface ownership are **not** baselines.
Constructor, datastore, lifecycle-role, and protocol-adapter ownership are **not** baselines.
Nothing counts or ratchets these design records.
A row per service would make adding a service a registration exercise.

They are published as design intent at [`docs/architecture/service-ownership-rationale.md`](../../architecture/service-ownership-rationale.md).
The document also records the destination-vocabulary rationale.
It records the deferred FND-06 Edges narrowing from the retired `docs/internal/packaged-service-structure/package-target-manifest.json`.

The initial path-lease freeze from that inventory lives at `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json`.
It reuses FND-10 `pss-path-lease-packet-manifest/v1` mechanics from `internal/psslease`.
It assigns exclusive changed-path leases for the ownership-inventory packet (`PSS-F01`).
It also covers the first PSS-F02 owner-boundary checker slice.
It rejects overlapping active leases.
It refuses CLI-manifest and provider-conductor portfolio holds.

Regenerate with the same freeze command.
Prove with `go test ./internal/ownershipinventory ./internal/psslease` or `make ownership-inventory-check`.
The combined verification gate (`ownershipinventory.VerifyFreeze`) proves completeness, stable sort order, and edge classifications.
It also proves named-owner coverage, the Process Edges exception, and non-overlapping active leases.
That check is part of `make lint`.

`functional-undocumented-tests.json` is an exact deletion-only ledger of
customer-facing `tests/functional` `Test*` identities that lack a conventional
Go-doc description. `internal/functionaltestmetadata` compares the current
undocumented customer set against that baseline: removals succeed, newly
undocumented customer tests and baseline expansions fail. Harness/internal
helpers are excluded from the ledger.
