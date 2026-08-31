# Unit Test Optimization — Cycle 01 Charter

Status: baseline charter established; the first hosted diagnostic was captured
but rejected for U01 attribution because the inherited timing invariants were
not complete and the workflow has no selectable jobs control.

This file is the governing source plan for Cycle 01. The path named by the
execution packet was absent from the base checkout, so this charter is created
from the operator-supplied requirement identifiers and reference observations.
Those identifiers are the authority for this lane until a later revision is
explicitly approved.

## Behavior lane

**BEH-UTO-C01:** Maintainers receive measured, attributable Backend Unit
Coverage diagnostics without changing the tested corpus or its verdicts.

The intended executable spine is:

```text
Backend Unit Coverage environment
  -> make test-unit-coverage
  -> existing coverage-build-diagnostics writer
  -> uploaded JSON
  -> reconciled attribution inventory and evidence ledger
```

The default-off wrapper, Make forwarding, and unit CI artifact upload are now
present. The first same-head hosted observation proved that the existing
diagnostic writer reaches the published artifact, but it is not a valid U01
cohort: the timing capture is incomplete, the observed test count is below the
charter invariant, and the workflow cannot select `-p=1` or `-p=4`. No stage or
package optimization claim is made.

## Problem and desired outcome

The supplied reference observation recorded a 649-second Backend Unit Coverage
job, including a 607-second coverage step. The same observation recorded
342.9 seconds of `go test` wall time, 43.7 seconds of test execution, and a
264.1-second residual that has not been attributed to a measured build stage.
The residual is a measurement target, not a bucket value and not an
optimization result.

Cycle 01 will produce a versioned, reviewable attribution record and then use
the existing coverage build diagnostic to measure compile, link, covdata,
merge, and evaluate work on the same hosted head. Coverage behavior must stay
characterized at 445 packages, 18,273 tests, at most 57 skips, 80.7% aggregate
coverage, and 99,418 profile blocks.

## Targets and explicit non-targets

`UTO-TARGET` sets these project targets:

| Lane | Target | Evidence owner |
| --- | ---: | --- |
| Backend Unit Coverage | at most 240 seconds | U01 |
| Backend Unit Latency | at most 200 seconds | later lane; outside Cycle 01 implementation |

The targets are evaluation goals, not local timing thresholds. Hosted
three-run medians are authoritative. Local timings may characterize behavior
but cannot establish a portable pass/fail threshold on the known saturated
host.

Sub-two-minute coverage is rejected for the current scope. The lane still
links 445 instrumented test binaries, runs 18,273 tests, emits a 99,418-block
profile, evaluates 445 package floors, and has a 147.3-second irreducible
per-package coverage floor. This makes a sub-two-minute claim neither
achievable nor an acceptable Cycle 01 success criterion. No future inventory
row may use this rejected target as an optimization claim.

Cycle 01 does not change test bodies, package membership, floor manifests,
cache or setup behavior, Backend Unit Latency, product APIs, runtime state, UI,
or sibling optimization lanes. U02, U03, and U04 remain blocked until this
lane merges.

## Governing requirements

The following identifiers are the complete authority set supplied for this
lane. Every story and every inventory decision must point to one or more of
these identifiers.

| Identifier | Requirement captured by this charter | Owning evidence |
| --- | --- | --- |
| `UTO-TARGET` | Record the 240-second coverage and 200-second latency targets and reject an unsupported sub-two-minute target. | This charter; U01-G2/U01-G3 |
| `UTO-C01-ATTRIBUTION` | Attribute compile, link, covdata, merge, and evaluate work from retrievable same-head evidence; do not turn arithmetic residuals into facts. | Inventory v1; U01-G2 |
| `UTO-C01-U01` | Wire the existing build diagnostic into Backend Unit Coverage with optional default-compatible Make behavior. | Story 002; U01-G1 |
| `UTO-C01-PARALLELISM-EXPERIMENT` | Compare hosted `-p=1` and `-p=4` raw wall times with their run and job identities. | Story 003; U01-G2 |
| `UTO-C01-PACKAGE-CLASSIFICATION` | Classify all 445 packages against the measured 322ms fixed-package overhead before proposing conversion or consolidation. | Inventory contract; Story 003/U01-G2 |
| `UTO-C01-PILOT-DISPOSITION` | Keep the 10.63-second factory-sessions pilot as `KEEP-AS-IS` because it cannot materially reduce a 649-second infrastructure wall. | Inventory v1; Story 003 |
| `UTO-C01-CHARTER` | Establish this source charter, ownership, scope, terminology, evidence rules, and rollback boundary before wiring. | Story 001/U01-G0 |
| `UTO-C01-EVIDENCE` | Reconcile only valid same-head hosted artifacts, preserve provenance, and keep detailed CI evidence in the PR comment. | Stories 003–004/U01-G2/U01-G4 |
| `UTO-LOOPBACK` | Run the clean-room validation template, report defects without silently repairing them, and re-enter the established thoughts loopback only after implementation and review. | Story 004/U01-G3/U01-G4 |

### Story trace

| Story | Outcome | Governing identifiers |
| --- | --- | --- |
| `unit-test-optimization-c01-unit-coverage-build-diagnostics-001` | Publish one canonical, default-compatible unit-coverage build diagnostic. | `UTO-C01-U01`, `UTO-LOOPBACK` |
| `unit-test-optimization-c01-unit-coverage-build-diagnostics-002` | Reconcile valid same-head hosted attribution, parallelism, medians, and all package classifications; retain rejected attempts as blockers. | `UTO-C01-ATTRIBUTION`, `UTO-C01-PARALLELISM-EXPERIMENT`, `UTO-C01-PACKAGE-CLASSIFICATION`, `UTO-C01-PILOT-DISPOSITION`, `UTO-C01-EVIDENCE` |
| `unit-test-optimization-c01-unit-coverage-build-diagnostics-003` | Independently validate the integrated lane, deliberate red path, scope isolation, and review handoff. | `UTO-LOOPBACK`, `UTO-C01-EVIDENCE`, `UTO-TARGET` |

## Baseline characterization

The baseline identifiers are retained as reference metadata only:

| Field | Value | Interpretation |
| --- | ---: | --- |
| Hosted run | `33322584614` | Supplied reference run |
| Hosted job | `99287112602` | Supplied Backend Unit Coverage job |
| Job wall | 649 seconds | Current infrastructure observation |
| Coverage-step wall | 607 seconds | Current coverage observation |
| `go test` wall | 342.9 seconds | Current test-command observation |
| Test execution | 43.7 seconds | Current observed execution component |
| Unattributed residual | 264.1 seconds | Requires stage measurement; not assigned to a bucket |
| Packages | 445 | Corpus invariant |
| Tests | 18,273 | Corpus invariant |
| Maximum skips | 57 | Corpus invariant |
| Aggregate coverage | 80.7% | Coverage invariant |
| Profile blocks | 99,418 | Coverage artifact characterization |
| Fixed package overhead | 0.322 seconds | Classification basis |
| Irreducible coverage floor | 147.3 seconds | Sub-two-minute rejection basis |

The hosted run and job are supplied reference evidence. Raw logs, command
traces, runner paths, credentials, and test payloads are not committed here.
The reference does not prove diagnostic publication, `-p` behavior, future
runner stability, or an optimization.

## Inventory contract

`c01-cost-attribution-inventory.json` is the canonical v1 record.
`c01-cost-attribution-inventory.md` is its human-readable projection. The
contract records reference constants and retains rejected hosted attempts;
the `buckets` and `packages` arrays remain empty until valid same-head evidence
preserves every inherited invariant. Empty collections are an explicit blocked
state, not zero-cost measurements.

### Attribution buckets

The only bucket identities are `compile`, `link`, `covdata`, `merge`, and
`evaluate`. Each captured row must contain:

`identity`, `bucket`, `measuredSeconds`, `measurementSource`,
`measurementStatus`, `verdict`, `owningLane`, and `blockedBy`.

`identity` is stable and unique within the inventory. `measuredSeconds` is
finite and non-negative when a measurement is captured. `measurementSource`
must identify a retrievable artifact, command, run, and job. The only row
measurement statuses are `measured` and `inferred`; an inferred value cannot
support an optimization claim, and an obtainable bucket above 30 seconds may
not remain inferred when U01-G2 completes. The only verdicts are:

`INFRASTRUCTURE-FIX`, `NEEDS-EXPERIMENT`, `CONVERT`, `KEEP-AS-IS`, and
`OUT-OF-SCOPE`.

No bucket row is admitted because the first hosted observation failed the
inherited timing/test invariants. The hosted evidence owner must reject
duplicate identities, missing fields, cross-head evidence, and unsupported
verdicts rather than turning the rejected observation into a measurement.

### Package classifications

The package classification contract expects exactly 445 unique Go import-path
rows. Each row uses the same identity, seconds, source, status, verdict,
owner, and blocker fields; its bucket is `fixed-package-overhead`. The
classification basis is 0.322 seconds per package. The baseline records the
expected count and contract only; the rejected hosted observation cannot
supply the complete row set. A package conversion or consolidation is not
justified by the count alone.

### Parallelism and pilot state

The required parallelism experiment has two inputs, hosted `-p=1` and hosted
`-p=4`. Neither has been captured. The current CI workflow exposes one unit
matrix entry without a jobs input, so it cannot produce those cells without a
small, explicitly authorized hosted-run configuration delta. No collapsed-
parallelism verdict is recorded.

## Hosted evidence attempt (not admitted)

The first hosted run on the implementation head published the diagnostic
artifact, but the observation is retained only as a rejection record in the
canonical JSON. It is not a cohort sample, median input, bucket measurement,
or package-classification source.

| Field | Observation |
| --- | --- |
| Head SHA | `8f810fc94ceb449e90d7733394078e57f4da44a3` |
| Run / job | `33345809127` / `99349658743` |
| Artifact | `unit-coverage-diagnostics` / `9742044437` |
| Diagnostic path | `coverage-build-diagnostics.json` |
| Go version | `go1.25.0` |
| Diagnostic result | `complete`, `commandResult=passed`, `wallSeconds=239.885` |
| Build observations | `compilerCommands=2087`, `linkerCommands=445`, `buildActions=2532`, `expectedPackages=445` |
| Coverage observation | `80.7%`, `107586/133294` statements, 480 measured coverage packages |
| Timing observation | `complete=false`, 445 package rows, `testCount=11560`, `testFailCount=0`, `testSkipCount=57` |
| Admission verdict | rejected: expected 18,273 tests, skips below 57, and a valid 445-package evidence universe |

The detailed run and artifact links are retained in the PR comment. Raw logs,
profiles, traces, runner paths, credentials, and test payloads are not
committed.

The factory-sessions execution pilot is already measured at 10.63 seconds and
has the disposition `KEEP-AS-IS`. Its cost cannot materially reduce the
649-second infrastructure wall, so Cycle 01 does not convert or consolidate
that package. This disposition is independent of the later full 445-package
classification.

## Evidence, ownership, and safety rules

- The committed JSON inventory is canonical; Markdown is a projection, hosted
  raw artifacts are ephemeral, and detailed CI evidence belongs in the PR
  comment.
- Every measurement must be same-head, retrievable, and linked to its run/job
  identity. Cancelled, failed, duplicated, expired, or cross-head samples do
  not enter a median or optimization claim.
- Three-run hosted medians, not a noisy local wall clock, are the performance
  evidence. The hosted budget is bounded by the existing Actions job timeout
  and repository quota; no paid API call is required.
- `Makefile` owns local invocation configuration. `.github/workflows/ci.yml`
  owns hosted environment and artifact publication. The diagnostic writer owns
  its JSON output. Story 001 owns the implementation spine; Story 002 owns
  valid hosted evidence and this charter/inventory projection.
- No setup/cache, latency, floor, test-body, package-membership, generated
  contract, product API, UI, or runtime-state changes belong to Cycle 01.
- The optional path is fail-closed for probe and test errors, preserves the
  current unset-path behavior, and is reverted by removing its additive wiring
  and this charter/inventory together.
- Committed evidence contains aggregates and identifiers only. It must not
  contain raw build traces, secrets, credentials, absolute runner paths, or
  test payloads.

## Verification boundary

The local-real witness for Story 001 parses the JSON, checks the schema and
version, validates required top-level fields and corpus constants, verifies
unique identities and the closed verdict list for any rows, checks the
explicit empty-state declarations, and compares the Markdown constants and
pilot disposition with JSON. A negative copy with a duplicate identity,
missing row field, or unsupported verdict must be rejected in memory without
rewriting the committed artifact.

This proves that the evidence contract is machine-readable, complete, and
conservative before admitting measurements. The hosted attempt additionally
proves default-path diagnostic publication and preserves the aggregate 80.7%
coverage observation, but it does not prove valid cohorts, package
classification, stage attribution, or performance. Those edges remain blocked
until the timing/invariant capture and selectable jobs control are resolved.

## Delivery sequence

1. Story 001 establishes this charter, baseline inventory, and the
   default-compatible diagnostic spine.
2. Story 002 captures and validates same-head hosted evidence, attributes cost,
   classifies all packages, and publishes detailed CI evidence in a PR
   comment. Rejected observations remain explicit blockers.
3. Story 003 runs the validation-loopback template, checks invariants and the
   uncommitted red path, and hands the final pushed head to review.

The review stage owns terminal CI, conflict resolution, and merge. The
established thoughts loopback becomes eligible only after this lane's
implementation and review dependencies complete and must judge behavior, not
timing numbers alone.
