# Stability cleanup C11 functional self-heal re-probe

## Verdict

**BLOCKED — the strict validation-loopback verdict is not PASS.**

The current-main snapshot is internally consistent on the hosted Linux
deadcode evidence and the local package-quality gates pass. The required
behavioral spine is not proven: the source plan and deliberate dual-drift
identity are absent, the qualifying later real source merge did not produce a
baseline self-heal PR, and the bot regeneration workflow stopped before it
could establish quiescence.

This is a validation report only. No baseline, workflow, source, or GitHub
state was repaired as part of this probe.

## Authority, clean-room boundary, and environment

The assigned source-plan reference is
'docs/temp/stability-cleanup.md'. It is absent from the fresh checkout and
'gh api repos/portpowered/you-agent-factory/contents/docs/temp/stability-cleanup.md?ref=main'
returned HTTP 404. The dependency
'stability-cleanup-c11-deliberate-dual-drift' also has no identifiable
source SHA, description, or delivery PR in the available repository and
GitHub state. These are authority gaps, not inferred passes.

The probe used one disposable fresh clone, detached at the full current-main
SHA, and read-only GitHub evidence. It did not read prior validation reports,
state/addenda, sibling probe output, implementation plans, or worker
summaries. The only tracked file changed by this work is this owned report.

| Item | Observed value |
| --- | --- |
| Pinned current main | '0d7d40b403bdd856fddfe3b6c1dfa5e092a60d67' |
| Fresh-checkout status | 'git status --porcelain=v1' produced no output |
| Go / Node / gh / Make | Go 1.25.0 / Node 22.12.0 / gh 2.88.0 / GNU Make 4.4.1 |
| Host | Windows 11 build 26200.7623 |
| Hosted artifact | 'backend-deadcode-evidence' from run '33258610796', 'deadcode-current.txt' |
| Report PR | #2448, https://github.com/portpowered/you-agent-factory/pull/2448 |

## Executable journey and evidence

1. A fresh clone was detached at 'origin/main' and its status was empty.
2. The current-main source CI run was inspected. Backend Lint and Backend Unit
   Latency succeeded, but Backend Functional Coverage failed in
   'tests/functional/transport/acp/stdio:Test...' after the 600-second
   timeout; Verification Policy consequently failed.
3. The exact hosted deadcode artifact was downloaded once. It has 3,074
   lines and SHA-256
   'F31645C911B22D76E5A121E0DA0C47D5549DE16045E1D803E0003A254AFDFE13',
   exactly matching the committed current-main baseline. A line comparison
   reported zero differences.
4. On the same fresh checkout, 'make backend-size' and 'make pkg-maint' both
   exited 0. 'make deadcode' is only a Windows diagnostic: it reported 3,072
   findings versus the committed 3,074 and exited 1. The hosted Linux
   artifact is the authoritative equality result; the local platform result
   was not used to alter any baseline.
5. The real source merge used for the later-self-heal probe was #2444:
   'functional-test-optimization-c12-transport-cli-output'. Its source CI
   passed, and its merge commit was followed by regeneration run '33256000211'.
   That regeneration ended before PR reconciliation with
   'spawnSync git ENOBUFS'; no self-heal PR was produced.
6. The bot-only quiescence probe used merged bot PR #2435. Its source CI and
   post-merge main CI passed, but regeneration run '33241025528' again ended
   with 'spawnSync git ENOBUFS'. Therefore no terminal quiescence or
   content-free successor rule can be established.

## Project acceptance criteria

| ID | Status | Evidence and remaining unproven edge |
| --- | --- | --- |
| PC-01 Report maps every criterion | PASS | This table covers PC-01 through PC-10 and the delivery gate with an explicit status, evidence, and unproven edge. |
| PC-02 Deliberate dependency/source merge changes both snapshots | BLOCKED | The source plan is missing, the named dependency has no identifiable delivery identity, and the merged bot history contains no dual change to 'deadcode-baseline.txt' and 'go-unit-lane-latency-budget.v1.json'. PR #2347 is the automation implementation, not the deliberate dual-drift delivery. |
| PC-03 Later real source merge self-heals through one bot PR | FAIL | PR #2444 is a real source merge with green source CI and changed functional transport tests. Its linked regeneration run #33256000211 failed on 'git ENOBUFS' before reconciliation, so no material bot self-heal PR exists. |
| PC-04 Bot-only merge reaches quiescence | BLOCKED | Bot PR #2435 merged only the latency snapshot and had green source/post-merge CI, but linked regeneration #33241025528 failed before quiescence determination. No content-free or 'reference.baseCommit' successor conclusion is justified. |
| PC-05 Current-main quality and snapshot equality | PASS | Hosted artifact from #33258610796 equals the pinned committed baseline byte-for-byte by SHA-256 and line comparison; Backend Lint job #99116560087 succeeded; 'make backend-size' and 'make pkg-maint' exited 0. |
| PC-06 Stranded PRs and open red baseline PRs are attributed | PASS | #2338 is closed unmerged and superseded in substance; #2343 is merged and its merge commit is an ancestor of current main. The exact shared-baseline path scan found #2211 and #2345: #2211's red functional run is an unchanged outside-diff flake, while #2345's red latency run is a source-diff measurement failure. Domain-specific near-miss "baseline" paths were excluded. |
| PC-07 Scope and safety | PASS | The probe made no production or GitHub repairs, used no secret, and limits the tracked change to this report. |
| PC-08 Strict verdict | BLOCKED | Strict PASS requires every preceding behavioral edge. PC-02 and PC-04 remain unproven and PC-03 is an observed failure. |
| PC-09 Validation evidence is reproducible | PASS | The pinned SHA, exact hosted run/artifact, hashes, commands, exit results, and PR/run URLs are recorded. Local Windows deadcode divergence is explicitly classified as diagnostic rather than substituted hosted evidence. |
| PC-10 Report PR handoff | BLOCKED | Existing PR #2448 is open and its prior comment records CI start on its earlier report head. This updated report still needs its final head pushed and a new CI-start comment before this criterion can be claimed. |
| Delivery gate | BLOCKED | The report can be handed to review after the final push, but the strict work-item verdict remains blocked by the missing authority and failed/incomplete self-heal evidence. |

## Disposition details

### Deliberate dual-drift lineage

The available merged bot history was filtered for the two exact shared
snapshot paths. It contains repeated latency-v1 reconciliations, including
#2435, but no bot PR that materially changes both required snapshots. The
automation implementation in #2347 changed workflow and tooling files only.
Without the missing source plan or a unique accepted source merge, there is no
safe way to label a run "deliberate" or manufacture its expected dual diff.

### Later real source merge

PR #2444 merged at '0228a6d5e081ea65b03f11ee9553f636071eaa01' after green
Backend Lint, Backend Unit Latency, Backend Functional Coverage, and
Verification Policy checks in its source run. The subsequent current-main
source run '33255476187' also passed. Regeneration run
'33256000211' recorded source conclusion success but failed in
'Validate generated working-tree scope' with 'spawnSync git ENOBUFS'; it never
reached bot-PR reconciliation. This is a failed self-heal edge, not a
missing observation.

### Stranded and open PRs

PR #2338 is closed with no merge commit. Its old head contained a deadcode
baseline of 3,103 lines and SHA-256
'A620F0FAE6462F0C36E95D12963BEF8A791F0294F9C417421F19B78169C39651';
the pinned current baseline has 3,074 lines and SHA-256
'F31645C911B22D76E5A121E0DA0C47D5549DE16045E1D803E0003A254AFDFE13', with
151 differing lines. The closing disposition identifies its correction
substance as absorbed by later current-main history.

PR #2343 merged at
'753e107b27fd7f2fceab738ad013abecd6347f85'; the fresh checkout confirmed
that merge commit is an ancestor of current main. Its original PR head is
not an ancestor, as expected for a merged PR.

The exact shared-path open-PR scan found:

- #2211, which includes the deadcode snapshot alongside package/source
  changes. Its failed functional test was outside the PR diff, passed in the
  focused/base comparison, and remained the documented baseline flake after
  one permitted rerun.
- #2345, which changes the latency-v2 snapshot/schema and related source
  contract. Its red latency check measured a 13.27% improvement against a
  required 25%, so its failure is attributable to the source-diff contract,
  not an ambiguous shared-baseline drift.

## Findings and smallest safe delta

- **F-01 / BLOCKED:** restore or provide the authoritative source plan and
  the accepted deliberate dependency/source merge identity.
- **F-02 / FAIL:** investigate the shared-baseline workflow's
  'spawnSync git ENOBUFS' failure in its owning implementation lane, then
  rerun the later-real-merge self-heal proof.
- **F-03 / BLOCKED:** repeat the bot-only quiescence observation after a
  successful regeneration and verify that no content-free or reference-only
  successor is emitted.
- **F-04 / CURRENT-MAIN SIGNAL:** current main at #2443 has a separate ACP
  stdio functional timeout ('tests/functional/transport/acp/stdio', about
  600 seconds) and a dependent Verification Policy failure. This report
  records it; it does not repair unrelated production/shared-host state.

The next validation iteration should begin from a new full current-main SHA
after those authority/workflow conditions change. No acceptance item is
marked pass merely because a routing disposition was recorded.

## Handoff

The report is ready to update PR #2448 after its final-head push. The final
handoff must preserve this report-only scope, add the final CI-start evidence
as a PR comment rather than a commit, and leave the strict verdict
**BLOCKED** until the missing and failed behavioral edges are resolved.
