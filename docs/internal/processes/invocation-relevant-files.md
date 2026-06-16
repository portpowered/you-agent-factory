# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- `pkg/invocations/` contains shared pure invocation contract logic used by CLI
  and API adapters.
- `pkg/invocations/primary_result.go` resolves invocation `primaryResult`
  against selected-tick `FactoryWorldState` using `WorkRequestsByID`,
  `TerminalWorkByID`, and payload-lineage scope rather than transport-specific
  polling logic.
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
- `pkg/config/layout.go` owns the built-in `@you/tts` factory JSON (`BuiltInTTSFactoryJSON`)
  registered from `builtInNamedFactoryCatalog` in `pkg/config/layout.go`.
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
- Managed-runtime invocation readiness gating lives in `pkg/modelhost/managed_runtime_compat.go` (`EnsureInvocationReady`) and `pkg/apisurface/managed_runtime_invocation.go`; direct model invocation wires through `pkg/service/model_catalog.go` and factory worker execution through `pkg/localmodels/runtime.go`. Process-wide local-runtime ownership and lease boundaries belong in `pkg/modelhost`; keep `pkg/localmodels` as the managed-runtime catalog compatibility projection layer.
