# C08 CI concurrency and gate-removal source plan

## Authority and source-plan resolution

The C08 task packet names `docs/temp/functional-test-optimization.md` as its
source plan. That path is absent from this checkout, `origin/main`, and Git
history. This document is the explicit, tracked replacement authority for the
C08 lane; it is a source-plan conflict resolution, not an inferred plan copied
from implementation evidence.

The checked-in `prd.json` remains the operator-authorized scope and acceptance
authority. Its `operatorAmendment` is null for this lane. Progress notes and PR
comments record evidence, but do not expand or replace the scope below.

## Behavior spine and outcome

`BEH-C08-FAST-GATE`: pull requests receive a materially faster single-job
Backend Functional Coverage check, and Verification Policy no longer waits on
the redundant changed-test replay, while coverage, quarantine, diagnostics,
failure attribution, and remaining policy checks retain their contracts.

The implementation boundary is the existing Go functional runners, the
functional coverage workflow cell, the Backend Test Stability callers and
implementation, and the policy composition that owns those callers. The
workflow remains one functional job; matrix sharding is not a solution in this
lane.

## Contract changes

### Runner command grammar

The instrumented coverage runner in
`cmd/gocoveragecheck/coverage_phases.go` changes from:

```text
go test -coverpkg=<packages> -p=<jobs> -count=1 -parallel=2 [-short] \
  -covermode=<mode> -timeout=<duration> <selected packages>
```

to:

```text
go test -coverpkg=<packages> -p=<jobs> [-short] \
  -covermode=<mode> -timeout=<duration> <selected packages>
```

Package selection, coverage instrumentation, timeout, timing output, failure
propagation, and diagnostic artifacts remain owned by the existing runner.
Go owns default in-package parallelism and cache eligibility after the forced
flags are removed.

The ordinary runner in `cmd/functionallane/main.go` changes from
`go test -p=<jobs> -parallel=2 ...` to `go test -p=<jobs> ...`. Its explicit
`-count=<n>` option remains supported for fresh or repeat investigations, and
the timing output and command failure behavior remain unchanged.

### Functional CI concurrency

The single functional matrix cell in `.github/workflows/ci.yml` selects
`functional_jobs=8`, logs the detected logical CPU value and selected jobs, and
passes that value through the existing `FUNCTIONAL_DEFAULT_JOBS` binding to
`gocoveragecheck`. There is no new matrix dimension, dispatch fan-out, or
distinct artifact root.

The same-SHA hosted measurement decision is:

| jobs | result | wall time | peak memory | eligibility |
| ---: | --- | ---: | ---: | --- |
| 2 | green | 8:07.28 | 784,600 kB | eligible |
| 8 | green | 6:06.02 | 789,104 kB | selected |
| 16 | six functional failures; coverage not evaluated | 6:08.12 | 792,988 kB | ineligible |

The exact run, job, artifact, quarantine, ACP, and attribution identities are
recorded in the PR measurement comment. The measurement SHA was
`d70d09c2b9dcb4f36799557be40935dcffc78131`. The hosted result chooses 8 as the
shortest eligible candidate; it does not authorize test-content changes.

### Stability-gate retirement

The `backend-test-stability` workflow job, its `cmd/teststability` source and
tests, its Makefile variables/target, and its Verification Policy dependency,
inputs, and lane row are removed together. The generic fail-closed policy,
Backend Unit Latency, Backend Functional Coverage, quarantine validation,
workflow lint, and all other policy lanes remain.

## Scope boundaries

In scope:

- remove the two forced Go test throttles;
- remove forced coverage `-count=1` and retain explicit fresh-run controls;
- select the measured jobs=8 functional configuration;
- delete the redundant stability gate and every owned caller;
- retain quarantine selection/ratchet, FLAKY metadata, coverage floors,
  timing diagnostics, artifacts, and readable package/test attribution;
- verify the runner, policy, workflow, and final-head behavior together.

Out of scope:

- matrix sharding or splitting the functional job;
- changing functional test bodies, fixtures, quarantine membership, or
  deflake policy;
- changing provider behavior, API contracts, generated clients, UI/browser
  behavior, shared-baseline workflows, or excluded baselines;
- adding paid/provider calls or relying on browser evidence;
- committing CI logs, downloaded artifacts, or timing evidence.

The accepted tradeoff is that changed tests lose the pre-merge three-attempt
stability replay. The surviving defenses are the unchanged functional
quarantine ratchet, FLAKY metadata, diagnostics, and dedicated deflake lanes.

## Work and gate map

| Work item | Required property | Evidence boundary |
| --- | --- | --- |
| `RUNNER-ARGS-01` | Removed flags are absent from the two owned runner grammars; explicit count remains available. | `go test ./cmd/gocoveragecheck ./cmd/functionallane -count=1` plus command-shape tests and the real-Go cache witness. |
| `CACHE-01` | Instrumented coverage output remains cache-eligible in the delivered flag shape. | PR comment with the exact real-package `-coverprofile` + `-json` command, first execution, second `(cached)` output, and profile identity. |
| `HOSTED-PERF-01` | Jobs=8 is selected from eligible same-SHA hosted measurements. | PR measurement comment with exact run/job/artifact identities and attribution inspection. |
| `QUARANTINE-01` / `DIAGNOSTICS-01` / `ATTRIBUTION-01` | Quarantine and diagnostic contracts survive the optimization. | Final Backend Functional Coverage artifacts and readable failure rows. |
| `POLICY-UNIT-01` / `POLICY-FINAL-01` | Remaining policy lanes are fail-closed without stability-gate inputs. | `node --test scripts/verification-policy.test.mjs scripts/ci/lane-budget.test.mjs` and final hosted Verification Policy. |
| `WORKFLOW-LINT-01` | Workflow syntax and lane wiring remain valid. | `make test-ci-workflows` and `actionlint .github/workflows/ci.yml`. |
| `FUNCTIONAL-FINAL-01` | Final Backend Functional Coverage is green with jobs=8 and required artifacts. | Current-head hosted CI; implementation stops after pushing the final head and starting CI. |
| `POSTMERGE-PERF-01` | Three uncached post-merge runs meet the PRD target, or the documented fallback binds. | Review-owned post-merge evidence; not an implementation-stage commit. |

## Verification and failure recovery

The focused command/property checks are:

```text
go test ./cmd/gocoveragecheck ./cmd/functionallane -count=1
node --test scripts/verification-policy.test.mjs scripts/ci/lane-budget.test.mjs
make test-ci-workflows
actionlint .github/workflows/ci.yml
```

The hosted final lane must retain the existing functional-boundary check,
quarantine evidence, coverage summary/profile, timing summary, ACP evidence,
and per-package failure attribution. A failed test is diagnosed from those
artifacts and reproduced at its owning package before considering any change;
the assertion or quarantine is not weakened to make the optimization pass.
Missing or malformed artifacts fail the lane. A quarantine mismatch remains a
policy failure. A policy failure is fixed only in the remaining policy inputs
or workflow wiring. High memory or host saturation is recorded as an
environmental observation unless the selected jobs configuration itself causes
an observable regression.

If the optimized final lane has an intermittent package failure, rerun the
unchanged required job once under the repository CI policy and preserve the
first-run attribution in the PR conversation. A deterministic regression is a
blocking implementation defect. A clean local reproduction that passes under
the hosted package topology is evidence against silently changing production
or functional-test content; route any independent contention repair to its
own lane.

## Rollback and delivery boundary

Rollback is one revert of the runner flag removal, jobs selection, and gate
retirement as required by the failure, while preserving the PRD's exclusions.
The final implementation head must be pushed with an open PR and required CI
started, and all blocking review feedback must be addressed. Review owns
terminal CI, conflict resolution, merge, and post-merge performance evidence.
CI evidence belongs in PR comments and never in a commit.
