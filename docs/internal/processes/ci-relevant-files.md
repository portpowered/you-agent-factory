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
  `make test-functional-coverage`.
  Windows Go suite coverage is a `windows-go-tests` matrix with independent
  `Unit`, `Functional`, `Stress`, and `Release` jobs. Keep `fail-fast: false`,
  preserve each job's Windows setup, and invoke the matching repository-owned
  local command (`make test-unit`, `make test-functional`, `make test-stress`,
  or `make test-release`). Each lane writes that command to its job summary so
  a failed CI result has a direct local rerun. In `pwsh` summary steps, write
  expanded GitHub expressions as plain text rather than surrounding them with
  PowerShell backticks, which can escape the closing quote after expansion.
  Both `make test-functional` and `make test-functional-coverage` run
  `required-functional-check` before executing scenarios. Keep that prerequisite
  on required functional lanes so reviewed scenario bindings and full-tree
  customer-boundary enforcement fail consistently before credential-free short
  tests begin. `contracts/functional-boundary-baseline.json` is an explicit,
  content-hash-locked migration quarantine for legacy files that already crossed
  direct implementation boundaries when repository-wide enforcement was enabled.
  New files and changed quarantined files fail the required functional check;
  remove entries as scenarios move to customer interfaces. GitHub issue #1176
  owns the migration while quarantined files remain; keep the baseline's
  `migrationTask` reference aligned with that approved tracker.
  Construction and state-machine tests that do not exercise a customer
  interface belong with the owning package rather than in `tests/functional`;
  move them there before removing their baseline entry, while keeping
  customer-observable scenarios on approved CLI, REST, MCP, or SSE seams.
  Replay serialization tests that construct event histories or invoke replay
  save/load APIs are owner-package coverage; keep them in `pkg/replay` rather
  than treating those internal calls as functional customer behavior.
  When a functional replay test and its dedicated helper only reconstruct
  internal world or transport projections, remove that pair from the
  functional suite after preserving customer-visible artifact assertions and
  confirming the projection owner's tests cover the internal contract.
  When a functional scenario already observes its queue outcome through the
  approved service-test harness, keep that runtime assertion and remove any
  duplicate direct projection reconstruction before deleting the file's
  baseline entry; projection-specific behavior belongs with its owner.
  When a functional scenario needs a customer read-model from canonical events
  it already observed, use `testutil.BuildFactoryWorldView` instead of
  importing `pkg/factory/projections` from `tests/functional`.
  Shared runtime-API helpers used by both short and `functionallong` tests must
  be build-tag neutral; do not leave a long-lane dependency on a helper supplied
  only by a removed or short-only test file.
