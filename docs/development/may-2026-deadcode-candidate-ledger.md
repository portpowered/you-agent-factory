# May 2026 Dead-Code Candidate Ledger

Date: 2026-05-05

## Scope

This ledger bounds the May 2026 dead-code cleanup batch requested in
`prd.json`. It records the candidate seams that should drive the follow-on
cleanup stories across backend, CLI, contracts, dashboard, tests, and tooling.

The intent is to remove only code that is demonstrably inactive or to collapse
duplicate ownership down to one explicit live owner without changing supported
API, CLI, runtime, replay, or dashboard behavior.

## Evidence Sources

- `docs/development/deadcode-baseline.txt`
- `docs/development/cleanup-analyzer-reports/2026-04-17-deadcode-lint-sweep.md`
- `docs/development/retire-scriptwrap-build-args-shim-closeout.md`
- `docs/development/retire-duplicate-ui-script-copies-closeout.md`
- `docs/processes/development-guide-relevant-files.md`
- Supported-behavior tests in `pkg/` and `tests/functional/`

## Candidate Ledger

| Lane | Candidate | Classification | Canonical surviving owner | Behavior-based reason | Evidence |
| --- | --- | --- | --- | --- | --- |
| Backend replay | `pkg/replay/event_stream_artifact.go`: `ArtifactFromEventStreamFile`, `SaveArtifactFromEventStreamFile`, adjacent-factory hydration helpers | `retain` | `pkg/replay/event_stream_artifact.go` | These wrappers still back the supported replay-event-stream import flow, including adjacent authored-factory hydration and embedded run-started payload rewriting. Removing them would break the file-based replay conversion surface instead of deleting dead code. | `tests/functional/replay_contracts/replay_event_stream_artifact_smoke_long_test.go`, `pkg/replay/event_stream_artifact_test.go`, `docs/processes/development-guide-relevant-files.md` |
| Tests and tooling | Stale accepted deadcode findings for `pkg/testutil.WithExecutionBaseDir`, `pkg/testutil.WriteSeedMarkdownFile`, `pkg/testutil.WriteSeedBatchFile`, `tests/functional/internal/support.CountFactoryEvents`, `AcceptedCommandResults`, and `ProviderCommandRequestsForWorker` | `retain` | The listed helper packages themselves | The checked-in deadcode baseline is stale for these helpers. Current functional replay, workflow, and guards suites still call them directly, so the correct action is to remove the stale baseline entries later, not remove the helpers. | `docs/development/deadcode-baseline.txt`, `tests/functional/workflow/*.go`, `tests/functional/guards_batch/*.go`, `tests/functional/replay_contracts/*.go` |
| Backend worker CLI | Dead `ScriptWrapProvider.buildArgs(...)` forwarding shim | `remove` | `pkg/workers/provider_behavior.go` | Provider-specific CLI argument ownership already lives in `provider_behavior.go`; the extra forwarding shim was only shadow ownership and should not survive once command assembly stays observable at `Infer(...)`. | `docs/development/retire-scriptwrap-build-args-shim-closeout.md`, `pkg/workers/inference_provider.go`, `pkg/workers/provider_behavior.go` |
| API contract and handler | Legacy list-work pagination shim or duplicate fallback logic | `collapse to canonical owner` | `pkg/api/handlers.go:ListWork` plus generated `ListWorkResponse` and `PaginationContext` | Supported pagination behavior is already observable at `GET /work` through `maxResults` and `nextToken`. Any alternate pagination shim should collapse into this one handler and generated contract path so the route has one owner. | `pkg/api/handlers.go`, `pkg/api/server_test.go:TestListWork_NextTokenContinuesPublicRoutePagination`, `api/openapi.yaml` |
| Config and contract compatibility | Workstation migration aliases such as top-level workstation `timeout`, singular stop-word aliases, and `runtimeStopWords` | `retain` | `pkg/config/factory_config_mapping.go`, `pkg/config/agents_config.go`, and `pkg/config/workstation_execution_limits.go` | These aliases still serve supported migration behavior at the config-load boundary. They normalize or reject legacy inputs with explicit guidance and are covered by current docs and tests, so they are not dead for this batch. | `docs/work.md`, `docs/authoring-agents-md.md`, `pkg/config/factory_config_mapping.go`, `pkg/config/agents_config_test.go`, `pkg/config/runtime_config_test.go` |
| Replay contract compatibility | Event-stream cron compatibility normalization for run-started payloads with missing cron schedule | `retain` | `pkg/replay/event_stream_artifact.go:normalizeEventStreamRunRequestFactories` | Recorded event streams still need this compatibility normalization to replay through the supported artifact-import path. Deleting it now would change supported replay behavior rather than removing dead code. | `pkg/replay/event_stream_artifact.go`, `pkg/replay/event_stream_artifact_test.go`, `docs/processes/development-guide-relevant-files.md` |
| Dashboard tooling | Duplicate tracked UI workflow scripts such as `normalize-dist-output copy.mjs` and `write-replay-coverage-report copy.ts` | `remove` | `ui/scripts/normalize-dist-output.mjs` and `ui/scripts/write-replay-coverage-report.ts` | The supported UI build and replay-coverage workflows already run through the canonical script paths. Duplicate tracked copies would be dead shadow owners and should be removed rather than kept in package scripts or docs. | `docs/development/retire-duplicate-ui-script-copies-closeout.md`, `ui/scripts/`, `ui/package.json` |
| Logging surface | Extra verbose-logging export wrappers beyond the `logging.Logger` interface and `logging.Verbose(...)` helper | `collapse to canonical owner` | `pkg/logging/logger.go` and `pkg/logging/runtime_logger.go` | The supported behavior is "service and CLI `--verbose` routes detail records through the logger interface." Any duplicate exported wrapper should collapse into the existing logger seam so verbose behavior has one owner. | `pkg/logging/logger.go`, `pkg/logging/runtime_logger.go`, `pkg/logging/logger_test.go`, `pkg/cli/root_test.go`, `pkg/service/factory_test.go` |

## Story Mapping

| Story | Ledger candidates that bound it |
| --- | --- |
| `US-002` | worker CLI shim removal, list-work pagination ownership collapse, logging-surface collapse |
| `US-003` | config and replay compatibility retain/remove decisions that affect public contract and generated-surface ownership |
| `US-004` | dashboard tooling/script-ownership cleanup and any later UI ownership collapses |
| `US-005` | stale deadcode baseline cleanup plus retained helper verification at the correct observable test layers |

## Immediate Conclusions

- The existing deadcode baseline cannot be treated as self-proving evidence. At
  least two groups of accepted findings are still live.
- Later cleanup stories should target one lane at a time and prove the
  surviving owner with observable tests, not source-topology assertions.
- Migration-only config aliases and replay compatibility shims remain in scope
  for the batch, but the current evidence says they are `retain`, not `remove`.
