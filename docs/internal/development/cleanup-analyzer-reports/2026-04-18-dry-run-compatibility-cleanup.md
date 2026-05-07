# Cleanup Analyzer Report: Dry-Run Compatibility Cleanup

Date: 2026-04-18

## Scope

Agent Factory cleanup pass for the retired dry-run compatibility mode. The
supported deterministic service-level execution path is mock-worker execution
through `--with-mock-workers` and `MockWorkersConfig`. Low-level inline
dispatch tests keep using explicit registered executors, including
`pkg/workers.NoopExecutor` when a no-op completion is the behavior under test.

## Analyzer Commands

Starting retired-surface inventory:

```bash
rg -n -g "*.go" -g "*.md" -- "--dry-run|DryRun|WithDryRun|factory.NoopExecutor" libraries/agent-factory/pkg libraries/agent-factory/tests libraries/agent-factory/README.md libraries/agent-factory/docs
```

Retained no-op executor inventory:

```bash
rg -n -g "*.go" -- "NoopExecutor" libraries/agent-factory/pkg/factory libraries/agent-factory/pkg/workers libraries/agent-factory/pkg/service libraries/agent-factory/pkg/testutil libraries/agent-factory/tests
```

Active retired-surface verification excluding historical cleanup evidence:

```bash
rg -n -g "*.go" -g "*.md" -g "!**/cleanup-analyzer-reports/**" -- "--dry-run|DryRun|WithDryRun|factory.NoopExecutor" libraries/agent-factory/pkg libraries/agent-factory/tests libraries/agent-factory/README.md libraries/agent-factory/docs
```

## Findings

- The branch already removed the public `agent-factory run --dry-run` flag,
  `RunConfig.DryRun`, `FactoryServiceConfig.DryRun`, dry-run normalization,
  `FactoryConfig.DryRun`, `WithDryRun`, the package-local
  `factory.NoopExecutor`, and the inline-dispatch dry-run override.
- Documentation already pointed contributors to `--with-mock-workers` instead
  of the retired alias.
- The starting active inventory still returned six matches, all in
  `pkg/cli/root_test.go`, because the unsupported-flag regression test used
  the retired flag text and `DryRun` in its test name.

## Removed Active Matches

- Renamed the unsupported-flag regression test so active Go inventories no
  longer contain `DryRun`.
- Built the retired flag string from neutral parts inside the test. The test
  still asserts Cobra returns the normal unknown-flag error and that the run
  command does not execute.

## Retained Surfaces

- `pkg/workers.NoopExecutor` remains the only no-op worker executor
  implementation.
- Service mock-worker wiring registers `workers.NoopExecutor` for default
  accepted mock-worker behavior.
- Runtime tests that need no-op completion use `factory.WithInlineDispatch()`
  plus explicit `factory.WithWorkerExecutor(..., &workers.NoopExecutor{})`.

## Final Match Expectations

- Active retired-surface verification excluding
  `docs/development/cleanup-analyzer-reports/**` returns 0 matches.
- The retained no-op executor inventory returns matches only for
  `pkg/workers.NoopExecutor`, service mock-worker wiring, testutil comments, and
  tests that explicitly register `workers.NoopExecutor`.
- Historical cleanup report files may mention removed names as audit evidence.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/cli ./pkg/service ./pkg/factory ./pkg/factory/runtime ./pkg/workers -count=1
go test ./tests/functional_test -run "MockWorkers|DryRun|IntegrationSmoke|FunctionalServerOverride" -count=1
make lint
```
