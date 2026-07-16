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
  `make test-functional-coverage`. When a newly measured package enters either
  profile, add its import-path-sorted entry to the matching
  `docs/internal/development/go-*-coverage-package-minimums.json` manifest.
  Use a measurement exception only when the active profile has no measurable
  statements; otherwise record the package's verified baseline.
  Windows Go suite coverage is a `windows-go-tests` matrix with independent
  `Unit`, `Functional`, `Stress`, and `Release` jobs. Keep `fail-fast: false`,
  preserve each job's Windows setup, and invoke the matching repository-owned
  local command (`make test-unit`, `make test-functional`, `make test-stress`,
  or `make test-release`). Each lane writes that command to its job summary so
  a failed CI result has a direct local rerun. In `pwsh` summary steps, write
  expanded GitHub expressions as plain text rather than surrounding them with
  PowerShell backticks, which can escape the closing quote after expansion.
  Compiled-binary release-smoke commands must use a named timeout that leaves
  room for Windows process startup and teardown; keep service-readiness bounds
  separate so a slow command cannot mask a service startup regression.
