# Go unit-lane latency evidence

`make test-unit-fresh` remains the canonical uncached Go unit-lane entry point.
It still discovers the same `pkg/...` test packages and invokes the same
`go test` flags. Add `UNIT_TIMING_OUTPUT=<path>` when a machine-readable v2
timing document is needed:

```text
make test-unit-fresh UNIT_DEFAULT_JOBS=2 UNIT_TIMING_OUTPUT=.artifacts/unit-latency/run-1.v2.json
```

The v2 document records the source commit, Go version, exact command, jobs,
computed lane budget, runner identity, environment invalidations, package
elapsed time/outcome/cache state, and the event-derived test inventory. Package
objects and test names are sorted before JSON serialization. The output is
written through a same-directory temporary file and renamed into place so a
failed write cannot leave a partial evidence document.

Use the pure checker for a three-sample comparison:

```text
make test-unit-latency-budget
```

To validate the retained reference distribution, use baseline mode with the
three accepted paths:

```text
go run ./cmd/unitlanebudget -mode baseline -samples docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-2.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-3.v2.json
```

The checker requires three complete, passing, uncached samples on one identity
and exact package/test inventories. It recomputes the reference and candidate
medians, requires at least 25% median improvement, and rejects any candidate
run more than 10% above its candidate median. Failures include expected and
actual values plus the rerun command. Invalid captures remain on disk for
review; they are never silently replaced.

The checked-in `go-unit-lane-latency-budget.v1.json` is the frozen policy and
reference instance. It records the accepted median of 239.612 seconds, 444
packages, and 18,122 tests. The adjacent Draft 2020-12 schema is compiled and
validated before final-mode checking; the checker never rewrites either file.

The retained baseline ledger records all three complete uncached captures and
the first incomplete attempt. The incomplete attempt remains separate from the
accepted replacement and is documented rather than silently discarded. The
accepted evidence is historical reference data. This implementation does
not create a new baseline, run a manual final three-sample gate, make a local
25% improvement claim, or run a final candidate comparison; hosted final
enforcement remains review-owned.
