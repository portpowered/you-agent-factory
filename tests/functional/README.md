# Functional Test Package Map

`tests/functional/` is the behavior-first home for functional coverage.

## Commands

- Default non-long lane: `make test-functional`
- Opt-in long lane: `make test-functional-long`

The default lane runs `go test -short ./tests/functional/...` through package
discovery without manual package lists. The long lane runs the full behavior
tree plus any `functionallong`-tagged files, so broad or slow scenarios stay
available without widening the default feedback loop.

## Package Taxonomy

| Package | Purpose |
| --- | --- |
| `smoke` | Small end-to-end confidence checks that prove the main system starts, accepts work, and completes representative flows quickly. |
| `workflow` | Core multi-step workflow behavior such as routing, review loops, and ordinary progression across workstations. |
| `guards_batch` | Guard evaluation, dependency gating, fan-in or batch semantics, and request-batch behavior that should fail in one narrow behavior area. |
| `runtime_api` | Runtime projections, HTTP API behavior, event or state queries, and other externally observable runtime read models. |
| `providers` | Provider-backed worker execution behavior, provider retries, provider failures, and command-request shaping that remains user-visible. |
| `replay_contracts` | Replay, event-history, and artifact reconstruction behavior that must stay stable across recording and playback surfaces. |
| `bootstrap_portability` | Init, bootstrap, portability, current-factory activation, and checked-in factory portability flows. |

## Shared Support

- Cross-package functional helpers belong in `tests/functional/internal/support`.
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
  full short-mode `./tests/functional/...` tree without ad hoc package or
  test arguments at invocation time.
- When a slow test is gated behind `functionallong`, name the file
  `*_long_test.go` so review-time scanning and guardrails can spot the lane
  boundary immediately.
- When every test in a file belongs to the long lane, move the whole file into
  a `*_long_test.go` unit instead of leaving short-mode builds to compile a
  file that only calls `support.SkipLongFunctional(...)` at runtime.
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

The remaining `tests/functional_test/` coverage is temporary while stories
finish the last migration gaps. New decomposition work should target the
behavior-first package tree rather than adding more unrelated coverage to the
legacy mixed bucket.

Compatibility rules during coexistence:

- `tests/functional_test/` stays open only for legacy checked-in fixture data
  and temporary local planning artifacts inside this worktree; observable
  non-long behavior coverage now belongs in `tests/functional/...`.
- `tests/functional_test/testdata` remains the checked-in legacy fixture store
  until fixture ownership is migrated separately.
- New behavior coverage belongs in `tests/functional/<behavior-package>/`.
- New shared harnesses, assertions, and fixture seams belong in
  `tests/functional/internal/support`.
- The repository-owned guard in `internal/contractguard/functional_layout_test.go`
  fails when a `_long_test.go` file misses the `functionallong` tag, when a
  `functionallong` file sits outside `tests/functional/`, or when a new helper
  shim appears in `tests/functional_test`.
