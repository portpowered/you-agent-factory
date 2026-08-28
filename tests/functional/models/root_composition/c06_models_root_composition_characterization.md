# C06 Models root-composition characterization

Status: `CHAR-001 PASS` for characterization. This report establishes the
current behavior and topology basis for TASK-002. It does not approve a
shared-process migration, claim CI improvement, or repair the retained
environment-isolation failure.

## Scope and evidence boundary

- Story: `functional-test-optimization-c06-models-root-composition-001`.
- Baseline: `8e1c909a34901919d6700054a4d3bf46efa64ee`.
- Measurement code head: `787df4b42`.
- Owned surface changed: `tests/functional/models/root_composition/**` only.
  No production Models code, shared functional support, API, CI, or baseline
  surface was changed.
- The PRD names `docs/temp/functional-test-optimization.md` as its source
  plan, but that file is not present in this checkout. The PRD/task packet and
  repository standards are the available authority; no absent-plan behavior is
  inferred here.
- All network calls in the executed default suite were local or controlled
  fixtures. Paid or uncontrolled remote calls: `0`.

## Inventory and topology

The behavior package grew from **6 files / 16 top-level tests** at the baseline
to **24 behavior Go files / 55 top-level tests**. The characterization helper
`characterization_ledger_test.go` is one additional measurement-only Go file;
therefore the raw current `*.go` file count is 25. `TestMain` is excluded from
the functional-test count.

Commands and observed inventory:

```text
git ls-tree -r --name-only 8e1c909a34901919d6700054a4d3bf46efa64ee -- tests/functional/models/root_composition '*.go'
=> 6 paths

git grep -n -E '^func Test[A-Za-z0-9_]+\(' 8e1c909a34901919d6700054a4d3bf46efa64ee -- tests/functional/models/root_composition
=> 16 matches

rg --files tests/functional/models/root_composition -g '*.go'
=> 25 paths (24 behavior files plus characterization_ledger_test.go)

rg -n '^func Test[A-Za-z0-9_]+\(' tests/functional/models/root_composition -g '*.go' |
  Where-Object { $_ -notmatch 'func TestMain\(' }
=> 55 matches

git diff --stat 8e1c909a34901919d6700054a4d3bf46efa64ee..HEAD -- tests/functional/models/root_composition
=> 25 files changed, 6509 insertions(+), 186 deletions(-)
```

The pre-characterization source census was 38 direct `BuildProcess` call
sites, 21 server-backed root starts, 6 LocalAI fixture starts, and 23 direct
test-server sites. Source call-site counts are not execution counts: table
rows, grouped helpers, and repeated public invocations expand them.

The final run used package-local wrappers and edge counters. Direct package
root builds and API-server starts are reported separately because the API
server helper constructs its own recording-reader root in shared support.

| Operation | Actual final-run count | Measurement boundary |
| --- | ---: | --- |
| Direct package `support.BuildProcess` calls | 51 | package wrapper; construction only |
| Functional API server starts / server-owned roots | 24 | package wrapper; one root per successful server start |
| Root-backed constructions observed by these boundaries | 75 | 51 direct + 24 server-owned |
| Package factory scaffolds | 58 | wrapper around `support.ScaffoldFactory` |
| Package-owned `t.TempDir` calls | 85 | wrapper; support-internal temp dirs are intentionally not counted |
| Package-owned `httptest.NewServer` starts | 33 | wrapper; API-server internals are kept in the API count |
| LocalAI fixture starts | 8 | wrapper around the package fixture entry point |
| TCP listeners | 2 | package listener wrapper |
| Managed model-host starts | 31 | all package host-launcher `Start` implementations |
| Model-asset HTTP calls | 6 | package asset doers, including controlled cache-miss fixtures |
| Model-host HTTP calls | 0 | package host doers |
| Wrapped local HTTP requests | 30 | package-local `httptest` handlers |

The ledger is test-only and prints this line at process exit. It has no effect
on the production root, lifecycle, session, or edge graph:

```text
CHAR-001 ledger root_builds=51 api_servers=24 httptest_servers=33 localai_starts=8 tcp_listeners=2 factory_roots=58 temp_dirs=85 host_starts=31 asset_http_calls=6 host_http_calls=0 local_http_calls=30
```

Added scenario families are catalog/discovery/readiness and coded diagnostics;
pull/remove/reconstruction and asset/backend-host selection; generic
inference, output rollback, cancellation, timeout, and crash redaction;
EMBED and delivered artifacts; Omni multimodal/protocol transport; LocalAI
CLI/HTTP conformance and failure modes; ASR; TTS; and guarded real-local
OMNIVOICE API/event diagnostics.

## Reproduction and raw result

Baseline was run from a detached worktree at the baseline commit:

```text
go test -count=1 -timeout=30m ./tests/functional/models/root_composition
exit=0
go output: ok github.com/portpowered/infinite-you/tests/functional/models/root_composition 2.728s
measured shell elapsed: 00:00:16.0417591
```

The final named verification was:

```text
go test -json -count=1 -timeout=30m ./tests/functional/models/root_composition
exit=1
package elapsed: 44.688s
```

The plain-output equivalent was also run on the same final characterization
tree and returned the same retained failure (`exit=1`, package line
`56.699s`). Comparable local runs varied substantially with parallel-test and
JSON-output contention (roughly 42–63 seconds), so these timings are
directional references, not a portable threshold and not a CI-performance
claim.

Raw failure excerpt from the final `-json` run:

```text
--- FAIL: TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess (10.44s)
    catalog_cli_test.go:79: models list built-in runtime = generated.ManagedRuntime{... Identity:"llm", LifecycleState:"INSTALLED", Locality:"LOCAL", ReadinessState:"READY", ...}, want MISSING/NOT_INSTALLED baseline
FAIL
CHAR-001 ledger root_builds=51 api_servers=24 httptest_servers=33 localai_starts=8 tcp_listeners=2 factory_roots=58 temp_dirs=85 host_starts=31 asset_http_calls=6 host_http_calls=0 local_http_calls=30
FAIL github.com/portpowered/infinite-you/tests/functional/models/root_composition 44.688s
```

Cause attribution: `catalog_cli_test.go` leaves the process environment
unscoped while asserting the zero-configuration built-in `llm` cache is
`MISSING/NOT_INSTALLED`. On this host the operator model cache was discovered,
so the public result is `READY/INSTALLED`. The failure is retained as an
environment/cache-isolation observation; this story does not change the
fixture, production readiness behavior, or the operator cache. The separate
controlled readiness-failure cell now exercises both HTTP collection/detail
and CLI list/inspect and passes.

The full run otherwise completed the catalog, assets, host, pull, inference,
ASR, TTS, EMBED, LocalAI, Omni, artifact, cancellation, recovery, and
diagnostic witnesses visible in the package output. The focused check

```text
go test -count=1 -run '^TestModelsCatalogReadinessFailureKeepsPublicUnavailableTaxonomy$' ./tests/functional/models/root_composition
=> ok (0.495s)
```

passed after the CLI readiness assertions were added. No missing cleanup
witness was found: process-owning rows use `support.CleanupProcess` or the
equivalent close helper, servers and listeners use `t.Cleanup`/deferred stop,
and LocalAI/temporary resources use their test-owned cleanup. Therefore the
“fail before TASK-002 when a cleanup witness is missing” condition was not
triggered.

## Eligibility rule

`shareable` means pure or detached behavior with no process-scoped external
state. `candidate-controlled` means it may be considered by TASK-002 only if
the same immutable Factory definition, explicit Factory Session, environment,
edge routing, and cleanup proof are established. `isolated-with-reason` means
the row remains isolated because it observes a process-scoped cache/home,
host/lease, asset source, port, protocol, executable, stream, cancellation,
or local-real boundary. `deferred` belongs to TASK-002 or TASK-003 and is not
claimed by this story.

The operation column in the matrix uses `P/A/H/L/T/F/D/S/AS/HS/R` for the
ledger metrics above: direct process, API server, local test server, LocalAI,
TCP listener, factory root, package temp directory, host start, asset HTTP,
host HTTP, and wrapped local HTTP. Unless a row gives a fixture assertion or a
per-table count, its actual operation evidence is the aggregate final-run
ledger; no artificial per-row split of concurrent counters is claimed.

## CASE-001–CASE-060 reconciliation

| ID | Exact current test / subtest / table row | Public witness and actual-cost evidence | Fidelity and cleanup owner | Eligibility |
| --- | --- | --- | --- | --- |
| CASE-001 | `build_process_inert_test.go` — `TestModelsEffectsRemainInertThroughRootBuildProcessConstruction` | `P`; effect recorder observes zero filesystem, network, host, protocol, and runtime effects during construction | Controlled injected edges; process cleanup helper | isolated-with-reason: construction-effect boundary is itself the isolation witness |
| CASE-002 | `catalog_discovery_test.go` — `TestGenericModelContractsRemainDetachedAtApplicationRoot`; `assertGenericOperationCatalog`, `assertBuiltInModelCatalog`, `assertGenericInvocationContracts`, `assertGenericRuntimeFailures` | `P`; detached catalog/operation/error assertions and non-aliasing mutation checks | Pure contract assertions; process cleanup helper | shareable |
| CASE-003 | `catalog_discovery_test.go` — `TestModelsCatalogDiscoveryActivatesThroughRootBuildProcessAfterLifecycle`; `GET /status`, `GET /models` | `A1/F1`; effective catalog, identities, readiness, and status projection | Controlled API boundary; `server.Stop` cleanup | candidate-controlled: read-only catalog, but lifecycle/server state must be scoped |
| CASE-004 | `catalog_discovery_test.go` — `TestModelsCatalogDiscoveryProjectsWorkerCapabilitiesAndFactoryPrecedence`; duplicate grouped row `TestModelsRootCompositionModelScenarios/catalog discovery projects worker capabilities and Factory precedence` | `A2/F2`; list/detail/invocation checks, worker/resource order, Factory precedence | Controlled HTTP; each server stopped | candidate-controlled only for the identical rich-catalog fixture; grouped/direct duplicate is not approval |
| CASE-005 | `catalog_discovery_test.go` — `TestModelsCatalogProjectsBuiltInsThroughRootBuildProcess` | `A1/F1`; five built-in detail reads, unknown detail, and unsupported operation | Controlled API/catalog; server stopped | candidate-controlled only with identical catalog config and isolated cache |
| CASE-006 | `catalog_discovery_test.go` — `TestModelsCatalogProjectsCustomModelThroughRootBuildProcess` | `A1/F1`; custom model list/detail and EMBED operation | Controlled API and custom Factory definition; server stopped | isolated-with-reason: process-scoped authored definition differs |
| CASE-007 | `catalog_cli_test.go` — `TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess`; list, Factory inspect, built-in list/inspect | `A1/F1`; CLI ordering/readiness parity; retained `READY/INSTALLED` vs expected `MISSING/NOT_INSTALLED` failure | Controlled API but unscoped operator cache observed; server stopped | isolated-with-reason: known environment/cache leak |
| CASE-008 | `catalog_discovery_test.go` — `TestModelsCatalogDiscoveryMapsUnknownDetailThroughHTTP`; unknown and blank-name HTTP rows plus local CLI inspect | `A1/P1/F1`; typed 404/not-found and safe CLI error | Controlled API; server/process cleanup | candidate-controlled only for identical read-only rich catalog |
| CASE-009 | `catalog_discovery_test.go` — `TestModelsCatalogDiscoveryMapsUnsupportedOperationThroughHTTP`; HTTP POST plus local CLI invoke | `A1/P1/F1`; typed bad request before model host/provider effect | Controlled API and inert edges; server/process cleanup | candidate-controlled only with immutable catalog and no host state |
| CASE-010 | `catalog_discovery_test.go` — `TestModelsCatalogReadinessFailureKeepsPublicUnavailableTaxonomy`; HTTP collection/detail and CLI `models list`/`inspect` rows | `A1/F1`; all four public reads retain `NOT_FOUND/MODEL_NOT_AVAILABLE`, no success stdout; focused test `PASS` | Controlled cache home and failing inspect edge; server stopped | isolated-with-reason: readiness failure and environment isolation are the observed cause |
| CASE-011 | `catalog_discovery_test.go` — `TestModelsCatalogReadinessFailureStaysUnavailableThroughHTTP`; `GET /models`, `GET /models/embed` | `A1/F1`; HTTP unavailable response, no ready detail | Controlled failing asset metadata edge; server stopped | isolated-with-reason: failure edge must not be shared with success catalog |
| CASE-012 | `catalog_discovery_test.go` — `TestModelsCatalogReadinessCancellationReturnsPublicFailure`; JSON CLI list through API server | `A1/F1`; cancellation returns error and emits no success stdout | Controlled cancelled inspect edge; server stopped | isolated-with-reason: cancellation/lifecycle state |
| CASE-013 | `catalog_discovery_test.go` — `TestModelsInvokeReadinessDependencyFailureIsUnavailableAfterCatalogSuccess` | `P/F1`; inspection counter proves catalog plus follow-up readiness, provider not reached, private text redacted | Controlled filesystem failure; process cleanup | isolated-with-reason: mutable two-phase readiness counter |
| CASE-014 | `catalog_discovery_test.go` — `TestModelsInvokeCatalogDependencyCancellationIsSafeThroughProcess` | `P/F1`; cancelled catalog readiness, no provider and no dependency leakage | Controlled cancellation edge; process cleanup | isolated-with-reason: cancellation state |
| CASE-015 | `catalog_discovery_test.go` — `TestModelsInvokeCatalogRequestCancellationStopsReadiness` | `P/F1`; blocked inspect receives caller cancellation, command terminates, no partial result | Controlled blocking edge and command join; process cleanup | isolated-with-reason: blocked goroutine/stream lifecycle |
| CASE-016 | `catalog_discovery_test.go` — `TestModelsInvokeReadinessCancellationAfterCatalogSuccessIsSafe` | `P/F1`; inspection counter proves catalog then cancelled direct readiness, no invoke | Controlled sequential failure edge; process cleanup | isolated-with-reason: phase-sensitive cancellation |
| CASE-017 | `catalog_discovery_test.go` — `TestModelsInvokeReadinessCancellationAfterSuccessfulObservationIsSafe` | `P/F1`; post-query cancellation guard prevents false invocation/output | Controlled cancelling context and inspect edge; process cleanup | isolated-with-reason: caller context and post-query state |
| CASE-018 | `readiness_assets_host_test.go` — `TestModelsReadinessAssetsHostEffectsRemainInertThroughRootBuildProcess`; inert recorder in build test | `P`; readiness/assets/host/compatibility effect totals remain zero | Controlled effect recorder; process cleanup | isolated-with-reason: negative external-effect boundary |
| CASE-019 | `readiness_assets_host_test.go` — `TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle` | `A1/H1/F1/S`; cached asset paths, host readiness, backend selection, output, and compatibility calls | Local-real filesystem plus controlled local host HTTP; server/host cleanup | isolated-with-reason: host, asset, executable, and readiness fidelity |
| CASE-020 | `story_003_test.go` — `TestModelsPublicPullWorkflowProvesTruthfulTerminalState`; before/during/after pull rows | `A1/P2/F1/H1`; controlled two-asset source, terminal readiness and output counters | Controlled source/network, test-owned cache, API/server/process cleanup | isolated-with-reason: pull state and durable cache mutate across observations |
| CASE-021 | `story_003_test.go` — same test, `controlled source failure` subtest | `A1/P1/F1/H1`; source returns 503, `SOURCE_FETCH_FAILED/FAILED`, no false ready or partial asset | Controlled failing source and owned cache; cleanup | isolated-with-reason: failure source and partial-download state |
| CASE-022 | `pull_to_ready_test.go` — `TestModelsPullToReadySurvivesProcessReconstruction` | `P2/F1`; first process pulls, reconstructed process reads readiness, exact asset client asserts no refetch | Controlled asset source and two root lifecycles; both processes cleanup | isolated-with-reason: reconstruction/persistence is the behavior |
| CASE-023 | `story_003_test.go` — `TestModelsPublicRemoveWorkflowProvesReclamationAndInUseRefusal`; successful remove portion | `A1/P1/F1`; cache bytes/path reclaimed and terminal response retained | Owned cache and API; server/process cleanup | isolated-with-reason: destructive asset reclamation |
| CASE-024 | `story_003_test.go` — same test, `in-use response` subtest | `P1/H1`; in-use removal refused until host/lease release | Controlled remove handler and lease; process/server cleanup | isolated-with-reason: live lease/host ownership |
| CASE-025 | `coded_diagnostics_test.go` — `TestModelsLocalRemoveMissingCacheRendersCodedDiagnostic`; table rows `normal`, `json`, `debug` | `P3/F3/D`; each mode asserts `MODEL_CACHE_NOT_FOUND`, stream placement, typed causes, and debug cause | Local CLI, unique cache/home per row; process cleanup | candidate-controlled only after env/stream isolation proof |
| CASE-026 | `coded_diagnostics_test.go` — `TestModelsLocalRemoveMissingCacheMatchesHTTPDiagnostic`; local/remote parity | `P2/F1/H1`; local and HTTP code/family/message parity | Controlled HTTP plus local cache; process/server cleanup | isolated-with-reason: parity spans distinct local and remote boundaries |
| CASE-027 | `coded_diagnostics_test.go` — `TestModelsLocalInspectUnknownRendersCodedDiagnostic`; table rows `normal`, `json`, `debug` | `P3/F3/D`; each mode asserts `NOT_FOUND`, safe message, typed cause, and debug cause | Local CLI with unique env per row; process cleanup | candidate-controlled only after env/stream isolation proof |
| CASE-028 | `coded_diagnostics_test.go` — `TestModelsLocalInspectUnknownMatchesHTTPDiagnostic`; local/remote parity | `P1/F1/H1`; local and HTTP code/family parity and model name | Controlled HTTP and local CLI; process/server cleanup | isolated-with-reason: parity spans distinct boundaries |
| CASE-029 | `inference_invoke_test.go` — `TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration` | `P1/F1/H1/S1`; pinned host receives built-in invocation and exact output | Controlled local host/protocol and cached assets; process/server/host cleanup | isolated-with-reason: host and executable selection |
| CASE-030 | `inference_invoke_test.go` — `TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess` | `A1/H1/F1`; generic HTTP response/output through live joined root | Controlled local host and API; server/host cleanup | isolated-with-reason: API session and host state |
| CASE-031 | `inference_invoke_test.go` — `TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker`; success, readiness failure, unknown-reference rows | `A1/F1`; effective built-in route, typed readiness and unknown errors, no worker fallback | Controlled API and provider command recorder; server cleanup | isolated-with-reason: named route/readiness state |
| CASE-032 | `inference_failure_test.go` — `TestModelsGenericCLIProcessPublishesSingleOutputToStdoutOnly` | `P1/F1`; one stdout output and empty stderr | Controlled generic fixture; process cleanup | isolated-with-reason: output stream ownership |
| CASE-033 | `inference_failure_test.go` — `TestModelsGenericCLIProcessRollsBackMappedOutputsThroughEdges` | `P1/F1/D1`; injected second rename failure restores both old files and removes temp artifacts | Controlled filesystem edge; process cleanup | isolated-with-reason: partial publication/rollback state |
| CASE-034 | `inference_failure_test.go` — `TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing` | `P1/F1/S1`; blocked readiness observes cancellation, no stdout, host stop count positive | Controlled blocking protocol/host; process cleanup | isolated-with-reason: cancellation and host lifecycle |
| CASE-035 | `inference_failure_test.go` — `TestModelsGenericCLIProcessTimeoutStopsReadinessAndPublishesNothing` | `P1/F1/S1`; deadline returns, no output, host stop count positive | Controlled deadline and host; process cleanup | isolated-with-reason: timeout/host lifecycle |
| CASE-036 | `inference_failure_test.go` — `TestModelsGenericCLIProcessRedactsCrashedHostDetails` | `P1/F1/S1`; crash taxonomy is safe and secret/endpoint details are absent | Controlled crashing host; process cleanup | isolated-with-reason: crash and redaction boundary |
| CASE-037 | `story_004_embed_test.go` — grouped `TestModelsEmbedRootCompositionBehavior/zero-configuration` | `P1/F1/H1/S`; exact vector, JSON/CLI outputs, cache-hit asset network `0`, failure/recovery | Controlled cached EMBED backend and host; process/server cleanup | isolated-with-reason: host lease and EMBED output state |
| CASE-038 | `story_004_embed_test.go` — grouped `/oversized-file` | `P1/F1/H1/S`; input limit is positive and backend/host/assets/artifact effects remain `0` | Controlled local input and host; process/server cleanup | isolated-with-reason: preflight file boundary |
| CASE-039 | `story_004_embed_test.go` — grouped `/invalid-vector` | `P1/F1/H1/S`; typed malformed response followed by successful invocation proves lease release | Controlled malformed protocol response and host; process/server cleanup | isolated-with-reason: malformed response/lease recovery |
| CASE-040 | `story_004_embed_test.go` — `TestModelsEmbedCacheMissThenHitAvoidsNetworkThroughRootBuildProcess` | `P1/F1/H1/S/AS>=3`; manifest/model/backend exchanges on miss, unchanged count on hit, exact paths | Controlled asset HTTP and local host; process/server cleanup | isolated-with-reason: cache mutation and asset network are the witness |
| CASE-041 | `story_004_embed_test.go` — `TestModelsEmbedHTTPParityUsesTheSameFixtureThroughRootBuildProcess` plus grouped `/named-generic-http-parity` | `A/H` per HTTP cell; same vector/slot/envelope and cache-hit asset `0` | Controlled API/local host; server cleanup | isolated-with-reason: named/generic HTTP session parity |
| CASE-042 | `delivered_embed_story_test.go` — `TestDeliveredEmbedCLIArtifactReachesProtocolFixture` and `TestDeliveredEmbedHTTPArtifactReachesProtocolFixture` | Delivered executable; cache miss `>=3` asset exchanges, cache hit unchanged, vector/artifact/protocol assertions | Shipped binary, controlled fixture, real local TCP for HTTP; deferred process/listener cleanup | isolated-with-reason: executable and process-level boundary |
| CASE-043 | `story_002_omni_test.go` — `TestModelsOmniTextInputReachesPinnedCodecThroughRootBuildProcess` | `P1/F1/H1/S1/AS0`; normalized prompt, one protocol call, exact response | Controlled pinned codec/local host; process/server/host cleanup | isolated-with-reason: pinned protocol and host |
| CASE-044 | `story_003_omni_test.go` — `TestModelsOmniFileInputsPreserveDetectedTypesAndImageOrderThroughRootBuildProcess` | `P1/F1/H1/S1/AS0`; prompt/repeated image/audio/video order, detected media types, exact response | Controlled local files and codec/host; process/server/host cleanup | isolated-with-reason: file/media ordering and protocol boundary |
| CASE-045 | `story_004_omni_test.go` — `TestModelsOmniVideoCapabilityAndCancellationThroughRootBuildProcess`, video success portion | `P1/F1/H1/S1`; video slot/modality/media reaches protocol and exact response | Controlled local host/protocol; process/server/host cleanup | isolated-with-reason: multimodal host capability |
| CASE-046 | same test, unsupported audio/video portion | Same fixture `P1/F1/H1/S1`; typed capability failure occurs before a second generation and output files remain absent | Controlled capability rejection; process/server/host cleanup | isolated-with-reason: preflight capability state |
| CASE-047 | same test, cancellation and follow-up portions | Same process/fixture; cancellation observed, no files/stdout, follow-up succeeds; top-level fixture assertion is 3 calls total | Controlled blocking protocol and recovery; process/server/host cleanup | isolated-with-reason: cancellation/recovery state |
| CASE-048 | `story_004_omni_test.go` — `TestModelsOmniCancellationReleasesHostAcrossRootProcesses` | `P2/F1/H1/S`; first root closes after cancellation, second root follow-up succeeds | Controlled exclusive host launcher; both processes/server/host cleanup | isolated-with-reason: cross-root host lease release |
| CASE-049 | `omni_protocol_transport_test.go` — `TestOmniProtocolTransportRoundTripsThroughNetworkDialer` | `T1`; local socket negotiation/envelope/content round-trip and listener closes | Local-real TCP transport; listener cleanup | isolated-with-reason: port/listener fidelity |
| CASE-050 | `localai_cli_conformance_test.go` — `TestLocalAICLIConformanceMatrixRunsThroughRootBuildProcess`; 7 rows × 2 surfaces | `L1/P1/A1/F1`; all 14 strict CLI rows, exact outputs/order, compatibility/host calls, asset network `0` | LocalAI fixture, local gRPC/protocol, managed host; fixture/server/process cleanup | isolated-with-reason: LocalAI/host/protocol fidelity |
| CASE-051 | `localai_http_conformance_test.go` — `TestLocalAIHTTPConformanceMatrixRunsThroughRootBuildProcess`; 7 HTTP rows plus no-vertical probe | `L1/A1/F1`; 7 implemented rows, expected-unimplemented probe, call counts and asset network `0` | LocalAI fixture and generic HTTP; server/fixture cleanup | isolated-with-reason: HTTP/LocalAI process boundary |
| CASE-052 | `localai_failure_conformance_test.go` — `TestLocalAIFailureDiagnosticsReachHTTPAndCLI`; table rows `backend unavailable`, `protocol mismatch`, `malformed response` | `L3/A3/F3`; each row crosses HTTP and CLI, safe typed failure, no success, cleanup | LocalAI failure fixtures and API servers; per-row cleanup | isolated-with-reason: failure mode and fixture state |
| CASE-053 | `asr_story_test.go` — `TestModelsASRDirectCLIEndToEndThroughRootBuildProcess` | `L1/P1/A1/F1/H1/S/AS0`; transcript, segments, audio artifact metadata, request, and zero asset network | Cached local ASR assets, LocalAI/host/protocol; all fixture/process/server cleanup | isolated-with-reason: local-real ASR boundary |
| CASE-054 | `tts_story_test.go` — `TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess`; generic, alias, failure, recovery rows | `L1/P1/F1/H1/S`; four canonical requests, exact audio, no partial failure file, recovery | Cached local TTS assets and LocalAI/host; fixture/process/server cleanup | isolated-with-reason: audio output/host recovery |
| CASE-055 | `delivered_cli_story_test.go` — `TestDeliveredCLIArtifactReachesProtocolFixture`; plus delivered EMBED CLI/HTTP artifact tests | Delivered executable subprocesses; exact protocol request, transcript/segments/audio and EMBED outputs | Shipped binaries, controlled fixtures, local process boundaries; deferred stops and temp cleanup | isolated-with-reason: artifact/executable boundary |
| CASE-056 | `api_model_local_inference_long_test.go` — `TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio`; `functionallong` guard | Not run in default suite; opt-in local-real CLI/API audio proof remains guarded, so default ledger contribution is `0` | Real local artifact/runtime only when explicitly enabled; test-owned process/cache cleanup | isolated-with-reason: guarded real-local dependency |
| CASE-057 | `api_model_local_inference_long_test.go` — `TestRecordedRealLocalModelEventDiagnostics`; 13 `realLocalModelDiagnosticCases` table rows | Not run in default suite; pure recorded-event reducer covers no response, decode/wrong outcome/operation, output-part/media errors, valid file/URL, malformed and duplicate responses | Pure recorded data; no root/network; table test cleanup | shareable (pure reducer), but opt-in guard remains |
| CASE-058 | No current row; PRD matrix gate for TASK-002 `TestModelsSharedProcessEligibleScenarios` | Deferred: requires two explicit Factory Sessions, unique routed edges, concurrency/race proof and cross-session witness | Not current-story evidence | deferred to TASK-002 |
| CASE-059 | No current row; PRD matrix gate for TASK-002 cleanup proof | Deferred: requires failure/timeout/cancellation/partial teardown after sharing and later-case usability | Not current-story evidence | deferred to TASK-002 |
| CASE-060 | No current row; PRD matrix gate for TASK-003 repeat proof | Deferred: requires `go test -count=3 -timeout=90m` and stable inventory/cleanup across runs | Not current-story evidence | deferred to TASK-003 |

This gives every matrix ID exactly one current row or an explicit owning-story
deferred gate; there are no unclassified cases. The candidate labels are
discovery inputs for TASK-002, not migration approval. In particular, the
known CASE-007 cache leak and every host, asset, executable, pull, local-real
protocol, listener, artifact, cancellation, and recovery row remain isolated
until a later story proves otherwise.

## Remaining edges

This story proves current behavior, growth, and measured cause attribution. It
does not prove:

- shared-process Factory Session isolation or migration cleanup;
- race safety for a shared router/ledger;
- three-run repeatability (`count=3`);
- exact-head PR contention improvement;
- clean-room delivered integration; or
- parent project CI median/pass-count gates.

Those are explicitly left for TASK-002, TASK-003, review CI, or the parent
project gates named by the PRD.
