# Stability cleanup C12: deliberate witness admission

Status: **Story 003 clean-room validation complete; report-only implementation handoff ready for review**

This report contains the story-001 factual ledger, the story-002 candidate
decision, and the story-003 clean-room loopback. It does not change a baseline,
alter a workflow, implement a future source witness, or use a synthetic
candidate. This Markdown artifact is not compiled, packaged, or executed by
the untouched `tests/functional/factory/packaged/full_flow` package, so any
functional-test result is review-owned CI evidence rather than a report
behavior claim. The final head/required-check handoff remains the
implementation finish boundary; terminal CI outcomes belong in the PR
conversation and merge remains review-owned.

## Observation boundary

The live `origin/main` pin was observed at `2026-08-29T22:54:50.3876276Z`:

```text
995137125a6f90bec0284cbe2ea1783e70b5d063
```

The pinned commit is a squash merge of bot PR [#2462](https://github.com/portpowered/you-agent-factory/pull/2462), with
merge commit [`995137125a6f90bec0284cbe2ea1783e70b5d063`](https://github.com/portpowered/you-agent-factory/commit/995137125a6f90bec0284cbe2ea1783e70b5d063) and parent
[`a27ea892f5feb5ada9578d0da8159bfe3b590107`](https://github.com/portpowered/you-agent-factory/commit/a27ea892f5feb5ada9578d0da8159bfe3b590107). Evidence below was collected
through `2026-08-29T22:55:26Z`; asynchronous checks still in progress at that
boundary remain unproven.

## Pinned checkout and source authority

Procedure:

```text
git ls-remote origin refs/heads/main
git clone --quiet --no-tags --branch main --single-branch --depth 1 \
  https://github.com/portpowered/you-agent-factory.git <disposable-checkout>
git rev-parse HEAD
git status --porcelain=v1
```

The disposable checkout was
`C:\Users\andre\AppData\Local\Temp\c12-admission-current-0ee91b1a602a4827a5d070fd3e062b47`.
Its `HEAD` was the 40-character pin above and `git status --porcelain=v1`
was empty. The commit subject was
`chore(ci): reconcile shared CI baselines (#2462)`.

The governing plan named by the PRD is not present at the pin:

```text
git ls-tree -r --name-only 995137125a6f90bec0284cbe2ea1783e70b5d063 -- docs/temp/stability-cleanup.md
# no output

gh api repos/portpowered/you-agent-factory/contents/docs/temp/stability-cleanup.md?ref=995137125a6f90bec0284cbe2ea1783e70b5d063
# HTTP 404: {"message":"Not Found", ...}
```

This is an authority gap, not permission to reconstruct the source plan from
comments or prior reports. The exact AC1 candidate rule therefore remains
unproven.

The immutable source blobs inspected at the pin were:

| Pinned path | Blob SHA |
| --- | --- |
| [`.github/workflows/ci.yml`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/.github/workflows/ci.yml) | `241d419b911556fb6212e68d7ef0844ae2f12287` |
| [`.github/workflows/regenerate-shared-ci-baselines.yml`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/.github/workflows/regenerate-shared-ci-baselines.yml) | `ac9dafcbe4af0a93718674f161751f1b792af090` |
| [`scripts/ci/backend-lint-policy.mjs`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/scripts/ci/backend-lint-policy.mjs) | `a3ada462aa63638416f633845e335d70aa912e8c` |
| [`scripts/ci/backend-lint-report.mjs`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/scripts/ci/backend-lint-report.mjs) | `014ee663f030f8ffa564e93aa5c4bee71af171d2` |
| [`scripts/ci/shared-baseline-regeneration-workflow.mjs`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/scripts/ci/shared-baseline-regeneration-workflow.mjs) | `80c768e61de8d4a6e428ac13d3485baeb1e33486` |
| [`scripts/verification-policy.mjs`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/scripts/verification-policy.mjs) | `ab127508010ac583b98e9b0c943f50bb6a86bb00` |
| [`cmd/deadcodecheck/main.go`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/cmd/deadcodecheck/main.go) | `76fec8f267dea6649c845909c38a5e389ddf35e8` |
| [`cmd/unitlanebudget/main.go`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/cmd/unitlanebudget/main.go) | `ffa2267bf8d10ba162207f8091450bb0775f17ff` |
| [`cmd/unitlanebudget/budget.go`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/cmd/unitlanebudget/budget.go) | `5d82792f8136239343c814d1b2d052ea5fe2cce4` |
| [`Makefile`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/Makefile) | `6135fad496105721d86e7f0a1669622e691b72e7` |
| [`docs/internal/baselines/deadcode-baseline.txt`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/docs/internal/baselines/deadcode-baseline.txt) | `358716aeb0095882890819e58e0b98c09a8c9993` |
| [`docs/internal/baselines/go-unit-lane-latency-budget.v1.json`](https://github.com/portpowered/you-agent-factory/blob/995137125a6f90bec0284cbe2ea1783e70b5d063/docs/internal/baselines/go-unit-lane-latency-budget.v1.json) | `695309991f927099fd24f98c1a18b2b74c273c77` |

At this pin the deadcode baseline is 3,074 lines with SHA-256
`F31645C911B22D76E5A121E0DA0C47D5549DE16045E1D803E0003A254AFDFE13`.
The latency baseline has SHA-256
`578845BCD39D19DF70D943350810F5D6B8524E7D9D8FF2EEBCF9F1D3260D3693`,
`reference.baseCommit` equal to the parent
`a27ea892f5feb5ada9578d0da8159bfe3b590107`, 445 packages, 18,239 tests,
reference median 239.612 seconds, and computed lane budget 2. Its policy is
three samples, at least 25% median improvement, no sample more than 10% above
the sample median, zero cached packages, zero unknown packages, and exact
inventory with reviewed diff.

## Protected admission authority

The repository-level merge settings allow squash, merge-commit, and rebase
methods, with auto-merge enabled and branch deletion disabled. The active
branch-targeted rulesets are the authority:

| Ruleset | Exact observed protection |
| --- | --- |
| `main-protect` ([ruleset 15809936](https://api.github.com/repos/portpowered/you-agent-factory/rulesets/15809936)) | Active on the default branch; deletion and non-fast-forward rules; no bypass actors; current user cannot bypass. |
| `must-pass-pr` ([ruleset 15943501](https://api.github.com/repos/portpowered/you-agent-factory/rulesets/15943501)) | Active on the default branch; deletion, non-fast-forward, and required-status-check rules; required contexts are exactly `Verification Policy` and `Backend Lint`; strict required-status policy is `false`; no bypass actors; current user cannot bypass. |

The legacy `branches/main/protection` endpoint returned `404 Branch not
protected`; that endpoint is not treated as evidence that the active rulesets
do not protect `main`.

## Comparison-unit contracts

### Backend lint / deadcode

The pinned `cmd/deadcodecheck` runs
`golang.org/x/tools/cmd/deadcode@v0.25.1 ./...`, normalizes line endings,
backslashes, whitespace, and ordering, then compares the resulting report
exactly with `docs/internal/baselines/deadcode-baseline.txt`. Drift exits
non-zero and emits `LINT_VIOLATION_COUNT`. The backend-lint policy makes
deadcode and service-cycle-check required no-allowance targets; only the
packaged-factory checker has a committed allowance.

For pull requests, the pinned Backend Lint workflow selects and checks the
event's `github.sha` merge result, records `BACKEND_LINT_TESTED_SHA`, runs the
canonical lint target, and uploads `backend-deadcode-evidence` plus the lint
diagnostics. For a push to `main`, the tested SHA is the pushed commit.
`Verification Policy` requires the selected Backend Lint result to be
`success`; missing, skipped, or failed selected results fail policy.

### Backend unit-lane latency

The pinned Backend Unit Latency workflow checks out the PR-head expression for
pull requests and the pushed SHA for pushes. It runs exactly three
`make test-unit-fresh` samples with `UNIT_DEFAULT_JOBS=2`, writes version-2
sample evidence, and runs `make test-unit-latency-budget`. The checker requires
complete samples with exact commit identity, 445-package and 18,239-test
inventory, runner `ubuntu-24.04`, Go `1.25.0`, two jobs, zero cached or unknown
packages, the committed reference shape, at least 25% median improvement, and
no run more than 10% above the sample median. It uploads
`backend-unit-latency-evidence`. `Verification Policy` requires this selected
result to be `success` when the lane is enabled.

One relevant identity fact is intentionally recorded for story 002: the
latency checker shape-validates `reference.baseCommit` but does not require it
to equal the sample commit, and the regeneration helper's canonical comparison
ignores only that field. This is a policy fact to assess against the missing
source plan, not a candidate verdict.

## Remote-real lineage observed

The following chain is complete for the predecessor source revision that
produced the bot PR subsequently merged into the pinned `main`:

| Stage | Identity and result |
| --- | --- |
| Source push CI | [Run 33278765602](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602), SHA [`a27ea892f5feb5ada9578d0da8159bfe3b590107`](https://github.com/portpowered/you-agent-factory/commit/a27ea892f5feb5ada9578d0da8159bfe3b590107), completed `success`. Classify Verification [job 99170600461](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/job/99170600461), Backend Lint [job 99170600489](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/job/99170600489), Backend Unit Latency [job 99170600477](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/job/99170600477), and Verification Policy [job 99171691695](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/job/99171691695) all succeeded. |
| Deadcode witness | Artifact `backend-deadcode-evidence`, ID `9722393303` ([artifact page](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/artifacts/9722393303); [API resource](https://api.github.com/repos/portpowered/you-agent-factory/actions/artifacts/9722393303)); SHA-256 exactly matched the committed 3,074-line baseline. |
| Latency witness | Artifact `backend-unit-latency-evidence`, ID `9722470916` ([artifact page](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/artifacts/9722470916); [API resource](https://api.github.com/repos/portpowered/you-agent-factory/actions/artifacts/9722470916)); three complete files had wall times 97.471, 91.656, and 91.728 seconds, exact identity/inventory, median improvement 61.72%, and maximum run above median 6.26%. |
| Protected regeneration | [Run 33279349933](https://github.com/portpowered/you-agent-factory/actions/runs/33279349933), [job 99171730943](https://github.com/portpowered/you-agent-factory/actions/runs/33279349933/job/99171730943), completed `success`. Its log records source SHA [`a27ea892f5feb5ada9578d0da8159bfe3b590107`](https://github.com/portpowered/you-agent-factory/commit/a27ea892f5feb5ada9578d0da8159bfe3b590107), source run [33278765602](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602), exact-source checkout, generated path `docs/internal/baselines/go-unit-lane-latency-budget.v1.json`, and `SHARED_BASELINE_RECONCILIATION action=merge-requested publish=true`. The helper reported `quiescent=false` because the source revision also contained three functional-worker test paths; this is recorded, not normalized away. |
| Bot PR checks | [PR #2462](https://github.com/portpowered/you-agent-factory/pull/2462), base [`a27ea892f5feb5ada9578d0da8159bfe3b590107`](https://github.com/portpowered/you-agent-factory/commit/a27ea892f5feb5ada9578d0da8159bfe3b590107), head [`2054022a7df746e271f7da597653844b7a801cdc`](https://github.com/portpowered/you-agent-factory/commit/2054022a7df746e271f7da597653844b7a801cdc), [generated diff](https://github.com/portpowered/you-agent-factory/pull/2462/files) ([raw diff](https://github.com/portpowered/you-agent-factory/pull/2462.diff)). Its exact patch was one line added/removed in the latency budget's `reference.baseCommit`; no deadcode-baseline change. The validation run [33279420389](https://github.com/portpowered/you-agent-factory/actions/runs/33279420389) recorded Classify [job 99171916675](https://github.com/portpowered/you-agent-factory/actions/runs/33279420389/job/99171916675), Workflow Lint [job 99171916709](https://github.com/portpowered/you-agent-factory/actions/runs/33279420389/job/99171916709), Backend Unit Latency [job 99171916712](https://github.com/portpowered/you-agent-factory/actions/runs/33279420389/job/99171916712), Backend Lint [job 99171916748](https://github.com/portpowered/you-agent-factory/actions/runs/33279420389/job/99171916748), and Verification Policy [job 99172635713](https://github.com/portpowered/you-agent-factory/actions/runs/33279420389/job/99172635713), all completed successfully. |
| Bot merge | PR #2462 merged at `2026-08-29T22:54:47Z` as [`995137125a6f90bec0284cbe2ea1783e70b5d063`](https://github.com/portpowered/you-agent-factory/commit/995137125a6f90bec0284cbe2ea1783e70b5d063). |

The preceding failed regeneration [run 33278920011](https://github.com/portpowered/you-agent-factory/actions/runs/33278920011), [job 99170605175](https://github.com/portpowered/you-agent-factory/actions/runs/33278920011/job/99170605175), is also retained as a failure record: its workflow-run payload supplied source SHA [`7af39e5bc23c8fc74e1402da85981a438eb455c`](https://github.com/portpowered/you-agent-factory/commit/7af39e5bc23c8fc74e1402da85981a438eb455c), source run [33277914281](https://github.com/portpowered/you-agent-factory/actions/runs/33277914281), conclusion `cancelled`, and stopped before checkout/generation/publication. The later successful run above used the completed `a27ea...` source CI and superseded this failed attempt; neither result establishes a dual-snapshot candidate.

The newly protected pin's own push CI has now completed:

| Job | Observed result |
| --- | --- |
| [CI run 33279723467](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467) | `success`, head SHA [`995137125a6f90bec0284cbe2ea1783e70b5d063`](https://github.com/portpowered/you-agent-factory/commit/995137125a6f90bec0284cbe2ea1783e70b5d063), completed `2026-08-29T23:04:29Z` |
| Classify Verification | `success`, [job 99172715424](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467/job/99172715424) |
| Backend Unit Latency | `success`, [job 99172715384](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467/job/99172715384) |
| Backend Lint | `success`, [job 99172715412](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467/job/99172715412) |
| Verification Policy | `success`, [job 99173684923](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467/job/99173684923) |
| Post-merge regeneration | [Run 33280105358](https://github.com/portpowered/you-agent-factory/actions/runs/33280105358), `success`, [job 99173719289](https://github.com/portpowered/you-agent-factory/actions/runs/33280105358/job/99173719289); source SHA [`995137125a6f90bec0284cbe2ea1783e70b5d063`](https://github.com/portpowered/you-agent-factory/commit/995137125a6f90bec0284cbe2ea1783e70b5d063), source run [33279723467](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467). |

The post-merge regeneration log records one transient generated baseline file,
then `SHARED_BASELINE_CHANGED=false paths=(none) quiescent=true` and
`SHARED_BASELINE_RECONCILIATION action=noop publish=false`. No bot PR or
post-merge snapshot drift resulted. The live `origin/main` ref subsequently
moved to `dd963b68b4c94d16cf4317476202d9f5220a4140`; that later tree is not
substituted for the pinned evidence in this report.

## Prior PR and c11 lead trace

The required prior leads were queried by exact pull-request, check, file, and
merge identities:

| Lead | Observed result and relevance |
| --- | --- |
| [PR #2347](https://github.com/portpowered/you-agent-factory/pull/2347) | Merged as [`fee3da73388514cfb5975307d2cd1e07b345cd84`](https://github.com/portpowered/you-agent-factory/commit/fee3da73388514cfb5975307d2cd1e07b345cd84); head [`26594fb95d34476d1e5473f0fc1e201e7cc44cb8`](https://github.com/portpowered/you-agent-factory/commit/26594fb95d34476d1e5473f0fc1e201e7cc44cb8), base [`6f6ad94c433d1fbec60c3fef0fe192094beae128`](https://github.com/portpowered/you-agent-factory/commit/6f6ad94c433d1fbec60c3fef0fe192094beae128); [PR files](https://github.com/portpowered/you-agent-factory/pull/2347/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/6f6ad94c433d1fbec60c3fef0fe192094beae128...26594fb95d34476d1e5473f0fc1e201e7cc44cb8.diff); run [33141331219](https://github.com/portpowered/you-agent-factory/actions/runs/33141331219) had Backend Lint [job 98754528854](https://github.com/portpowered/you-agent-factory/actions/runs/33141331219/job/98754528854), Backend Unit Latency [job 98754529293](https://github.com/portpowered/you-agent-factory/actions/runs/33141331219/job/98754529293), Backend Functional Coverage [job 98754525706](https://github.com/portpowered/you-agent-factory/actions/runs/33141331219/job/98754525706), and Verification Policy [job 98755550143](https://github.com/portpowered/you-agent-factory/actions/runs/33141331219/job/98755550143) success. Its eight changed files were workflow/Makefile/generator sources and tests; neither comparison baseline changed. |
| [PR #2408](https://github.com/portpowered/you-agent-factory/pull/2408) | Merged as [`182ccb00da13c159eda46caee7a75c8640c97067`](https://github.com/portpowered/you-agent-factory/commit/182ccb00da13c159eda46caee7a75c8640c97067); head [`8115631237e0cb6a317113c9dee9ead9e05cee86`](https://github.com/portpowered/you-agent-factory/commit/8115631237e0cb6a317113c9dee9ead9e05cee86), base [`0d56c18b386ab77e823bd7d2da7988c2fdd636d1`](https://github.com/portpowered/you-agent-factory/commit/0d56c18b386ab77e823bd7d2da7988c2fdd636d1); [PR files](https://github.com/portpowered/you-agent-factory/pull/2408/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/0d56c18b386ab77e823bd7d2da7988c2fdd636d1...8115631237e0cb6a317113c9dee9ead9e05cee86.diff); run [33223243464](https://github.com/portpowered/you-agent-factory/actions/runs/33223243464) had Backend Lint [job 99021591536](https://github.com/portpowered/you-agent-factory/actions/runs/33223243464/job/99021591536), Backend Unit Latency [job 99021591539](https://github.com/portpowered/you-agent-factory/actions/runs/33223243464/job/99021591539), Backend Functional Coverage [job 99021719418](https://github.com/portpowered/you-agent-factory/actions/runs/33223243464/job/99021719418), and Verification Policy [job 99023240513](https://github.com/portpowered/you-agent-factory/actions/runs/33223243464/job/99023240513) success. Its three changed files were workflow/helper/test files; neither comparison baseline changed. The operator's merge note explicitly treated the post-merge witness as follow-up, not merge-precondition evidence. |
| [PR #2444](https://github.com/portpowered/you-agent-factory/pull/2444) | Merged source lead as [`0228a6d5e081ea65b03f11ee9553f636071eaa01`](https://github.com/portpowered/you-agent-factory/commit/0228a6d5e081ea65b03f11ee9553f636071eaa01); head [`542bb48d169acbd18b912de5ee12edc264b6c7c7`](https://github.com/portpowered/you-agent-factory/commit/542bb48d169acbd18b912de5ee12edc264b6c7c7), base [`5b81ba3602c2875cf72fbabad759cac1687ee771`](https://github.com/portpowered/you-agent-factory/commit/5b81ba3602c2875cf72fbabad759cac1687ee771); [PR files](https://github.com/portpowered/you-agent-factory/pull/2444/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/5b81ba3602c2875cf72fbabad759cac1687ee771...542bb48d169acbd18b912de5ee12edc264b6c7c7.diff); run [33253550758](https://github.com/portpowered/you-agent-factory/actions/runs/33253550758) relevant checks—Backend Lint [job 99105658027](https://github.com/portpowered/you-agent-factory/actions/runs/33253550758/job/99105658027), Backend Unit Latency [job 99105656477](https://github.com/portpowered/you-agent-factory/actions/runs/33253550758/job/99105656477), Backend Functional Coverage [job 99105646497](https://github.com/portpowered/you-agent-factory/actions/runs/33253550758/job/99105646497), and Verification Policy [job 99106515702](https://github.com/portpowered/you-agent-factory/actions/runs/33253550758/job/99106515702)—succeeded; five functional CLI-output test files, no comparison-baseline patch. |
| [PR #2435](https://github.com/portpowered/you-agent-factory/pull/2435) | Merged bot lead as [`3fa80f6e548a07ff7ec02a8047bffb37402a7039`](https://github.com/portpowered/you-agent-factory/commit/3fa80f6e548a07ff7ec02a8047bffb37402a7039); head [`319583426d659c5ae29ef0ab3c6ee66fa199f404`](https://github.com/portpowered/you-agent-factory/commit/319583426d659c5ae29ef0ab3c6ee66fa199f404), base [`64f6c992091a79aad6b7ebce84f9a94fdb7168e3`](https://github.com/portpowered/you-agent-factory/commit/64f6c992091a79aad6b7ebce84f9a94fdb7168e3); [PR files](https://github.com/portpowered/you-agent-factory/pull/2435/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/64f6c992091a79aad6b7ebce84f9a94fdb7168e3...319583426d659c5ae29ef0ab3c6ee66fa199f404.diff); run [33240184371](https://github.com/portpowered/you-agent-factory/actions/runs/33240184371) relevant checks—Backend Lint [job 99068071245](https://github.com/portpowered/you-agent-factory/actions/runs/33240184371/job/99068071245), Backend Unit Latency [job 99068071400](https://github.com/portpowered/you-agent-factory/actions/runs/33240184371/job/99068071400), and Verification Policy [job 99068934987](https://github.com/portpowered/you-agent-factory/actions/runs/33240184371/job/99068934987)—succeeded; one latency-budget `reference.baseCommit` patch, no deadcode-baseline patch. |
| [PR #2338](https://github.com/portpowered/you-agent-factory/pull/2338) | Closed unmerged; head [`0989ac9a9320130ed5be2472442867e372dcfb43`](https://github.com/portpowered/you-agent-factory/commit/0989ac9a9320130ed5be2472442867e372dcfb43), base [`3d9bfe1d9ac891341639fff5e73bef39b5cb3b16`](https://github.com/portpowered/you-agent-factory/commit/3d9bfe1d9ac891341639fff5e73bef39b5cb3b16); [PR files](https://github.com/portpowered/you-agent-factory/pull/2338/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/3d9bfe1d9ac891341639fff5e73bef39b5cb3b16...0989ac9a9320130ed5be2472442867e372dcfb43.diff); run [33064719861](https://github.com/portpowered/you-agent-factory/actions/runs/33064719861) had Backend Unit Latency [job 98724674884](https://github.com/portpowered/you-agent-factory/actions/runs/33064719861/job/98724674884) and Verification Policy [job 98726061660](https://github.com/portpowered/you-agent-factory/actions/runs/33064719861/job/98726061660) failures. It is negative lineage evidence, not a merge. |
| [PR #2345](https://github.com/portpowered/you-agent-factory/pull/2345) | Open unit-latency reference lead; head [`a05a800aecdea0e60d1a6398a9eeda4b55e0f0eb`](https://github.com/portpowered/you-agent-factory/commit/a05a800aecdea0e60d1a6398a9eeda4b55e0f0eb), base [`6f6ad94c433d1fbec60c3fef0fe192094beae128`](https://github.com/portpowered/you-agent-factory/commit/6f6ad94c433d1fbec60c3fef0fe192094beae128); [PR files](https://github.com/portpowered/you-agent-factory/pull/2345/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/6f6ad94c433d1fbec60c3fef0fe192094beae128...a05a800aecdea0e60d1a6398a9eeda4b55e0f0eb.diff); run [33125634420](https://github.com/portpowered/you-agent-factory/actions/runs/33125634420) observed 13.27% improvement against the 25% policy and failed Unit Latency [job 98702988665](https://github.com/portpowered/you-agent-factory/actions/runs/33125634420/job/98702988665)/Verification Policy [job 98705528185](https://github.com/portpowered/you-agent-factory/actions/runs/33125634420/job/98705528185). It is not an admitted candidate. |
| [PR #2211](https://github.com/portpowered/you-agent-factory/pull/2211) | Open source/deadcode lead; head [`ba3d766614437b88cf2fb65b7b432c5264cab85e`](https://github.com/portpowered/you-agent-factory/commit/ba3d766614437b88cf2fb65b7b432c5264cab85e), base [`5637f83a757d8727cc5acf3d7d5e8e87d25d1a10`](https://github.com/portpowered/you-agent-factory/commit/5637f83a757d8727cc5acf3d7d5e8e87d25d1a10); [PR files](https://github.com/portpowered/you-agent-factory/pull/2211/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/5637f83a757d8727cc5acf3d7d5e8e87d25d1a10...ba3d766614437b88cf2fb65b7b432c5264cab85e.diff); run [32638912321](https://github.com/portpowered/you-agent-factory/actions/runs/32638912321) had Backend Lint [job 97439735969](https://github.com/portpowered/you-agent-factory/actions/runs/32638912321/job/97439735969) success but Backend Functional Coverage [job 97439735972](https://github.com/portpowered/you-agent-factory/actions/runs/32638912321/job/97439735972) and Verification Policy [job 97442436163](https://github.com/portpowered/you-agent-factory/actions/runs/32638912321/job/97442436163) failures. It is not an admitted candidate. |

The selected bot-PR scan also inspected the following exact PR/base/head/merge
lineages. Each patch changed only the latency budget's `reference.baseCommit`;
no dual deadcode-plus-latency snapshot was observed in that scan.

| Bot PR | Base SHA | Head SHA | Merge SHA | Generated diff |
| --- | --- | --- | --- | --- |
| [#2402](https://github.com/portpowered/you-agent-factory/pull/2402) | [`7ca60f6098eca7eb87634d0277117b31900698dd`](https://github.com/portpowered/you-agent-factory/commit/7ca60f6098eca7eb87634d0277117b31900698dd) | [`4ce6add416589c99b11ef14269cb2aaf17be840d`](https://github.com/portpowered/you-agent-factory/commit/4ce6add416589c99b11ef14269cb2aaf17be840d) | [`f1bc372c1c16ef5401f05cfca1d497d817bc891a`](https://github.com/portpowered/you-agent-factory/commit/f1bc372c1c16ef5401f05cfca1d497d817bc891a) | [PR files](https://github.com/portpowered/you-agent-factory/pull/2402/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/7ca60f6098eca7eb87634d0277117b31900698dd...4ce6add416589c99b11ef14269cb2aaf17be840d.diff) |
| [#2404](https://github.com/portpowered/you-agent-factory/pull/2404) | [`fbc67210656779076210bcb066db2ecfe6067c7f`](https://github.com/portpowered/you-agent-factory/commit/fbc67210656779076210bcb066db2ecfe6067c7f) | [`4a657eccaa9a64be8298e1da60ee9e0f98856f9d`](https://github.com/portpowered/you-agent-factory/commit/4a657eccaa9a64be8298e1da60ee9e0f98856f9d) | [`72003ae42b2608a8f5f8c967b783ede8e198fd93`](https://github.com/portpowered/you-agent-factory/commit/72003ae42b2608a8f5f8c967b783ede8e198fd93) | [PR files](https://github.com/portpowered/you-agent-factory/pull/2404/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/fbc67210656779076210bcb066db2ecfe6067c7f...4a657eccaa9a64be8298e1da60ee9e0f98856f9d.diff) |
| [#2406](https://github.com/portpowered/you-agent-factory/pull/2406) | [`72003ae42b2608a8f5f8c967b783ede8e198fd93`](https://github.com/portpowered/you-agent-factory/commit/72003ae42b2608a8f5f8c967b783ede8e198fd93) | [`c9010161ac536ceff31dd90b5e965db19aa5ef34`](https://github.com/portpowered/you-agent-factory/commit/c9010161ac536ceff31dd90b5e965db19aa5ef34) | [`51733a283a1ce79a17e45bc5cd7859f0399f518b`](https://github.com/portpowered/you-agent-factory/commit/51733a283a1ce79a17e45bc5cd7859f0399f518b) | [PR files](https://github.com/portpowered/you-agent-factory/pull/2406/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/72003ae42b2608a8f5f8c967b783ede8e198fd93...c9010161ac536ceff31dd90b5e965db19aa5ef34.diff) |
| [#2407](https://github.com/portpowered/you-agent-factory/pull/2407) | [`51733a283a1ce79a17e45bc5cd7859f0399f518b`](https://github.com/portpowered/you-agent-factory/commit/51733a283a1ce79a17e45bc5cd7859f0399f518b) | [`dc584218fb7d6291f891c45f358f1f4e06b186d9`](https://github.com/portpowered/you-agent-factory/commit/dc584218fb7d6291f891c45f358f1f4e06b186d9) | [`31ba882786d5d9427b43c1c7cee7996a152a2a4d`](https://github.com/portpowered/you-agent-factory/commit/31ba882786d5d9427b43c1c7cee7996a152a2a4d) | [PR files](https://github.com/portpowered/you-agent-factory/pull/2407/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/51733a283a1ce79a17e45bc5cd7859f0399f518b...dc584218fb7d6291f891c45f358f1f4e06b186d9.diff) |
| [#2460](https://github.com/portpowered/you-agent-factory/pull/2460) | [`4b2a0582d6db24e66482c3117d378bb28be32717`](https://github.com/portpowered/you-agent-factory/commit/4b2a0582d6db24e66482c3117d378bb28be32717) | [`1f207cb5333a9dd7f2e73a0b3f6793a75f723065`](https://github.com/portpowered/you-agent-factory/commit/1f207cb5333a9dd7f2e73a0b3f6793a75f723065) | [`7af39e5bc23c8fc74e1402da85981a438eb455c5`](https://github.com/portpowered/you-agent-factory/commit/7af39e5bc23c8fc74e1402da85981a438eb455c5) | [PR files](https://github.com/portpowered/you-agent-factory/pull/2460/files), [commit-pinned raw diff](https://github.com/portpowered/you-agent-factory/compare/4b2a0582d6db24e66482c3117d378bb28be32717...1f207cb5333a9dd7f2e73a0b3f6793a75f723065.diff) |

## Local-real probes and evidence boundaries

These probes were read-only with respect to the repository branch and were
run in disposable checkouts or against downloaded hosted artifacts:

| Procedure | Observed result | Property proved / boundary |
| --- | --- | --- |
| `go test ./cmd/deadcodecheck ./cmd/unitlanebudget` in the pinned disposable checkout | Exit 0 | The two checker packages compile and their unit tests pass on the local host; not hosted Linux admission evidence. |
| `make -C <checkout> test-ci-workflows` | Exit 0; 169 tests, 168 pass, 1 skipped, 0 fail | Pinned workflow/helper policy tests pass locally; not a substitute for protected CI. |
| `make test-unit-latency-budget` with the three downloaded run files from artifact `9722470916` ([artifact page](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/artifacts/9722470916)) | Exit 0; exact inventory, 61.72% median improvement, 6.26% maximum-above-median | Replays the authoritative hosted latency evidence through the pinned checker. |
| `make deadcode` on Windows in the disposable checkout | Exit 2; generated 3,072 findings versus 3,074 baseline lines | Platform diagnostic only. The hosted Ubuntu artifact matched the baseline exactly, so this Windows result is not used to fail or pass the candidate. The earlier report revision recorded Exit 1 for this same-named probe; that discrepancy is retained as `VAL-C12-005` below. |
| `node --test scripts/ci/backend-lint-report.test.mjs scripts/ci/backend-lint-workflow.test.mjs scripts/ci/unit-latency-workflow.test.mjs scripts/verification-policy.test.mjs` in the pinned disposable checkout | Exit 0; 51 tests passed | Pinned policy, merge-result selection, no-allowance deadcode failure, required-result propagation, and latency inventory wiring behave as traced; not hosted execution of a future source head. |
| `git merge-tree --write-tree 7af39e5bc23c8fc74e1402da85981a438eb455c5 4571780148515d6cbc4f1cd9a5877f3f7f517cd2` in the pinned disposable checkout | Exit 0; result tree `c05d9943274d8789dc0791d821f2bd1b76a56311`, equal to committed merge `a27ea892f5feb5ada9578d0da8159bfe3b590107`'s tree | A real conflict-free source merge produces one merged content tree. Its base tree was `62f37dd42e1b82a4a1e4880c2915d9c841749e14` and head tree was `ea40d67e535f49ffd041fb4803ed83dece824720`; this characterizes merge-tree behavior, not a new candidate. |
| `make test-ci-workflows` in the pinned disposable checkout | Exit 0; 169 tests, 168 pass, 1 skipped, 0 fail | Pinned workflow and reconciliation policy tests pass locally; not a substitute for protected CI. |

The local environment was Git 2.44.0, Go 1.25.0, Node 22.12.0, GNU Make
4.4.1, and GitHub CLI 2.88.0. No source, baseline, workflow, branch, or
GitHub setting was changed by this story.

## Story 002: candidate audit and decision

### Authority, notation, and result

The governing plan remains absent at the pinned SHA and returns HTTP 404, as
recorded above. The `CAND-01` through `CAND-09` labels below are therefore an
explicit partition of the nine classes named by story 002, not labels recovered
from the missing plan.

Let:

- `B` be the pinned protected-main commit
  `995137125a6f90bec0284cbe2ea1783e70b5d063`, with tree
  `444f780c53deff8660ab87a1c2f8ce1fe0667dc4`.
- `H` be a source pull-request head and `M` its conflict-free GitHub
  merge-result commit.
- `D(X)` be the normalized output of pinned `cmd/deadcodecheck` for tree `X`.
- `dB` be the committed deadcode baseline in `B`, blob
  `358716aeb0095882890819e58e0b98c09a8c9993`, SHA-256
  `F31645C911B22D76E5A121E0DA0C47D5549DE16045E1D803E0003A254AFDFE13`.
- `U(X)` be the pinned unit-lane package/test inventory and budget comparison
  for tree `X`; `B` records 445 packages and 18,239 tests in
  `go-unit-lane-latency-budget.v1.json`, blob
  `695309991f927099fd24f98c1a18b2b74c273c77`.

**Infeasible result: PASS. Feasible result: NOT APPLICABLE.** No feasible
package/file/symbol/test witness is named because the pinned admission
contract rules out every permitted dual-drift class before a useful witness
could be admitted.

The contradiction is direct and does not depend on the missing plan's
definition of “useful”:

1. A deliberate dual-drift route requires `D(M) != dB` after the ordinary
   source merge, because protected regeneration uses the source CI's normalized
   deadcode artifact to update the deadcode snapshot.
2. The pinned pull-request Backend Lint job sets
   `BACKEND_LINT_TESTED_SHA=${{ github.sha }}` and checks out that SHA. For a
   pull request, `github.sha` is the merge-result identity selected by
   `scripts/ci/backend-lint-workflow.mjs`; `cmd/deadcodecheck` then compares
   `D(M)` exactly with the deadcode baseline present in `M`.
3. The source route is forbidden from carrying either generated baseline. With
   a clean current base, the baseline present in `M` is therefore `dB`. If
   `D(M) != dB`, `cmd/deadcodecheck` exits 1 and emits
   `LINT_VIOLATION_COUNT`; `deadcode` is a no-allowance target in the pinned
   `scripts/ci/backend-lint-policy.mjs`, so the target is `new failure`.
4. The pinned `Verification Policy` requires Backend Lint to be `success`.
   Its selected-lane evaluator therefore fails the pull request. If instead
   `D(M) == dB`, protected regeneration cannot change the deadcode snapshot.

Thus every allowed source tree either fails the required pre-merge deadcode
comparison or does not produce the required deadcode drift. Latency-only drift
cannot satisfy the dual requirement.

### Exhaustive safe-class audit

| Class | Exact permitted class evaluated | PR-head / merge-result comparison | Verification Policy consequence | Disposition |
| --- | --- | --- | --- | --- |
| `CAND-01` | Direct production deadcode delta | `D(M) != dB`; pinned `cmd/deadcodecheck` exits 1 with `LINT_VIOLATION_COUNT`. | Backend Lint is `new failure`; selected result is not `success`; policy fails. | Infeasible by the contradiction above. |
| `CAND-02` | Direct unit-test inventory delta | `D(M) == dB`, so deadcode does not drift. On `H`, `cmd/unitlanebudget` compares sorted package/test identities against the committed 445/18,239 inventory; a changed test identity exits 1 with an expected/actual inventory diagnostic. If the inventory remains equal, latency cannot drift from this class. | Backend Unit Latency is required on pull requests; a failed or incomplete result fails policy. | Infeasible for dual drift. |
| `CAND-03` | Direct source delta intended to change both inventories | It contains `CAND-01`'s `D(M) != dB`, so Backend Lint exits 1 before any latency result can admit it. | Backend Lint failure is sufficient; any simultaneous latency failure also remains required. | Infeasible. |
| `CAND-04` | Stale-base semantic composition with a conflict-free merge-result-only delta | Non-strict branch currency can add base changes to `M`, but Backend Lint checks `M`. A deadcode change in `M` is `CAND-01`; a test-only change visible only in `M` can at most change latency, not deadcode. A pre-existing base deadcode mismatch would contradict the clean pinned `B` and is not a candidate. | Any deadcode delta fails Backend Lint/Verification Policy; test-only merge drift cannot satisfy both snapshots. | Infeasible. |
| `CAND-05` | Enabled normal squash merge | The real conflict-free fixture below produced result tree `c05d9943274d8789dc0791d821f2bd1b76a56311`; its hosted source checks were deadcode `success`, unit latency `success`, and no dual snapshot drift. A future `D(M) != dB` remains caught by Backend Lint. | Required checks pass only for the no-drift fixture; a dual-drift variant fails Backend Lint and policy. | No safe witness. |
| `CAND-06` | Enabled normal merge-commit merge | The same conflict-free result tree is the content checked by the pull-request job; an extra commit parent does not hide a changed `D(M)`. | Same required-check consequence as `CAND-05`. | No safe witness. |
| `CAND-07` | Enabled normal rebase merge | Rebase may change commit identity, but a conflict-free final content tree still contains the source delta and is checked before merge. | Same required-check consequence as `CAND-05`. | No safe witness. |
| `CAND-08` | Ordinary platform-independent Go behavior or call-graph interaction | If the call graph changes `D(M)`, it is `CAND-01`; if it does not, the deadcode baseline cannot change. The pinned hosted Linux comparison is the authoritative platform, so the Windows diagnostic cannot create an alternate route. | No ordinary call-graph outcome escapes the required deadcode result. | Infeasible. |
| `CAND-09` | Existing real ordering witnesses from source CI through regeneration and bot merge | The observed `a27ea... -> 33279349933 -> #2462 -> B` chain changed only the latency budget's `reference.baseCommit`; the later regeneration of `B` returned `SHARED_BASELINE_CHANGED=false` and `action=noop`. It is evidence of a latency-only path, not dual drift. | Existing successful checks do not establish a dual-drift source witness. | Rejected as non-witness evidence. |

### Exact merge-tree characterization

The local-real merge procedure used an existing conflict-free source merge in
the pinned disposable checkout, without creating or pushing a new branch:
the base [`7af39e5bc23c8fc74e1402da85981a438eb455c5`](https://github.com/portpowered/you-agent-factory/commit/7af39e5bc23c8fc74e1402da85981a438eb455c5), head [`4571780148515d6cbc4f1cd9a5877f3f7f517cd2`](https://github.com/portpowered/you-agent-factory/commit/4571780148515d6cbc4f1cd9a5877f3f7f517cd2), and committed merge [`a27ea892f5feb5ada9578d0da8159bfe3b590107`](https://github.com/portpowered/you-agent-factory/commit/a27ea892f5feb5ada9578d0da8159bfe3b590107) are immutable commit objects.

```text
git merge-tree --write-tree \
  7af39e5bc23c8fc74e1402da85981a438eb455c5 \
  4571780148515d6cbc4f1cd9a5877f3f7f517cd2
# exit 0
# c05d9943274d8789dc0791d821f2bd1b76a56311

git rev-parse 7af39e5bc23c8fc74e1402da85981a438eb455c5^{tree}
# 62f37dd42e1b82a4a1e4880c2915d9c841749e14
git rev-parse 4571780148515d6cbc4f1cd9a5877f3f7f517cd2^{tree}
# ea40d67e535f49ffd041fb4803ed83dece824720
git rev-parse a27ea892f5feb5ada9578d0da8159bfe3b590107^{tree}
# c05d9943274d8789dc0791d821f2bd1b76a56311
git diff --quiet a27ea892f5feb5ada9578d0da8159bfe3b590107^{tree} \
  c05d9943274d8789dc0791d821f2bd1b76a56311
# exit 0
```

The fixture's generated source-to-bot diff is the exact [PR #2462 generated diff](https://github.com/portpowered/you-agent-factory/pull/2462/files) ([raw diff](https://github.com/portpowered/you-agent-factory/pull/2462.diff)); it changed only the latency baseline among the two
target snapshots. Its hosted source run [33278765602](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602)
recorded exact deadcode and unit-lane success. This is a tree/ordering
characterization, not synthetic proof of a future candidate. For all three
enabled conflict-free merge methods, the final content tree is what the
pull-request Backend Lint job selects; commit-parent or commit-message
differences do not make a changed deadcode report invisible.

### Operator-owned decision boundary

The smallest explicit decision is whether the “neither generated baseline is
committed in the deliberate source change” constraint remains strict. If it
remains strict, the AC1 dual-drift requirement is infeasible under the pinned
rules. If the operator instead permits a reviewed deadcode baseline update in
the source pull request, that is a new contract and must be planned and
validated separately. This lane makes neither change. No bypass, direct push,
comparison weakening, synthetic-only simulation, platform trick, no-op test,
or manual merge repair is proposed.

## Facts, inferences, and unproven edges

### Facts

- The protected-main pin, checkout cleanliness, pinned blobs, baseline hashes,
  rulesets, required contexts, and merge settings are recorded above.
- The predecessor `a27ea...` source CI produced both comparison-unit artifacts
  and passed its selected checks.
- Protected regeneration consumed that exact completed source run, generated
  only the latency-budget candidate, and opened bot PR #2462.
- Bot PR #2462 passed its required checks and merged to the current pin.
- The current pin's push CI and post-merge regeneration both completed
  successfully; the regeneration run produced no publishable candidate.
- The governing source plan is absent and returns 404 at the pin.

### Inferences, kept separate from facts

- The observed completed chain demonstrates an admitted latency-only bot
  reconciliation path; it does not demonstrate admission of a dual-snapshot
  candidate.
- The absence of a deadcode patch in #2462 is a patch fact. Story 002's
  policy-level contradiction independently proves that a source change causing
  deadcode drift cannot pass the required pre-merge comparison without a
  reviewed baseline change.
- A legacy branch-protection 404 cannot override the active ruleset responses.

### Unproven / blocked

- The exact governing AC1 source-plan rule and its definition of a useful
  protected-main candidate remains unavailable because the plan returns 404.
- The report-only PR's terminal required checks and merge, which are review-owned
  after implementation handoff.
- The review-correction Windows Go/`make deadcode` wrappers did not provide
  bounded exit statuses on the saturated host; this is retained as
  `VAL-C12-006` and does not replace the hosted evidence.

## Story and project gate status

| Gate | Status | Reason |
| --- | --- | --- |
| `GATE-PIN` | PASS | 40-character current-main pin, clean disposable checkout, immutable blobs and exact ruleset/merge evidence recorded. |
| `GATE-ADMISSION` | PASS | Backend Lint/deadcode and Backend Unit Latency contracts, normalization, artifacts, and Verification Policy propagation recorded. |
| `GATE-LINEAGE` | PASS | The predecessor source-to-regeneration-to-bot-merge chain and current-pin CI/regeneration results are bound to exact SHAs, runs, jobs, artifacts, and URLs. |
| `GATE-CANDIDATE` | PASS — INFEASIBLE RESULT | All nine story-002 classes terminate in the exact deadcode pre-merge contradiction or fail to produce dual drift; no forbidden class is counted. |
| Feasible result criterion | NOT APPLICABLE | No feasible witness exists under the pinned contract; package/file/symbol/test/owner fields are intentionally N/A. |
| Infeasible result criterion | PASS | The pinned checker source, required contexts, exact merge-result selection, direct comparison outputs, safe-class matrix, and operator decision boundary are recorded above. |
| Governing plan authority | BLOCKED / NON-DECISIVE | `docs/temp/stability-cleanup.md` is absent at the pin; the policy contradiction does not rely on reconstructing its “useful” definition. |
| `VAL-001` | PASS — clean-room replay | The story-003 validator independently replayed the pinned identities, checker behavior, merge tree, artifact checker, safe-class contradiction, and the corrected Windows diagnostic; the missing plan and corrected report-PR terminal state remain explicit unproven edges. |
| Report PR handoff | READY FOR REVIEW | Existing [PR #2469](https://github.com/portpowered/you-agent-factory/pull/2469) is the report-only delivery PR; the final report head is pushed and required-check outcomes are tracked in the PR conversation. |

## Verdict and smallest next step

**Verdict for story 001:** PASS as a factual ledger. **Verdict for story 002:**
PASS with the infeasible result applicable and the feasible result explicitly
not applicable. **Verdict for the overall lane:** STORY 003 VALIDATION
COMPLETE; the report-only handoff is the remaining implementation finish step.

The smallest safe next step is the story-003 report-only handoff: push the
validated report, open its ordinary PR, and confirm that required checks start.
No source or CI mutation is requested by this decision.

## Story 003: clean-room validation and implementation handoff

### Environment and artifact

- Commit/build identifier: pinned protected-main commit
  `995137125a6f90bec0284cbe2ea1783e70b5d063`; the validator used a fresh
  detached checkout at
  `C:\Users\andre\AppData\Local\Temp\c12-clean-room-3e2156c6c7f64aeca9f56cdd8141f7b1`.
- Environment and configuration: Windows `amd64`, Git `2.44.0`, Go
  `1.25.0`, Node `22.12.0`, GNU Make `4.4.1`, and GitHub CLI `2.88.0`.
  The checkout resolved to the pinned SHA and
  `git status --porcelain=v1` was empty after every read-only probe.
- Customer entry point: a reviewer follows this report from its immutable
  main pin through the protected GitHub rulesets, required CI comparison
  units, existing source-to-regeneration history, and the report-only PR.
- Real and substituted dependencies: GitHub REST, pull-request, check-run,
  workflow-run, artifact, and ruleset reads were remote-real and read-only;
  the disposable Git checkout and pinned Go/Node checkers were local-real.
  No source PR, baseline, workflow, ruleset, branch, setting, provider, or
  paid service was mutated. The local Windows deadcode run remains only a
  platform diagnostic.
- Cost/call budget used: `$0`; zero paid/provider calls. GitHub reads and one
  hosted artifact download were used for evidence; no remote application
  effect was invoked.

### Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| `GATE-PIN` and `GATE-LINEAGE` | PASS | Fresh checkout resolved to the pinned 40-character SHA with empty status. `gh api` independently returned tree `444f780c53deff8660ab87a1c2f8ce1fe0667dc4`, parent `a27ea892f5feb5ada9578d0da8159bfe3b590107`, all twelve recorded source/baseline blobs, the two active rulesets, repository merge settings, PR identities, run identities, jobs, artifacts, and the cited URLs. | A later main or ruleset change is outside this pinned replay and requires a new admission probe. |
| Feasible result criterion | NOT APPLICABLE | The report's infeasible result is reproduced below; no package/file/symbol/test witness is counted. | A future policy change could create a different candidate space. |
| Infeasible result criterion | PASS | Pinned policy tests passed 51/51. The exact hosted latency artifact from [run 33278765602](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602), [artifact 9722470916](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/artifacts/9722470916), passed the pinned budget checker with three samples, 445 packages, 18,239 tests, 61.72% median improvement, and 6.26% maximum-above-median. The safe-class matrix and the required merge-result deadcode comparison still yield the same contradiction. | This lane does not run a future source PR; the contradiction is the authorized proof for the pinned policy. |
| Exactly one result applies | PASS | The report explicitly marks `Infeasible result: PASS` and `Feasible result: NOT APPLICABLE`; no forbidden baseline or bypass route is included. | The operator-owned decision boundary remains whether the no-baseline-commit constraint changes in a new lane. |
| Facts, inference, and unproven edges | PASS | The report keeps immutable observations, policy deductions, missing-plan authority, platform diagnostics, and review-owned delivery edges in separate sections. The validator reproduced the cited objects without converting missing evidence into success. | The governing `docs/temp/stability-cleanup.md` remains absent at the pin and returns 404. |
| No repository/GitHub mutation outside the owned report and ordinary PR | PASS | The clean-room checkout stayed clean; all remote calls were reads; no source, generated baseline, workflow, ruleset, setting, branch, or experimental PR was changed. Final three-dot scope proof is required immediately before delivery. | Review may merge the ordinary report PR after implementation handoff. |
| No weakened comparison, generated baseline commit, bypass, synthetic-only simulation, platform trick, or no-op test | PASS | Pinned unit tests include deadcode drift as a blocking no-allowance result and required-result propagation. The replay used the real hosted artifact and real merge tree; the Windows deadcode mismatch was retained as a non-authoritative diagnostic. | Hosted execution of an unimplemented future source witness is intentionally not proved. |
| No local timing threshold | PASS | The validator replayed the existing hosted artifact through `make test-unit-latency-budget`; it introduced no wall-clock threshold or local performance claim. | Future source performance remains hosted package/PR CI evidence. |
| `VAL-001` clean-room validator | PASS | Every project criterion has a PASS, NOT APPLICABLE, or BLOCKED/review-owned row, with exact evidence and an unproven edge. The shallow-ancestry merge-tree failure, absent governing plan, and corrected Exit 1/Exit 2 discrepancy are recorded as findings with no silent production repair. | The corrected report head's terminal CI and merge are not implementation-stage evidence. |
| `GATE-REPORT-CI` implementation handoff | READY FOR HANDOFF | Existing [PR #2469](https://github.com/portpowered/you-agent-factory/pull/2469) is open. Its prior head [`d0c37a0a534a70637a664b7063aeaa0457307a6a`](https://github.com/portpowered/you-agent-factory/commit/d0c37a0a534a70637a664b7063aeaa0457307a6a) received terminal [Backend Lint job 99186042768](https://github.com/portpowered/you-agent-factory/actions/runs/33284808735/job/99186042768) and [Verification Policy job 99187187835](https://github.com/portpowered/you-agent-factory/actions/runs/33284808735/job/99187187835); the corrected head requires a new CI run. | Implementation stops after the corrected final head is pushed and required checks start; terminal checks and merge remain review-owned. |

### Customer journey

1. The validator created a fresh shallow checkout, fetched and detached at
   `995137125a6f90bec0284cbe2ea1783e70b5d063`, confirmed the exact commit tree,
   all recorded blobs, empty status, and absent governing plan. It then
   deepened only that disposable checkout so the cited historical merge tips
   shared ancestry.
2. `go test ./cmd/deadcodecheck ./cmd/unitlanebudget` exited `0`. The targeted
   policy/checker command
   `node --test scripts/ci/backend-lint-report.test.mjs
   scripts/ci/backend-lint-workflow.test.mjs
   scripts/ci/unit-latency-workflow.test.mjs scripts/verification-policy.test.mjs`
   exited `0` with 51 tests passed. `make test-ci-workflows` exited `0` with
   169 tests, 168 passed, 1 skipped, and 0 failed.
3. `gh run download 33278765602 --repo portpowered/you-agent-factory
   --name backend-unit-latency-evidence --dir <fresh-artifact-directory>`
   ([artifact 9722470916](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/artifacts/9722470916))
   exited `0`. Running `make test-unit-latency-budget` with the three
   downloaded `run-1.v2.json`, `run-2.v2.json`, and `run-3.v2.json` files
   exited `0` and emitted:

   ```text
   Samples: [97.471,91.656,91.728]
   Median wall time: 91.728s
   Reference median wall time: 239.612s
   Median improvement: 61.72%
   Maximum run above median: 6.26%
   Inventory: 445 packages, 18239 tests
   Cache: 0 cached, 0 unknown
   Result: pass
   ```

4. The first clean-room `git merge-tree --write-tree` attempt exited `128`
   with `fatal: refusing to merge unrelated histories` because the shallow
   checkout had the tips but not their common ancestry. After
   `git fetch --unshallow origin` in that disposable checkout, the same
   read-only procedure returned exit `0`, merge base
   `fea2e30a499384182d2fabe7038767e3c2f9c5e5`, and result tree
   `c05d9943274d8789dc0791d821f2bd1b76a56311`. The result matched
   `a27ea892f5feb5ada9578d0da8159bfe3b590107^{tree}` exactly; status remained
   empty.
5. `make deadcode` in the Windows disposable checkout exited `2` with
   `baseline findings: 3074, current findings: 3072` and
   `LINT_VIOLATION_COUNT: 3072`. This is the same documented host diagnostic,
   not protected Ubuntu evidence and not a candidate decision. No local
   output was promoted to a baseline.
6. Independent `gh api` reads returned the pinned commit, active
   `main-protect`/`must-pass-pr` rulesets, exact merge settings, the PR
   `#2462` source-to-bot identities, PR/check/run/artifact identities for the
   predecessor chain, and the current-pin CI/regeneration identities already
   recorded in the ledger. The report's direct links identify those immutable
   objects; the live `main` move is not substituted for the pin.
7. The replayed contradiction is therefore unchanged: any ordinary source
   merge with `D(M) != dB` is checked by required Backend Lint at merge-result
   SHA `M` and fails the no-allowance deadcode policy, while `D(M) == dB`
   cannot produce deadcode snapshot drift. Direct test-inventory, stale-base,
   merge-method, call-graph, and existing-ordering classes cannot escape that
   result.

8. Review-correction replay: a fresh disposable checkout at
   `C:\Users\andre\AppData\Local\Temp\c12-review-clean-room-20260830` was
   detached at `995137125a6f90bec0284cbe2ea1783e70b5d063`; its initial
   `git status --porcelain=v1` was empty. The independent Node policy command
   exited `0` with 51/51 tests passed, and `make test-ci-workflows` exited `0`
   with 169 tests, 168 passed, 1 skipped, and 0 failed. The real
   `gh run download 33278765602 --repo portpowered/you-agent-factory
   --name backend-unit-latency-evidence --dir .artifacts/unit-latency` command
   exited `0` and produced the three version-2 samples plus their status,
   stdout, and stderr files.
9. The downloaded samples were checked by a binary compiled from the pinned
   `cmd/unitlanebudget` source. Its direct invocation exited `0` and emitted
   the recorded 97.471/91.656/91.728-second samples, 61.72% improvement,
   6.26% maximum-above-median, 445-package/18,239-test inventory, zero cached
   or unknown packages, and `Result: pass`. The same pinned checkout's
   `git fetch --unshallow origin` exited `0`; `git merge-tree --write-tree`
   exited `0` with result tree `c05d9943274d8789dc0791d821f2bd1b76a56311`,
   and the result-versus-committed-merge tree comparison exited `0`.
10. The Go `go test ./cmd/deadcodecheck ./cmd/unitlanebudget` and `make
    test-unit-latency-budget` wrappers emitted their expected passing output
    but did not return within the bounded host window; the direct compiled
    latency checker above supplied the observed exit `0`. The exact Windows
    `make deadcode` replay was separately bounded at 180 seconds and timed
    out before producing a diagnostic. These are recorded as environment
    boundaries, not converted into successful exit claims; the prior complete
    clean-room `make deadcode` observation remains canonical Exit `2`.

### Cross-task integration and usability

- Documentation discoverability: the factual ledger, candidate matrix, and
  clean-room validation are co-located at
  `docs/temp/stability-cleanup/validation/c12-deliberate-witness-admission.md`;
  the report is the sole intended delivery artifact.
- Permission and error behavior: read-only GitHub queries succeeded with the
  authenticated CLI; the legacy branch-protection endpoint returned the
  recorded 404 and was not treated as authority. The shallow ancestry error
  was surfaced, bounded to the disposable checkout, and corrected only by
  fetching ancestry needed to reproduce the historical merge.
- Persistence/reload behavior: immutable Git objects, hosted artifact
  contents, and exact PR/run identities were re-read independently; no
  mutable local report or generated snapshot was used as a substitute for
  those objects.
- Accessibility/keyboard/responsive behavior: not applicable; this story
  changes no UI or browser surface.
- Operational signals: every decisive command had an explicit exit status;
  remote objects were bound to SHA, ID, and URL; missing plan authority,
  shallow ancestry, Windows checker variance, Go-wrapper timeout, and
  review-owned terminal CI remain visible rather than being normalized away.

### Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| `VAL-C12-001` | Info | Run the initial shallow-checkout merge-tree command with the cited base and head tips. | A clean-room replay should have the common ancestry needed for the exact historical merge. | Git exited `128` with `fatal: refusing to merge unrelated histories`; `git fetch --unshallow origin` in the disposable checkout restored ancestry, after which the exact command exited `0` and matched the recorded result tree. | Clean-room command output and result-tree comparison above. |
| `VAL-C12-002` | Blocked / non-decisive authority | Inspect `docs/temp/stability-cleanup.md` at the pinned SHA and query its immutable contents URL. | The PRD-named governing plan would be available to validate the source-plan wording. | `git ls-tree` returned no path and GitHub returned HTTP `404 Not Found`. The policy contradiction does not depend on reconstructing the missing plan, so no source or decision was silently repaired. | Pinned checkout inspection and `gh api repos/portpowered/you-agent-factory/contents/docs/temp/stability-cleanup.md?ref=995137125a6f90bec0284cbe2ea1783e70b5d063`. |
| `VAL-C12-003` | Info | Run `make deadcode` on the Windows disposable checkout. | Local diagnostics must not be confused with hosted Ubuntu admission. | The complete prior clean-room observation was Exit `2`; current 3,072 findings versus 3,074 committed findings. The review-correction replay of the same wrapper timed out at 180 seconds before output, so no new exit status is inferred. The result is retained only as a platform boundary. | Exact command output above, bounded timeout in customer-journey step 10, and [hosted artifact 9722393303](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602/artifacts/9722393303) / [run 33278765602](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602) recorded in the ledger. |
| `VAL-C12-004` | Review-owned | Complete the clean-room replay before report delivery. | Review-owned GATE-REPORT-CI requires terminal Backend Lint and Verification Policy results recorded in a PR comment after handoff. | Existing [PR #2469](https://github.com/portpowered/you-agent-factory/pull/2469) had terminal checks on prior head [`d0c37a0a534a70637a664b7063aeaa0457307a6a`](https://github.com/portpowered/you-agent-factory/commit/d0c37a0a534a70637a664b7063aeaa0457307a6a); the corrected head must receive a new run, after which implementation stops once required checks start. | Prior [Backend Lint job 99186042768](https://github.com/portpowered/you-agent-factory/actions/runs/33284808735/job/99186042768), [Verification Policy job 99187187835](https://github.com/portpowered/you-agent-factory/actions/runs/33284808735/job/99187187835), and implementation-stage boundary above. |
| `VAL-C12-005` | Info / corrected evidence | Compare the prior report revision's local-real probe with the final clean-room replay of the same-named `make deadcode` command. | A single command/context must have one recorded exit status, or distinct wrappers and contexts must be named. | The prior report revision recorded Exit `1`, while the final clean-room replay recorded Exit `2`; no distinct wrapper/context was recorded. The prior Exit `1` entry was stale/underspecified, so the current table and customer journey canonicalize the replayed Exit `2` and retain this discrepancy rather than merging the observations silently. | [Prior report line 208 at head d0c37a0a534a70637a664b7063aeaa0457307a6a](https://github.com/portpowered/you-agent-factory/blob/d0c37a0a534a70637a664b7063aeaa0457307a6a/docs/temp/stability-cleanup/validation/c12-deliberate-witness-admission.md#L208) and the final clean-room output in customer-journey step 5 above. |
| `VAL-C12-006` | Blocked / environment | Rerun the Go wrappers and the exact Windows `make deadcode` command in the review-correction disposable checkout. | Each bounded command should return an observable exit status before the loopback claims completion. | The Go wrappers emitted expected passing output but did not return in the host window; the bounded `make deadcode` wrapper timed out at 180 seconds before emitting a diagnostic. The direct compiled latency checker, Node policy tests, workflow tests, artifact download, merge-tree replay, and tree comparison all returned their recorded statuses, so no wrapper timeout is promoted to an admission result. | Review-correction replay steps 8–10 above; status is retained as BLOCKED for this environment edge. |

### Verdict

PASS for story 003's clean-room validation and implementation handoff
readiness. The infeasible decision reproduces, the sole-file report is ready
for delivery, and no silent repair or forbidden mutation occurred. The absent
governing plan is a non-decisive authority gap; report-PR terminal checks and
merge are review-owned unproven edges, not claims made by this replay.

### Delta-plan request [Required for the BLOCKED finding]

- Affected behavior and criterion: governing-plan provenance for
  `GATE-CANDIDATE`/`VAL-001`, and any future admission probe that might rely on
  the exact source-plan definition of “useful.”
- Root-cause evidence or remaining uncertainty: the PRD-named
  `docs/temp/stability-cleanup.md` path is absent from the pinned tree and its
  immutable GitHub contents request returns 404. The current infeasibility
  proof is independently grounded in the pinned checker and required policy,
  but the plan wording cannot be validated.
- Smallest recommended correction/prerequisite: the operator should restore
  or explicitly repin the governing plan in a new admission probe, then rerun
  only the plan-dependent candidate wording checks. Do not reconstruct the
  plan from comments, weaken the comparison, or mutate this lane's source.
- Dependencies and retest scope: a new plan pin would require a fresh
  `GATE-PIN`/`GATE-LINEAGE` identity check and a focused review of the
  `GATE-CANDIDATE` wording; the current checker contradiction, clean-room
  replay, and report-only handoff evidence remain reusable only while their
  pinned identities match.

### Delta-plan request: `VAL-C12-005` corrected evidence

- Affected behavior and criterion: exact command evidence for `VAL-001` and
  the report's separation of authoritative hosted results from local platform
  diagnostics.
- Root-cause evidence or remaining uncertainty: the prior report revision at
  [`d0c37a0a534a70637a664b7063aeaa0457307a6a`](https://github.com/portpowered/you-agent-factory/commit/d0c37a0a534a70637a664b7063aeaa0457307a6a) recorded Exit `1` for
  `make deadcode`, while the later clean-room replay recorded Exit `2`; the
  earlier entry did not preserve enough wrapper/context detail to establish a
  distinct invocation.
- Smallest recommended correction/prerequisite: retain Exit `2` as the
  canonical final replay result, keep the discrepancy finding, and require
  future local probes to record the shell, wrapper, checkout, and raw output
  with the exit status. If the historical Exit `1` provenance becomes
  decision-relevant, rerun that exact invocation in a fresh disposable checkout
  rather than inferring its cause.
- Dependencies and retest scope: rerun only the clean-room command ledger and
  `VAL-001` evidence review when that provenance is needed; the hosted
  deadcode artifact and pinned infeasibility contradiction are unchanged.

### Delta-plan request: `VAL-C12-006` environment boundary

- Affected behavior and criterion: bounded local command-exit evidence for
  `VAL-001`; this does not alter the remote-real admission decision.
- Root-cause evidence or remaining uncertainty: the Windows Go wrappers did
  not return after emitting their final output on the saturated host, and the
  exact `make deadcode` wrapper timed out after 180 seconds without output.
  The direct compiled latency checker returned exit `0`; the prior complete
  clean-room deadcode observation returned exit `2`.
- Smallest recommended correction/prerequisite: retain the blocked wrapper
  status and use direct compiled checker execution or a clean host when a
  wrapper exit status is specifically required. Do not infer a deadcode result
  from the timeout or change the hosted admission conclusion.
- Dependencies and retest scope: rerun only the affected Windows command
  ledger if a clean host becomes available; Node policy, artifact, merge-tree,
  and pinned contradiction evidence remain independently valid.
