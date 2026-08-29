# Validation report: stability-cleanup-c11-functional-self-heal-reprobe

## Environment and artifact

- Commit/build identifier: `0228a6d5e081ea65b03f11ee9553f636071eaa01`, the
  immutable `origin/main` pin observed at the start and final remote-main
  observation.
- Fresh checkout: a disposable clone was created with
  `git clone --depth 1 --no-tags --branch main
  https://github.com/portpowered/you-agent-factory.git` and then detached at
  `origin/main`. `git status --porcelain=v1` was empty. The checkout path was
  `C:\Users\andre\AppData\Local\Temp\stability-cleanup-c11-functional-self-heal-reprobe-4184e58c287d4b6a92cf66c7f42bfe19`.
- Environment: Git `2.44.0.windows.1`, Go `go1.25.0 windows/amd64`, Node
  `v22.12.0`, GitHub CLI `2.88.0`, Windows `10.0.26200.0`.
- Customer entry point: a protected `main` push runs `CI`, followed by
  `Regenerate Shared CI Baselines`; the latter is the self-heal entry point.
  The active `must-pass-pr` ruleset requires `Verification Policy` and
  `Backend Lint`.
- Real and substituted dependencies: GitHub PR/run/check/ruleset metadata,
  hosted Ubuntu 24.04 CI, and the hosted dead-code artifact were real
  dependencies. The local Windows `make deadcode` result was retained only as
  a platform diagnostic; hosted package-level evidence is authoritative. No
  paid provider, worker, or GitHub repair/mutation was used during validation.
- Cost/call budget: one temporary hosted artifact download; no workflow rerun,
  no check rerun, no paid call, and no source/workflow/baseline mutation.
- Source-plan authority: `docs/temp/stability-cleanup.md` was absent from the
  checkout, `origin/main`, and the repository contents API (404). The report
  therefore does not invent the missing deliberate lineage; this absence is a
  blocker for the historical dual-snapshot claim.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| PC-01 — map every assigned criterion | PASS | This report maps PC-01 through PC-10 and DELIVERY, with commands, immutable identities, evidence, and unproven edges. | None in the report structure. |
| PC-02 — deliberate two-snapshot bot delivery | BLOCKED | `gh pr list --author app/you-baseline-bot --state closed` found bot PRs `#2370, #2381, #2384, #2388, #2389, #2402, #2404, #2406, #2407, #2410, #2411, #2413, #2414, #2417, #2420, #2421, #2425, #2428, #2430, #2432, #2433, #2435`; their diffs changed the unit-latency baseline (and, for `#2410`, a CLI fixture), but none changed `docs/internal/baselines/deadcode-baseline.txt`. PR `#2347` changed the delivery mechanism and no snapshot files. | A unique source merge and one bot PR changing both named snapshots cannot be identified from the available public evidence or the missing source plan. |
| PC-03 — later real main self-heal | FAIL | Human PR `#2444` merged the real nonsynthetic test change at `542bb48d169acbd18b912de5ee12edc264b6c7c7`; its merge is pinned main `0228a6d5...`. CI run [33255476187](https://github.com/portpowered/you-agent-factory/actions/runs/33255476187) completed successfully, but the linked regeneration run [33256000211](https://github.com/portpowered/you-agent-factory/actions/runs/33256000211), source SHA `0228a6d5...`, failed in `Validate generated working-tree scope` with `shared baseline workflow: unable to execute git: spawnSync git ENOBUFS`; publication was skipped. | No successful later real self-heal through bot publication. |
| PC-04 — bot-only quiescence | FAIL | Bot PR `#2435` changed only `docs/internal/baselines/go-unit-lane-latency-budget.v1.json`; its required checks were green in CI run `33240184371`, auto-merge was enabled by `app/you-baseline-bot`, and it merged by that bot as `3fa80f6e...`. Its successor regeneration run [33241025528](https://github.com/portpowered/you-agent-factory/actions/runs/33241025528) failed at the same validation step with `spawnSync git ENOBUFS`; no quiescent completion or content-free-cycle proof exists. | A successful post-merge no-op/quiescence observation. |
| PC-05 — pinned current-main quality | PASS | Hosted CI run `33255476187` had terminal-success `Backend Lint` job `99108285501` and `Verification Policy` check `99109664912`. Artifact `backend-deadcode-evidence`, ID `9715691337`, was compared with the committed baseline: both Git blobs were `358716aeb0095882890819e58e0b98c09a8c9993`, both were 315,124 bytes and 3,074 lines. In the fresh checkout, `make backend-size` exited 0 in 5.063s and `make pkg-maint` exited 0 in 7.691s. | Local Windows `make deadcode` was diagnostic-only and exited 2 (`baseline findings: 3074, current findings: 3072`); hosted equality is the supported verdict. |
| PC-06 — dispositions and open-PR attribution | BLOCKED | `#2338` is CLOSED with `mergedAt:null` and `mergeCommit:null`; its final targeted Unix correction is present in the pinned baseline while the full baseline blob legitimately differs, proving substance rather than ancestry. `#2343` is merged as `753e107b...`, is an ancestor of pinned main, and its `Classify Verification`, `Workflow Lint`, `Backend Unit Latency`, `Backend Lint`, and `Verification Policy` checks all succeeded. The open-PR inventory and immutable-head attribution are recorded below. | Open PR `#2447` reported a Backend Lint failure while run `33256801996` was still in progress; its failed check had no terminal log when inspected. |
| PC-07 — clean-room and scope | PASS | Fresh detached pin, empty status, read-only repository/GitHub validation, no secret exposure, and one owned report are recorded. The only tracked change intended by this lane is this report; `prd.json` and `progress.txt` remain ignored scaffolding. | The absent source plan remains a plan-authority gap, not a scope expansion. |
| PC-08 — strict verdict rule | PASS | The report returns `FAIL`, not `PASS`, because PC-02 is BLOCKED and PC-03/PC-04 are failed. No failed or missing edge was converted into a pass. | None. |
| PC-09 — hosted performance authority | PASS | Hosted Backend Lint, unit-lane, and PR check identities are treated as primary. Saturated Windows timing and the local dead-code mismatch are explicitly diagnostic only. | No local timing threshold is claimed. |
| PC-10 — report-PR CI handoff | PASS | The final handoff opens the named report-only PR from this report commit, starts its own CI, and records the initial run identity in the PR conversation rather than in a commit. | Review owns terminal CI, conflict resolution, and merge. |
| DELIVERY — implementation-stage stop point | PASS | After the final head is pushed, the report-only PR is open, initial CI is started, and the handoff comment contains its run identity. This lane stops there. | Merge remains the review-stage boundary. |

## Story acceptance mapping

| Story edge | Status | Evidence / remaining edge |
| --- | --- | --- |
| Fresh checkout, complete pin, clean status | PASS | Detached clone at `0228a6d5...`; clean porcelain status. |
| Deliberate source merge changes both snapshots | BLOCKED | No public bot PR in the enumerated history changes `deadcode-baseline.txt`; the source plan is missing. |
| Required-green, unattended bot PR for the deliberate delivery | BLOCKED | `#2435` proves the bot auto-merge mechanism for one baseline only; it is not evidence for the missing dual-snapshot delivery. |
| Separate later real main change self-heals | FAIL | `#2444` → CI `33255476187` → regeneration `33256000211`; regeneration failed with `spawnSync git ENOBUFS`. |
| Baseline-only bot merge reaches quiescence | FAIL | `#2435` → merge SHA `3fa80f6e...` → regeneration `33241025528`; validation failed with the same error. |
| Hosted deadcode artifact matches committed baseline | PASS | Artifact ID `9715691337` has the same Git blob, size, and line count as the committed baseline. |
| Backend-size/pkg-maint quality gates | PASS | Each target ran once in the fresh pin and exited 0; no hot-file violation. |
| `#2338` superseded in substance | PASS | Closed without merge; targeted Unix entries are in the current committed baseline and hosted artifact. Full-file equality is intentionally not claimed. |
| `#2343` disposition | PASS | Merged commit `753e107b...` is in pinned main; all five required checks succeeded. |
| Open baseline-related PR attribution | BLOCKED | Eleven terminal failures were attributable to changed source/diff or stale-main drift; `#2447` was nonterminal at inspection. |
| Fail-closed handling | PASS | Missing authority is BLOCKED; completed regeneration failures are FAIL; no repair or synthetic lineage was used. |
| Strict report verdict | PASS | The report verdict is FAIL because every criterion is not independently passing. |
| Report-only scope | PASS | No product, workflow, baseline, configuration, branch, or unrelated documentation file was changed. |
| Final report PR handoff | PASS | External PR/CI evidence is recorded in the PR conversation; CI results are not committed. |

## Customer journey

1. A fresh disposable checkout was pinned to current protected `main` at
   `0228a6d5...` and verified clean.
2. The real main change from PR `#2444` completed CI, then entered the actual
   regeneration workflow. Generation completed, but validation failed with
   `spawnSync git ENOBUFS` before a bot PR could be published.
3. The independently observed bot-only baseline merge `#2435` had green
   required checks and bot auto-merge, but its successor regeneration failed at
   the same validation boundary; quiescence was not demonstrated.
4. Hosted current-main dead-code equality and the two local quality targets
   were checked independently. The Windows dead-code discrepancy was retained
   as diagnostic and did not override hosted evidence.
5. Stranded PRs and all open PRs with baseline-related required-check failures
   were inspected at immutable heads. The one nonterminal open check remains a
   blocker.
6. This report is the sole owned change handed to review; no repair was made.

## Cross-task integration and usability

- Documentation discoverability: the requested report path is present, but the
  referenced source plan is absent from the checkout, `origin/main`, and the
  repository API.
- Permission and error behavior: repository and GitHub evidence was read-only;
  missing authority and incomplete workflow evidence remain explicitly
  BLOCKED, while completed workflow contradictions remain FAIL.
- Persistence/reload behavior: not applicable to this report-only validation
  story.
- Accessibility/keyboard/responsive behavior: not applicable; no UI surface
  changed.
- Operational signals: each material remote result has a PR, commit, run,
  check, or artifact identity and URL; credentials were not recorded.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C11-DUAL-001 | blocking | Enumerate the baseline bot PR history and compare each `gh pr diff --name-only`; look up the source plan. | One uniquely identified deliberate source merge should produce one bot PR changing both named snapshots. | No enumerated bot PR changes `deadcode-baseline.txt`, PR `#2347` has no snapshot files, and the source plan returns 404. | Public PR file lists; [PR #2347](https://github.com/portpowered/you-agent-factory/pull/2347); source-plan 404. |
| C11-REAL-001 | blocking | Observe PR `#2444` merge, CI run `33255476187`, then the linked regeneration run. | A later real main change should generate and publish the materially changed snapshot through the same mechanism. | Regeneration run `33256000211` completed with failure at `Validate generated working-tree scope`: `shared baseline workflow: unable to execute git: spawnSync git ENOBUFS`; publication was skipped. | [CI run 33255476187](https://github.com/portpowered/you-agent-factory/actions/runs/33255476187); [regeneration run 33256000211](https://github.com/portpowered/you-agent-factory/actions/runs/33256000211). |
| C11-QUIESCENCE-001 | blocking | Follow bot-only merge SHA `3fa80f6e...` from PR `#2435` to regeneration run `33241025528`. | The next run should establish no further material baseline change and no content-free successor cycle. | The run failed before quiescence evaluation with `spawnSync git ENOBUFS`. | [PR #2435](https://github.com/portpowered/you-agent-factory/pull/2435); [run 33241025528](https://github.com/portpowered/you-agent-factory/actions/runs/33241025528). |
| C11-OPEN-001 | blocking | Inspect open PR required-check failures at immutable heads. | Every baseline-related red PR should have a terminal, attributable cause. | PR `#2447` had a reported Backend Lint failure while run `33256801996` remained in progress; no terminal log was available. | [PR #2447](https://github.com/portpowered/you-agent-factory/pull/2447); [run 33256801996](https://github.com/portpowered/you-agent-factory/actions/runs/33256801996). |
| C11-LOCAL-001 | informational | Run `make deadcode` once in the fresh Windows checkout and compare with hosted artifact. | Local output may diagnose platform differences; hosted package evidence controls. | Local target exited 2 with baseline 3074/current 3072; hosted artifact exactly matched the committed 3074-line baseline. | Local command output; hosted run `33255476187`, artifact `9715691337`. |

## Dispositions and open-PR attribution

### Stranded PRs

- `#2338` (`fix-deadcode-baseline-drift-recordings`) is CLOSED, not merged:
  head `0989ac9a...`, `mergedAt:null`, `mergeCommit:null`, and its head is not
  an ancestor of pinned main. Its final correction restored the targeted Unix
  entries for repository-staging locks, command-process tests, and the
  terminal-port lock. Those targeted entries are present in the pinned
  baseline, and the hosted artifact equals that baseline. The full blob is not
  claimed equal because later valid baseline content exists.
- `#2343` (`Clarify stale-head routing precedence over convergence hold`) is
  merged as `753e107b27fd7f2fceab738ad013abecd6347f85`, and that merge is an
  ancestor of pinned main. Required checks `Classify Verification`, `Workflow
  Lint`, `Backend Unit Latency`, `Backend Lint`, and `Verification Policy` all
  completed successfully in run `33183683082`.

### Open PRs with baseline-related red checks

The command sequence was `gh pr list ... --json ...`, `gh pr diff <n>
--name-only`, and `gh run view <run> --job <job> --log-failed`; no PR was
changed or rerun.

| PR / immutable head | Red required check | Attribution |
| --- | --- | --- |
| `#2447` / `a50e8dbf...` | Backend Lint, run `33256801996` | BLOCKED: check was nonterminal and its log was unavailable at inspection. |
| `#2445` / `24591250...` | Backend Lint, run `33253618223` | Source diff: the report names stale/unregistered exemption entries for modified `named_factory_test.go`. |
| `#2443` / `55e7459b...` | Backend Unit Latency, run `33256284829` | Source diff: the changed Petri cross/guard functional tests caused the unit-sample wrapper to exit 1. |
| `#2439` / `1363d727...` | Backend Unit Latency, run `33253485328` | Source/measurement diff: the PR changes the harness and the process test path; the budget check was terminally failed and no baseline snapshot file was changed. |
| `#2434` / `bfff9d51...` | Backend Lint, run `33256086458` | Source diff: diagnostics include changed `failure_baseline_no_server_test.go` maintainability complexity and backend-size failure. |
| `#2363` / `ee45675e...` | Backend Lint, run `33133308827` | Source diff: the PR changes factory-session implementation/tests; backend-size and pkg-maint were red on that head. |
| `#2361` / `b434aa34...` | Backend Lint and Backend Unit Latency, run `33132509158` | Source diff: changed `window_test.go` contains a 117-line test over the 100-line limit; the unit sample wrapper also exited 1. |
| `#2345` / `a05a800a...` | Backend Unit Latency, run `33125634420` | Source diff: its own unit-lane/reference machinery failed the unchanged 25% median-improvement gate at 13.27%. |
| `#2311` / `8db15fc6...` | Backend Lint, run `32967079777` | Source diff: the changed `board_persistence_cli_test.go` is 1,038 lines against the 1,000-line limit. |
| `#2192` / `3fd15db9...` | Backend Lint, run `32805304951` | Source diff: broad production and functional changes on the head coincided with dead-code baseline drift (46 to 42). |
| `#2181` / `40b6e96c...` | Backend Lint, run `32591686112` | Main drift: the only changed file is review-agent documentation, while the old head reported dead-code drift (398 to 400). |
| `#2174` / `6b1d15bc...` | Backend Lint, run `32590859316` | Source/stale-base surface: the PR changes factory-visualization implementation and tests, and its old dead-code check reported 398 to 399; no baseline file was changed. |

All other open PR red checks in the inventory were coverage, functional,
integration, or stability checks rather than baseline-related `Backend Lint`
or `Backend Unit Latency` failures.

## Verdict

FAIL

The strict verdict cannot be PASS: the historical dual-snapshot lineage is
BLOCKED, and two completed real workflow paths fail before self-heal or
quiescence can be proven. Current-main hosted equality and local quality gates
pass, but they do not override the failed behavior edges.

## Delta-plan request

- Affected behavior and criterion: `PC-02` / `GATE-DUAL`.
  - Root-cause evidence or remaining uncertainty: the source plan is absent and
    no uniquely identified bot PR changes both required snapshots.
  - Smallest recommended correction/prerequisite: provide an authoritative
    source-plan copy or public PR/run lineage naming the deliberate source SHA
    and dual-snapshot bot PR; otherwise retain BLOCKED. Do not synthesize a
    candidate from unrelated bot PRs.
  - Dependencies and retest scope: repeat the dual-snapshot trace, required
    checks, bot auto-merge, and merge ancestry only after that authority exists.
- Affected behavior and criterion: `PC-03` / `GATE-REAL`.
  - Root-cause evidence or remaining uncertainty: completed run `33256000211`
    fails in the helper's `spawnSync git` validation with `ENOBUFS` after
    regeneration and before publication.
  - Smallest recommended correction/prerequisite: the owning automation lane
    should bound or stream the validation subprocess output, then rerun the
    exact current-main source CI → regeneration path. This validation lane did
    not repair it.
  - Dependencies and retest scope: one fresh pinned main source event, linked
    terminal regeneration run, material snapshot diff, bot PR, and required
    checks.
- Affected behavior and criterion: `PC-04` / `GATE-QUIESCENCE`.
  - Root-cause evidence or remaining uncertainty: the known bot-only merge
    `#2435` reaches the same completed `ENOBUFS` failure in successor run
    `33241025528`.
  - Smallest recommended correction/prerequisite: after the automation fix,
    observe the successor run through its content-sensitive quiescence branch
    and verify no content-free successor PR.
  - Dependencies and retest scope: bot-only baseline merge, linked terminal
    source and regeneration runs, PR diff, and main ancestry.
- Affected behavior and criterion: `PC-06` / `GATE-OPEN-PRS`.
  - Root-cause evidence or remaining uncertainty: `#2447` was not terminal at
    the single permitted inspection point.
  - Smallest recommended correction/prerequisite: inspect that immutable head
    after the workflow reaches a terminal state in a future validation
    iteration; do not rerun it in this session.
  - Dependencies and retest scope: only PR `#2447`'s Backend Lint check and
    its final log, plus the existing attribution inventory.
