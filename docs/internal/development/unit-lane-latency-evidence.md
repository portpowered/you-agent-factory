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

The hosted Backend Unit Latency job uses one runner allocation for both live
cohorts. It adds a detached worktree at the pinned reference commit, prewarms
that checkout, captures reference ordinals 1–3, then prewarms the PR-head
checkout and captures candidate ordinals 1–3. The prewarm commands run
`go mod download`, build the unit-lane and checker commands, and compile
`pkg/...` with `go test -run '^$'`; they are outside the six measured
`make test-unit-fresh` calls. Each measured call still uses `-count=1`, so test
results remain uncached. Reference and candidate evidence are retained in
separate `reference/` and `candidate/` directories, along with setup logs and
per-ordinal status files. The job records the hosted image version and CPU
model alongside the Go OS/architecture identity; a failed ordinal never causes
a retry or replacement.

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
reconciled candidate expectation of 444 packages and 18,156 tests. The
candidate inventory SHA-256 is
`451f3276fb95d5998dcba67cf65b039c0791351d15bdbe505c27a160a8bb6ede`.
Reference-CI is pinned to commit
`9e19e26e0fb6df47cfdd4c4d4469ce712aae04ff`, while the measurement-only
`ba8ef900ee29347295ac7657742fd1aab42f064c` identity remains retained in the
historical audit files. The historical/candidate inventories use the exact
hashes recorded in the v2 policy. This is the declared
`exact-with-reviewed-diff` reconciliation; the checker never rewrites the
budget, schemas, or retained historical files.

The retained baseline ledger records all three complete uncached captures and
the first incomplete attempt. The incomplete attempt remains separate from the
accepted replacement and is documented rather than silently discarded. The
accepted evidence is historical reference data; v2 final mode compares only
the live reference-CI and candidate cohorts.
