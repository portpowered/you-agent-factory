# C14 Models root-composition characterization

Status: `CHAR-C14-001 PASS` for characterization. This report freezes the
pre-change witness inventory and records the measured setup profile for
`fto-c14-pkg-models-root-composition-001`. It does not claim that the default
package is green on this host, approve a shared-process migration, claim a
post-change performance result, or satisfy the later clean-room and delivery
gates.

## Scope and evidence boundary

- Parent behavior: `BEH-C14-INTRINSIC-SPEED`.
- Story: `fto-c14-pkg-models-root-composition-001`.
- Frozen head: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`.
- Branch state before measurement: clean, at `origin/main`; no operator
  amendment was present in `prd.json`.
- Owned evidence surface: this ledger only. The existing test-only counter
  helper under `tests/functional/models/root_composition` was unchanged in
  this story; no production, shared-support, contract, generated, workflow,
  or sibling real-inference surface was edited.
- The exact default command was retained as requested:
  `go test ./tests/functional/models/root_composition/... -count=1`.
- All observed calls were local or controlled fixtures. Paid or uncontrolled
  remote calls: `0`.

The baseline commands failed in the existing catalog CLI scenario on this
machine. That is retained evidence, not a weakened assertion or a fix hidden
in the characterization story. The later optimization story must either make
the named suite pass without changing its witness or return a structured
blocker with the smallest required delta.

## Environment and executable inventory

The measurements ran on 2026-08-29 from the worktree above with:

```text
Go: go1.25.0 windows/amd64
OS: Microsoft Windows NT 10.0.26200.0
CPU workers reported by environment: 24
GOMAXPROCS: unset
CI: unset
Go module: C:\Users\andre\work\portos\infinite-you\.claude\worktrees\fto-c14-pkg-models-root-composition\go.mod
Go build cache: C:\Users\andre\AppData\Local\go-build
```

The source census at the frozen head found 27 Go files, 59 `Test*`
declarations including `TestMain`, and 58 functional top-level declarations.
The default list command selected 56 top-level tests. The two additional
declarations are in `api_model_local_inference_long_test.go`, which starts
with `//go:build functionallong`; its real-local test also requires explicit
`INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1` enablement. The tagged inventory
command selected all 58 declarations and exited successfully.

Source-only topology counts were:

```text
t.Run( calls: 15
t.Parallel() calls: 38
time.Sleep calls: 1
time.After calls: 7
support/api WaitFor* calls: 2
```

The one sleep is the pre-existing 100 ms process-level retry in
`delivered_embed_story_test.go`; it is not part of this story's changes. The
bounded `time.After` calls are join/deadline guards for cancellation and
forced-cleanup witnesses. The tagged real-local observation uses
`support.WaitForObservation` and is excluded from the default suite.

Commands and results:

```text
go test ./tests/functional/models/root_composition -list '^Test'
=> exit 0; 56 default top-level tests listed

go test -tags functionallong ./tests/functional/models/root_composition -list '^Test'
=> exit 0; 58 top-level tests listed
```

The default list also emitted the test-only zero-effect counter lines from
`TestMain`; listing performs no root, server, host, factory, or temporary
directory construction.

## Baseline samples

Each sample was a separate process invocation of the exact default command,
run sequentially with `-count=1`. The wrapper measured monotonic wall time and
captured the complete output outside the repository. The package elapsed value
is the `go test` package line; wall time includes the command wrapper.

| Sample | Exit | Wall elapsed | Package elapsed | Result |
| --- | ---: | ---: | ---: | --- |
| 1 | 1 | 78.443s | 73.773s | `TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess` timed out waiting for its local `/models` endpoint |
| 2 | 1 | 59.523s | 55.527s | Same catalog CLI local API readiness timeout |
| 3 | 1 | 48.386s | 46.244s | Same catalog CLI local API readiness timeout |

Sorted samples and medians:

```text
wall:    48.386s, 59.523s, 78.443s  => median 59.523s
package: 46.244s, 55.527s, 73.773s  => median 55.527s
```

Sanitized repeated failure excerpt from all three raw logs:

```text
--- FAIL: TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess
    catalog_cli_test.go:33: Process.Execute(remote models list) error =
    CLI_COMMAND_FAILED: command failed: models endpoint not reachable at
    http://127.0.0.1:<ephemeral-port>/models: Get "http://127.0.0.1:<ephemeral-port>/models":
    context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    stderr={"code":"CLI_COMMAND_FAILED","family":"INTERNAL_SERVER_ERROR","message":"command failed"}
```

Focused reproduction on the same untouched head also failed:

```text
go test ./tests/functional/models/root_composition -run '^TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess$' -count=1
=> exit 1; wall 17.303s; package 13.539s
=> same context deadline exceeded while awaiting headers
=> counter line: root_builds=0 api_servers=1 factory_roots=1
```

The focused failure means that parallel contention is a plausible cost
contributor but is not a proven sole cause. No test or production repair was
made here. The raw files retained during this run were:

```text
C:\Users\andre\AppData\Local\Temp\c14-baseline-20260829T184010Z\run-1.log
C:\Users\andre\AppData\Local\Temp\c14-baseline-20260829T184010Z\run-2.log
C:\Users\andre\AppData\Local\Temp\c14-baseline-20260829T184010Z\run-3.log
C:\Users\andre\AppData\Local\Temp\c14-focused-catalog-cli-20260829.log
```

These temporary files are diagnostic inputs, not committed artifacts.

## Counter/profile pass

The required test-only profile pass used JSON event timing plus the existing
package-local `TestMain` counters:

```text
go test ./tests/functional/models/root_composition/... -count=1 -json
=> exit 1; wrapper wall 85.269s; package elapsed 74.254s
=> profile log: C:\Users\andre\AppData\Local\Temp\c14-profile-20260829T184406Z.jsonl
```

The package-local counter line was stable across the three baseline samples
and the profile pass:

```text
CHAR-001 ledger root_builds=51 api_servers=22 httptest_servers=33 localai_starts=8 tcp_listeners=2 factory_roots=58 temp_dirs=85 host_starts=31 asset_http_calls=14 host_http_calls=0 local_http_calls=36
C06-002 shared_models_root_builds=1 shared_models_api_starts=1 shared_models_session_opens=5 shared_models_session_closes=5
```

The counters mean:

| Operation | Observed count | Measurement boundary and interpretation |
| --- | ---: | --- |
| Direct package root builds | 51 | `characterizationBuildProcess`; construction only |
| Functional API server starts | 22 | `characterizationStartFunctionalAPIServer`; each server owns another root in shared support |
| Root-backed constructions seen by these boundaries | 73 | 51 direct plus 22 server-owned constructions |
| Package factory scaffolds | 58 | `characterizationScaffoldFactory` |
| Package-owned temporary directories | 85 | `characterizationTempDir`; support-internal directories are not counted |
| Package-owned `httptest` servers | 33 | `characterizationNewHTTPServer` |
| LocalAI fixture starts | 8 | `characterizationStartLocalAI` |
| TCP listeners | 2 | `characterizationListen` |
| Managed model-host starts | 31 | package host-launcher `Start` implementations |
| Asset HTTP calls | 14 | package asset edge calls, including controlled cache-miss fixtures |
| Host HTTP calls | 0 | no host HTTP edge was used by this run |
| Wrapped local HTTP calls | 36 | package-local `httptest` handler requests |

The shared rich-catalog candidate already has an explicit stronger topology
witness: one package root and one API server are used by two overlapping
scenario subtests, five Factory Sessions are opened, and all five close. Its
cleanup assertions also prove the fixture-owned cache selector, cache
availability, unique session IDs, session deletion, and Factory-directory
removal. This is a candidate-controlled cluster, not approval to share any
other row.

The JSON event profile's slowest completed top-level tests were:

| Test | Result | Elapsed |
| --- | --- | ---: |
| `TestModelsCatalogProjectsBuiltInsThroughRootBuildProcess` | pass | 31.06s |
| `TestModelsCatalogDiscoveryActivatesThroughRootBuildProcessAfterLifecycle` | pass | 24.70s |
| `TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker` | pass | 24.64s |
| `TestModelsCatalogProjectsCustomModelThroughRootBuildProcess` | pass | 22.94s |
| `TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess` | fail | 17.74s |
| `TestLocalAIFailureDiagnosticsReachHTTPAndCLI/malformed_response` | pass | 10.31s |
| `TestLocalAIFailureDiagnosticsReachHTTPAndCLI/backend_unavailable` | pass | 10.28s |
| `TestModelsEmbedHTTPParityUsesTheSameFixtureThroughRootBuildProcess` | pass | 9.98s |
| `TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess` | pass | 9.98s |
| `TestModelsCatalogReadinessFailureStaysUnavailableThroughHTTP` | pass | 9.82s |

The timing profile attributes runtime at test boundaries, while the counter
line attributes construction and edge operations. It does not claim an
individual millisecond total for each wait helper. The source wait census
found one shared `WaitForBaseURL`, one shared `WaitForStatus`, two explicit
catalog barriers, cancellation/result channel joins in the generic failure
tests, forced-cleanup protocol joining, and the tagged observation loop. The
server-backed tests also request `WaitForServiceModeRuntime` through their
existing support configuration. These are synchronization boundaries to
preserve, not blind waits authorized for removal.

## Candidate eligibility and isolation rule

`shareable` means detached or pure behavior with no process-scoped external
state. `candidate-controlled` means the same immutable Factory definition,
explicit Factory Session, environment, edge routing, and cleanup ownership
must be established before a later change shares it. `isolated-with-reason`
means the row observes mutable cache/home/environment, Factory definitions,
host or lease state, asset source, port, protocol, executable, stream,
cancellation, artifact, or local-real behavior. `deferred` is owned by a later
story and is not claimed by this story.

The counter operation abbreviations used below are `P` direct process,
`A` API server, `H` local test server, `L` LocalAI, `T` TCP listener, `F`
Factory scaffold, `D` package temp directory, `S` host start, and `AS` asset
HTTP. Aggregate counters are not artificially divided among parallel rows.

## Assertion and witness inventory

The inventory below maps every C14 matrix row to the exact current test,
subtest, table row, build boundary, or later evidence gate. The assertion
column is the semantic property that must remain executable. Since this story
changed no test source, the frozen assertion inventory is equal to the source
at the baseline head; the later optimization story must prove equal-or-
stronger mapping after its change.

| ID | Exact current source / row | Semantic assertion or witness | Fidelity, cleanup, and eligibility |
| --- | --- | --- | --- |
| C14-001 | `build_process_inert_test.go:25` `TestModelsEffectsRemainInertThroughRootBuildProcessConstruction` | Root construction causes no filesystem, network, host, protocol, or inference effect. | `P`, injected controlled edges, process cleanup; isolated boundary |
| C14-002 | `build_process_inert_test.go:51` `TestModelsBuildProcessRejectsMissingAssetStagingCoordination` | Nil asset-staging coordination fails construction with the required Models diagnostic before runtime. | `P`, failure is before activation; isolated failure boundary |
| C14-003 | `catalog_discovery_test.go:28` `TestGenericModelContractsRemainDetachedAtApplicationRoot` and its four assertion helpers | Returned catalog, operation, and failure values do not alias; typed detached contracts remain stable. | `P`, inert edges, process cleanup; shareable contract behavior |
| C14-004 | `catalog_discovery_test.go:170` `TestModelsCatalogDiscoveryActivatesThroughRootBuildProcessAfterLifecycle` | Status and model reads project effective identities, readiness, operations, and ordering. | `A/F`, controlled API and stopped server; candidate-controlled only with immutable setup |
| C14-005 | `catalog_discovery_test.go:324` plus grouped `TestModelsRootCompositionModelScenarios/catalog discovery projects worker capabilities and Factory precedence`; shared scenario is also exercised by `shared_process_test.go:141` | Overlapping catalog scenarios retain list, detail, invocation, and Factory/session-route isolation. | `A/F/P`, controlled rich catalog and explicit sessions; candidate-controlled |
| C14-006 | `catalog_discovery_test.go:759` `TestModelsCatalogProjectsBuiltInsThroughRootBuildProcess` | Five built-ins resolve; unknown detail and unsupported operation retain typed failures. | `A/F`, controlled catalog and stopped server; candidate-controlled only with isolated cache |
| C14-007 | `catalog_discovery_test.go:853` `TestModelsCatalogProjectsCustomModelThroughRootBuildProcess` | Custom EMBED identity and operation appear without changing built-ins. | `A/F`, distinct authored Factory; isolated-with-reason |
| C14-008 | `catalog_cli_test.go:18` `TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess` | CLI list/inspect ordering and readiness use the fixture-owned cache/home and do not leak operator cache. | `A/F`, server/process cleanup; isolated environment boundary; currently failing baseline diagnostic |
| C14-009 | `catalog_discovery_test.go:331` `TestModelsCatalogDiscoveryMapsUnknownDetailThroughHTTP` | Unknown and blank names return typed not-found and sanitized CLI failure without effects. | `A/P/F`, controlled API and process cleanup; candidate-controlled only after environment proof |
| C14-010 | `catalog_discovery_test.go:412` `TestModelsCatalogDiscoveryMapsUnsupportedOperationThroughHTTP` | Unsupported operation returns typed bad request before host/provider effects. | `A/P/F`, inert edges and stopped server; candidate-controlled only without host state |
| C14-011 | `catalog_discovery_test.go:452` `TestModelsCatalogReadinessFailureKeepsPublicUnavailableTaxonomy` and `:812` `TestModelsCatalogReadinessFailureStaysUnavailableThroughHTTP` | Dependency failure leaves public unavailable/not-found taxonomy and no success output across HTTP and CLI reads. | `A/F`, failing readiness edge and server cleanup; isolated failure state |
| C14-012 | `catalog_discovery_test.go:509` `TestModelsCatalogReadinessCancellationReturnsPublicFailure` | Canceled readiness in JSON CLI list returns public failure and no success stdout. | `A/F`, canceled inspect edge and stopped server; isolated cancellation state |
| C14-013 | `catalog_discovery_test.go:541` `TestModelsInvokeReadinessDependencyFailureIsUnavailableAfterCatalogSuccess` | Catalog succeeds, direct readiness fails, provider is not reached, and private dependency text is redacted. | `P/F`, controlled filesystem failure and process cleanup; isolated two-phase state |
| C14-014 | `catalog_discovery_test.go:573` `TestModelsInvokeCatalogDependencyCancellationIsSafeThroughProcess` | Canceled catalog dependency prevents provider call and dependency leakage. | `P/F`, canceled edge and process cleanup; isolated cancellation state |
| C14-015 | `catalog_discovery_test.go:597` `TestModelsInvokeCatalogRequestCancellationStopsReadiness` | Blocking readiness observes caller cancellation, command joins, and no partial result appears. | `P/F`, blocking edge and deterministic join; isolated lifecycle state |
| C14-016 | `catalog_discovery_test.go:639` `TestModelsInvokeReadinessCancellationAfterCatalogSuccessIsSafe` | Follow-up direct readiness cancellation returns safely with no invocation. | `P/F`, sequential canceled edge and process cleanup; isolated phase state |
| C14-017 | `catalog_discovery_test.go:667` `TestModelsInvokeReadinessCancellationAfterSuccessfulObservationIsSafe` | Post-query cancellation guard prevents false invocation and output. | `P/F`, caller context and inspect edge; isolated cancellation/recovery state |
| C14-018 | `readiness_assets_host_test.go:26` `TestModelsReadinessAssetsHostEffectsRemainInertThroughRootBuildProcess` | Readiness, asset, host, and compatibility effect totals remain zero during construction. | `P`, controlled effect recorder and process cleanup; isolated negative boundary |
| C14-019 | `readiness_assets_host_test.go:47` `TestModelsReadinessAssetsHostActivateThroughRootBuildProcessAfterLifecycle` | Cached paths, backend selection, readiness, output, and compatibility calls match after lifecycle. | `A/H/F/S`, local filesystem and controlled host; isolated host/asset/executable boundary |
| C14-020 | `story_003_test.go:38` `TestModelsPublicPullWorkflowProvesTruthfulTerminalState` | Empty-cache pull is observed before/during/after with truthful ready state, bytes, paths, and output. | `A/P/F/H`, owned cache/source and cleanup; isolated mutable pull state |
| C14-021 | `story_003_test.go:135` `controlled source failure` subtest | Source failure is typed, no false ready state appears, and no partial asset remains. | `A/P/F/H`, failing source and owned cache; isolated partial-download state |
| C14-022 | `pull_to_ready_test.go:35` `TestModelsPullToReadySurvivesProcessReconstruction` | A second root observes persisted readiness/cache and exact source requests without refetch. | `P/F`, two root lifecycles and asset client; isolated reconstruction/persistence |
| C14-023 | `story_003_test.go:207` `TestModelsPublicRemoveWorkflowProvesReclamationAndInUseRefusal` success portion | Unused cache bytes/path are reclaimed and terminal response is truthful. | `A/P/F`, destructive owned cache and cleanup; isolated destructive state |
| C14-024 | `story_003_test.go:260` `in-use response` subtest | Removal is refused while a lease is live and succeeds after release. | `P/H`, lease and handler cleanup; isolated concurrency/lease state |
| C14-025 | `coded_diagnostics_test.go:22` normal/json/debug rows plus `story_001_test.go:17` and `story_002_test.go:17` | Missing-cache code, family, stream, typed cause, debug cause, and safe message remain exact. | `P/F/D` and controlled HTTP parity fixtures; isolated output modes and local/HTTP boundary |
| C14-026 | `coded_diagnostics_test.go:66` `TestModelsLocalRemoveMissingCacheMatchesHTTPDiagnostic` | Local CLI and HTTP remove retain code, family, and message parity. | `P/A/H/F`, separate local and remote boundaries; isolated parity state |
| C14-027 | `coded_diagnostics_test.go:108` normal/json/debug inspect rows | Unknown-model inspect retains safe not-found code, stream, typed cause, and debug cause. | `P/F/D`, unique environment per row; candidate-controlled only after isolation proof |
| C14-028 | `coded_diagnostics_test.go:152` `TestModelsLocalInspectUnknownMatchesHTTPDiagnostic` | Local CLI and HTTP inspect retain code, family, and model-name parity. | `P/A/H/F`, separate boundaries; isolated parity state |
| C14-029 | `inference_invoke_test.go:162` `TestModelsJoinedBuiltinInvokeWithoutFactoryDeclaration` | Cached built-in reaches pinned host/protocol with exact request and output. | `P/F/H/S`, cached assets and host cleanup; isolated host/executable selection |
| C14-030 | `inference_invoke_test.go:275` `TestModelsGenericHTTPInvocationReachesJoinedRootThroughProcess` | Generic HTTP output crosses live root and controlled host exactly. | `A/H/F/S`, server/host cleanup; isolated API session and host state |
| C14-031 | `inference_invoke_test.go:425` `TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker` | Named route succeeds through effective definition; readiness and unknown-reference failures are typed without worker fallback. | `A/F`, provider command recorder and server cleanup; isolated route state |
| C14-032 | `inference_failure_test.go:67` `TestModelsGenericCLIProcessPublishesSingleOutputToStdoutOnly` | Generic CLI emits one stdout output and empty stderr. | `P/F`, controlled fixture and process cleanup; isolated stream ownership |
| C14-033 | `inference_failure_test.go:89` `TestModelsGenericCLIProcessRollsBackMappedOutputsThroughEdges` | Second publication failure restores prior files and removes temporary/partial artifacts. | `P/F/D`, filesystem edge; isolated rollback state |
| C14-034 | `inference_failure_test.go:131` `TestModelsGenericCLIProcessCancellationStopsReadinessAndPublishesNothing` | Cancellation yields no output, host stops, and typed cancellation returns. | `P/F/S`, blocking protocol and host cleanup; isolated cancellation/host state |
| C14-035 | `inference_failure_test.go:163` `TestModelsGenericCLIProcessTimeoutStopsReadinessAndPublishesNothing` | Deadline yields no output, host stops, and timeout class returns. | `P/F/S`, deadline and host cleanup; isolated timeout state |
| C14-036 | `inference_failure_test.go:196` `TestModelsGenericCLIProcessRedactsCrashedHostDetails` | Host crash becomes provider-neutral and excludes endpoint/secret details. | `P/F/S`, controlled crash and process cleanup; isolated privacy boundary |
| C14-037 | `story_004_embed_test.go:26` `zero-configuration` subtest | Zero-config EMBED JSON/CLI vector/output, cache-hit zero asset fetch, failure taxonomy, and recovery hold. | `P/F/H/S`, cached backend and host cleanup; isolated lease/output state |
| C14-038 | `story_004_embed_test.go:26` `oversized-file` subtest | Positive input limit rejects before backend, host, asset, or artifact effects. | `P/F/H/S`, local input and cleanup; isolated preflight boundary |
| C14-039 | `story_004_embed_test.go:26` `invalid-vector` subtest | Malformed backend vector is typed and a follow-up succeeds after lease release. | `P/F/H/S`, malformed protocol and host cleanup; isolated recovery state |
| C14-040 | `story_004_embed_test.go:253` `TestModelsEmbedCacheMissThenHitAvoidsNetworkThroughRootBuildProcess` | Miss performs manifest/model/backend exchanges; hit does not repeat them and paths remain exact. | `P/F/H/S/AS`, mutable cache and asset server cleanup; isolated cache boundary |
| C14-041 | `story_004_embed_test.go:304` `TestModelsEmbedHTTPParityUsesTheSameFixtureThroughRootBuildProcess` and grouped parity row | Named and generic HTTP routes retain vector, slot, envelope, and cache-hit behavior. | `A/H/S`, shared within one parity fixture only; isolated HTTP/session state |
| C14-042 | `delivered_embed_story_test.go:31,86` delivered CLI/HTTP artifact tests | Delivered artifacts retain miss/hit journey, exit/status, estimate, vector, protocol, asset calls, and cleanup. | Shipped executable, controlled fixture, local TCP/process boundary; isolated artifact evidence |
| C14-043 | `story_002_omni_test.go:15` `TestModelsOmniTextInputReachesPinnedCodecThroughRootBuildProcess` | Normalized prompt, one codec call, and exact response reach the pinned protocol. | `P/F/H/S`, process/server/host cleanup; isolated codec/host boundary |
| C14-044 | `story_003_omni_test.go:16` `TestModelsOmniFileInputsPreserveDetectedTypesAndImageOrderThroughRootBuildProcess` | Repeated image plus audio/video types and prompt/media order are preserved. | `P/F/H/S`, local files and codec cleanup; isolated media-order state |
| C14-045 | `story_004_omni_test.go:19` video-success portion | Supported video slot, modality, media, and exact response reach protocol. | `P/F/H/S`, controlled host/protocol; isolated capability state |
| C14-046 | `story_004_omni_test.go:19` unsupported audio/video portion | Capability rejection is typed before generation and no output files appear. | `P/F/H/S`, same fixture but preflight failure; isolated capability state |
| C14-047 | `story_004_omni_test.go:19` cancellation/follow-up portions | Cancellation emits no output; follow-up succeeds with exact total call count. | `P/F/H/S`, blocking protocol and recovery cleanup; isolated cancellation/recovery |
| C14-048 | `story_004_omni_test.go:34` `TestModelsOmniCancellationReleasesHostAcrossRootProcesses` | First root closes after cancellation and second root succeeds, proving host release. | `P/A/H/S`, two roots and exclusive launcher cleanup; isolated cross-root lease |
| C14-049 | `omni_protocol_transport_test.go:13` `TestOmniProtocolTransportRoundTripsThroughNetworkDialer` | Loopback listener round-trips envelope/content and closes connection/listener. | `T`, local-real transport; listener cleanup; isolated network fidelity |
| C14-050 | `localai_cli_conformance_test.go:22` `TestLocalAICLIConformanceMatrixRunsThroughRootBuildProcess` | Seven operations across two CLI surfaces retain exact output/order, host/compatibility calls, and zero asset network. | `L/P/A/F/S`, LocalAI fixture and process cleanup; isolated LocalAI boundary |
| C14-051 | `localai_http_conformance_test.go:29` `TestLocalAIHTTPConformanceMatrixRunsThroughRootBuildProcess` | Seven HTTP results succeed and the no-vertical probe remains unimplemented. | `L/A/F/S`, fixture/server cleanup; isolated HTTP/LocalAI boundary |
| C14-052 | `localai_failure_conformance_test.go:20` `TestLocalAIFailureDiagnosticsReachHTTPAndCLI` failure rows | Backend unavailable, protocol mismatch, and malformed response remain safe typed failures with no success and cleanup. | `L/A/P/F`, per-row fixture cleanup; isolated failure modes |
| C14-053 | `asr_story_test.go:27` `TestModelsASRDirectCLIEndToEndThroughRootBuildProcess` | Transcript, segments, audio metadata, request, and zero asset network remain exact. | `L/P/A/F/H/S`, cached ASR and LocalAI cleanup; isolated local-real artifact boundary |
| C14-054 | `tts_story_test.go:26` `TestModelsDirectTTSAliasEndToEndThroughRootBuildProcess` | Generic, alias, failure, and recovery rows retain four requests, exact audio, no partial file, and recovery. | `L/P/F/H/S`, cached TTS and host cleanup; isolated audio/recovery state |
| C14-055 | `delivered_cli_story_test.go:31` `TestDeliveredCLIArtifactReachesProtocolFixture` | Delivered ASR CLI retains protocol request, transcript, segments, audio, exit, and cleanup. | Shipped executable and local process boundary; isolated artifact evidence |
| C14-056 | `api_model_local_inference_long_test.go:35` with `functionallong` and explicit environment guard | Default package excludes real OMNIVOICE inference exactly as before; this lane does not enable or edit it. | Tagged local-real boundary; excluded from default evidence and isolated |
| C14-057 | `api_model_local_inference_long_test.go:623` `TestRecordedRealLocalModelEventDiagnostics`, 13 table rows | Recorded reducer preserves absent, malformed, wrong, valid file/URL, mixed, and duplicate response outcomes. | Pure recorded data under tagged file; shareable reducer, tagged boundary remains |
| C14-058 | `shared_process_test.go:141` `TestModelsSharedProcessEligibleScenarios`, two parallel subtests | One root/API serves two distinct sessions; route isolation, one-start topology, five opens/five closes, and cleanup all hold. | `P/A/F`, `sync.Once` fixture, explicit session cleanup; candidate-controlled |
| C14-059 | `forced_cleanup_test.go:33` `TestModels_ForcedAssertionFailureCleansOwnedResources`, child at `:110` | Intentional child failure still proves nonzero exit, host stop, cancellation, listener/process closure, and absent paths. | Local child process and controlled host/artifact; isolated forced-failure cleanup |
| C14-060 | `characterization_ledger_test.go:39` `TestMain`, three exact runs, JSON profile pass | Baseline samples, medians, exits, counter topology, wait census, hot spots, and eligibility are recorded here. | Test-only evidence; story 001 characterization gate |
| C14-061 | No current optimized row; owned by story `...-002` | Post-change every frozen assertion maps equal-or-stronger and none is deleted/skipped. | Deferred optimization/behavior gate |
| C14-062 | No current repeat/race row; owned by story `...-002` | Focused/package repeats and supported race prove reuse, cleanup, and no observed race. | Deferred repeat/race gate |
| C14-063 | No current final row; owned by story `...-003` | Clean rebased package passes with final median reduction or profile-backed floor. | Deferred clean-room/delivery gate |

This gives each matrix ID an exact current row or an explicit later-story
owner. Rows C14-001 through C14-059 are the frozen behavioral denominator;
C14-060 is the characterization evidence row, and C14-061 through C14-063
are explicitly deferred rather than claimed by this story.

## Cleanup and external-effect result

The baseline and profile processes reached `TestMain` cleanup. The existing
counter helper reported balanced shared-session opens/closes (`5/5`), and the
forced-failure parent remained able to observe its child cleanup witness. The
package's caller-owned processes use `support.CleanupProcess` or an equivalent
close path; servers, listeners, LocalAI fixtures, temporary directories,
hosts, caches, and artifacts retain their existing test-owned cleanup. No
cleanup assertion was removed or weakened.

No host HTTP calls or uncontrolled network calls were observed. Local TCP,
loopback HTTP, LocalAI, asset HTTP, and process-level artifact tests remain
isolated because their external state is itself part of the witness.

## Remaining unproven edges

- Optimized topology and behavior, including the current catalog CLI timeout:
  `GATE-C14-BEHAVIOR` and `GATE-C14-PERF` in story `...-002`.
- Three-run repeat and supported race evidence: `GATE-C14-REPEAT` and
  `GATE-C14-RACE` in story `...-002`.
- Independent clean-room proof on the final rebased head:
  `GATE-C14-CLEANROOM` in story `...-003`.
- PR CI-start comment and final delivery handoff:
  `GATE-C14-PR-CI` in story `...-003`.
- Terminal CI, merge, portable timing, and real local/remote inference remain
  review-owned or explicitly outside this lane.
