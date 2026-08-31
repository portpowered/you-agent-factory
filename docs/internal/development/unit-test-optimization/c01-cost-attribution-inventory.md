# Cycle 01 cost-attribution inventory v1

Status: `BLOCKED` after the operator-authorized hosted attempt; twelve completed
observations were captured and rejected, so no stage or package rows are
admitted. This is the amendment's complete-delivery disposition, not a timing
or attribution claim.

Canonical source: [`c01-cost-attribution-inventory.json`](c01-cost-attribution-inventory.json)

U01 evidence ledger: [`c01-u01-evidence-ledger.md`](c01-u01-evidence-ledger.md)

Governing charter: [`README.md`](README.md)

## Baseline reference

The reference is the operator-supplied Backend Unit Coverage observation
(`run 33322584614`, `job 99287112602`). It is retained as identity and
aggregate context; raw logs and traces are not committed.

| Observation | Value | Evidence meaning |
| --- | ---: | --- |
| Job wall | 649.0 seconds | Infrastructure baseline |
| Coverage-step wall | 607.0 seconds | Coverage baseline |
| `go test` wall | 342.9 seconds | Test-command baseline |
| Test execution | 43.7 seconds | Observed execution component |
| Unattributed residual | 264.1 seconds | Measurement work remains; no bucket assignment |

The residual is deliberately not represented as an inferred compile, link,
covdata, merge, or evaluate value. No optimization claim is made by this v1
baseline.

## Corpus and target constants

| Constant | Value |
| --- | ---: |
| Packages | 445 |
| Tests | 18,273 |
| Maximum skips | 57 |
| Aggregate coverage | 80.7% |
| Profile blocks | 99,418 |
| Fixed package overhead | 0.322 seconds |
| Irreducible coverage floor | 147.3 seconds |
| Backend Unit Coverage target | at most 240 seconds |
| Backend Unit Latency target | at most 200 seconds |

Sub-two-minute coverage is rejected for the current scope. Linking 445
instrumented test binaries, running 18,273 tests, emitting 99,418 profile
blocks, evaluating 445 package floors, and the 147.3-second irreducible floor
make that target neither achievable nor acceptable as a Cycle 01 claim.

## Attribution contract

The canonical JSON currently has zero admitted bucket rows and records this
explicitly. The hosted attempt below is retained as rejected evidence, not as
a bucket measurement. The only allowed future bucket identities are:

| Bucket identity | Required evidence owner | Baseline state |
| --- | --- | --- |
| `compile` | U01-G2 | not captured |
| `link` | U01-G2 | not captured |
| `covdata` | U01-G2 | not captured |
| `merge` | U01-G2 | not captured |
| `evaluate` | U01-G2 | not captured |

Each captured row must contain these exact fields: `stableIdentity`, `bucket`,
`measuredSeconds`, `measurementSource`, `measuredOrInferred`, `verdict`,
`owningLane`, and `blockedBy`. Captured seconds must be finite and
non-negative, and the source must identify a retrievable artifact, command,
run, and job. Measurement status is either `measured` or `inferred`; an
obtainable bucket above 30 seconds cannot remain inferred at U01-G2
completion. Allowed row verdicts are `optimize`, `retain`, `park`, `blocked`,
and `not-material`.

No bucket row is admitted because every completed authorized observation failed
the inherited timing/test/package invariants. The hosted evidence owner must
reject duplicate identities, missing fields, cross-head evidence, and
unsupported verdicts rather than turning a rejected observation into a
measurement.

## Package classification contract

The inventory expects exactly 445 unique Go import-path rows under the
`fixed-package-overhead` bucket. The classification basis is 0.322 seconds
per package. The package array and `packageClassifications` array are empty
because every completed timing artifact was incomplete and failed the
inherited test/skip/package invariants. The expected count and row fields
remain canonical and reviewable. A conversion or consolidation is not
justified by the count alone.

## Parallelism experiment

The required hosted pair is:

| Parameter | Completed observations | Raw wall seconds |
| --- | --- | --- |
| `-p=1` | diagnostic `33356894321/99380925320`; controls `33356915970/99381063719`, `33356915983/99381031153` | diagnostic `390.983`; controls `440.585`, `481.061` |
| `-p=4` | diagnostic `33356916239/99381102075`; control `33356916302/99381113336` | diagnostic `305.377`; control `311.144` |

All completed observations use head
`4d13d577ce699ea80ff9643b2221bbd2f178bd09`, Go `go1.25.0`, and
`ubuntu-latest`. The requested three-sample cells were not completed. No
median or collapsed-parallelism verdict is recorded because all completed
timing artifacts report `complete=false`, 11,560 tests, and 57 skips.

The raw artifact IDs, paths, diagnostic hashes, cache observation, canceled
cells, and invariant audit are in the [U01 evidence ledger](c01-u01-evidence-ledger.md).

## Prior hosted evidence attempt (not admitted)

| Field | Observation |
| --- | --- |
| Head SHA | `8f810fc94ceb449e90d7733394078e57f4da44a3` |
| Run / job | `33345809127` / `99349658743` |
| Artifact ID | `9742044437` (`unit-coverage-diagnostics`) |
| Go version | `go1.25.0` |
| Diagnostic | complete; wall `239.885s`; compiler `2087`; linker `445`; build actions `2532`; expected packages `445` |
| Coverage | `80.7%`; `107586/133294` statements; `480` measured coverage packages |
| Timing | incomplete; `445` package rows; `11560` tests; `0` failures; `57` skips |
| Verdict | rejected before cohort, median, bucket, or package admission |

The timing record fails the charter invariants of 18,273 tests and skips below
57. The detailed CI run and artifact links are in the PR comment; raw logs and
profiles are not committed.

## Authorized hosted attempt (not admitted)

The operator amendment authorized a minimum workflow-only change to expose
`UNIT_COVERAGE_JOBS=1`/`4`, diagnostic/control mode, and sample identity. Twelve
completed observations are recorded in the [U01 evidence ledger](c01-u01-evidence-ledger.md).
Every one preserved 445 timing package rows and 80.7% aggregate coverage, but
reported `complete=false`, 11,560 tests, 57 skips, and 480 coverage-summary
packages. The timing rows do not expose cached/unknown state. These failures
reject the observations before medians, diagnostic overhead, infrastructure
buckets, or package classifications.

The first diagnostic's setup-go log records an exact cache hit for the
`setup-go-Linux-x64-ubuntu24-go-1.25.0-70c8dd24106c110416ea09866dce4ff9e81bf705c128680b5f66ebeb8f4fa90b`
key, with an archive of approximately 0 MB / 7,439 bytes. Its diagnostic
action-cache key fields are empty; no populated Go build-cache content or
cache-cost attribution is claimed.

## Pilot disposition

The factory-sessions execution pilot measured 10.63 seconds and is recorded as
`KEEP-AS-IS`. Its cost cannot materially reduce a 649-second infrastructure
wall. This is a bounded disposition, not a package conversion recommendation
and not a substitute for the complete 445-package classification.

## Traceability and evidence rules

The charter traces all stories to `UTO-TARGET`, `UTO-C01-ATTRIBUTION`,
`UTO-C01-U01`, `UTO-C01-PARALLELISM-EXPERIMENT`,
`UTO-C01-PACKAGE-CLASSIFICATION`, `UTO-C01-PILOT-DISPOSITION`,
`UTO-C01-CHARTER`, `UTO-C01-EVIDENCE`, and `UTO-LOOPBACK`.

Only same-head hosted evidence may populate the attribution rows or medians.
Cancelled, failed, duplicate, expired, and cross-head samples are excluded.
Detailed CI evidence stays in the PR comment; committed artifacts contain
aggregates and identifiers only. The clean-room loopback owns behavior and
failure sensitivity after wiring and hosted evidence are complete.

## Baseline verification boundary

Story 001 validates this JSON locally by checking its schema/version, required
fields, corpus constants, closed verdict set, unique identities, explicit
empty-state declarations, pilot disposition, and Markdown agreement. A
negative in-memory copy with a duplicate identity, missing field, or
unsupported verdict must fail validation without rewriting the committed file.

This proves the v1 evidence contract before admitting measurements. The
authorized attempt proves the jobs forwarding control and diagnostic
publication, but it does not prove valid cohorts, package classification,
stage attribution, performance, or the clean-room red path; those remain
blocked by the rejected invariants. The amendment permits this explicitly
bounded blocked delivery and leaves corrective instrumentation to a separate
owner.
