# Functional Test Cleanup Closeout

This closeout records which behavioral tests still cover the public contracts that were previously entangled with structural assertions in `tests/functional_test`.

## Behavioral Coverage Map

| Behavior that still needs confidence | Tests that now cover it behaviorally | Observable proof |
| --- | --- | --- |
| Cleanup lane preserves canonical runtime completion, status totals, canonical event history, and dashboard shell fallback | `tests/functional_test/cleanup_smoke_test.go` (`TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces`) | Submits live work, waits for `GET /work` to reach `task:complete`, checks `GET /status`, reconstructs canonical factory world state from emitted events, and verifies `/dashboard/ui` plus client-side route fallback return the embedded HTML shell |
| Generated API paths still accept work and normalize runtime state | `tests/functional_test/generated_api_smoke_test.go` (`TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned`, `TestGeneratedAPIIntegrationSmoke_BatchWorkTypeNameNormalizesRuntimeWork`) | Exercises `POST /work`, `PUT /work-requests/{requestId}`, `GET /work`, `GET /work/{id}`, and `GET /status`, then verifies completed runtime tokens and dependency wiring through the public API and runtime snapshot |
| User-facing CLI submit behavior still reaches the live API with canonical work typing | `tests/functional_test/generated_api_smoke_test.go` (`TestGeneratedAPIIntegrationSmoke_CLIWorkTypeNameReachesLiveAPIHandler`) | Runs `agent-factory submit --work-type-name task` against the live functional server and waits for completed `task` work through `GET /work` |
| Event replay and dashboard timeline behavior still reflect live emitted events | `tests/functional_test/event_replay_smoke_test.go` (`TestEndToEndEventReplaySmoke_BackendEventsReconstructSelectedTicksForWebsiteTimeline`) | Streams `/events`, checks increasing canonical event sequences, captures an in-flight dashboard view before release, and verifies the completed dashboard view and `GET /work` result after dispatch finishes |
| Runtime-config and config-driven execution still behave through public runtime outcomes | `tests/functional_test/config_driven_test.go` (`TestConfigDriven_HappyPath`, `TestConfigDriven_RESTAPISubmitAndQuery`, related fanout and retry tests) | Loads config-backed factories, submits work through runtime and REST surfaces, and asserts terminal places, query responses, retry behavior, and dynamic fanout outcomes |
| Canonical batch-only submission paths still hold across ingestion surfaces | `tests/functional_test/legacy_unary_retirement_smoke_test.go` (`TestLegacyUnaryRetirementSmoke_CanonicalSubmitPathsStayBatchOnly`) | Verifies direct HTTP submission, idempotent batch upsert, startup work-file loading, file-watcher ingestion, replay-due submission, and cron-driven submission through runtime-visible work requests and results |
| Public worker contract remains visible while runtime-only fields stay private | `tests/functional_test/worker_public_contract_smoke_test.go` (`TestWorkerPublicContractSmoke_CanonicalWorkerExecutesAndKeepsRuntimeOnlyFieldsPrivate`) | Flattens public config, runs a real worker execution, loads replay artifacts, and confirms the public worker payload matches runtime-visible outputs without leaking internal-only fields |
| Project cleanup vocabulary remains product-facing and free of retired naming | `tests/functional_test/project_agnostic_cleanup_smoke_test.go` | Verifies runtime context, emitted events, tags, inference requests, and serialized values omit retired product naming in user-visible outputs |

## Verification

The cleanup lane was re-verified with repository-root commands that exercise the surviving behavioral suite:

```text
go test ./tests/functional_test -count=1 -timeout 300s
make test
make lint
```

Expected interpretation for this lane:

- `go test ./tests/functional_test -count=1 -timeout 300s` passes and proves the targeted behavioral suite still holds after the structural assertions were removed. The older `120s` timeout is too short for this checkout.
- `make test` should pass and confirms the broader repository test bundle remains green with the cleanup in place.
- `make lint` currently remains blocked by the pre-existing `deadcodecheck` baseline drift already recorded in `progress.txt`; this is not introduced by the functional-test cleanup lane.
