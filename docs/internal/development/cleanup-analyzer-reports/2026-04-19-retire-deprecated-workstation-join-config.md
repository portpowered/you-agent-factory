# Cleanup Analyzer Report: Retire Deprecated Workstation Join Config

Date: 2026-04-19

## Scope

Agent Factory cleanup evidence for retiring the deprecated workstation-level
fan-in `join` authoring surface. The supported fan-in contract is per-input
guards on `WorkstationIO`. This report covers public config structs, OpenAPI
schemas, generated Go and UI API types, mapper and validator paths, replay and
clone handling, topology projection helpers, fixtures, stress coverage, and
active authoring documentation.

Historical cleanup reports under
`libraries/agent-factory/docs/development/cleanup-analyzer-reports/` are
excluded from active-symbol conclusions because reports intentionally preserve
earlier analyzer output.

## Analyzer Commands

The before snapshot uses merge-base
`44d67787105ef4cc55d24b5514e0545a37de925b`, the parent of this branch's first
join-retirement commit.

Before inventory command:

```powershell
git grep -n -E "\bJoinConfig\b|\bJoinRequire\b|\bJoinChildFailureConfig\b|applyJoinConfig|ruleJoinConfig|workstationJoin|WorkstationJoin|legacy-join|Deprecated workstation-level|workstations\[\*\]\.join" 44d67787105ef4cc55d24b5514e0545a37de925b -- libraries/agent-factory/pkg libraries/agent-factory/tests libraries/agent-factory/api libraries/agent-factory/ui/src libraries/agent-factory/docs ':!libraries/agent-factory/docs/development/cleanup-analyzer-reports'
```

After inventory command:

```powershell
rg -n "\bJoinConfig\b|\bJoinRequire\b|\bJoinChildFailureConfig\b|applyJoinConfig|ruleJoinConfig|workstationJoin|WorkstationJoin|legacy-join|Deprecated workstation-level|workstations\[\*\]\.join" libraries/agent-factory/pkg libraries/agent-factory/tests libraries/agent-factory/api libraries/agent-factory/ui/src libraries/agent-factory/docs -g "*.go" -g "*.yaml" -g "*.ts" -g "*.tsx" -g "*.md" -g "!**/cleanup-analyzer-reports/**"
```

Workstation authoring doc check:

```powershell
rg -n "workstation-level join|WorkstationJoin|JoinConfig|join property" libraries/agent-factory/docs/workstations.md
```

Final active-code smoke command:

```powershell
rg -n "\bJoinConfig\b|\bJoinRequire\b|\bJoinChildFailureConfig\b|applyJoinConfig|ruleJoinConfig|workstationJoin|WorkstationJoin|workstations\[\*\]\.join|Deprecated workstation-level" libraries/agent-factory/pkg libraries/agent-factory/tests libraries/agent-factory/api libraries/agent-factory/ui/src -g "*.go" -g "*.yaml" -g "*.ts" -g "*.tsx"
```

## Before Inventory

The branch baseline contained the retired symbols and warning text in active
code, generated contracts, tests, and fixtures:

| Term | Matches | Files |
|------|---------|-------|
| `JoinConfig` | 43 | 10 |
| `JoinRequire` | 41 | 12 |
| `JoinChildFailureConfig` | 9 | 5 |
| `applyJoinConfig` | 4 | 1 |
| `ruleJoinConfig` | 8 | 2 |
| `workstationJoin` | 6 | 2 |
| `WorkstationJoin` | 31 | 4 |
| `legacy-join` | 3 | 2 |
| `Deprecated workstation-level` | 4 | 3 |
| `workstations[*].join` | 0 | 0 |

The exact `workstations[*].join` warning text was not present in the baseline
as a literal string. Equivalent active warning and schema text existed as
`Deprecated workstation-level fan-in join configuration. Prefer per-input
guards on WorkstationIO.`

The before inventory showed these active surfaces:

- `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, and
  `ui/src/api/generated/openapi.ts` advertised `WorkstationJoin`,
  `WorkstationJoinRequire`, `WorkstationJoinChildFailure`, and a `join`
  property on `Workstation`.
- `pkg/interfaces/factory_config.go` exposed `FactoryWorkstationConfig.Join`,
  `JoinConfig`, `JoinRequire`, and `JoinChildFailureConfig`.
- `pkg/config/config_mapper.go` contained `applyJoinConfig(...)` and
  child-failure join mapping.
- `pkg/config/config_validator.go` registered and implemented
  `ruleJoinConfig`.
- `pkg/config/factory_config_mapping.go` converted between API and internal
  workstation join shapes with `workstationJoinInternalFromAPI(...)` and
  `workstationJoinAPIFromInternal(...)`.
- Mapper, validator, topology projection, replay, and stress tests still used
  join-shaped fixtures or skipped join-specific stress coverage.

## After Inventory

The after inventory command returns no active matches outside historical
cleanup reports.

Per-term active inventory after the cleanup:

| Term | Matches | Files |
|------|---------|-------|
| `JoinConfig` | 0 | 0 |
| `JoinRequire` | 0 | 0 |
| `JoinChildFailureConfig` | 0 | 0 |
| `applyJoinConfig` | 0 | 0 |
| `ruleJoinConfig` | 0 | 0 |
| `workstationJoin` | 0 | 0 |
| `WorkstationJoin` | 0 | 0 |
| `legacy-join` | 0 | 0 |
| `Deprecated workstation-level` | 0 | 0 |
| `workstations[*].join` | 0 | 0 |

The final active-code smoke command also returns no matches under
`libraries/agent-factory/pkg`, `libraries/agent-factory/tests`,
`libraries/agent-factory/api`, or `libraries/agent-factory/ui/src`.

`libraries/agent-factory/docs/workstations.md` contains no workstation-level
join guidance, so no authoring-doc edit was required.

## Removed Active Analyzer Residue

One boundary rejection helper and one test name still used the removed
generated-type name even though the behavior was only raw-field rejection. They
were renamed to neutral fan-in wording while preserving the assertion that a
raw `join` payload is rejected and that per-input guards remain the supported
representation.

## Validation Commands

Commands were run from `libraries/agent-factory` unless noted.

```bash
go test ./pkg/config -count=1
go test ./pkg/config ./pkg/replay ./tests/functional_test ./tests/stress -count=1
make lint
```

Results on 2026-04-19:

- `go test ./pkg/config -count=1` passed.
- `go test ./pkg/config ./pkg/replay ./tests/functional_test ./tests/stress -count=1` passed.
- `make lint` passed; `go vet ./...` completed and the deadcode baseline
  matched.
- From the repository root, `make lint-docs` passed.
- From the repository root, `make docs-check` passed.
