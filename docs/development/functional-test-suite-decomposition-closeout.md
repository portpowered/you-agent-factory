# Functional Test Suite Decomposition Closeout

This closeout records the contributor-facing contract and the measured command
evidence for the functional suite decomposition PRD.

## Command Contract

- Default non-long functional lane: `make test-functional`
- Opt-in long functional lane: `make test-functional-long`
- Review-time guardrail for lane placement and legacy helper drift:
  `make functional-layout-contract`

## Measured Runtime

- Closeout verification on 2026-05-01 measured `make test-functional` at
  `51.608s` in this Windows worktree.
- The earlier migration verification recorded a warmer `32.691s` run on the
  same branch, so the suite is materially lower than the pre-decomposition
  baseline of roughly `74s` but still above the original `10s` target from the
  PRD.
- Treat the decomposition as a structure and lane-separation win, not as the
  final runtime-optimization endpoint.
- The long lane remains explicitly opt-in through
  `make test-functional-long`, which runs the `functionallong`-tagged behavior
  package files without widening the default command.

## Compatibility Strategy

- `tests/functional/` is the default home for new behavior-owned functional
  coverage.
- `tests/functional/internal/support` is the only shared cross-package helper
  seam for decomposed functional coverage.
- `tests/functional_test/` now remains only for the still-unmigrated replay
  scheduler smoke plus legacy checked-in fixture data under `testdata/`.
- New shared helper files must not be added to `tests/functional_test`; move
  that code into `tests/functional/internal/support` instead.
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
