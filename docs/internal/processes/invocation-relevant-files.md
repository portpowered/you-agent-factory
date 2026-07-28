# Invocation Relevant Files

Use this map when changing factory invocation input, return-policy, or
primary-result behavior.

- Providers-owned one-attempt execution must clone and validate
  `providers.ExecuteRequest` before consulting request-time catalog readiness,
  resolve canonical IDs and accepted aliases through the same private Catalog
  authority used by List/Get, and invoke only the adapter registered for that
  canonical ID. Keep retry, fallback/default selection, scheduling, throttle,
  and Work-outcome policy above this boundary; recording fakes should prove
  validation-before-I/O, full detached request delivery, and exactly one
  adapter call. On success, clone before publishing, validate any returned
  SessionRef against the resolved provider, bound ordered progress and metadata
  deterministically, redact request-derived and sensitive native diagnostics,
  and suppress the entire result whenever the adapter returns an error.
- Compose Providers Catalog and Execution as required sibling capabilities
  behind one `providers.Service`. Validate adapter registrations at construction
  through Catalog's side-effect-free canonical identity resolver, reject aliases
  and unknown IDs before publishing the root, and reserve request-time Catalog
  lookup for live readiness/selectability so construction remains inert.
- Parent-private Runner implementations that expose subprocess progress should
  consume an injected streaming command capability, serialize publication only
  within each invocation, and build terminal diagnostics from the command
  edge's complete stdout/stderr result. Use
  `workers.ProjectCommandEnvForDiagnostics` for the shared allowlist,
  metadata-only, and sensitive-value redaction policy; never publish effective
  environment values directly.
- Compose a parent-private Runner and its detached capability metadata in the
  private Runners wire package, then publish only its common `workers.Runner`
  binding through the immutable registry. Mirror Script with
  `NewInferenceRegistry` for the inference identity, snapshot worker/resources
  at construction, and declare supported working-directory/worktree optional
  capabilities. Run the shared conformance kit against the registry-resolved
  implementation; when the implementation translates the common request into a
  narrower effect request, use the kit's boundary-specific captured-request
  assertion to prove caller-owned isolation. For inference execution-failure
  conformance, route fixture failures through Models not-handled plus an
  injected delegate that returns the normalized `ProviderError`. Cut existing
  model-inference composition over through
  `runnerswire.NewInferenceCompositionRunner` in
  `pkg/services/workers/service/runtime_options.go` instead of decorator-only
  local-model wrappers.
- A Runner implemented as a nested private service under
  `internal/services/runners/internal/services/<name>` follows the recursive
  service shape enforced by `make pkg-structure`: expose exactly one `Service`
  interface at the service root, keep implementation and construction under
  `internal/service`, and let the service-local `wire` package project that
  implementation to the parent Runners wire package. The parent must not import
  through the nested Go `internal` boundary directly.
- When a Providers-backed Runner translates the ordered progress retained in a
  detached `providers.ExecuteResult`, snapshot that result once, publish each
  progress fact synchronously through the injected Workers observation edge,
  and clone progress metadata and Provider Session correlation independently
  for every publication. Return the detached terminal result only after those
  publications so publisher mutation cannot alter later observations or the
  one terminal outcome.
- Selection-aware `you run` schema resolution belongs at the CLI read boundary:
  resolve an already-selected named Factory config path or explicit Factory
  source through the read-only Factory Definitions loader, check cancellation
  before and after each synchronous lookup, and pass only the loaded invocation
  signature into pure `climanifest.ComposeRunInputs`. Keep selection identity,
  catalog provenance, and filesystem location out of the effective schema so
  equivalent definitions produce the same downstream facts without
  materialization, sessions, provider startup, or runtime probing.
  Explicit config-root resolution must use the same injected authored-reader
  filesystem edge as config loading so `root.BuildProcess` tests can cancel or
  fail the metadata lookup itself without replacing service construction.
- Effective Factory catalog CLI projections resolve project and global roots
  from invocation-local home and working-directory edges, then pass both roots
  to the Factory Definitions operation. They must not enter system
  initialization: packaged-only entries are read from the published manifest
  and stay unmaterialized, while current markers remain a read-only
  canonical-name comparison over project-first pointer lookup.
- Selected-Factory request preparation must recheck cancellation after pure
  schema composition and after Work normalization before publishing any
  prepared result. Compose the selected Factory schema before deciding that an
  empty argv/TTY invocation has no input: an active signature can still apply
  defaults or reject missing required input, and composition can still reject
  reserved-name collisions. CLI adapters should recover Work's typed stable
  failures with `errors.As`, because the real service boundary wraps domain errors.
  Prove failure ordering through `root.BuildProcess` using Factory Session ID,
  runtime-host, Work request ID, and provider-command edges; all must remain
  untouched for composition, sensitive normalization, and cancellation
  failures.
- For a selected Factory with an active invocation signature, effective-schema
  composition replaces the static positional/stdin compatibility carrier while
  retaining run-level flags and other static inputs as reserved namespaces.
  Map the effective Factory-parameter projection into Work's preparation
  contract once, then carry the detached `PreparedInvocationInput` through the
  CLI-only Factory Session request field. Factory Sessions must consume that
  prepared value directly so canonical ordering and positional, named, stdin,
  and default provenance are not collapsed into a second structured-argument
  normalization. The same detached carrier must preserve no-signature
  compatibility input and its positional/stdin source through Factory Sessions
  without converting it back into API content for a second normalization.
  Factory Sessions validates that structured prepared input accompanies a
  signature and compatibility prepared input accompanies no signature. Public
  API requests leave the prepared field nil and retain Factory Sessions-owned
  normalization.
- A conductor-routed native integration must carry the complete cloned
  `workers.ProviderInferenceRequest` through `inferencecontract.InvocationRequest`.
  Keep provider selection and response delivery conductor-owned, while the
  provider-owned command builder receives the original environment, process
  environment, working directory, worktree, dispatch metadata, worker and
  workstation metadata, project ID, and input tokens. Prove configured
  environment and working-directory delivery through `root.BuildProcess` with
  an injected command-runner edge; do not expose configured secrets in events
  or assertion output.
- A provider command-ownership migration should move pure argv, prompt,
  environment, and dispatch assembly into the provider package before replacing
  an aggregate path that still owns effectful materialization or cleanup.
  Preserve the catalog-declared executable identity even when it differs from
  the canonical registry identity, and cut the aggregate path over only after
  its typed effects have moved with equivalent terminal-path cleanup evidence.
  Provider-owned command preparation can return an idempotent cleanup through
  `adapter.CommandBuildResult`; neutral orchestration defers it across command,
  decoder, and final-result handling. Close a temporary prompt before command
  launch, make removal safe under concurrent terminal signals, and clean an
  exact created path immediately when preparation fails before a command exists.
- A native-streaming integration should keep decode state invocation-local,
  publish each provider-neutral draft through the response writer in observed
  subprocess order, and use the parsed terminal record only for the authoritative
  completion. Guard the publication callback so the first writer failure stops
  later publication and prevents a terminal close; successful, malformed,
  failed, canceled, and timed-out paths must otherwise reach at most one close.
  Start a stable message lifecycle before its first delta and complete that same
  item from the authoritative terminal snapshot; otherwise the neutral protocol
  correctly rejects the delta before it can reach a Factory Session. The
  conductor-to-Runner destination must forward each validated draft through the
  injected `ProgressPublisher` as a canonical draft with the dispatch ID restored,
  rather than collecting only the terminal response, so the session-owned store
  remains the sole owner of public response-event identity, ordering, retention,
  and SSE delivery.
  Initialize invocation-local decode and final-parse state with a validated
  requested Provider Session, replace it only from accepted structured native
  records with valid identifiers, and carry the effective session through both
  successful and retryable failed completions; free-form, malformed, empty, and
  unsupported records must never create or replace session continuity.
  Exercise the same integration concurrently under `go test -race` so decoder
  state, writer state, and cleanup remain isolated per invocation.
- Final-only provider integrations should keep native final stdout as response
  content and derive resumable Provider Session metadata only from explicit
  structured fields that satisfy the provider's identifier contract. If a
  successful resumed invocation emits no replacement identifier, preserve the
  validated requested Provider Session; never recover identifiers from
  free-form assistant text. The product runner carries configured resume state
  in the cloned `ProviderInferenceRequest.SessionID`, while direct protocol
  callers can use `InvocationRequest.ProviderSession`; a migrated provider must
  normalize both accepted inputs before applying its emitted-session rules.
- Retryable conductor failures must preserve both retry posture and any
  Provider Session when they cross into the Worker failure contract. A neutral
  `unknown` failure with `Retryable=true` maps to retryable
  `internal_server_error`, and the next attempt must require session-resume
  capability while carrying that session ID. Wrap provider-boundary recording
  around the final decorated runner so registry-selected conductor calls and
  retained native calls each emit one canonical inference request/response
  pair; otherwise conductor failures and replacement sessions disappear from
  Factory events.

- Review-gated factories that must revise rejected work should preserve the
  original input on the work-stage route, retain non-empty worker output in the
  `_last_output` token tag, and read `Payload`, `PreviousOutput`, and
  `RejectionFeedback` from the retry prompt. This keeps the request, candidate,
  and reviewer feedback available without weakening the approval-only terminal
  return contract. Focused coverage belongs with the packaged invocation tests.
- Package tests are subject to the same `pkg-maint` cyclomatic-complexity limit
  as production code. Keep topology fixtures readable by delegating independent
  identity, routing, and validation assertions to named test helpers.
- Focused `climanifestcobra` implementation scenarios belong beside the owning
  package. Production-integration evidence must enter through
  `root.BuildProcess`; use the injected `CLIObserver` edge when the
  customer-visible contract is the detached command/input projection rather
  than a service side effect. The generic constructor's exported black-box
  conformance suite also runs in the functional lane because arbitrary
  manifests are its public input boundary; keep those cases limited to
  observable Cobra construction, parsing, help, completion, and dispatch
  behavior. Do not use source inspection or inventory scans to raise coverage.
- Manifest-owned root no-argument discovery is projected by the generic Cobra
  constructor before external persistent-source collection, resolved-input
  compatibility binding, chained pre-runs, and handler dispatch. It may
  initialize a manifest-default-only observation snapshot, but it must not read
  Operator Settings. Prove its inert behavior through `root.BuildProcess` with
  exact Operator Settings, Factory-loading/materialization, listener,
  runtime-host, provider, and browser edges, plus a compiled-binary malformed
  config case; do not substitute source-shape assertions for observable help
  output and zero effect calls.
- Implicit Current Factory execution must derive the exact
  `<invocation working directory>/factory/factory.json` path from the
  invocation-local process context and carry it as the Factory Definition
  `SourcePath`. Do not pass only the relative `factory` directory through the
  current-pointer resolver: that can depend on the host process directory or
  redirect through `.current-factory`. Keep API/dashboard activation separate
  from terminal presentation so a server-disabled batch run can still emit its
  canonical final view.
- Current Factory server activation uses that same exact source selection but
  a distinct non-bootstrapping continuous configuration. Treat the HTTP
  starter's bound callback as an internal synchronization signal: publish the
  runtime-host binding to CLI presentation and browser effects only after
  `CompleteStartup` succeeds, so startup failure cannot report false readiness
  or open a browser. Cancellation must join the starter before reverse-order
  Factory Session cleanup returns.
- Local run/server hosting carries the CLI-validated loopback host through the
  Factory Sessions host request into `platform/httpserver`; never reduce it to
  a port-only `:<port>` listener. Automatic collision fallback tries every
  higher port monotonically through `65535`, publishes the actual host and port
  from the successful binding, and exposes terminal binding exhaustion as a
  typed platform failure that the CLI maps to `SERVER_BIND_FAILED`.
- A one-shot run that owns an API listener must express its terminal
  invocation as part of the Factory Sessions lifecycle plan. Start the runtime,
  workers, and transport first; gate the terminal operation on the published
  ready binding; then let either transport failure or terminal completion
  cancel and join the other side before reverse-order cleanup. Keep the hosted
  runtime alive in service mode during that transaction without changing the
  customer-visible one-shot run mode.
- Root/global CLI inputs have one writable definition path:
  `contracts/cli/commands.json`. `climanifestcobra` generically projects those
  records into Cobra and resolved inputs; `make cli-manifest-check` compares
  the constructed production root/global inventory against those records and
  rejects extra, missing, or metadata-drifted persistent inputs regardless of
  registration source shape.
- Generic relationship presence must inspect every registered flag spelling.
  Cobra marks the canonical flag when a shorthand is used, but aliases are
  separate `pflag.Flag` records even when they share typed storage.
- Manifest-owned relationship validation may retain a legacy customer
  diagnostic through a narrow constructor presentation mapping after the
  generic validator rejects the invocation. Keep the relationship as the sole
  validation authority and prove the resolved handler or operation is not
  invoked for the conflicting input.
- Run output selectors are validated immediately after the command's custom
  manifest-backed flag parser and before Current Factory selection or
  Initializer activation. Because `you run` deliberately owns that parsing
  compatibility boundary, map output conflicts and unsupported values to their
  manifest-declared `ErrorResponse` there; JSON plus `response-stream` is the
  accepted JSON-stream mode, while quiet conflicts with global JSON and every
  explicit `--output` selection.
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
- Generated packaged Factory definitions have no per-Factory Go wrapper or
  handwritten registration package. Add authored sources under
  `packages/packaged-factories/factories/`, regenerate the publication, and let
  `internal/packagedfactorycatalog` derive membership from its manifest.
- `packages/packaged-factories.Published()` is the inert, read-only Go boundary
  for the exact `factories/`, `generated/`, and `schemas/` trees included in the
  npm package. Backend catalog consumers should read the generated manifest and
  its referenced artifacts through that filesystem instead of embedding or
  reconstructing publication bytes elsewhere; `Source()` remains the authored
  source compatibility boundary.
- `internal/packagedfactorycatalog.LoadPublishedDefinitionCatalog()` is the
  fail-closed backend projection of that publication. It validates the manifest,
  schema identity, locators, hashes, unique identities, both generated formats,
  canonical Factory mapping, and Factory Definitions validation before exposing
  detached exact JSON definitions. Use `LoadDefinitionCatalog(fs.FS)` only for
  inert fixture injection and consumer tests; lifecycle and installation effects
  remain outside the catalog.
- `pkg/wire.providePackagedFactoryDefinitions()` loads that validated catalog
  while constructing the inert process graph and supplies detached definitions
  to the existing System Initialization service. Keep packaged installation and
  non-overwrite policy in Factory Definitions and lifecycle activation in
  Initializer; the embedded publication package and catalog must not acquire
  either responsibility.
- The customer-implementable provider inference contract currently lives in
  `pkg/services/workers/provider/inferencecontract/` as migration debt. Durable
  ownership belongs to the Providers Execution leaf
  (`pkg/services/providers/execution/inferencecontract`); `cmd/pkgboundarycheck`
  encodes that ownership with deliberate fixtures, while Workers continues to
  host the live declaration until later Providers packets land. The checker
  resolves imports before classifying aliases, defined selector types, or
  interfaces embedding the canonical leaf, so local type names and valid Go
  declaration forms cannot create a second edges-owned contract. Catalog
  enumeration and one-attempt execution share one Providers-owned source of
  truth that absorbs Standardized Providers protocol/registry/open-config/testkit;
  the checker rejects competing provider catalog, registry, conductor, or
  execution-contract abstractions outside Providers and the absorbed Workers
  `provider/` migration-debt surfaces. `pkg/services/edges`
  may aggregate the exact leaf effect contract unchanged and must not redefine
  or alias it. The checker recognizes the effect by its `Infer` method
  signature rather than a local type name, resolves the standard-library
  `context.Context` parameter through normal, renamed, and dot imports, and
  resolves leaf aliases through the declaring file's imports, so unrelated
  `Provider` interfaces and aliases remain valid. Do not exempt declarations
  solely because they reuse the production aggregator type name: the AST shape
  distinguishes an allowed `Edges` struct field from an `Edges` alias, defined
  type, or interface that redeclares the leaf contract. Inspect nested field type
  expressions in every non-Providers declaration too: direct leaf fields preserve
  the imported contract, while anonymous interfaces or other wrappers create a
  locally owned redefinition even outside `edges`. Preserve the explicit Workers
  inference-contract migration-debt exception. Prove ownership behavior with
  deliberate `run()` fixtures rather than package-local source inventories.
- Providers Execution is the normalization boundary for private adapter
  failures. Adapters may return a declared `providers.ExecuteFailure` or
  parent-private lifecycle facts for native, decode, flush, and final-parse
  failures; Execution applies deadline/cancellation precedence, then declared
  classification, then deterministic final-parse/flush/decode/native precedence.
  It returns only bounded detached Providers diagnostics and never forwards a
  native error message or error type to peers. Adapter-owned finalization and
  cleanup must complete before the synchronous attempt returns. Reject an
  already terminated context before Catalog or adapter I/O, and recheck context
  termination on every Execution exit so Catalog failures and nominal adapter
  success racing with cancellation still become the typed Providers timeout or
  cancellation terminal outcome.
  Invoke implementations
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
- Provider-native Codex execution state lives below the parent-private
  Providers Execution implementation. Keep its JSONL partial-record buffer,
  item/tool correlation, progress projection, flush guard, authoritative final
  candidate, and detached session extraction invocation-local. Inject the
  native effect into the adapter registration; do not let adapter construction
  start a process or let Workers-owned request/response types cross this
  boundary. Reconcile context termination, provider-declared terminal records,
  native effect errors, decode errors, flush errors, and final-selection errors
  before returning: context wins, recognized declared failures outrank unknown
  native outcomes, and every failure returns a zero result with only bounded
  Providers-owned diagnostics.
- Provider-native Claude execution state lives below the same parent-private
  Providers Execution boundary. Keep its stream-json partial-record buffer,
  message/content-block/tool correlation, mixed text/tool progress projection,
  deferred message completion, flush guard, authoritative result record
  selection, and detached session extraction invocation-local. Classify terminal
  `result` records by subtype and bounded result text before declaring failure,
  mirroring Codex declared-record precedence over native effect errors. Inject the
  native effect into the adapter registration and keep system/control records
  out of customer-visible content; preserve native `text_delta` whitespace with
  `boundedStreamDetail` (do not `TrimSpace` stream fragments) so full-stream
  fidelity concatenation matches completed snapshots; reconcile lifecycle failures
  with the same precedence rules as other Providers-owned adapters.
- Keep reusable one-attempt conformance under the Providers-private Execution
  testkit. Build the singular Providers root around a fresh
  controllable adapter for each scenario, observe only Providers-owned
  request/result/error facts plus explicit adapter call/cleanup probes, and run
  both streaming/progress and final-only/failure-oriented implementations so
  the harness cannot encode one provider-native protocol.
- The provider-neutral invocation conductor lives in
  `pkg/services/workers/provider/conductor/`. Factory Sessions and worker
  executors should enter registry-selected integrations through that conductor
  rather than calling Discover, request-sensitive Capabilities, or Invoke
  directly. Before any of those provider I/O paths, the conductor validates
  requested capabilities against the selected integration's registry/manifest
  maximum and rejects escalation, unknown capabilities, and contradictory
  capability dependencies with deterministic symbolic
  `conductor.Rejection` diagnostics (`invariant` + offending `capability`,
  plus `requires` for dependency failures). Accepted Invoke paths compose
  `inferencecontract.ExecuteInvocation` with a conductor-owned structured
  response writer that stamps conductor correlation (`RunID` = invocation ID)
  before leaf Draft validation, preserves emission order, stops immediately on
  destination write failure, and rejects late writes or closes after close.
  Invoke wraps the orchestration destination in a terminal guard that sanitizes
  normalized failures before publication, collapses missing/contradictory
  terminals into exactly one safe failure close, and preserves destination
  write failures without publishing a competing close. Cancellation and
  deadline expiry normalize to conductor-owned canceled/timeout terminals with
  symbolic diagnostics (`invariant=canceled|timeout`) and provider-neutral
  retryability (timeout retryable, canceled not). Shared orchestration reads
  retry handoff only through `conductor.RetryHandoffFromFailure` rather than
  concrete provider switches. Factory Sessions and worker executors enter
  registry-selected integrations through the Workers-owned conductor composed
  from the same authoritative registry: `NewRuntimeWithSelection` constructs
  `conductor.New(providerRegistry)` and `runtimeRunnerDecorators` wrap the
  retained provider-native runner with `conductorInvocationRunner` when
  `ProviderOverride` is absent. Externally supplied selectable identities
  resolve onto their canonical conductor identity. Bundled built-ins remain on
  the provider-native Infer/command path until their package-owned Integration
  replaces the native-runtime compatibility stub; `UsesNativeRunner` keeps the
  stub on the native path and routes migrated Integrations (currently Gemini,
  Kiro, and Cursor)
  through the conductor without a concrete-provider switch in shared
  orchestration. Aggregate dispatch/failure branches and `ProviderOverride`
  remain intact and bypass the registry/conductor decorators. Concurrent cancel,
  overlapping dispatch, and destination write-failure/backpressure evidence lives
  in `conductor/concurrency_test.go`: cancelled closes still reject late writes,
  shared-conductor dispatch keeps per-invocation correlation/order/terminals
  isolated, and sink backpressure remains the sole terminal for the affected
  invocation without leaking unsafe provider detail into sibling successes.
  New measured conductor packages must be registered in both
  `docs/internal/baselines/go-unit-coverage-package-minimums.json` and
  `docs/internal/baselines/go-functional-coverage-package-minimums.json`;
  unit-only registration leaves `make test-functional-coverage` red.
- The authoritative manifest-to-Integration join belongs in
  `pkg/services/workers/provider/registry/`. Catalog registrations name only
  the canonical embedded identity; external registrations carry one detached
  public inference-contract manifest. Keep generated OpenAPI types out of this
  Workers domain boundary: parse the exact bytes from
  `packages/model-providers` into Workers-owned values and validate them
  against the published schemas.
  Construction must aggregate and sort normalized identity, collision,
  implementation-coverage, support-posture, and maximum-capability violations
  without calling provider discovery, request-sensitive capabilities, or
  invocation. Registry query projections must clone and canonically order
  manifest-backed collections; normalized canonical IDs and aliases share one
  fail-closed resolver. Static entry, diagnostic, and maximum-capability reads
  remain inert. Only explicit request-sensitive capability and discovery
  methods delegate to an Integration, validate the returned provider-neutral
  contract, and return canonical detached values; invocation access resolves
  the bound Integration without invoking it.
- Provider registry composition is additive and inert. `pkg/services/edges`
  aggregates `[]inferencecontract.Registration` unchanged and `edges.Merge`
  appends into a detached slice; `pkg/wire` maps those public external bindings
  into the registry and joins them with catalog-derived built-in registrations
  exactly once. The application process retains only a narrow structural
  canonical-identity projection of that same immutable registry so root-level
  embedding tests can verify composition without importing service packages
  into `pkg/initializer`. Keep `ProviderOverride` on its existing replacement
  path until provider-native execution migrates to the neutral conductor.
  Externally supplied registrations become selectable conductor identities
  through the same registry `ResolveRunnerSelection` precedence; they do not
  use the provider-native executable LookPath path. Runtime copies that rebuild
  worker executor factories (for example `WithCommandRunners`) must preserve
  registry-backed runner selection and provider-identity resolution wiring
  through `construction.Service.WithExecutionFactories` rather than constructing
  a fresh builder that drops those resolvers. When those resolvers stay wired,
  authored public provider vocabulary such as `CODEX` canonicalizes to the
  internal command identity (`codex` / `models.ProviderCodex`) before native
  Infer; packaged-quorum and other built-in smoke assertions must expect that
  canonical command, not the public enum spelling. After the Codex/Claude
  conductor cutover, functional tests that inject subprocess stdout through
  `Edges.ProviderCommandRunner` must emit provider-native JSONL
  (`tests/functional/internal/support.CodexSuccessStdout`,
  `ClaudeSuccessStdout`, or `NewShapedProviderCommandRunner`); plain-text
  stdout hangs or fails protocol validation because conductor integrations
  decode through the Providers-owned adapters. Codex progress maps
  `turn.started`/`turn.completed` to RUN lifecycle events and lowercases RUN
  payload status for REST smoke assertions. Codex and Claude conductor
  integrations must emit the authoritative `message.completed` snapshot from
  `ExecuteResult.Content`, not sanitized `ExecuteDiagnostics.Progress`
  message facts, because Providers root redacts request prompt substrings from
  diagnostic progress and the inference protocol requires completed-message
  content to agree with the terminal response. When skipping diagnostic
  `message.completed` facts, reuse the native `message_id` correlation on the
  authoritative snapshot so earlier `message.started` lifecycles still
  terminate. Fake custom Integration E2E
  proof belongs in `tests/functional/workers/inference/` (approved
  domain/subsection under `make pkg-structure`; leave legacy
  `tests/functional/providers/contract/doc.go` as the required package
  placeholder) and must register Integrations constructed inside Workers
  (for example `inferencecontract.ProgressingExternalIntegration`) rather
  than calling `inferencecontract.NewDiscovery` / `NewEventDraft` /
  `NewResponse` from the functional package.   Provider failure normalization
  (non-zero exit, auth/rate-limit/timeout distinction, and public diagnostic
  redaction) belongs in
  `tests/functional/workers/inference/failure_normalization_test.go`; drive
  command-backed failures through `root.BuildProcess` with
  `serviceedges.Edges{ProviderCommandRunner: ...}` and assert on Work,
  Factory Event, and Provider Session surfaces only. Provider and model
  selection (explicit provider/model edge routing, worker provider precedence
  over operator defaults, and unknown-provider fail-before-start validation)
  belongs in `tests/functional/workers/inference/selection_test.go`; drive
  proofs through `support.RunFactoryToCompletionWithEdgesAndObservations` or
  `support.BuildProcess` with `serviceedges.Edges{ProviderRegistrations: ...}`
  and assert on registered integration stats plus public Work outcomes only.
  Provider command-flag mapping and unsupported-flag capability rejection
  (skip-permissions policy, resolved worktree names, explicit model values, and
  pre-start unsupported-capability failures such as workstation `outputSchema`
  on Gemini) belongs in
  `tests/functional/workers/inference/flags_test.go`; drive proofs through
  `support.RunFactoryToCompletionWithEdgesAndObservations` with
  `serviceedges.Edges{ProviderCommandRunner: ...}` and assert on provider-process
  command args plus public Work outcomes only. Provider stream fidelity
  (full-stream truthful deltas and snapshots, partial-stream non-fabrication of
  missing deltas, snapshot-only completed snapshots without deltas, and final-only
  terminal messages without native-stream claims) belongs in
  `tests/functional/workers/inference/stream_fidelity_test.go`; drive proofs
  through `support.RunFactoryToCompletionWithEdgesAndResponseEvents` with
  sanitized FND-006 provider-session goldens replayed via
  `serviceedges.Edges{ProviderCommandRunner: ...}` (and OpenCode snapshot-only
  executable-locator edges when required) and assert on public Factory response
  events and Work outcomes only. Script execution-environment
  boundary proofs (declared env filtering, missing-executable public failure,
  resource-token template resolution, multi-input ordering, and worktree
  passthrough) belong in
  `tests/functional/workers/script/environment_test.go`; drive them through
  `root.BuildProcess` with `serviceedges.Edges{ScriptCommandRunner: ...}` or
  `ProviderCommandRunner` as appropriate and assert only on external command
  effects plus public Work / Factory Event outcomes. Script execution-outcome
  proofs (successful primary result, non-zero-exit failure, and cancellation
  termination) belong in
  `tests/functional/workers/script/execution_test.go`; drive them through
  `support.RunFactoryToCompletionWithEdgesAndObservations` with a replaced
  `ScriptCommandRunner` and assert on Work customer states plus dispatch
  response events via the shared `helpers_test.go` assertions. Provider process
  and companion cleanup (timeout process-tree termination, cancellation
  companion teardown, and success-path process/stream closure) belongs in
  `tests/functional/workers/inference/process_cleanup_test.go` with
  functionallong companion coverage in `process_cleanup_long_test.go`; drive
  script-backed subprocess proofs through `root.BuildProcess` with
  `serviceedges.Edges{ScriptCommandRunner: platformprocess.NewExecCommandRunner(...)}`
  and assert on Work, Factory Event, and dispatch outcomes only.
- Wire supplies that same registry to the Workers runtime for routed provider
  selection, conductor composition, manifest-maximum capability checks, and
  executable-prerequisite preflight, and to Factory Sessions through the narrow
  `ProviderIdentityResolver` opening contract. After operator defaults are
  applied, Factory opening resolves concrete worker and guard selections to
  canonical registry identities; operator-file defaults and JavaScript worker
  presets use the same authority. Leave declared invocation interpolation
  expressions unresolved at this stage. Do not restore built-in membership or
  alias lists in Factory Runtime or operator-default helpers.
  Preserve the existing selection precedence and native runner IDs for bundled
  providers; the registry resolves canonical IDs and published aliases first,
  with the legacy `cursor-cli` runner ID mapped only at the native-execution
  compatibility boundary. Externally supplied integrations retain their
  canonical provider identity during runner selection so opening can validate
  and carry them onto the provider-neutral conductor without pretending they
  are a bundled native runner.
  Preserve accepted public model-provider aliases
  (`openai` and `anthropic`) as collision-validated registry identity claims so
  static lookup and routed selection cannot disagree. Carry the registry's
  canonical legacy-provider selection through the workstation boundary into
  the final `ProviderInferenceRequest.ModelProvider`; restoring the authored
  alias there makes provider command behavior disagree with registry lookup.
  Defer unresolved authored `${...}` provider templates to the existing
  default-selection path.
  Other unknown, catalog-only, and not-supported identities fail before
  dispatch instead of falling through to the default Codex runner.
  After invocation interpolation, the Workers workstation executor resolves
  the concrete `modelProvider` independently through the registry-backed
  `ProviderIdentityResolver` before applying runner precedence. Do not rely on
  `ResolveRunnerSelection` alone for this check: an explicit workstation or
  Factory runner wins precedence and would otherwise mask a malformed,
  unknown, or non-selectable interpolated provider value. The identity
  resolver also carries the registry's canonical identity into the execution
  request before worktree preparation, capability checks, discovery, or
  provider invocation.
  Provider-native command construction and `ProviderOverride` execution remain
  unchanged; the manifest-backed capability guard is bypassed for an explicit
  replacement provider because that edge owns its own test/runtime contract.

## CLI run and submit command contracts

- Portable invocation selection carries two distinct values: the concrete
  authored source selected by `--factory` and its asset directory. Keep the
  source path through `runconfig.Config`, `factorysessions.InvocationTarget`,
  and `factorydefinitions.RuntimeOpeningRequest`; resolving only the directory
  can silently replace an explicit YAML selection with `factory.json` or lose
  strict missing/ambiguity diagnostics. Named and default runtime selection
  continue to resolve through the current-Factory directory policy.
- `pkg/services/factory_definitions/loading/loader.go` must retain the complete
  `AuthoredFactorySource` after root selection and wrap every source-backed
  mapping, validation, portability, discovery, merge, and normalization failure
  with its selected path and format using `%w`. The CLI formatter in
  `pkg/transports/cli/factoryload/operator_error.go` must preserve that outer
  context when rendering typed blocking findings and recovery guidance.
  Behavioral coverage belongs in Factory Definitions loading tests and a
  `root.BuildProcess` functional invocation, with assertions for source
  identity, original validation targets, and zero provider execution.
- Canonical metadata for `you.run`, `you.submit`, and `you.submit.batch` lives
  in `contracts/cli/commands.json`. Keep positional cardinality, stdin channels,
  source precedence, conflicts, no-option defaults, output modes, effects, and
  stable handler/OpenAPI bindings aligned with the typed execution adapters in
  `pkg/transports/cli/root_work.go` and
  `pkg/transports/cli/root_submit_batch.go`. Local flag help also belongs in
  the manifest; do not construct or scrape a secondary Cobra tree to supply it.
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
- Peer-facing Factory Visualization root contracts live on the singular
  `factory_visualization.Root` in
  `pkg/services/factory_visualization/root_contract.go`: request-activated
  lifecycle (`Activate` / `Join` / `StopDrain`), live projection (`Observe`),
  and presentation/drain (`OpenPresentation` / `PresentProgress` /
  `FinalizePresentation` / `ClosePresentation`). Peers use plain
  request/result/typed-error vocabulary only; transports must not treat
  `io.Writer`, queue capacity, backpressure, or final-write ordering as their
  policy source of truth on that seam. Characterization proof:
  `TestRootContractInvariants_AllSlicesThroughSingularRoot`.
- Shared ordered output serialization and final-once terminal write helpers
  (transport-shaped, non-authority):
  `pkg/services/factory_visualization/factory_event_stream.go`,
  `pkg/services/factory_visualization/response_presentation.go`
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
  status in `tests/functional/acceptance/invalid_quiet_outcomes_test.go`;
  stdout, stderr, and event-presentation behavior belongs to the canonical raw
  packages above, with terminal failure stream contracts owned by
  `tests/functional/transport/cli/process/stdout_stderr_test.go`.

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
  Compare transport parity through canonical values and semantic provenance
  (explicit versus default) because physical CLI and structured API source
  labels intentionally differ. Validation diagnostics for sensitive parameters
  must retain the stable error code and parameter identity while replacing the
  rejected value with the Work-owned redaction marker.
- JavaScript named-factory lookup carries the authored `argsSchema` and `defaultPolicy` through `pkg/orchestrators/javascript/source/` into `pkg/factory/sessions/execution/PrepareStart`. Validate resolved arguments before runtime execution and resolve policy with that default; `workflowruntime.Request.ArgsSchema` preserves the same no-side-effect guard for direct runtime callers.
- `WORKFLOW_FILE` JavaScript factories with authored `defaultPolicy` also attach an `InlineWorkflow` overlay in `pkg/services/factory_sessions/internal/runtimeopening/invocation/operation.go` so durable execution receives the same policy defaults as inline workflows.
- JavaScript one-shot invocation maps terminal failures in `javaScriptInvocationResult` (`pkg/services/factory_sessions/internal/runtimeopening/invocation/operation.go`). When `GetResult` final-mode projection returns `UNAVAILABLE` without `Failure`, fall back to `GetSession().Failure` so policy/runtime denials surface the actionable message on CLI/API invocation responses instead of the generic "did not produce a final result" placeholder.
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
- `pkg/transports/mapping/factoryconfig/openapi_factory.go` must preserve exact
  `${parameter}` placeholders on authored fields that support invocation
  interpolation (for example `workers[].modelProvider`) instead of forcing them
  through concrete provider-identity validation at the JSON boundary. Keep the
  exact-placeholder pattern aligned with the accepted OpenAPI one-of. Concrete
  provider values use the open `ProviderIdentity` syntax contract; built-in
  aliases canonicalize through compatibility mapping, while syntactically valid
  extension identities remain unchanged for registry selection.
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
- Factory Sessions production Work imports are sealed by
  `pkg/services/factory_sessions/work_import_boundary_test.go`
  (`TestProductionPackagesImportWorkRootOnly`). Mirror
  `pkg/services/recordings/runtime_import_boundary_test.go` when adding similar
  CUT consumer-edge import proofs: `go list` every package under the Sessions
  root and fail on any import outside `pkg/services/work`.
- Work owner-local Wire at `pkg/services/work/wire` must stay registered under
  destination `work` in
  `docs/internal/packaged-service-structure/package-target-manifest.json`,
  `docs/internal/baselines/ownership-inventory.json`, and both
  `go-*-coverage-package-minimums.json` baselines; prove registration with
  `wire/manifest_registration_test.go` rather than re-editing manifests when
  IMP-WORK already landed the rows. The Work CLI adapter at
  `pkg/services/work/transports/cli` must stay registered under destination
  `work` in the same shared manifests; prove registration with
  `transports/cli/manifest_registration_test.go` rather than re-editing
  manifests when CLI-WORK already landed the rows.
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
- Root and persistent/global CLI metadata is authored only on the `you` record
  in `contracts/cli/commands.json`. Root flags use the canonical typed-input
  shape with stable handler/source bindings plus manifest-owned usage,
  sensitivity, completion, and lifecycle metadata. `rootLifecycle` declares
  the no-argument help intent and the stable ownership boundaries for root
  help, `you init`, `you run`, and the global `--server` input; it does not move
  executable lifecycle policy out of command handlers or Initializer. Run
  `make cli-manifest-generate` after changing this contract so generated Cobra
  metadata stays current.
- Canonical default-path ownership for operator config
  (`~/.you-agent-factory/config.json`) and generated live replay recording roots
  (`~/.you-agent-factory/recordings/...`) belongs in `pkg/config/defaultpaths`;
  `pkg/services/operator_settings` and `pkg/transports/cli/run` should keep only precedence,
  filename, and reporting behavior around those defaults.
- Effective operator-settings resolution belongs in the parent-private owner at
  `pkg/services/operator_settings/internal/services/resolution`; the published
  `operatorsettings.Service` root delegates `ResolveEffective` through
  `internal/service` and `wire` only. Query the accepted `providers.Service` root
  for provider identity canonicalization at resolve time; do not cache availability
  in settings state or add Providers→Operator Settings imports.
- Operator Settings owner-local Wire at `pkg/services/operator_settings/wire`
  must stay registered under destination `operator_settings` in
  `docs/internal/packaged-service-structure/package-target-manifest.json`,
  `docs/internal/baselines/ownership-inventory.json`, and both
  `go-*-coverage-package-minimums.json` baselines; prove registration with
  `wire/manifest_registration_test.go` rather than re-editing manifests when
  IMP-SET already landed the rows.
- When global named-factory guidance changes, update its authored
  `contracts/cli/commands.json` records and the task-oriented guidance in
  `docs/reference/authoring-factories.md` plus `config.md`; do not restore
  handwritten command help in `root_factory.go` or `root_work.go`. Run
  `make cli-manifest-generate` and `make contracts-generate` for the executable
  and packaged projections, then update intentional CLI baselines and run
  `make docs-reference-smoke`.
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
  boundary after write, sync, close, permission, and cancellation checks. Hold
  one explicit persistence lock across the complete read-merge-replace
  transaction, and share that lock between service copies and standalone reads,
  so concurrent partial updates preserve each other's fields and overlapping
  replacements remain deterministic on platforms with sharing violations.
  Failed attempts remove only their own temporary artifact and never rewrite
  the committed destination directly.
  Prompted setup should use a write-free function contract that receives the
  current semantic defaults, maps EOF to an explicit cancellation outcome, and
  delegates successful input to the same context-aware load/merge/persist
  operation used by pre-supplied values.
- The Operator Settings CLI adapter at
  `pkg/services/operator_settings/transports/cli` must stay registered under
  destination `operator_settings` in
  `docs/internal/packaged-service-structure/package-target-manifest.json`,
  `docs/internal/baselines/ownership-inventory.json`, and both
  `go-*-coverage-package-minimums.json` baselines; prove registration with
  `transports/cli/manifest_registration_test.go` rather than re-editing manifests
  when CLI-SET already landed the rows.
- Public provider/model setup enters through the manifest-derived `you init`
  handler in `pkg/transports/cli/commandregistry`, which translates stable
  `you.init.flag.provider` and `you.init.flag.model` inputs into the narrow
  `pkg/transports/cli/initsetup.Config` request. Packaged Factory installation
  enters through the same handler when `you.init.flag.package` is supplied,
  delegating to `pkg/services/factory_definitions/transports/cli` and the shared
  `InstallPackagedFactory` operation. The initsetup adapter owns
  home-to-config-path translation, prompt rendering, and human output. Product
  portability evidence for init-materialized packaged Factories belongs in
  `tests/functional/product/packaged_factory_portability`: initialize through
  `you init --package`, assert restored split-layout assets and portable paths,
  then invoke from an unrelated working directory outside the repository.
  Functional API-server fixtures that materialize YAML/YML authored roots must
  set `support.FunctionalAPIServerConfig.FactoryConfigPath` instead of relying
  on `--dir`, because `--factory` and `--dir` are mutually exclusive at the CLI
  boundary. Enable
  prompts only from the invocation-local stdin/stdout TTY classifications on
  the process context, pass Cobra's invocation-local input/output streams, and
  preserve cancellation from that same context; do not inspect host streams in
  the transport. The prompt must collect every value without writing before it
  delegates to the prompted settings operation, while
  `ConfigDocumentService` retains provider-catalog validation, semantic merge,
  unrelated-field preservation, and the atomic commit.
- Normal executable commands run the exact
  `application.SystemInitializationOperation` through `pkg/initializer` before
  their handler or lifecycle opens. Wire adapts
  `pkg/services/system_initialization.Service` to that role; fresh homes create
  operator config and materialize packaged/default Factories, while reruns
  validate and preserve customer-owned files. Bootstrap workflow ownership stays
  in `pkg/services/system_initialization/internal/workflow` and must not import
  `pkg/initializer`, transports, `pkg/wire`, or Settings/Definitions store
  surfaces such as `factory_definitions/packagedinstallation`; boundary proof
  lives in `internal/workflow/boundary_test.go` and `wire/boundary_test.go`.
  Bootstrap owner-local Wire at `pkg/services/system_initialization/wire` must
  stay registered under destination `system_initialization` in
  `docs/internal/packaged-service-structure/package-target-manifest.json`,
  `docs/internal/baselines/ownership-inventory.json`, and both
  `go-*-coverage-package-minimums.json` baselines; prove registration with
  `wire/manifest_registration_test.go` rather than re-editing manifests when
  IMP-BOOT already landed the rows. The Bootstrap CLI adapter at
  `pkg/services/system_initialization/transports/cli` must stay registered under
  destination `system_initialization` in the same shared manifests; prove
  registration with `transports/cli/manifest_registration_test.go` rather than
  re-editing manifests when CLI-BOOT already landed the rows. Bare root/help, invalid commands,
  and `you init` do not activate system initialization: `you init` owns only the
  atomic provider/model settings update. The retired `you config init` command,
  its CLI renderer, and installer invocation must remain absent. Root-built
  replacement/retirement evidence lives in
  `tests/functional/product/init_setup/init_setup_test.go`; installer behavior
  lives in `tests/release/install_script_test.go` and
  `scripts/release/smoke-install.{sh,ps1}`.
- Root-built functional fixtures that execute initializer-owned commands must
  use an invocation-local HOME/USERPROFILE and explicit Factory test data.
  Environment overrides follow last-value-wins process semantics. Do not
  bootstrap fixtures through the retired `you init --type`/`--executor` scaffold
  path or let
  parallel packages share one mutable customer home.
- JavaScript packaged factories keep authored workflow files in the package
  definition's `scripts/` assets and assemble them through
  `pkg/factory/packages/packageassets`. Their `sourceRef` must use the
  corresponding materialized `scripts/...` path, which normal initializer
  startup installs as editable factory files.
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
  Canonical command adapters that still need Cobra-owned streams use
  `GenericBindings.ResolvedCobraHandlers`: local arguments and flags are
  resolved into `resolvedinput.Inputs` with explicit CLI/default provenance in
  `pkg/transports/cli/climanifestcobra/bindings.go`, and inherited root inputs
  arrive as a separate resolved snapshot. Keep raw argument slices and public
  spellings out of these adapters.
  For an incremental family migration, project the canonical root, family
  parent, and completed leaves as one temporary generic tree, detach the family
  parent, and attach only still-unmigrated leaves through their narrow legacy
  registry. This preserves inherited root resolution after production root
  composition without forcing later behavioral slices into the current change;
  remove each legacy entry as its leaf gains a resolved handler.
  The fully migrated factory/config/init family follows the same detached-tree
  pattern in `factory_config_init_constructor.go`, with every local input
  consumed through `ResolvedCobraHandlers` and no mutable flag-binding table.
  Families whose established parent commands reject arbitrary trailing tokens
  should opt into `GenericBindings.GuardUnknownSubcommands`; keep that
  compatibility behavior in the generic projector instead of reintroducing
  family-owned flag parsing or public-name dispatch.
  The Session-family production seam loads
  `generated.SessionFamilyManifest`, constructs
  `commandregistry.NewSessionResolvedRegistry`, and adds every runnable record
  to the representative root's `GenericBindings.ResolvedCobraHandlers` by the
  record's manifest-declared `Handler.ID`. Each invocation maps fresh local and
  inherited `resolvedinput.Inputs` into the injected Factory Sessions
  operations; `sessionListPrepare(options)` and `diagnostics.writer` remain the
  injected `PrepareList` and `Diagnostics` roles. The former
  `SessionFamilyBindings`, command-ID registry, mutable input-binding table,
  handwritten Session `RunE` adapters, flag-usage patching, and duplicate
  representative `session show` handler have been removed. Preserve this
  split when extending the family: the representative registry owns only the
  root lifecycle handler, while Session execution identity comes exclusively
  from the Session manifest handler IDs.
  Validate the complete input and inheritance plan before registering any pflag
  values, and register inherited records against their persistent ancestor's
  canonical storage rather than allocating command-local copies.
  The same constructor plans positional inputs in position order, rejects gaps
  and non-terminal variadics before Cobra mutation, and records parsed typed
  values on the invocation-local command for stable-ID handler access.
  Relationship evaluation uses stable flag or argument references and explicit
  CLI presence, runs in Cobra's pre-handler phase, and reports public input
  spellings without exposing input values. Deduplicate relationship participants
  by effective semantic identity so an inherited record and its persistent
  ancestor cannot count as separate inputs. Attach the complete Cobra hierarchy
  before projecting flag-group annotations so inherited persistent flags are
  resolvable without a construction panic. Only project Cobra group annotations
  when every participant is a flag locally owned by the relationship command;
  persistent, inherited, and ancestor flags share `pflag.Flag` objects, so
  command-owned relationships involving them must rely on the generic
  pre-handler enforcement without mutating shared annotations.
  Generic help, lifecycle, and completion projection lives beside the docs
  transport boundary in
  `pkg/transports/cli/climanifestcobra/docs_constructor.go`. Validate command
  and flag lifecycle records, positional lifecycle records when authored, and
  every completion mode before Cobra mutation. Static completion consumes
  declared enum choices; dynamic completion callbacks are supplied through
  `GenericBindings.Completions` keyed by stable input ID, never by a public flag
  name or command path. Use the same cardinality-allocation helper for parsing
  and positional completion so later required inputs reserve their tokens in
  both paths. Derive positional usage labels from ordered argument records,
  render declared aliases in canonical flag help, and reject sibling
  command-name/alias collisions before creating Cobra commands; Cobra otherwise
  resolves the first matching sibling and can silently dispatch the wrong stable
  handler.
  The detached docs/models family wrappers live in
  `pkg/transports/cli/climanifestcobra/models_constructor.go`; `you docs`
  projects a root/docs subset through the generic constructor and then detaches
  the docs command for composition into the production root. Its static topic
  completion must use the authored argument enum, while the packaged docs
  operation retains its established unsupported-topic diagnostic.
  Treat argument `doubleDash: terminates-flags` as the Cobra-compatible mode and
  fail construction for missing, unknown, or currently unrepresentable modes
  instead of accepting changed parsing semantics. Hidden positional inputs
  remain part of cardinality allocation and parsing but are omitted from
  generated usage/help. Before projection, require each input to be one complete
  compatibility or canonical record, validate its source vocabulary and stable
  handler binding, and require `defaultValue` presence to exactly match the
  canonical `manifest-default` source. For canonical inputs, also reconcile
  every input-owned binding ID with the command's `handlerBindings`, require
  explicit `sourceBindings` for accepted external sources, and validate the
  complete canonical precedence policy before allocating Cobra state. Repeated,
  fixed-multiple, and unbounded cardinality use `stringArray`; reject repeated
  scalar types and typed defaults outside the declared range. Apply
  `lowercase`, `trim`, and `lowercase-trim` to explicit string and string-array
  values, while requiring authored defaults and enum choices to already be in
  their declared normalized form. Validate relationship records both
  individually and as one command-owned set so equivalent duplicates,
  contradictions, and directed dependency cycles fail during planning rather
  than reaching pre-handler evaluation.
  Generic runnable dispatch follows the same boundary: validate every
  `Command.Handler.ID` and `GenericBindings.Handlers` entry before Cobra
  projection, reject duplicate handler ownership, and invoke the selected
  stable-ID handler with a detached normalized `InputValues` snapshot. Public
  command paths and aliases must not participate in executable lookup.
  The detached Models family projection in
  `pkg/transports/cli/climanifestcobra/models_constructor.go` uses this resolved
  handler boundary for every leaf. Model-name positionals and invoke-local
  operation, text, output, and compatibility-port flags are canonical inputs;
  adapters consume their typed local snapshot plus the inherited root snapshot
  and must not fall back to Cobra arguments or mutable flag targets.
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
  The injected `CLIObserver` edge parses and validates the selected command,
  runs only the root persistent-input resolution boundary, and serializes the
  resulting stable-ID observations without dispatching the selected handler.
  Keep its `ResolvedInputsJSON` projection detached and redaction-safe, and
  refresh after compatibility parsing for commands that disable Cobra parsing.
  Static-plus-Factory composition is owned by
  `pkg/transports/cli/climanifest.ComposeRunInputs`: pass the validated `you.run`
  command and only the selected Factory's `InvocationSignatureConfig`. The pure
  projection keeps manifest inputs separate from dynamic Factory parameters,
  detaches all returned collections, and exposes canonical/preferred names,
  default shape, normalized value mode, cardinality/consumption, type hints,
  sensitivity, and bindings without requiring downstream adapters to reinterpret
  the authored signature. It
  rejects command-name, long-name, alias, shorthand, positional, stdin-owner,
  and stable-binding collisions with sorted diagnostics that identify both
  owners. Check canonical parameter names, preferred external names, and every
  alias independently after applying the shared Work normalization trimming;
  do not limit reserved-spelling checks to parameters with named CLI bindings.
  Named and explicit-file selectors must not enter this composition
  policy; equivalent selected signatures produce equivalent results.
  Shell-completion adapters must check invocation cancellation before and
  after effective-catalog discovery and after schema composition so a
  dependency that returns partial data after cancellation cannot expose
  candidates or accidentally select compatibility fallback.
  Cobra's generated PowerShell completer does not delegate an empty default
  result to native file completion. Keep `FILE_PATH` projection detached and
  candidate-free, then patch the generated PowerShell boundary to enumerate
  the entered local path in the shell; prove separate and inline named-value
  forms against a built executable so a raw default directive cannot mask a
  null `CompletionResult`.
  Preserve a nil selected signature as explicit compatibility mode rather than
  replacing it with an empty active signature: compatibility mode exposes only
  static inputs and delegates positional text, stdin, and API content to the
  Work-owned compatibility normalizer. It must not synthesize Factory
  parameters, unknown-named policy, defaults, validation, help, or completion
  facts, and both CLI-shaped and API-structured named inputs must fail instead
  of being ignored.
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
  `VerifyRepresentativeRunnableCoverage`) and production wiring uses
  `newRepresentativeHandlerRegistry` in
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
  been removed. Session, work, and submit local flag help is projected directly
  from each manifest flag's `usage`; transport binding structs carry typed
  values and operations, not presentation fallbacks.
- The canonical detached Work-family seam is
  `climanifestcobra.NewResolvedWorkCommand`; focused execution can use
  `NewResolvedWorkCommandTree` or
  `NewResolvedWorkCommandTreeFromManifest`. The generic manifest-to-Cobra
  constructor owns public flags, positional arguments, aliases, defaults,
  cardinality, and stable handler attachment. `commandregistry.ResolvedWorkHandlers`
  supplies `ResolvedListRunE`, `ResolvedShowRunE`, `ResolvedMoveRunE`, and
  `ResolvedVisualizeRunE`; each adapter reads invocation-local
  `resolvedinput.Inputs` and creates one fresh `work.ListConfig`,
  `work.ShowConfig`, `work.MoveConfig`, or `work.VisualizeConfig`. Do not add
  Work-specific flag registration, argument validation, live config pointers,
  or RunE binding structs to this canonical path.
- `pkg/transports/cli/root_work.go` remains on the explicitly deprecated
  `NewWorkFamilyCommand` / `NewWorkRegistry` compatibility path while its
  composition lease is active. The exact post-lease integration is: replace
  `productionWorkCommand` and `newWorkHandlerRegistry` composition with
  `climanifestcobra.NewResolvedWorkCommand` plus a
  `commandregistry.ResolvedWorkHandlers` value built from the four resolved
  adapters; run root-level CLI parity and `root.BuildProcess` tests; then delete
  `newWorkFamilyBindings`, the legacy Work registry and live-binding constructor
  path (`WorkFamilyBindings`, `WorkFamilyComponents`, `NewWorkFamilyCommand*`,
  and `NewWorkFamilyComponents*`), and any superseded handwritten Work
  constructors. Do not perform only part of this deletion before the root swap
  because the current production command still consumes those compatibility
  symbols.
- Whole-production CLI closure is checked by `pkg/transports/cli/clicontract`
  and exposed through `cmd/clicontractsmoke` / `make cli-contract-smoke`.
  The same behavioral smoke is part of `make cli-manifest-check` and compares
  the constructed root persistent-flag spelling and metadata set against the
  authored manifest, so duplicate registration is rejected independently of
  the Go source shape or helper used to register it.
  Keep deliberate smoke violations snapshot-only: they must use the production
  validator and diagnostics without executing commands, invoking services,
  mutating Cobra state, or requiring network access.
- Operator default worker model settings resolve at the CLI/process boundary in
  `pkg/transports/cli/root.go`: the injected Operator Settings config loader
  supplies raw `operator-config` candidates to manifest-owned global input
  resolution, while `resolveOperatorDefaults` consumes those resolved values
  for legacy run behavior. The effective defaults flow through
  `run.RunConfig.OperatorDefaults` into `service.FactoryServiceConfig` before
  `cmd/factory/compose.InjectCLITransport`; Wire providers must not read
  `~/.you-agent-factory/config.json` or `YOU_DEFAULT_WORKER_MODEL_*` directly.
- Process startup follows `cmd/factory -> pkg/root.BuildProcess -> pkg/wire.InjectBundle -> application.Process.Execute -> CLI-selected initializer -> pkg/initializer`. Production and functional tests construct the same reusable process through `BuildProcess`; production supplies empty edges while functional tests replace explicit external boundaries. Every `Execute` call constructs a fresh command tree from invocation-local input. Only after CLI parsing does the matching `Run` or `Stdio` initializer construct its service subtree. There is no generic construction request, alternate production injector, root service-splicing path, or `ProcessGraph`. Keep domain construction out of root and initializer, do not restore root-local lifecycle closures or process-global builder registration, and never construct HTTP/dashboard resources for stdio or an MCP stdio transport for run/API. The normalized invocation home remains authoritative through config initialization, named-factory lookup, `run.RunConfig.HomeDir`, persistence, recording, runtime logging, and metrics. `pkg/initializer` only starts, joins, unwinds, and closes the selected bundle. Boundary coverage lives in   `pkg/root/root_test.go`, `pkg/root/edges_override_compatibility_test.go` (typed `edges.Edges` override versus empty-default replacement through `BuildProcess` + post-construction `Execute`), `pkg/wire/cli_test.go`, initializer application tests, functional CLI tests, and the compiled-binary matrix in `tests/release/root_process_smoke_test.go`. Wire projects that process-edge bag into `runtimeopening.ExternalEffects` before Factory Session applicationopening, runtimeopening, invocation, and executionopening implementations; those packages must not import `pkg/services/edges`. `cmd/pkgboundarycheck` rejects constructed-service production imports or `edges.Edges` fields/parameters under `pkg/services/**` (except `pkg/services/edges` itself) and points maintainers to exact-port injection at `pkg/wire` / `root.BuildProcess` rather than deleting the process-edge aggregator. Package docs in pkg/services/edges and docs/internal/standards/code/general-backend-standards.md keep Edges documented as that process-edge architecture exception for root/Wire construction and functional overrides—not a service locator or Initializer dependency bag—while ownership tests continue to prove it aggregates leaf effect contracts via Merge.
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
  runner. When migrating a built-in out of aggregate `provider_behavior`, move
  argv construction, optional-capability rejection, and
  `BuildCommandRequest`/env assembly into `pkg/services/workers/provider/<name>`
  first (see Gemini `BuildArgs`/`BuildCommandRequest`/`Adapter.BuildCommand`);
  keep only a thin aggregate delegate until the later legacy-branch deletion
  story. After the migrated provider is registry+conductor exclusive, delete
  only that provider's corresponding aggregate command/decode/failure/timeout/
  session branches (and relocate aggregate-owned tests into the provider
  package); leave the aggregate shell, ProviderOverride, and other providers
  intact. Move provider-native failure and timeout parsing into the same package
  (`ParseProviderFailure`, `TimeoutFailureResult`, `Adapter.ClassifyFailure`) so
  the conductor path consumes provider-owned normalized facts; aggregate exit
  and timeout bridges may only thin-delegate until legacy deletion. Treat all
  provider-supplied failure text as classification input only: published
  failures use fixed class-specific messages, including unknown fallbacks, so
  unmarked prompts, credentials, and machine-local paths cannot escape a
  deny-list sanitizer. If a provider command returns `context.Canceled`, return
  that error before closing the provider response writer so
  `inferencecontract.ExecuteInvocation` remains the single owner of canonical,
  non-retryable cancellation. Bind the
  migrated providers as registry catalog Integrations
  (`gemini.NewIntegration`, `kiro.NewIntegration`, `cursor.NewIntegration`) from
  `BuiltInRegistrations`, and let
  `UsesNativeRunner` route Integrations that no longer advertise the
  native-runtime compatibility marker through `conductor.Invoke` without adding
  a concrete-provider switch in shared orchestration. Process composition
  passes the shared `ProviderCommandRunner` edge into
  `BuiltInRegistrations(BuiltInDependencies{CommandRunner})` so migrated
  Integrations and native executors share one command boundary. Cursor also
  receives the resolved Workers operating system and exact
  `WorkersProviderTemporaryFileSystem` edge through those dependencies; its
  provider-scoped root-process functional tests replace only those two typed
  effects and prove that every created oversized-prompt file is closed and
  removed on success and adverse outcomes. Worker
  construction resolves persisted plus invocation-override permission policy
  once, then an outer invocation-policy runner records the effective value on
  `ProviderInferenceRequest`; this outer boundary must wrap the conductor
  decorator because a migrated Integration does not call the retained native
  runner. Provider Integrations consume that request-local value when building
  commands and must not store worker permission policy in registry-global
  Integration instances. Functional provider packages under
  `tests/functional/providers/<name>` prove success, command policy, and safe
  native-failure postures through `root.BuildProcess` /
  `support.RunFactoryToCompletionWithEdges` without importing provider package
  internals. `providers` is an approved deep functional domain; aggregate files
  directly under `tests/functional/providers` remain shallow deletion-only
  debt. Prove each migrated
  Integration against the shared inference contract through
  `inferencecontract.ExecuteInvocation` for the success and failure postures
  that apply to that provider's authored support/capability set (for Gemini:
  prompt_submission + message_snapshots success; for Kiro:
  prompt_submission + session_resume + message_snapshots success; plus each
  provider's native auth/invalid/throttle/timeout/unknown failures). Do not
  invent streaming or tool-lifecycle factories just to call
  `inferencecontract/testkit.Run` when the manifest does not advertise those
  capabilities; keep selection on the registry Integration boundary rather
  than Adapter internals. Native
  JSONL fixture tests should
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
  normalization and Codex reporting-path agreement probes.
  Provider-owned failure classifiers may inspect bounded native stdout/stderr
  to select a stable category, but customer-visible messages and terminal
  drafts must come from category-specific product guidance rather than native
  excerpts. Keep the original command surfaces only in internal diagnostics,
  give context or command-boundary deadlines timeout precedence, and propagate
  cancellation to the inference protocol so its validating writer creates the
  single canonical canceled completion.
  A streaming decoder must hold
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
  invocation failure stderr diagnostics, non-zero exit status, and absence of a
  false primary result on stdout are asserted in
  `tests/functional/transport/cli/process/stdout_stderr_test.go`. Presentation bytes and
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
- `packages/packaged-factories/factories/` owns authored Factory sources and
  `internal/packagedfactorycatalog` owns manifest-derived backend lookup;
  `pkg/services/factory_definitions` owns public-name selection and the
  catalog-to-disk installation boundary. Named
  resolution in `pkg/config/layout.go` reads project-local then global disk
  state only; it does not install packages or expose compatibility JSON aliases.
  `pkg/services/factory_definitions/packages/packageassets` is the shared,
  side-effect-free packaged
  asset assembly entry point: package owners supply the authored `factory.json`
  and an explicit embedded `fs.FS`, and definitions call this assembler before
  their payload enters the catalog. It delegates prompt declarations to
  `pkg/services/factory_definitions/packages/promptassets` and discovers regular
  UTF-8 `scripts/**`, `docs/**`, and `inputs/**` assets as deterministic
  `SCRIPT`, `DOC`, and `INPUT` bundled files at matching `factory/**` targets.
  Discovery rejects non-regular, unreadable, or invalid UTF-8 assets, and
  assembly rejects unsafe or duplicate canonical bundled targets before the
  payload can reach config initialization. The assembler attaches exact asset
  bytes but does not install or persist anything.
  `pkg/services/factory_definitions/packagedinstallation` passes each selected
  detached catalog payload through the injected Factory Definitions
  `Persistence` boundary. Bootstrap and customer selection share this
  materializer. The prepared layout carries an explicit safe root filename so
  an accepted JSON, YAML, or YML selection writes exactly one `factory.json`,
  `factory.yaml`, or `factory.yml` while the omitted/default selection remains
  JSON. That shared persistence path materializes `SCRIPT` entries at mode
  `0755`, writes only thin UTF-8 bundled-file metadata to the selected root
  definition, and validates the staged runtime before publishing the
  named-factory directory. Existing valid package directories are loaded
  read-only and skipped as a whole when the requested authored-root format
  matches the committed layout, so later init runs do not normalize
  permissions or replace operator-edited scripts. Alternate format selection
  without explicit replacement returns `ErrNamedFactoryAlreadyExists`, while
  accepted replacement uses the same atomic `ReplaceNamedFactory` path and
  reports a `replaced` outcome. Package selection combined with scaffold-specific
  inputs is rejected by `ValidateInstallPackagedFactoryRequest` before catalog
  lookup or filesystem effects begin.
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
  `packages/packaged-factories/factories/goal/` owns the authored factory and concise
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
- `packages/packaged-factories/factories/subagent/` owns the authored
  `@you/subagent` one-pass Factory scaffold (`factory.json`, prompt files), and
  the generated manifest registers it for every package consumer. The topology uses exactly one `AGENT_WORKER`
  with explicit `agentTools.policy` and one `AGENT_RUN` workstation that interpolates
  `${input}` from the invocation signature into the workstation prompt body.
  normal initializer startup installs `@you/subagent` under the global
  named-factory root before named invocation can resolve it.
- `pkg/services/factory_definitions/packages/subagent/` retains only packaged
  subagent metadata and response-shaping behavior; shared catalog validation,
  installation tests, and public functional outcomes own definition evidence.
- Packaged `@you/subagent` invocation functional coverage lives in
  `tests/functional/factory/packaged/subagent/invocation_test.go` for child
  primary-result return, child Factory Response Event streaming, and stable child
  failure through public CLI/API boundaries with mock workers. The mapped
  `test-built-cli-acceptance` specialty binding for
  `TestSubagentInvocation_SuccessfulNamedRun_ReturnsAuthoritativePrimaryResultJSON`
  remains in `tests/functional/acceptance/invoke_repeat_subagent_outcomes_test.go`.
- Hermetic no-server named `@you/subagent` package proof also lives in
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
- `tests/functional/transport/cli/output/text_stream_test.go` holds focused
  regression coverage for adjacent `you run` human text presentation after
  packaged-goal changes: operator-oriented continuous startup output without
  `--quiet`, factory text invocation stdout that suppresses operator chatter,
  and named-goal batch stdout that stays primary-result-only. Reuse helpers
  from sibling `json_result_test.go`, `ndjson_stream_test.go`, and
  `stream_backpressure_test.go` when extending these regressions.
- `cmd/factory` owns the operating-system interrupt boundary for the CLI.
  Derive the reusable process context with `signal.NotifyContext`, stop the
  signal subscription on return, and classify wrapped `context.Canceled` as the
  manifest-declared exit 130. This lets continuous runs unwind through the
  canonical Initializer lifecycle instead of being terminated outside it.
- `pkg/factory/packages/tts/` owns packaged TTS invocation metadata shaping
  helpers used when `INFERENCE_RUN` (or legacy `MODEL_INVOKE`) work completes on the `execute-tts` workstation.
  `metadata.go` derives the `backend` metadata field from the loaded on-disk
  worker model so customer edits to materialized `factory.json` affect the next
  invocation result.
- `docs/reference/run.md` (`you docs run`) owns supported `@you/goal` batch
  invocation, stdout primary-result, and response-stream guidance.
- `pkg/config/defaultpaths/default_paths.go` owns the canonical shared named-factory
  root for both initializer-owned materialization and `you run --named` lookup;
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
- `pkg/services/factory_definitions/packages/tts/observability.go` classifies packaged TTS loading,
  model-not-ready, and generation-failure outcomes and defines stable invocation
  error codes plus packaged-factory metric names.
- `pkg/transports/cli/run/packaged_tts_invocation.go` logs named-factory resolution context at
  the CLI boundary without recording packaged-factory metrics or logging submitted
  text or generated artifact bodies.
- `pkg/services/factory_definitions/packages/goal/` owns packaged `@you/goal` factory metadata
  constants (`PackagedFactoryName`, `PackagedInvokeWorkstationName`).
- `packages/packaged-factories/generated/manifest.json` is the single
  registration point for shipped named Factories. Regeneration derives it from
  authored Factory directories, and normal initializer startup materializes every
  validated manifest entry without separate Go registration. Customer-facing
  packaged invocation guidance belongs in `docs/reference/run.md`.
- Website Packaged Factory discovery belongs behind
  `ui/src/features/packaged-factories/lib/public-contract.ts`. Keep package data
  unknown until that boundary validates manifest format 1, and resolve selected
  schemas and artifacts only through the documented
  `@you-agent-factory/packaged-factories` public export specifiers; manifest
  locators are integrity metadata, not browser or filesystem lookup paths.
- Derive website Packaged Factory inventory and detail models with the pure
  projectors in `ui/src/features/packaged-factories/lib/projection.ts`. Preserve
  stable public names as identity, resolve only exact locale keys before the
  package-owned base value, and represent missing descriptions and examples as
  explicit presentation states rather than copying fallback catalog metadata
  into the website.
- Keep manifest-driven website loading and ephemeral selection in
  `ui/src/features/packaged-factories/hooks/use-packaged-factory-inventory.ts`.
  Scope asynchronous results to the current data-source and locale identities,
  clear selected detail before each artifact request, and preserve a valid
  inventory when one selected artifact fails so another selection can recover.
  Render the state through the feature public boundary rather than duplicating
  package-owned inventory or presentation metadata in an app shell.
- The website's bundler resolver for data-only Packaged Factory exports is
  generated by `ui/scripts/generate-packaged-factory-resolver.mjs` from the
  package's public manifest import and lives under the feature's `lib/generated/`
  lane. Prepare the declared local dependency as a physical npm-packed candidate
  before website quality/build commands so root Go tooling does not traverse a
  workspace link. The generator, TypeScript, and Vite must then use normal
  installed-package resolution; do not map public-looking specifiers back to
  repository package paths. Keep every generated import on a documented public
  package specifier and run `bun run check:public-package-boundaries`; its
  freshness check must reject manifest drift instead of letting the generated
  map become a handwritten catalog. Run
  `bun run verify:packaged-factories-installed-consumer` to prove a relocated UI
  builds while the repository Packaged Factories source path is unavailable.
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
- Functional coverage for packaged `@you/fusion` Petri invocation belongs in
  `tests/functional/factory/packaged/fusion/invocation_test.go`. Exercise
  `POST /factory-sessions/~default/invocations` with `InvocationRequest.Args`,
  observe multi-worker merge order through public factory dispatch events
  (`draft-fusion` then `refine-fusion`), and correlate optional overrides from
  `MODEL_REQUEST`, `INFERENCE_RESPONSE`, and `AGENT_RUN_RESPONSE` on the
  factory event stream. Durable-session `GET /factory-sessions/{id}/dispatches`
  is for JavaScript workflow sessions such as deep-research, not Petri
  invocation-only runs.
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
- `docs/reference/agents.md` owns root discovery and the submitter-facing
  liveness decision: bare `you` is successful side-effect-free help, while a
  service intended for later `you submit` calls must start through `you server`
  or a server-enabled run. Do not describe an ordinary serverless `you run` as
  a listening submission target.
- Dashboard current-factory decoding for signature-backed invocation widgets
  lives in `ui/src/api/factory-definition/api.ts` and
  `ui/src/api/current-factory-definition/api.test.ts`; keep exact
  `${parameter}` placeholders accepted on invocation-interpolated enum-backed
  authored fields when the current factory payload also declares that parameter
  in `invocationSignature`, or live session pages will fall back to legacy UI
  flows even when backend runtime validation already accepts the factory.
- Managed-runtime invocation readiness gating and direct invocation policy live in `pkg/models/service/invoke.go`; the canonical service consumes neutral `pkg/models/host.Host.InspectReadiness` snapshots, projects public readiness through `pkg/transports/mapping/managed_runtime_invocation.go`, and owns invocation failure classification and readiness logs. Stable provider command identity lives in `pkg/models/provider`; worker and model invocation exchange `pkg/workers/execution` requests, results, provider sessions, and normalized failures, while `pkg/workers/diagnostics` owns the safe generated projection. `pkg/wire/production.go` supplies the active-runtime reader, process model host, assets, logger, clock, metrics, invocation executor builder, and runner identity directly; `FactoryService` and `runtimehost.Host` only retain compatibility forwarding/composition seams and are never passed into the model family. Factory worker execution routes through `pkg/models/host/execution.go` when a process-wide host is configured, otherwise through the local manager fallback. Process-wide local-runtime ownership and lease boundaries belong in `pkg/models/host`; keep `pkg/models/local` as the managed-runtime catalog compatibility projection layer. See `docs/architecture/model-host.md`.
- When a shared merge introduces a backend package-coverage floor that the reviewed head no longer reaches, use the failing CI profile's exact reported value to make the smallest manifest adjustment; do not run the manifest updater against the whole repository because it can ratchet unrelated package floors.
- Functional event-leak assertions must target the injected sensitive fixture, not generic temporary-directory fragments: root-process event payloads legitimately include the harness's factory source and working-directory paths.
- After a canonical CLI-family cutover shrinks a package (for example
  `pkg/transports/cli/mcp` or Models
  `pkg/services/models/transports/cli` command handlers), restore unit-coverage
  floors with behavioral tests of resolved-input adapter error paths: missing
  stable-ID local/inherited inputs, missing injected dependencies, and
  initializer/home failures that must not invoke the operation. Do not weaken
  `go-unit-coverage-package-minimums.json` for migration-owned packages.
- Functional coverage does not inherit unit-test hits. After the same cutover,
  restore `go-functional-coverage-package-minimums.json` floors with short
  `tests/functional/...` evidence that exercises the migrated packages under the
  functional profile: docs topic inventory accessors
  (`TopicIndexEntries` / `SupportedTopicCommands`) plus alias `you docs` paths,
  MCP `ResolvedServeHandler` fixture/runtime/error paths (and production
  `you mcp serve --runtime` missing-home), and process-level
  `you models list` / `inspect` against an injected `--server`. Do not weaken
  the functional package floors for migration-owned packages.
- `pkg/workers/mockworker/runner.go` preserves the original provider command, args,
  and worker identity in `YOU_MOCK_WORKER_*` script environment variables before a
  script mock replaces the command. Functional CLI tests can capture those values
  to prove effective provider/model selection without allowing a live provider;
  retain a behavioral output sequence as well so selection assertions do not
  replace workflow-result coverage.
- A named factory whose submitted Work fans out into derived terminal Work must define an explicit `invocationReturn` targeting the final Work type and terminal state. The default submitted-work return policy cannot follow a fan-out to a separately derived merge result.
- Structured invocation input is normalized into the submitted Work's canonical text content at `pkg/factory/sessions/invocation/session_owner.go`; `WorkRequestFromSubmitRequests` and `NormalizeWorkRequest` must preserve cloned invocation arguments so fan-out-derived Work can render the original request without relying on a transient `${input}` placeholder. Use `workPropagation.mode: PRESERVE_INPUT` plus a dedicated processing-state route when a final fan-in must consume that original Work alongside derived branch results.
- Canonical CLI-family cutovers must extend the production `clicontract` check
  beyond command identity: compare the constructed positional arguments,
  effective local/inherited flags, completion choices, normalization, and input
  relationships against the authored manifest. Generated commands with no
  declared arguments must install `cobra.NoArgs`; leaving `Args` unset makes
  grouped commands appear variadic to the observable input inventory.
- For manifest-authoritative `you run` / `you server` construction, keep local
  flag help in `contracts/cli/commands.json` and bind stable flag names through
  an explicit target map assembled beside the handler config. Do not recreate a
  hidden Cobra command to harvest help strings or add flag-name switches in the
  family constructor; the `retired-surface-check` static-policy lint plus
  executable/manifest parity tests under `pkg/transports/cli/baseline` and
  `climanifestcobra` protect that seam. Keep source scanning out of the
  behavioral Go test suite.
- Keep `retired-surface-check` focused on non-test production boundaries. For
  manifest-owned root, session, work, submit, run, server, and Factory-authoring
  families, it rejects handwritten `cobra.Command` presentation metadata,
  direct Cobra/pflag public-input registration, and the retired CLI-shape
  mirrors/functions. Generic projectors under `climanifestcobra` may register
  parser storage only from manifest records. Test the lint diagnostic with
  synthetic source fixtures; prove customer behavior separately through
  `root.BuildProcess`, built-executable, and semantic contract tests.
- Generated command-family handlers should translate invocation-local values
  addressed by stable manifest input ID into a fresh typed transport config for
  each execution. Do not retain shared config pointers as parser storage across
  `application.Process.Execute` calls. When compatibility behavior depends on
  whether a projected flag was explicitly supplied, use
  `climanifestcobra.InputChanged` with the stable input ID so public spellings
  remain private to the generated Cobra projection. Prove omitted and explicit
  values on repeated executions through `root.BuildProcess`.
- Retained family constructors that cannot yet use the generic command-tree
  projector should bind Cobra only to scalar parser storage keyed by stable
  input ID. Annotate those flags for `climanifestcobra.InputValues`, then map
  the typed snapshot into a fresh domain-facing transport config in the handler;
  never bind generated flags directly into a reusable request/config struct.
- Raw JavaScript `you run --factory workflow.js` owns a standalone durable
  execution service rather than a Factory Runtime. When hosting that run, bind
  the same execution service to the generated HTTP/dashboard transport, gate
  the terminal sync operation on listener readiness, and place transport,
  completion, and execution cleanup in one Factory Sessions lifecycle plan.
  Do not open a second execution scope merely to serve the API.
- When a migrated command starts projecting authored Examples into Cobra help,
  refresh the matching intentional help fixture under
  `pkg/transports/cli/baseline/testdata/` (for docs:
  `docs_help.txt` / `TestDocsHelpBaseline_MatchesFixture`) to the normalized
  production `--help` output. That ledger path is separate from
  `intentional_changes.json`, which only tracks planned removals and moves.
- When migrated construction intentionally changes observable command identity
  (Examples, Long text, or related inventory fields), refresh
  `contracts/testdata/baseline/cli-commands.json` with
  `UPDATE_CLI_BASELINES=1 go test ./pkg/transports/cli/commandidentity -run TestWriteProductionInventoryBaseline`,
  and prove it with `TestWalk_ProductionInventoryMatchesCommittedBaseline`.
  That executable observation is evidence, not a publication source. The
  package-facing CLI contract and its schema are byte-identical generated
  projections of `contracts/cli/commands.json` and
  `contracts/cli/command-manifest.schema.json`; refresh them with
  `make contracts-generate` / `make contracts-check` and keep semantic parity
  against generated families plus the `root.BuildProcess` observation in
  `pkg/transports/cli/clicontract`. Do not hand-edit staged package copies.
- After residual baseline/coverage fixes on a completed docs/models/mcp
  cutover, re-prove preserved public behavior with
  `make cli-manifest-check`, `make cli-contract-smoke`, focused
  docs/models/mcp unit + `tests/functional/transport/docs`,
  `tests/functional/transport/mcp/protocol`, `tests/functional/transport/mcp_serve`,
  `tests/functional/models/model_list`, and
  `tests/functional/smoke -run TestDocsCommandSmoke_` evidence, then the
  `make verify-fast` constituents (`make typecheck`, `make mcp-contract-check`,
  `make ui-test`, `make test`) plus `make lint`. New residual functional sources
  must use an allowed product-domain noun such as `transport` or `models`
  (`tests/functional/<domain>/<subsection>/...`); do not add files under the
  deletion-only `tests/functional/cli` catch-all. Do not remigrate families or
  expand into out-of-scope CLI commands during that re-proof.
- Dashboard feature routes must account for the production `/dashboard/ui/` SPA
  mount as well as any intentional standalone development path. Prove new routes
  with a built-preview browser test that navigates the hosted path directly;
  component tests that inject a pathname do not verify production routing.
- Registry-migrated provider integrations receive subprocess and filesystem
  effects through `pkg/wire` when built-in registrations are constructed.
  Preserve optional subprocess capabilities when adapting those effects:
  expose Workers `RunStreaming` only when the injected Platform runner actually
  implements streaming, so buffered functional overrides continue through the
  same registry route without a false streaming capability.
- A Runner that starts an injected command must retain partial stdout/stderr in
  detached result and failure diagnostics, then emit exactly one terminal event
  after all progress fragments. Normal non-zero exits keep their exact exit code
  and use the failed-exit outcome; process-start failures omit an exit code and
  use the process-error outcome. Validation failures occur before request-event
  recording or command invocation.
- Script interruption remains split across boundaries: the injected platform
  process edge terminates the complete process tree and waits for command and
  cleanup completion before returning, while the Script Runner classifies
  cancellation separately from deadline timeout, retains partial streams, marks
  timeout diagnostics, and records one matching terminal response. Pre-start
  cancellation returns without command or event effects.
- Script-worker cutovers keep the generic workstation request/result adapter
  thin: construct and resolve the configured implementation through the private
  immutable Runners registry, invoke only the common `workers.Runner`, and map
  its detached result and normalized failure metadata back to `WorkResult`.
  Compatibility command edges that expose only `Run` may publish one complete
  stdout chunk followed by one complete stderr chunk; real process edges retain
  live mixed-stream ordering through `RunStreaming`.
- Agent Runner root-built functional evidence belongs in
  `tests/functional/workers/agent/` for `root.BuildProcess` construction
  inertness and in `pkg/services/workers/service/agent_runner_root_test.go`
  for Workers-service composition of the registered Agent Runner over an
  injected `providers.Service`. Do not import `pkg/root` from
  `runners/wire` tests; that creates a `wire -> root -> wire` cycle.
- Providers-backed Agent Runners map typed one-attempt failure kinds at the
  Workers boundary: authentication and invalid requests remain terminal,
  throttling enters the throttle family, dependency and timeout failures remain
  retryable, and unknown failures remain terminal. Publish detached failure
  progress before returning the single normalized failure, preserve the
  Providers error as a cause, and let caller context cancellation or deadline
  win any concurrent provider failure classification; the Runner never retries.
- Agent dispatch cutover routes model/agent/inference workers through
  `construction.Service.WithAgentRunnerCutover(true)`, which resolves the
  registered parent-private Agent Runner over `providersroot.NewService` and
  skips conductor/registry-capability runner decorators. Retire executor-level
  `inferWithRetry`; caller-owned retry remains outside the Runner boundary.
