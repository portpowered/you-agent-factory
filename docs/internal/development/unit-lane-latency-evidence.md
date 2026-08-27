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

The hosted Backend Unit Latency job warms the Go module, command, dependency,
and test compilation caches before measuring samples. It runs `go mod download`,
builds the unit-lane and checker commands, and compiles `pkg/...` with
`go test -run '^$'`; these warm-up commands are outside the three measured
`make test-unit-fresh` calls. Each measured call still uses `-count=1`, so test
results remain uncached while dependency and compilation setup cannot make the
first sample incomparable with the next two.

Use the final checker for the retained historical audit and two live,
same-runner cohorts:

```text
make test-unit-latency-budget
```

The final target supplies the v2 budget, three retained historical samples,
three reference-CI samples, three candidate samples, and an atomic
`reference-ci-manifest.v1.json` output path. The reference-CI and candidate
cohorts must be captured by the same job allocation; their runner provider,
image, image version, OS, architecture, and CPU model must match exactly.
The historical WSL/Intel wall times are retained for audit only and never
authorize the final improvement claim.

To validate the retained reference distribution, use baseline mode with the
three accepted paths:

```text
go run ./cmd/unitlanebudget -mode baseline -samples docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-1-replacement.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-2.v2.json,docs/internal/development/plans/unit-test-optimization-c01-wire-timeout-witness/baseline-make-run-3.v2.json
```

The checker requires three complete, passing, uncached samples in each cohort
and exact event-derived package/test inventories. It recomputes the live
reference and candidate medians, requires at least 25% median improvement, and
rejects any candidate run more than 10% above its candidate median. Failures
include expected and actual values plus the rerun command. Invalid captures
remain on disk for review; they are never silently replaced. The manifest
records each timing-file SHA-256, command, wall time, cohort inventory summary,
runner identity, thresholds, verdict, and all diagnostics.

The checked-in `go-unit-lane-latency-budget.v1.json` and its schema remain
retained audit inputs. The v2 policy in
`go-unit-lane-latency-budget.v2.json` records the unchanged historical
distribution of 239.612 seconds over 444 packages and 18,122 tests, plus the
reviewed candidate expectation of 444 packages and 18,154 tests. The reviewed
candidate inventory SHA-256 is
`19b437b4925c799b2b3ac928dd3f070c1846ed0c588ab65a1b722c05827a1677`.
Reference-CI is pinned to commit
`ba8ef900ee29347295ac7657742fd1aab42f064c` and the historical/candidate
inventories use the exact hashes recorded in the v2 policy. This is the
declared `exact-with-reviewed-diff` reconciliation; the checker never rewrites
the budget, schemas, or retained historical files.

The retained baseline ledger records all three complete uncached captures and
the first incomplete attempt. The incomplete attempt remains separate from the
accepted replacement and is documented rather than silently discarded. The
accepted evidence is historical reference data; v2 final mode compares only
the live reference-CI and candidate cohorts.
