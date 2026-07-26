# CI Verification Classification Plan

## Status

Proposed.

## Problem

Pull request CI currently starts many unrelated jobs for every change. The
existing classifier in `cmd/ciclassify` has only four exclusive outcomes and
only controls UI coverage/browser and backend coverage. Build, lint, API,
release, real inference, and Development Package validation are still broadly
unconditional. The Development Package workflow is independently triggered, so
it cannot use the CI workflow's classification result.

The dashboard also directly consumes `@you-agent-factory/packaged-factories`.
That is an unintended product boundary: `ui/package.json` declares a file
dependency, lifecycle hooks install its candidate before ordinary UI commands,
and generated UI code imports its JSON/YAML assets directly. Classification
must not hide that coupling; the coupling must be removed first.

## Goals

- Run no product verification by default when a pull request changes no owned
  verification surface.
- Route verification by changed ownership area and take the union for mixed
  changes.
- Treat `.github/workflows/**`, CI scripts, `Makefile`, `go.mod`, and `go.sum`
  as conservative full-verification changes.
- Treat `factory/**`-only changes as exempt from CI verification.
- Keep package ownership separate from dashboard ownership.
- Avoid allocating runners to jobs whose selected lane is skipped.

## Non-Goals

- Reducing the required test depth for a selected area.
- Treating an unknown path as safe; unknown paths remain full verification until
  they receive an explicit owner.
- Changing production package publication policy on `main` in this work.

## Classification Contract

`cmd/ciclassify` will emit additive impact and lane outputs rather than one
exclusive `docs-only`, `ui-only`, `backend-only`, or `shared-risk` result. A
summary can still display the affected areas, but workflow decisions use named
boolean outputs.

| Impact | Owned paths | Selected verification |
| --- | --- | --- |
| Documentation reference | `docs/reference/**`, `docs/README.md` | `make docs-reference-smoke` |
| Root README | `README.md` | `make readme-check` |
| Factory content | `factory/**` only | None |
| Frontend | `ui/**`, excluding generated API output and removed package coupling | Frontend typecheck, lint, unit/coverage, browser verification; public-package verification only for public package/export paths |
| Backend or CLI | `pkg/**`, `cmd/**`, `internal/**`, backend tests, CLI/release fixtures | Backend unit and functional verification, backend build, focused UI-backend integration |
| API contract | `api/**`, `contracts/**`, HTTP transport/mapping, generated API outputs | Frontend verification, backend verification, focused UI-backend integration, API package verification |
| API package | `packages/api/**` and API package scripts | API package verification, frontend verification, backend verification, focused UI-backend integration |
| Packaged Factories package | `packages/packaged-factories/**` and its package scripts | Packaged Factories package verification and backend verification |
| Model Provider package | `packages/model-providers/**` and provider package scripts | Model Provider package verification and backend verification |
| Local inference | managed-local model runtime, inference bindings, OMNIVOICE installers, local-inference workflow/tests | One Linux local-inference verification job that executes the managed-runtime and real-inference regression once |
| CI/tooling | `.github/workflows/**`, CI scripts, `Makefile`, `go.mod`, `go.sum` | Full verification |
| Unknown | any unowned path | Full verification |

For a mixed pull request, selected lanes are the union of every row that
matches. `factory/**` is neutral: a PR changing `factory/**` and backend files
runs backend verification, while a `factory/**`-only PR runs nothing.

Package self-verification is mandatory when that package changes. This is in
addition to the requested consumer lanes; otherwise a package can break its own
tarball or manifest while all selected tests pass.

## Existing Documentation And Integration Evidence

Documentation has focused checks already:

- `make docs-reference-smoke` lint-checks reference docs and verifies embedded
  CLI documentation and the docs command smoke path.
- `make readme-check` verifies root README structure and local references.

Frontend-backend integration exists. `make ui-integration-test` runs browser
integration coverage and includes real-backend harness tests, including
`ui/integration/browser-test-harness.real-backend-setup.integration.test.mjs`.
That test starts a real backend, calls its HTTP API, and verifies session and
dispatch state. The current command also runs Storybook browser checks, so this
plan introduces a narrower `ui-backend-integration` target for backend/CLI and
API-contract changes rather than reusing the entire UI integration command.

## Prerequisite: Remove Dashboard Package Consumption

This work must precede the final CI routing change.

1. Remove `@you-agent-factory/packaged-factories` from `ui/package.json` and
   the lockfile.
2. Remove `prepare:packaged-factories`, its pre-build/pre-lint/pre-test hooks,
   candidate-install scripts, generated import resolver, and dashboard package
   consumer release checks.
3. Replace UI-bundled packaged factory artifacts with an explicit backend API
   contract for catalog, metadata, and selected artifact data. The backend
   remains the authoritative owner; the UI consumes generated API types and
   fetches data at runtime.
4. Update UI unit tests to use API-shaped fixtures and browser tests to use the
   real API boundary.
5. Add a dependency-boundary check rejecting
   `@you-agent-factory/packaged-factories` imports and dependencies from
   `ui/**`.

After this work, a Packaged Factories package change does not require frontend
verification merely because the dashboard is present in the repository.

## Implementation Stories

### 1. Establish Focused Verification Targets

Add or extract targets for frontend verification, backend verification, focused
UI-backend integration, each package family, and the existing docs checks. The
focused integration target must execute only browser tests that start or call a
real backend; Storybook-only checks stay in frontend verification.

Acceptance criteria:

- Each target has a documented owner and direct local rerun command.
- The focused UI-backend target proves a browser request against a real backend.
- The Linux inference target has no duplicate real-inference invocation.

### 2. Implement Additive Classification

Replace the exclusive classifier result with a path ownership table and named
lane outputs. Retain a conservative `run_everything` fallback for CI/tooling
and unknown paths.

Acceptance criteria:

- Table-driven tests cover every row in the classification contract.
- Tests prove mixed changes select the union, not a generic full run.
- Tests prove `factory/**`-only changes select no lanes.
- Tests prove workflow, Makefile, Go toolchain, and unknown paths select full
  verification.

### 3. Route Main CI At Job Level

Apply the classifier outputs to `ci.yml`. Conditions belong on jobs, not only
their steps, so skipped lanes do not reserve a hosted runner. Split broad jobs
where needed so backend build, frontend build, API contract verification, and
release/CLI verification can be independently selected.

Acceptance criteria:

- No selected-false job starts a runner.
- Backend/CLI changes select backend verification and focused UI-backend
  integration, but not frontend coverage.
- API contract changes select frontend, backend, focused UI-backend, and API
  package verification.
- CI/tooling and Makefile changes select every required lane.

### 4. Route Development Package Verification

Make Development Package classification-aware, preferably as a reusable
workflow called from main CI and passed the classifier outputs. Package
candidate dry runs and package release verification run only for the changed
package family. Protected-main publishing continues to use its required
candidate prerequisites.

Acceptance criteria:

- Package-only pull requests run only their package verification plus the
  consumer lanes defined above.
- Development Package no longer runs all package candidates for unrelated PRs.
- Main publication still receives the candidate artifacts it requires.

### 5. Stabilize Required Checks And Documentation

Add a stable `Verification Policy` job that validates the selected lane results
and publishes the touched areas, selected lanes, skipped lanes, and reasons.
Use this stable check for branch protection rather than requiring dynamic,
conditionally skipped job names. Update the development documentation to make
the new routing contract the canonical contributor reference.

Acceptance criteria:

- A reviewer can see why every lane ran or skipped in one job summary.
- Required checks remain stable across all classifications.
- Documentation matches classifier fixtures and workflow behavior.

## Validation Matrix

Before enabling the policy, run classifier fixtures and CI validation for:

1. `factory/**` only: no verification lanes.
2. `docs/reference/**`: docs reference smoke only.
3. `README.md`: README check only.
4. `ui/**`: frontend verification only.
5. `pkg/**` or `cmd/**`: backend plus focused UI-backend integration.
6. `api/**`: frontend, backend, focused UI-backend, and API package lanes.
7. Each package family independently.
8. A mixed frontend/backend PR: exactly the union of those lanes.
9. `.github/workflows/**`, `Makefile`, `go.mod`, and an unknown root path: full
   verification.

Measure elapsed time, queued time, and billable runner minutes for each case
after rollout. Do not reduce the CI/tooling fallback until fixture coverage and
observed routing prove the path map is complete.
