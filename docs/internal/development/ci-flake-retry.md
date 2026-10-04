# CI flake retry and flake ledger

One flaky functional test must not turn a whole Backend Functional Coverage run
red and cost a 20-40 minute rerun. The functional coverage job therefore reruns
a failing test once on the same commit and records the result.

This does not weaken any assertion: a test that fails twice still fails the job.

## Behaviour

Implemented in `cmd/gocoveragecheck/coverage_flake_retry.go`, hooked into
`executeCoverageInvocationPlan`. It is opt-in through environment variables,
so local `make functional-test-viz` runs are unchanged.

| Variable | Meaning |
| --- | --- |
| `FUNCTIONAL_FLAKE_RETRY_MAX` | Maximum distinct failing top-level tests to retry. Unset or `0` disables the retry. CI sets `5`. |
| `FUNCTIONAL_FLAKE_LEDGER` | Path of the JSON ledger. CI uses `.artifacts/functional-test-viz/flake-ledger.json`. |
| `FUNCTIONAL_FLAKE_HEAD_SHA` | Commit recorded in the ledger (the PR head, not the merge commit). |

When the coverage test run fails:

1. The `go test -json` stream is parsed. A failing subtest is retried through its
   top-level test.
2. The run is retried only if every failure is an ordinary test failure. It is
   **not** retried when a package failed with zero failing tests, when the output
   shows `panic:`, `fatal error:` or `[build failed]` (binary death, timeout,
   build failure), or when more than five distinct tests failed (mass failure is
   not a flake).
3. Each failing package is rerun once with the same flags (`-coverpkg`,
   `-covermode`, `-p`, `-timeout`, ...) and `-run=^(TestA|TestB)$`, writing its
   own coverage profile.
4. If every retried test passes, the failing events are replaced by the retry's
   events, the retry profiles are merged into the lane profile (so package floors
   still evaluate), and the job passes. If any retried test fails again, nothing
   is rewritten and the job fails exactly as before.

The unit lane is not retried: it uses covdata profiles and a different runner.

## Reading a flake record

The job writes three things, none of which are committed automatically:

- the step summary section **Functional flake ledger** (a table of test, package,
  retry result and first-failure excerpt) and one `Flaky functional test`
  warning annotation per flake;
- the `functional-flake-ledger` artifact (`flake-ledger.json`, 30 days);
- stderr lines `flake retry: package=... test=... retry=pass|fail` in the job log.

`flake-ledger.json` fields: `headSha`, `outcome`, `reason`, `retryLimit`, and
`entries[]` with `package`, `test`, `failedSubtests`, `firstFailureExcerpt`,
`retryOutcome`.

| `outcome` | Meaning | Job result |
| --- | --- | --- |
| `flake-recovered` | Every retried test passed on retry | pass |
| `failed-after-retry` | A retried test failed again | fail |
| `not-retried` | Binary death, zero-test package failure, or over the limit; `reason` says which | fail |

No ledger file means no test failed in the run.

## Giving a flake an owner and an expiry

A recorded flake is a defect. Retries must not hide a test that flakes every
run. Handle each `flake-recovered` entry in a follow-up change, by hand:

1. Open an issue or task naming the package, test, and the run URL.
2. If it must be excluded while it is fixed, add an entry to
   `tests/functional/functional-quarantine.json` through its existing
   `package`/`test`/`bucket`/`reason` fields. Put the owner, the run URL, and an
   expiry date in `reason`, since the schema has no dedicated owner or expiry
   fields.
3. Remove the entry when the fix merges or the expiry passes.

Changes to the quarantine list are deliberately outside the retry mechanism.

## Tests

- `cmd/gocoveragecheck/coverage_flake_retry_test.go` covers the parser, the retry
  decision (limit, zero-test packages, panics), retry argument construction, stream
  rewriting and the end-to-end recovered, failed-again and disabled paths.
- `scripts/ci/flake-ledger-summary.test.mjs` covers the summary renderer.
- `scripts/ci/functional-coverage-workflow.test.mjs` pins the workflow wiring.
  Run the Node suites with `make test-ci-workflows`.
