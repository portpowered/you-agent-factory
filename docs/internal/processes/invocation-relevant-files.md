# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- Review-gated factories that must revise rejected work should preserve the
  original input on the work-stage route, retain non-empty worker output in the
  `_last_output` token tag, and read `Payload`, `PreviousOutput`, and
  `RejectionFeedback` from the retry prompt. This keeps the request, candidate,
  and reviewer feedback available without weakening the approval-only terminal
  return contract. Focused coverage belongs with the packaged invocation tests.
- Package tests are subject to the same `pkg-maint` cyclomatic-complexity limit
  as production code. Keep topology fixtures readable by delegating independent
  identity, routing, and validation assertions to named test helpers.
- Generic relationship presence must inspect every registered flag spelling.
  Cobra marks the canonical flag when a shorthand is used, but aliases are
  separate `pflag.Flag` records even when they share typed storage.
- An inherited generic flag reuses its ancestor's persistent Cobra record, so
  its projected metadata must match the declaration after normalizing only the
  stable input ID, scope, inheritance reference, and lifecycle item ID.
  Reject presentation or lifecycle differences during construction rather
  than accepting child metadata that cannot be projected independently.
- Adding an embedded packaged-factory definition also adds two measured backend
  packages to the unit and functional coverage manifests: record the definition
  package's observed numeric floor and the wrapper package's documented
  measurement exception when it has no executable statements. Verify both with
  `make test-unit-coverage` and `make test-functional-coverage`.
- The customer-implementable provider inference contract lives in
  `pkg/services/workers/provider/inferencecontract/`. Invoke implementations
  through `ExecuteInvocation` so provider-authored drafts are validated for
  provenance, invocation and item correlation, lifecycle ordering, terminal
  result agreement, and exactly-once close before they reach orchestration.
  Keep this boundary provider-neutral and test it with deterministic writers;
  Factory Session publication identity, sequencing, retention, and replay stay
  outside this package. Customer integrations can reuse
  `inferencecontract/testkit.Run` with fresh factories for final-only,
  streaming, and tool-lifecycle modes; pass at least two opaque identities so
  conformance never depends on a built-in provider name. Reuse
  `inferencecontract/testkit.RunAdverse` for normalized failure, cancellation,
  deadline, response-sink backpressure, and terminal-state scenarios. A sink
  write failure is terminal: preserve it for orchestration and reject every
  later provider write or close without sending a competing completion. Once
  an authoritative completed message represents success, reject a later
  failure completion and discard the buffered terminal tail so orchestration
  observes neither side of a contradictory outcome. Reject a second
  authoritative completed message as `final_result_agreement`, even when it
  uses a different item correlation, so no earlier represented result can be
  overwritten before completion validation.

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
  `productionRunSubmitCommands` always selects the generated family.
  `NewGeneratedRunSubmitFamilyCommandForParity` exposes the isolated generated
  tree for focused generated-constructor verification. Runtime behavior is
  tested through the production root with injected `RunServices` and
  `SubmitServices`; no handwritten command-tree oracle is retained.

## CLI invocation output modes (primary-result, human response-stream, NDJSON)

Use this lane when changing `you run` stdout modes, `--output response-stream`,
root `--json` NDJSON records, or packaged `you docs run` output-mode guidance.
Supported live and replayed one-shot factory invocations expose three modes;
continuous, `--work`, and other non-invocation run shapes do not offer
response-stream output.

| Mode | Selection | Stdout contract |
|---|---|---|
| Primary-result (default) | `you run --factory …` or `you run --named …` without `--output response-stream` | Successful invocations write only the configured `primaryResult` to stdout |
| Human response-stream | `you run --factory … --output response-stream` (no root `--json`) | Customer lifecycle summaries from ordered canonical `FactoryEvent` records, followed by the terminal primary result or invocation outcome |
| NDJSON automation | `you --json run --factory … --output response-stream` | Each non-empty stdout line is one JSON record: `recordType=factory_event` with a nested canonical `FactoryEvent`, ending with at most one terminal `recordType=invocation_result` whose `response` is the `InvocationResponse` |

**CLI boundary ownership**

- Mode flag wiring and unsupported run-shape rejection:
  `pkg/transports/cli/root_work.go`, `pkg/transports/cli/root_run_test.go`
  (manual `you run --output response-stream` parsing after `DisableFlagParsing`)
- `RunConfig.InvocationOutputMode`, validation, and error mapping:
  `pkg/transports/cli/run/invocation_error.go`
- Session-owned canonical event collection, human lifecycle mapping, and
  lossless JSON stdout ordering:
  `pkg/transports/cli/run/run_clean_invocation.go`,
  `pkg/transports/cli/run/factory_invocation_input.go`, and
  `pkg/services/factory_sessions/runtimeopening/invocation/operation.go`

**Shared observation contract**

- Canonical Factory Event vocabulary and sequence context:
  `pkg/services/factory_definitions/contracts/factory_events.go`
- Shared ordered output serialization and final-once terminal write:
  `pkg/services/factory_visualization/factory_event_stream.go`
- Keep provider-response chunks and ephemeral `FactoryResponseEvent` values out
  of this presentation boundary. The Factory Session invocation operation
  attaches the canonical consumer before live execution, durable JavaScript
  execution publishes canonical phase/checkpoint updates through the same
  invocation-local callback, and finite replay history enters that consumer
  before the separate terminal response is finalized.
- JavaScript canonical history must remain append-only while an invocation-local
  consumer is attached. Build phase/checkpoint events in runtime-record order,
  represent phase completion as a distinct immutable transition, and assign
  sequence context only when appending; never resequence or replace a record
  that may already have reached stdout. Prove changes with a real
  phase → checkpoint → phase execution whose live events exactly equal its
  durable replay in IDs, types, payloads, and strictly increasing sequence data.
- Preserve the canonical event envelope and sequence context, but recursively
  omit provider response, diagnostic, Provider Session, delta, and tool-call
  fields from the JSON presentation payload before encoding. Keep this pure
  projection in `factory_invocation_input.go`; do not mutate stored history.

**Packaged operator guidance**

- `docs/reference/run.md` (invocation output modes and copyable examples);
  cross-link `you docs config` for return/output policy
- Provider fidelity variability:
  `docs/reference/workers.md` (`## Response-stream provider fidelity`)
- Session SSE counterpart:
  `docs/reference/sessions.md` (`## Response-event stream lifecycle and reconnect`)
- Run `make docs-reference-smoke` after `docs/reference/` edits

**Focused CLI and docs verification**

- Canonical raw customer-boundary coverage:
  `tests/functional/cli/factory_run/output` owns non-streaming presentation and
  standard failure stderr/terminal-record behavior, while
  `tests/functional/cli/factory_run/events` owns discriminated NDJSON event
  integrity. These tests construct the process through `root.BuildProcess` and
  use mock-worker `accept` or `reject` entries for deterministic success and
  terminal failure without a live provider.
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
- Built-executable coverage for this lane is limited to operating-system exit
  status in `tests/functional/acceptance/output_outcomes_test.go` and
  `tests/functional/acceptance/invalid_quiet_outcomes_test.go`; stdout, stderr,
  and event-presentation behavior belongs to the canonical raw packages above.

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
  It must use Factory-contract binding constants directly; generated HTTP enums
  are transport-boundary types and must not enter this domain package.
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
- `pkg/initializer/runtimeconstruction/operatordefaults/operator_defaults_runtime.go`
  is the
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
  validation shared by validate-only and save pre-check flows. It validates the
  authored policy and invocation-signature vocabulary declared by
  `pkg/factory/contracts/factory_config.go`; generated OpenAPI enums are only
  converted at config and transport boundaries. It also validates explicit
  model-worker `modelProvider` values; permit only supported providers, the
  symbolic `DEFAULT`, or an exact interpolation reference to a declared
  invocation parameter so invalid materialized worker edits fail before runtime.
  Keep established public aliases such as `openai` accepted through the same
  canonicalization helper; validation must not turn a permissive legacy alias
  into an incompatibility.
- `pkg/config/factory_config_mapping*.go` maps `invocationReturn` between the
  OpenAPI factory contract and the internal runtime config.
- `pkg/work` owns canonical `Work`, `WorkRequest`, `WorkContentPart`, invocation
  argument, dispatch identity, relation, and payload-lineage contracts. The
  remaining request-validation error and `FactoryInvocationResult` session
  result shape stay at their current boundary until Factory Session contracts
  converge; the Factory Session owner constructs that shared result.
- `pkg/work/content/contract` translates between generated OpenAPI `WorkContent`
  and the backend-owned `work.WorkContentPart` shape; pure content rules remain
  in `pkg/work/content`.
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
  `pkg/services/operator_settings` and `pkg/transports/cli/run` should keep only precedence,
  filename, and reporting behavior around those defaults.
- When global named-factory guidance changes, update the handwritten CLI help in
  `pkg/transports/cli/root_factory.go` and `root_work.go`, the authored
  `contracts/cli/commands.json` records, and `docs/reference/authoring-factories.md`
  plus `config.md`. Run `make cli-manifest-generate` and
  `make contracts-generate` for derived CLI artifacts, then update intentional
  CLI baselines and run `make docs-reference-smoke`.
- Persisted local `backendScopeID` values live in the same
  `~/.you-agent-factory/config.json` system config file. Keep load/generate/persist
  logic in `pkg/services/operator_settings`, inject the generated-model codec from
  `pkg/transports/mapping/globalconfig`, and resolve identity before any Factory
  Session identity is exposed. Present documents are decoded as the closed
  `GlobalConfig` contract before reuse or persistence, so malformed content,
  trailing JSON, and unknown fields fail with the config path.
  Local backend scope policy: blank values generate `local-<uuid>` once and persist
  it; valid `local-<uuid>` and other explicit non-empty scopes are reused across
  restarts; values starting with `local-` that are not valid `local-<uuid>` fail
  startup with a config error instead of being silently replaced.
- Complete operator-config provider/model updates belong in
  `pkg/services/operator_settings.ConfigDocumentService`: validate and encode the
  full candidate before filesystem side effects, publish through a uniquely
  created same-directory temporary file, and treat `Rename` as the single commit
  boundary after write, sync, close, permission, and cancellation checks. Share
  one explicit persistence lock between service copies and reads so concurrent
  callers remain deterministic on platforms where overlapping replacement and
  reads otherwise produce sharing violations; failed attempts remove only their
  own temporary artifact and never rewrite the committed destination directly.
  Prompted setup should use a write-free function contract that receives the
  current semantic defaults, maps EOF to an explicit cancellation outcome, and
  delegates successful input to the same context-aware load/merge/persist
  operation used by pre-supplied values.
- Canonical `you config init` system bootstrap belongs in
  `pkg/initializer/configinit` (`Init`, `SystemConfigOutcome`) and
  `pkg/transports/cli/configinit` (`Init`, `InitConfig`) with command wiring in
  `pkg/transports/cli/root.go` (`newSystemConfigCommand`, `newSystemConfigInitCommand`).
  Fresh homes create `~/.you-agent-factory/config.json` through
  `pkg/services/operator_settings.EnsureLocalBackendScope`; existing config files are
  validated with `operatorconfig.LoadFileConfig` and left byte-identical on
  re-run. `pkg/initializer/configinit` receives the Factory Definitions packaged
  catalog and root persistence capability from Wire and persists only missing
  catalog entries through that capability;
  valid installed directories are loaded and skipped without rewriting
  customer-owned files. Isolated-home rerun coverage lives in
  `pkg/initializer/configinit/init_test.go` (`TestInit_DoubleRunIsSuccessfulNoOp`,
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
  participants on flag or argument IDs visible in the command's effective scope
  so diagnostics name the exact record path. Group relationship participants are
  unordered sets; dependency and conditional relationships direct `when` toward
  their participant targets and must remain acyclic. Compatibility records must
  not be copied into the primary manifest
  merely to make generation convenient. Apply family-completeness validators only
  to the manifest classification that owns that family: `LoadProduction` owns
  canonical run/submit validation, while `LoadCompatibility` must remain able to
  decode the separately classified workflow-only manifest.
- Canonical flag and positional-input shapes are defined in
  `contracts/cli/command-manifest.schema.json` and decoded by
  `pkg/transports/cli/climanifest`. New canonical records use the shared typed
  `defaultValue` / `noOptionDefaultValue` shape plus explicit cardinality,
  accepted sources, and stable `handlerBindingId`; serialized `default`,
  `noOptionDefault`, `changedDefault`, `binding`, and argument `channels` remain
  compatibility fields for manifests not yet migrated. Preserve both shapes in
  generated family artifacts until the authored production manifest is migrated.
  Every `scope: inherited` flag must identify its persistent ancestor through
  `inheritedFromInputId` and preserve that source's public and value semantics.
  Positional arguments are command-local; reject persistent or inherited
  positional scope instead of accepting a declaration with no resolvable
  ancestor identity. A canonical `defaultValue` is optional, but its presence
  must exactly match the input declaring `manifest-default` in
  `acceptedSources` so consumers never infer a default-source policy.
  Runtime handler bindings for an inherited flag must read the persistent
  ancestor's live storage; do not leave execution dependent on retired
  command-local storage after canonicalizing an input as inherited.
  Generic Cobra projection lives in
  `pkg/transports/cli/climanifestcobra/constructor.go`, while its invocation-local
  typed flag values and stable-ID `InputValues` access live with the package's
  other binding state in `pkg/transports/cli/climanifestcobra/options.go`.
  Validate the complete input and inheritance plan before registering any pflag
  values, and register inherited records against their persistent ancestor's
  canonical storage rather than allocating command-local copies.
  The same constructor plans positional inputs in position order, rejects gaps
  and non-terminal variadics before Cobra mutation, and records parsed typed
  values on the invocation-local command for stable-ID handler access.
  Relationship evaluation uses stable flag or argument references and explicit
  CLI presence, runs in Cobra's pre-handler phase, and reports public input
  spellings without exposing input values.
  Generic help, lifecycle, and completion projection lives beside the docs
  transport boundary in
  `pkg/transports/cli/climanifestcobra/docs_constructor.go`. Validate command
  and flag lifecycle records, positional lifecycle records when authored, and
  every completion mode before Cobra mutation. Static completion consumes
  declared enum choices; dynamic completion callbacks are supplied through
  `GenericBindings.Completions` keyed by stable input ID, never by a public flag
  name or command path.
  Generic runnable dispatch follows the same boundary: validate every
  `Command.Handler.ID` and `GenericBindings.Handlers` entry before Cobra
  projection, reject duplicate handler ownership, and invoke the selected
  stable-ID handler with a detached normalized `InputValues` snapshot. Public
  command paths and aliases must not participate in executable lookup.
  `GenericConstructor.Construct` is the strict stateless transport role for
  functional projection evidence; keep `NewCommandTree` as the convenience
  constructor while later family migrations remain outside this foundation
  work. Functional tests may call the role, but must not assemble customer
  behavior through transport constructors in place of `root.BuildProcess`.
  Declare canonical environment, operator-config, and stdin routing in command
  `sourceBindings`, with an external key where applicable and an explicit input
  target. Declare each canonical handler route in `handlerBindings`; its stable
  ID is the value referenced by the input's `handlerBindingId`.
  Every command containing a canonical input must declare a precedence record,
  even when the command is not otherwise marked authoritative. That record uses
  the exact highest-to-lowest order `cli`, `stdin`, `environment`,
  `operator-config`, `manifest-default`, `factory-signature-default`.
  `climanifest.CanonicalPrecedence` owns the pure policy: higher tiers replace
  lower tiers, scalar observations from one binding
  use the last value, repeated observations append in order, and multiple
  same-tier bindings for one input are rejected.
  Resolved runtime CLI values belong in
  `pkg/transports/cli/resolvedinput`: its collection-based resolver and accessors
  use stable schema input IDs and canonical typed values, with no Cobra, public
  spelling, position, environment, filesystem, or process-global dependency.
  Definitions supply typed source precedence explicitly; the resolved snapshot
  retains the winning provenance and derives changed/default state from that
  source instead of asking handlers to infer it. Typed, wrap-safe access
  diagnostics identify missing IDs and value-kind mismatches so handler adapters
  can translate failures without string parsing. Definitions also carry schema
  sensitivity into the detached snapshot: diagnostic and observation boundaries
  expose provenance and changed/default state while replacing sensitive scalar
  or collection values with `resolvedinput.RedactedValue`.
  Static-plus-Factory composition is owned by
  `pkg/transports/cli/climanifest.ComposeRunInputs`: pass the validated `you.run`
  command and only the selected Factory's `InvocationSignatureConfig`. The pure
  projection keeps manifest inputs separate from dynamic Factory parameters and
  rejects command-name, long-name, alias, shorthand, positional, stdin-owner,
  and stable-binding collisions with sorted diagnostics that identify both
  owners. Named and explicit-file selectors must not enter this composition
  policy; equivalent selected signatures produce equivalent results.
  Keep effective-scope spelling and inheritance checks in
  `internal/contractvalidator` so schema-valid manifests still receive stable,
  path-specific semantic diagnostics before generation or consumption.
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
  construction outside this family slice.
- MCP protocol and resume smokes in `pkg/transports/cli/mcp/serve_*_test.go`
  should enter through `cli.NewRootCommandWithOptions` and the injected startup
  boundary, then delegate to the existing `mcp.RunServe` implementation with the
  exact parsed stdio streams. This keeps fixture/runtime selection, JSON-RPC,
  EOF/cancellation, and durable resume assertions attached to the generated
  production `you mcp serve` construction instead of proving only its detached
  handwritten execution adapter.
- Representative-family metadata generation lives in
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
  plus registry-attached handwritten handlers. Production root construction is
  generated-only through `newRootCommandWithGeneratedRepresentativeFamily`;
  deprecated handwritten command trees and constructor-parity interfaces have
  been removed. `WorkFamilyBindings.FlagUsages` supplies local flag help text.
- Whole-production CLI closure is checked by `pkg/transports/cli/clicontract`
  and exposed through `cmd/clicontractsmoke` / `make cli-contract-smoke`.
  Keep deliberate smoke violations snapshot-only: they must use the production
  validator and diagnostics without executing commands, invoking services,
  mutating Cobra state, or requiring network access.
- Operator default worker model settings resolve at the CLI/process boundary in
  `pkg/transports/cli/root.go` (`resolveOperatorDefaults`) and flow through
  `run.RunConfig.OperatorDefaults` into `service.FactoryServiceConfig` before
  `cmd/factory/compose.InjectCLITransport`; Wire providers must not read
  `~/.you-agent-factory/config.json` or `YOU_DEFAULT_WORKER_MODEL_*` directly.
- Process startup follows `cmd/factory -> pkg/root.BuildProcess -> pkg/wire.InjectBundle -> application.Process.Execute -> CLI-selected initializer -> pkg/initializer`. Production and functional tests construct the same reusable process through `BuildProcess`; production supplies empty edges while functional tests replace explicit external boundaries. Every `Execute` call constructs a fresh command tree from invocation-local input. Only after CLI parsing does the matching `Run` or `Stdio` initializer construct its service subtree. There is no generic construction request, alternate production injector, root service-splicing path, or `ProcessGraph`. Keep domain construction out of root and initializer, do not restore root-local lifecycle closures or process-global builder registration, and never construct HTTP/dashboard resources for stdio or an MCP stdio transport for run/API. The normalized invocation home remains authoritative through config initialization, named-factory lookup, `run.RunConfig.HomeDir`, persistence, recording, runtime logging, and metrics. `pkg/initializer` only starts, joins, unwinds, and closes the selected bundle. Boundary coverage lives in `pkg/root/root_test.go`, `pkg/wire/cli_test.go`, initializer application tests, functional CLI tests, and the compiled-binary matrix in `tests/release/root_process_smoke_test.go`.
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
  subscription is unavailable. Keep mode validation and unsupported run-shape
  rejection in `pkg/transports/cli/run/invocation_error.go`, canonical human and
  JSON rendering in `pkg/transports/cli/run/run_clean_invocation.go`, live/replay
  consumer wiring in `pkg/transports/cli/run/factory_invocation_input.go` and
  `pkg/services/factory_sessions/runtimeopening/invocation/operation.go`, and
  JavaScript canonical event publication in
  `pkg/services/factory_sessions/execution`. `pkg/transports/cli/root_work.go` and
  `pkg/transports/cli/root_run_test.go` apply manually parsed `you run --output response-stream`
  to `RunConfig.InvocationOutputMode` after `DisableFlagParsing` argument parsing.
  The `pkg/transports/cli/run` package is at the
  15-file limit; extend existing files instead of adding new ones. Human response-stream
  terminal outcomes use `--- invocation outcome ---` with structured status/error
  fields. Both human and JSON modes consume canonical events incrementally for
  live invocations and consume finite canonical history through the same callback
  for replay. Human mode renders only its
  bounded typed allow-list; JSON emits only `factory_event` records and sends
  every accepted event plus the final `invocation_result` through one lossless
  ordered writer. Do not reuse the human progress queue's drop policy for JSON.
  Invocation failures are mapped in `pkg/transports/cli/run/invocation_error.go`
  to one generated API `ErrorResponse` with the established symbolic invocation
  code and `INTERNAL_SERVER_ERROR` family. `pkg/transports/cli/root_work.go`
  writes that object directly to stderr before applying quiet human-terminal
  suppression, so human, quiet, single-JSON, and NDJSON modes share one error
  boundary while retaining their distinct terminal stdout rules.
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
  Provider-native typed adapters live under `pkg/services/workers/provider/<provider>`
  and emit validated `responseevents.Draft` values. Sanitized cross-provider
  parity transcripts and the terminal harness are Workers-owned, same-package
  test support under `pkg/services/workers/provider/paritytests`, with
  fidelity-class fixtures under `testdata/`; extend that catalog for CLI/API
  parity proofs instead of inventing parallel fixture trees. Use
  `RunTransportParity` plus `AssertCLIAPITransportParity` and
  `AssertTruthfulStreamingFidelity` there to
  compare decoded CLI NDJSON and API SSE `FactoryResponseEvent` values and
  terminal `InvocationResponse` outcomes for every fidelity class (full-stream,
  partial-stream, snapshot-only, and final-only including Agy) plus the structured
  tool-lifecycle fixture via `AssertObservableToolLifecycle` before adding new
  transport parity tests. Use `AssertPrimaryStreamModeParity` with
  `ProjectPrimaryOnlyInvocation` and `ProjectResponseStreamInvocation` to prove
  primary-only and response-stream observation modes agree on authoritative
  terminal `InvocationResponse` outcomes for the same fixture run. Consolidated
  Batch 09 parity proofs live in `AssertCrossProviderParityCatalog`
  and `AssertCrossProviderParityForFixture`; run them from
  `pkg/services/workers/provider/paritytests/suite_test.go`
  (`TestCrossProviderParitySuite_Catalog`). Maintainer lanes:
  `make provider-parity-smoke` (also invoked by `make api-smoke`) and
  `make response-stream-stress-smoke` for response-event backpressure/race proofs.
  Batch 09 private-contract removal gates live in
  `internal/testutil/responsestreamremovalgate` (`AssertGate`, `AssertClosure`,
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
  `pkg/services/factory_sessions/internal/responsestream/ndjsoncontract`; the removal
  gate validates retired vocabulary directly, while CLI renderer tests decode
  public envelopes through transport-local canonical fixtures. Run these before deleting
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
  through Factory-owned `pkg/factory/replay`, backed by policy-free artifact IO in
  `pkg/platform/replay`. Production-path tests must exercise `FactoryService.Run`
  plus public session, event, artifact, and result reads with fail-on-use live
  dependencies.
- `pkg/cli/run/factory_invocation_input.go` must pass raw positional/stdin
  bytes into `pkg/work/invocation.ResolveTextInput` and surface `INVOCATION_INPUT_EMPTY`
  from the shared resolver instead of pre-trimming or short-circuiting with
  transport-specific empty-stdin errors. When `Stdin` is overridden away from
  `os.Stdin` (cobra `SetIn`, tests, or programmatic callers), treat it as piped
  input even if the process-level `os.Stdin` is still a TTY.
- `pkg/workers/inference/inference.go` resolves worker-owned inference-run
  operation bindings, builds the provider-neutral request envelope, and shapes
  canonical `WorkContentPart` output shared by direct model invocation and
  Factory Session execution. Generated OpenAPI binding conversion belongs in
  `pkg/transports/mapping/workerinference`; inference output parsing consumes the
  Work-owned content shape and must not import generated or transport mapping
  packages.
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
  process exit status and unrelated invalid-topology guidance are asserted in
  `tests/functional/acceptance/invalid_quiet_outcomes_test.go`, while terminal
  invocation failure exit status is asserted in
  `tests/functional/acceptance/output_outcomes_test.go`. Presentation bytes and
  event shapes are intentionally owned by the raw Factory-run suites. Local-model
  invoke and goal-repeat customer outcomes are asserted in
  `tests/functional/acceptance/invoke_repeat_subagent_outcomes_test.go` via
  built-CLI `models invoke` bootstrap readiness failures, repeated named
  `@you/goal` JSON invocations with distinct `requestId`/`traceId` and stable
  installed-factory reuse, and the unrelated packaged `@you/subagent` primary
  JSON outcome. S24 scenario-to-outcome documentation is canonical in
  `internal/builtcliacceptance/scenarios.go` (`S24Scenarios`); observable
  behavior is proved by the focused acceptance scenarios rather than
  inventory-only test-name assertions.
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
  `pkg/initializer/configinit` passes each missing assembled catalog payload
  through the injected Factory Definitions `Persistence` boundary. That shared
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
- Packaged `@you/review` uses that same catalog, assembly, and editable
  materialization path. Its `reviewable-work` lifecycle routes `init` through
  `execute-review-work` to `in-review`, then through `review-review-work` to
  the explicit invocation-return terminal `approved` state. The reviewer uses
  `decision-envelope`: accepted envelopes carry the approved candidate in
  `output`, rejection returns to `init` with feedback, and failures route to
  `failed`. Keep this approval-only topology and explicit return policy aligned
  when changing packaged-factory plumbing.
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
  the injected `run.Open` operation must also skip listener reservation entirely in
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
- Named `@you/review` CLI invocation coverage lives in
  `tests/functional/smoke/cli_named_review_invocation_smoke_test.go`. Materialize
  the package beneath an isolated home, run the built CLI with scripted mock
  workers, and use a durable reviewer counter to prove a rejection precedes
  approval. Run the same gate for default configuration, edited materialized
  worker configuration, and `--default-worker-model-provider` /
  `--default-worker-model` operator overrides.
- Named `@you/goal` operator-control smoke coverage lives in
  `tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go`,
  proving API and CLI pause/resume buffering, ordered post-resume drain via
  plan-goal dispatch `StartTime` ordering in `DispatchHistory`, interrupted
  inspect summaries via `session show` and `work show`, and durable
  `SESSION_LIFECYCLE_CONTROL` replay events. Reuse
  `writePackagedGoalSlowPlannerTopologyMockWorkers` when ordered drain timing
  needs observable separation between buffered submissions.
- Canonical CLI event/output presentation coverage lives only under
  `tests/functional/cli/factory_run/output` and
  `tests/functional/cli/factory_run/events`. Older goal, subagent, API-parity,
  and private-record functional smoke matrices were retired when this raw
  customer-boundary owner was established; the development-plan documents remain
  historical records, not active test-ownership maps.
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
- `pkg/config/defaultpaths/default_paths.go` owns the canonical shared named-factory
  root for both `you config init` materialization and `you run --named` lookup;
  use `defaultpaths.NamedFactoriesRoot` instead of duplicating the home-relative
  directory in runtime code or tests. `configinit.Init` inventories legacy
  factory identities from their hierarchical directories and migrates them from
  `LegacyNamedFactoriesRoot` before packaged installation, even when an edited
  `factory.json` is temporarily invalid. Migration must preflight conflicts and
  never overwrite the canonical customer-owned copy or silently install a
  packaged replacement for an invalid legacy edit.
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
- `pkg/factory/packages/catalog.go` is the single registration point for
  shipped named factories. A new package needs a catalog definition plus a
  directly-loadable factory payload; `you config init` materializes every
  catalog entry without separate CLI registration. Customer-facing packaged
  invocation guidance belongs in `docs/reference/run.md`.
- Invocation-interpolated worker `modelProvider` and `model` fields are resolved
  at dispatch time. A packaged factory that must be runnable without role flags
  should declare parameter `defaultValue`s in its invocation signature; operator
  defaults only fill authored empty worker fields before interpolation.
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
- Packaged runtime behavior in `pkg/factory/subsystems/subsystem_transitioner.go`
  must first verify the effective `RuntimeFactoryConfigLookup` identity before
  applying package-owned token relations or metadata. Workstation and Work type
  names are authored customer data, so they are never sufficient to identify a
  packaged topology; keep a mutation-level customer-name-collision regression
  test alongside the transitioner behavior.
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
- Managed-runtime invocation readiness gating and direct invocation policy live in `pkg/models/service/invoke.go`; the canonical service consumes neutral `pkg/models/host.Host.InspectReadiness` snapshots, projects public readiness through `pkg/transports/mapping/managed_runtime_invocation.go`, and owns invocation failure classification and readiness logs. Stable provider command identity lives in `pkg/models/provider`; worker and model invocation exchange `pkg/workers/execution` requests, results, provider sessions, and normalized failures, while `pkg/workers/diagnostics` owns the safe generated projection. `pkg/wire/production.go` supplies the active-runtime reader, process model host, assets, logger, clock, metrics, invocation executor builder, and runner identity directly; `FactoryService` and `runtimehost.Host` only retain compatibility forwarding/composition seams and are never passed into the model family. Factory worker execution routes through `pkg/models/host/execution.go` when a process-wide host is configured, otherwise through the local manager fallback. Process-wide local-runtime ownership and lease boundaries belong in `pkg/models/host`; keep `pkg/models/local` as the managed-runtime catalog compatibility projection layer. See `docs/architecture/model-host.md`.
- When a shared merge introduces a backend package-coverage floor that the reviewed head no longer reaches, use the failing CI profile's exact reported value to make the smallest manifest adjustment; do not run the manifest updater against the whole repository because it can ratchet unrelated package floors.
- `pkg/workers/mockworker/runner.go` preserves the original provider command, args,
  and worker identity in `YOU_MOCK_WORKER_*` script environment variables before a
  script mock replaces the command. Functional CLI tests can capture those values
  to prove effective provider/model selection without allowing a live provider;
  retain a behavioral output sequence as well so selection assertions do not
  replace workflow-result coverage.
- A named factory whose submitted Work fans out into derived terminal Work must define an explicit `invocationReturn` targeting the final Work type and terminal state. The default submitted-work return policy cannot follow a fan-out to a separately derived merge result.
- Structured invocation input is normalized into the submitted Work's canonical text content at `pkg/factory/sessions/invocation/session_owner.go`; `WorkRequestFromSubmitRequests` and `NormalizeWorkRequest` must preserve cloned invocation arguments so fan-out-derived Work can render the original request without relying on a transient `${input}` placeholder. Use `workPropagation.mode: PRESERVE_INPUT` plus a dedicated processing-state route when a final fan-in must consume that original Work alongside derived branch results.
