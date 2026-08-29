# C10 Claude inference disposition audit

Status: `AUDIT-C10-01` and `CURRENT-C10-01` pass for story
`functional-test-optimization-c10-workers-inference-claude-disposition-001`.

This document is the story-001 audit artifact. It records provenance and the
current-main characterization before any recovered edit is reused. No recovered
commit was cherry-picked. No Claude test, protected baseline, public contract,
or pull-request state was changed by this story.

## Scope and authority

| Item | Recorded value |
| --- | --- |
| Repository | `portpowered/you-agent-factory` |
| Work branch | `functional-test-optimization-c10-workers-inference-claude-disposition` |
| Current checkout and `origin/main` | `0c3bb857910bf4e356b01942954e7272572510f9` |
| PRD story | `functional-test-optimization-c10-workers-inference-claude-disposition-001` |
| Parent behavior | `BEH-C10-CLAUDE-DISPOSITION` |
| Paid or remote calls | `0`; USD `0` |
| Source-plan path named by the PRD | `docs/temp/functional-test-optimization.md` |

The PRD-named source plan and its referenced `addenda.md` are absent from this
checkout and from the refs inspected by `git log --all --`. The missing source
plan is recorded as a recovery conflict. The PRD, repository standards, current
source, and GitHub PR conversation supply the bounded audit authority. The safe
policy decision is to create a c10 successor and link the closed PR rather than
reopen it silently.

## AUDIT-C10-01: provenance and ownership

### Inputs and procedure

The audit used the repository-real and GitHub-real procedure required by the
task:

```text
git fetch origin main refs/pull/2331/head:refs/remotes/origin/pr-2331
gh pr view 2331 --repo portpowered/you-agent-factory
gh pr checks 2331 --repo portpowered/you-agent-factory
gh api repos/portpowered/you-agent-factory/issues/2331/comments --paginate
gh api repos/portpowered/you-agent-factory/pulls/2331/reviews --paginate
git show <each recovered commit>
git merge-base origin/main origin/pr-2331
git rev-list --count origin/pr-2331..origin/main
git rev-list --count origin/main..origin/pr-2331
git cherry -v origin/main origin/pr-2331
git diff --name-status <merge-base> origin/pr-2331
git diff --check <merge-base> origin/pr-2331
git merge-tree origin/main origin/pr-2331
```

The commands ran at current `origin/main` `0c3bb857910bf4e356b01942954e7272572510f9`.
The recovered PR ref is `origin/pr-2331`.

### PR and graph identity

| Fact | Evidence |
| --- | --- |
| PR | [#2331](https://github.com/portpowered/you-agent-factory/pull/2331) |
| PR title | `functional-test-optimization-c03-workers-inference-claude` |
| PR state | `CLOSED`; `mergedAt` is `null` |
| Historical head | `636e02b92f8eef43374a8211cc18bc10e9aa640b` |
| Historical branch | `functional-test-optimization-c03-workers-inference-claude` |
| Historical PR base | `34cbab9f253208328c59b82eb8a3f17b76c09d15` |
| Three-dot merge base with current main | `34cbab9f253208328c59b82eb8a3f17b76c09d15` |
| Current main commits after merge base | `66` |
| Recovered commits after merge base | `5` |
| `git cherry -v origin/main origin/pr-2331` | Five `+` entries, one for each recovered commit |
| Read-only merge simulation | `git merge-tree` exit `0`; merged tree `854fcbec1d0228fb15e79439a75239b56739f920`; no conflict output |
| Patch whitespace check | `git diff --check` exit `0` |
| c02 separation | `git merge-base --is-ancestor 57dfe4e6e origin/pr-2331` exit `1` |

The historical branch is not current-main plus five commits. It is current-main
minus 66 commits plus the five recovered commits. Only the five package commits
are eligible for ordered import.

### Complete three-dot ownership

The complete `34cbab9f253208328c59b82eb8a3f17b76c09d15...origin/pr-2331` diff
contains eight paths, with `1,206` additions and `287` deletions:

```text
A tests/functional/workers/inference/claude/conductor_assertions_test.go
A tests/functional/workers/inference/claude/conductor_fixture_test.go
A tests/functional/workers/inference/claude/conductor_http_test.go
A tests/functional/workers/inference/claude/conductor_router_test.go
A tests/functional/workers/inference/claude/conductor_scenarios_test.go
M tests/functional/workers/inference/claude/conductor_test.go
M tests/functional/workers/inference/claude/golden_common.go
M tests/functional/workers/inference/claude/golden_failure_test.go
```

The excluded-path filter found `0` matches. The diff contains no OpenAPI, CLI,
event, persisted-schema, configuration, generated, UI, localization,
workflow, production, shared-support, sibling-package, or protected-baseline
path. A direct tip-to-tip diff against current main is much larger because it
also shows the 66 current-main commits absent from the historical branch. That
tip-to-tip result is not the recovered ownership boundary.

### Current-main package source

There are five default-source files in the Claude package at both the merge base
and current main. The package history query after the merge base returned no
entries, and every file hash matches the merge-base hash:

| File | Merge-base/current-main blob |
| --- | --- |
| `conductor_test.go` | `8bdb64bf0bcf8ab843d1cd566645ebdbf8d1fe01` |
| `golden_common.go` | `f0eda3bb7247ecc42497bbfaee4ca941d3d320a2` |
| `golden_common_long_test.go` | `60e2f115697a2f6343d57a1f7fb2d458273211c1` |
| `golden_failure_test.go` | `31ec52c3436db0ddf8c1bf977b8c810ddf345b10` |
| `golden_success_test.go` | `5816945aeda341126fd4334a28a821654cd6d89e` |

The recovered head adds five conductor helper files and changes three existing
package files. The 32 tracked Claude provider-session fixture files are present
at the merge base, current main, and recovered head. The fixture directory has
no three-dot diff.

The protected files have identical blobs at all three refs:

| Protected file | Blob at merge base, current main, and recovered head |
| --- | --- |
| `golden_success_test.go` | `5816945aeda341126fd4334a28a821654cd6d89e` |
| `golden_common_long_test.go` | `60e2f115697a2f6343d57a1f7fb2d458273211c1` |

## Per-commit disposition

Each recovered commit has exactly one disposition. All five are `revive`. The
ordered import is adopted only after this document is committed and the story
passes. No import occurred during the audit.

| Order | Commit and parent | Patch purpose and exact paths | Decision and compatibility evidence | Exact import action |
| ---: | --- | --- | --- | --- |
| 1 | `e655ff198754e09ebfccf4c13ffff7079c7092f9`<br>parent `34cbab9f253208328c59b82eb8a3f17b76c09d15` | Establishes the shared Claude conductor process and explicit-session route spine.<br>`conductor_test.go`: `+680/-61` | **Revive.** It is unique in `git cherry`, is package-only, and the current package source is unchanged from its parent. It must land first because later commits build on this fixture. | From a fresh c10 branch at current `origin/main`, cherry-pick this commit first. Do not merge the stale branch. |
| 2 | `35e6d82d3575daa1a193e319fa4a9f93dd348661`<br>parent `e655ff198754e09ebfccf4c13ffff7079c7092f9` | Extends the shared fixture to structured failure and timeout behavior.<br>`conductor_test.go`: `+316/-81`; `golden_common.go`: `+2/-11`; `golden_failure_test.go`: `+4/-187` | **Revive.** It is unique, follows commit 1, and changes only the owned package. Its golden helper changes require the story-002 parity and protected-byte checks. | Cherry-pick second, then run the focused failure and timeout selectors. |
| 3 | `6b6f6e2f2473d8de8fcaa3d16572cb2d0a126a5e`<br>parent `35e6d82d3575daa1a193e319fa4a9f93dd348661` | Adds the shared cleanup boundary.<br>`conductor_test.go`: `+150/-12` | **Revive.** It is unique, package-only, and depends on the explicit-session process created by commits 1 and 2. Cleanup is a later behavior proof, not a current audit pass. | Cherry-pick third, then run cleanup/recovery evidence in story 002. |
| 4 | `334aaab4add1700b0994fe6fb5e0ba9e3d38d9b9`<br>parent `6b6f6e2f2473d8de8fcaa3d16572cb2d0a126a5e` | Scopes anchored cleanup probes to the intended package selectors.<br>`conductor_test.go`: `+9/-8` | **Revive.** It is unique, package-only, and preserves the full-parent cleanup boundary while narrowing anchored probes. | Cherry-pick fourth without squashing, then retain its selector-specific evidence. |
| 5 | `636e02b92f8eef43374a8211cc18bc10e9aa640b`<br>parent `334aaab4add1700b0994fe6fb5e0ba9e3d38d9b9` | Splits the conductor into size-compliant focused helpers.<br>`conductor_assertions_test.go`: `+314`; `conductor_fixture_test.go`: `+362`; `conductor_http_test.go`: `+56`; `conductor_router_test.go`: `+210`; `conductor_scenarios_test.go`: `+207`; `conductor_test.go`: `-1031` | **Revive.** It is unique, package-only, and is the reviewed correction for the earlier backend-size blocker. The merge simulation reports no conflict. Its semantic parity still requires story-002 runtime evidence. | Cherry-pick fifth and preserve the five-file helper split. Do not reintroduce the old oversized file or repair the protected baseline. |

The exact post-audit action is therefore:

```text
git switch -c functional-test-optimization-c10-workers-inference-claude-disposition origin/main
git cherry-pick e655ff198754e09ebfccf4c13ffff7079c7092f9
git cherry-pick 35e6d82d3575daa1a193e319fa4a9f93dd348661
git cherry-pick 6b6f6e2f2473d8de8fcaa3d16572cb2d0a126a5e
git cherry-pick 334aaab4add1700b0994fe6fb5e0ba9e3d38d9b9
git cherry-pick 636e02b92f8eef43374a8211cc18bc10e9aa640b
```

The commands above are the story-002 import plan. They were not executed by
story 001. A conflict, excluded-path change, or public witness drift stops the
import and requests a plan delta.

## CURRENT-C10-01: current behavior characterization

### Executable inventory

The current-main command was run against the local production composition:

```text
go test ./tests/functional/workers/inference/claude -list '^Test'
```

It exited `0` and listed the four default tests below. Go reported `0.083s`.

| Case | Current public witness |
| --- | --- |
| `TestClaudeConductorSuccessThroughRootBuildProcess` | Completed Work, zero failed Work, one controlled Claude call, `claude`, model, and `stream-json` arguments |
| `TestClaudeCommandCancellationThroughRootBuildProcessIsCanonical` | Failed Work, one controlled Claude call, canonical cancellation Factory Event text, no Claude-local fallback text |
| `TestClaudeGoldenStructuredFailure` | Failed Work, one controlled Claude call, `PERMANENT_BAD_REQUEST`, actionable message, Provider Session and response-event golden observation |
| `TestClaudeGoldenTimeoutClosesResponseStream` | Failed Work, no completed Work, timeout failure detail, terminal `FAILED` response event, no fabricated completion |

The two `functionallong` tests are visible in the source but excluded from the
default command by their build tag:

| Tagged case | Protected witness and reason for current isolation |
| --- | --- |
| `TestClaudeGoldenFullStreamTextSuccess` | Full-stream text delta/final-snapshot and Provider Session/invocation goldens. It remains in the tagged golden lane. |
| `TestClaudeGoldenToolLifecycleAndSessionIdentity` | Tool start/completion order and stable Provider Session identity. It remains in the tagged golden lane. |

The required current-main package observation then ran exactly once with:

```text
go test -count=1 -timeout=10m ./tests/functional/workers/inference/claude
```

It exited `0`. Go reported package time `11.155s`; the outer Windows stopwatch
reported wall time `16.135s`. The host was running unrelated Go suites at the
same time, so this is a noisy directional baseline. It is not a fixed threshold
and does not prove successor performance or CI contention.

### Current process and resource topology

Each default test calls the shared support helper independently. The helper
performs the following sequence through the production test boundary:

```text
test case
  -> CopyFixtureDir and WriteAgentConfig
  -> NewProcessAPIServer
  -> root.BuildProcess
  -> Process.Execute through StartProcessCommand
  -> local HTTP server and one default Factory Session
  -> public Work and Factory Event observations
  -> daemon.Stop
  -> Process.Close
```

The helper passes `--no-record`, sets an invocation-local `HOME` and
`USERPROFILE`, and replaces only `edges.Edges.ProviderCommandRunner` plus the
API-server starter. No live Claude binary, credential, or network provider is
used.

| Resource or boundary | Current default topology | Evidence limit |
| --- | --- | --- |
| Root-built application process | Four independent `BuildProcess` calls, one per default test | Inferred from four test/helper call paths; no process counter exists in this package |
| API server/listener | Four independent `NewProcessAPIServer` instances | The helper closes the process, but no package census asserts listener count |
| Factory Session | Four implicit default sessions obtained from four servers | No explicit session open/delete or cross-session identity assertion exists yet |
| Controlled Claude calls | Success `1`, cancellation `1`, structured failure `1`; timeout queues `9` results but asserts only `>=1` call | Exact timeout call and dispatch counts remain unproven in current main |
| Response-event capture | Structured failure and timeout capture the public stream; success and cancellation do not | Current package does not assert event IDs, ordering, or duplicate absence for all cases |
| Temporary roots | `CopyFixtureDir` and helper `t.TempDir` homes are created per test | `t.Cleanup` and `Process.Close` are used, but no independent zero-residue census exists |
| Worktrees | No direct worktree setup appears in the current Claude package | Worktree cleanup is not claimed by this characterization |
| Recording state | `--no-record` | No restart-recovery claim is made |

### Current public witness limits

The current tests preserve useful behavior, but they do not prove the shared
process or explicit-session optimization. The following gaps are intentional
inputs to later tasks:

- Success does not currently assert dispatch identity, Provider Session
  identity, ordered response events, or explicit session deletion.
- Cancellation does not currently capture a response stream or assert dispatch
  identity and session deletion.
- Structured failure compares Provider Session observations, but does not assert
  the exact shared route, dispatch identity, or cross-session isolation.
- Timeout queues nine controlled results and asserts timeout semantics, but does
  not assert exactly nine calls, three dispatch failures, or route isolation.
- The tagged cases remain separate golden-contract witnesses and were not run by
  the default characterization command.

## Complete C10 behavior matrix at the audit boundary

The table records one current witness or an explicit unchanged owning gate for
every matrix row. `Gap` means story 001 does not claim the property.

| ID | Current witness or unchanged owner | Audit status and next owner |
| --- | --- | --- |
| `C10-H01` | `TestClaudeConductorSuccessThroughRootBuildProcess` proves Work completion, no failure, command, model, and stream format. | Partial witness; dispatch, Provider Session, ordered stream, and explicit-session isolation -> story 002. |
| `C10-H02` | `TestClaudeGoldenStructuredFailure` proves failed Work, `PERMANENT_BAD_REQUEST`, actionable message, and sanitized golden observations. | Partial witness; shared route and full dispatch parity -> story 002. |
| `C10-H03` | `TestClaudeGoldenTimeoutClosesResponseStream` proves timeout Work failure, `FAILED` stream terminal state, and no fabricated success. | Partial witness; exact nine calls and three dispatches -> story 002. |
| `C10-H04` | `TestClaudeCommandCancellationThroughRootBuildProcessIsCanonical` proves failed Work, one call, canonical cancellation text, and no fallback. | Partial witness; dispatch, response stream, and explicit-session cleanup -> story 002. |
| `C10-H05` | `TestClaudeGoldenFullStreamTextSuccess` under `functionallong` compares full-stream golden observations. | Tagged case remains isolated; exact reason and current-head run -> `CLAUDE-LONG-C10-01` in story 003. |
| `C10-H06` | `TestClaudeGoldenToolLifecycleAndSessionIdentity` under `functionallong` compares tool/session golden observations. | Tagged case remains isolated; exact reason and current-head run -> `CLAUDE-LONG-C10-01` in story 003. |
| `C10-U01` | No current route selector exists. | Gap; unknown selector must fail closed -> story 002. |
| `C10-U02` | No current duplicate route construction exists. | Gap; duplicate selector construction must fail -> story 002. |
| `C10-U03` | Provider-neutral malformed/missing completion remains owned by excluded root-inference tests. | Unchanged owning gate: `ROOT-INFERENCE-REGRESSION`. |
| `C10-U04` | Provider-neutral partial completion remains owned by excluded root-inference tests. | Unchanged owning gate: `ROOT-INFERENCE-REGRESSION`. |
| `C10-U05` | No current empty-route or empty-terminal witness exists. | Gap; fail closed without fabricated Work or Provider Session -> story 002. |
| `C10-U06` | No current response-event identity ledger exists. | Gap; non-empty ordered unique IDs -> story 002. |
| `C10-C01` | Current tests run independently and never overlap shared routes. | Gap; one process and four isolated sessions under concurrency -> story 002. |
| `C10-C02` | Timeout helper has a nine-result queue, but current assertions do not bind capacity to one route. | Partial fixture only; exact nine-call/three-dispatch isolation -> story 002. |
| `C10-X01` | Support stops and closes each independent process. | Gap; active-call zero, stream close, session delete, and reuse -> story 002. |
| `C10-X02` | No partial-setup cleanup assertion exists in the Claude package. | Gap; acquired-resource attempt and original-error preservation -> story 002. |
| `C10-R01` | No adverse-session then success-session recovery witness exists. | Gap; fresh identities and same-process recovery -> story 002. |
| `C10-R02` | All current cases use `--no-record`; restart recovery remains excluded. | Unchanged owning gate: `ROOT-INFERENCE-REGRESSION`; no restart claim. |
| `C10-CL01` | `t.Cleanup` and `Process.Close` provide ordinary teardown paths. | Gap; zero process/session/stream/route/call/path census -> `CLEAN-C10-01` in story 002. |
| `C10-CL02` | No adverse-exit census exists. | Gap; exact non-zero residue must be reported -> `CLEAN-C10-01` in story 002. |
| `C10-S01` | The local injected HTTP server has no authorization mode, and no credentials or bypass hooks are added. | Authorization not applicable; preserve this boundary in story 002. |

## Prior PR evidence and closure reason

Prior PR evidence is separated from successor acceptance. It is useful for
disposition and risk assessment, but it is not evidence for a future c10 head.

### Green behavior and performance evidence reported by #2331

The PR body and issue comments reported the following historical results:

- Focused Claude package behavior passed at the reviewed heads.
- The selected Claude race command passed.
- `make pkg-structure`, `make test-lane-audit`, test listing, formatting, and
  diff checks passed at the reviewed heads.
- `Backend Functional Coverage` passed on final run
  [33192026813](https://github.com/portpowered/you-agent-factory/actions/runs/33192026813)
  at head `636e02b92f8eef43374a8211cc18bc10e9aa640b`.
- Historical implementation-checkout samples were wall/package seconds
  `5.566/1.956`, `4.465/1.913`, and `4.526/1.929`, with medians
  `4.526/1.929` and variance `0.2433/0.0223`.
- Historical clean-room samples were `6.214/2.351`, `5.693/2.237`, and
  `5.604/2.265`, with medians `5.693/2.265` and variance `0.1071/0.0503`.
- The reported recovered baseline was `12.256`, `18.160`, and `15.340` wall
  seconds, with median `15.340` and variance `0.3849`.
- The PR reported unchanged protected success/functionallong blobs and 32
  unchanged Claude provider-session fixture files.

These results describe historical heads, environments, and test topology. They
do not prove current-main compatibility beyond this audit and do not satisfy
successor behavior or CI criteria.

### Red check and closure reason

The final PR run at head `636e02b92f8eef43374a8211cc18bc10e9aa640b` had these
relevant outcomes:

| Check | Result |
| --- | --- |
| `Backend Functional Coverage` | Pass |
| `Backend Unit Coverage` | Pass |
| `Backend Test Stability` | Pass |
| `Backend Unit Latency` | Pass |
| `Backend Lint` | Fail |
| `Verification Policy` | Fail because the required lint result was red |

The final project-disposition comment
[`5455340233`](https://github.com/portpowered/you-agent-factory/pull/2331#issuecomment-5455340233)
records the closure reason: the test-only split changed the dead-code finding
count from protected baseline `3130` to current `3129`, and the plan forbids
editing `docs/internal/baselines/**`, adding artificial dead code, or overriding
the check. The comment describes the PR as closed and unmerged after the
authorized rebase path was exhausted. This is a protected-baseline delivery
conflict, not a Claude behavior failure.

The successor must therefore preserve the baseline unchanged. Review owns any
same-head classification if the conflict recurs. Story 001 does not edit or
reopen PR #2331.

## Security, cost, and protected surfaces

- The current and recovered evidence uses sanitized command output and
  synthetic identifiers.
- The functional edge is `edges.Edges.ProviderCommandRunner`; no custom
  in-process provider service or `MockWorkers` path is introduced.
- No remote Claude call, credential, paid call, public API, CLI grammar,
  persisted schema, event schema, configuration, generated output, UI, or
  localization surface is changed.
- Current tests use local temporary directories and `--no-record`.
- No evidence records secrets, prompts, or unsanitized temporary absolute paths.

## Disposition and handoff

`AUDIT-C10-01` passes. `CURRENT-C10-01` passes as a local characterization.
The five commits are approved for ordered revival onto current main, with no
stale ancestry import and no baseline edit. Story 002 owns the actual import,
behavior strengthening, explicit-session isolation, cleanup census, and touched
determinism proof. Story 003 owns final package evidence, successor PR linkage,
and review handoff.

Remaining unproven edges are revived behavior (`CLAUDE-DEFAULT-C10-01`), cleanup
and recovery (`CLEAN-C10-01`), the complete final package
(`PACKAGE-FINAL-C10-01`), tagged golden behavior (`CLAUDE-LONG-C10-01`), and
successor CI contention/performance (`CI-C10-01`).

## Story 002 — revived package evidence

The approved series was cherry-picked in audit order onto current `origin/main`.
The resulting commits are `97212a30ce`, `dd8ab75087`, `6dcab7ea76`,
`a9611e5f9a`, and `0df4793a10`, corresponding respectively to the five
recovered commits listed above. No conflict or excluded-path change occurred.

The package now constructs one production-composed process and one injected API
server for the four concurrent default scenarios. Each scenario selects an
immutable Factory-directory route, opens a unique non-default Factory Session,
submits unique Work/request/trace identities, and collects its own
session-scoped Factory Events and Response Events. The public assertions prove:

- success, cancellation, structured failure, and timeout parity, including
  one call for success/cancellation/structured failure and nine timeout calls
  across three failed dispatches;
- command, model, `stream-json`, Work, dispatch, Provider Session, failure,
  response-stream terminal, session/request scope, and response-event ID/order
  witnesses;
- empty, whitespace, and `.` route selectors rejected at construction,
  duplicate selectors rejected at construction, and unknown selectors rejected
  without consuming a known route;
- cancellation followed by a fresh successful explicit session on the same
  process, with exactly one API-server start; and
- normal/adverse stream, session, process, listener, active-call, and
  test-owned Factory-directory cleanup. The forced child-test exits at an
  intentional assertion after acquisition; its parent observed the expected
  non-zero child exit, one session and stream opened/closed, zero active calls,
  a stopped listener, and absent owned directories.

### Story-002 verification

All procedures used local production composition through `support.BuildProcess`
and controlled `edges.Edges.ProviderCommandRunner` effects. No remote Claude
call or credential was used.

| Scope | Exact procedure | Result and property proved |
| --- | --- | --- |
| Functional focused | `go test -count=1 -timeout=10m ./tests/functional/workers/inference/claude -run '^(TestClaudeDefaultLaneSharedProcess\|TestClaudeSameProcessRecoveryAfterAdverseSession\|TestClaudeCommandRouterFailsClosed\|TestClaudeForcedAssertionFailureCleansOwnedResources)$'` | Exit `0`; package reported `7.890s`. Revived default matrix, recovery, fail-closed routing, and forced adverse cleanup passed. |
| Functional repeat | `go test -count=3 -timeout=10m ./tests/functional/workers/inference/claude -run '^(TestClaudeDefaultLaneSharedProcess\|TestClaudeSameProcessRecoveryAfterAdverseSession\|TestClaudeCommandRouterFailsClosed)$'` | Exit `0`; package reported `18.623s`. Three repetitions preserved route/session/recovery/cleanup determinism. |
| Functional race | `go test -race -count=1 -timeout=10m ./tests/functional/workers/inference/claude -run '^(TestClaudeDefaultLaneSharedProcess\|TestClaudeSameProcessRecoveryAfterAdverseSession\|TestClaudeCommandRouterFailsClosed)$'` | Exit `0`; package reported `14.158s`. No race was detected in the exercised shared-process path. |
| Adverse cleanup | `go test -count=1 -timeout=10m ./tests/functional/workers/inference/claude -run '^TestClaudeForcedAssertionFailureCleansOwnedResources$' -v` | Exit `0`; package reported `1.889s`. The child intentionally failed; the parent verified original failure visibility and zero owned residue after cleanup. |
| Ownership | `git diff --check`, `git diff --name-only origin/main...HEAD`, protected-file comparison, and excluded-path filter | Whitespace check passed; only the c10 evidence file and Claude package are owned. Tagged full-stream/tool sources, fixtures, protected baselines, and excluded surfaces remain unchanged. |

This evidence proves the local revived behavior and exercised deterministic
cleanup. It does not prove tagged full-stream/tool goldens, the exact final
package command, GitHub-hosted contention/performance, independent loopback
convergence, or remote Claude compatibility; those remain story-003 gates.
