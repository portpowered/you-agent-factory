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
  `/dispatches` without wall-clock sleeps. Catalog metadata infers domain
  `orchestration` and subsection `javascript/composition` from the path; every
  top-level `Test*` needs a customer-readable Go doc so `functionaltestmetadata`
  stays viz-compatible.
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
  `deletion_only_batch: n/a`), remove its id from
  `internal/migrationledgercheck.ExpectedDeletionOnlyBatches` and mark the
  batch `released` in `migration-ledger.md`.
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
  plan's committed nested subservice names. The top-level `inventory` array is
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
  `make lint`. Keep validators beside this checker rather than inventing
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
  ownership.
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
