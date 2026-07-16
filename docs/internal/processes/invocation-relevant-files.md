# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

## CLI run and submit command contracts

- Canonical metadata for `you.run`, `you.submit`, and `you.submit.batch` lives
  in `contracts/cli/commands.json`. Keep positional cardinality, stdin channels,
  source precedence, conflicts, no-option defaults, output modes, effects, and
  stable handler/OpenAPI bindings aligned with the handwritten constructors in
  `pkg/transports/cli/root_work.go` and
  `pkg/transports/cli/root_submit_batch.go`.
- `pkg/transports/cli/climanifest/run_submit_validation.go` rejects incomplete
  or contradictory family records before generation. Update its focused
  validation cases whenever the supported family contract changes.
- String flags may define a non-empty `noOptionDefault` when presence without a
  value selects a stable sentinel, as `--with-mock-workers` does. Boolean
  no-option defaults remain restricted to `true` or `false` by
  `contracts/cli/command-manifest.schema.json`.
- Generated run/submit metadata is embedded from
  `pkg/transports/cli/generated/run_submit_family.json`, with stable IDs in
  `run_submit_command_ids_gen.go`. `climanifestcobra.NewRunSubmitFamilyComponents`
  constructs only detached `run` and `submit` roots plus the nested `submit batch`
  leaf; `commandregistry.NewRunSubmitRegistry` attaches retained `PreRunE` and
  `RunE` lifecycles by stable command ID. Production execution bindings are
  assembled by `newRunSubmitHandlerRegistry` in `root_work.go`.
  `productionRunSubmitCommands` selects the generated family by default while
  retaining the handwritten constructors behind the localized
  `useGeneratedRunSubmitFamily` rollback constant.
  `NewGeneratedRunSubmitFamilyCommandForParity` exposes the isolated generated
  tree for focused verification.
  `NewRunSubmitFamilyParityRoots` builds independent legacy and generated roots
  with injected `RootCommandOptions`; use it for observable parser, resolved
  `RunConfig`, service-call, stdout/stderr, and error parity without sharing
  mutable Cobra flag state. Inject unary submit through `RootCommandOptions.SubmitWork`
  and call the real submit transport against an `httptest` server when parity
  must prove request path/body, default or explicit session selection, and
  human/JSON output. Manifest-required submit inputs remain validated by the
  retained handwritten handler so generated construction does not preempt its
  stable diagnostics or validation ordering. Batch positional cardinality is
  likewise retained in `submit.resolveBatchInput`; inject the real batch
  transport through `RootCommandOptions.SubmitBatch` so parity roots receive
  independent Cobra stdin/stdout/stderr streams while proving `--file`,
  positional file, inline JSON, explicit or implicit stdin, dry-run, session
  routing, request, output, diagnostic, and failure behavior. When both parity
  roots share a migrated execution helper, also assert fixed pre-migration
  outcomes (including nil optional sinks and exact stdout/stderr) so changing
  that helper cannot redefine the legacy oracle. Run-specific
  coverage lives in `pkg/transports/cli/climanifestparity/runparity/run_parity_test.go`;
  unary and batch submit coverage lives in the sibling `submitparity` and
  `submitbatchparity` packages.

## CLI invocation output modes (primary-result, human response-stream, NDJSON)

Use this lane when changing `you run` stdout modes, `--output response-stream`,
root `--json` NDJSON records, or packaged `you docs run` output-mode guidance.
Supported one-shot factory invocations expose three modes; continuous, replay,
`--work`, and other non-invocation run shapes do not offer response-stream output.

| Mode | Selection | Stdout contract |
|---|---|---|
| Primary-result (default) | `you run --factory …` or `you run --named …` without `--output response-stream` | Successful invocations write only the configured `primaryResult` to stdout |
| Human response-stream | `you run --factory … --output response-stream` (no root `--json`) | Bounded human progress from canonical `FactoryResponseEvent` records, then `--- invocation outcome ---` with structured status/error fields and the primary result |
| NDJSON automation | `you --json run --factory … --output response-stream` | Each non-empty stdout line is one JSON record: `recordType=response_event` with nested public `FactoryResponseEvent`, ending with exactly one terminal `recordType=invocation_result` |

**CLI boundary ownership**

- Mode flag wiring and unsupported run-shape rejection:
  `pkg/transports/cli/root_work.go`, `pkg/transports/cli/root_run_test.go`
  (manual `you run --output response-stream` parsing after `DisableFlagParsing`)
- `RunConfig.InvocationOutputMode`, validation, and error mapping:
  `pkg/transports/cli/run/invocation_error.go`
- Session-owned canonical subscription, human progress draining, and lossless JSON
  stdout ordering:
  `pkg/transports/cli/run/invocation_observability.go`,
  `pkg/transports/cli/run/run_clean_invocation.go`,
  `pkg/transports/cli/run/factory_invocation_input.go`
- Shared bootstrap forward and post-invocation retained-window drain:
  `service.InvocationBootstrap.SubscribeSessionResponseEventsFromLatest` via
  `pkg/transports/cli/run/factory_invocation_input.go`

**Shared observation contract**

- Provider-neutral public event vocabulary:
  `pkg/factory/sessions/responseevents/`
- Session-scoped ephemeral store (CLI and API share the same records):
  `pkg/factory/sessions/responseeventstore/`
- Retained-window `STREAM_GAP` visibility applies to both human and NDJSON modes;
  do not fall back to legacy provider-progress payloads when the canonical
  subscription is unavailable

**Packaged operator guidance**

- `docs/reference/run.md` (invocation output modes and copyable examples);
  cross-link `you docs config` for return/output policy
- Provider fidelity variability:
  `docs/reference/workers.md` (`## Response-stream provider fidelity`)
- Session SSE counterpart:
  `docs/reference/sessions.md` (`## Response-event stream lifecycle and reconnect`)
- Run `make docs-reference-smoke` after `docs/reference/` edits

**Focused CLI and docs verification**

- Documented commands reach the current CLI output-mode boundary:
  `pkg/transports/cli/root_docs_test.go`
  (`TestRunDocumentation_InvocationOutputModeExamplesReachCurrentCLIBoundary`)
- Mode unit coverage:
  `pkg/transports/cli/run/run_config_test.go`,
  `pkg/transports/cli/run/run_response_stream_renderer_test.go`
- Integration coverage (human and NDJSON ordering, slow stdout, terminal
  `invocation_result`):
  `pkg/transports/cli/run/run_wire_api_test.go`
- Packaged topic smoke markers:
  `tests/functional/smoke/cli_docs_smoke_test.go` (`run` topic)
- End-to-end response-stream boundary with real CLI and API:
  `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go`

**Maintainer verification commands**

- Packaged run-topic edits: `make docs-reference-smoke`
- Focused CLI lane:
  `go test ./pkg/transports/cli/run/... ./pkg/transports/cli/... -run 'ResponseStream|InvocationOutput' -count=1`

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
- JavaScript named-factory lookup carries the authored `argsSchema` and `defaultPolicy` through `pkg/orchestrators/javascript/source/` into `pkg/factory/sessions/execution/PrepareStart`. Validate resolved arguments before runtime execution and resolve policy with that default; `workflowruntime.Request.ArgsSchema` preserves the same no-side-effect guard for direct runtime callers.
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
- JavaScript packaged factories keep authored workflow files in the package
  definition's `scripts/` assets and assemble them through
  `pkg/factory/packages/packageassets`. Their `sourceRef` must use the
  corresponding materialized `scripts/...` path, which `you config init`
  installs as editable factory files.
- Package-owned execution selections belong in the invocation signature and
  JavaScript args schema. Mirror their defaults in the workflow and constrain
  selectable model and reasoning values with the package `defaultPolicy`
  allowlists before specialist dispatch can begin.
- Packaged JavaScript workflows can use `parallel([...])` with literal
  `agent.run` specifications for bounded specialist dispatches, followed by a
  lead `agent.run` whose computed prompt consumes their completed outputs.
  Computed supported agent fields are runtime-normalized and policy-validated
  before dispatch; literal non-string fields remain a source-validation error.
  Keep the factory `defaultPolicy.maxAgents` and `concurrency` explicit (the
  lead consumes one total dispatch slot in addition to any specialists), and
  prove both a no-delegation completion and specialist-informed lead synthesis
  through the materialized workflow runtime.
- Canonical CLI metadata belongs in `contracts/cli/commands.json`. Separately
  approved compatibility-only command metadata belongs in
  `contracts/cli/deprecated-commands.json`, while its classification, successor,
  approval, evidence, and removal gates remain in `contracts/cli/deprecated.json`.
  Mark generation-ready records with `completeness: authoritative`; the CLI schema
  then requires complete channels, outputs, exits, effects, runtime constraints,
  and stable handler metadata for runnable records. Register every authored command
  manifest in `internal/contractvalidator.CLIRegistry`, and keep relationship
  participants on same-command flag or argument IDs so diagnostics name the exact
  record path. Compatibility records must not be copied into the primary manifest
  merely to make generation convenient. Apply family-completeness validators only
  to the manifest classification that owns that family: `LoadProduction` owns
  canonical run/submit validation, while `LoadCompatibility` must remain able to
  decode the separately classified workflow-only manifest.
- Classification-aware workflow/MCP generation lives in
  `pkg/transports/cli/climanifestgen`: canonical `you mcp` / `you mcp serve`
  metadata is emitted from `commands.json` into `mcp_family.json`, while approved
  `you workflow validate` / `you workflow preview` metadata is emitted separately
  from `deprecated-commands.json` into `workflow_compatibility_family.json`.
  Keep their generated stable-ID lists source-labeled and disjoint; `Check` must
  report the affected stable IDs for drift, and generation must reject either
  family when its IDs appear in the wrong classification source.
- Whole-production-tree CLI boundary validation lives in
  `pkg/transports/cli/clicontract`. `CheckProduction` joins the read-only
  `commandidentity.Walk` inventory with `contracts/cli/commands.json`, approved
  callable entries from `contracts/cli/deprecated.json`, the separately authored
  compatibility metadata in `contracts/cli/deprecated-commands.json`, and every
  embedded generated family manifest. Keep this check free of command execution,
  services, and network access; preserve full lifecycle and input/completion
  fields in `climanifest` so generated freshness comparisons cannot silently
  discard authored metadata before validation.
- Workflow/MCP handwritten handler binding lives in
  `pkg/transports/cli/commandregistry/workflowmcp`. Keep canonical MCP and
  workflow-compatibility registries separate, verify each against its own
  generated manifest, and report missing or classification-mismatched bindings
  by stable command ID. The execution adapters remain with their transport
  owners (`workflow.ValidateRunE`, `workflow.PreviewRunE`, and `mcp.ServeRunE`),
  while `newWorkflowMCPHandlerRegistries` supplies root dependencies without
  moving workflow resolution or MCP lifecycle logic into generated artifacts.
- Workflow/MCP production construction lives in
  `pkg/transports/cli/climanifestcobra/workflow_mcp_constructor.go` and
  `pkg/transports/cli/root_workflow.go`. Build canonical MCP metadata and the
  two approved workflow compatibility leaves from their separate generated
  manifests, bind both metadata and handlers to the same local flag state, then
  attach only the generated validate/preview leaves to the existing handwritten
  workflow parent. Keep workflow run/start/status/result/dispatch/artifact/event
  construction outside this family slice. Constructor parity in
  `pkg/transports/cli/climanifestparity` must compare help, parsing, completion,
  handler outcomes, stdout/stderr, and success/failure behavior before this
  production cutover is changed.
- MCP protocol and resume smokes in `pkg/transports/cli/mcp/serve_*_test.go`
  should enter through `cli.NewRootCommandWithOptions` and the injected startup
  boundary, then delegate to the existing `mcp.RunServe` implementation with the
  exact parsed stdio streams. This keeps fixture/runtime selection, JSON-RPC,
  EOF/cancellation, and durable resume assertions attached to the generated
  production `you mcp serve` construction instead of proving only its detached
  handwritten execution adapter.
- Production CLI command manifest parity for the root + `session show` family lives in
  `pkg/transports/cli/climanifest` (`LoadProduction`, `ProductionManifestPath`) and
  `pkg/transports/cli/climanifestparity` (`CompareDeclaredHandler`,
  `CompareHandlerOpenAPIBinding`, `OpenAPIOperationBinding`, `CompareLiveExitCodes`,
  `CompareBaselineSideEffects`, `CompareBaselineConstraints`, and
  `TestProductionCLIRootSessionFamily_RepresentativeCutover`). Approved execution metadata
  for side-effects/constraints is loaded from
  `contracts/testdata/baseline/cli-command-execution.json`. Handler/OpenAPI binding for
  `you.session.show` asserts `operationId` `getFactorySession` maps to
  `GET /factory-sessions/{session_id}` in `api/openapi.yaml` and matches live
  `session.Show` JSON transport.   Representative-family metadata generation lives in
  `pkg/transports/cli/climanifestgen` (`Generate`, `Check`, `ExtractRepresentativeFamily`,
  `ExtractWorkFamily`, `WorkArtifact`) with `cmd/climanifestgen` and committed artifacts under
  `pkg/transports/cli/generated` (`RepresentativeFamilyManifest`, embedded
  `representative_family.json`; `WorkFamilyManifest`, embedded `work_family.json`;
  `WorkFamilyCommandIDs` in `command_ids_gen.go`). Handwritten representative-family handlers are
  registered by stable command ID in `pkg/transports/cli/commandregistry`
  (`NewRepresentativeRegistry`, `SessionShowRunE`, `AttachRunE`,
  `VerifyRepresentativeRunnableCoverage`; work-family:
  `NewWorkRegistry`, `ListRunE`, `ShowRunE`, `MoveRunE`, `VisualizeRunE`,
  `VerifyWorkRunnableCoverage`) and production wiring helpers
  `newRepresentativeHandlerRegistry` and `newWorkHandlerRegistry` in
  `pkg/transports/cli/root_work.go`. The generated representative-family constructor
  lives in `pkg/transports/cli/climanifestcobra` (`NewRepresentativeFamilyCommand`,
  `NewRepresentativeFamilyComponents`, `NewRepresentativeFamilyCommandFromManifest`)
  and builds only `you` → `you session` → `you session show` from embedded generated
  metadata plus registry-attached handwritten handlers. The generated work-family
  constructor lives in the same package (`NewWorkFamilyCommand`,
  `NewWorkFamilyComponents`, `NewWorkFamilyCommandFromManifest`) and builds
  `you work` → `you work list|show|move|visualize` from embedded generated metadata
  plus registry-attached handwritten handlers; isolated legacy construction for parity
  uses `cli.NewLegacyWorkFamilyCommand` and shared-root helpers
  `NewLegacyWorkFamilyRootForParity`, `NewWorkFamilyParityRoots`, and
  `NewWorkFamilyParityRootsWithProductionHandlers`. Production root cutover is
  controlled by `useGeneratedRepresentativeFamily` in `pkg/transports/cli/root_work.go`
  (`newRootCommandWithGeneratedRepresentativeFamily` with
  `newLegacyRootCommandWithOptions` rollback). Generated-vs-legacy parity for the
  representative family lives in `pkg/transports/cli/climanifestparity`
  (`CompareConstructorIdentityParity`, `CompareConstructorHelpParity`,
  `CompareConstructorParseParity`, `CompareConstructorCompletionInventoryParity`,
  `TestGeneratedVsLegacyParityMatrix_RepresentativeFamily`) with isolated legacy
  construction via `cli.NewLegacyRepresentativeFamilyCommand`. Work-family parity
  mirrors the same package (`TestGeneratedVsLegacyParityMatrix_WorkFamily`,
  `TestProductionManifestParsingParity_WorkFamily`,
  `TestProductionManifestOutputModeParity_WorkFamily`) using the shared-root helpers
  above; `WorkFamilyBindings.FlagUsages` bridges handwritten local flag help text.
- Whole-production CLI closure is checked by `pkg/transports/cli/clicontract`
  and exposed through `cmd/clicontractsmoke` / `make cli-contract-smoke`.
  Keep deliberate smoke violations snapshot-only: they must use the production
  validator and diagnostics without executing commands, invoking services,
  mutating Cobra state, or requiring network access. The smoke target also runs
  every `climanifestparity` package so generated and legacy trees stay
  independently constructible with behavior parity.
- Models-family manifest parity for `you.models` and list/inspect/invoke/pull leaves lives in
  `pkg/transports/cli/climanifestparity` (`CompareModelsHelpIdentity`,
  `TestProductionManifestHelpIdentityParity_ModelsFamily`,
  `TestProductionManifestParsingParity_ModelsFamily`,
  `TestProductionManifestOutputModeParity_ModelsFamily`,
  `TestGeneratedVsLegacyParityMatrix_ModelsFamily`) with isolated legacy construction via
  `cli.NewLegacyModelsFamilyCommand` and generated parity roots via
  `cli.NewGeneratedModelsFamilyParityCommand`. Models output-mode tests stub delegates through
  `ListModelsAccessor` / `SetListModelsAccessor` (and inspect/invoke/pull siblings) in
  `pkg/transports/cli/root.go`. Leaf help parity intentionally omits contracted examples until
  the live tree carries `Example` text again.
- Docs-family manifest parity for `you.docs` lives in
  `pkg/transports/cli/climanifestparity` (`CompareDocsHelpIdentity`,
  `TestProductionManifestHelpIdentityParity_DocsFamily`,
  `TestProductionManifestParsingParity_DocsFamily`,
  `TestProductionManifestCompletionParity_DocsFamily`,
  `TestProductionManifestOfflineDocsParity_DocsFamily`,
  `TestGeneratedVsLegacyParityMatrix_DocsFamily`,
  `TestGeneratedVsLegacyOfflineDocsParity_DocsFamily`) with isolated legacy construction via
  `cli.NewLegacyDocsFamilyCommand` and generated parity roots via
  `cli.NewGeneratedDocsFamilyParityCommand`. Offline docs behavior remains proven through
  `pkg/transports/cli/root_docs_test.go`; help parity intentionally omits contracted
  examples until the live tree carries `Example` text again.
- Operator default worker model settings resolve at the CLI/process boundary in
  `pkg/transports/cli/root.go` (`resolveOperatorDefaults`) and flow through
  `run.RunConfig.OperatorDefaults` into `service.FactoryServiceConfig` before
  `cmd/factory/compose.InjectCLITransport`; Wire providers must not read
  `~/.you-agent-factory/config.json` or `YOU_DEFAULT_WORKER_MODEL_*` directly.
- Process startup follows `cmd/factory -> pkg/root -> pkg/wire -> pkg/initializer`: `pkg/transports/cli/startup` carries parsed run or MCP inputs, `pkg/root` selects one `initializer.ProcessPolicy`, `pkg/wire/process.go` applies that policy while constructing exactly one typed `initializer.ProcessGraph`, and `pkg/initializer/core.go` validates the graph policy before starting the already-built graph. Do not duplicate or recompute mode/sidecar policy downstream; API, dashboard, runtime mode, worker-scheduler, and watcher enablement must be governed by the root-selected policy carried on the graph. Keep domain construction out of root and initializer, and do not restore root-local deferred lifecycle closures, initializer config-based runtime constructors, or process-global builder registration. The normalized root home must likewise remain authoritative: thread it through config initialization, named-factory lookup, `run.RunConfig.HomeDir`, system-config persistence, automatic recording, runtime logging, and runtime metrics rather than consulting ambient process globals after command construction. Run construction is split between `run.BuildApplication` and `Application.Run`, with `wire.BuildCLIRunner` returning an initializer application over the completed graph; MCP construction similarly retains its injected execution owner on the Wire graph before initializer startup. Dashboard-suppressed one-shot invocation normalizes through `service.NormalizeInvocationBootstrapConfig`, constructs its session foundation through `wire.InjectRuntimeCore`, and adapts that completed core through `service.NewInvocationBootstrap`; the service adapter must retain the exact graph-owned registry, persistence, durable execution, and runtime-build identities. `InvocationBootstrap.InvokeFactorySession` and `InvocationBootstrap.CloseFactorySession` stay transparent forwards to the wrapped `FactoryService`; `runFactoryInvocation` releases sessions through `releaseInvocationSession` after invocation instead of a CLI-local submit/wait loop. Boundary coverage lives in `pkg/root/process_test.go`, `pkg/wire/process_test.go`, `pkg/wire/cli_test.go`, `pkg/initializer/application_concurrency_test.go`, and the compiled-binary matrix in `tests/release/root_process_smoke_test.go`. Focused initializer migration verification: `go test ./cmd/... ./pkg/transports/http/... ./pkg/transports/cli/... ./pkg/transports/mcp/... ./pkg/initializer/... -short`.
- `you models invoke` reuses the same Wire-built runtime core and
  `service.NormalizeInvocationBootstrapConfig` adapter path as one-shot factory
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

  Internal response-stream event projection into canonical FactoryResponseEvent
  values lives in `pkg/factory/sessions/responsestream/fragmentmap`
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
  `fragmentmap/mapper_fixture_matrix_test.go` is the consolidated coverage matrix for
  every declared internal response-stream event kind plus publisher smoke; focused
  invocation primary-result byte fixtures live in
  `pkg/work/invocation/testdata/primary_result_regression/` and are asserted by
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
  Batch 09 private-contract removal gates live in
  `pkg/factory/sessions/responsestream/removalgate` (`AssertGate`, `AssertClosure`,
  `AssertDocsPrerequisite`, `AssertNoPrivateNDJSONInProductionSurfaces`,
  `AssertPublicTransportLayersDoNotImportLegacyCompat`,
  `AssertLegacyCompatMapperDeleted`,
  `AssertNoRetiredPrivateContractSymbolsInProductionSurfaces`,
  `AssertReleaseNotesMigrationMapping`,
  `AssertPrivateNDJSONRecordTypesRejected`) with package tests in `gate_test.go` and
  functional entrypoints
  `tests/functional/smoke/response_stream_private_contract_removal_gate_smoke_test.go`
  (`TestResponseStreamPrivateContractRemovalGateSmoke`),
  `tests/functional/smoke/response_stream_private_ndjson_contract_smoke_test.go`
  (`TestResponseStreamPrivateNDJSONContractSmoke`), and
  `tests/functional/smoke/response_stream_private_contract_closure_smoke_test.go`
  (`TestResponseStreamPrivateContractClosureSmoke`). Supported CLI NDJSON
  recordType constants and retired-record rejection live in
  `pkg/factory/sessions/responsestream/ndjsoncontract`; the canonical decoder in
  `pkg/workers/provider/parityfixtures/transport.go` rejects retired private
  record types before validating public envelopes. Run these before deleting
  private NDJSON record types. The retired `responsestream/compat` mapper package
  must stay deleted; internal fragment projection now lives in `fragmentmap`.
  The exact old→new CLI JSON migration map for retired private NDJSON records lives in
  `docs/release-notes/response-stream-private-ndjson-removal.md` (indexed by
  `docs/release-notes/README.md`) and is asserted by
  `AssertReleaseNotesMigrationMapping`.
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
- `internal/builtcliacceptance` owns the hermetic built-CLI acceptance harness for
  the S24 cross-surface matrix: `NewHarness` builds `./cmd/factory`, `NewSession`
  allocates isolated home/log/work directories, `ProcessEnvForIsolatedHome` redirects
  profile env vars, `WithNoExternalServer` reserves a loopback `--server` URL without
  a pre-running listener, and `ScenarioFailure` carries exit status plus stdout/stderr
  tails for scenario mismatches. Focused harness proof lives in
  `tests/functional/acceptance/harness_smoke_test.go`; fresh/migrated install
  customer outcomes are asserted in
  `tests/functional/acceptance/install_outcomes_test.go` via built-CLI
  `config init` against isolated homes; provider absent/configured/discovered
  postures are asserted in
  `tests/functional/acceptance/provider_outcomes_test.go` via built-CLI
  operator-default resolution and named `@you/goal` mock-worker runs; invalid-goal
  and quiet-mode customer outcomes are asserted in
  `tests/functional/acceptance/invalid_quiet_outcomes_test.go` via unknown
  named-factory rejection, invalid topology graph-reference guidance, quiet
  operational-failure terminal mute, and quiet successful primary-result-only
  named `@you/goal` mock-worker runs; primary-only and human/JSON response-stream
  output customer outcomes are asserted in
  `tests/functional/acceptance/output_outcomes_test.go` via built-CLI primary
  result-only stdout, canonical response-stream NDJSON records, human
  response-stream vocabulary, and primary-only versus response-stream terminal
  `InvocationResponse` parity on the same mock-worker fixture; local-model invoke,
  goal-repeat, and subagent customer outcomes are asserted in
  `tests/functional/acceptance/invoke_repeat_subagent_outcomes_test.go` via
  built-CLI `models invoke` bootstrap readiness failures, repeated named
  `@you/goal` JSON invocations with distinct `requestId`/`traceId` and stable
  installed-factory reuse, and named `@you/subagent` primary JSON plus
  primary-only versus response-stream terminal parity on mock-worker fixtures.
  S24 scenario-to-outcome mapping is canonical in `internal/builtcliacceptance/scenarios.go`
  (`S24Scenarios`) and locked by `tests/functional/acceptance/scenario_matrix_test.go`;
  PR verification runs the focused suite through `make test-built-cli-acceptance`
  inside `make verify-tests`.
  Later S24 scenario stories should compose scenario assertions on top of this
  package rather than re-building binary/home/log wiring in each test file.
- `pkg/factory/packages/catalog.go` owns packaged factory lookup and metadata;
  payload sources live under `pkg/factory/packages/definitions/`, and config
  initialization is the only catalog-to-disk installation boundary. Named
  resolution in `pkg/config/layout.go` reads project-local then global disk
  state only; it does not install packages or expose compatibility JSON aliases.
  `pkg/factory/packages/packageassets` is the shared, side-effect-free packaged
  asset assembly entry point: package owners supply the authored `factory.json`
  and an explicit embedded `fs.FS`, and definitions call this assembler before
  their payload enters the catalog. It delegates prompt declarations to
  `pkg/factory/packages/promptassets` and discovers regular UTF-8 `scripts/**`
  assets as deterministic `SCRIPT` bundled files at matching
  `factory/scripts/**` targets. Discovery rejects non-regular, unreadable, or
  invalid UTF-8 assets, and assembly rejects unsafe or duplicate canonical
  bundled targets before the payload can reach config initialization. The
  assembler attaches exact asset bytes but does not install or persist anything.
  `pkg/config/configinit` passes each missing assembled catalog payload through
  the transactional `factoryconfig.PersistNamedFactory` boundary. That shared
  persistence path materializes `SCRIPT` entries at mode `0755`, writes only
  thin UTF-8 bundled-file metadata to `factory.json`, and validates the staged
  runtime before publishing the named-factory directory. Existing valid package
  directories are loaded read-only and skipped as a whole, so later init runs
  do not normalize permissions or replace operator-edited scripts. At runtime,
  `pkg/workers/executor.ScriptExecutor` resolves portable `scripts/**` commands
  (and legacy `factory/scripts/**` references) against the active runtime
  configuration's factory directory before using the generic subprocess path;
  package assembly and config initialization do not own process execution.
  Worker prompt declarations become
  canonical inline bodies, while workstation declarations retain `promptFile`
  metadata for editable split-layout materialization.
  Packaged `@you/goal`
  has one `execute-goal` `AGENT_RUN` workstation with `REPEATER` behavior:
  accepted completion routes to `goal:complete`, continue/reject route back to
  `goal:init`, and worker or workstation failure routes to `goal:failed`.
  `pkg/factory/packages/definitions/goal/` owns the authored factory and concise
  executor prompt. Both `goal-executor` and `execute-goal` declare that shared
  package-relative asset and use the package-neutral prompt assembler; goal does
  not own a JSON walker or name-to-prompt map.
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
  records wrapping public `FactoryResponseEvent` values and ends with exactly one
  `invocation_result` record, primary-only and response-stream terminal
  `InvocationResponse` outcomes match for the same successful fixture, JSON
  NDJSON rejects retired private `recordType` values (`progress`, `stream_gap`,
  `compaction`, `primary_result`), human stdout uses canonical response-event
  formatting and avoids legacy provider fragment dialect, durable `FactoryEvent`
  history omits internal response-stream terms,
  and generated public API artifacts stay internal-only. Reuse
  `writePackagedGoalBuiltinTopologyMockWorkers`, `materializeNamedGoalFactoryForRoutingSmoke`,
  and `support.StartFunctionalAPIServer` when extending boundary verification.
  Stream-responses gate audit intake and blocking residuals are recorded in
  `docs/internal/development/plans/you-goal/stream-responses-final-audit.md`.
  Story-002 goal stream integration evidence is recorded in
  `docs/internal/development/plans/you-goal/goal-response-stream-integration.md`.
- Named `@you/subagent` response-stream boundary smoke coverage lives in
  `tests/functional/smoke/cli_named_subagent_response_stream_smoke_test.go`,
  proving real CLI `--output response-stream` on the one-pass subagent factory
  reuses the shared canonical response-stream renderer contract proven for
  `@you/goal`: human stdout uses canonical response-event formatting and avoids
  legacy fragment dialect, JSON NDJSON uses `response_event` and
  `invocation_result` records with validated `FactoryResponseEvent` values,
  exactly one terminal `invocation_result` wraps the shared `InvocationResponse`,
  and primary-only versus response-stream terminal outcomes match for the same
  successful fixture. Reuse `writePackagedSubagentMockWorkers` and the goal
  stream NDJSON helpers when extending subagent stream verification.
  Story-003 subagent stream integration evidence is recorded in
  `docs/internal/development/plans/you-goal/subagent-response-stream-integration.md`.
- API/CLI response-stream terminal parity smoke coverage lives in
  `tests/functional/smoke/cli_named_response_stream_api_parity_smoke_test.go`,
  proving live session `POST /factory-sessions/{session_id}/invocations`
  `InvocationResponse` outcomes match CLI JSON response-stream terminal
  `primary_result` records for the same successful `@you/goal` and
  `@you/subagent` fixtures. Reuse `materializeNamedGoalFactoryForRoutingSmoke`,
  `materializeNamedSubagentFactoryForSmoke`, `startNamedGoalRoutingAPIServer`,
  `postNamedGoalRoutingInvocationOnServer`, and the goal/subagent response-stream
  CLI helpers when extending API/CLI stream parity verification. Story-004 gate
  evidence for canonical `FactoryResponseEvent` SSE payload encoding parity and
  terminal outcome parity is recorded in
  `docs/internal/development/plans/you-goal/api-cli-response-stream-parity.md`.
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
- `pkg/service/javascript_session_invocation.go` adapts JavaScript current-factory
  invocation to `pkg/factory/sessions/execution.Service.StartSync`; JavaScript
  factories do not submit Petri Work. Keep signature normalization shared with
  `pkg/factory/sessions/invocation`, then coerce declared JSON-schema scalar
  types before durable JavaScript validation so CLI/API string carriers retain
  the workflow's typed argument contract. When the current factory has already
  resolved an authored workflow asset, pass that asset as an inline durable
  source together with its source identity, args schema, agents, and default
  policy; resolving its scoped name again from the session directory loses the
  materialized named-factory context.
- Functional coverage for `@you/deep-research` belongs in
  `tests/functional/smoke/cli_named_deep_research_smoke_test.go` and
  `tests/functional/runtime_api/api_packaged_deep_research_invocation_test.go`.
  For a signature-backed CLI run with `--with-mock-workers`, place the mock-worker
  config path before `--`, then place factory-defined arguments after `--`; this
  keeps the run-level config-path convention separate from signature parsing.
  Verify delegated runs through the durable session ID returned by invocation and
  `GET /factory-sessions/{session_id}/dispatches`, rather than inferring child
  activity from the final synthesis alone.
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
