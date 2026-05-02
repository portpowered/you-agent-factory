# Functional Test Suite Decomposition Closeout

This closeout records the contributor-facing contract and the measured command
evidence for the functional suite decomposition PRD.

## Command Contract

- Default non-long functional lane: `make test-functional`
- Opt-in long functional lane: `make test-functional-long`
- Review-time guardrail for lane placement and legacy helper drift:
  `make functional-layout-contract`

The default lane runs `go test -short ./tests/functional/...` through package
discovery in short mode. The long lane runs the full behavior tree plus any
`functionallong`-tagged files.

## Measured Runtime

- The branch originally recorded a `1.76s` representative-only run, but that
  contract has been reverted because it did not execute all non-long short-mode
  functional behavior.
- The current repository-owned command is again `make test-functional` over the
  full short-mode `./tests/functional/...` tree.
- As of 2026-05-01 in this Windows worktree, the runtime target remains open:
  `go test -short ./tests/functional/... -count=1` still measured about `54s`,
  so additional long-gating or test-splitting is still required before the
  PRD's `<=10s` target is met.

## Compatibility Strategy

- `tests/functional/` is the default home for new behavior-owned functional
  coverage.
- `tests/functional/internal/support` is the only shared cross-package helper
  seam for decomposed functional coverage.
- `tests/functional_test/` now remains only for legacy checked-in fixture data
  under `testdata/` plus worktree-local planning artifacts when this workflow
  materializes them there.
- New shared helper files must not be added to `tests/functional_test`; move
  that code into `tests/functional/internal/support` instead.
- Broad or slow package-local scenarios should call
  `tests/functional/internal/support.SkipLongFunctional(...)` when they must
  stay out of the full short-mode default lane without justifying a dedicated
  `_long_test.go` split yet.
- Long-lane files stay in the behavior package they validate and must use both
  the `functionallong` build tag and a `*_long_test.go` filename.

## Guardrail Coverage

The repository-owned guard in
`internal/contractguard/functional_layout_test.go` fails when:

- a `*_long_test.go` file is missing the `functionallong` build tag
- a `functionallong`-tagged file sits outside `tests/functional/...`
- a new helper or compatibility shim appears in `tests/functional_test/`
  outside the current allowlist

## Verification

The decomposition closeout was verified with these repository-root commands:

```text
make functional-layout-contract
make test-functional
make test-functional-long
make test
make lint
```
