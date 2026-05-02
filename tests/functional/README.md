# Functional Test Package Map

`tests/functional/` is the behavior-first home for functional coverage.

## Commands

- Default non-long lane: `make test-functional`
- Opt-in long lane: `make test-functional-long`

The default lane runs one repository-owned package-discovery command through
`make test-functional`: it uses `go list ./tests/functional/...` to discover
the behavior packages, excludes `tests/functional/internal/support`, and then
executes one explicit `go test -p 2 -short ...` command over that discovered
package list. That keeps the full behavior tree on package discovery without
hard-coded package names while avoiding the slow Windows wildcard
`./tests/functional/...` path. The long lane runs the full behavior tree plus
any `functionallong`-tagged files, so broad or slow scenarios stay available
without widening the default feedback loop.

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
