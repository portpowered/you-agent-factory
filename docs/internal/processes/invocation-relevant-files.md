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
- `pkg/cli/run/run.go` resolves positional versus non-TTY stdin through the
  shared `pkg/invocations` contract, then runs the local service in
  invocation-only service mode so stdout stays reserved for primary-result
  output instead of startup or dashboard noise; CLI-only source conflicts are
  logged and counted there before the service runtime exists.
- `pkg/cli/run/factory_invocation_input.go` must pass raw positional/stdin
  bytes into `invocations.ResolveTextInput` and surface `INVOCATION_INPUT_EMPTY`
  from the shared resolver instead of pre-trimming or short-circuiting with
  transport-specific empty-stdin errors.
- `pkg/invocations/input.go` owns logical empty-text detection via
  `strings.TrimSpace` inside `ResolveTextInput` and `ResolveAPITextInputContent`;
  CLI and API adapters must not duplicate whitespace-only rejection.
- `pkg/cli/root.go` owns the customer-facing `you run --factory` help text for
  invocation input-source rules and the canonical pointers into packaged docs.
  `runInvocationModes` and `resolveRunFactoryPrompt` also treat `you run --named`
  as an invocation factory selector for positional/stdin text.
- `pkg/config/builtin_tts_factory.go` owns the built-in `@you/tts` factory JSON
  registered from `builtInNamedFactoryCatalog` in `pkg/config/layout.go`.
- `pkg/packagedfactories/tts/` owns packaged TTS invocation metadata shaping
  helpers used when MODEL_INVOKE work completes on the `execute-tts` workstation.
- `pkg/factory/subsystems/subsystem_transitioner.go` applies packaged TTS
  invocation metadata to terminal token `Content` for the `execute-tts` TTS
  MODEL_INVOKE workstation so primary-result selection returns JSON metadata
  instead of submitted input text or raw audio payload bytes.
- `docs/architecture/invocation-contract.md` documents CLI/API equivalence and
  invocation-return policy ownership.
- `docs/reference/config.md` and `docs/reference/sessions.md` are the packaged
  `you docs` reference topics for invocation input sources, return policy, and
  the session-scoped invocation API.
