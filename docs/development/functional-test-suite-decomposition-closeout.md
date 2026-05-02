# Functional Test Suite Decomposition Closeout

This closeout records the contributor-facing contract and the measured command
evidence for the functional suite decomposition PRD.

## Command Contract

- Default non-long functional lane: `make test-functional`
- Opt-in long functional lane: `make test-functional-long`

The default lane runs `go test -p 2 -short ./tests/functional/...` through the
repository-owned `make test-functional` target, so the full behavior tree still
uses package discovery without a hard-coded package list while avoiding the
slow Windows cross-package parallel scheduling path. The long lane runs the
full behavior tree plus any `functionallong`-tagged files.

## Measured Runtime

- The branch previously tried a representative-only `1.76s` run, but that
  contract was reverted because it did not execute all non-long short-mode
  functional behavior.
- Later Windows worktree validation showed that the default `go test -short
  ./tests/functional/...` invocation could regress badly because Go's
  cross-package parallel scheduling reported multi-dozen-second wall-clock
  totals even after the broad sweeps moved behind `functionallong`.
- On 2026-05-02 in this Windows worktree, `Measure-Command { go test -p 2
  -short ./tests/functional/... -count=1 -timeout 300s *> $null }` measured
  about `4.31s`.
- On 2026-05-02 in this Windows worktree, `make test-functional` measured
  about `4.20s` after repointing the target to that `-p 2` invocation.
- The runtime target is therefore met for the documented default command while
  the explicit long lane remains `make test-functional-long`.

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

Contributor guidance and review-time checks now carry the decomposition
guardrails:

- long-lane files stay under `tests/functional/<behavior-package>/`
- slow-lane files that use the `functionallong` build tag should use
  `*_long_test.go` names
- new cross-package helper growth belongs in
  `tests/functional/internal/support`, not `tests/functional_test/`

## Verification

The decomposition closeout was verified with these repository-root commands:

```text
make test-functional
make test-functional-long
make test
make lint
```
