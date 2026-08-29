# Stability cleanup C12: deliberate witness admission

Status: **BLOCKED for the lane; factual ledger complete for story 001**

This is the story-001 evidence ledger. It pins the protected-main admission
surface and records the two comparison units and their observed lineage. It
does not select a candidate, change a baseline, alter a workflow, or create an
experimental branch or pull request. Candidate classification belongs to story
002; clean-room validation and the report pull request belong to story 003.

## Observation boundary

The live `origin/main` pin was observed at `2026-08-29T22:54:50.3876276Z`:

```text
995137125a6f90bec0284cbe2ea1783e70b5d063
```

The pinned commit is a squash merge of bot PR [#2462](https://github.com/portpowered/you-agent-factory/pull/2462), with parent
`a27ea892f5feb5ada9578d0da8159bfe3b590107`. Evidence below was collected
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
| `.github/workflows/ci.yml` | `241d419b911556fb6212e68d7ef0844ae2f12287` |
| `.github/workflows/regenerate-shared-ci-baselines.yml` | `ac9dafcbe4af0a93718674f161751f1b792af090` |
| `scripts/ci/backend-lint-policy.mjs` | `a3ada462aa63638416f633845e335d70aa912e8c` |
| `scripts/ci/backend-lint-report.mjs` | `014ee663f030f8ffa564e93aa5c4bee71af171d2` |
| `scripts/ci/shared-baseline-regeneration-workflow.mjs` | `80c768e61de8d4a6e428ac13d3485baeb1e33486` |
| `scripts/verification-policy.mjs` | `ab127508010ac583b98e9b0c943f50bb6a86bb00` |
| `cmd/deadcodecheck/main.go` | `76fec8f267dea6649c845909c38a5e389ddf35e8` |
| `cmd/unitlanebudget/main.go` | `ffa2267bf8d10ba162207f8091450bb0775f17ff` |
| `cmd/unitlanebudget/budget.go` | `5d82792f8136239343c814d1b2d052ea5fe2cce4` |
| `Makefile` | `6135fad496105721d86e7f0a1669622e691b72e7` |
| `docs/internal/baselines/deadcode-baseline.txt` | `358716aeb0095882890819e58e0b98c09a8c9993` |
| `docs/internal/baselines/go-unit-lane-latency-budget.v1.json` | `695309991f927099fd24f98c1a18b2b74c273c77` |

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
| `main-protect` (`15809936`) | Active on the default branch; deletion and non-fast-forward rules; no bypass actors; current user cannot bypass. |
| `must-pass-pr` (`15943501`) | Active on the default branch; deletion, non-fast-forward, and required-status-check rules; required contexts are exactly `Verification Policy` and `Backend Lint`; strict required-status policy is `false`; no bypass actors; current user cannot bypass. |

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
| Source push CI | [Run 33278765602](https://github.com/portpowered/you-agent-factory/actions/runs/33278765602), SHA `a27ea892f5feb5ada9578d0da8159bfe3b590107`, completed `success`. Classify Verification, Backend Lint, Backend Unit Latency, and Verification Policy all succeeded. |
| Deadcode witness | Artifact `backend-deadcode-evidence`, ID `9722393303`; SHA-256 exactly matched the committed 3,074-line baseline. |
| Latency witness | Artifact `backend-unit-latency-evidence`, ID `9722470916`; three complete files had wall times 97.471, 91.656, and 91.728 seconds, exact identity/inventory, median improvement 61.72%, and maximum run above median 6.26%. |
| Protected regeneration | [Run 33279349933](https://github.com/portpowered/you-agent-factory/actions/runs/33279349933), completed `success`. Its log records source SHA `a27ea892f5feb5ada9578d0da8159bfe3b590107`, source run `33278765602`, exact-source checkout, generated path `docs/internal/baselines/go-unit-lane-latency-budget.v1.json`, and `SHARED_BASELINE_RECONCILIATION action=merge-requested publish=true`. The helper reported `quiescent=false` because the source revision also contained three functional-worker test paths; this is recorded, not normalized away. |
| Bot PR checks | [PR #2462](https://github.com/portpowered/you-agent-factory/pull/2462), base `a27ea892f5feb5ada9578d0da8159bfe3b590107`, head `2054022a7df746e271f7da597653844b7a801cdc`. Its exact patch was one line added/removed in the latency budget's `reference.baseCommit`; no deadcode-baseline change. Classify, Workflow Lint, Backend Unit Latency, Backend Lint, and Verification Policy all completed successfully. |
| Bot merge | PR #2462 merged at `2026-08-29T22:54:47Z` as `995137125a6f90bec0284cbe2ea1783e70b5d063`. |

The preceding failed regeneration [run 33278920011](https://github.com/portpowered/you-agent-factory/actions/runs/33278920011) is also retained as a failure record: its workflow-run payload supplied source SHA `7af39e5bc23c8fc74e1402da85981a438eb455c5`, run `33277914281`, conclusion `cancelled`, and stopped before checkout/generation/publication. The later successful run above used the completed `a27ea...` source CI and superseded this failed attempt; neither result establishes a dual-snapshot candidate.

At the final observation boundary, the newly protected pin's own push CI was
still running:

| Job | Result at `2026-08-29T22:55:26Z` |
| --- | --- |
| [CI run 33279723467](https://github.com/portpowered/you-agent-factory/actions/runs/33279723467) | `in_progress`, head SHA `995137125a6f90bec0284cbe2ea1783e70b5d063` |
| Classify Verification | `success`, job `99172715424` |
| Backend Unit Latency | `in_progress`, job `99172715384` |
| Backend Lint | `in_progress`, job `99172715412` |
| Post-merge regeneration | No newer regeneration run was present at the observation boundary; the latest was successful run `33279349933` for parent `a27ea...`. |

Therefore the post-merge `9951371...` artifacts, verification-policy result,
and a subsequent regeneration decision are **unproven**, rather than failed.

## Prior PR and c11 lead trace

The required prior leads were queried by exact pull-request, check, file, and
merge identities:

| Lead | Observed result and relevance |
| --- | --- |
| [PR #2347](https://github.com/portpowered/you-agent-factory/pull/2347) | Merged as `fee3da73388514cfb5975307d2cd1e07b345cd84`; head `26594fb95d34476d1e5473f0fc1e201e7cc44cb8`, base `6f6ad94c433d1fbec60c3fef0fe192094beae128`; run [33141331219](https://github.com/portpowered/you-agent-factory/actions/runs/33141331219) had Backend Lint, Backend Unit Latency, Backend Functional Coverage, and Verification Policy success. Its eight changed files were workflow/Makefile/generator sources and tests; neither comparison baseline changed. |
| [PR #2408](https://github.com/portpowered/you-agent-factory/pull/2408) | Merged as `182ccb00da13c159eda46caee7a75c8640c97067`; head `8115631237e0cb6a317113c9dee9ead9e05cee86`, base `0d56c18b386ab77e823bd7d2da7988c2fdd636d1`; run [33223243464](https://github.com/portpowered/you-agent-factory/actions/runs/33223243464) had the relevant checks success. Its three changed files were workflow/helper/test files; neither comparison baseline changed. The operator's merge note explicitly treated the post-merge witness as follow-up, not merge-precondition evidence. |
| [PR #2444](https://github.com/portpowered/you-agent-factory/pull/2444) | Merged source lead as `0228a6d5e081ea65b03f11ee9553f636071eaa01`; head `542bb48d169acbd18b912de5ee12edc264b6c7c7`, base `5b81ba3602c2875cf72fbabad759cac1687ee771`; run [33253550758](https://github.com/portpowered/you-agent-factory/actions/runs/33253550758) relevant checks succeeded; five functional CLI-output test files, no comparison-baseline patch. |
| [PR #2435](https://github.com/portpowered/you-agent-factory/pull/2435) | Merged bot lead as `3fa80f6e548a07ff7ec02a8047bffb37402a7039`; head `319583426d659c5ae29ef0ab3c6ee66fa199f404`, base `64f6c992091a79aad6b7ebce84f9a94fdb7168e3`; run [33240184371](https://github.com/portpowered/you-agent-factory/actions/runs/33240184371) relevant checks succeeded; one latency-budget `reference.baseCommit` patch, no deadcode-baseline patch. |
| [PR #2338](https://github.com/portpowered/you-agent-factory/pull/2338) | Closed unmerged; head `0989ac9a9320130ed5be2472442867e372dcfb43`, base `3d9bfe1d9ac891341639fff5e73bef39b5cb3b16`; run [33064719861](https://github.com/portpowered/you-agent-factory/actions/runs/33064719861) had Backend Unit Latency and Verification Policy failures. It is negative lineage evidence, not a merge. |
| [PR #2345](https://github.com/portpowered/you-agent-factory/pull/2345) | Open unit-latency reference lead; head `a05a800aecdea0e60d1a6398a9eeda4b55e0f0eb`, base `6f6ad94c433d1fbec60c3fef0fe192094beae128`; run [33125634420](https://github.com/portpowered/you-agent-factory/actions/runs/33125634420) observed 13.27% improvement against the 25% policy and failed Unit Latency/Verification Policy. It is not an admitted candidate. |
| [PR #2211](https://github.com/portpowered/you-agent-factory/pull/2211) | Open source/deadcode lead; head `ba3d766614437b88cf2fb65b7b432c5264cab85e`, base `5637f83a757d8727cc5acf3d7d5e8e87d25d1a10`; run [32638912321](https://github.com/portpowered/you-agent-factory/actions/runs/32638912321) had Backend Lint success but Backend Functional Coverage and Verification Policy failures. It is not an admitted candidate. |

The selected bot-PR scan also inspected the following exact PR/base/head/merge
lineages. Each patch changed only the latency budget's `reference.baseCommit`;
no dual deadcode-plus-latency snapshot was observed in that scan.

| Bot PR | Base SHA | Head SHA | Merge SHA |
| --- | --- | --- | --- |
| [#2402](https://github.com/portpowered/you-agent-factory/pull/2402) | `7ca60f6098eca7eb87634d0277117b31900698dd` | `4ce6add416589c99b11ef14269cb2aaf17be840d` | `f1bc372c1c16ef5401f05cfca1d497d817bc891a` |
| [#2404](https://github.com/portpowered/you-agent-factory/pull/2404) | `fbc67210656779076210bcb066db2ecfe6067c7f` | `4a657eccaa9a64be8298e1da60ee9e0f98856f9d` | `72003ae42b2608a8f5f8c967b783ede8e198fd93` |
| [#2406](https://github.com/portpowered/you-agent-factory/pull/2406) | `72003ae42b2608a8f5f8c967b783ede8e198fd93` | `c9010161ac536ceff31dd90b5e965db19aa5ef34` | `51733a283a1ce79a17e45bc5cd7859f0399f518b` |
| [#2407](https://github.com/portpowered/you-agent-factory/pull/2407) | `51733a283a1ce79a17e45bc5cd7859f0399f518b` | `dc584218fb7d6291f891c45f358f1f4e06b186d9` | `31ba882786d5d9427b43c1c7cee7996a152a2a4d` |
| [#2460](https://github.com/portpowered/you-agent-factory/pull/2460) | `4b2a0582d6db24e66482c3117d378bb28be32717` | `1f207cb5333a9dd7f2e73a0b3f6793a75f723065` | `7af39e5bc23c8fc74e1402da85981a438eb455c5` |

## Local-real probes and evidence boundaries

These probes were read-only with respect to the repository branch and were
run in disposable checkouts or against downloaded hosted artifacts:

| Procedure | Observed result | Property proved / boundary |
| --- | --- | --- |
| `go test ./cmd/deadcodecheck ./cmd/unitlanebudget` in the pinned disposable checkout | Exit 0 | The two checker packages compile and their unit tests pass on the local host; not hosted Linux admission evidence. |
| `make -C <checkout> test-ci-workflows` | Exit 0; 169 tests, 168 pass, 1 skipped, 0 fail | Pinned workflow/helper policy tests pass locally; not a substitute for protected CI. |
| `make test-unit-latency-budget` with the three downloaded run files from artifact `9722470916` | Exit 0; exact inventory, 61.72% median improvement, 6.26% maximum-above-median | Replays the authoritative hosted latency evidence through the pinned checker. |
| `make deadcode` on Windows in the disposable checkout | Exit 1; generated 3,072 findings versus 3,074 baseline lines | Platform diagnostic only. The hosted Ubuntu artifact matched the baseline exactly, so this Windows result is not used to fail or pass the candidate. |

The local environment was Git 2.44.0, Go 1.25.0, Node 22.12.0, GNU Make
4.4.1, and GitHub CLI 2.88.0. No source, baseline, workflow, branch, or
GitHub setting was changed by this story.

## Facts, inferences, and unproven edges

### Facts

- The protected-main pin, checkout cleanliness, pinned blobs, baseline hashes,
  rulesets, required contexts, and merge settings are recorded above.
- The predecessor `a27ea...` source CI produced both comparison-unit artifacts
  and passed its selected checks.
- Protected regeneration consumed that exact completed source run, generated
  only the latency-budget candidate, and opened bot PR #2462.
- Bot PR #2462 passed its required checks and merged to the current pin.
- The current pin's first push CI was still in progress at the observation
  boundary.
- The governing source plan is absent and returns 404 at the pin.

### Inferences, kept separate from facts

- The observed completed chain demonstrates an admitted latency-only bot
  reconciliation path; it does not demonstrate admission of a dual-snapshot
  candidate.
- The absence of a deadcode patch in #2462 is a patch fact; deciding whether
  that makes the candidate feasible or infeasible requires the missing plan and
  story-002 rule.
- A legacy branch-protection 404 cannot override the active ruleset responses.

### Unproven / blocked

- The exact governing AC1 source-plan rule and its definition of a useful
  protected-main candidate.
- A single candidate satisfying both comparison units with the required
  baseline-change shape.
- Terminal CI artifacts and Verification Policy for current pin `9951371...`.
- A post-merge regeneration run for current pin `9951371...`.
- The clean-room validator result and this report's own protected PR checks.

## Story and project gate status

| Gate | Status | Reason |
| --- | --- | --- |
| `GATE-PIN` | PASS | 40-character current-main pin, clean disposable checkout, immutable blobs and exact ruleset/merge evidence recorded. |
| `GATE-ADMISSION` | PASS | Backend Lint/deadcode and Backend Unit Latency contracts, normalization, artifacts, and Verification Policy propagation recorded. |
| `GATE-LINEAGE` | BLOCKED | The predecessor source-to-regeneration-to-bot-merge chain is factual, but current-pin CI/regeneration is still pending and the source plan is absent. |
| `GATE-CANDIDATE` | DEFERRED/BLOCKED | Candidate selection is story 002 and cannot be inferred from the latency-only bot patch. |
| `VAL-001` | DEFERRED/BLOCKED | Clean-room validator is story 003. |
| Report PR checks | DEFERRED/BLOCKED | No report PR is authorized until the remaining stories complete. |

## Verdict and smallest next step

**Verdict for story 001:** PASS as a factual ledger, with the above blocked
edges explicitly retained. **Verdict for the overall lane:** BLOCKED; no
candidate or final validation-loopback conclusion is made.

The smallest safe next step is to obtain or operator-amend the governing plan
source, then let story 002 assess one exact candidate against it and the current
protected-main lineage. Story 003 must independently run the clean-room
validator, create the report-only PR, and start its required checks. No source
or CI mutation is requested by this ledger.
