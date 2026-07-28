# CI Relevant Files

## Pull-request verification workflow

- The five public frontend packages under `ui/packages/` share a family-level
  dependency gate at `ui/scripts/check-public-package-boundaries.mjs`. Keep the
  package graph and runtime dependency allowlists explicit there, preserve its
  real-tree scanner in the UI lint/check commands plus
  `make ui-public-package-boundaries`. Package-local boundary checks still own
  narrower runtime restrictions and internal layer direction; do not duplicate
  source-scanner policy as a behavioral unit test.

- `make ui-public-package-release` is the aggregate release gate for the client,
  replay, emulator, components, and visualizers packages. It owns generation
  freshness, package-local typecheck/tests/builds, tarball inventories, clean
  installed consumers, and visualizer Storybook/browser evidence, and runs from
  the required `ui-integration-test` CI-equivalent lane after Playwright is
  installed. The emulator's `verify:release` intentionally omits only its
  change-scope development check; the ordinary package `verify` command retains
  that additional repository-scope guard.

- `make ui-public-package-publish-prepare PACKAGE_VERSION=<semver>` builds the
  canonical five-package family, stages manifests with one version and aligned
  internal dependency pins, and writes registry-format tarballs plus evidence
  under `.artifacts/public-packages` by default. The Development Package
  workflow runs this command as a frontend-only pull-request dry run and
  publishes only this family at an immutable `dev` version after protected
  `main` succeeds. API and Packaged Factories join the frontend family only in
  the complete tagged-release candidate, which publishes one release semver
  under `latest`. Publication uses npm trusted publishing and verifies every
  exact version after upload.

- `make public-release-package-smoke` is the required complete-set release
  behavior gate. It exercises scope validation, real seven-package candidate
  preparation, and protected publication preflight from the repository root on
  every Development Package workflow run. Keep the command portable when it
  invokes npm outside an npm script, including on Windows.

- `.github/workflows/ci.yml` owns pull-request and `main` CI lane scheduling.
  Build, Lint, and API are independent Ubuntu jobs, respectively rerunnable
  with `make verify-build`, `make verify-lint`, and `make verify-api`. Keep
  their setup and verification ownership separate: Build produces the backend
  binary and dashboard bundle; Lint owns static and maintainability checks;
  API owns contract, generated-code, API integration, package, and composition
  smoke checks. Do not publish a dashboard artifact unless a downstream lane
  genuinely consumes that exact bundle; the browser integration harness builds
  its own origin-scoped bundle.
  The `Build, Lint, and API` compatibility job publishes the legacy required
  status after those three independent jobs finish. It uses `always()` so a
  failed or cancelled prerequisite becomes an explicit failed compatibility
  result instead of leaving the required context pending. Keep this transition
  job free of verification work, and remove it once the repository ruleset
  requires the three focused statuses directly.
  Release Surface Smoke and PR Inference Approval are also independent jobs:
  neither may depend on Build, Lint, or API. Release smoke rebuilds the dashboard
  and CLI and installs Playwright locally; inference approval restores its own
  managed-model cache and installs its runtime. Keep those self-contained setup
  steps so an unrelated verification failure cannot suppress either result.
  Focused lint reruns must stay Windows-compatible: do not add standalone
  `printf` recipe lines to targets used by `make verify-lint`.
  The Lint job also owns the public components distribution gate through
  `make ui-components-verify`. Install Chromium before that gate and keep its
  production build, registry-pack inventory, and clean installed-consumer
  phases as separately rerunnable Make targets so failures identify the broken
  distribution boundary without replacing the existing typecheck, unit,
  Storybook, package-boundary, or dependency-direction checks. Run the package
  build before typecheck because clean-checkout Storybook sources resolve the
  package's public self-reference through the compiled manifest targets.
  Backend Unit Coverage and Backend Functional Coverage are an independent,
  fail-fast-disabled matrix gated only by Classify PR Impact. Preserve the
  explicit run/intentional-skip summary, per-suite coverage thresholds,
  diagnostics, and artifacts; neither lane may depend on Build, Lint, or API.
  Exact local reruns are `make test-unit-coverage` and
  `make functional-test-viz` (required Backend Functional Coverage). Coverage-
  only functional reruns remain `make test-functional-coverage`.
  `make test-functional-coverage` always runs `functional-boundary-check` first,
  and `make functional-test-viz` composes that coverage lane once with profile
  plus `-json-output` before rendering Markdown, so the required CI functional
  lane cannot succeed without a successful boundary check; the unit coverage
  lane stays independent of that prerequisite. CI keeps the established upload
  root `.artifacts/backend-functional-coverage/` by setting
  `FUNCTIONAL_TEST_VIZ_DIR` for the functional matrix leg, tees `command.log`,
  and uploads `functional-tests.md`, `coverage-summary.json`, `coverage.out`,
  and `command.log` on success and failure when those files exist (upload step
  uses `if: always()`). Both coverage commands serialize Go package coverage
  writers before canonicalizing repeated source blocks into one sorted profile;
  keep that ordering so concurrent packages cannot corrupt the shared profile
  and the uploaded artifact matches the totals enforced by the lane. Prove the
  functional boundary-before-coverage composition with stubbed Make wrapper
  smoke under
  `tests/functional/observability/coverage/functional_coverage_boundary_test.go`
  rather than the full functional suite.
  The default functional and functional-coverage lanes share dynamic package
  selection through `internal/testlanes`: retain execution of the complete
  `go list ./tests/functional/...` result, the shared-support exclusion, and
  required provider-destination validation in both callers. Required package
  validation protects topology but must not become an execution allowlist.
  Durable top-level `Test*` inventory for functional-test visualization lives in
  `internal/functionaltestmetadata`: walk `*_test.go` with `go/parser` /
  `go/ast`, emit slash-normalized file/package/name/line metadata, take the
  first Go-doc sentence through `go/doc.Synopsis`, attach file-level
  `//go:build` (preferring it over legacy `// +build`) expressions as
  `BuildTags`, and capture explicit golden fixture/manifest paths from a
  `//golden: <path>` doc directive or a test-owned `golden` /
  `goldenManifest` / `goldenFixture` string declaration. Classify
  `internal/**` and `*helpers*_test.go` paths as harness verification
  (`ClassificationHarness`); all other inventoried `Test*` records are
  `ClassificationCustomer`. `CustomerScenarioCount` equals the customer
  record count only. Fail closed with a file-scoped error on malformed
  source. Undocumented customer identities are enforced against the exact
  deletion-only ledger
  `docs/internal/baselines/functional-undocumented-tests.json` via
  `CheckAgainstBaseline` / `ValidateBaselineUpdate` (subset or identical
  match succeeds; new undocumented customer tests and baseline expansions
  fail; harness/internal helpers stay out of the ledger). Later FND cells
  wire that check into Make/CI; do not reintroduce regex or line-scraping
  inventories.
  The maintainer-readable Markdown catalog generator lives in
  `internal/functionaltestviz` with thin CLI entrypoint
  `cmd/functionaltestviz`. It consumes inventoried
  `functionaltestmetadata.Record` values plus an existing
  `gocoveragecheck` coverage-summary JSON file (never a second `.out`
  profile parser), attaches golden manifest provenance fail-closed, and
  writes `.artifacts/functional-test-viz/functional-tests.md` by default
  via `Generate` / `WriteCatalogFile`. Report semantics stay in the library.
  Maintainer/CI composition is `make functional-test-viz`: boundary check,
  one short functional coverage lane with
  `GO_FUNCTIONAL_COVERAGE_PROFILE` / `GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT`
  under `.artifacts/functional-test-viz/`, then `cmd/functionaltestviz`.
  The target is fail-closed: boundary, suite, coverage-floor, metadata, or
  rendering failures exit non-zero. It never deletes the artifact root on
  failure, so already-written diagnostics remain (for example profile/JSON
  after a floor fail, or those files before a render fail). Prove fail-closed
  preservation with stubbed Make wrapper smoke under
  `tests/functional/observability/coverage/functional_test_viz_fail_closed_test.go`
  rather than the full functional suite. Prove rendering with focused
  package/cmd golden fixtures under `internal/functionaltestviz/testdata/`.
  Required CI Backend Functional Coverage runs `make functional-test-viz`
  (with `FUNCTIONAL_TEST_VIZ_DIR=.artifacts/backend-functional-coverage`) so
  `functional-boundary-check` stays unavoidable through the nested
  `test-functional-coverage` call, and the lane uploads Markdown, coverage
  JSON, profile, and command log on success and failure.
  JavaScript file-backed loading functional coverage belongs in
  `tests/functional/orchestration/javascript/loading/file_javascript_test.go`:
  drive sync Factory Session execution through `support.BuildProcess` +
  `support.FakeInputs` with `you --json run`, `--factory`, and
  `--with-mock-workers`; scaffold file-backed factories with
  `orchestrator.javascript.sourceRef` and workflow modules beside
  `factory.json`; prove factory-relative ES module imports resolve under the
  Factory root with a terminal `COMPLETED` primary result that reflects the
  imported module contribution and zero provider dispatch; prove missing
  factory-relative imports fail before work starts with customer-stable
  `workflow.source.notFound` diagnostics that name the missing path without
  private VM stack frames or live provider execution. Substitute external
  effects only through `edges.Edges`. Catalog metadata infers domain
  `orchestration` and subsection `javascript/loading` from the path; every
  top-level `Test*` needs a customer-readable Go doc so `functionaltestmetadata`
  stays viz-compatible.
  Named JavaScript Factory loading functional coverage belongs in
  `tests/functional/orchestration/javascript/loading/named_factory_test.go`:
  register inline factories with `support.CreateNamedFactory`, invoke by name
  through `you --json run --named` from an unrelated working directory with
  `HOME` pinned to the test catalog, and prove terminal `COMPLETED` primary
  outcomes tied to the named Factory identity; prove the same named Factory
  through `POST /factory-sessions/sync` with `source.kind=FACTORY_ID` and
  `edges.FactoryRuntimeWorkflowHome` so global catalog lookup resolves; prove
  pause/resume Factory Session controls on async durable sessions started with
  `POST /factory-sessions/async` plus matching CLI `session pause|resume` with
  `--server`, asserting durable `status` and `resolvedSource.sourceRef` on the
  named identity. Substitute external effects only through `edges.Edges`.
  JavaScript output mapping functional coverage belongs in
  `tests/functional/orchestration/javascript/contracts/output_mapping_test.go`:
  drive sync Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `WaitForServiceModeRuntime: true`, `UseMockWorkers: true`, and a recording
  `edges.Edges.ProviderCommandRunner`; prove script `return` values map to
  `result.primaryResult`, final results, durable `resultSummary`, and
  `SESSION_RESULT_UPDATED` Factory Events; prove `workflow.artifact()`
  structured artifacts appear on artifact list/detail, final result
  `artifactIds`, session `artifactRefs`, and result/completion Factory Events;
  prove unsupported root return values (for example function values) yield
  `FAILED` session status with actionable `failureDetail` containing
  `workflow.result.unsupportedType` without private VM stack frames or live
  provider execution. Substitute external effects only through `edges.Edges`.
  Catalog metadata infers domain `orchestration` and subsection
  `javascript/contracts` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  TypeScript Factory loading functional coverage belongs in
  `tests/functional/orchestration/javascript/loading/typescript_test.go`:
  drive sync Factory Session execution through `support.BuildProcess` +
  `support.FakeInputs` with `you --json run`, `--factory`, and
  `--with-mock-workers`; scaffold file-backed factories with
  `orchestrator.javascript.sourceRef` pointing to `.ts` workflow files that
  require MVP TypeScript stripping; prove supported TypeScript transpiles and
  runs to terminal `COMPLETED` primary outcomes that reflect typed source
  execution with zero provider dispatch; prove deliberate type or syntax
  failures fail before dispatch with customer-stable `workflow.source.syntaxError`
  diagnostics that are actionable without private VM internals; prove
  source-map remapping reports authored `.ts` line/column suffixes in failure
  diagnostics rather than only emitted JavaScript locations. Substitute
  external effects only through `edges.Edges`.
  JavaScript agent composition functional coverage belongs in
  `tests/functional/orchestration/javascript/composition/agent_test.go`:
  drive sync Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `UseMockWorkers: true` and a recording `edges.Edges.ProviderCommandRunner`,
  assert unary child results on `result.primaryResult`, and prove stable
  `FAILED` dispatch records via the `fail:` fake-child prompt prefix without
  live provider execution. JavaScript pipeline composition functional coverage
  belongs in
  `tests/functional/orchestration/javascript/composition/pipeline_test.go`:
  use `pipeline(items, worker, next?)` with at least two stages so stage-two
  prompts depend on stage-one child output, and assert stage-output data flow
  on `result.primaryResult` and the Factory Session dispatch listing without
  live provider execution. JavaScript parallel composition functional coverage
  belongs in
  `tests/functional/orchestration/javascript/composition/parallel_test.go`:
  drive async Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `WaitForServiceModeRuntime: true` and a controllable
  `edges.Edges.ProviderOverride`, prove concurrent external child dispatch by
  blocking `Provider.Infer` until public `inFlightDispatches` reaches the fan-out
  size, release children to assert declared input-order results and documented
  partial-failure shaping on `/factory-sessions/{id}/results?mode=final` and
  `/dispatches` without wall-clock sleeps. JavaScript for-each composition
  functional coverage belongs in
  `tests/functional/orchestration/javascript/composition/for_each_test.go`:
  drive sync Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `UseMockWorkers: true` and a recording `edges.Edges.ProviderCommandRunner`,
  use single-stage `pipeline(items, worker)` (no `next` callback) to prove
  per-input child dispatch cardinality, input/result correlation on public
  dispatch listings and `result.primaryResult`, and empty `items = []`
  completion with zero child dispatches without live provider execution.
  Catalog metadata infers domain `orchestration` and subsection
  `javascript/composition` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  JavaScript staged composition functional coverage belongs in
  `tests/functional/orchestration/javascript/composition/stages_test.go`:
  drive sync Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `UseMockWorkers: true` and a recording `edges.Edges.ProviderCommandRunner`,
  prove named `pipeline` stage labels and `phase(name)` ordered progress on
  public dispatch listings, `result.primaryResult`, and
  `GET /factory-sessions/{id}/events` `ORCHESTRATOR_PHASE_CHANGED` events, and
  prove `pipeline([], worker, next?)` completes with the documented empty
  ordered per-item public result and zero child dispatches without live provider
  execution. Catalog metadata infers domain `orchestration` and subsection
  `javascript/composition` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  JavaScript nested composition functional coverage belongs in
  `tests/functional/orchestration/javascript/composition/nested_test.go`:
  drive sync Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `WaitForServiceModeRuntime: true`, `UseMockWorkers: true`, and a recording
  `edges.Edges.ProviderCommandRunner`; nest `parallel([...])` inside a
  `pipeline(items, worker, next?)` stage by returning the `parallel()` promise
  from a sync stage worker (not `async function`), assert nested child labels
  and dispatch ids on `result.primaryResult` and `/factory-sessions/{id}/dispatches`,
  and prove nested failure naming via `fail:` mock-child prompts on dispatch
  `failureDetail` plus stage-indexed parallel child diagnostics without live
  provider execution or private VM stack frames. Catalog metadata infers domain
  `orchestration` and subsection `javascript/composition` from the path; every
  top-level `Test*` needs a customer-readable Go doc so `functionaltestmetadata`
  stays viz-compatible.
  JavaScript per-child worker override functional coverage belongs in
  `tests/functional/orchestration/javascript/workers/overrides_test.go`:
  drive sync or async Factory Session execution through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `WaitForServiceModeRuntime: true`, assert per-child `modelProvider`/`model`
  selections on `/factory-sessions/{id}/dispatches` and injected
  `edges.Edges.ProviderOverride` inference requests, prove partial
  `workers.MockWorkersConfig` behavior with `UseMockWorkers: true` via
  `javascript.executionMode` and public child results, and surface invalid
  per-child overrides on durable session `failureDetail` with synchronous
  inline workflows when override validation fails before dispatch. Catalog
  metadata infers domain `orchestration` and subsection `javascript/workers`
  from the path.
  Script-poller packaged-owner evidence belongs to
  `pkg/services/automations/internal/services/script_pollers` and is reached
  through the composed Automations root in `pkg/services/automations/internal`.
  Unit evidence covers submit, timeout/restart, malformed rejection, and cursor
  persistence in the packaged owner tests; Automations-root composition evidence
  belongs in `pkg/services/automations/internal/service_internal_test.go`
  (`TestProductionRootScriptPollerCursorThroughCompositionPath`). Root
  `BuildProcess` functional evidence belongs in
  `tests/functional/workstations/poller/poller_test.go` and
  `tests/functional/workstations/poller/build_process_test.go`: drive POLLER
  workstation supervision through `tests/functional/internal/support.BuildProcess`
  with injected `edges.Edges.ScriptCommandRunner` and optional
  `edges.Edges.Clock`, observe Work admission through public session listings,
  and prove construction remains inert before an explicit run invocation.
  Catalog metadata infers domain `workstations` and subsection `poller` from
  the path; every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  Factory_runtime Petri authored eligibility guard functional coverage belongs
  in `tests/functional/factory_runtime/orchestrators/petri/guards/eligibility_test.go`:
  prove VISIT_COUNT block-until-satisfied, SAME_NAME correlated release, and
  VISIT_COUNT/MATCHES_FIELDS failure visibility through public Factory Event
  dispatch ordering, Work listings, and API status observation via
  `support.RunFactoryToCompletionWithEdgesAndObservations`,
  `support.StartFunctionalAPIServer`, and `support.NewShapedProviderCommandRunner`
  without service-internal Petri imports. Catalog metadata infers domain
  `factory_runtime` and subsection `orchestrators/petri/guards` from the path;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  Mock-worker replacement functional coverage belongs in
  `tests/functional/workers/mock/replacement_test.go`: prove named-only
  `--with-mock-workers` replacement through
  `tests/functional/internal/support.StartFunctionalAPIServer` with
  `MockWorkersConfig`, `UnmatchedDispatchPolicy: passthrough`, and an injected
  `edges.Edges.ProviderCommandRunner`; prove invalid override contract failures
  through `support.BuildProcess` + `process.Execute` before dispatch; and prove
  configured mock rejection with stable public `WorkOutcomeFailed` /
  `WorkFailureTypeUnknown` dispatch responses without live provider credentials
  or leaking configured reject stdout/stderr on customer-visible surfaces.
  Catalog metadata infers domain `workers` and subsection `mock` from the path;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  Workers CLI run invocation-help functional coverage belongs in
  `tests/functional/workers/transports/cli/run/help/invocation_help_test.go`:
  prove Factory invocation signature help for `you run --named <factory> --help`
  through `support.BuildProcess` + `support.FakeInputs` with
  `HOME`/`USERPROFILE` pinned to the seeded named-factory catalog; prove
  required vs optional parameter labels and usage tokens from observable stdout
  only; prove read-only help with
  `serviceedges.Edges{ProviderCommandRunner: testutil.NewProviderCommandRunner()}`
  and zero provider dispatch. Substitute external effects only through
  `edges.Edges`; do not use `--with-mock-workers` for this cell. Catalog
  metadata infers domain `workers` and subsection `transports` from the path;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  Packaged `@you/subagent` invocation functional coverage belongs in
  `tests/functional/factory/packaged/subagent/invocation_test.go`: prove child
  primary-result return through public CLI JSON and hermetic no-server named
  invocation, child Factory Response Event streaming on
  `GET /factory-sessions/~default/response-events`, and stable child failure
  through CLI JSON plus API invocation with rejecting mock workers. Drive proofs
  through `support.InstallPackagedFactory`, `support.WriteMockWorkersConfig`,
  `support.BuildProcess`, and `support.StartFunctionalAPIServer` without
  service-internal Petri imports. Catalog metadata infers domain `factory` and
  subsection `packaged/subagent` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  CLI single-JSON result functional coverage belongs in
  `tests/functional/transport/cli/output/json_result_test.go`: invoke
  `support.BuildProcess(t, serviceedges.Edges{}).Execute` with global `--json`
  and without `--output response-stream`, decode stdout through generated
  `factoryapi.InvocationResponse`, prove stderr `factoryapi.ErrorResponse` on
  failures, and assert private-runtime key exclusion at the public CLI boundary.
  Output-selection conflicts use instrumented `edges.Edges` to prove zero
  product side effects before activation. Catalog metadata infers domain
  `transport` and subsection `cli/output` from the path. Every top-level `Test*`
  needs a customer-readable Go doc so `functionaltestmetadata` stays
  viz-compatible.
  Work-owned unary `you submit` contract functional coverage belongs in
  `tests/functional/work/transports/cli/submit/unary_contract/unary_contract_test.go`:
  drive `support.BuildProcess` + `Process.Execute` with public `you submit --json`,
  prove file-backed and stdin (`--payload -`) payloads reach Work with public
  acknowledgment and session-scoped list observations, prove omitted `--session`
  targets `~default` while explicit `--session <id>` scopes correctly, and prove
  structured backend rejection preserves only public typed failure markers through
  a controlled `httptest` edge. Catalog metadata infers domain `work` and
  subsection `transports/cli/submit/unary_contract` from the path. Every top-level
  `Test*` needs a customer-readable Go doc so `functionaltestmetadata` stays
  viz-compatible.
  CLI positional parameter values functional coverage belongs in
  `tests/functional/transport/cli/parameters/positional_values_test.go`: prove
  one `you run --factory` positional prompt with spaces and Unicode survives on
  `CLIObserver` `Parse.Positionals`, prove surplus prompt positionals against a
  single-slot `invocationSignature` fail with
  `INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW` and zero provider dispatch, and
  prove `you session pause` default versus explicit session targeting through
  mock HTTP request paths at the public `support.BuildProcess` boundary. Catalog
  metadata infers domain `transport` and subsection `cli/parameters` from the
  path; every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  CLI key=value parameter mapping functional coverage belongs in
  `tests/functional/transport/cli/parameters/key_value_test.go`: prove repeated
  `--key=value` tokens reach canonical `InvocationArguments` through
  `SubmissionRecorder`, prove values with embedded `=` survive intact, prove
  duplicate keys on REPEATED parameters append in CLI observation order, and
  prove malformed shapes (missing value, bare `key=value` without `--`) fail
  with stable diagnostics and zero provider dispatch through
  `ProviderCommandRunner` at the public `support.BuildProcess` boundary. Catalog
  metadata infers domain `transport` and subsection `cli/parameters` from the
  path; every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  HTTP API server startup/shutdown functional coverage belongs in
  `tests/functional/transport/http/server/startup_shutdown_test.go`: prove
  configured `--server` loopback listeners serve non-empty `GET /status`
  readiness after start through `support.StartFunctionalAPIServer`, prove
  shutdown closes the listener and terminates active public streams (including
  in-flight Factory Session invocations) without leaking ports, and prove bind
  failure through `platformhttpserver.NewStarter` on `edges.Edges.APIServerStarter`
  reports `SERVER_BIND_FAILED` with zero browser/readiness side effects and
  rebound availability on the requested address. Catalog metadata infers domain
  `transport` and subsection `http/server` from the path; every top-level
  `Test*` needs a customer-readable Go doc so `functionaltestmetadata` stays
  viz-compatible.
  HTTP API server content-negotiation functional coverage belongs in
  `tests/functional/transport/http/server/content_negotiation_test.go`: prove
  JSON requests and responses use the documented `application/json` media type
  from the published OpenAPI inventory, prove unsupported `Content-Type` values
  against JSON-bodied endpoints return structured HTTP 415
  `UNSUPPORTED_MEDIA_TYPE` errors before body decode, and prove malformed JSON
  bodies with the documented JSON media type return structured HTTP 400
  `BAD_REQUEST` errors distinct from media-type rejection. Catalog metadata
  infers domain `transport` and subsection `http/server` from the path; every
  top-level `Test*` needs a customer-readable Go doc so `functionaltestmetadata`
  stays viz-compatible.
  HTTP generated-client functional coverage belongs in
  `tests/functional/transport/http/server/generated_client_test.go`: prove status
  and Factory Session round-trips through the published generated HTTP client
  with caller-owned HTTP dependencies and cancellation/deadline bounds, prove
  representative structured API failures decode into typed client results, and
  prove generated client and server schemas stay aligned with the published
  OpenAPI contract against the live functional server. `make api-smoke` and
  `artifact-contract-closeout` run
  `TestGeneratedClientAndServerSchemaStayAligned` from this cell. Catalog
  metadata infers domain `transport` and subsection `http/server` from the path;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  HTTP API server OpenAPI routing functional coverage belongs in
  `tests/functional/transport/http/server/routing_test.go`: prove every
  published OpenAPI operation inventory entry reaches a non-404 handler through
  safe public HTTP requests against `support.StartFunctionalAPIServer` with
  `WaitForServiceModeRuntime: true` and `UseMockWorkers: true`, prove unknown
  paths outside the OpenAPI surface return structured `NOT_FOUND` JSON errors
  via `Server.NotFoundHandler`, and prove wrong HTTP methods on known routes
  return structured `405` `METHOD_NOT_ALLOWED` errors via
  `Server.MethodNotAllowedHandler` instead of not-found outcomes that would
  hide method mismatches. Catalog metadata infers domain `transport` and
  subsection `http/server` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  CLI JSON parameter values functional coverage belongs in
  `tests/functional/transport/cli/parameters/json_values_test.go`: prove nested
  JSON object and array named parameters reach canonical `InvocationArguments`
  intact through `SubmissionRecorder`, prove invalid JSON for a `typeHint: "JSON"`
  parameter fails with a named-parameter diagnostic and zero provider dispatch
  through `ProviderCommandRunner`, and prove JSON `null`, empty string, empty
  object, and empty array remain observably distinct without normalization loss
  at the public `support.BuildProcess` boundary. Catalog metadata infers domain
  `transport` and subsection `cli/parameters` from the path; every top-level
  `Test*` needs a customer-readable Go doc so `functionaltestmetadata` stays
  viz-compatible.
  CLI operator-default environment precedence functional coverage belongs in
  `tests/functional/transport/cli/parameters/environment_precedence_test.go`:
  prove explicit `--default-worker-model-provider` and `--default-worker-model`
  flags override conflicting `YOU_DEFAULT_WORKER_MODEL_PROVIDER` and
  `YOU_DEFAULT_WORKER_MODEL` environment values with `SourceCLIFlag` provenance
  on `CLIObserver` resolved inputs, prove environment overrides conflicting
  `~/.you-agent-factory/config.json` defaults with `SourceEnvironment` when no
  overriding flag is present, and prove unset operator-default environment
  variables fall back to global config with `SourceOperatorConfig` without
  fabricating `SourceEnvironment` overrides at the public
  `support.BuildProcess` boundary. Catalog metadata infers domain `transport`
  and subsection `cli/parameters` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  CLI response-stream backpressure functional coverage belongs in
  `tests/functional/transport/cli/output/stream_backpressure_test.go`:
  invoke `support.BuildProcess` with a gated or mid-stream-failing stdout writer
  on `you --json run … --output response-stream`, prove NDJSON Factory Event
  order and terminal `invocation_result` placement survive slow stdout drains,
  and prove stdout writer failure ends the invocation unsuccessfully while
  cancelling in-flight mock-worker external work through
  `edges.Edges.ProviderCommandRunner` with a runner that blocks until its
  context is cancelled. Response-stream stdout write failures cancel the
  invocation through `pkg/transports/cli/run/factory_invocation_input.go`, and
  worker-pool shutdown cancels in-flight executor contexts through
  `pkg/services/factory_runtime/internal/services/orchestration/runtime/worker_pool.go`. Catalog metadata infers domain
  `transport` and subsection `cli/output` from the path. Every top-level `Test*`
  needs a customer-readable Go doc so `functionaltestmetadata` stays
  viz-compatible.
  CLI docs command wiring functional coverage belongs in
  `tests/functional/transport/cli/commands/docs_wiring_test.go`: prove packaged
  topic discovery, index-driven non-empty topic rendering, and actionable
  unknown-topic failure through `support.BuildProcess` + `support.FakeInputs`
  from an isolated temp working directory without a local `docs/` tree; derive
  topics from the customer-visible packaged docs index stdout rather than
  scanning repository files or embedded registries; assert only transport
  discovery, render-wiring, and failure diagnostics without product/docs content
  contracts. Catalog metadata infers domain `transport` and subsection
  `cli/commands` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
  Prove default
  `functional-test-viz` wiring (boundary first, single coverage with profile
  + JSON under `.artifacts/functional-test-viz/`, Markdown generator) with
  dry-run / stubbed Make wrapper smoke under
  `tests/functional/observability/coverage/functional_test_viz_contract_test.go`
  rather than the full functional suite. Those Make helpers scrub ambient
  `FUNCTIONAL_TEST_VIZ_*` / `GO_FUNCTIONAL_COVERAGE_*` from the child Make
  environment so default-path assertions still hold when the suite inherits
  CI's `FUNCTIONAL_TEST_VIZ_DIR=.artifacts/backend-functional-coverage`.
  `make functional-boundary-check` also owns the deletion-only inventory of
  grandfathered `tests/functional/providers/*_test.go` files: existing entries
  must be removed in the same change as their files migrate so stale exceptions
  cannot authorize reintroduction. New provider scenarios must begin in a
  dedicated provider or provider-domain subpackage. The same boundary check
  rejects all service implementation and composition subpackage imports from
  dedicated provider packages, while retaining service-root contracts and the
  exact public external-effect ports used by typed `edges.Edges` replacements.
Wave 0 functional-tests-expansion planning authority lives under
  `docs/temp/functional-tests-expansion/` and is durable via a narrow
  `.gitignore` exception for that directory. Existing-scenario source→destination
  mapping is owned by `migration-ledger.md` (planning-only; later move batches
  consume its rows and deletion-only batch ids). The Inventory companion
  `migration-ledger-inventory.json` mirrors the same required row fields for
  tooling. Destination topology remains `test-file-checklist.md`; ownership
  rules remain `plan.md`.   `cmd/migrationledgercheck` validates live
  `tests/functional` inventory coverage, checklist destination validity, and
  lane preservation against the companion JSON. When a deletion-only batch is
  fully consumed (all scenarios owned by destination cells with
  `deletion_only_batch: n/a`; for example `runtime_api-delete-03-workstations-cron`),
  remove the source scenarios and ledger rows, add destination rows with
  `deletion_only_batch: n/a`, drop the batch id from
  `internal/migrationledgercheck/check.go` `ExpectedDeletionOnlyBatches`, mark the
  batch `released` in `migration-ledger.md`, update summary counts in
  `migration-ledger-inventory.json`, retarget any specialty Make bindings, and
  refresh `test-file-checklist.md` plus narrowly coupled baselines
  (`package-structure-baseline.json`, `functional-undocumented-tests.json`).
  `tests/functional/factory/definitions/init_test.go` owns public Factory-init
  functional coverage through `session create --init-new-factory` against
  `support.StartFunctionalAPIServer`, with seeded Work run via
  `support.RunFactoryToCompletionWithEdgesAndWork` and terminal assertions via
  `support.CountWorkAtCustomerState`; catalog metadata infers domain
  `factory/definitions`.
  `tests/functional/factory/definitions/defaults_test.go` owns operator
  default provider/model supply, Factory-authored override precedence, and
  single-discovered-provider fallback through `support.BuildProcess` +
  `support.StartProcessCommand` with operator `HOME` global config or
  `edges.WorkersExecutableLocator`, asserting resolved selection via
  `support.NewShapedProviderCommandRunner` call records; catalog metadata
  infers domain `factory/definitions`.
  `tests/functional/factory_definitions/transports/cli/named_lifecycle/named_lifecycle_test.go`
  owns named Factory create/list/update/delete, list membership after
  create/delete, and actionable delete-missing failure through
  `support.BuildProcess` + `support.FakeInputs` with isolated `--dir`
  catalog roots, asserting public CLI success/failure output and persisted
  `factory.json` presence or absence; catalog metadata infers domain
  `factory_definitions` and subsection `transports/cli/named_lifecycle`.
  Every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
  Factory Definitions CLI validate/persist depth belongs in
  `tests/functional/factory_definitions/transports/cli/validate_persist/validate_persist_test.go`:
  prove actionable public `you --json factory config validate` rejection before
  provider dispatch, failed validate non-mutation of durable named Factory state
  via `factory list` and on-disk `factory.json`, and persist-from-file through
  `factory create --from` followed by successful `you --json run --named
  --with-server` with terminal primary results. Drive proofs through
  `support.BuildProcess` + `Process.Execute`, substituting external effects
  only through `edges.Edges` with `ProviderCommandRunner` / mocked Codex
  preferred over `MockWorkers`. Catalog metadata infers domain
  `factory_definitions` and subsection `transports/cli` from the path; every
  top-level `Test*` needs a customer-readable Go doc so `functionaltestmetadata`
  stays viz-compatible.
  `make pkg-structure` enforces the domain-mirrored functional layout
  `tests/functional/<domain>/<subsection>/...`: new shallow, catch-all, or
  unclassified scenario packages are blocking, while existing nonconforming
  paths and `runtime_api` remain exact deletion-only debt in
  `docs/internal/baselines/package-structure-baseline.json`.
  `tests/functional/internal/support` is the only shared harness exception;
  other `tests/functional/internal/*` roots (for example `restclient`) are
  unclassified deletion-only debt, and new `runtime_api` files or top-level
  `Test*` scenarios fail immediately. `tests/functional/providers/<provider>`
  is an approved provider-specific domain path, while aggregate Go files
  directly under `tests/functional/providers` remain shallow deletion-only
  debt guarded by both package-structure and functional-boundary checks. Prove
  accept/reject outcomes with
  focused `cmd/pkgstructurecheck` tests (see
  `TestDomainLayoutEnforcementProof`) plus `make pkg-structure` and
  `make verify-fast`. When enabling a new layout rule, baseline the current
  repository debt in the same change so `make pkg-structure` stays green.
  When merging `main` into a branch, retain `main`'s reviewed package-minimum
  manifest entries unless the branch has independently regenerated and proven
  a stricter floor. Reintroducing a stale branch floor can turn a passing
  profile into a coverage-policy failure without any source behavior change.
  Factory-service command-runner overrides are resolved while composing the
  runtime worker application, including through `wire.InjectFactoryService`.
  Functional API tests should set the service-level provider and script
  overrides directly and assert the runner is invoked, rather than supplying
  a prebuilt worker application that bypasses this composition boundary.

- `cmd/packagetargetmanifestcheck` owns the Packaged Service Structure
  package-to-target and deletion manifest schema. The committed inventory lives
  at `docs/internal/packaged-service-structure/package-target-manifest.json` and
  may only use the closed destination vocabulary (13 product owners, approved
  non-service families, and the `edges` architecture exception). Nested
  destinations are limited to `<owner>/internal/services/<subservice>` using the
  plan's committed nested subservice names. `factory_runtime` unexpected siblings
  map to committed subservices in `nestedOwnerMoveRules` (`owners.go`): engine/runtime/state/scheduler/subsystems/token clusters and orchestration helpers → `orchestration`; `build` → `instance_host`; `checkpointstore`/`checkpointsummary` → `checkpoint_recovery`; owner-local residuals (`internal/factorystatus`, `internal/legacysnapshot`, `internal/service`, `internal/rootobservation`, `testkit`) → `factory_runtime/internal`. `workers` unexpected siblings map to `runtime_assembly` (`construction`), `workstations` (`prompting`, `worktree`, `skippermissions`), and `runners` (execution/executor/diagnostics/process/invocation/runner/interface plus `services/*` hosted paths); Providers extraction (`agypty`, `cliprovider`, `provider*`, `provider_test`) stays on the existing move→Providers mapper. `operator_settings` unexpected siblings map `identityinventory` → `document`; `servicewire`, `testlink`, and `testdata` → `operator_settings/internal`. `work` maps `stateaccessrecordings` → `state_access` and `testdata` → `work/internal`; transitional `service/` facades for Automations, Provider Sessions, and Work still use the legacy move→`<owner>/internal` mapper. Factory Definitions residual public dirs include `clonetests` and `systeminitializationtests` → `factory_definitions/internal` alongside the existing catalog/compilation/validation/snapshots/distribution rules; packages already under committed subservices (`internal/services/authoring_layout`, `catalog`, `compilation`, `validation`, `distribution`) retain at the subservice destination while `snapshots_portability` stays transitional move→`factory_definitions/internal`. Product-owner top-level sibling
  inventories (`cmd/packagetargetmanifestcheck/owner_top_level.go`, mirrored in
  `internal/ownershipinventory/owner_top_level.go`) classify immediate children
  under `pkg/services/<owner>/` as expected retain (`wire`, `internal`,
  `transports`, plus owner-specific retain exceptions such as Factory
  Definitions `namevalue`) or unexpected move siblings; keep `recordings_top_level`
  lists wired through the unified registry for INV-REC consistency. The top-level
  `inventory` array is
  the stable-sorted (byte-order / slash path) ledger seed of every production
  `pkg/` package (directories with at least one non-test `.go` file); regenerate
  with `go run ./cmd/packagetargetmanifestcheck -write-inventory`. Committed
  product-owner destination rows (including Providers extraction moves from
  `workers/provider*` / `cliprovider` / `agypty`) regenerate with
  `-write-owner-packages`. Process Edges rows retain destination `edges` as the
  sole broad external-effect architecture exception; regenerate with
  `-write-edges-packages`, which also records FND-06 Edges-narrowing
  `futureDebt` without performing that migration. Approved non-service family
  rows (`initializer`, `root`, `wire`, `platform`, `transports`) and any
  remaining residual deletion-queue mappings regenerate with
  `-write-residual-packages`; unknown residuals must not invent top-level
  owners. Focused validation requires exact one-destination coverage: every
  `inventory[]` path has exactly one stable-sorted `packages[]` row, and the
  checker fails on missing, duplicate, unsorted, closed-vocabulary, or incomplete
  delete-row mappings. Keep `make package-target-manifest-check` in default
  `make lint`. Regenerate the full committed manifest with
  `go run ./cmd/packagetargetmanifestcheck -write-inventory -write-owner-packages -write-edges-packages -write-residual-packages`
  and regenerate `docs/internal/baselines/ownership-inventory.json` with
  `go run ./cmd/ownershipinventoryfreeze` after mapping-rule changes. Keep validators beside this checker rather than inventing
  alternate destination trees.

- `cmd/packagedfactorysourcecheck` owns the static source-ownership gate for
  shipped first-party Factory documents. Keep it in the default `make lint`
  aggregation: it requires exactly one root Factory document per authored
  directory under `packages/packaged-factories/factories`, preserves the seven
  shipped identities, and rejects `@you/*` root definitions elsewhere while
  also rejecting production Go literals that embed a first-party definition.
  Examples, fixtures, test data, generated output, and the customer-authored
  repository `factory/` scaffold remain outside that classification.
  The package's passive embedded-filesystem tests run in `make test-maintenance`,
  matching the repository-boundary checker that protects its authored data
  ownership. Focused command-exit failure evidence lives in
  `cmd/packagedfactorysourcecheck/guard_failure_behavior_test.go`.
- `cmd/packagedfactoryconsumptioncheck` owns the static consumption gate for
  shipped first-party Factory definition bytes. Keep it in the default
  `make lint` aggregation: production Go may import
  `packages/packaged-factories` only from the embed package itself and
  `internal/packagedfactorycatalog`, and may call
  `packagedfactorycatalog.LoadPublishedDefinitionCatalog` only from the
  approved wiring and catalog-projection files
  (`pkg/wire/profiles.go`, `pkg/transports/http/handlers_models.go`,
  `pkg/services/factory_definitions/packages/goal/prompt_drift.go`). Factory
  Definitions list/resolve/install must continue to consume detached catalog
  bytes through `PackagedFactoryCatalogOperations` rather than alternate embed
  or filesystem paths.   Focused unknown-identity resolve/install rejection
  evidence lives in `pkg/wire/packaged_factory_guard_failure_test.go` and
  `tests/functional/product/packaged_factory_guard_failure/`.
  First-party definition guard closeout verification runs, in order,
  `make packaged-factory-source-check`, `make packaged-factory-consumption-check`,
  `make packaged-factory-catalog-check`, and `make packaged-factory-package-verify`
  (or the focused `packaged-factory-package-consumer-smoke` subset for
  clean-consumer proof). No additional coverage, ownership, or package-target
  manifest edits are required when those gates already register the new cmd
  surfaces through default `make lint`.
  Public Go packages under `packages/` are not selected by the default unit
  lane. Classify passive publication boundaries explicitly in
  `internal/testlanes` and list them in `make test-maintenance`; the
  `packages/model-providers` byte-parity and detached-caller tests use this
  path.
  The portable catalog generator is
  `cmd/packagedfactorycataloggenerate`; its repository surface is
  `make packaged-factory-catalog-generate`. It computes and validates the whole
  output set before atomically replacing
  `packages/packaged-factories/generated`. The npm data-only package allowlist
  publishes its manifest, flattened generated Factory pairs, schemas, and
  package documentation while excluding authored Factory sources, prompts, and
  scripts. `cmd/packagedfactorycatalogcheck` recomputes that same complete plan
  without writing, reports sorted package-relative stale, missing, and
  unexpected outputs with the regeneration remedy, and runs through
  `make packaged-factory-catalog-check` in the default lint aggregation.
  PSS-F01 ownership freeze gating lives in `internal/ownershipinventory`:
  per-owner `*_mapping.go` helpers mirror `cmd/packagetargetmanifestcheck`
  `nestedOwnerMoveRules` (Recordings, Factory Definitions, Factory Runtime,
  Workers, Work, and Operator Settings) and emit move rows with concrete
  `successor` paths rather than prefix-retaining transitional public siblings.
  Cross-owner retain-rejection guards live in
  `cmd/packagetargetmanifestcheck/unexpected_sibling_retain_test.go` and
  `internal/ownershipinventory/unexpected_sibling_retain_test.go` (plus
  `internal/ownershipinventory/remaining_owners_test.go`); they sweep inventoried
  unexpected siblings and fail on deliberate retain→owner-root mappings.
  Factory Definitions invocation-policy inventory locks live in
  `internal/ownershipinventory/factory_definitions_invocation_policy_lock_test.go`
  and mirrored residual-destination cases in
  `cmd/packagetargetmanifestcheck/factory_definitions_test.go`; they assert the
  nested `factory_definitions/invocation_policy` rationale card, dual-ledger move
  targets for residual policy packages, snapshots_portability retain rows, and
  absence of inventory deletes for the packet.
  `make ownership-inventory-check` runs `VerifyFreeze` against
  `docs/internal/baselines/ownership-inventory.json` and
  `docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json`,
  proving completeness, stable sort, rationale fields, edge classifications,
  named-owner coverage, Process Edges exception presence, and non-overlapping
  active leases. Keep that check in the default lint aggregation so PSS-F02
  starts from a proven freeze rather than prose-only claims.
  Packaged Factory npm candidates use the shared release identity and staging
  core in `scripts/package-release-candidate.mjs` through
  `scripts/packaged-factories-package-candidate.mjs`; keep its run ID, full
  source commit, candidate-version policy, staged manifest provenance, and
  evidence shape aligned with `scripts/api-package-candidate.mjs`. The focused
  behavioral coverage runs in `make packaged-factory-package-smoke`.
  `scripts/packaged-factories-package-pack.mjs` owns the manifest-derived exact
  tarball inventory and portable-file boundary after that drift gate; keep npm
  lifecycle scripts disabled and reject missing, unexpected, stale, escaping,
  symlinked, externally dependent, or digest-mismatched candidate contents.
  `scripts/packaged-factories-package-consumer.mjs` owns the clean installed
  data-contract proof: create the consumer outside the workspace, disable npm
  scripts, lockfiles, workspaces, and links, resolve artifacts only through
  public package specifiers, and validate both generated representations
  against the installed schema before removing the consumer.
  The data-package development policy and dry-run helpers remain available as
  focused local verification tools, but the Development Package workflow does
  not prepare or publish API or Packaged Factories candidates. Protected-main
  development publication preserves and publishes only the frontend family at
  `0.0.0-dev.<run-id>.<reviewed-source-commit>`.
  `scripts/public-release-package-candidate.mjs` prepares the tagged-release
  candidate set from the successful release-candidate workflow's exact head
  commit: API, Packaged Factories, and the canonical frontend family share one
  stable release version, preserve source manifests and frontend build outputs,
  and are recorded exactly once in `release-candidate-evidence.json`. The
  shared `scripts/package-export-validation.mjs` check rejects any packed API,
  Packaged Factories, or frontend candidate missing a concrete export target or
  every match for a wildcard target. Keep the legacy
  `factories/goal/factory.json` compatibility artifact as a separate Packaged
  Factories required-file check, outside the general export-map contract, and
  keep lifecycle scripts disabled for every candidate pack. Prove that behavior
  at the API, Packaged Factories, and frontend production packing boundaries
  with lifecycle-hook sentinel fixtures; command-argument inventory assertions
  are not sufficient release evidence.
  tagged Release workflow uploads that complete set as one artifact. Its
  `scripts/public-release-package-publish.mjs` boundary rejects unknown scopes,
  duplicates, missing or extra packages, source-commit drift, and child
  evidence or tarball paths that do not match the reviewed top-level evidence,
  and preflights every represented tarball against all recorded artifact
  digests before publishing any package. The frontend publisher accepts only the
  `frontend-only` development scope, while the protected tagged publisher
  requires the complete `tagged-release` scope. Local
  maintainers can isolate generation, drift, script tests, exact packing,
  pull-request dry-run, and clean-consumer behavior through the focused
  `packaged-factory-*` Make targets documented in
  `docs/internal/development/cli-release-policy.md`.

  Local concurrent lane scripts must redirect each background command directly
  to its retained log, wait on that command, and replay the log afterward. Do
  not put background commands behind live `while`/`tee` pipelines: on Windows,
  detached descendants can inherit the pipe handle after the tested command
  exits and prevent the repository-owned verification target from terminating.
  When a change adds a measurable Go package to either profile, add its
  package-specific minimum to the matching
  `docs/internal/baselines/go-*-coverage-package-minimums.json` manifest in
  the same change; the coverage gate rejects unowned measured packages.
  Optional machine-readable coverage summaries for CI/visualizer consumers come
  from `gocoveragecheck -json-output <path>` after a completed measurement run.
  The JSON includes overall totals plus per-package covered/measurable counts,
  percentage, package floor, and measurement exception from the same gate
  policy the checker already enforces; do not invent a second coverage-profile
  parser for those summaries. When floors fail after measurement completes, the
  JSON file is still written before the process returns the floor failure so
  uploaded artifacts keep diagnostics; incomplete runs that never produce
  measured results do not invent coverage JSON.
  Windows Go suite coverage is a `windows-go-tests` matrix with independent
  `Unit`, `Functional`, `Stress`, and `Release` jobs. Keep `fail-fast: false`,
  preserve each job's Windows setup, and invoke the matching repository-owned
  local command (`make test-unit`, `make test-functional`, `make test-stress`,
  or `make test-release`). Each lane writes that command to its job summary so
  a failed CI result has a direct local rerun. In `pwsh` summary steps, write
  expanded GitHub expressions as plain text rather than surrounding them with
  PowerShell backticks, which can escape the closing quote after expansion.

- Provider-session golden fixtures live under
  `docs/temp/functional/provider-sessions/**`. Keep that path narrowly
  un-ignored after the general `docs/temp/**` rule by re-including each parent
  directory (`!docs/temp/functional/`, `!docs/temp/functional/provider-sessions/`,
  then `!docs/temp/functional/provider-sessions/**`). Prove the exception with
  `git check-ignore -q` (exit 1 = not ignored, exit 0 = ignored) plus a sibling
  `docs/temp/...` path that remains ignored. Shared helpers and fixture-root
  constants belong in `tests/functional/internal/support`.
- Provider-session golden `manifest.json` validation lives in
  `tests/functional/internal/support` (`LoadProviderSessionCaseManifest` /
  `ValidateProviderSessionGoldenManifest`). Require schema version 1, identity
  (`id`, `provider`, `providerVersion`, `case`), `fidelityClass` in
  `{full-stream, partial-stream, snapshot-only, final-only}`, sanitizer/source,
  `normalizedFields`, and relative file pointers for request/process/stdout/
  stderr plus the three expected outputs. Diagnostics must name the case id and
  failing field or rule; pointer resolution must stay inside the case directory.
  Reject slash-root pointers with `path.IsAbs` as well as native paths with
  `filepath.IsAbs`, because Windows does not classify `/tmp/...` as an absolute
  native path even though the portable manifest contract must reject it.
- Provider-session golden sanitization (`ValidateProviderSessionCaseSanitization`
  / `ValidateProviderSessionFixtureContent`) rejects unsanitized fixture material
  with named categories: `credential`, `host-path`, `private-repo-url`,
  `env-dump`, `unbounded-content`, and `account-identifier`. Diagnostics must
  name the category plus fixture path or JSON field. Retain sanitized structural
  values (fake session/tool/item IDs, usage counts, finish reasons, error codes,
  `@example.com` emails). Run the gate after manifest validation and before
  golden comparison.
- Provider-session golden loading (`LoadProviderSessionCase`) runs
  manifest → sanitization → request/process/stdout/stderr → expected goldens.
  `process.json` must expose argv (no secrets), provider/model, exitCode and/or
  signal, stdout/stderr stream flags, `workingDirectoryRole`,
  `timeoutCancelClass`, and `terminalErrorClass` without an env dump. Stdout
  media type follows the declared filename (`*.jsonl`/`*.ndjson`, `*.json`, or
  text). Expected response events decode as NDJSON records. Load failures use
  `ProviderSessionLoadError` naming case id, role, and path/field.
- Provider-session golden comparison (`CompareProviderSessionGoldens`) normalizes
  only field names listed in `manifest.normalizedFields` (any depth) to
  `<normalized>`, then structurally compares Provider Session JSON, response-
  event NDJSON records, and invocation-result JSON. Whitespace-only differences
  do not fail. Callers supply observed public metadata; comparison must never
  synthesize expected output by calling the mapper/adapter under test.
  Mismatches use `ProviderSessionCompareError` naming case id, artifact role,
  and JSON path.
- Provider-session golden update gating (`CompareOrUpdateProviderSessionGoldens`)
  fails on drift without rewriting unless `UPDATE_FUNCTIONAL_GOLDENS=1`. With that
  env set, the helper may rewrite the three expected golden files from observed
  values and returns `ProviderSessionGoldensUpdatedError` so CI still fails until
  a non-update re-run passes. Missing required fixtures fail with
  `ProviderSessionLoadError` naming case id, role (`request`, `process`, `stdout`,
  `stderr`, `expected-provider-session`, `expected-response-events`,
  `expected-invocation-result`), and path—never silent skip.
  Cursor failure goldens under `docs/temp/functional/provider-sessions/cursor/`
  (`malformed-record`, `process-failure`, `timeout`) replay through
  `tests/functional/workers/inference/cursor/golden_failure_test.go`. Use
  `stdout.txt` when fixtures include non-JSON stream lines; `.jsonl` loaders
  reject invalid JSON per line.   Retryable timeout cases must queue multiple
  identical `ProviderCommandRunner` results so retries do not fall through to
  the default mock.
- Workers inference provider/model selection functional coverage belongs in
  `tests/functional/workers/inference/selection_test.go`: prove explicit
  worker `modelProvider` and `model` dispatch invoke only the matching
  registered `ProviderRegistrations` edge and complete factory dispatch, prove
  worker-authored providers override operator defaults when both edges are
  registered, and prove unknown provider aliases fail factory startup with a
  stable validation error before any registered provider edge is invoked.
  Drive proofs through `support.RunFactoryToCompletionWithEdgesAndObservations`
  or `support.BuildProcess` with `serviceedges.Edges{ProviderRegistrations: ...}`
  and assert on integration stats plus public Work outcomes only. Catalog
  metadata infers domain `workers` and subsection `inference` from the path;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.
- Workers-owned CLI run output-mode functional coverage belongs in
  `tests/functional/workers/transports/cli/run/modes/output_modes_test.go`:
  drive public `you run` through `support.BuildProcess` + `support.FakeInputs`
  with `serviceedges.Edges` populated via `support.ConfigureWorkerCommands` and
  `support.NewStaticSuccessCommandRunner` (preferred over `--with-mock-workers`);
  scaffold a minimal model-worker factory with `support.ScaffoldFactory` and
  `support.BuildModelWorkerConfig`; prove quiet text, single-JSON, and NDJSON
  (`--json --output response-stream`) primary-result fidelity on stdout only.
  Place `--quiet` and `--output` on the `run` subcommand, not as global flags.
  Catalog metadata infers domain `workers` and subsection
  `transports/cli/run/modes` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
- Workers-owned CLI run lifecycle functional coverage belongs in
  `tests/functional/workers/transports/cli/run/lifecycle/lifecycle_test.go`:
  drive clean/prompt-style public `you run --factory` through
  `support.BuildProcess` + `support.FakeInputs` with `serviceedges.Edges`
  populated via `support.ConfigureWorkerCommands` and
  `support.NewStaticSuccessCommandRunner` (preferred over `--with-mock-workers`);
  scaffold a minimal Codex model-worker factory with `support.ScaffoldFactory`
  and `support.BuildModelWorkerConfig`; prove default primary-result stdout is
  pipeable and free of dashboard open/startup sidecar chatter. For
  server-attached targeting, start an already-open Factory Session with
  `support.StartFunctionalAPIServer` + `WaitForServiceModeRuntime`, optionally
  open an explicit session through `support.OpenFactorySessionAt`, then issue
  `you --server <url> run --factory ...` (no `--with-server`) and correlate the
  unchanged default session identity through public `session show` / session GET
  reads. For clean-invocation failure, use
  `support.NewShapedProviderCommandRunner` with a deterministic non-zero exit
  and stderr payload, assert empty stdout without false-success primary result,
  and decode exactly one stderr `ErrorResponse` with actionable code/message.
  Catalog metadata infers domain `workers` and subsection
  `transports/cli/run/lifecycle` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
- Packaged `@you/tts` invocation functional coverage belongs in
  `tests/functional/factory/packaged/tts/invocation_test.go`: prove required-text
  audio artifact metadata, optional voice/format reachability on fake provider
  bindings, and model-failure no-false-artifact contracts through the public API
  invocation boundary with `support.InstallPackagedFactory`,
  `ProviderOverride`, and scaffold inline topology that removes split worker
  sidecars. Catalog metadata infers domain `factory` and subsection
  `packaged/tts` from the path; every top-level `Test*` needs a customer-readable
  Go doc so `functionaltestmetadata` stays viz-compatible.
- Workers inference provider command-flag functional coverage belongs in
  `tests/functional/workers/inference/flags_test.go`: prove skip-permissions
  policy, resolved worktree names, and explicit model values map onto the
  selected provider-process command args, and prove unsupported provider flags
  (for example workstation `outputSchema` on Gemini) fail with a capability
  error before any `ProviderCommandRunner` call. Drive proofs through
  `support.RunFactoryToCompletionWithEdgesAndObservations` with
  `serviceedges.Edges{ProviderCommandRunner: ...}` and assert on command args
  plus public Work outcomes only. Catalog metadata infers domain `workers` and
  subsection `inference` from the path; every top-level `Test*` needs a
  customer-readable Go doc so `functionaltestmetadata` stays viz-compatible.
- Workers inference provider stream-fidelity functional coverage belongs in
  `tests/functional/workers/inference/stream_fidelity_test.go`: prove full-stream
  providers publish truthful message deltas and completed snapshots (native-stream
  delivery, no final-only fabrication), partial-stream providers do not fabricate
  missing message deltas, snapshot-only providers emit completed snapshots only
  (zero deltas, no final-only), and final-only providers emit only the terminal
  completed message (`FINAL_ONLY` / `NATIVE_FINAL`, zero message deltas). Drive
  proofs through `support.RunFactoryToCompletionWithEdgesAndResponseEvents` with
  sanitized FND-006 provider-session goldens replayed via
  `serviceedges.Edges{ProviderCommandRunner: ...}` (and OpenCode snapshot-only
  executable-locator edges when required) and assert on public
  `FactoryResponseEvent` provenance plus terminal Work outcomes only. Catalog
  metadata infers domain `workers` and subsection `inference` from the path;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.

- `pkg/services/provider_sessions/packaged_root_shape_test.go` and
  `pkg/services/provider_sessions/service_import_boundary_test.go` seal FUN-provider-sessions
  packaged-service shape and production peer import boundaries: Provider Sessions
  ships only `wire/`, `internal/`, and `transports/` package directories plus thin
  root contracts, `service/` stays absent, and production peers import only the
  published Provider Sessions root except the documented `pkg/wire` injector seam.

- `tests/functional/provider_sessions/peer_import_boundary_test.go` owns
  Provider Sessions FUN functional import seal: every package under
  `tests/functional/provider_sessions/...` must construct through
  `root.BuildProcess` / shared functional support and must not import
  `pkg/services/provider_sessions/internal` or deleted
  `pkg/services/provider_sessions/service`. Complements inert-construction and
  post-lifecycle behavioral proofs; does not replace them. Catalog metadata
  infers domain `provider_sessions` and subsection `root_composition`; every
  top-level `Test*` needs a customer-readable Go doc so `functionaltestmetadata`
  stays viz-compatible.

- `tests/functional/provider_sessions/build_process_inert_test.go` owns
  Provider Sessions inert-construction proof through `support.BuildProcess` /
  `root.BuildProcess`. Replace `serviceedges.Edges` Provider Session ports
  (`ProviderSessionResolveHomeDirectory`, `ProviderSessionFileSystem`,
  `ProviderSessionCodexWalkDirectory`, `ProviderSessionCodexResolveSymlinks`,
  `ProviderSessionCursorWalkDirectory`, `ProviderSessionCursorResolveSymlinks`,
  `ProviderSessionCursorOpenDatabase`) with recording stubs and assert directory
  walks, symlink resolution, filesystem opens, and Cursor database opens stay at
  zero during composition. Root path derivation may call the home resolver and
  filesystem `Stat` without session discovery; do not treat
  `packaged_root_shape` or `del_pses_*` unit gates as substitutes. Catalog
  metadata infers domain `provider_sessions` and subsection `root_composition`;
  every top-level `Test*` needs a customer-readable Go doc so
  `functionaltestmetadata` stays viz-compatible.

- `tests/functional/provider_sessions/association/association_test.go` owns
  Provider Session ref correlation on public Factory Session dispatch projections
  after runtime lifecycle starts (FUN post-lifecycle association activation).
  Drive proofs through `support.StartFunctionalAPIServer` (`root.BuildProcess` +
  `edges.Edges` only) with a JavaScript
  `agent.run` fake-child workflow (no live provider runner calls), then assert on
  `GET /factory-sessions/{session_id}/dispatches` and
  `GET /factory-sessions/{session_id}/dispatches/{dispatch_id}` only:
  `providerSessionRefs`, owning dispatch `id`, and list/detail `sessionId`
  correlation.   Use a successful fake child for present-ref association and a
  deterministic `fail:` fake-child prompt for absent-ref non-fabrication (nil or
  empty `providerSessionRefs` on list/detail, not `fake-provider-session-1`).
  For distinct multi-dispatch refs, run two successful `agent.run` children in
  one workflow and assert `fake-provider-session-1` / `fake-provider-session-2`
  join to their owning dispatch ids without collision. Do not invent provider
  metadata or widen into `details/*` or
  `response_exec_metadata_test.go`. Catalog metadata infers domain
  `provider_sessions` and subsection `association`; every top-level `Test*`
  needs a customer-readable Go doc so `functionaltestmetadata` stays
  viz-compatible.

- `tests/functional/provider_sessions/association/response_exec_metadata_test.go`
  owns response-exec golden metadata survival across CLI Factory Event projection,
  API `FactoryResponseEvent` history, and replay observation. Drive proofs through
  `support.RunFactoryToCompletionWithEdgesAndResponseEvents` or
  `support.StartFunctionalAPIServer` with `--record` / `--replay`, replaying
  sanitized FND-006 Codex goldens via `serviceedges.Edges{ProviderCommandRunner:
  ...}` and asserting against checked-in provider-session, invocation-result, and
  response-event golden metadata with `support.CompareProviderSessionJSON` /
  `CompareProviderSessionNDJSON` only (never production mappers under assertion).
  Do not widen into `association_test.go` correlation scenarios or `details/*`.
  Catalog metadata infers domain `provider_sessions` and subsection `association`;
  every top-level `Test*` needs a customer-readable Go doc and `//golden:` manifest
  directive so `functionaltestmetadata` stays viz-compatible.

- `tests/functional/provider_sessions/details/codex_details_test.go` owns Codex
  Provider Session detail inspection through the public `GET /provider-sessions/detail`
  boundary after runtime lifecycle starts (FUN post-lifecycle detail activation).
  Drive proofs through `support.StartFunctionalAPIServer` (`root.BuildProcess` +
  `edges.Edges` only) with `ProviderSessionResolveHomeDirectory` edge override.
  Write sanitized Codex rollout fixtures under `~/.codex/sessions/...`, compare
  success detail to `docs/temp/functional/provider-sessions/codex/success/expected-provider-session-detail.json`
  with normalized timestamps/size fields, assert missing session ids return HTTP 404
  `NOT_FOUND` without fabricated detail, and assert corrupt rollouts surface parse
  diagnostics without fabricated transcript or host-path leakage. Do not widen into
  `cursor_details_test.go` or `http_test.go`. Close catalog metadata with
  customer-readable Go docs (plus `//golden:` on the success load test) so
  `functionaltestmetadata` stays viz-compatible.

- `tests/functional/provider_sessions/details/http_test.go` owns HTTP/API Provider
  Session detail contracts through the public `GET /provider-sessions/detail` boundary
  after runtime lifecycle starts (FUN post-lifecycle detail activation) via
  `support.StartFunctionalAPIServer` (`root.BuildProcess` + `edges.Edges` only) with
  `ProviderSessionResolveHomeDirectory` edge override and without MockWorkers when
  sanitized on-disk fixtures suffice. Prove golden-backed detail matches checked-in
  expected metadata (`//golden:` on the success load test), reject raw filesystem path
  input with typed `BAD_REQUEST` errors, and return typed `BAD_REQUEST` for unsupported
  provider session kinds without fabricating detail bodies. Reuse same-package Codex/Cursor
  fixture helpers from sibling cells; do not widen into `codex_details_test.go` or
  `cursor_details_test.go`. Close catalog metadata with customer-readable Go docs so
  `functionaltestmetadata` stays viz-compatible.

- `tests/functional/provider_sessions/details/cursor_details_test.go` owns Cursor
  Provider Session detail inspection through the public `GET /provider-sessions/detail`
  boundary after runtime lifecycle starts (FUN post-lifecycle detail activation) via
  `support.StartFunctionalAPIServer` (`root.BuildProcess` + `edges.Edges` only) with
  `ProviderSessionResolveHomeDirectory` edge override. Write sanitized Cursor sqlite
  fixtures under `~/.cursor/chats/{workspaceHash}/{sessionID}/store.db`, compare
  success detail to `docs/temp/functional/provider-sessions/cursor/success/expected-provider-session-detail.json`
  with `modifiedAt` and `sizeBytes` normalized, assert unavailable blobs surface
  `unknownEventCount`/`unknownEvents` without fabricated transcript, and assert
  missing session ids return HTTP 404 `NOT_FOUND` without fabricated detail bodies.
  Do not widen into `codex_details_test.go` or `http_test.go`. Close catalog metadata
  with customer-readable Go docs (plus `//golden:` on the success load test) so
  `functionaltestmetadata` stays viz-compatible.

- `tests/functional/factory_runtime/orchestrators/petri/routing/multi_transition_test.go`
  owns service-mirrored Factory Runtime Petri multi-transition routing depth
  through `support.RunFactoryToCompletionWithEdgesAndWork` and public Work /
  session / Factory Event assertions only. Close catalog metadata with
  `test-file-checklist.md`, `migration-ledger-inventory.json`,
  `package-structure-baseline.json` entries for the `factory_runtime` domain
  noun, and customer-readable Go docs on every top-level `Test*` so
  `functionaltestmetadata` stays viz-compatible.

- `tests/functional/automations/` owns root.BuildProcess evidence for packaged
  Automations cron scheduling and filesystem watcher preseed. Keep cron workstation
  factories explicit with `"behavior": "CRON"` and observe submissions through
  `serviceedges.Edges.SubmissionRecorder` on `support.StartFunctionalAPIServer`,
  matching the runtime_api cron smoke helpers. For filesystem watchers, scaffold
  factories with workstation `inputs`, seed `inputs/<workType>/default/` before
  `StartFunctionalAPIServer`, and assert preseed submissions through the same
  recorder seam rather than importing parent-private `filesystem_watchers` packages.

- `tests/functional/automations/hosted_sources_root_composition_test.go` owns
  root.BuildProcess inert-construction evidence for Automations hosted Linear
  polling. Assert zero `SubmissionRecorder` submissions after `support.BuildProcess`
  before runtime lifecycle starts, matching the cron inert-construction pattern.

- `tests/functional/automations/script_poller_root_composition_test.go` owns
  root.BuildProcess inert-construction and post-lifecycle admission evidence for
  Automations script pollers. Assert zero `ScriptCommandRunner` invocations after
  `support.BuildProcess` before runtime lifecycle starts, then replace only
  `serviceedges.Edges.ScriptCommandRunner` on `support.StartFunctionalAPIServer`
  and observe polled Work through the public Work listing path.

- `tests/functional/automations/reconciliation_root_composition_test.go` owns
  root.BuildProcess inert-construction and post-composition Automations Root
  reconciliation admission. Assert zero `SubmissionRecorder` submissions after
  `support.BuildProcess` before explicit Root invocation, then obtain the published
  Root through `support.AutomationsRootFromProcessEdges` (backed by
  `wire.AutomationsRootFromEdges`) and assert converged or created reconcile
  outcomes without importing `automations/internal`, `automations/wire`, or deleted
  `automations/service`.

- `pkg/services/automations/packaged_root_shape_test.go` and
  `pkg/services/automations/peer_import_boundary_test.go` seal FUN-automations
  packaged-service shape and production peer import boundaries: Automations ships
  only `wire/`, `internal/`, and `transports/` package directories plus thin root
  contracts, `service/` stays absent, and production peers import only the
  published Automations root except the documented `pkg/wire` injector seam.

- `tests/functional/automations/peer_import_boundary_test.go` seals FUN-scoped
  functional proofs against `automations/internal`, `automations/wire`, and deleted
  `automations/service` imports while allowing `support.BuildProcess`,
  `support.AutomationsRootFromProcessEdges`, and published `automations` contracts.
