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
  Focused lint reruns must stay Windows-compatible: do not add standalone
  `printf` recipe lines to targets used by `make verify-lint`.
  Windows Go suite coverage is a `windows-go-tests` matrix with independent
  `Unit`, `Functional`, `Stress`, and `Release` jobs. Keep `fail-fast: false`,
  preserve each job's Windows setup, and invoke the matching repository-owned
  local command (`make test-unit`, `make test-functional`, `make test-stress`,
  or `make test-release`). Each lane writes that command to its job summary so
  a failed CI result has a direct local rerun.
