# Functional Test Package Map

`tests/functional/` is the behavior-first home for functional coverage.

## Commands

- Default non-long lane: `make test-functional`
- Opt-in long lane: `make test-functional-long`

The default lane runs `go test ./tests/functional/...` without manual package
lists. The long lane reserves the `functionallong` build tag for slow or broad
coverage that should not join the default command.

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
  test belongs in the slow lane, gate it behind the `functionallong` tag so
  `make test-functional` stays package-discovery-only and does not need manual
  excludes.
- When a legacy fixture-directory smoke loop mixes unrelated behaviors, replace
  it with package-owned tests that assert the user-visible outcome for each
  behavior instead of keeping one umbrella "loads every fixture" check in the
  default lane.
- Keep long-lane tests in the behavior package they validate. For example, the
  broad provider normalization sweep lives in
  `tests/functional/providers/cli_provider_error_long_test.go` behind the
  `functionallong` tag instead of widening `make test-functional` or reviving
  the legacy mixed bucket.

## Migration Compatibility

The existing `tests/functional_test/` suite can coexist temporarily while
stories migrate coverage into `tests/functional/`. New decomposition work
should target the behavior-first package tree rather than adding more unrelated
coverage to the legacy mixed bucket.
