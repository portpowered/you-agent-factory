# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- `pkg/invocations/` contains shared pure invocation contract logic used by CLI
  and API adapters.
- `pkg/invocations/primary_result.go` resolves invocation `primaryResult`
  against selected-tick `FactoryWorldState` using `WorkRequestsByID`,
  `TerminalWorkByID`, and payload-lineage scope rather than transport-specific
  polling logic. The same package also classifies missing-primary-result waits
  from scoped current work state when authored workflow states such as
  `blocked` or `needs-human` explain the stopped invocation better than the
  generic unresolved-primary-result fallback, classifies terminal failed work
  in invocation scope before that unresolved fallback, and classifies
  invocation control-state outcomes such as paused sessions or interrupted
  dispatches from reconstructed session and dispatch lifecycle facts.
- `pkg/factory/validation/validate.go` owns factory-level `invocationReturn`
  validation shared by validate-only and save pre-check flows.
- `pkg/config/factory_config_mapping*.go` maps `invocationReturn` between the
  OpenAPI factory contract and the internal runtime config.
- `pkg/interfaces/factory_runtime.go` owns the backend canonical
  `WorkContentPart` shape returned by invocation resolvers.
- `pkg/workcontent/` translates between generated OpenAPI `WorkContent` and the
  backend-owned `interfaces.WorkContentPart` shape.
- `pkg/api/handlers_work_write.go` includes the session invocation HTTP
  boundary alongside other session work-write handlers.
- `pkg/service/runtime_sessions.go` owns the session-scoped invocation
  orchestration that resolves API input, submits the default handling work
  item, polls selected-tick world state, and maps timeout/cancel/unresolved
  outcomes into `InvocationResponse`; it also owns invocation boundary logs and
  optional `InvocationMetricsRecorder` counter emission for runtime outcomes.
- `pkg/cli/run/` is the `you run --factory` CLI boundary.
- Canonical default-path ownership for operator config
  (`~/.you-agent-factory/config.json`) and generated live replay recording roots
  (`~/.you-agent-factory/recordings/...`) belongs in `pkg/config/defaultpaths`;
  `pkg/config/operatorconfig` and `pkg/cli/run` should keep only precedence,
  filename, and reporting behavior around those defaults.
- Operator default worker model settings resolve at the CLI/process boundary in
  `pkg/cli/root.go` (`resolveOperatorDefaults`) and flow through
  `run.RunConfig.OperatorDefaults` into `service.FactoryServiceConfig` before
  `cmd/factory/compose.InjectFactoryService`; Wire providers must not read
  `~/.you-agent-factory/config.json` or `YOU_DEFAULT_WORKER_MODEL_*` directly.
- `pkg/cli/run/run.go` resolves positional versus non-TTY stdin through the
  shared `pkg/invocations` contract, then runs the local service in
  invocation-only service mode so stdout stays reserved for primary-result
  output instead of startup or dashboard noise; CLI-only source conflicts are
  logged and counted there before the service runtime exists.
- `pkg/cli/run/factory_invocation_input.go` must pass raw positional/stdin
  bytes into `invocations.ResolveTextInput` and surface `INVOCATION_INPUT_EMPTY`
  from the shared resolver instead of pre-trimming or short-circuiting with
  transport-specific empty-stdin errors. When `Stdin` is overridden away from
  `os.Stdin` (cobra `SetIn`, tests, or programmatic callers), treat it as piped
  input even if the process-level `os.Stdin` is still a TTY.
- `pkg/invocations/input.go` owns logical empty-text detection via
  `strings.TrimSpace` inside `ResolveTextInput` and `ResolveAPITextInputContent`;
  CLI and API adapters must not duplicate whitespace-only rejection.
- `pkg/cli/root.go` owns the customer-facing `you run --factory` help text for
  invocation input-source rules and the canonical pointers into packaged docs.
  `runInvocationModes` and `resolveRunFactoryPrompt` also treat `you run --named`
  as an invocation factory selector for positional/stdin text.
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
- `pkg/packagedfactories/tts/` owns packaged TTS invocation metadata shaping
  helpers used when `INFERENCE_RUN` (or legacy `MODEL_INVOKE`) work completes on the `execute-tts` workstation.
  `metadata.go` derives the `backend` metadata field from the loaded on-disk
  worker model so customer edits to materialized `factory.json` affect the next
  invocation result.
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
- `pkg/service/model_catalog.go` owns the session invocation wait loop, packaged TTS
  loading/completion/failure logs, and packaged-factory metrics while polling for
  primary results.
- `pkg/factory/subsystems/subsystem_transitioner.go` applies packaged TTS
  invocation metadata to terminal token `Content` for the `execute-tts` TTS
  MODEL_INVOKE workstation so primary-result selection returns JSON metadata
  instead of submitted input text or raw audio payload bytes.
- `docs/architecture/invocation-contract.md` documents CLI/API equivalence and
  invocation-return policy ownership.
- `docs/reference/config.md` and `docs/reference/sessions.md` are the packaged
  `you docs` reference topics for invocation input sources, return policy, and
  the session-scoped invocation API.
- Managed-runtime invocation readiness gating lives in `pkg/modelhost/managed_runtime_compat.go` (`EnsureInvocationReady`) and `pkg/apisurface/managed_runtime_invocation.go`; direct model invocation wires through `pkg/service/model_catalog.go` and factory worker execution through `pkg/modelhost/execution.go` (`LeaseExecution.WrapRunner`) when a process-wide host is configured, otherwise `pkg/localmodels/runtime.go` manager fallback. `EnsureInvocationReady` consumes live host readiness via `InspectReadiness` so supervised loading and crash outcomes gate invocation. Supervised leases pass `lease.Endpoint` into `localmodels.LoadRequest.ServingEndpoint` for host-owned HTTP execution. Process-wide local-runtime ownership and lease boundaries belong in `pkg/modelhost`; keep `pkg/localmodels` as the managed-runtime catalog compatibility projection layer. Model host operator diagnostics for pull/load/lease/unload/crash paths live in `pkg/modelhost/diagnostics.go`; see `docs/architecture/model-host.md`.
