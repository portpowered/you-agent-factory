# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- `pkg/work/invocation/` owns pure invocation input, argument normalization,
  interpolation, return-policy selection, and stable policy errors used by CLI,
  API, workers, and Factory Session orchestration. It consumes domain-owned
  values and has no generated transport or live-session dependency.
- `pkg/workers/inference/` owns provider-neutral inference operation binding,
  request-envelope construction, and ordered output shaping shared by direct
  model invocation and Factory Session worker execution.
- `pkg/workers/skippermissions/` owns provider-backed worker capability and
  invocation-override policy for skip-permissions.
- `pkg/factory/sessions/invocation/session_owner.go` owns live-session request normalization,
  interpolation validation, default-handling Work submission, lifecycle
  sequencing, and delegation into the owner-local event-derived result waiter.
  `pkg/factory/sessions/invocation/session_wait.go` owns polling, timeout and cancellation,
  primary-result selection, and terminal classification over narrow runtime
  observations. `pkg/factory/sessions/invocation/session_telemetry.go` owns invocation metric
  names, low-cardinality labels, exactly-once emission points, safe structured
  log fields, and packaged-factory telemetry policy. Keep session configuration,
  Work submission, observation, wait/time behavior, telemetry sinks, and
  packaged-factory classification as explicit collaborators; service and
  runtime-host facades should only adapt those dependencies and forward
  `InvokeFactorySession` unchanged.
- `pkg/work/invocation/arguments.go` owns signature-backed invocation argument
  normalization for positional, named, stdin, defaulted, repeated, variadic,
  alias-backed, and compatibility fallback inputs. Transport stories should
  adapt CLI or API payloads into `NormalizeArgumentsInput` rather than
  re-implementing binding, default, or validation rules at the boundary.
- `pkg/work/invocation/interpolation.go` owns runtime `${parameter}` interpolation
  for signature-backed worker and workstation fields plus pre-dispatch
  interpolation validation. Keep file-contents substitution, omitted-exact-field
  behavior, and interpolation error codes there instead of duplicating
  string-replacement rules in service or worker executors. FILE_CONTENTS reads
  enter through the explicit `FileReader` collaborator supplied by those IO
  boundaries. The same package also
  owns replay-safe invocation diagnostics such as `InvocationSignatureHash` and
  `InvocationDiagnostic`; execution layers should reuse that summary instead of
  inventing transport- or worker-specific argument telemetry.
- `pkg/config/openapi_factory.go` must preserve exact `${parameter}` placeholders
  on enum-backed authored fields that support invocation interpolation (for
  example `workers[].modelProvider`) instead of rejecting them as invalid public
  enum values at the JSON boundary. Keep ordinary non-placeholder values on the
  existing strict enum normalization path so packaged and customer-authored
  factories can use the same interpolation-enabled authored fields.
- `pkg/config/operatordefaultsruntime/operator_defaults_runtime.go` is the
  startup-time runtime-validation seam for operator-defaulted model workers.
  When a model-worker `modelProvider` uses exact `${parameter}` invocation
  interpolation, validate that it references a declared signature parameter
  rather than forcing the authored placeholder through concrete provider
  validation during session startup.
- `pkg/work/invocation/primary_result.go` resolves invocation `primaryResult`
  against selected-tick `FactoryWorldState` using `WorkRequestsByID`,
  `TerminalWorkByID`, and payload-lineage scope rather than transport-specific
  polling logic. The same package also classifies missing-primary-result waits
  from scoped current work state when authored workflow states such as
  `blocked` or `needs-human` explain the stopped invocation better than the
  generic unresolved-primary-result fallback, classifies terminal failed work
  in invocation scope before that unresolved fallback, and classifies
  invocation control-state outcomes such as paused sessions or interrupted
  dispatches from reconstructed session and dispatch lifecycle facts. Stable
  non-success context for `sessionId`, `workId`, `workName`, and `workState`
  also originates here so CLI and API stay aligned on the same recovery facts.
- `pkg/factory/validation/validate.go` owns factory-level `invocationReturn`
  validation shared by validate-only and save pre-check flows.
- `pkg/config/factory_config_mapping*.go` maps `invocationReturn` between the
  OpenAPI factory contract and the internal runtime config.
- `pkg/interfaces/factory_runtime.go` owns the backend canonical
  `WorkContentPart`, request-validation error, and `FactoryInvocationResult`
  shapes used below transport and service boundaries; the Factory Session owner
  constructs that shared result.
- `pkg/work/content/contract` translates between generated OpenAPI `WorkContent`
  and the backend-owned `interfaces.WorkContentPart` shape; pure content rules
  remain in `pkg/work/content`.
- `pkg/api/handlers_work_write.go` includes the session invocation HTTP
  boundary alongside other session work-write handlers, including projection of
  shared invocation non-success context into the public `InvocationResponse`.
- `pkg/service/runtime_sessions.go` and
  `pkg/runtimehost/session_invocation.go` retain session-runtime compatibility adapters for
  session config, canonical Work submission, event-derived observations,
  metric/log sinks, and packaged-factory terminal classification. Their
  `InvokeFactorySession` methods must remain transparent forwards to
  `pkg/factory/sessions/invocation.SessionInvoker`; model-catalog files must not own Factory
  Session invocation behavior. Metric names, label policy, log shaping, and
  emission sequencing must not be reimplemented in these adapters. Submission
  sequencing, polling, and timeout/cancellation belong to
  `pkg/factory/sessions/invocation`; normalization, interpolation, primary-result selection,
  and general terminal classification belong to `pkg/work/invocation` and are
  delegated to by the stateful owner.
- API structured args use the direct structured-argument carrier rather than
  being reinterpreted as CLI named flags, so canonical parameter-name keys
  still work for positional-only or stdin-bound parameters. Treat `args: {}` as
  an explicit structured invocation request, not as omitted args, so
  all-optional or defaulted signatures stay transport-equivalent with CLI.
- `pkg/transports/cli/run/` is the `you run --factory` CLI boundary.
- Canonical default-path ownership for operator config
  (`~/.you-agent-factory/config.json`) and generated live replay recording roots
  (`~/.you-agent-factory/recordings/...`) belongs in `pkg/config/defaultpaths`;
  `pkg/config/operatorconfig` and `pkg/transports/cli/run` should keep only precedence,
  filename, and reporting behavior around those defaults.
- Persisted local `backendScopeID` values live in the same
  `~/.you-agent-factory/config.json` system config file. Keep load/generate/persist
  logic in `pkg/config/systemconfig`, resolve it during `service.BuildFactoryCore`
  before session identity is exposed, and keep `pkg/config/operatorconfig` tolerant
  of the top-level `backendScopeID` field so operator-default parsing still works.
  Local backend scope policy: blank values generate `local-<uuid>` once and persist
  it; valid `local-<uuid>` and other explicit non-empty scopes are reused across
  restarts; values starting with `local-` that are not valid `local-<uuid>` fail
  startup with a config error instead of being silently replaced.
- Canonical `you config init` system bootstrap belongs in
  `pkg/config/configinit` (`Init`, `SystemConfigOutcome`) and
  `pkg/transports/cli/configinit` (`Init`, `InitConfig`) with command wiring in
  `pkg/transports/cli/root.go` (`newSystemConfigCommand`, `newSystemConfigInitCommand`).
  Fresh homes create `~/.you-agent-factory/config.json` through
  `pkg/config/systemconfig.EnsureLocalBackendScope`; existing config files are
  validated with `operatorconfig.LoadFileConfig` and left byte-identical on
  re-run. `pkg/config/configinit` enumerates `pkg/factory/packages` and persists
  only missing catalog entries through `factoryconfig.PersistNamedFactory`;
  valid installed directories are loaded and skipped without rewriting
  customer-owned files. Isolated-home rerun coverage lives in
  `pkg/config/configinit/init_test.go` (`TestInit_DoubleRunIsSuccessfulNoOp`,
  `TestInit_PreservesUserEditedFactoryFilesOnRerun`,
  `TestInit_CreatesMissingPackagedDefaultsWithoutTouchingExisting`) and
  `pkg/transports/cli/configinit/init_test.go` / `pkg/transports/cli/root_config_init_test.go`. Keep
  `you factory config` factory.json tooling separate from this top-level
  operator/system initializer. Post-install bootstrap is invoked from
  `scripts/install.sh` and `scripts/install.ps1` via the installed binary's
  `config init` subcommand; installer smoke coverage lives in
  `tests/release/install_script_test.go` and `scripts/release/smoke-install.sh`
  / `scripts/release/smoke-install.ps1`.
- Production CLI command manifest parity for the root + `session show` family lives in
  `pkg/transports/cli/climanifest` (`LoadProduction`, `ProductionManifestPath`) and
  `pkg/transports/cli/climanifestparity` (`CompareDeclaredHandler`,
  `CompareHandlerOpenAPIBinding`, `OpenAPIOperationBinding`, `CompareLiveExitCodes`,
  `CompareBaselineSideEffects`, `CompareBaselineConstraints`, and
  `TestProductionCLIRootSessionFamily_NoGeneratorCutover`). Approved execution metadata
  for side-effects/constraints is loaded from
  `contracts/testdata/baseline/cli-command-execution.json`. Handler/OpenAPI binding for
  `you.session.show` asserts `operationId` `getFactorySession` maps to
  `GET /factory-sessions/{session_id}` in `api/openapi.yaml` and matches live
  `session.Show` JSON transport; constructor/generator cutover remains deferred to
  B10-CLI-GENERATOR.
- Operator default worker model settings resolve at the CLI/process boundary in
  `pkg/transports/cli/root.go` (`resolveOperatorDefaults`) and flow through
  `run.RunConfig.OperatorDefaults` into `service.FactoryServiceConfig` before
  `cmd/factory/compose.InjectCLITransport`; Wire providers must not read
  `~/.you-agent-factory/config.json` or `YOU_DEFAULT_WORKER_MODEL_*` directly.
- Process startup follows `cmd/factory -> pkg/root -> pkg/wire -> pkg/initializer`: `pkg/transports/cli/startup` carries parsed run or MCP inputs, `pkg/root` selects one `initializer.ProcessPolicy`, `pkg/wire/process.go` applies that policy while constructing exactly one typed `initializer.ProcessGraph`, and `pkg/initializer/core.go` validates the graph policy before starting the already-built graph. Do not duplicate or recompute mode/sidecar policy downstream; API, dashboard, runtime mode, worker-scheduler, and watcher enablement must be governed by the root-selected policy carried on the graph. Keep domain construction out of root and do not restore root-local deferred lifecycle closures or process-global builder registration. The normalized root home must likewise remain authoritative: thread it through config initialization, named-factory lookup, `run.RunConfig.HomeDir`, system-config persistence, automatic recording, runtime logging, and runtime metrics rather than consulting ambient process globals after command construction. Run construction is split between `run.BuildApplication` and `Application.Run`; MCP construction is split between `mcp.BuildServeApplication` and `ServeApplication.Run`, so construction failures occur before initializer startup. Initializer-backed local construction uses `pkg/initializer/cli_transport.go` (`InitializeCLITransport`, `CLITransport.Runner`) and `pkg/wire/cli.go` (`BuildCLIRunner`). Dashboard-suppressed non-invocation CLI runs (`--quiet`, work-file batch, clean-invocation batch) stay on `service.BuildFactoryService` through `wire.BuildCLIRunner`, while dashboard-suppressed one-shot invocation uses `service.BuildInvocationBootstrap` / `service.NormalizeInvocationBootstrapConfig` from `pkg/service/factory_build.go` via `pkg/transports/cli/run/factory_invocation_input.go` only. `InvocationBootstrap.InvokeFactorySession` and `InvocationBootstrap.CloseFactorySession` must stay transparent forwards to the wrapped `FactoryService`; `runFactoryInvocation` releases sessions through `releaseInvocationSession` after invocation instead of a CLI-local submit/wait loop. Boundary coverage lives in `pkg/root/process_test.go`, `pkg/wire/process_test.go`, `pkg/initializer/initialize_test.go`, and the compiled-binary matrix in `tests/release/root_process_smoke_test.go`; transport parity coverage remains in `pkg/initializer/startup_compatibility_test.go`. Focused initializer migration verification: `go test ./cmd/... ./pkg/transports/http/... ./pkg/transports/cli/... ./pkg/transports/mcp/... ./pkg/initializer/... -short`.
- `you models invoke` reuses the same `service.BuildInvocationBootstrap` /
  `service.NormalizeInvocationBootstrapConfig` path as one-shot factory
  invocation, constructed by `pkg/wire/model_invocation.go` from the typed
  request carried through `pkg/root` and `pkg/transports/cli/root.go`. The models
  transport must fail fast when the builder is absent and call
  `FactoryService.InvokeModel` through the bootstrap-owned service rather than
  posting to `/models/{model_name}/invocations`; keep managed readiness/lifecycle
  error mapping aligned with `pkg/transports/http/handlers_models.go` via
  `mapBootstrapModelInvokeError`. Return classified `InferenceFailure` and
  `ManagedRuntimeInvocationError` values without re-wrapping so readiness causes
  stay `errors.Is`-able;   non-ready bootstrap invoke coverage lives in
  `pkg/transports/cli/models/non_ready_invoke_test.go` (stub bootstrap lifecycle vocabulary) and
  `pkg/transports/cli/models/bootstrap_lifecycle_invoke_test.go` (offline MISSING/LOADING/FAILED
  integration through the real bootstrap). Ready offline invoke coverage lives in
  `pkg/cli/models/offline_ready_invoke_test.go`; bootstrap routing and failure-baseline
  contracts live in `pkg/cli/models/bootstrap_invoke_test.go` and
  `pkg/cli/models/failure_baseline_no_server_test.go`. Factory root resolution for invoke belongs in
  `pkg/cli/models` (`resolveModelsInvokeFactoryDir`), with operator defaults and
  logger passed from `pkg/cli/root.go` `newModelsInvokeCommand`.
- `pkg/cli/run/run.go` resolves positional versus non-TTY stdin through the
  shared `pkg/work/invocation` contract, then runs the local service in
  invocation-only service mode so stdout stays reserved for primary-result
  output instead of startup or dashboard noise; CLI-only source conflicts are
  logged and counted there before the service runtime exists. `you run
  --skip-permissions` is registered in `pkg/transports/cli/root_work.go`, mapped to
  `RunConfig.InvocationSkipPermissionsOverride`, and forwarded through
  `buildRunServiceConfig` into `service.FactoryServiceConfig` as an ephemeral
  invocation override that must not mutate persisted worker `skipPermissions`.
  `pkg/workers/skippermissions.EffectiveSkipPermissions` resolves persisted worker config plus
  `FactoryServiceConfig.InvocationSkipPermissionsOverride` when building
  provider-backed worker CLI args in `pkg/service/factory_build.go` and
  `pkg/runtimehost/build_workers.go`. `skippermissions.ValidateInvocationSkipPermissionsWorkers`
  and `ValidateInvocationSkipPermissionsForWorker` fail closed before worker
  construction when `--skip-permissions` is set but an agent worker uses an
  unsupported CLI provider or local managed model path. S14 regression evidence
  lives in `pkg/workers/skippermissions/skip_permissions_test.go`,
  `pkg/workers/provider/provider_behavior_test.go`, and
  `pkg/service/factory_test_helpers_test.go` alongside the story-level
  propagation and fail-closed service tests. `RunConfig.JSONOutput`
  must stay aligned with the shared `InvocationResponse` envelope for both
  successful and non-success invocation results rather than becoming a
  success-only CLI fork. `RunConfig.InvocationOutputMode` and `you run --output`
  select primary-result-only versus the session-owned canonical
  `FactoryResponseEvent` subscription for supported one-shot factory invocations.
  Do not fall back to legacy provider-progress payloads when the canonical
  subscription is unavailable. Keep mode validation, unsupported
  run-shape rejection, and fallback behavior in `pkg/transports/cli/run/invocation_error.go`,
  stream attachment, bounded human-progress draining, and lossless canonical
  JSON stdout ordering in
  `pkg/transports/cli/run/invocation_observability.go`, human progress and canonical JSON rendering in
  `pkg/transports/cli/run/run_clean_invocation.go`, response-stream unit tests in
  `pkg/transports/cli/run/run_config_test.go`, response-stream CLI integration tests in
  `pkg/transports/cli/run/run_wire_api_test.go`, and invocation wiring in
  `pkg/transports/cli/run/factory_invocation_input.go`. `pkg/transports/cli/root_work.go` and
  `pkg/transports/cli/root_run_test.go` apply manually parsed `you run --output response-stream`
  to `RunConfig.InvocationOutputMode` after `DisableFlagParsing` argument parsing.
  The `pkg/transports/cli/run` package is at the
  15-file limit; extend existing files instead of adding new ones. Human response-stream
  terminal outcomes use `--- invocation outcome ---` with structured status/error
  fields. Both human and JSON modes subscribe from the latest session-owned
  canonical response event. Human mode validates kind/phase and renders only its
  bounded typed allow-list; JSON emits only `response_event` records and sends every event plus the
  final `invocation_result` through one lossless ordered writer. Do not reuse the
  human progress queue's drop or drain-timeout policy for canonical JSON records.
  Keep `service.InvocationBootstrap.SubscribeSessionResponseEventsFromLatest`
  as a transparent forward and explicitly drain the retained subscription after
  stopping its live consumer so an event published at invocation return remains
  ordered before the terminal record.
  Canonical retained-window loss reaches both modes as a public `STREAM_GAP`
  event. The legacy internal stream remains available to non-human compatibility
  consumers through `pkg/service/runtime_sessions.go`, but is not a CLI human
  presentation fallback.
  Provider-neutral `FactoryResponseEvent` vocabulary lives in
  `pkg/factory/sessions/responseevents` (distinct from internal
  `pkg/factory/sessions/responsestream` fragment kinds).

  Provider-native structured response adapters live in provider-owned
  subpackages under `pkg/workers/provider/` and implement the neutral lifecycle
  in `pkg/workers/provider/adapter`. Keep each decoder invocation-local and
  stateful, return only canonical `responseevents.Draft` values plus bounded
  diagnostics, and leave event ID, sequence, recorded time, and Factory Session
  publication to the session owner. Adapter `BuildCommand` selects structured
  output only for the response-adapter execution path; the established
  final-only provider command path must remain unchanged until its caller
  explicitly opts into response streaming. Both command paths must use
  `pkg/workers/provider/commandenv` so provider variables retain the established
  non-interactive Git/editor safeguards, and the production mode-selection
  boundary must preserve provider input validation before starting either
  runner. Native JSONL fixture tests should
  fragment reads and flush an unterminated final record so command selection,
  decoder buffering, and final-result parsing are proven independently.
  Provider retry and compaction records should publish only bounded typed facts
  with static safe messages; adapters may classify those facts but must not
  sleep, rerun commands, choose backoff, or expose raw provider payloads.
  Preserve optional provider attribution only from an explicit native field on
  the supported record, and omit malformed or absent attribution instead of
  inferring it from neighboring stream activity.

  Legacy fragment
  compatibility mapping lives in `pkg/factory/sessions/responsestream/compat`
  (`MapFragment` over `responsestream.Event` with session/run `Context`); keep
  the mapper pure, table-tested, and free of CLI/HTTP/provider imports while
  later transport lanes adopt mapped canonical events. Response fragments map to
  `MESSAGE`/`DELTA` with `MessageDeltaPayload` (`contentBlockIndex` 0,
  `contentBlockKind` `TEXT`, `textDelta` from fragment payload without parsing
  provider grammar) and dispatch-scoped `item-legacy-*` IDs from
  `factorySessionId|runId|dispatchId|providerSessionRef`. Terminal stream markers
  map to `RUN`/`COMPLETED` (`RunPayload.status` `completed`) or `ERROR`/`FAILED`
  (`ErrorPayload` with stable `stream_failed` / `stream_canceled` codes and
  fragment payload as message) without selecting invocation primary results.
  Providers with structured stream decoders must publish their
  `responseevents.Draft` values from the real subprocess observation boundary
  through `workerprovider.StructuredResponseEvent`; the session stream manager
  publishes those drafts directly to the canonical response-event store and
  bypasses legacy fragment compatibility mapping. Keep final invocation result
  parsing independent so the provider's authoritative terminal value remains
  both the response-event message snapshot and the invocation primary result.
  Timeout- or cancellation-captured assistant snapshots carry
  `MessagePayload.partial=true`; use `responseevents.IsAuthoritativeMessageSnapshot`
  to exclude them from authoritative final-result selection. Agy timeout partial
  text must stay out of `InferenceResponse.Content` and invocation primary-result
  selection even when published on the canonical response stream.
  Compaction signals map to `STREAM_GAP`/`UPDATED` with `StreamGapPayload`
  (`fromSequence`/`toSequence` from `CompactionSummary` dropped bounds when
  present, `reason` from compaction reason) and always `LOSSY` provenance.
  `compat/mapper_fixture_matrix_test.go` is the consolidated coverage matrix for
  every declared legacy fragment kind plus legacy publisher smoke; focused
  invocation primary-result byte fixtures live in
  `compat/testdata/primary_result_regression/` and are asserted by
  `primary_result_regression_test.go` without wiring the mapper into selection.
  Provider-native typed adapters live under `pkg/workers/provider/<provider>`
  and emit validated `responseevents.Draft` values. Sanitized cross-provider
  parity transcripts and the adapter-neutral terminal harness live in
  `pkg/workers/provider/parityfixtures` with fidelity-class fixtures under
  `testdata/`; extend that catalog for CLI/API parity proofs instead of
  inventing parallel fixture trees. Use `parityfixtures.RunTransportParity`
  plus `AssertCLIAPITransportParity` and `AssertTruthfulStreamingFidelity` to
  compare decoded CLI NDJSON and API SSE `FactoryResponseEvent` values and
  terminal `InvocationResponse` outcomes for every fidelity class (full-stream,
  partial-stream, snapshot-only, and final-only including Agy) plus the structured
  tool-lifecycle fixture via `AssertObservableToolLifecycle` before adding new
  transport parity tests. Use `AssertPrimaryStreamModeParity` with
  `ProjectPrimaryOnlyInvocation` and `ProjectResponseStreamInvocation` to prove
  primary-only and response-stream observation modes agree on authoritative
  terminal `InvocationResponse` outcomes for the same fixture run. Consolidated
  Batch 09 parity proofs live in `parityfixtures.AssertCrossProviderParityCatalog`
  and `AssertCrossProviderParityForFixture`; run them from
  `pkg/workers/provider/parityfixtures/suite_test.go`
  (`TestCrossProviderParitySuite_Catalog`) and the provider-suite entrypoint
  `tests/functional/providers/cross_provider_parity_smoke_test.go`
  (`TestCrossProviderParitySmoke_ProviderSuiteEntrypoint`). Maintainer lanes:
  `make provider-parity-smoke` (also invoked by `make api-smoke`) and
  `make response-stream-stress-smoke` for response-event backpressure/race proofs.
  While legacy response-stream consumers remain supported, carry an exact draft beside the compatibility
  fragment and let `pkg/factory/sessions/stream/manager.go` publish that draft
  directly; do not remap it through the lossy legacy fragment mapper. Keep the
  provider's final-result parser independent from decoder observation state so
  streamed message snapshots cannot select or duplicate invocation
  `primaryResult`. For typed item unions, classify semantics only from the exact
  nested item discriminator, retain the provider's native item ID across start,
  update, and completion records, and represent the completed full item as the
  authoritative snapshot rather than synthesizing a second completed item.
  When the native item union distinguishes non-fatal error items from terminal
  stream errors, map those items to correlated `ERROR`/`UPDATED` snapshots and
  reserve `ERROR`/`FAILED` plus invocation failure classification for terminal
  records and process outcomes.
  Reconcile typed terminal failures before generic process-exit fallback, but
  preserve cancellation and timeout precedence. Shared selection lives in
  `pkg/workers/provider/failure_precedence.go` (`SelectFailureByPrecedence`) and
  provider-owned collectors such as `ResolveCodexProviderFailure` in
  `codex_failure_resolution.go`; precedence table tests belong beside those
  helpers rather than in transport layers. Cross-path agreement tests for
  listed failure classes belong in `codex_failure_reporting_agreement_test.go`
  and should compare `CodexStructuredStreamReportingOutcome` against
  `CodexProcessExitReportingOutcome` before shared precedence selection.
  Bounded internal-cause excerpts ride on `ProviderFailureResolution.InternalCause`
  and `ProviderError.Cause`; sanitized alignment fixtures belong in
  `codex_failure_sanitized_fixture.go` with leakage negatives in
  `codex_failure_internal_cause_test.go`. Invocation error code compatibility
  coverage lives in `provider_invocation_error_compatibility_test.go` and should
  lock stable `WorkFailureType` / `FailureDetail.Reason` values across corpus
  normalization and Codex reporting-path agreement probes. A streaming decoder must hold
  terminal `ERROR` drafts until the shared executor flushes it with the process
  outcome; discard a native failure when cancellation, deadline, or exit 124 wins.
  When multiple typed terminal records arrive, the held canonical draft and
  final failure parser must use the same selection rule: recognized failures
  outrank later unrecognized cleanup errors, while later recognized failures
  may replace earlier ones.
  When the native decoder publishes the surviving exact terminal `ERROR` draft,
  keep the legacy terminal marker for response-stream consumers while explicitly
  suppressing its second canonical projection.
  Treat provider JSONL as a bounded record stream: diagnose and discard one
  oversized record without retaining the rest of that line, then resume at the
  next newline. Decoder flush and independent final/failure parsers must apply
  the same record boundary so an unknown oversized record cannot hide a later
  authoritative completion or typed failure, and diagnostics must describe the
  class/discriminator without copying raw provider payloads.
  Session-scoped immutable response-event storage lives in
  `pkg/factory/sessions/responseeventstore` with
  `factorysessions.SessionResponseEventStore` aliases in `types.go`; it is
  session-runtime-local state separate from canonical `FactoryEvent` history.
  `factorysessions.NewLiveSession` allocates one store using the canonical
  Factory Session ID. Runtime composition binds canonical `SESSION_COMPLETED`
  observation to `CompleteResponseEvents`, and live-session teardown closes the
  store alongside legacy response streams; keep this lifecycle state on
  `LiveSession`, not `FactoryService`.
  `SessionResponseEventStore.Subscribe(afterSequence)` delivers retained events
  after the cursor, then continues live via `Subscription.Next`. Retention keeps
  ordered unavailable-sequence spans separate from immutable retained events;
  stale reads receive one cursor-clipped `STREAM_GAP`/`UPDATED` marker with
  `retention_window` reason and lossy provenance before catch-up. The marker is
  out-of-band at sequence zero and advances only the private subscription
  cursor, so it never consumes, reuses, or renumbers published identities;
  optional
  `WithDispatchFilter(dispatchID)` omits non-matching events while preserving
  each delivered event's global session sequence and eventId. `Complete()` stops
  further publishes while retained events remain for catch-up; catch-up readers
  created after completion are not registered as live subscribers. Publishing
  rejects an explicit Factory Session ID that differs from the store's canonical
  identity. `Close()` rejects new subscriptions and publishes and detaches active
  subscribers. Mirror
  `responsestream.Subscription` patterns when extending close/complete behavior.
  Package docs in `responseevents/doc.go` record resolved v1 transport, retention, and CLI JSON
  decisions without implementing transports; `responseevents/boundary_test.go`
  enforces isolation from CLI, HTTP, subprocess, and provider imports.
  `responsestream.StreamSet.CloseDispatch` retains completed dispatch streams so
  late CLI pollers can still subscribe and drain retained progress until the
  completed-dispatch retention window expires; `runResponseStreamAttachment` in
  `pkg/transports/cli/run/invocation_observability.go` performs one final dispatch-ID
  discovery pass when attachment shutdown is requested so dispatches that
  complete between poll ticks are still subscribed before `streamAttachment.stop()`
  returns. `StreamSet` evicts completed streams after
  `DefaultCompletedDispatchRetention()` and re-enforces per-stream retention
  through `SessionResponseStream.EnforceRetention()` without relying on future
  `Append` calls. `responseStreamProgressWriter` serializes all stdout writes
  through `outputMu`; after a drain timeout it abandons further progress writes
  and `writeFinalInvocationResult` acquires the same lock so final
  primary-result/outcome output cannot interleave with an in-flight progress write.
- `you run --replay` format detection belongs at service composition before legacy
  `ReplayArtifact` config loading. Privacy-bounded JavaScript Factory Session
  recordings compose `recordingreplay.Service` as an inspection-only durable read
  owner and skip Petri/runtime/provider construction; legacy artifacts continue
  through `pkg/replay`. Production-path tests must exercise `FactoryService.Run`
  plus public session, event, artifact, and result reads with fail-on-use live
  dependencies.
- `pkg/cli/run/factory_invocation_input.go` must pass raw positional/stdin
  bytes into `pkg/work/invocation.ResolveTextInput` and surface `INVOCATION_INPUT_EMPTY`
  from the shared resolver instead of pre-trimming or short-circuiting with
  transport-specific empty-stdin errors. When `Stdin` is overridden away from
  `os.Stdin` (cobra `SetIn`, tests, or programmatic callers), treat it as piped
  input even if the process-level `os.Stdin` is still a TTY.
- `pkg/workers/inference/inference.go` resolves inference-run operation bindings,
  maps direct invocation request bindings, builds the provider-neutral inference
  request envelope, and shapes inference responses into ordered canonical
  `WorkContentPart` output shared by direct model invocation and factory-session
  execution paths.
- `pkg/transports/mapping/inference_failure.go` classifies inference readiness and
  execution failures into actionable customer-facing outcomes for missing model,
  loading model, unsupported operation, timeout, and runtime failure cases
  shared by direct model invocation and HTTP handlers.
- `pkg/workers/executor/model_operation_bindings.go` delegates inference binding
  resolution to `pkg/workers/inference`.
- `pkg/cli/root_run_args.go` owns the `you run` manual flag split that preserves
  known run and inherited flags while leaving unknown `--factory-arg` tokens
  intact for signature-backed parsing; keep factory-argument normalization
  itself in `pkg/cli/run/factory_invocation_signature_input.go` plus
  `pkg/work/invocation/arguments.go` rather than re-implementing binding logic in
  Cobra parsing.
- `pkg/work/invocation/input.go` owns logical empty-text detection via
  `strings.TrimSpace` inside `ResolveTextInput` and `ResolveAPITextInputContent`;
  CLI and API adapters must not duplicate whitespace-only rejection.
- `pkg/transports/cli/root.go` owns the customer-facing `you run --factory` help text for
  invocation input-source rules and the canonical pointers into packaged docs.
  `runInvocationModes` and `resolveRunFactoryPrompt` also treat `you run --named`
  as an invocation factory selector for positional/stdin text.
  `runFactory` resolves `--named` / `--factory` / `--dir` conflicts and portable
  `--factory` preflight before loading operator defaults so flag and path failures
  stay independent of `~/.you-agent-factory/config.json` contents.
- `pkg/transports/cli/root_run_test.go` isolates `HOME` for the whole CLI package so `make test`
  does not depend on the developer's real operator config file.
- Real-service tests under `pkg/transports/cli/run` must set `ExecutionBaseDir` to a
  `t.TempDir()` root so project-local durable Factory Session snapshots never land in
  the package working directory. Set `DisableDefaultRecording` when replay recording is
  irrelevant; tests that cover recording should inject `defaultLiveRunRecordPath` beneath
  `t.TempDir()` and assert the resolved artifact there.
- `internal/releasesmoke/harness.go` isolates spawned `you run` smoke processes from
  the developer's real `HOME` so `tests/release` stays hermetic through
  `make test`.
- `pkg/factory/packages/catalog.go` owns packaged factory lookup and metadata;
  payload sources live under `pkg/factory/packages/definitions/`, and config
  initialization is the only catalog-to-disk installation boundary. Named
  resolution in `pkg/config/layout.go` reads project-local then global disk
  state only; it does not install packages or expose compatibility JSON aliases.
  Packaged `@you/goal`
  has one `execute-goal` `AGENT_RUN` workstation with `REPEATER` behavior:
  accepted completion routes to `goal:complete`, continue/reject route back to
  `goal:init`, and worker or workstation failure routes to `goal:failed`.
  `pkg/factory/packages/definitions/goal/` owns the authored factory and concise
  executor prompt; assembly and materialization require only `goal-executor` and
  `execute-goal`.
  Packaged workstation `body` templates must use canonical `PromptData` roots
  such as `(index .Inputs 0).Payload`; legacy top-level aliases like
  `{{ .WorkID }}` fail prompt rendering before mock-worker dispatch. Resolution
  never repairs legacy prompts, so installed files remain customer-owned and
  byte-for-byte unchanged by `you run --named` lookup.
- `pkg/factory/packages/definitions/subagent/` owns the authored `@you/subagent` one-pass factory
  scaffold (`factory.json`, prompt files) assembled into `BuiltInSubagentFactoryJSON`
  and registered by `pkg/factory/packages/catalog.go`. The topology uses exactly one `AGENT_WORKER`
  with explicit `agentTools.policy` and one `AGENT_RUN` workstation that interpolates
  `${input}` from the invocation signature into the workstation prompt body.
  `you config init` installs `@you/subagent` under the global named-factory root
  before named invocation can resolve it.
- `pkg/factory/packages/subagent/` owns packaged subagent factory metadata constants,
  topology validation coverage, materialization/edit-safe identity tests, response
  shaping helpers for terminal `task:complete` work content, and primary-result
  selection tests for the one-pass built-in factory JSON.
- Hermetic no-server named `@you/subagent` package proof lives in
  `pkg/transports/cli/run/run_invocation_test.go`
  (`TestRun_NamedSubagentHermeticInvocationSucceedsWithoutListeningServer`,
  `TestRun_NamedSubagentNoServerBootstrap_TextPrimaryResultIsAgentResponse`,
  `TestRun_NamedSubagentNoServerBootstrap_SuccessJSONMatchesAPIProjection`,
  `TestNoServerNamedSubagentInvocationIntegrationAndEquivalenceProof`), using the
  real shared bootstrap path with mock workers and deterministic API-server
  starter guards to assert no factory API/dashboard listener is served and
  exactly one agent-response `primaryResult` is returned. Do not use a
  close-and-rebind TCP probe for this assertion: another package test or process
  can claim the released port and make the proof nondeterministic.
- `pkg/transports/cli/run/factory_invocation_help.go` owns the factory-aware help renderer
  for `you run --named <factory> --help` and `you run --factory <factory.json> --help`.
  Keep usage lines, parameter descriptions, defaults, accepted values, output
  hints, and example rendering derived from `interfaces.InvocationSignatureConfig`
  instead of hard-coding packaged-factory argument inventories in CLI help.
- `docs/reference/run.md` (`you docs run`) owns supported `@you/fusion`
  invocation and signature-aware help. Factory materialization, examples, and
  edit-after-materialize behavior belong in
  `docs/reference/authoring-factories.md`.
- `pkg/factory/packages/goal/` owns packaged goal factory metadata constants and
  config-load regression coverage for the authored `invocationReturn` policy that
  selects terminal `goal:complete` work content as the primary result.
  `summary.go` shapes terminal `execute-goal` work content from worker output so
  EXPLICIT primary-result selection returns the final summary instead of
  submitted goal input text. `primary_result_test.go` covers both
  successful EXPLICIT selection and unresolved failure when `goal:complete` is
  absent from terminal work in scope.
- `pkg/factory/packages/goal/decision_envelope.go` owns the canonical
  reviewer/checker JSON envelope and its mapping onto `interfaces.WorkResult`.
  Goal routing envelopes with authored `classificationRoutes` map parsed
  `decision` labels onto `SelectedClassificationLabel` while preserving
  `Feedback`, optional `Output`, and `RecordedOutputWork`.
- `pkg/workers/executor/agent.go` routes workstations with
  `outcomeFormat: decision-envelope` through
  `goal.WorkResultFromDecisionEnvelopeJSONOrFailed` instead of stop-token parsing.
  Those workstations with authored
  `classificationRoutes` use `goal.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed`.
- `factory/docs/decision-envelope.md` is the packaged-authoring guide for the
  reviewer/checker envelope shape, the standard outcome vocabulary, the
  packaged-goal goal-routing decision vocabulary used when
  `classificationRoutes` are present, and malformed-input behavior.
- `pkg/factory/subsystems/subsystem_transitioner.go` applies packaged goal
  invocation summary shaping on the single `execute-goal` repeater alongside
  packaged TTS metadata shaping.
- `pkg/factory/subsystems/goalroutingtests/transitioner_goal_routing_test.go`
  proves the assembled minimal topology repeats continue/reject outcomes through
  `goal:init` and routes worker failure to `goal:failed` without live providers.
- Behavioral proof for named goal batch invocation lives in
  `tests/functional/smoke/cli_named_goal_run_smoke_test.go` using the real
  `you run --named @you/goal` CLI path with `--with-mock-workers` after explicit
  configuration initialization. Read-only miss and edit-preservation coverage
  lives in `pkg/config/runtimetests/named_factory_resolution_test.go`.
- Hermetic no-server named `@you/goal` package proof lives in
  `pkg/transports/cli/run/run_invocation_test.go`
  (`TestRun_NamedGoalHermeticInvocationSucceedsWithoutListeningServer`), using
  the real shared bootstrap path with mock workers and deterministic API-server
  starter guards to assert no factory API/dashboard listener is served.
  `run.BuildApplication` must also skip listener reservation entirely in
  invocation mode rather than briefly binding and then discarding a listener.
- No-server bootstrap CLI/API invocation-equivalence proof lives in
  `pkg/transports/cli/run/run_invocation_test.go`
  (`TestRun_NoServerBootstrap_PositionalInputMatchesAPIContract`,
  `TestRun_NoServerBootstrap_StdinInputMatchesAPIContract`,
  `TestRun_NoServerBootstrap_SuccessJSONMatchesAPIProjection`,
  `TestRun_NoServerBootstrap_TextPrimaryResultFollowsInvocationReturn`), capturing real
  `defaultBuildInvocationBootstrap` invoke requests/results and comparing them
  to the shared API text-input resolver plus `apisurface.InvocationResponseFromResult`
  projection for packaged `@you/goal` primary-result selection.
- Consolidated no-server named integration and invocation-equivalence proof for
  reviewers lives in `pkg/transports/cli/run/run_invocation_test.go`
  (`TestNoServerNamedInvocationIntegrationAndEquivalenceProof`), combining
  hermetic `@you/goal` success without a TCP listener with shared input-resolution
  and primary-result equivalence on the real bootstrap path.
- CLI/API invocation parity for packaged `@you/goal` lives in
  `tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go`,
  comparing live session invocation API responses with real CLI `--json` output
  for positional, stdin, and named-factory success paths plus representative
  empty-input and unresolved-primary-result failures. Reuse
  `scaffoldPackagedGoalInvocationFactoryForSmoke`, `buildYouCLIBinary`, and
  `support.StartFunctionalAPIServer` when extending parity coverage.
- Final `@you/goal` decision-routing smoke coverage lives in
  `tests/functional/smoke/cli_named_goal_routing_smoke_test.go`, exercising
  named-factory CLI `--json` outcomes for accepted, blocked, needs-human, and
  failed classifier labels plus API-backed fixtures for interrupted routing,
  needs_changes rework loops, and structured unknown decisions. Reuse
  `writePackagedGoalBuiltinTopologyMockWorkers`, `materializeNamedGoalFactoryForRoutingSmoke`,
  and `support.StartFunctionalAPIServer` when extending routing verification.
  Complement with `pkg/factory/subsystems/goalroutingtests/transitioner_goal_routing_test.go`
  and `tests/functional/runtime_api/api_packaged_goal_invocation_test.go` for
  transitioner and topology-level routing proofs.
- Named `@you/goal` operator-control smoke coverage lives in
  `tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go`,
  proving API and CLI pause/resume buffering, ordered post-resume drain via
  plan-goal dispatch `StartTime` ordering in `DispatchHistory`, interrupted
  inspect summaries via `session show` and `work show`, and durable
  `SESSION_LIFECYCLE_CONTROL` replay events. Reuse
  `writePackagedGoalSlowPlannerTopologyMockWorkers` when ordered drain timing
  needs observable separation between buffered submissions.
- Named `@you/goal` response-stream boundary smoke coverage lives in
  `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go`,
  proving real CLI `--output response-stream` still returns the packaged
  `primaryResult`, JSON response-stream NDJSON contains canonical `response_event`
  records and ends with an `invocation_result` record, durable `FactoryEvent`
  history omits internal response-stream terms,
  and generated public API artifacts stay internal-only. Reuse
  `writePackagedGoalBuiltinTopologyMockWorkers`, `materializeNamedGoalFactoryForRoutingSmoke`,
  and `support.StartFunctionalAPIServer` when extending boundary verification.
- `tests/functional/smoke/cli_run_mode_compat_smoke_test.go` holds focused
  regression coverage for adjacent `you run` modes after packaged-goal changes:
  operator-oriented continuous startup output without `--quiet`, factory text
  invocation stdout that suppresses operator chatter, and named-goal batch
  stdout that stays primary-result-only. Reuse helpers from
  `cli_factory_prompt_run_smoke_test.go` when extending these regressions.
- `pkg/factory/packages/tts/` owns packaged TTS invocation metadata shaping
  helpers used when `INFERENCE_RUN` (or legacy `MODEL_INVOKE`) work completes on the `execute-tts` workstation.
  `metadata.go` derives the `backend` metadata field from the loaded on-disk
  worker model so customer edits to materialized `factory.json` affect the next
  invocation result.
- `docs/reference/run.md` (`you docs run`) owns supported `@you/goal` batch
  invocation, stdout primary-result, and response-stream guidance.
  `docs/reference/sessions.md` owns operator controls and inspect-first recovery;
  `docs/reference/authoring-factories.md` owns named Factory authoring and
  materialization. Prove packaged guidance through the installed command in
  `tests/functional/smoke/cli_docs_smoke_test.go`; maintainer final-verification
  evidence for public-contract boundaries lives in
  `docs/internal/development/plans/you-goal/api-contract-audit.md`.
- `docs/reference/models.md` is the customer guide for `INFERENCE_RUN`,
  `INFERENCE_WORKER`, managed-runtime `/models` surfaces, local modelhost lease
  execution, and legacy `MODEL_INVOKE` / `MODEL_WORKER` migration aliases.
- `docs/reference/models.md` (`you docs models`) owns supported TTS discovery,
  readiness, direct invocation, result selection, and raw-audio output guidance.
  Named `@you/tts` invocation belongs in `docs/reference/run.md`; Factory
  authoring, materialization, metadata, and edit-after-materialize behavior
  belong in `docs/reference/authoring-factories.md`. Prefer `INFERENCE_WORKER` /
  `INFERENCE_RUN` terminology in retained guidance while documenting
  `MODEL_WORKER` / `MODEL_INVOKE` as migration aliases.
- `pkg/factory/packages/tts/observability.go` classifies packaged TTS loading,
  model-not-ready, and generation-failure outcomes and defines stable invocation
  error codes plus packaged-factory metric names.
- `pkg/transports/cli/run/packaged_tts_invocation.go` logs named-factory resolution context at
  the CLI boundary without recording packaged-factory metrics or logging submitted
  text or generated artifact bodies.
- `pkg/factory/packages/goal/` owns packaged `@you/goal` factory metadata
  constants (`PackagedFactoryName`, `PackagedInvokeWorkstationName`).
- `pkg/transports/cli/run/run_invocation_test.go` proves `@you/goal` CLI invocation input
  sources resolve through `invocations.ResolveTextInput`, reach the shared
  `InvocationRequest` payload shape, fail with stable
  `INVOCATION_INPUT_SOURCE_CONFLICT` before `InvokeFactorySession`, and match the
  session invocation API contract for the same logical text input and JSON success
  envelopes.
- `pkg/transports/cli/root_run_server_test.go` proves root `you run --named @you/goal` and
  `@you/tts` wiring for positional text, piped stdin, explicit `-` stdin forms,
  and stable `INVOCATION_INPUT_SOURCE_CONFLICT` rejection when sources combine.
- `pkg/transports/http/server_factory_sessions_test.go` proves the session invocation API
  returns the same observable request and primary-result behavior for packaged
  `@you/goal` text input and source-conflict failures as the CLI parity tests.
- Dashboard signature-backed invocation submission belongs in
  `ui/src/api/session-factory/` plus `ui/src/features/submit-work/`. Keep the
  transport wrapper on `POST /factory-sessions/{session_id}/invocations`,
  generated field projection/serialization in feature-local pure helpers, and
  dev/preview proxy coverage aligned in `ui/vite.config.ts` +
  `ui/vite.config.test.ts` so local dashboard submits use the same session API
  route outside production builds. For signature-backed submits, preserve
  `args: {}` when the user leaves every field empty; collapsing that payload to
  omitted args changes backend behavior by re-entering the legacy compatibility
  path instead of the explicit structured-invocation path. When a successful
  invocation triggers a same-session dashboard refresh, invalidate the
  current-factory query instead of removing it so the signature-backed widget
  can preserve visible success state while still refetching the current factory
  contract, and skip resuming the event stream from a persisted reconnect cursor
  on that same-session refresh via `shouldResumeFromPersistedCheckpoint` in
  `ui/src/features/dashboard/lib/dashboard-session-lifecycle.ts` plus
  `useDashboardInitialReconnectCursor`.
- `pkg/factory/sessions/invocation/session_wait.go` owns the session invocation wait loop and
  calls explicit packaged-factory hooks at active, completed, and terminal-failure
  boundaries. `pkg/service/runtime_sessions.go` and
  `pkg/runtimehost/session_invocation.go` adapt packaged TTS classification,
  logs, and metrics to those hooks.
- `pkg/factory/subsystems/subsystem_transitioner.go` applies packaged TTS
  invocation metadata to terminal token `Content` for the `execute-tts` TTS
  MODEL_INVOKE workstation so primary-result selection returns JSON metadata
  instead of submitted input text or raw audio payload bytes.
- `docs/architecture/invocation-contract.md` documents CLI/API equivalence and
  invocation-return policy ownership.
- Production provider mode selection lives at the `pkg/workers/provider`
  execution boundary: a configured Factory Session response-stream publisher
  selects a registered structured adapter, while final-only invocations retain
  the established provider behavior. Provider-native `responseevents.Draft`
  values must be published directly by `pkg/factory/sessions/stream` so the
  session store assigns event ID, sequence, recorded time, and Factory Session
  identity without flattening stable message or tool identity through the
  legacy fragment compatibility mapper.
- `docs/reference/run.md` is the customer-facing owner for packaged `@you/goal`
  invocation behavior. Operator-visible blocked, needs-human, paused,
  interrupted, failed, timed-out, and unresolved-primary-result outcomes plus
  inspect-first recovery belong in `docs/reference/sessions.md`; keep shared
  `FactorySession`/`Work` control vocabulary there so future goal lanes extend
  one operator flow instead of inventing route- or factory-specific recovery
  docs.
- `docs/reference/config.md` and `docs/reference/sessions.md` are the packaged
  `you docs` reference topics for invocation input sources, return policy, and
  the session-scoped invocation API.
- Dashboard current-factory decoding for signature-backed invocation widgets
  lives in `ui/src/api/factory-definition/api.ts` and
  `ui/src/api/current-factory-definition/api.test.ts`; keep exact
  `${parameter}` placeholders accepted on invocation-interpolated enum-backed
  authored fields when the current factory payload also declares that parameter
  in `invocationSignature`, or live session pages will fall back to legacy UI
  flows even when backend runtime validation already accepts the factory.
- Managed-runtime invocation readiness gating and direct invocation policy live in `pkg/models/service/invoke.go`; the canonical service consumes neutral `pkg/models/host.Host.InspectReadiness` snapshots, projects public readiness through `pkg/transports/mapping/managed_runtime_invocation.go`, and owns invocation failure classification and readiness logs. `pkg/wire/production.go` supplies the active-runtime reader, process model host, assets, logger, clock, metrics, invocation executor builder, and runner identity directly; `FactoryService` and `runtimehost.Host` only retain compatibility forwarding/composition seams and are never passed into the model family. Factory worker execution routes through `pkg/models/host/execution.go` (`LeaseExecution.WrapRunner`) when a process-wide host is configured, otherwise `pkg/models/local/runtime.go` manager fallback. Supervised leases pass `lease.Endpoint` into `localmodels.LoadRequest.ServingEndpoint` for host-owned HTTP execution. Process-wide local-runtime ownership and lease boundaries belong in `pkg/models/host`; keep `pkg/models/local` as the managed-runtime catalog compatibility projection layer. Model host operator diagnostics for load/lease/unload/crash paths live in `pkg/models/host/diagnostics.go`; managed-runtime pull logs and metrics live only in `pkg/models/service/pull.go`. See `docs/architecture/model-host.md`. Focused modelhost lease coverage for INFERENCE_WORKER/INFERENCE_RUN lives in `pkg/service/inference_modelhost_test.go`.
