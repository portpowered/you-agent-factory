# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- `pkg/invocations/` contains the shared invocation contract logic used by CLI
  and API adapters plus the canonical Factory Session invocation owner.
- `pkg/invocations/session_owner.go` owns live-session request normalization,
  interpolation validation, default-handling Work submission, lifecycle
  sequencing, and delegation into the owner-local event-derived result waiter.
  `pkg/invocations/session_wait.go` owns polling, timeout and cancellation,
  primary-result selection, and terminal classification over narrow runtime
  observations. `pkg/invocations/session_telemetry.go` owns invocation metric
  names, low-cardinality labels, exactly-once emission points, safe structured
  log fields, and packaged-factory telemetry policy. Keep session configuration,
  Work submission, observation, wait/time behavior, telemetry sinks, and
  packaged-factory classification as explicit collaborators; service and
  runtime-host facades should only adapt those dependencies and forward
  `InvokeFactorySession` unchanged.
- `pkg/invocations/arguments.go` owns signature-backed invocation argument
  normalization for positional, named, stdin, defaulted, repeated, variadic,
  alias-backed, and compatibility fallback inputs. Transport stories should
  adapt CLI or API payloads into `NormalizeArgumentsInput` rather than
  re-implementing binding, default, or validation rules at the boundary.
- `pkg/invocations/interpolation.go` owns runtime `${parameter}` interpolation
  for signature-backed worker and workstation fields plus pre-dispatch
  interpolation validation. Keep file-contents substitution, omitted-exact-field
  behavior, and interpolation error codes there instead of duplicating
  string-replacement rules in service or worker executors. The same package also
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
- `pkg/invocations/primary_result.go` resolves invocation `primaryResult`
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
  `WorkContentPart` and request-validation error shapes used below transport
  and service boundaries; `pkg/invocations/session_owner.go` owns the shared
  `FactoryInvocationResult` returned by the canonical owner.
- `pkg/workcontent/` translates between generated OpenAPI `WorkContent` and the
  backend-owned `interfaces.WorkContentPart` shape.
- `pkg/api/handlers_work_write.go` includes the session invocation HTTP
  boundary alongside other session work-write handlers, including projection of
  shared invocation non-success context into the public `InvocationResponse`.
- `pkg/service/runtime_sessions.go` and
  `pkg/runtimehost/session_invocation.go` retain session-runtime compatibility adapters for
  session config, canonical Work submission, event-derived observations,
  metric/log sinks, and packaged-factory terminal classification. Their
  `InvokeFactorySession` methods must remain transparent forwards to
  `invocations.SessionInvoker`; model-catalog files must not own Factory
  Session invocation behavior. Metric names, label policy, log shaping, and
  emission sequencing must not be reimplemented in these adapters; request
  normalization, interpolation validation, submission sequencing, polling,
  timeout/cancellation, primary-result selection, and general terminal
  classification belong only to `pkg/invocations`.
- API structured args use the direct structured-argument carrier rather than
  being reinterpreted as CLI named flags, so canonical parameter-name keys
  still work for positional-only or stdin-bound parameters. Treat `args: {}` as
  an explicit structured invocation request, not as omitted args, so
  all-optional or defaulted signatures stay transport-equivalent with CLI.
- `pkg/cli/run/` is the `you run --factory` CLI boundary.
- Canonical default-path ownership for operator config
  (`~/.you-agent-factory/config.json`) and generated live replay recording roots
  (`~/.you-agent-factory/recordings/...`) belongs in `pkg/config/defaultpaths`;
  `pkg/config/operatorconfig` and `pkg/cli/run` should keep only precedence,
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
  `pkg/cli/configinit` (`Init`, `InitConfig`) with command wiring in
  `pkg/cli/root.go` (`newSystemConfigCommand`, `newSystemConfigInitCommand`).
  Fresh homes create `~/.you-agent-factory/config.json` through
  `pkg/config/systemconfig.EnsureLocalBackendScope`; existing config files are
  validated with `operatorconfig.LoadFileConfig` and left byte-identical on
  re-run. Packaged defaults materialize through
  `factoryconfig.EnsureBuiltInNamedFactories`, which skips existing factory
  directories without rewriting user-edited files and can still create missing
  catalog entries on later runs. Isolated-home rerun coverage lives in
  `pkg/config/configinit/init_test.go` (`TestInit_DoubleRunIsSuccessfulNoOp`,
  `TestInit_PreservesUserEditedFactoryFilesOnRerun`,
  `TestInit_CreatesMissingPackagedDefaultsWithoutTouchingExisting`) and
  `pkg/cli/configinit/init_test.go` / `pkg/cli/root_config_init_test.go`. Keep
  `you factory config` factory.json tooling separate from this top-level
  operator/system initializer. Post-install bootstrap is invoked from
  `scripts/install.sh` and `scripts/install.ps1` via the installed binary's
  `config init` subcommand; installer smoke coverage lives in
  `tests/release/install_script_test.go` and `scripts/release/smoke-install.sh`
  / `scripts/release/smoke-install.ps1`.
- Operator default worker model settings resolve at the CLI/process boundary in
  `pkg/cli/root.go` (`resolveOperatorDefaults`) and flow through
  `run.RunConfig.OperatorDefaults` into `service.FactoryServiceConfig` before
  `cmd/factory/compose.InjectCLITransport`; Wire providers must not read
  `~/.you-agent-factory/config.json` or `YOU_DEFAULT_WORKER_MODEL_*` directly.
- Initializer-backed CLI local in-process startup belongs in `pkg/initializer/cli_transport.go` (`InitializeCLITransport`, `CLITransport.Runner`, `CLITransport.Run`), `cmd/factory/compose/cli_transport.go` (`InjectCLITransport`, `InjectCLIRunner`), and `cmd/factory/main.go` (`buildCLIRunner` registered through `run.SetBuildFactoryService`). Pass `transport.Runner()` to `pkg/cli/run` rather than `compose.InjectFactoryService` when proving the initializer composition path; dashboard-suppressed non-invocation CLI runs (`--quiet`, work-file batch, clean-invocation batch) stay on `service.BuildFactoryService` through `InjectCLIRunner`, while dashboard-suppressed one-shot invocation uses `service.BuildInvocationBootstrap` / `service.NormalizeInvocationBootstrapConfig` from `pkg/service/factory_build.go` via `pkg/cli/run/factory_invocation_input.go` only. `InvocationBootstrap.InvokeFactorySession` and `InvocationBootstrap.CloseFactorySession` must stay transparent forwards to the wrapped `FactoryService`; `runFactoryInvocation` releases sessions through `releaseInvocationSession` after invocation instead of a CLI-local submit/wait loop. Compose regression coverage for quiet batch work-file preservation lives in `cmd/factory/compose/compose_test.go` (`TestInjectCLIRunner_DashboardSuppressedQuietBatchPreservesWorkFileAndBatchMode`). Focused smoke coverage lives in `pkg/initializer/cli_transport_test.go`, `pkg/service/invocation_bootstrap_test.go`, `pkg/service/invocation_bootstrap_ownership_test.go`, and consolidated startup parity plus cross-transport composition evidence in `pkg/initializer/startup_compatibility_test.go`.   Focused initializer migration verification: `go test ./cmd/... ./pkg/api/... ./pkg/cli/... ./pkg/mcp/... ./pkg/initializer/... -short`.
- `you models invoke` reuses the same `service.BuildInvocationBootstrap` /
  `service.NormalizeInvocationBootstrapConfig` path as one-shot factory
  invocation, wired from `pkg/cli/models/bootstrap_invoke.go`. The CLI must call
  `FactoryService.InvokeModel` through the bootstrap-owned service rather than
  posting to `/models/{model_name}/invocations`; keep managed readiness/lifecycle
  error mapping aligned with `pkg/api/handlers_models.go` via
  `mapBootstrapModelInvokeError`. Return classified `InferenceFailure` and
  `ManagedRuntimeInvocationError` values without re-wrapping so readiness causes
  stay `errors.Is`-able;   non-ready bootstrap invoke coverage lives in
  `pkg/cli/models/non_ready_invoke_test.go` (stub bootstrap lifecycle vocabulary) and
  `pkg/cli/models/bootstrap_lifecycle_invoke_test.go` (offline MISSING/LOADING/FAILED
  integration through the real bootstrap). Ready offline invoke coverage lives in
  `pkg/cli/models/offline_ready_invoke_test.go`; bootstrap routing and failure-baseline
  contracts live in `pkg/cli/models/bootstrap_invoke_test.go` and
  `pkg/cli/models/failure_baseline_no_server_test.go`. Factory root resolution for invoke belongs in
  `pkg/cli/models` (`resolveModelsInvokeFactoryDir`), with operator defaults and
  logger passed from `pkg/cli/root.go` `newModelsInvokeCommand`.
- `pkg/cli/run/run.go` resolves positional versus non-TTY stdin through the
  shared `pkg/invocations` contract, then runs the local service in
  invocation-only service mode so stdout stays reserved for primary-result
  output instead of startup or dashboard noise; CLI-only source conflicts are
  logged and counted there before the service runtime exists. `RunConfig.JSONOutput`
  must stay aligned with the shared `InvocationResponse` envelope for both
  successful and non-success invocation results rather than becoming a
  success-only CLI fork. `RunConfig.InvocationOutputMode` and `you run --output`
  select primary-result-only versus internal `SessionResponseStream` attachment
  for supported one-shot factory invocations; keep mode validation, unsupported
  run-shape rejection, and fallback behavior in `pkg/cli/run/invocation_error.go`,
  stream attachment and bounded async progress stdout draining in
  `pkg/cli/run/invocation_observability.go`, human and JSON progress rendering in
  `pkg/cli/run/run_clean_invocation.go`, response-stream unit tests in
  `pkg/cli/run/run_config_test.go`, response-stream CLI integration tests in
  `pkg/cli/run/run_wire_api_test.go`, and invocation wiring in
  `pkg/cli/run/factory_invocation_input.go`. `pkg/cli/root_work.go` and
  `pkg/cli/root_run_test.go` apply manually parsed `you run --output response-stream`
  to `RunConfig.InvocationOutputMode` after `DisableFlagParsing` argument parsing.
  The `pkg/cli/run` package is at the
  15-file limit; extend existing files instead of adding new ones. Human response-stream
  terminal outcomes use `--- invocation outcome ---` with structured status/error
  fields; JSON response-stream terminal outcomes stay on the final
  `primary_result` NDJSON record. Human-only suppression helpers:
  `humanProgressRenderableEvent`, `humanInternalProgressPayload`, and
  `humanTokenUsageProgressEvent` in `pkg/cli/run/run_clean_invocation.go` drop
  compaction/backlog/stream-gap text and token-usage chatter while JSON mode
  keeps `compaction` / `stream_gap` records. Internal stream listing for
  `pkg/service/runtime_sessions.go` alongside `SubscribeSessionResponseStream`.
  Provider-neutral `FactoryResponseEvent` vocabulary lives in
  `pkg/factorysessions/responseevents` (distinct from internal
  `pkg/factorysessions/responsestream` fragment kinds). Session-scoped immutable
  response-event storage lives in `pkg/factorysessions/responseeventstore` with
  `factorysessions.SessionResponseEventStore` aliases in `types.go`; it is
  session-runtime-local state separate from canonical `FactoryEvent` history.
  `SessionResponseEventStore.Subscribe(afterSequence)` delivers retained events
  after the cursor, then continues live via `Subscription.Next`; optional
  `WithDispatchFilter(dispatchID)` omits non-matching events while preserving
  each delivered event's global session sequence and eventId. `Complete()` stops
  further publishes while retained events remain for catch-up; `Close()` rejects
  new subscriptions and publishes and detaches active subscribers. Mirror
  `responsestream.Subscription` patterns when extending close/complete behavior.
  Package docs in
  `responseevents/doc.go` record resolved v1 transport, retention, and CLI JSON
  decisions without implementing transports; `responseevents/boundary_test.go`
  enforces isolation from CLI, HTTP, subprocess, and provider imports.
  `responsestream.StreamSet.CloseDispatch` retains completed dispatch streams so
  late CLI pollers can still subscribe and drain retained progress until the
  completed-dispatch retention window expires; `runResponseStreamAttachment` in
  `pkg/cli/run/invocation_observability.go` performs one final dispatch-ID
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
  bytes into `invocations.ResolveTextInput` and surface `INVOCATION_INPUT_EMPTY`
  from the shared resolver instead of pre-trimming or short-circuiting with
  transport-specific empty-stdin errors. When `Stdin` is overridden away from
  `os.Stdin` (cobra `SetIn`, tests, or programmatic callers), treat it as piped
  input even if the process-level `os.Stdin` is still a TTY.
- `pkg/invocations/inference.go` resolves inference-run operation bindings,
  maps direct invocation request bindings, builds the provider-neutral inference
  request envelope, and shapes inference responses into ordered canonical
  `WorkContentPart` output shared by direct model invocation and factory-session
  execution paths.
- `pkg/apisurface/inference_failure.go` classifies inference readiness and
  execution failures into actionable customer-facing outcomes for missing model,
  loading model, unsupported operation, timeout, and runtime failure cases
  shared by direct model invocation and HTTP handlers.
- `pkg/workers/executor/model_operation_bindings.go` delegates inference binding
  resolution to `pkg/invocations`.
- `pkg/cli/root_run_args.go` owns the `you run` manual flag split that preserves
  known run and inherited flags while leaving unknown `--factory-arg` tokens
  intact for signature-backed parsing; keep factory-argument normalization
  itself in `pkg/cli/run/factory_invocation_signature_input.go` plus
  `pkg/invocations/arguments.go` rather than re-implementing binding logic in
  Cobra parsing.
- `pkg/invocations/input.go` owns logical empty-text detection via
  `strings.TrimSpace` inside `ResolveTextInput` and `ResolveAPITextInputContent`;
  CLI and API adapters must not duplicate whitespace-only rejection.
- `pkg/cli/root.go` owns the customer-facing `you run --factory` help text for
  invocation input-source rules and the canonical pointers into packaged docs.
  `runInvocationModes` and `resolveRunFactoryPrompt` also treat `you run --named`
  as an invocation factory selector for positional/stdin text.
  `runFactory` resolves `--named` / `--factory` / `--dir` conflicts and portable
  `--factory` preflight before loading operator defaults so flag and path failures
  stay independent of `~/.you-agent-factory/config.json` contents.
- `pkg/cli/root_run_test.go` isolates `HOME` for the whole CLI package so `make test`
  does not depend on the developer's real operator config file.
- `internal/releasesmoke/harness.go` isolates spawned `you run` smoke processes from
  the developer's real `HOME` so `tests/release` stays hermetic through
  `make test`.
-   `pkg/config/layout.go` owns the built-in `@you/goal` and `@you/tts` factory JSON
  (`BuiltInGoalFactoryJSON`, `BuiltInTTSFactoryJSON`) registered from
  `builtInNamedFactoryCatalog` in `pkg/config/layout.go`. Packaged `@you/goal`
  routes review mode from `check-goal` (`plain` -> `goal:review`, `structured` ->
  `goal:structured-review`) so plain classifier and structured envelope lanes are
  both reachable without competing logical advances from `goal:check`. The built-in
  `goal-checker` script worker must emit only the lane label on stdout after
  verification (`plain` by default, opt-in `structured` via
  `YOU_GOAL_REVIEW_MODE`) because `check-goal` is a `CLASSIFIER_WORKSTATION`.
  Retry exhaustion is authored separately for `review-goal` and
  `structured-review-goal`, each with its own guarded loop-breaker from `goal:plan`
  to `goal:failed`.
  Packaged workstation `body` templates must use canonical `PromptData` roots
  such as `(index .Inputs 0).Payload`; legacy top-level aliases like
  `{{ .WorkID }}` fail prompt rendering before mock-worker dispatch.
  `upgradeMaterializedBuiltInNamedFactoryIfNeeded` repairs already-materialized
  built-ins that still carry the legacy alias when the catalog payload has
  canonical templates, and those repairs must patch the specific legacy prompt
  files in place rather than replacing the whole materialized named-factory
  directory so customer edits survive later `you run --named` reuse.
  `@you/fusion` factory JSON (`BuiltInFusionFactoryJSON`) is also registered from
  `builtInNamedFactoryCatalog`.
- `pkg/config/builtinsubagent/` owns the authored `@you/subagent` one-pass factory
  scaffold (`factory.json`, prompt files) assembled into `BuiltInSubagentFactoryJSON`
  exported from `pkg/config/layout.go`. The topology uses exactly one `AGENT_WORKER`
  with explicit `agentTools.policy` and one `AGENT_RUN` workstation that interpolates
  `${input}` from the invocation signature into the workstation prompt body.
  `@you/subagent` is registered in `builtInNamedFactoryCatalog` so first named
  resolution materializes the split-layout factory under the global named-factory root.
- `pkg/packagedfactories/subagent/` owns packaged subagent factory metadata constants,
  topology validation coverage, materialization/edit-safe identity tests, response
  shaping helpers for terminal `task:complete` work content, and primary-result
  selection tests for the one-pass built-in factory JSON.
- Hermetic no-server named `@you/subagent` package proof lives in
  `pkg/cli/run/run_invocation_test.go`
  (`TestRun_NamedSubagentHermeticInvocationSucceedsWithoutListeningServer`,
  `TestRun_NamedSubagentNoServerBootstrap_TextPrimaryResultIsAgentResponse`,
  `TestRun_NamedSubagentNoServerBootstrap_SuccessJSONMatchesAPIProjection`,
  `TestNoServerNamedSubagentInvocationIntegrationAndEquivalenceProof`), using the
  real shared bootstrap path with mock workers and a TCP probe port to assert no
  factory API/dashboard listener is bound and exactly one agent-response
  `primaryResult` is returned.
- `pkg/cli/run/factory_invocation_help.go` owns the factory-aware help renderer
  for `you run --named <factory> --help` and `you run --factory <factory.json> --help`.
  Keep usage lines, parameter descriptions, defaults, accepted values, output
  hints, and example rendering derived from `interfaces.InvocationSignatureConfig`
  instead of hard-coding packaged-factory argument inventories in CLI help.
- `docs/reference/packaged-fusion.md` is the packaged `you docs packaged-fusion`
  customer guide for `@you/fusion` invocation, signature-aware help, examples,
  materialization, and edit-after-materialize behavior.
- `pkg/packagedfactories/goal/` owns packaged goal factory metadata constants and
  config-load regression coverage for the authored `invocationReturn` policy that
  selects terminal `goal:complete` work content as the primary result.
  `summary.go` shapes terminal `execute-goal` work content from worker output so
  EXPLICIT primary-result selection returns the final summary instead of
  submitted goal input text; classifier `review-goal` output is a route label
  and must preserve carried summary content. `primary_result_test.go` covers both
  successful EXPLICIT selection and unresolved failure when `goal:complete` is
  absent from terminal work in scope.
- `pkg/packagedfactories/goal/decision_envelope.go` owns the canonical
  reviewer/checker JSON envelope and its mapping onto `interfaces.WorkResult`.
  Goal routing envelopes with authored `classificationRoutes` map parsed
  `decision` labels onto `SelectedClassificationLabel` while preserving
  `Feedback`, optional `Output`, and `RecordedOutputWork`.
- `pkg/workers/executor/agent.go` routes `review` workstation agent output through
  `goal.WorkResultFromDecisionEnvelopeJSONOrFailed` instead of stop-token parsing.
  Workstations with `outcomeFormat: decision-envelope` and authored
  `classificationRoutes` use `goal.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed`.
- `factory/docs/decision-envelope.md` is the packaged-authoring guide for the
  reviewer/checker envelope shape, the standard outcome vocabulary, the
  packaged-goal goal-routing decision vocabulary used when
  `classificationRoutes` are present, and malformed-input behavior used by
  `factory/workstations/review/AGENTS.md`.
- `pkg/factory/subsystems/subsystem_transitioner.go` applies packaged goal
  invocation summary shaping on `execute-goal` workstations alongside packaged
  TTS metadata shaping. `pkg/factory/subsystems/goalroutingtests/transitioner_goal_routing_test.go`
  proves each authored `review-goal` classifier label routes to the expected goal place
  through the mapped runtime net and proves structured `structured-review-goal`
  envelopes route from parsed decision labels while preserving mapped
  `WorkResult` fields. The same file also proves malformed JSON and unknown
  decisions route to `goal:failed` with actionable failure text instead of
  misrouting to complete, rework, or escalation states.
- `pkg/packagedfactories/goal/factory_test.go` proves `goal:execute` schedules
  the `check-goal` review-mode classifier in the mapped runtime net.
- `tests/functional/runtime_api/api_packaged_goal_invocation_test.go` proves the
  materialized built-in goal topology dispatches `review-goal` when
  `check-goal` returns `plain` and `structured-review-goal` when `check-goal`
  returns `structured`, using the real authored `goal-checker` contract rather
  than mocked lane-label output. The same file proves repeated structured
  `needs_changes` rework trips the structured loop-breaker instead of retrying
  forever.
- Behavioral proof for named goal batch invocation lives in
  `tests/functional/smoke/cli_named_goal_run_smoke_test.go` using the real
  `you run --named @you/goal` CLI path with `--with-mock-workers`, including a
  fresh-home materialization smoke case, a customer-edit preservation rerun
  smoke case, and a legacy-materialized upgrade smoke case.
- Hermetic no-server named `@you/goal` package proof lives in
  `pkg/cli/run/run_invocation_test.go`
  (`TestRun_NamedGoalHermeticInvocationSucceedsWithoutListeningServer`), using
  the real shared bootstrap path with mock workers and a TCP probe port to
  assert no factory API/dashboard listener is bound.
- No-server bootstrap CLI/API invocation-equivalence proof lives in
  `pkg/cli/run/run_invocation_test.go`
  (`TestRun_NoServerBootstrap_PositionalInputMatchesAPIContract`,
  `TestRun_NoServerBootstrap_StdinInputMatchesAPIContract`,
  `TestRun_NoServerBootstrap_SuccessJSONMatchesAPIProjection`,
  `TestRun_NoServerBootstrap_TextPrimaryResultFollowsInvocationReturn`), capturing real
  `defaultBuildInvocationBootstrap` invoke requests/results and comparing them
  to the shared API text-input resolver plus `apisurface.InvocationResponseFromResult`
  projection for packaged `@you/goal` primary-result selection.
- Consolidated no-server named integration and invocation-equivalence proof for
  reviewers lives in `pkg/cli/run/run_invocation_test.go`
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
  `primaryResult`, JSON response-stream NDJSON ends with a `primary_result`
  record, durable `FactoryEvent` history omits internal response-stream terms,
  and generated public API artifacts stay internal-only. Reuse
  `writePackagedGoalBuiltinTopologyMockWorkers`, `materializeNamedGoalFactoryForRoutingSmoke`,
  and `support.StartFunctionalAPIServer` when extending boundary verification.
- `tests/functional/smoke/cli_run_mode_compat_smoke_test.go` holds focused
  regression coverage for adjacent `you run` modes after packaged-goal changes:
  operator-oriented continuous startup output without `--quiet`, factory text
  invocation stdout that suppresses operator chatter, and named-goal batch
  stdout that stays primary-result-only. Reuse helpers from
  `cli_factory_prompt_run_smoke_test.go` when extending these regressions.
- `pkg/packagedfactories/tts/` owns packaged TTS invocation metadata shaping
  helpers used when `INFERENCE_RUN` (or legacy `MODEL_INVOKE`) work completes on the `execute-tts` workstation.
  `metadata.go` derives the `backend` metadata field from the loaded on-disk
  worker model so customer edits to materialized `factory.json` affect the next
  invocation result.
- `docs/reference/packaged-goal.md` is the packaged `you docs packaged-goal`
  customer guide for `@you/goal` batch invocation, stdout primary result, operator
  controls during active execution, internal-only CLI response-stream scope, and
  the supported headless operator-interaction scope without widening localhost
  listener promises. Packaged docs proof lives in
  `pkg/cli/docs/docs_packaged_reference_test.go` and
  `tests/functional/smoke/cli_docs_smoke_test.go`; maintainer final-verification
  evidence for public-contract boundaries lives in
  `docs/internal/development/plans/you-goal/api-contract-audit.md`.
- `docs/reference/models.md` is the customer guide for `INFERENCE_RUN`,
  `INFERENCE_WORKER`, managed-runtime `/models` surfaces, local modelhost lease
  execution, and legacy `MODEL_INVOKE` / `MODEL_WORKER` migration aliases.
- `docs/reference/packaged-tts.md` is the packaged `you docs packaged-tts`
  customer guide for `@you/tts` invocation, materialization, metadata result,
  edit-after-materialize behavior, and raw-artifact streaming scope. Prefer
  `INFERENCE_WORKER` / `INFERENCE_RUN` terminology there while documenting
  `MODEL_WORKER` / `MODEL_INVOKE` as migration aliases.
- `pkg/packagedfactories/tts/observability.go` classifies packaged TTS loading,
  model-not-ready, and generation-failure outcomes and defines stable invocation
  error codes plus packaged-factory metric names.
- `pkg/cli/run/packaged_tts_invocation.go` logs named-factory resolution context at
  the CLI boundary without recording packaged-factory metrics or logging submitted
  text or generated artifact bodies.
- `pkg/packagedfactories/goal/` owns packaged `@you/goal` factory metadata
  constants (`PackagedFactoryName`, `PackagedInvokeWorkstationName`).
- `pkg/cli/run/run_invocation_test.go` proves `@you/goal` CLI invocation input
  sources resolve through `invocations.ResolveTextInput`, reach the shared
  `InvocationRequest` payload shape, fail with stable
  `INVOCATION_INPUT_SOURCE_CONFLICT` before `InvokeFactorySession`, and match the
  session invocation API contract for the same logical text input and JSON success
  envelopes.
- `pkg/cli/root_run_server_test.go` proves root `you run --named @you/goal` and
  `@you/tts` wiring for positional text, piped stdin, explicit `-` stdin forms,
  and stable `INVOCATION_INPUT_SOURCE_CONFLICT` rejection when sources combine.
- `pkg/api/server_factory_sessions_test.go` proves the session invocation API
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
- `pkg/invocations/session_wait.go` owns the session invocation wait loop and
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
- `docs/reference/packaged-goal.md` is the customer-facing reference for
  packaged `@you/goal` invocation behavior, including operator-visible blocked,
  needs-human, paused, interrupted, failed, timed-out, and unresolved-primary-result
  outcomes plus recovery through existing session/work commands. Keep the
  `@you/goal`-specific inspect-first recovery sequence there, but keep the
  shared `FactorySession`/`Work` control vocabulary and command ownership in
  `docs/reference/sessions.md` so future goal lanes extend one operator flow
  instead of inventing route- or factory-specific recovery docs.
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
- Managed-runtime invocation readiness gating and direct invocation policy live in `pkg/models/service/invoke.go`; the canonical service consumes neutral `pkg/modelhost.Host.InspectReadiness` snapshots, projects public readiness through `pkg/apisurface/managed_runtime_invocation.go`, and owns invocation failure classification and readiness logs. `FactoryService` and `runtimehost.Host` only compose or forward the model collaborator. Factory worker execution routes through `pkg/modelhost/execution.go` (`LeaseExecution.WrapRunner`) when a process-wide host is configured, otherwise `pkg/localmodels/runtime.go` manager fallback. Supervised leases pass `lease.Endpoint` into `localmodels.LoadRequest.ServingEndpoint` for host-owned HTTP execution. Process-wide local-runtime ownership and lease boundaries belong in `pkg/modelhost`; keep `pkg/localmodels` as the managed-runtime catalog compatibility projection layer. Model host operator diagnostics for load/lease/unload/crash paths live in `pkg/modelhost/diagnostics.go`; managed-runtime pull logs and metrics live only in `pkg/models/service/pull.go`. See `docs/architecture/model-host.md`. Focused modelhost lease coverage for INFERENCE_WORKER/INFERENCE_RUN lives in `pkg/service/inference_modelhost_test.go`.
