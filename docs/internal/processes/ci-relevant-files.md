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
  workflow runs this command as a read-only pull-request dry run, publishes an
  immutable `dev` version after protected `main` succeeds, and the tagged
  release workflow publishes the release semver under `latest`. Publication
  uses npm trusted publishing and verifies every exact version after upload.

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
  Their exact local reruns are `make test-unit-coverage` and
  `make test-functional-coverage`. Both commands serialize Go package coverage
  writers before canonicalizing repeated source blocks into one sorted profile;
  keep that ordering so concurrent packages cannot corrupt the shared profile
  and the uploaded artifact matches the totals enforced by the lane.
  The default functional and functional-coverage lanes share dynamic package
  selection through `internal/testlanes`: retain execution of the complete
  `go list ./tests/functional/...` result, the shared-support exclusion, and
  required provider-destination validation in both callers. Required package
  validation protects topology but must not become an execution allowlist.
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
  rules remain `plan.md`. `cmd/migrationledgercheck` validates live
  `tests/functional` inventory coverage, checklist destination validity, and
  lane preservation against the companion JSON.
  `make pkg-structure` enforces the domain-mirrored functional layout
  `tests/functional/<domain>/<subsection>/...`: new shallow, catch-all, or
  unclassified scenario packages are blocking, while existing nonconforming
  paths and `runtime_api` remain exact deletion-only debt in
  `docs/internal/baselines/package-structure-baseline.json`.
  `tests/functional/internal/support` is the only shared harness exception;
  other `tests/functional/internal/*` roots (for example `restclient`) are
  unclassified deletion-only debt, and new `runtime_api` files or top-level
  `Test*` scenarios fail immediately. Prove accept/reject outcomes with
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
  Pull-request authorization is shared through
  `scripts/package-development-policy.mjs`; keep the API compatibility re-export
  and require every candidate to match the reviewed full head SHA after
  prerequisites succeed. The Development Package pull-request job runs
  `scripts/packaged-factories-package-pr-dry-run.mjs` without registry access
  and preserves the exact tarball, candidate evidence, consumer evidence, and
  no-publish outcome for review. Keep generation, identity, inventory, pack,
  and installed-consumer failures stage-specific.
  Protected-main publication must download that package's preserved candidate,
  rebind its evidence source commit to the protected workflow head, and use the
  shared `scripts/package-registry.mjs` and
  `scripts/package-publication.mjs` mechanics. Package wrappers own exact npm
  identity and installed-consumer semantics; shared orchestration owns local
  digest verification, immutable-version reconciliation, publish-at-most-once
  behavior, and bounded retries for transient lookup, download, visibility, and
  registry-consumer install failures. Candidate identity/digest, immutable
  conflict, registry integrity, authentication, permission, and installed-data
  contract failures remain fail-fast and retain classified diagnostics.
  The tagged Release workflow prepares API and Packaged Factories candidates
  together from the successful release-candidate workflow's exact head commit,
  uploads them under separate artifact names, and publishes only those
  downloaded directories after rechecking their source commit. Local
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
  Windows Go suite coverage is a `windows-go-tests` matrix with independent
  `Unit`, `Functional`, `Stress`, and `Release` jobs. Keep `fail-fast: false`,
  preserve each job's Windows setup, and invoke the matching repository-owned
  local command (`make test-unit`, `make test-functional`, `make test-stress`,
  or `make test-release`). Each lane writes that command to its job summary so
  a failed CI result has a direct local rerun. In `pwsh` summary steps, write
  expanded GitHub expressions as plain text rather than surrounding them with
  PowerShell backticks, which can escape the closing quote after expansion.
