# CI Relevant Files

## Pull-request verification workflow

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
  Backend Unit Coverage and Backend Functional Coverage are an independent,
  fail-fast-disabled matrix gated only by Classify PR Impact. Preserve the
  explicit run/intentional-skip summary, per-suite coverage thresholds,
  diagnostics, and artifacts; neither lane may depend on Build, Lint, or API.
  Their exact local reruns are `make test-unit-coverage` and
  `make test-functional-coverage`. Both commands serialize Go package coverage
  writers before canonicalizing repeated source blocks into one sorted profile;
  keep that ordering so concurrent packages cannot corrupt the shared profile
  and the uploaded artifact matches the totals enforced by the lane.
  When merging `main` into a branch, retain `main`'s reviewed package-minimum
  manifest entries unless the branch has independently regenerated and proven
  a stricter floor. Reintroducing a stale branch floor can turn a passing
  profile into a coverage-policy failure without any source behavior change.
  Factory-service command-runner overrides are resolved while composing the
  runtime worker application, including through `wire.InjectFactoryService`.
  Functional API tests should set the service-level provider and script
  overrides directly and assert the runner is invoked, rather than supplying
  a prebuilt worker application that bypasses this composition boundary.
  Local concurrent lane scripts must redirect each background command directly
  to its retained log, wait on that command, and replay the log afterward. Do
  not put background commands behind live `while`/`tee` pipelines: on Windows,
  detached descendants can inherit the pipe handle after the tested command
  exits and prevent the repository-owned verification target from terminating.
  When a change adds a measurable Go package to either profile, add its
  package-specific minimum to the matching
  `docs/internal/development/go-*-coverage-package-minimums.json` manifest in
  the same change; the coverage gate rejects unowned measured packages.
  Windows Go suite coverage is a `windows-go-tests` matrix with independent
  `Unit`, `Functional`, `Stress`, and `Release` jobs. Keep `fail-fast: false`,
  preserve each job's Windows setup, and invoke the matching repository-owned
  local command (`make test-unit`, `make test-functional`, `make test-stress`,
  or `make test-release`). Each lane writes that command to its job summary so
  a failed CI result has a direct local rerun. In `pwsh` summary steps, write
  expanded GitHub expressions as plain text rather than surrounding them with
  PowerShell backticks, which can escape the closing quote after expansion.
