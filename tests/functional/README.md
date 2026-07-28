# Functional Test Package Map

`tests/functional/` is the behavior-first home for functional coverage.

## Commands

- Default non-long lane: `make test-functional`
- Ordinary unit and package-integration lane: `make test-unit`
- Stress lane: `make test-stress`
- Release-package lane: `make test-release`
- Independent functional coverage report: `make test-functional-coverage` (runs `functional-boundary-check` first; coverage-only local rerun)
- Independent backend unit coverage report: `make test-unit-coverage`
- Inventory-plus-coverage Markdown catalog (boundary → one coverage run → viz): `make functional-test-viz` (fail-closed; keeps already-written `.artifacts/functional-test-viz/` diagnostics on later-step failure). Required CI Backend Functional Coverage runs this target with `FUNCTIONAL_TEST_VIZ_DIR=.artifacts/backend-functional-coverage` and uploads `functional-tests.md`, `coverage-summary.json`, `coverage.out`, and `command.log` on success and failure when present. Wiring is covered by stubbed/dry-run Make contract smoke under `tests/functional/observability/coverage/functional_test_viz_contract_test.go` (does not run the full functional suite).
- Built-CLI S24 acceptance lane (also run by `make verify-pr`): `make test-built-cli-acceptance`
- Opt-in long lane: `make test-functional-long`
- Real local-inference lane: `make long-tests`

`make long-tests` is the maintainer entrypoint for OMNIVOICE real local
inference coverage. It first reruns the managed-local-model integration tests
in `pkg/models/local`, then runs the tagged runtime API long test
that exercises `POST /models/{model_name}/pull`, direct
`/models/{model_name}/invocations`, and a factory-level `MODEL_INVOKE` path.
The real-runtime test is opt-in: set `INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1`
and ensure the `omnivoice-llamacpp` command is available on `PATH`, or point
`INFINITE_YOU_OMNIVOICE_COMMAND` at the executable explicitly. Set
`INFINITE_YOU_OMNIVOICE_CACHE_DIR` to reuse an existing managed cache; when
omitted, the long test pulls assets into a temporary managed cache directory.
GitHub Actions automation for that lane lives in
`.github/workflows/long-local-inference.yml`; it restores `.cache/managed-models`
between runs, installs the runtime from per-platform
`OMNIVOICE_COMMAND_URL_*` repository variables when available, and otherwise
builds the real `ServeurpersoCom/omnivoice.cpp` `omnivoice-tts` backend from a
pinned commit before building the repo-owned `cmd/omnivoice-llamacpp` adapter
that speaks the shared subprocess contract used by the service and long tests.

The default lane runs one repository-owned package-discovery command through
`make test-functional`: `go run ./cmd/functionallane` uses
`go list ./tests/functional/...` to discover the behavior packages, excludes
`tests/functional/internal/support`, and then executes one explicit
`go test -p 2 -short ...` command over that discovered package list. That
keeps the full behavior tree on package discovery without hard-coded package
names, stays portable across environments, and avoids the slow Windows
wildcard `./tests/functional/...` path. The long lane runs the full behavior
tree plus any `functionallong`-tagged files, so broad or slow scenarios stay
available without widening the default feedback loop.

The default and functional-coverage lanes share their runnable-package policy
from `internal/testlanes`. Both lanes still execute every package returned by
discovery rather than selecting from an allowlist. The same policy verifies
that the provider contract, all eight built-in providers, script, mock-worker,
and observability destinations remain present; a missing required destination
fails discovery with the omitted import path.

The coverage lanes intentionally use separate profiles. The
`make test-functional-coverage` command executes only the maintained non-long
functional packages while measuring backend-owned `cmd/factory` and `pkg/...`.
It uses the same bounded `go test -p 2` package concurrency as the default
functional lane so coverage instrumentation does not increase cross-package
resource contention.
Its total and per-package percentages come from the functional-only profile;
packages untouched by those functional flows are shown at `0.0%` even when
their package-local unit tests have higher coverage.
The lane enforces the current aggregate floor and an 80% target for every new
backend package. Existing packages below 80% are explicitly grandfathered in
the functional package baseline and should be removed from that file as their
functional coverage reaches the target.
Provider test destinations are test packages rather than measured backend
packages, so adding an empty destination does not create a package-minimum
manifest entry. Its scenarios still contribute to the shared backend profile
as soon as tests are added there.

The `make test-unit-coverage` command executes only backend package tests
against that same owned code set. Functional coverage therefore remains
visible independently instead of being merged with unit, stress, or release
test results.

## Package Taxonomy

| Package | Purpose |
| --- | --- |
| `acceptance` | Hermetic built-CLI S24 cross-surface acceptance: install, provider posture, invalid goal, quiet, primary/stream output, local-model invoke, goal repeat, and subagent outcomes from the built you binary under isolated home/log directories. |
| `smoke` | Small end-to-end confidence checks that prove the main system starts, accepts work, and completes representative flows quickly. |
| `workflow` | Core multi-step workflow behavior such as routing, review loops, and ordinary progression across workstations. |
| `guards_batch` | Guard evaluation, dependency gating, fan-in or batch semantics, and request-batch behavior that should fail in one narrow behavior area. |
| `runtime_api` | Runtime projections, HTTP API behavior, event or state queries, and other externally observable runtime read models. |
| `providers` | Legacy aggregate provider coverage that remains runnable while scenarios migrate to the dedicated packages below. `make functional-boundary-check` requires the grandfathered inventory to match the remaining root-level tests exactly, so remove a migrated file and its exception together; do not add or reintroduce scenarios here. |
| `providers/contract` | Provider-neutral extension behavior shared across provider identities. |
| `providers/agy`, `providers/claude`, `providers/codex`, `providers/cursor`, `providers/gemini`, `providers/kiro`, `providers/opencode`, `providers/pi` | Behavior owned by the named built-in model provider. |
| `providers/script` | Script-worker behavior, which is not model-provider behavior. |
| `providers/mock_workers` | Mock-worker behavior, which is not model-provider behavior. |
| `providers/observability` | Provider-facing logging and diagnostics behavior, which is not model-provider behavior. |
| `replay_contracts` | Replay, event-history, and artifact reconstruction behavior that must stay stable across recording and playback surfaces. |
| `work/transports/cli/submit/unary_contract` | Work-owned unary `you submit` contract proofs: file and stdin payload ingress, default and explicit Factory Session targeting, and structured failure public-message preservation through `root.BuildProcess` + `Process.Execute`. |
| `bootstrap_portability` | Init, bootstrap, portability, current-factory activation, and checked-in factory portability flows. |

## Shared Support

- Cross-package functional helpers belong in `tests/functional/internal/support`.
- `tests/functional/internal/support.StartFunctionalAPIServer` is the
  canonical production-composed HTTP harness for backend transport regressions:
  it builds the customer process through `root.BuildProcess` with exact typed
  `edges.Edges` replacements, so runtime
  API smoke coverage for `/status`, `/models`, session routes, and factory
  activation should prefer that seam over hand-built HTTP doubles when the
  goal is startup-path parity.
- Provider functional packages must obtain executable processes through
  `tests/functional/internal/support.BuildProcess`; they must not import
  `pkg/root`, `pkg/wire`, initializer or runtime composition internals, service
  implementation/composition subpackages, or concrete built-in provider
  implementations. Service-root contracts and the exact public external-effect
  ports needed to populate typed `edges.Edges` replacements remain available.
- Keep package-local helpers next to the tests until a second behavior package
  needs them, then promote them into the support package instead of importing
  or copying another package's `*_test.go` helpers.
- During the migration, behavior packages may temporarily reuse legacy fixture
  data from `tests/functional_test/testdata`, but the fixture lookup and other
  shared wiring should flow through `internal/support`.
- Do not add new cross-package helper or compatibility files under
  `tests/functional_test`. That legacy bucket may keep only narrow temporary
  shims for still-unmigrated tests; new shared helpers must land in
  `tests/functional/internal/support`.

## Placement Rules

- Behavior decides package ownership. Put a test in the package that best
  matches the regression users would name first.
- Transport prefixes improve discoverability inside a package but do not define
  package ownership.
- Use `cli_`, `api_`, `replay_`, or `watcher_` filename prefixes when the
  transport boundary is important to scanning the package quickly.
- Keep helpers package-local by default. Only promote a helper into
  `tests/functional/internal/support` when it is reused across behavior
  packages.
- Keep long-running or broad-sweep coverage out of the default lane. When a
  test belongs in the slow lane, gate it behind
  `tests/functional/internal/support.SkipLongFunctional(...)` or the
  `functionallong` build tag so `make test-functional` can keep running the
  full short-mode behavior package set through repository-owned package
  discovery without ad hoc package or test arguments at invocation time.
- When a slow test is gated behind `functionallong`, name the file
  `*_long_test.go` so review-time checks can spot the lane boundary
  immediately.
- When every test in a file belongs to the long lane, move the whole file into
  a `*_long_test.go` unit instead of leaving short-mode builds to compile a
  file that only calls `support.SkipLongFunctional(...)` at runtime.
- When a mixed file keeps only a few short-lane assertions, split the slow
  tests and any long-only helpers into sibling `*_long_test.go` files so the
  default build stops compiling broad sweeps that are already long-lane only.
- When a legacy fixture-directory smoke loop mixes unrelated behaviors, replace
  it with package-owned tests that assert the user-visible outcome for each
  behavior instead of keeping one umbrella "loads every fixture" check in the
  default lane.
- Keep long-lane tests in the behavior package they validate. For example, the
  broad provider normalization sweep lives in
  `tests/functional/providers/cli_provider_error_long_test.go` behind the
  `functionallong` tag instead of widening `make test-functional` or reviving
  the legacy mixed bucket, while broad-but-still-package-local sweeps can use
  `support.SkipLongFunctional(...)` when they do not justify a dedicated
  `_long_test.go` split yet.

## Migration Compatibility

`tests/functional_test/` is now a legacy fixture store only. New decomposition
work should target the behavior-first package tree rather than adding more
coverage or helpers to that legacy directory.

Compatibility rules during coexistence:

- `tests/functional_test/` stays open only for legacy checked-in fixture data;
  observable non-long behavior coverage belongs in `tests/functional/...`.
- `tests/functional_test/testdata` remains the checked-in legacy fixture store
  until fixture ownership is migrated separately.
- New behavior coverage belongs in `tests/functional/<behavior-package>/`.
- New shared harnesses, assertions, and fixture seams belong in
  `tests/functional/internal/support`.
- Review changes for long-lane placement and helper drift before merge:
  `functionallong` files belong under `tests/functional/...`, slow-lane files
  should use `*_long_test.go`, and new cross-package helpers should land in
  `tests/functional/internal/support` rather than `tests/functional_test`.
