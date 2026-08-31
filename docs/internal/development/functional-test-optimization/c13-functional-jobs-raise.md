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

## Historical C13 jobs-raise verification

The following command evidence belongs to the earlier jobs-raise lane and is
retained as historical context. The current post-C09 correction story's
verification and blocker are recorded in the section below.

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

## Story 001 post-C09 characterization

This section records the current correction lane's read-only characterization.
The retained artifacts are all from `main`, `merge-full`, `short=false`, the
Ubuntu 24.04 hosted runner, Go 1.25.0, and the schema-v2 diagnostic path. The
selected-width log in each job reported `Functional CI runner:
logical_cpus=4 jobs=12` and the functional command received
`FUNCTIONAL_DEFAULT_JOBS: 12`. Run and job identities are retained here; raw
URLs and downloaded artifacts remain PR evidence rather than repository files.

### Comparable post-C09 runs

| Run / Backend Functional Coverage job / head | Timing and inventory | Build/cache evidence | Critical path and outcome | Eligibility |
| --- | --- | --- | --- | --- |
| `33306288107` / `99243386085` / `767e28f06696102447d433076057f2ea4b5e7740` | Complete; `372.747s`; `149/149` packages; `1,079` tests: `1,076` pass / `2` fail / `1` skip | Schema v2; no cache match; `891` compiler + `147` linker = `1,038` build actions; `compile-work-observed`; command failed | Supervisor `10:22:39.442Z`–`10:31:14.844Z`; coverage `10:22:39.454Z`–`10:31:14.841Z`; quarantine `10:22:39.454Z`–`10:24:38.426Z`; statuses `0/0`; test-failure | Ineligible: red functional verdict; retained as ordinary-head failure evidence |
| `33306504049` / `99244523859` / `76262db480315bbe915a9037bc25dc4edf337ae1` | Complete; `219.970s`; `149/149` packages; `1,079` tests: `1,077` pass / `1` fail / `1` skip | Schema v2; exact primary hit; `0` compiler + `74` linker = `74` build actions; `compile-work-observed`; command failed | Supervisor `10:33:32.140Z`–`10:37:39.603Z`; coverage `10:33:32.153Z`–`10:37:39.600Z`; quarantine `10:33:32.153Z`–`10:33:42.113Z`; statuses `0/0`; test-failure | Ineligible: red functional verdict; retained to separate cache state from success |
| `33307796498` / `99247450066` / `8be2317b1ce84b1091038992bb3aa4ac7fd5d986` | Complete; `323.011s`; `149/149` packages; `1,079` tests: `1,078` pass / `0` fail / `1` skip | Schema v2; prefix match, `exactPrimaryHit=false`; `12` compiler + `141` linker = `153` build actions; `compile-work-observed`; command passed | Supervisor `11:01:25.167Z`–`11:07:15.952Z`; coverage `11:01:25.179Z`–`11:07:15.948Z`; quarantine `11:01:25.179Z`–`11:01:33.275Z`; statuses `0/0`; green | Eligible incident comparator and correction target |
| `33308660159` / `99249678769` / `1117d2db599e21ef56c433ff626ae673328b3444` | Complete; `214.055s`; `149/149` packages; `1,079` tests: `1,078` pass / `0` fail / `1` skip | Schema v2; exact primary hit; `0` compiler + `73` linker = `73` build actions; `compile-work-observed`; command passed | Supervisor `11:21:32.235Z`–`11:25:28.426Z`; coverage `11:21:32.264Z`–`11:25:28.422Z`; quarantine `11:21:32.264Z`–`11:21:36.998Z`; statuses `0/0`; green | Eligible same-width green reuse comparator |
| `33309463382` / `99251815827` / `745e2e88da2fa14e9a0d760c2dc0fb532314b7bd` | Complete; `226.886s`; `149/149` packages; `1,067` tests: `1,066` pass / `0` fail / `1` skip | Schema v2; prefix match, `exactPrimaryHit=false`; `2` compiler + `73` linker = `75` build actions; `compile-work-observed`; command passed | Supervisor `11:40:27.695Z`–`11:44:39.171Z`; coverage `11:40:27.700Z`–`11:44:39.167Z`; quarantine `11:40:27.700Z`–`11:40:32.964Z`; statuses `0/0`; green | Eligible current-inventory green comparator |
| `33310295950` / `99254214168` / `a44ed015421b1ed42b919f1178531f38fd5087b5` | Complete; `173.204s`; `149/149` packages; `1,068` tests: `1,067` pass / `0` fail / `1` skip | Schema v2; prefix match, `exactPrimaryHit=false`; `2` compiler + `74` linker = `76` build actions; `compile-work-observed`; command passed | Supervisor `12:03:11.763Z`–`12:06:23.539Z`; coverage `12:03:11.773Z`–`12:06:23.536Z`; quarantine `12:03:11.773Z`–`12:03:15.814Z`; statuses `0/0`; green | Eligible current-inventory green comparator |

The green `323.011s` and `214.055s` runs have the same selected width,
schema, package accounting, test accounting, coverage outcome, quarantine
outcome, and ACP outcome. Their numeric difference is `323.011 - 214.055 =
108.956s` (`33.7%` of the slower run), while the incident threshold gap is
`323.011 - 302.721 = 20.290s`. The slower run restored a non-exact cache
prefix and executed `153` build actions; the faster run restored the exact
invocation key and executed `73` link-only actions. The two observations make
the reclaim envelope larger than the required target without mistaking a red
run or an incomplete artifact for success.

The package distribution supports the same conclusion. In the `323.011s`
run, several packages that were near-zero in the exact-cache `214.055s` run
expanded together: `workers/transports/cli/run/modes` was `65.857s` versus
`0.020s`, `sessions/chat_sessions/root_composition` was `43.298s` versus
`0.040s`, `workers/transports/cli/run/lifecycle` was `35.500s` versus
`0.010s`, and `factory/packaged/catalog` was `29.550s` versus `0.070s`.
`providers/agy` remained about `17`–`18s` in both runs. This is broad build or
setup work, not one stable functional test tail. The later green runs also
span `226.886s` and `173.204s` with the same `149/149` package accounting,
which is additional host/cache variance evidence.

### Controlled topology witness

The current quarantine manifest contains eight package selectors and one
test-level selector, `providers/agy#TestAgyLiveSmoke`. The production selection
code in `cmd/gocoveragecheck/functional_quarantine.go` therefore creates an
unrestricted package group and a precise `-run` group. The invocation builder
in `cmd/gocoveragecheck/coverage_invocation.go` deliberately executes one
coverage invocation per group so the test-level selector cannot be applied to
unrelated packages.

The existing controlled witness was run as:

```text
go test ./cmd/gocoveragecheck -run 'TestFunctionalCoverageSelectionSubtractsOnlyDeclaredSelectors|TestBuildFunctionalCoverageInvocationPlanKeepsRunGroupsSeparate' -count=1
```

It exited `0` (`0.034s`). The first test proves that declared package and test
selectors produce one unrestricted and one precise group. The second proves
that the groups remain separate, that only the precise group receives its
`-run` pattern, and that intermediate coverage profiles are isolated. This
confirms the current executable-spine topology, but it does not measure the
hosted wall contribution of the second group.

### Correction boundary and structured blocker

The measured `108.956s` reclaim envelope is attributable to the
source-sensitive Actions Go build-cache restore state: the repository-owned
boundary is the functional cache restore/save key and prefix in
`.github/workflows/ci.yml` (the `functional-coverage-build-*` block), not a
package scheduling decision. The supervisor critical paths show quarantine
finishing within roughly `5`–`10s` while the coverage child spans almost the
entire supervised interval, so supervisor overlap is not a demonstrated
`20.290s` contributor.

**Structured blocker `C13-001-CHAR-01`:** no retained artifact contains
per-group timing or another controlled observation that assigns the required
`20.290s` to the internal two-group topology in
`cmd/gocoveragecheck/coverage_invocation.go`. Changing that topology could
alter the current per-package quarantine semantics, and claiming the whole
cache-state delta as an internal Go correction would be unsupported. Story
002 explicitly forbids editing `.github/workflows/ci.yml`, so the smallest
safe delta plan is either (a) authorize the workflow cache restore/reuse
boundary as the correction owner, or (b) add a bounded per-group setup
measurement before selecting a `cmd/gocoveragecheck` correction. No runtime,
workflow, configuration, selection, or failure policy was changed in this
story.

This satisfies the characterization story as a structured blocker. The
correction behavior, final-head hosted wall, and post-merge main behavior
remain unproven and are carried by GATE-FOCUSED, GATE-PR, and
GATE-MAIN-LOOPBACK respectively.

## Story 002 post-Amendment2 remeasurement

After operatorAmendment2 lifted the temporary C13 workflow-cache exclusion,
the branch was rebased onto current `origin/main` at `9c5e2bc6cb`. The
post-#2466 current-main run was then inspected read-only before considering a
new edit. The C09 cache topology is present in the rebased source: module and
build caches are separate, the build key includes `jobs=12` and authored build
inputs, restore identity is forwarded to the diagnostic, and a non-exact
restore is saved under its primary key.

| Property | Post-C09 remeasurement |
| --- | --- |
| Main head / run / Backend Functional Coverage job | `9c5e2bc6cb4e55e4ec0bb02bf1bcafeccf41322e` / `33328050085` / `99301657776` |
| Hosted runner / selected width | Ubuntu 24.04 / `logical_cpus=4`, `jobs=12` |
| Functional timing | Complete; `wallSeconds=180.497` |
| Inventory and outcomes | `149/149` selected packages; `1,063` tests: `1,062` pass / `0` fail / `1` skip |
| Build/cache evidence | Schema v2; exact primary-key hit; `0` compiler + `73` linker = `73` build actions; `compile-work-observed`; command passed |
| Coverage, quarantine, ACP | Coverage green at `61.6%` against `33.1%`; quarantine `9/9` expected skips and status `0`; pinned ACP evidence passed and cleanup was complete |
| Required artifacts | Timing, coverage, command, diagnostics, verdict, quarantine, ACP, and critical-path artifacts present and complete |

The remeasurement is `142.514s` faster than the prior green incident
comparator (`323.011 - 180.497`) and `122.224s` below the jobs-8 threshold
(`302.721 - 180.497`), so no residual C13 correction is needed to cross the
acceptance wall. The exact-key `73`-action result shows that the source-
sensitive cache boundary already merged in C09 has absorbed the characterized
cache-state regression. This is the operator-authorized terminal no-op
outcome; no second cache-key, width, `-p=12`, quarantine, inventory, or
failure-policy change is justified. The final-head hosted proof remains the
story-003 edge, and first natural post-merge behavior remains
GATE-MAIN-LOOPBACK.
