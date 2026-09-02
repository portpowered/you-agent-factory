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
`42eeee4472656b8290f798c36a5b8c871b24d7d0` on 2026-08-30.
The checkout had an empty `git status --porcelain`.
One row represents one committed comparison file.
Shared consumers do not collapse rows.
The source-first and reverse passes resolved to 30 active repository-state comparison files.

The set contains 19 files in this directory, one functional-test inventory,
five Packaged Service Structure live inventories, and three contract or transport identity inventories.
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
6. Reverse-trace every literal path with
   `rg -n -F --glob '!docs/internal/baselines/README.md' -- "$comparisonPath"`.
7. For source-built or relative paths, run the source-aware resolver witnesses
   below. Resolve each expression from its owning source, then verify the
   resolved path with `git ls-files --error-unmatch -- "$comparisonPath"`.
8. Sort and deduplicate both ledgers, then compare their exact path sets.
9. Run each documented read-only consumer at the pinned revision.
10. Recompute each committed path's raw SHA-256 and scalar content counts.
11. Inspect hosted-only claims from the matching protected-main workflow run.

The independent path-name sweep is only a completeness witness. This command
returned 673 tracked candidates at the audit revision:

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
| `Makefile:569-570` `functional-os-boundary-check` → `cmd/functionalosboundarycheck` | R-18 and R-19 |
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

The reverse pass has 28 literal path matches and two source-aware resolutions.
The two non-literal paths use the following reproducible witnesses:

A missing full-path literal is not an exclusion. Trace its owning source and
record the expression, base directory, resolver rule, resolved Git path, and
path-existence result.

| Class | Owning source and expression | Resolution and path verification |
| --- | --- | --- |
| R-04 | `internal/functionaltestmetadata/baseline_repo_test.go:20` assigns `baselinePath` with `filepath.Join(repoRoot, "docs", "internal", "baselines", "functional-undocumented-tests.json")`. | Resolve from `repoRoot`, normalize to the Git path, and confirm the source and committed path with the commands below. |
| R-17 | `ui/src/styles/palette-contrast-ratchet.component.test.ts:9` imports `./palette-contrast-baseline`. | Resolve the relative module from `ui/src/styles`, apply the repository's `.ts` module extension, and confirm the source and committed path with the commands below. |

Run the source-aware witnesses from the repository root. Normalize output paths
to Git separators before comparing ledger entries:

```text
rg -n -F --glob '*.go' -- 'functional-undocumented-tests.json' internal/functionaltestmetadata
git ls-files --error-unmatch -- docs/internal/baselines/functional-undocumented-tests.json
internal/functionaltestmetadata/baseline_repo_test.go:20: baselinePath := filepath.Join(repoRoot, "docs", "internal", "baselines", "functional-undocumented-tests.json")
docs/internal/baselines/functional-undocumented-tests.json

rg -n -F --glob '*.ts' -- 'from "./palette-contrast-baseline"' ui/src/styles
git ls-files --error-unmatch -- ui/src/styles/palette-contrast-baseline.ts
ui/src/styles/palette-contrast-ratchet.component.test.ts:9:import { PALETTE_CONTRAST_BASELINE } from "./palette-contrast-baseline";
ui/src/styles/palette-contrast-baseline.ts
```

The full-path literal lookup is not used for these two rows. The source
expression, resolver rule, and committed-path result form their reverse
ledger entries.

### GATE-AUDIT bidirectional ledgers

The following two ledgers were produced independently. They are sorted by
repository path and show the same 30-path set. The file-to-source ledger uses
literal matches for 28 rows and the source-aware witnesses above for two rows.

| # | Source-to-file ledger | File-to-source ledger |
| ---: | --- | --- |
| 1 | `contracts/testdata/baseline/cli-command-inputs.json` | `contracts/testdata/baseline/cli-command-inputs.json` |
| 2 | `contracts/testdata/baseline/cli-commands.json` | `contracts/testdata/baseline/cli-commands.json` |
| 3 | `contracts/testdata/baseline/mcp-tools.json` | `contracts/testdata/baseline/mcp-tools.json` |
| 4 | `docs/internal/baselines/backend-exemption-budget.json` | `docs/internal/baselines/backend-exemption-budget.json` |
| 5 | `docs/internal/baselines/backend-package-file-count.json` | `docs/internal/baselines/backend-package-file-count.json` |
| 6 | `docs/internal/baselines/deadcode-baseline.txt` | `docs/internal/baselines/deadcode-baseline.txt` |
| 7 | `docs/internal/baselines/frontend-deadcode-baseline.json` | `docs/internal/baselines/frontend-deadcode-baseline.json` |
| 8 | `docs/internal/baselines/functional-os-spawn-baseline.json` | `docs/internal/baselines/functional-os-spawn-baseline.json` |
| 9 | `docs/internal/baselines/functional-undocumented-tests.json` | `docs/internal/baselines/functional-undocumented-tests.json` |
| 10 | `docs/internal/baselines/go-functional-coverage-package-minimums.json` | `docs/internal/baselines/go-functional-coverage-package-minimums.json` |
| 11 | `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `docs/internal/baselines/go-unit-coverage-package-minimums.json` |
| 12 | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` |
| 13 | `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` |
| 14 | `docs/internal/baselines/ownership-inventory.json` | `docs/internal/baselines/ownership-inventory.json` |
| 15 | `docs/internal/baselines/package-structure-baseline.json` | `docs/internal/baselines/package-structure-baseline.json` |
| 16 | `docs/internal/baselines/package-target-test-only-baseline.json` | `docs/internal/baselines/package-target-test-only-baseline.json` |
| 17 | `docs/internal/baselines/petri-public-surface-baseline.json` | `docs/internal/baselines/petri-public-surface-baseline.json` |
| 18 | `docs/internal/baselines/service-construction-baseline.json` | `docs/internal/baselines/service-construction-baseline.json` |
| 19 | `docs/internal/baselines/service-cycle-ceiling.json` | `docs/internal/baselines/service-cycle-ceiling.json` |
| 20 | `docs/internal/baselines/test-service-import-baseline.json` | `docs/internal/baselines/test-service-import-baseline.json` |
| 21 | `docs/internal/baselines/transport-behavior-baseline.json` | `docs/internal/baselines/transport-behavior-baseline.json` |
| 22 | `docs/internal/baselines/unfinished-package-moves.json` | `docs/internal/baselines/unfinished-package-moves.json` |
| 23 | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` |
| 24 | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` |
| 25 | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` |
| 26 | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` |
| 27 | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` |
| 28 | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` |
| 29 | `ownership-boundary-baseline.json` | `ownership-boundary-baseline.json` |
| 30 | `ui/src/styles/palette-contrast-baseline.ts` | `ui/src/styles/palette-contrast-baseline.ts` |

GATE-AUDIT result: both ledgers contain 30 entries.
Both ledgers have zero duplicate rows and zero unclassified active comparison files.
The reverse ledger contains 28 literal matches and two source-aware resolutions
for R-04 and R-17. Their set difference is empty at audit SHA
`42eeee4472656b8290f798c36a5b8c871b24d7d0`.
The consumer blocker in GATE-BLOCKER does not change this set result.

### Repository quality and coverage comparisons

| Comparison file | Required consumer | Implementation source | Owning surface | Comparison unit | Read-only command | Class | Evidence identity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `docs/internal/baselines/backend-exemption-budget.json` | `make lint` → `backend-size`, `pkg-maint` | `internal/exemptionbudget/budget.go:Reconcile`, `cmd/backendsizecheck`, `cmd/pkgmaintcheck` | Backend quality gates | Directive identity and exemption-budget row | `make backend-size`, `make pkg-maint` | R-01 | `sha256:e3f3e6b60a34ad3f48cd9e497e405f0b63a6e663abc39d6e36bfc75f8d7bd8cc` |
| `docs/internal/baselines/backend-package-file-count.json` | `make lint` → `pkg-file-count` | `cmd/pkgfilecountcheck/main.go` | Backend package-shape gate | Package path and tracked Go file count | `make pkg-file-count` | R-02 | `sha256:c87c5eed895cfbab2a9923849f1678851f0f0853b98cca930bc3d2fc75c18cc3` |
| `docs/internal/baselines/deadcode-baseline.txt` | `make lint` → `deadcode` | `cmd/deadcodecheck/main.go` | Backend dead-code gate | Normalized unreachable-symbol identity | `make deadcode` | S-01 | `sha256:85d4df809d0d8789edb86837dd45da313c3c33e5b2779d076ef7628637272076` |
| `docs/internal/baselines/frontend-deadcode-baseline.json` | `make lint` → `ui-deadcode` | `ui/scripts/check-deadcode-baseline.ts` | Dashboard dead-code gate | Normalized Knip issue identity | `make ui-deadcode` | R-03 | `sha256:988791a647d158530d962cf9b6f03b187f381b97ed78d10bcad1439b3d6d2e5b` |
| `docs/internal/baselines/functional-os-spawn-baseline.json` | `make lint` → `functional-os-boundary-check` | `Makefile:569-570`, `cmd/functionalosboundarycheck/main.go`, `cmd/functionalosboundarycheck/scanner.go`, `cmd/functionalosboundarycheck/json.go`, `cmd/functionalosboundarycheck/model.go`, `cmd/functionalosboundarycheck/policy.go` | Functional-test OS-boundary gate | Package count ceiling plus stable OS-spawn site identity | `make functional-os-boundary-check` | R-18 | `sha256:28b22f4f35fabc4ca3e2343cbe57662431d7af9d2e222a329495dde4a0e565d1` |
| `docs/internal/baselines/functional-undocumented-tests.json` | `make verify-tests` → `test-maintenance` | `internal/functionaltestmetadata/baseline_repo_test.go:TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests` | Functional test metadata | Relative test file and `Test*` name | `go test ./internal/functionaltestmetadata -run TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests -count=1` | R-04 | `sha256:6fb6ce8170a2c0518acb65051d830a38a4d122c343475023112ab2ad2aeaa7b0` |
| `docs/internal/baselines/go-functional-coverage-package-minimums.json` | `make functional-test-viz` → backend functional coverage | `cmd/functionaltestviz/main.go:packageManifestPath`, `Makefile:GO_FUNCTIONAL_COVERAGE_MANIFEST` | Functional coverage gate | Go import package and minimum statement-coverage percentage | `make functional-test-viz` | R-05 | `sha256:cc5069ee2152659a502ec202eebbec917c29e1e82791393fc17cf7717288f407` |
| `docs/internal/baselines/go-unit-coverage-package-minimums.json` | `make test-unit-coverage` → backend unit coverage | `cmd/unitcoverage/main.go`, `Makefile:GO_UNIT_COVERAGE_MANIFEST` | Unit coverage gate | Go import package and minimum statement-coverage percentage | `make test-unit-coverage` | R-06 | `sha256:ee1faa58bad1f07903868c3eb791184ea299c10250cfddb5478adfbecb299eb3` |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `make test-unit-latency-budget` | `cmd/unitlanebudget/main.go`, `cmd/unitlanebudget/budget.go` | Unit-lane performance gate | Three unit-lane wall samples plus package and test inventories | `make test-unit-latency-budget` | S-02 | `sha256:6add0f6ef92dec8e91b0d1279f2a4cd7e9da411d14c648e19c1e06a5d1327fbe` |
| `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | `make lint` → `ui-lint` | `ui/scripts/check-hardcoded-ui-copy.ts`, `ui/package.json:check:localized-copy` | Dashboard localization gate | Source location and literal finding | `cd ui && bun run check:localized-copy` | R-07 | `sha256:bb6ee7e94bc96d013f164dc81004471112e457fa8a22d11ab17c1804982c609a` |
| `docs/internal/baselines/package-structure-baseline.json` | `make lint` → `pkg-structure` | `cmd/pkgstructurecheck/main.go` | Packaged Service Structure | Package path and exact recorded structure finding | `make pkg-structure` | R-08 | `sha256:83fa29f913faf932babb7593767e375e1499fc4e6f2a96290bbf74206edb635e` |
| `docs/internal/baselines/package-target-test-only-baseline.json` | `make lint` → `package-target-manifest-check` | `cmd/packagetargetmanifestcheck/manifest.go` | Package-target migration gate | Open-move package path and test-only source identity | `make package-target-manifest-check` | R-09 | `sha256:64c98e7f5ee3b25d74bda79ee50a571b1b3a21e985946bc34688b330df5af40a` |
| `docs/internal/baselines/petri-public-surface-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/petri_public_surface.go` | Runtime public-boundary gate | File, symbol, kind, and migration identity | `make pkg-boundary` | R-10 | `sha256:07f84f9ce316d81ac4d80f13ec8435853274b3481136a8eb1db14e8f464599cc` |
| `docs/internal/baselines/service-construction-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/service_baselines.go` | Service construction boundary | Source file, import path, symbol, and class | `make pkg-boundary` | R-11 | `sha256:a6c401bb41481149a3576bb7b29aa42f4999de122c88b70520d3e848de259059` |
| `docs/internal/baselines/service-cycle-ceiling.json` | `make lint` → `service-cycle-check` | `cmd/servicecyclecheck/report.go` | Service dependency graph | Minimum feedback-arc-set weight of the service graph | `make service-cycle-check` | R-12 | `sha256:410eebddbff280ff10c346699cf4585877f5a752dd184901d24ef1595d1f764d` |
| `docs/internal/baselines/test-service-import-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/service_baselines.go` | Test service-boundary gate | Test source file, concrete import, and target service | `make pkg-boundary` | R-13 | `sha256:336548d5046747b3a823faf4cdca89648d67a3896ed3a5fd076f66131e9cc49a` |
| `docs/internal/baselines/transport-behavior-baseline.json` | `make lint` → `pkg-boundary` | `cmd/pkgboundarycheck/transport_behavior.go` | Transport boundary gate | Transport source, imported service, and behavior edge | `make pkg-boundary` | R-14 | `sha256:ce7868b58936d0cd6cd635a2ac9ca404baf007069573e395c77466393eeaa29e` |
| `docs/internal/baselines/unfinished-package-moves.json` | `make lint` → `ownership-inventory-check`, `package-target-manifest-check` | `internal/ownershipinventory/moves.go`, `cmd/packagetargetmanifestcheck/manifest.go` | Packaged Service Structure migration | Live `pkg/` package path and successor move row | `make ownership-inventory-check`, `make package-target-manifest-check` | R-15 | `sha256:21bb2cfc079413494f5d10b093b6f775fefa019385e6b4ec1ef95366e938f26f` |
| `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` | `make lint` → `functional-os-boundary-check` | `Makefile:569-570`, `cmd/functionalosboundarycheck/main.go`, `cmd/functionalosboundarycheck/scanner.go`, `cmd/functionalosboundarycheck/json.go`, `cmd/functionalosboundarycheck/model.go`, `cmd/functionalosboundarycheck/policy.go` | Functional-test OS-boundary gate | Exact `siteId`, package/source/enclosing/occurrence metadata, verdict, assertion evidence, and conversion obligation | `make functional-os-boundary-check` | R-19 | `sha256:d4a93d94eaa3231b5ed10a4ecb7f22172883f6a418a9860657f56229181839b2` |

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

| Class | Comparison file | Current scalar observation at `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| --- | --- | --- |
| R-01 | `backend-exemption-budget.json` | 457 exemption-budget entries |
| R-02 | `backend-package-file-count.json` | 61 package-file-count entries |
| S-01 | `deadcode-baseline.txt` | 3,075 normalized findings |
| R-03 | `frontend-deadcode-baseline.json` | 30 accepted Knip issues |
| R-04 | `functional-undocumented-tests.json` | 356 committed undocumented-test identities; the current parser discovers 359 identities, so the consumer fails without a baseline edit |
| R-05 | `go-functional-coverage-package-minimums.json` | 363 packages and 29 floor holds |
| R-06 | `go-unit-coverage-package-minimums.json` | 403 packages and 33 floor holds |
| S-02 | `go-unit-lane-latency-budget.v1.json` | 445 packages and 18,286 final-mode test identities; stored `reference.baseCommit` `e39e614dab8a2ea31b49fda5b99ad3b9cd5ab0ce` is historical relative to this audit |
| R-07 | `hardcoded-ui-copy-baseline.txt` | 2 accepted literal findings |
| R-08 | `package-structure-baseline.json` | 505 structure findings |
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
| R-18 | `functional-os-spawn-baseline.json` | 14 functional packages and 33 static OS-spawn sites |
| R-19 | `c01-eligibility-inventory.json` | 812 test rows and 33 OS-spawn records, including 26 `INTENTIONAL-OS` and 7 `ACCIDENTAL-OS` records |

## Maintenance classification

Every active inventory row above has exactly one class ID.
The catalog contains 30 active rows: 19 manual ratchets and 11 snapshots.
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
| R-18 | `docs/internal/baselines/functional-os-spawn-baseline.json` | Functional-test OS-boundary maintainers — package count ceiling plus stable OS-spawn site identity | Run `make functional-os-boundary-check`. When a site is removed, lower the package ceiling and remove its site ID in the same reviewed change. When a site is intentionally added, review the source and paired `INTENTIONAL-OS` inventory admission before changing the baseline. Never raise a ceiling or admit a site automatically. | Manual ratchet. The checker has no baseline writer, and automatic recapture could accept a new OS-spawn site and weaken the boundary. |
| R-19 | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` | Functional-test OS-boundary maintainers — exact site identity and metadata, semantic verdict, assertion evidence, and conversion obligation | Run `make functional-os-boundary-check`. Reconcile each `osSpawnSites` record by exact `siteId` and non-line metadata. Treat source-line drift as informational only when every other field matches. Review the owning assertion before changing a verdict, evidence, or conversion obligation. Never recapture verdicts automatically. | Manual ratchet. The checker has no inventory writer, and automatic recapture could accept a new site, alter a semantic verdict, or erase a conversion obligation. |

### Self-maintained snapshots

| Class | Comparison file | Owner and comparison unit | Deterministic generation command | Generator status and safe use |
| --- | --- | --- | --- | --- |
| S-01 | `docs/internal/baselines/deadcode-baseline.txt` | Backend dead-code gate — normalized unreachable-symbol identity | `make regenerate-shared-ci-baselines BASELINE_REGEN_ROOT=. UNIT_LATENCY_BUDGET="$UNIT_BUDGET_BASELINE" UNIT_LATENCY_SAMPLES=".artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json" BASELINE_REGEN_DEADCODE_REPORT="$DEADCODE_REPORT_PATH"` | Merged on main through PR #2347, merge commit `fee3da73388514cfb5975307d2cd1e07b345cd84`, using `cmd/unitlanebudget/regenerate.go:regenerateSharedBaselines`. The protected workflow validates the delivered normalized dead-code report, writes the eleven-path allowlist, and leaves `make deadcode` as verification only. |
| S-02 | `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | Unit-lane performance gate — three unit-lane wall samples plus package and test inventories | `make regenerate-shared-ci-baselines BASELINE_REGEN_ROOT=. UNIT_LATENCY_BUDGET="$UNIT_BUDGET_BASELINE" UNIT_LATENCY_SAMPLES=".artifacts/unit-latency/run-1.v2.json,.artifacts/unit-latency/run-2.v2.json,.artifacts/unit-latency/run-3.v2.json" BASELINE_REGEN_DEADCODE_REPORT="$DEADCODE_REPORT_PATH"` | Merged on main through PR #2347, merge commit `fee3da73388514cfb5975307d2cd1e07b345cd84`, using `cmd/unitlanebudget/regenerate.go:regenerateSharedBaselines`. The protected workflow requires three complete hosted `github-actions`/`ubuntu-24.04` samples. Local timing runs are diagnostic only. |
| S-03 | `docs/internal/baselines/ownership-inventory.json` | PSS-F01 ownership inventory — package path, destination mapping, named owner, and guard row | `go run ./cmd/ownershipinventoryfreeze` | Deterministic writer: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.PublishSnapshotGroup` for this file and five sibling snapshots. The protected workflow uses the common eleven-path allowlist. |
| S-04 | `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json` | PSS-F01 path-lease freeze — packet ID, exclusive path, and active-lease overlap | `go run ./cmd/ownershipinventoryfreeze` | Deterministic writer: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.PublishSnapshotGroup` for this file and five sibling snapshots. The protected workflow uses the common eleven-path allowlist. Verify with `make ownership-inventory-check`. |
| S-05 | `docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json` | Operator Settings ownership — root `.go` filename and contract/fold classification | `go run ./cmd/ownershipinventoryfreeze` | Deterministic writer: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.PublishSnapshotGroup` for this file and five sibling snapshots. The protected workflow uses the common eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-06 | `docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json` | Operator Settings ownership — immediate child directory and classification | `go run ./cmd/ownershipinventoryfreeze` | Deterministic writer: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.PublishSnapshotGroup` for this file and five sibling snapshots. The protected workflow uses the common eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-07 | `docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json` | Provider Sessions ownership — root `.go` filename and contract/fold classification | `go run ./cmd/ownershipinventoryfreeze` | Deterministic writer: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.PublishSnapshotGroup` for this file and five sibling snapshots. The protected workflow uses the common eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
| S-08 | `docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json` | Provider Sessions ownership — immediate child directory and classification | `go run ./cmd/ownershipinventoryfreeze` | Deterministic writer: `cmd/ownershipinventoryfreeze/main.go:runAtRoot` calls `ownershipinventory.PublishSnapshotGroup` for this file and five sibling snapshots. The protected workflow uses the common eleven-path allowlist. Verify with `go test ./internal/ownershipinventory -count=1`. |
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

At the audit revision, `ownershipinventory.PublishSnapshotGroup` builds the
fixed ordered group S-03 through S-08. It validates all payloads before any
filesystem replacement. It preflights the root, destination directories, and
existing regular targets. It stages each payload beside its target, replaces
targets with destination-local backups, and restores every touched target when
a handled replacement fails. It reports cleanup failures and retains an
unrecoverable backup for diagnosis. This is failure-isolated handled-error
publication. It does not claim power-loss atomicity.

The group is S-03 `ownership-inventory.json`, S-04
`ownership-path-lease-freeze.json`, S-05 `operator-settings-root-go-inventory.json`,
S-06 `operator-settings-top-level-inventory.json`, S-07
`provider-sessions-root-go-inventory.json`, and S-08
`provider-sessions-top-level-inventory.json`. The protected workflow allowlist
contains S-01 through S-11 only; no R-* ratchet is an unattended generation
target.

S-01 and S-02 consume those hosted artifacts directly.
S-03 through S-11 are source projections generated in the same protected path.
R-18 and R-19 are outside the protected eleven-path snapshot allowlist.
The functional-OS checker reads both canonical files and writes neither.
Neither ratchet has a canonical writer or snapshot mechanism.

The classification separates backend dead-code from frontend dead-code.
S-01 has a self-maintaining writer on main through merged PR #2347.
R-03's writer flag would accept new unused-code findings into a quality gate.
The coverage manifests are also ratchets.
Their generator flags produce candidates.
Unattended replacement could lower a package floor.

All snapshot writers named above are present at the audited main revision.
The same protected-main target invokes them through the common allowlist.

This catalog does not alter any ratchet procedure, comparison file, or required check.

### GATE-CONSUMERS

The source-first and reverse audit used a clean disposable Windows checkout at
the catalog audit SHA. The pre-existing consumer observations below retain
their original historical pins where shown. Generated validation files were
not retained. The following commands passed with exit 0: `make
backend-size`, `make pkg-maint`, `make pkg-file-count`, `make pkg-boundary`,
`make pkg-structure`, and `make service-cycle-check`.

They included `make package-target-manifest-check`, `make ownership-inventory-check`,
and `go test ./internal/ownershipinventory -count=1`.
The focused S-09 and S-10 identity tests passed.
The focused S-11 MCP identity tests passed.

After `make ui-deps`, `make ui-deadcode` matched 30 accepted issues.
`make ui-component-test` exited 1 on Windows.
Its Vitest leg passed 1,079 tests.
Its Bun leg passed 855 tests and failed one test after `RangeError: Out of memory`.
This local runner failure does not authorize an R-17 edit.

The R-07 command `bun run check:localized-copy` also exited 0.

The following consumer observations remain explicit.
`make deadcode` returned outer Make exit 2. Its checker returned exit 1 because the Windows report had 3,072 findings against the committed 3,074-line Linux snapshot.
The platform-sensitive count and Unix-to-Windows file substitutions are local diagnostics.
They do not authorize a snapshot edit.

`make test-unit-latency-budget` returned outer Make exit 2. Its validator returned exit 1 because the hosted `.artifacts/unit-latency/run-1.v2.json` input was absent.
No local timing run substitutes for that hosted artifact.
`make test-unit-coverage` exited 0 and reported 80.7% overall coverage against
the 75.9% minimum, with zero gate failures. The committed R-06 manifest still
contains 403 package floors and 33 explicit floor holds; no manifest edit was
made.
The bounded local `make functional-test-viz` invocation was externally
cancelled after the declared 35-minute budget. The enclosing command returned
shell exit 1. The target did not complete, so no inner checker result or normal
Make exit code was observed. Its inspection-only timing summary recorded `complete=false`,
149 selected packages, and 113 observed packages: 102 pass, 7 fail, and 4
skip. The failures were `models/root_composition`, `providers`,
`recordings/root_composition`, `runtime_api`, `sessions/execution`,
`sessions/standalone` (600-second package limit), and
`transport/cli/commands`; two packages were in flight and 34 were unobserved.
This proves the command was exercised but does not prove R-05; the incomplete
coverage result and existing functional failures require hosted or owning-lane
follow-up. The timing summary was not committed, and no manifest edit was made.
Their hosted evidence remains separate.

`go test ./internal/functionaltestmetadata -run
^TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests$ -count=1`
returned exit 1. It reports 356 committed undocumented-test identities and
359 identities discovered from current source. This is a current-main
comparison failure, not permission to shrink the ratchet in this audit.

`make test-contract` returned outer Make exit 2 at the audit SHA. Its test
process returned exit 1. The focused S-09
and S-10 identity comparisons passed, but
`contracts/session_command_family_test.go:116` reported
`you.session.list --scope default = all, want live`. This is a separate
current-main contract defect. It is recorded without changing a fixture or
generated contract.

### GATE-HOSTED

The hosted evidence was inspected at audit SHA
`2b56dd815b4a2d6fea6c39d52db07e6b222f0b91`.
CI run [33314683256](https://github.com/portpowered/you-agent-factory/actions/runs/33314683256)
completed with `failure`.
Backend Lint [job 99265802724](https://github.com/portpowered/you-agent-factory/actions/runs/33314683256/job/99265802724)
and Backend Unit Latency [job 99265802703](https://github.com/portpowered/you-agent-factory/actions/runs/33314683256/job/99265802703)
completed with `success`.
Backend Functional Coverage [job 99265866917](https://github.com/portpowered/you-agent-factory/actions/runs/33314683256/job/99265866917)
failed in `tests/functional/providers/acp`, and Verification Policy
[job 99266840600](https://github.com/portpowered/you-agent-factory/actions/runs/33314683256/job/99266840600)
also failed.
The hosted functional result is therefore not a catalog-refresh authority.

The matching protected regeneration run
[33315083228](https://github.com/portpowered/you-agent-factory/actions/runs/33315083228)
completed with `success` at the same audit SHA.
Its [job 99266896097](https://github.com/portpowered/you-agent-factory/actions/runs/33315083228/job/99266896097)
downloaded the source artifacts, ran the canonical
`make regenerate-shared-ci-baselines` path, validated the eleven-path
allowlist, and recorded `SHARED_BASELINE_RECONCILIATION action=merge-requested publish=true`.
It generated only an S-02 candidate and recorded `quiescent=false` because
the source revision also contained non-baseline paths.
The candidate was not retained by this README-only lane.
Local timing remains diagnostic.

### GATE-BLOCKER

| ID | Owning gate and owner | Reproduction and observed result | Smallest separate correction |
| --- | --- | --- | --- |
| C12-R04-001 | `GATE-CONSUMERS`, `internal/functionaltestmetadata` maintainers | At audit SHA `42eeee4472656b8290f798c36a5b8c871b24d7d0`, run `go test ./internal/functionaltestmetadata -run ^TestCommittedBaselineMatchesCurrentUndocumentedCustomerTests$ -count=1`. The checker returned exit 1 with 356 committed identities and 359 discovered identities. | Review the three-entry set difference, document the tests or remove only resolved rows, and rerun the checker in a separately owned correction. This lane makes no baseline edit. |
| C12-CONTRACT-002 | `GATE-CONSUMERS`, CLI contract owner | At audit SHA `2b56dd815b4a2d6fea6c39d52db07e6b222f0b91`, run `make test-contract`. The command returned exit 1 at `contracts/session_command_family_test.go:116`: `you.session.list --scope default = all, want live`. Focused S-09 and S-10 identity comparisons passed. | Confirm the intended default at the contract and production-command sources, then correct the owning contract surface and rerun `make test-contract`. This lane makes no fixture or generated-file edit. |
| C12-FUNCTIONAL-003 | `GATE-HOSTED`, functional coverage owners | CI run `33314683256` at audit SHA `2b56dd815b4a2d6fea6c39d52db07e6b222f0b91` failed Backend Functional Coverage job `99265866917` in `tests/functional/providers/acp`; Verification Policy job `99266840600` also failed. | Diagnose the existing provider functional failure in its owning lane and rerun hosted CI. This catalog records the failure and does not edit a comparison file. |

These blockers remain assigned to separate correction lanes. The local dead-code,
component-runner, and missing latency-artifact results are environment
limitations. They are not combined with either current-main blocker and do not
justify changing a comparison file.
The bounded local functional run is likewise diagnostic only: its incomplete
35-minute result does not authorize an R-05 manifest edit.

### GATE-DOC-REVIEW

The documentation checklist passed for this revision. The intended tracked
diff is one file, all 30 rows have one class, and protected literals retain
their exact paths and commands. Current and historical values are labeled,
manual ratchets retain shrink or repin review, and snapshot writers name their
protected path and allowlist. No typed surface changed.

### GATE-SCOPE-SECURITY-ROLLBACK

The audit evidence contains no credentials, tokens, payloads, or private
runtime output. The authorized implementation surface is only this README.
Reverting this one file restores the prior catalog and does not require a
generated-file, schema, baseline, or code rollback.

### GATE-LOOPBACK and GATE-PR-CI

The implementation audit at immutable SHA
`42eeee4472656b8290f798c36a5b8c871b24d7d0` produced 30 unique source-ledger
paths and 30 unique reverse-ledger paths.
The reverse pass produced 28 literal matches and two source-aware resolutions
for R-04 and R-17.
Their set difference was empty, with zero duplicate paths, zero duplicate
classes, zero missing files, and zero SHA mismatches.
The S-04 witness reached `PathLeaseFreezeRelativePath` from
`make ownership-inventory-check` through `VerifyFreeze`.
Recomputed scalar values matched the catalog at that audit SHA.

Story 002 owns the clean final-head checkout and independent loopback report.
The prior c11 loopback and its run identities are historical evidence only.

The loopback used the report shape from
`factory/docs/standards/validation-loopback-template.md`. Its project criteria
were:

| Criterion | Status | Evidence | Unproven edge |
| --- | --- | --- | --- |
| C1 source/file reconciliation | PASS | 30 source paths matched 28 literal reverse matches and two source-aware resolutions | Future source drift |
| C2 S-04 and row provenance | PASS | Ownership consumer reached `PathLeaseFreezeRelativePath`, and R-04/R-17 had resolver witnesses alongside the 30 row records | Future consumer changes |
| C3 revision-pinned hashes and scalars | PASS | All 30 raw SHA-256 values and scalar observations matched | Future artifact changes |
| C4 ratchet procedures | PASS | R-* rows retained deliberate shrink or manual-repin procedures | Semantic review of future procedures |
| C5 hosted writer evidence | PASS | Protected regeneration run 33315083228 matched the prior catalog audit SHA, validated the allowlist, and produced only the S-02 candidate | Future protected-main runs |
| C6 defect handling | PASS | R-04, contract, platform deadcode, missing latency artifact, and hosted functional failures were reproduced and left unchanged | Separately owned corrections |
| C7 performance scope | PASS | Hosted latency remained authoritative, while local timing and UI wall time remained diagnostic | Future hosted samples |
| C8 security, scope, and rollback | PASS | Clean status, one README diff, no secrets, and one-file revert path | Review-time merge state |
| C9 documentation review | PASS | README and prose checks passed with no typed-surface change | Reviewer interpretation |
| C10 PR CI | BLOCKED | No PR exists for this implementation head | Required CI on final pushed head |
| C11 explicit status report | PASS | This table records PASS, FAIL, or BLOCKED for every criterion | None for this report |
| C12 implementation handoff | BLOCKED | Final push and PR creation remained after loopback capture | Review-owned terminal CI and merge |

The final clean-room checkout and validation report belong to story 002.
The current local UI result is recorded in GATE-CONSUMERS.

GATE-PR-CI is the implementation handoff gate. It becomes satisfied when the
final head is pushed, a PR is open, and required CI has started. Review owns
terminal CI, merge conflicts, and merge after that handoff.

### Unit identity reconciliation

The unit-lane budget contains two deliberately different inventory units that
must remain visible. The checked-in comparison file has **445 packages and
18,286 current test identities** at the audit commit. Its internal
`reference.baseCommit` is `e39e614dab8a2ea31b49fda5b99ad3b9cd5ab0ce`, the
historical source revision used for the accepted timing sample. It is not the
catalog audit SHA. The accepted historical sample evidence records **444
packages and 18,122 tests** in each of its three captures. The latter is not
silently substituted for the former: it is a historical timing distribution,
while the budget's `testInventory` is the current final-mode identity set.

| Evidence source | Reproduction command | Unit and observed value | SHA-256 |
| --- | --- | --- | --- |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `make test-unit-latency-budget` | Current final-mode reference at the audit SHA: 445 packages, 18,286 test identities. Stored `reference.baseCommit` is historical `e39e614dab8a2ea31b49fda5b99ad3b9cd5ab0ce` | `6add0f6ef92dec8e91b0d1279f2a4cd7e9da411d14c648e19c1e06a5d1327fbe` |
| `docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-summary.md` and its three linked `baseline-make-run-*.v2.json` captures | `go run ./cmd/unitlanebudget -mode baseline -samples docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-2.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-3.v2.json` | Historical baseline distribution: 444 packages, 18,122 tests per capture. The three wall samples are 222.006s, 239.612s, and 258.271s | Summary `801e62cbff17729f7c256309f058fc961ed0959a321de86e3783933049d43a93`, captures `ba7e1364ed5c88d66071d4cac4b2bf027571044ef7d159b16d25435f7fc95d8a`, `d30fdc0215d50a14c0a4cef65b234fde68a680e4015e37b5d9a463c9f361723f`, `e4288d9085e19ea3e7f8a87e0ad67ca52b38a255e0bc1e1a569ad59fbd008d98` |

The mapping is therefore **same package unit, different test-inventory
revision**. The historical captures support the accepted timing distribution.
The current budget preserves the full current identity set. Neither count is a
claim about the other, and future maintenance must identify which unit changed.

## Exclusion ledger

The reverse file-to-consumer pass also checked the 673 tracked paths returned
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
At the named audit commit, both ledgers contained 30 entries.
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
