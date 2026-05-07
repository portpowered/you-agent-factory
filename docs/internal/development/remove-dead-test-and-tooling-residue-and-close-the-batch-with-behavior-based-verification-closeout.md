# Remove Dead Test And Tooling Residue Closeout

This closeout records the `US-005` cleanup on
`ralph/remove-deadcode-2026-may`.

## Cleaned Residue

- Removed six stale accepted deadcode-baseline entries for helper contracts that
  are still part of supported functional coverage:
  - `pkg/testutil.WithExecutionBaseDir`
  - `pkg/testutil.WriteSeedMarkdownFile`
  - `pkg/testutil.WriteSeedBatchFile`
  - `tests/functional/internal/support.CountFactoryEvents`
  - `tests/functional/internal/support.AcceptedCommandResults`
  - `tests/functional/internal/support.ProviderCommandRequestsForWorker`
- Added default-build helper tests so `cmd/deadcodecheck` can see those live
  helper contracts without depending on `functionallong`-tagged suites.

## Surviving Owners And Observable Proof

| Helper contract | Surviving owner | Observable proof |
| --- | --- | --- |
| `WithExecutionBaseDir` | `pkg/testutil/service_harness.go` | `TestWithExecutionBaseDir_SetsHarnessServiceConfig` proves the harness option still drives the service execution-base-dir seam, and `TestReplayEventStreamArtifactSmoke_ReplaysWithCopiedRootFactoryDefinition` proves the copied-factory replay path still resolves relative workstation execution from that base directory. |
| `WriteSeedMarkdownFile` | `pkg/testutil/testutil.go` | `TestWriteSeedMarkdownFile_WritesCanonicalMarkdownSeed` proves the helper still writes the supported markdown seed path, and `TestNamePropagation_MarkdownFile` proves the workflow lane still ingests that markdown watched-file input through the supported runtime flow. |
| `WriteSeedBatchFile` | `pkg/testutil/testutil.go` | `TestWriteSeedBatchFile_WritesCanonicalBatchSeed` proves the helper still writes canonical batch watched-file input, and `TestFileWatcherParentChildBatch_SubmittedFanInSmoke` proves the watched-file batch path still reaches the supported runtime behavior. |
| `CountFactoryEvents` | `tests/functional/internal/support/harness.go` | `TestCountFactoryEvents_CountsMatchingEventTypes` proves the shared functional helper still counts the canonical event stream correctly, and `TestMatchesFieldsGuard_IntegrationSmoke_GroupedExecution` proves the guard lane still verifies dispatch-request and dispatch-response behavior through emitted events. |
| `AcceptedCommandResults` and `ProviderCommandRequestsForWorker` | `tests/functional/internal/support/provider.go` | `TestAcceptedCommandResults_ReturnsRequestedCompleteResponses` and `TestProviderCommandRequestsForWorker_FiltersRecordedRequests` prove the shared workflow support contracts directly, and `TestDispatcherWorkflow_SingleSeedFile` proves the supported dispatcher workflow still observes planner, executor, and reviewer command activity through those helpers. |

## Verification

The lane was re-verified with the following commands:

```text
go test ./pkg/testutil ./tests/functional/internal/support -count=1
go test -tags=functionallong ./tests/functional/workflow -run "Test(NamePropagation_MarkdownFile|DispatcherWorkflow_SingleSeedFile)" -count=1 -timeout 300s
go test -tags=functionallong ./tests/functional/guards_batch -run "Test(MatchesFieldsGuard_IntegrationSmoke_GroupedExecution|FileWatcherParentChildBatch_SubmittedFanInSmoke)" -count=1 -timeout 300s
go test -tags=functionallong ./tests/functional/replay_contracts -run TestReplayEventStreamArtifactSmoke_ReplaysWithCopiedRootFactoryDefinition -count=1 -timeout 300s
make test
make typecheck
make lint
```

Expected outcome for this lane:

- The helper-package tests pass and keep the deadcode checker aware of the live
  default-build contracts.
- The targeted long functional tests pass and prove the supported replay,
  workflow, and watched-file behaviors still run through the surviving helper
  owners.
- `make lint` passes with the reduced accepted deadcode baseline.
