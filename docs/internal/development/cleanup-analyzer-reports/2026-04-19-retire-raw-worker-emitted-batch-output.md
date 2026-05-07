# Cleanup Analyzer Report: Retire Raw Worker-Emitted Batch Output

Date: 2026-04-19

## Scope

Agent Factory cleanup evidence for retiring raw worker-emitted
`FACTORY_REQUEST_BATCH` output as a generated fanout contract. Accepted worker
output can create `GeneratedBatches` only through the generated-batch envelope:

```json
{
  "request": { "type": "FACTORY_REQUEST_BATCH" },
  "metadata": {},
  "submissions": []
}
```

The removed worker-output shape was a raw top-level request object:

```json
{
  "type": "FACTORY_REQUEST_BATCH",
  "works": []
}
```

Raw top-level `FACTORY_REQUEST_BATCH` payloads remain in scope for documented
public submit boundaries such as API, file watcher, examples, and functional
public-ingress tests. This cleanup does not remove that legacy public ingress.

Historical cleanup reports under
`libraries/agent-factory/docs/development/cleanup-analyzer-reports/` are
excluded from active-symbol conclusions because reports intentionally preserve
earlier analyzer output.

## Analyzer Commands

The before snapshot used merge-base
`929e59dff1813bf1f68acccf4e005697ec819ad6`, which is the parent of the first
cleanup commit on this branch. The snapshot was created with:

```powershell
$base = git merge-base origin/main HEAD
$before = Join-Path $env:TEMP "portos-raw-worker-before-$base"
Remove-Item -Recurse -Force $before -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $before | Out-Null
git archive $base libraries/agent-factory | tar -xf - -C $before
```

For this run, `$before` resolved to:

```text
C:\Users\andre\AppData\Local\Temp\portos-raw-worker-before-929e59dff1813bf1f68acccf4e005697ec819ad6\libraries\agent-factory
```

Before inventory commands:

```powershell
rg -n "workerEmittedBatchWork" $before -g "*.go"
rg -n "legacyEnvelope" $before\pkg\factory\subsystems -g "*.go"
rg -n "GeneratedSubmissionBatch" $before\pkg $before\tests\functional_test -g "*.go"
rg -n "FACTORY_REQUEST_BATCH" $before -g "*.go" -g "*.md" -g "*.json" -g "!docs/development/cleanup-analyzer-reports/**"
```

After inventory commands:

```powershell
rg -n "workerEmittedBatchWork" libraries/agent-factory -g "*.go"
rg -n "legacyEnvelope" libraries/agent-factory/pkg/factory/subsystems -g "*.go"
rg -n "GeneratedSubmissionBatch" libraries/agent-factory/pkg libraries/agent-factory/tests/functional_test -g "*.go"
rg -n "FACTORY_REQUEST_BATCH" libraries/agent-factory -g "*.go" -g "*.md" -g "*.json" -g "!docs/development/cleanup-analyzer-reports/**"
```

Focused worker-output guard commands:

```powershell
rg -n -F 'output := `{"type":"FACTORY_REQUEST_BATCH"' libraries/agent-factory/pkg/factory/subsystems libraries/agent-factory/tests/functional_test -g "*.go"
rg -n "workerEmittedBatchWork|NormalizeGeneratedSubmissionBatch|NormalizeWorkRequest" libraries/agent-factory/pkg/factory/subsystems/subsystem_transitioner.go
```

## Before Inventory

The before snapshot contained:

- `workerEmittedBatchWork`: 2 matches in 1 file.
- `legacyEnvelope`: 3 matches in 1 file.
- `GeneratedSubmissionBatch`: 45 matches in 9 files.
- `FACTORY_REQUEST_BATCH`: 79 matches in 22 files.

The raw worker-output fallback lived in
`pkg/factory/subsystems/subsystem_transitioner.go`. After the envelope parse
path, the function unmarshaled the whole worker output into `legacyEnvelope`
and accepted a raw top-level `type == FACTORY_REQUEST_BATCH` object.

The before test inventory preserved raw worker-output compatibility in active
worker-output fixtures:

- `pkg/factory/subsystems/subsystem_transitioner_test.go` used raw top-level
  worker output for generated fanout happy paths.
- `tests/functional_test/record_replay_end_to_end_test.go` used raw top-level
  worker output for the record/replay generated-batch scenario.

## After Inventory

The current branch contains:

- `workerEmittedBatchWork`: 2 matches in 1 file.
- `legacyEnvelope`: 0 matches under
  `libraries/agent-factory/pkg/factory/subsystems`.
- `GeneratedSubmissionBatch`: 45 matches in 9 files.
- `FACTORY_REQUEST_BATCH`: 80 matches in 22 files, excluding historical
  cleanup reports.

`workerEmittedBatchWork(...)` now detects generated worker fanout only by
extracting `request` from the generated-batch envelope. The function decodes the
full envelope only after the nested `request.type` is
`FACTORY_REQUEST_BATCH`, applies deterministic request ID generation, parent
enrichment, metadata defaults, and calls
`factory.NormalizeGeneratedSubmissionBatch(...)`.

The transitioner no longer calls `NormalizeWorkRequest(...)` from the
worker-output generated-fanout path. The focused command returned only:

```text
173: generatedBatch, detectedBatch, batchErr := t.workerEmittedBatchWork(resolved, inputColors)
425: func (t *TransitionerSubsystem) workerEmittedBatchWork(...)
472: normalized, err := factory.NormalizeGeneratedSubmissionBatch(...)
```

## Remaining FACTORY_REQUEST_BATCH Classification

The remaining `FACTORY_REQUEST_BATCH` matches are expected and fall into these
categories:

- Canonical generated-batch envelope use:
  `pkg/factory/subsystems/subsystem_transitioner_test.go`,
  `tests/functional_test/factory_request_batch_test.go`, and
  `tests/functional_test/record_replay_end_to_end_test.go` use
  `{"request":{"type":"FACTORY_REQUEST_BATCH", ...}}` for worker-generated
  fanout.
- Negative raw worker-output coverage:
  `pkg/factory/subsystems/subsystem_transitioner_test.go` intentionally emits
  one raw top-level `{"type":"FACTORY_REQUEST_BATCH", ...}` worker-output
  fixture and asserts it remains ordinary accepted output with no
  `GeneratedBatches`.
- Public submit-boundary coverage:
  `pkg/api/server_test.go`, `pkg/api/generated_contract_test.go`,
  `pkg/listeners/filewatcher_test.go`,
  `tests/functional_test/factory_request_batch_test.go`, sample input JSON, and
  example files retain documented public raw `FACTORY_REQUEST_BATCH` submit
  behavior.
- Interface constants:
  `pkg/interfaces/factory_runtime.go` and
  `pkg/api/generated/server.gen.go` define generated or domain constants for
  the public request type.
- Engine generated-batch handling:
  `pkg/factory/work_request.go`, `pkg/factory/factory_event_test.go`, and the
  `GeneratedSubmissionBatch` inventory preserve canonical normalization and
  event-recording coverage for `WORK_REQUEST`, `WORK_INPUT`, and generated
  batch metadata.
- Historical documentation and fixtures:
  `docs/authoring-workflows.md`, `docs/work.md`,
  `docs/development/factory-batch-input.md`,
  `docs/development/development.md`, `tests/adhoc/**`, and
  `tests/functional_test/testdata/**` preserve public examples, historical
  event logs, or development context.

No active happy-path worker-output test fixture emits raw top-level
`FACTORY_REQUEST_BATCH` JSON. The only active raw top-level worker-output match
from the focused guard command is the negative transitioner test.

## Validation Commands

Commands were run from `libraries/agent-factory` unless noted.

```bash
go test ./pkg/factory/subsystems ./pkg/factory/engine ./pkg/factory/runtime -count=1
go test ./tests/functional_test -run "FactoryRequestBatch|RecordReplay" -count=1
make lint
```

Results on 2026-04-19:

- `go test ./pkg/factory/subsystems ./pkg/factory/engine ./pkg/factory/runtime -count=1` passed.
- `go test ./tests/functional_test -run "FactoryRequestBatch|RecordReplay" -count=1` passed.
- `make lint` passed; `go vet ./...` completed and the deadcode baseline matched.
