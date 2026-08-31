# Cycle 01 cost-attribution inventory v1

Status: baseline contract established; the first hosted diagnostic was
captured but rejected, so no stage or package rows are admitted.

Canonical source: [`c01-cost-attribution-inventory.json`](c01-cost-attribution-inventory.json)

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

Each captured row must contain `identity`, `bucket`, `measuredSeconds`,
`measurementSource`, `measurementStatus`, `verdict`, `owningLane`, and
`blockedBy`. Captured seconds must be finite and non-negative, and the source
must identify a retrievable artifact, command, run, and job. Measurement
status is either `measured` or `inferred`; an obtainable bucket above 30
seconds cannot remain inferred at U01-G2 completion. Allowed verdicts are:
`INFRASTRUCTURE-FIX`, `NEEDS-EXPERIMENT`, `CONVERT`, `KEEP-AS-IS`, and
`OUT-OF-SCOPE`.

## Package classification contract

The inventory expects exactly 445 unique Go import-path rows under the
`fixed-package-overhead` bucket. The classification basis is 0.322 seconds
per package. The package array is empty because the only hosted timing
artifact was incomplete and failed the inherited test/skip invariants. The
expected count and row fields remain canonical and reviewable. A conversion or
consolidation is not justified by the count alone.

## Parallelism experiment

The required hosted pair is:

| Parameter | Run/job identity | Raw wall |
| --- | --- | --- |
| `-p=1` | not captured | not captured |
| `-p=4` | not captured | not captured |

The collapsed-parallelism verdict is intentionally not recorded until both
same-head cells complete. The current workflow has no jobs input or matrix
entry for these cells; adding one is an explicit scope/owner delta, not a
local timing substitute.

## Hosted evidence attempt (not admitted)

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

This proves the v1 evidence contract before admitting measurements. The hosted
attempt proves default-path diagnostic publication, but it does not prove
valid cohorts, `-p` behavior, package classification, stage attribution,
performance, or the clean-room red path; those remain blocked by the rejected
invariants and missing jobs control.
