# C13 functional jobs raise evidence ledger

## Lane and story boundary

This ledger records the baseline and scheduling trace for
`BEH-C13-RAISED-FUNCTIONAL-JOBS`, owned by
`functional-test-optimization-c13-functional-jobs-raise-001`. The story
establishes the evidence needed before the workflow value is edited. It does
not change `.github/workflows/ci.yml`, tests, quarantine membership, coverage
policy, generated files, or product behavior.

The authoritative Linux functional matrix cell is the single
`Backend Functional Coverage` job. The baseline below is a functional-lane
baseline, not a claim that every job in its containing workflow run was green.

## Prerequisite state and baseline decision

At collection time, `origin/main` was the merge commit for PR #2438. PR #2437
is an ancestor of the main history. The required isolation and critical-path
prerequisites are therefore present on `origin/main` before the candidate width
is changed.

The latest eligible completed post-#2437 main functional baseline was selected
from the latest completed push run whose Backend Functional Coverage job was
green:

| Property | Observed baseline |
| --- | --- |
| Hosted runner | Ubuntu 24.04 (`ubuntu-24.04`), GitHub Actions runner |
| Logical CPUs / selected jobs | `4 / 8` |
| Functional lane | `merge-full`, `short=false`, subtractive quarantine selection |
| Supervised functional step | `512.524s` from supervisor start through joined completion |
| Coverage child | `512.493s`, status `0` |
| Quarantine child | `21.668s`, status `0` |
| Functional timing | `302.721s` test wall time; `474.976s` compile + test diagnostic total |
| Inventory | `157` discovered packages / `1,067` discovered tests; `149` selected packages / `1,066` selected tests |
| Test result | `1,065` pass, `0` fail, `1` skip; complete timing capture |
| Coverage verdict | Green; `61.3%` against the `33.1%` minimum |
| Artifacts | Timing, coverage, command, quarantine, diagnostics, ACP, and critical-path artifacts present; quarantine and coverage statuses `0` |
| Eligibility | Eligible for the functional timing baseline because the functional job completed green with complete artifacts and the unchanged jobs-8 path |

The containing workflow run was red for unrelated `Backend Unit Latency` and
the dependent `Verification Policy` job. That fact is retained rather than
converted into a synthetic all-green baseline; it does not invalidate the
completed functional job as the functional-lane comparison point.

The newer main run on the #2438 merge was retained as ineligible: its
functional step completed, but the verdict was `test-failure` because
`tests/functional/workers/script` failed. It is evidence of a newer red main
functional result, not a width-attributable result and not a baseline.

The exact run/job URLs, full head identities, timestamp boundaries, and raw
log/artifact details are reserved for the PR evidence comment. This tracked
ledger keeps the summarized measurements and eligibility rationale only.

## Workflow-to-go-test scheduling trace

The selected value is carried through the existing path without an
authoritative-width cap:

| Boundary | Source behavior |
| --- | --- |
| Workflow selection | `.github/workflows/ci.yml` selected `functional_jobs=8` for the baseline and `functional_jobs=12` for the accepted candidate, logging `logical_cpus` and `jobs` and writing `jobs` to `GITHUB_OUTPUT`. |
| Workflow environment | The functional supervisor receives `FUNCTIONAL_DEFAULT_JOBS: ${{ steps.functional-parallelism.outputs.jobs }}`. |
| Make default/forwarding | `Makefile` defines `FUNCTIONAL_DEFAULT_JOBS ?= $(GO_LANE_BUDGET)` and passes it to `functional-test-viz -jobs $(FUNCTIONAL_DEFAULT_JOBS)` and `test-functional-coverage ... gocoveragecheck ... -jobs $(FUNCTIONAL_DEFAULT_JOBS)`. |
| Report runner | `cmd/functionaltestviz/suite.go` records `jobs=%d` and builds `gocoveragecheck -suite functional ... -jobs <value>`. |
| Coverage runner | `cmd/gocoveragecheck/coverage_phases.go` builds `go test ... -p=<cfg.testJobs(...)>`. `config.testJobs` returns a positive explicit `cfg.jobs` unchanged, so the authoritative functional invocation receives `-p=8` for the baseline and `-p=12` for the accepted candidate. |
| Package scheduler | Go owns package scheduling at the final `go test -p` boundary; no discovery helper cap rewrites that value. |

Preparatory paths have independent bounded caps and are not the coverage
package width:

| Preparatory path | Cap |
| --- | ---: |
| Functional package/test discovery worker and list batches | `4` (`functionalDiscoveryMaxJobs`) |
| Functional metadata `go list` batches | `8` (`functionalDiscoveryMetadataMaxJobs`) |
| Quarantine selector verification | `8` maximum (`maxFunctionalQuarantineVerificationJobs`) |

The command-composition proof confirms that an explicit
`FUNCTIONAL_DEFAULT_JOBS` override reaches functional discovery and coverage
command construction while the unit command retains its own default.

## Historical C08 decision context

C08 recorded a same-SHA comparison before the current isolation/critical-path
prerequisites:

| Jobs | Result | Wall time | Peak memory | Historical eligibility |
| ---: | --- | ---: | ---: | --- |
| 2 | green | `8:07.28` | `784,600 kB` | eligible |
| 8 | green | `6:06.02` | `789,104 kB` | selected |
| 16 | six functional failures; coverage not evaluated | `6:08.12` | `792,988 kB` | ineligible |

These values are decision context, not a current acceptance threshold. Story
002 selected width `16` as the first bounded candidate because the named
prerequisites were on main. A failure or capacity symptom was diagnosed from
its package and artifact evidence, and recovery was limited to `16 -> 12`;
width `12` was accepted without needing the final `10` candidate. The value
must not return to `8` or change test/policy inputs.

## Story 002 measured result and bounded recovery

The accepted authored workflow value is `functional_jobs=12`. Both hosted
candidate runs used Ubuntu 24.04 with four logical CPUs and the unchanged
complete functional lane:

| Candidate | Selected jobs | Supervised step | Functional test wall | Result | Decision evidence |
| --- | ---: | ---: | ---: | --- | --- |
| Width-16 candidate | `16` | `1010.457s` | `793.348s` | Failed: `tests/functional/transport/acp/stdio` timed out at `600.419s`; coverage was not evaluated | New timeout relative to the green jobs-8 baseline; recorded in the PR comment and recovered to `12` |
| Width-12 candidate | `12` | `485.685s` | `275.462s` | Green: `149/149` selected packages, `1,066` pass / `0` fail / `1` skip; coverage `61.3%` against `33.1%` minimum | Accepted first green raised width; quarantine, coverage, ACP, timing, diagnostics, command, and required verdict artifacts were present |

Compared with the eligible jobs-8 baseline (`512.524s` supervised and
`302.721s` functional test wall), the accepted jobs-12 run reduced supervised
time by `26.839s` (`5.2%`) and functional test wall time by `27.259s`
(`9.0%`). The observed width-attributable bottleneck was package contention in
the ACP stdio functional package at width 16; width 12 completed the same
package in `16.444s` and preserved the complete green lane. No width-10 run
was required after the first green raised candidate.

The PR evidence comment retains the exact run and job URLs, full heads,
timestamp boundaries, selected-width logs, artifact URLs, and failed
package/width pair. No raw CI log or artifact is committed here.

## Story 002 verification

Local focused gates on the width-12 authored workflow all passed:

- `make test-ci-workflows` — exit `0`; `160` tests passed and `1` supported
  environment-conditional test was skipped.
- `actionlint .github/workflows/ci.yml` — exit `0`.
- `node --test scripts/ci/lane-budget.test.mjs scripts/ci/functional-coverage-workflow.test.mjs`
  — exit `0`; `6/6` tests passed.

Remote candidate evidence passed at width 12 as summarized above. It proves
the hosted four-CPU runner selected the raised value, the value reached the
authoritative package scheduler, and the complete functional coverage lane
remained green with its required artifacts. Final-head independent loopback
and review-owned terminal CI/merge remain unproven here.

## Story 001 verification

Command:

```text
node --test scripts/ci/lane-budget.test.mjs
```

Observed result: exit `0`; four subtests passed, including the explicit
functional override assertion. This proves local Make command composition for
the functional discovery and coverage entrypoints. It does not prove raised
width safety, hosted contention, memory behavior, or final-head CI.

Remaining edges are owned by `CI-RAISED-01` (raised-width hosted behavior) and
`REVIEW-CI-01` (terminal final-head CI).
