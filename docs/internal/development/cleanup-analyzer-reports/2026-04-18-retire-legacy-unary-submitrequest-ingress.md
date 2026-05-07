# Retire Legacy Unary SubmitRequest Ingress Cleanup

Date: 2026-04-18
Scope: Agent Factory public submission cleanup for `US-008`.

## Summary

The cleanup retired public unary `SubmitRequest` ingress in favor of
canonical `FACTORY_REQUEST_BATCH` submissions through `SubmitWorkRequest`.
The final smoke coverage exercises the public API, idempotent request upsert,
`agent-factory run --work` startup files, file watcher raw JSON wrapping,
replayed due submissions, and cron internal time work through the canonical
batch path.

The architecture-sensitive data-model construction artifact for this cleanup is
`libraries/agent-factory/docs/development/retire-legacy-unary-submitrequest-ingress-data-model.md`.

## Initial Inventory

Initial counts were collected from the branch base:

```powershell
$base = git merge-base origin/main HEAD
```

| Surface | Command | Initial count |
| --- | --- | ---: |
| Public unary submit signatures | `git grep -n -E "Submit\(.*\[\]interfaces\.SubmitRequest" $base -- libraries/agent-factory/pkg libraries/agent-factory/tests` | 30 |
| Legacy hook submissions | `git grep -n -E "\bHookSubmission\b|SubmissionHookResult.*Submissions|Submissions.*\[\]HookSubmission" $base -- libraries/agent-factory/pkg libraries/agent-factory/tests` | 12 |
| Mixed generated/legacy tick suppression | `git grep -n -E "filterMixedLegacySubmissions|filterMixedTickResultRecords|mixed generated|TickResult.*WorkRequests|TickResult.*WorkInputs|WorkRequests \[\]|WorkInputs \[\]" $base -- libraries/agent-factory/pkg libraries/agent-factory/tests` | 8 |
| CLI alias | `git grep -n -e "--work-type-id" $base -- libraries/agent-factory api docs` | 10 |
| `work_type_id` text | `git grep -n "work_type_id" $base -- libraries/agent-factory api docs` | 278 |

## Removed Compatibility Surfaces

- Removed public factory and service `Submit(ctx, []interfaces.SubmitRequest)`
  ingress paths.
- Removed `HookSubmission` and
  `SubmissionHookResult.Submissions` from active hook contracts.
- Removed tick-result `WorkRequests` and `WorkInputs` compatibility records
  plus mixed generated/legacy suppression helpers.
- Removed public `work_type_id` submit normalization at API, CLI, file
  watcher, OpenAPI, README, and example boundaries.
- Removed `--work-type-id` registration from the CLI submit command.
- Removed the legacy public `DEFAULT` `WorkRequestType` enum value from the
  OpenAPI contract, generated API model, dashboard event type, and runtime
  timeline documentation.
- Converted `agent-factory run --work` startup files from the flat
  `SubmitRequest` JSON shape to canonical `FACTORY_REQUEST_BATCH` JSON.

## Final Inventory

Final commands exclude this report directory so the audit text does not count
as an active match.

| Surface | Command | Final result | Classification |
| --- | --- | ---: | --- |
| Public unary submit signatures | `rg -n "Submit\(.*\[\]interfaces\.SubmitRequest" libraries/agent-factory/pkg libraries/agent-factory/tests --glob "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**"` | 0 matches / 0 files | Removed. |
| `HookSubmission` | `rg -n "\bHookSubmission\b" libraries/agent-factory/pkg libraries/agent-factory/tests --glob "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**"` | 0 matches / 0 files | Removed. |
| Mixed generated/legacy suppression | `rg -n "filterMixedLegacySubmissions|filterMixedTickResultRecords|mixed generated|mixed generated/legacy|TickResult\..*WorkRequests|TickResult\..*WorkInputs|legacy WorkRequests|legacy WorkInputs" libraries/agent-factory/pkg libraries/agent-factory/tests --glob "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**"` | 0 matches / 0 files | Removed. |
| Legacy `DEFAULT` work request type | `rg -n "WorkRequestTypeDefault|Legacy single-work request shape|\"type\": \"DEFAULT\"|type: \"DEFAULT\"" libraries/agent-factory/api libraries/agent-factory/pkg/api/generated libraries/agent-factory/docs/run-timeline.md libraries/agent-factory/ui/src --glob "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**"` | 0 matches / 0 files | Removed from public API, generated API, docs, and dashboard event contracts. |
| CLI `--work-type-id` | `rg -n -e "--work-type-id" libraries/agent-factory api docs --glob "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**" --glob "!progress.txt"` | 5 matches / 1 file | Negative CLI tests in `pkg/cli/root_test.go` assert the retired flag fails. |
| `work_type_id` | `rg -n "work_type_id" libraries/agent-factory/pkg libraries/agent-factory/tests/functional_test libraries/agent-factory/tests/stress libraries/agent-factory/ui/src docs/processes/agent-factory-development.md libraries/agent-factory/docs/development/live-dashboard.md --glob "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**" --glob "!progress.txt"` | 191 matches / 39 files | Retained internal token/read-model fields, dashboard projections, rejection tests, and process guidance only. |

## Retained Internal Exceptions

- `interfaces.Work.WorkTypeID` remains the internal Go field consumed by token
  construction, but its direct JSON contract is `work_type_name`.
- `interfaces.Token` and dashboard/read-model structs retain
  `work_type_id` for internal runtime identity, topology edges, token history,
  and existing dashboard projections.
- API and CLI tests intentionally mention `work_type_id` and
  `--work-type-id` only to prove retired aliases are rejected.
- File watcher tests intentionally mention `work_type_id` only to prove
  `FACTORY_REQUEST_BATCH` files reject the retired alias before folder-based
  work-type defaulting can accept the payload.
- `GeneratedSubmissionBatch.Submissions` remains an internal generated-batch
  enrichment field. It is not the deleted `HookSubmission` public hook output.

## Smoke Coverage

`TestLegacyUnaryRetirementSmoke_CanonicalSubmitPathsStayBatchOnly` covers:

- direct `POST /work` with `work_type_name`;
- idempotent `PUT /work-requests/{request_id}` with a
  `FACTORY_REQUEST_BATCH` payload and one canonical `WORK_REQUEST` event;
- `agent-factory run --work` startup-file decoding through the canonical
  `FACTORY_REQUEST_BATCH` service path;
- file watcher non-batch JSON wrapping into a one-item
  `FACTORY_REQUEST_BATCH`;
- replay delivery from recorded canonical `WORK_REQUEST` events; and
- cron internal `__system_time` work retained in canonical history while
  normal public views continue to filter it.

## Validation

Commands for this closeout:

```powershell
cd libraries/agent-factory
go test ./tests/functional_test -run TestLegacyUnaryRetirementSmoke_CanonicalSubmitPathsStayBatchOnly -count=1
go test ./pkg/factory/engine ./pkg/factory/runtime ./pkg/service ./pkg/listeners ./pkg/replay ./pkg/timework ./pkg/api ./pkg/cli -count=1
make lint
cd ../..
make docs-check
```

All commands passed. The deadcode baseline was reviewed after rebasing onto
`main`; the current analyzer report has zero findings, so the accepted baseline
is empty.

Additional review follow-up validation after adding the data-model artifact:

```powershell
make docs-check
make check-go-architecture-quality
```

Additional review follow-up validation after rejecting file watcher
`work_type_id` aliases and deriving generated-batch request history from the
normalized request ID:

```powershell
cd libraries/agent-factory
go test ./pkg/listeners -run 'TestFileWatcher_JSONFactoryRequestBatch(RejectsWorkTypeIDAlias|MapsWorkTypeName)' -count=1
go test ./pkg/factory/engine -run TestSubmissionHook_GeneratedBatchRecordsCanonicalHistoryBeforeInjection -count=1
go test ./pkg/factory/engine ./pkg/factory/runtime ./pkg/service ./pkg/listeners ./pkg/replay ./pkg/timework ./pkg/api ./pkg/cli -count=1
make lint
cd ../..
make check-go-architecture-quality
```

Additional review follow-up validation after removing the legacy public
`DEFAULT` `WorkRequestType` enum and docs example:

```powershell
cd libraries/agent-factory
go generate -tags=interfaces ./pkg/api
go test ./pkg/api -run 'TestOpenAPIContract_ContainsCoveredJSONOperations|TestGeneratedOpenAPIContractsCompile' -count=1
go test ./pkg/factory/engine ./pkg/factory/runtime ./pkg/service ./pkg/listeners ./pkg/replay ./pkg/timework ./pkg/api ./pkg/cli -count=1
go test ./... -count=1
make lint
cd ui
bun install --frozen-lockfile
bun run tsc
cd ../../..
make check-go-architecture-quality
make docs-check
```
