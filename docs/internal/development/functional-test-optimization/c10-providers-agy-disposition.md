# C10 AGY disposition and current-main characterization

## Status and scope

- Story: `functional-test-optimization-c10-providers-agy-disposition-001`
- Gate: `GATE-DISP-001`
- Status: **PASS**
- Recorded: 2026-08-28, branch `functional-test-optimization-c10-providers-agy-disposition`
- Current head during characterization: `0c3bb857910bf4e356b01942954e7272572510f9`
- Current `origin/main`: `0c3bb857910bf4e356b01942954e7272572510f9`
- Scope: #2316 disposition and read-only inventory of
  `tests/functional/providers/agy` before structural implementation.
- Dependency fidelity: current Git/GitHub state plus current-main source and
  Go test discovery; no live or paid AGY calls.
- Contract/configuration status: no OpenAPI, CLI grammar, Factory Event,
  persisted schema, product configuration, generated output, shared support,
  production, UI, or sibling-provider file was changed.

This is the pre-migration contract. It proves the recovered-change
disposition, current source denominator, public witnesses, ownership, and
static before topology. It does not claim post-migration behavior, cleanup,
performance, or remote AGY availability; those belong to later gates.

The referenced `docs/temp/functional-test-optimization.md` source plan is not
present in this checkout or in `origin/main`. The checked-in task packet
(`prd.md`/`prd.json`) supplies the same scope, matrix, and gate definitions;
the missing source-plan path is recorded rather than silently reconstructed.

## Disposition decision

The selected path is **close #2316 as superseded and recover only compatible
edits on current main**. #2316 was commented and closed before implementation:

- Supersede comment:
  [issue comment](https://github.com/portpowered/you-agent-factory/pull/2316#issuecomment-5460107039)
- Final PR state: `CLOSED`, `mergedAt=null`, closed at
  `2026-08-29T03:40:40Z`:
  [#2316](https://github.com/portpowered/you-agent-factory/pull/2316)
- New recovery branch: `functional-test-optimization-c10-providers-agy-disposition`

The comment names the current-main lane, compatible caller-process
initialization reuse, rejected blanket parallelism, stale checks, the empty
trigger commit, and the clean recovery path. No real or paid AGY call was
authorized or made.

## Reproducible #2316 audit

The following commands were run against the live PR and local Git objects.

| Check | Observed result |
| --- | --- |
| `gh pr view 2316 --json number,title,state,isDraft,mergeable,mergeStateStatus,headRefName,headRefOid,baseRefName,baseRefOid,body,commits,files,reviewDecision,statusCheckRollup,comments,reviews,url` | #2316 `OPEN` during audit; title `fpkg-providers-agy-package-latency`; base `main` at `7967378350e7c5aaa78cdc44df2e53f570d795c4`; head `fpkg-providers-agy-package-latency` at `d7c545090d4c2da3bf72c010ee033c1425429ad2`; API reported `MERGEABLE` and `BLOCKED`. |
| `git merge-base origin/main d7c545090d4c2da3bf72c010ee033c1425429ad2` | `deb077d8dbe121e6fba5316ef00636a42238de6f`. |
| `git merge-base 7967378350e7c5aaa78cdc44df2e53f570d795c4 d7c545090d4c2da3bf72c010ee033c1425429ad2` | `deb077d8dbe121e6fba5316ef00636a42238de6f`; the PR base was stale relative to current main. |
| `git log --oneline --reverse deb077d8dbe121e6fba5316ef00636a42238de6f..d7c545090d4c2da3bf72c010ee033c1425429ad2` | `66430639c0 test(providers/agy): eliminate duplicate process build and add t.Parallel to isolated tests`; `d7c545090d trigger CI`. |
| `git diff --name-status --stat origin/main...d7c545090d4c2da3bf72c010ee033c1425429ad2` | Four AGY test files: one added and three modified; 44 insertions and 5 deletions. |
| `git diff --check origin/main...d7c545090d4c2da3bf72c010ee033c1425429ad2` | Exit `0`; no whitespace errors. |
| `git merge-tree --write-tree origin/main d7c545090d4c2da3bf72c010ee033c1425429ad2` | Exit `0`, tree `27022423c8752dc5f43cda7b508cf583082c5aab`; no textual conflict reported. Mergeability was not treated as behavior proof. |
| `git diff origin/main...d7c545090d4c2da3bf72c010ee033c1425429ad2 -- tests/functional/providers/agy` | Complete 169-line diff: the helper, four top-level direct/golden `t.Parallel()` calls, and the two role-helper process-reuse edits. |
| `go test ./tests/functional/providers/agy -list '^Test'` | Exit `0`; 10 top-level identities listed, package discovery completed in `0.078s` after compilation. |

### PR body

The PR body was fetched by the command above and parsed as its authored JSON
payload: 26,512 UTF-8 bytes, SHA-256
`8884a4647d0a9a5409297b070fc9d9bc3c376bc5b3025042b4ad1ab71ec2224`, 14
acceptance criteria, one `FPKG-AGY` behavior lane, and one implementation
story. Its recorded project was “Reduce latency of the providers/agy
functional Go package.” The body proposed only
`tests/functional/providers/agy/**`, caller-process role initialization reuse,
invocation-local state, queued command results, and selective parallelism; it
excluded remote/paid calls, production/shared support, sibling packages,
generated contracts, Makefile/CI, public contracts, fixtures, sleeps, timeout
inflation, and weakened assertions.

The body’s baseline and scope claims were retained as historical context, not
accepted as current proof: it cited local samples `26.281s`, `25.853s`, and
`30.893s` (median `26.281s`) and required a later same-fidelity matrix and
cleanup proof. The body’s proposed solution used one reusable process per
role group and selective `t.Parallel()`; the latter is conditional under the
current c10 contract.

### PR comments and checks

Three automated PR comments were present before the disposition. Their
material facts were recorded without copying sensitive or oversized payloads:

| Comment | Result relevant to disposition |
| --- | --- |
| [Backend Lint](https://github.com/portpowered/you-agent-factory/pull/2316#issuecomment-5428128682) | Harness `FAILED` and explicitly untrustworthy: report missing/unparseable, zero checker inventory, empty `-jobs`; hosted tested SHA `153fab49ce56fe24e0a288a5c7cef079e6b9e5f7`. |
| [Backend Unit Coverage](https://github.com/portpowered/you-agent-factory/pull/2316#issuecomment-5428206101) | Reported 80.6% across 474 packages, two floor violations, and 11,343 observed top-level tests with zero failures; it also retained coverage holds, so it was not a clean current-head proof. |
| [Backend Functional Coverage](https://github.com/portpowered/you-agent-factory/pull/2316#issuecomment-5428237737) | The AGY package row was `8.941s`, but the report was for the old head and is historical timing only. |

The hosted run was
[32989185600](https://github.com/portpowered/you-agent-factory/actions/runs/32989185600),
head `d7c545090d4c2da3bf72c010ee033c1425429ad2`. The complete check inventory
was:

- **Success:** Classify Verification, Workflow Lint, Docs Reference, README,
  Frontend, Frontend Component, Frontend Coverage, Frontend Browser, Frontend
  Storybook, Backend Unit Coverage, Backend Functional Coverage, Backend Test
  Stability, Development Package / API Contract And Package, Development
  Package / Packaged Factories Package, Development Package / Model Providers
  Package, Development Package / Build API Candidate (No Publish), and
  Development Package / Build Packaged Factories Candidate (No Publish).
- **Failure:** Backend Lint, UI Backend Integration, and Verification Policy.
- **Skipped:** Development Package Workflow Behavior, Development Package /
  Prepare Protected Main Candidate, Resolve Release Tag, Publish and Verify
  Protected Main Candidate, Prepare Complete Public Package Candidate, Publish
  Tagged Release Candidate, Publish GitHub Release, Smoke Hosted Installer,
  Verify Go Install Surface, and Smoke Released Artifacts.

Changed-file history was inspected with `git log --follow --all` for all four
paths. The recovered commit is the only history entry for the added helper;
the existing files retain their earlier AGY construction/assertion history,
including the original role composition and process-boundary commits. No
changed-file history justified carrying the empty trigger commit forward.

## Recovered edit disposition

| Recovered edit | Classification | Current decision and evidence |
| --- | --- | --- |
| Add `assertAgyPackagedFactoryInstalled` and initialize the packaged Factory through the caller’s root-built process | **Recover** — compatible immutable construction reduction | Reimplement from current main. `support.InstallPackagedFactory` currently constructs a process for initialization and the role helper constructs another for invocation; `InitializeCustomerHomeWithProcess` already exposes the same public missing-Factory probe on a caller-owned process. The home, environment, working directory, Factory Session, Work, events, recording, and streams remain invocation-owned. |
| Add `t.Parallel()` to multimodal, ClipQA, structured JSON, and missing-file golden tests | **Conditional** — do not cherry-pick | Reintroduce only through frozen per-invocation routes after route uniqueness, ordering, recovery, and cleanup evidence. The four direct/golden cases remain `shareable-with-mock` in c01 P029, but their concurrency proof was absent from #2316. |
| Add `t.Parallel()` to conductor, native failure, timeout, and cancellation tests | **Conditional** — do not cherry-pick | Preserve the c01 `shareable-with-mock` classification and current adverse assertions; introduce concurrency only after invocation-owned success/error routes prove no result, event, or runner crossover. |
| Empty `trigger CI` commit `d7c545090d` | **Discard** | No product, test, or behavior value; it only triggered a stale run. |

## Current source and executable denominator

The current package is c01 `P029`. The c01 inventory records all nine offline
top-level tests as `shareable-with-mock` and `TestAgyLiveSmoke` as
`isolated-with-reason`. It also records the public witnesses as CLI, Work,
Factory Session, Factory Event, response stream, Provider edge, and (for live)
the real executable/credentials/quota/remote response.

`go test ./tests/functional/providers/agy -list '^Test'` returned these 10
top-level identities:

1. `TestAgyMultimodalGoldenThroughRootBuildProcess`
2. `TestAgyClipQAGoldenPassThroughRootBuildProcess`
3. `TestAgyStructuredJSONGoldenThroughRootBuildProcess`
4. `TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess`
5. `TestAgyLiveSmoke`
6. `TestAgyConductorSuccessThroughRootBuildProcess`
7. `TestAgyNativeFailureThroughRootBuildProcessIsSafe`
8. `TestAgyTimeoutFailureThroughRootBuildProcess`
9. `TestAgyCommandCancellationThroughRootBuildProcessIsCanonical`
10. `TestAgyProductionReviewRolesThroughRootBuildProcess`

The Go list command exposes top-level identities only, so the leaf denominator
was reconciled from source `t.Run` tables. There are nine offline top-level
tests and 25 offline leaves: four direct process leaves, five golden leaves,
and 16 production-role leaves. Together with the one live cell this is the
exact current 10/25/1 denominator described by the task packet.

## CASE-AGY reconciliation

The table maps every current leaf, the external live cell, and the later
planned route/recovery rows. “Controlled edge” means the real production
composition is retained and only `edges.Edges.ProviderCommandRunner` is
substituted. All current package rows use local-real root/services, temporary
test-owned files, and pinned AGY traces unless marked as the external live
gate.

| Case | Current source/leaf | Observable witness and dependency fidelity | Owned resources and later gate |
| --- | --- | --- | --- |
| `CASE-AGY-01` | `TestAgyConductorSuccessThroughRootBuildProcess` | One controlled AGY command, completed Work, final-only response, accepted dispatch, Provider Session; root.BuildProcess + Process.Execute + controlled ProviderCommandRunner. | Fixture/process/session/stream/runner; `GATE-FAIL-001`, later `GATE-ISO-001`. |
| `CASE-AGY-02` | `TestAgyNativeFailureThroughRootBuildProcessIsSafe` | Non-zero native/auth result; failed Work and Provider Session, no done Work, `/tmp/secret-key` and raw detail absent from serialized Factory Events. | Fixture/process/session/stream/runner; `GATE-FAIL-001`, `GATE-SECURITY-001`. |
| `CASE-AGY-03` | `TestAgyTimeoutFailureThroughRootBuildProcess` | Deadline error normalizes to timeout failure, at least one attempt, no partial success, Provider Session. | Fixture/process/session/stream/runner; `GATE-FAIL-001`, `GATE-CLEAN-001`. |
| `CASE-AGY-04` | `TestAgyCommandCancellationThroughRootBuildProcessIsCanonical` | Exactly one canceled command, failed Work, canonical cancellation diagnostic, Provider Session. | Fixture/process/session/stream/runner; `GATE-FAIL-001`, `GATE-CLEAN-001`. |
| `CASE-AGY-05` | `TestAgyMultimodalGoldenThroughRootBuildProcess/video-watch` | Real `clip-fixture.mp4` plus pinned stream trace; exact argv/workdir, usage, Provider Session, response evidence, Work, dispatch. | Fixture/media/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-06` | `TestAgyMultimodalGoldenThroughRootBuildProcess/groundtruth-video` | Real groundtruth media plus pinned trace; duration, resolution, frame/rate, PHASE text, cut, audio, usage, Work, dispatch. | Fixture/media/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-07` | `TestAgyClipQAGoldenPassThroughRootBuildProcess` | Real clip and ClipQA stream trace; visual/audio response evidence, one command, Provider Session, completed Work, accepted dispatch. | Fixture/media/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-08` | `TestAgyStructuredJSONGoldenThroughRootBuildProcess` | Pinned JSON and authored sentiment schema; positive/0.98 output, exact schema argv, Provider Session, Work, dispatch. | Fixture/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-09` | `TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess` | Exit-zero refusal retains missing-file response, produces one failed Work and no done Work, with failed/rejected actionable dispatch. | Fixture/media/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-10` | `TestAgyProductionReviewRolesThroughRootBuildProcess/cold-watch-complete-report-contract` | Named cold-watch Factory returns all report sections and pass recommendation; one command and accepted Work/Event dispatch. | Home/workdir/media/recording/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-11` | `.../cold-watch-incomplete-real-traces-fail/video-watch` | Incomplete real trace fails with no primary result, retained provider evidence, non-empty output-contract diagnostic. | Home/workdir/media/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-12` | `.../cold-watch-incomplete-real-traces-fail/groundtruth-video` | Same failed/no-primary contract, retaining PHASE, cut, and audio provider evidence. | Home/workdir/media/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-13` | `.../missing-file-fails-work-after-provider-success` | Named cold-watch missing media fails with no primary result, diagnostic, refusal evidence, and failed events. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-14` | `.../clip-qa-structured-pass-with-audio-evidence` | Named ClipQA primary verdict is pass/noise/no-speech, reason arrays are present and empty, confidence is bounded, dispatch accepted. | Home/workdir/media/recording/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-15` | `.../clip-qa-structured-reroll-is-accepted` | Valid inspected reroll completes with deviation and incomplete action, preserving the existing accepted-reroll contract. | Home/workdir/media/recording/process/session/stream/runner; `GATE-SPINE-001`. |
| `CASE-AGY-16` | `.../clip-qa-semantic-invalid-results-fail/confidence below zero` | Confidence `-0.01` fails with no primary result, actionable diagnostic, failed dispatch. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-17` | `.../clip-qa-semantic-invalid-results-fail/confidence above one` | Confidence `1.01` follows the canonical failure and no-primary-result contract. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-18` | `.../clip-qa-semantic-invalid-results-fail/pass with incomplete action` | Pass verdict with incomplete action fails and exposes no primary result. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-19` | `.../clip-qa-semantic-invalid-results-fail/pass with specification deviation` | Pass verdict with a specification deviation fails and exposes no primary result. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-20` | `.../clip-qa-semantic-invalid-results-fail/pass with temporal artifact` | Pass verdict with a temporal artifact fails and exposes no primary result. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-21` | `.../clip-qa-semantic-invalid-results-fail/pass with unexpected speech` | Pass verdict with unexpected speech fails and exposes no primary result. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-22` | `.../clip-qa-semantic-invalid-results-fail/reroll with provider failure status` | Reroll carrying provider error status fails and exposes no primary result. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-23` | `.../clip-qa-missing-file-fails-work` | Missing media produces failed invocation, exact command, no primary result, actionable failed dispatch. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-24` | `.../clip-qa-schema-invalid-result-fails-work` | Wrong structured trace fails with no primary result, actionable diagnostic, and failed dispatch. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-25` | `.../clip-qa-provider-failure-fails-work` | Empty stdout with exit code 17 fails after an attempt, with no primary result and actionable diagnostic. | Home/workdir/recording/process/session/stream/runner; `GATE-FAIL-001`. |
| `CASE-AGY-26` | `TestAgyLiveSmoke` with `YOU_AGY_LIVE_SMOKE` unset | Explicit skip is the default; no production AGY runner is selected. | No live process/home/media acquired; `GATE-LIVE-001`. |
| `CASE-AGY-27` | External `provider-release-gate` only | Explicit operator gate may run one real call and seek `TRACE_OK`; no c10 evidence claims availability. | External credentials/executable/quota; not run in c10. |
| `CASE-AGY-28` | Not present before migration; planned frozen-route boundary | Duplicate normalized routes must reject before build; unknown routes fail closed with no unintended provider call. | Route registry/process; `GATE-SPINE-001`, task 002. |
| `CASE-AGY-29` | Not present before migration; planned recovery boundary | Empty exit-zero invocation must not fabricate a result; following valid invocation succeeds without crossed calls/events/routes. | Route/process/session/recording; `GATE-FAIL-001`, `GATE-ISO-001`, task 003. |
| `CASE-AGY-30` | Not present before migration; planned concurrency boundary | Distinct frozen routes under `-parallel=2` receive only their own output and ordered terminal events, without sibling teardown. | Shared process/route/runner/streams; `GATE-ISO-001`, task 003. |
| `CASE-AGY-31` | Not present before migration; planned cleanup ledger | Success, native failure, timeout, cancellation, malformed, missing, empty, and early-exit paths release process/session/stream/route/recording/media/workspace/home resources to zero. | Every listed owner/resource; `GATE-CLEAN-001`, task 003. |
| `CASE-AGY-32` | Not present before migration; planned idempotent recovery boundary | Failed invocation followed by a fresh valid identity completes once with distinct Work/session/recording identities and no duplicate final event. | Process/session/recording/route; `GATE-ISO-001`, task 003. |

## Before resource and topology contract

### Current executable spine

The current offline path is:

`go test` -> per-leaf `support.BuildProcess`/`root.BuildProcess` ->
`Process.Execute` -> Factory Session/Runtime -> Workers -> Providers AGY
adapter -> controlled `ProviderCommandRunner` -> local media/pinned trace ->
public Work, Factory Events, response events, Provider Session and recording
assertions -> process/test cleanup.

The current production-role helper adds a second path before invocation:

`support.InstallPackagedFactory` -> `initializeCustomerHome` -> a throwaway
`BuildProcess` -> public missing-Factory probe -> packaged Factory materialized;
then the role helper builds a new process and executes the named Factory.

### Static process count

The source-estimated current offline count is **41 root builds**:

| Family | Leaves | Current builds per leaf | Total |
| --- | ---: | ---: | ---: |
| Direct process harness (`CASE-AGY-01..04`) | 4 | 1 | 4 |
| Golden media/JSON (`CASE-AGY-05..09`) | 5 | 1 | 5 |
| Production review roles (`CASE-AGY-10..25`) | 16 | 2 (`InstallPackagedFactory` plus invocation) | 32 |
| **Offline total** | **25** |  | **41** |

`TestAgyLiveSmoke` is a separate production process only when explicitly
enabled and contributes zero default builds/calls. The c10 target remains the
PRD target of one package-owned offline process and zero live processes by
default, subject to replanning if characterization finds a genuine
process-scoped exception.

### Ownership ledger before structural work

| Resource | Current owner/acquisition | Current observation/cleanup | Structural constraint |
| --- | --- | --- | --- |
| Root application process and services | `support.BuildProcess` in each run; role initialization additionally calls it through `InstallPackagedFactory` | `support.CleanupProcess` registers `Close` with a bounded context; test cleanup handles temporary directories. | Only immutable construction may become shared; lifecycle state must remain invocation-safe. |
| Factory Session, Work, Factory Events, response stream | `RunFactoryToCompletion...` or direct `Process.Execute` and public recording/API observations | Support waits for public terminal observations, reads event/response history, and closes the process; role recordings are read before return. | No canonical state may cross invocations. |
| Provider command route and call accounting | One `ProviderCommandRunner` per current leaf, selected through `serviceedges.Edges` | `CallCount`/last request assertions and test-owned runner state. | Frozen normalized route selection must be explicit before shared construction. |
| Home and named Factory root | `t.TempDir` plus `HOME`/`USERPROFILE`; current `InstallPackagedFactory` initializes through a separate process. | Temporary home is removed by testing cleanup; public missing-Factory probe verifies installation. | Home/environment must be invocation-local even when the process is shared. |
| Working directory and media | `t.TempDir`; real MP4 files copied from `testdata`; missing-file paths intentionally absent. | Testing cleanup removes the directory; command assertions verify `--add-dir`, path and workdir. | Media and path keys must not be mutable shared state. |
| Recording artifact | Role helpers use a `t.TempDir` path and read `events` from the replay JSON. | File is test-owned and removed by `t.TempDir`; read happens before process teardown. | Recording identity/content remains invocation-local. |
| OS process/server/handles | Application lifecycle and external command boundary; live smoke selects the production subprocess runner only under its gate. | Existing process close and test cleanup are present; zero-residue proof is later `GATE-CLEAN-001`. | Do not share genuine executable, startup, environment, or handle-scoped behavior without a proven route. |

The before ledger is ownership characterization, not a claim that every leaf
materializes every resource. It also does not claim a zero-residue census;
that is explicitly deferred to `GATE-CLEAN-001`.

## Verification and remaining edges

`GATE-DISP-001` is **PASS** for this story because the external PR audit,
edit-level disposition, closure, current test listing, 10/25/1 denominator,
CASE-AGY reconciliation, ownership ledger, and 41-build before topology are
all recorded above. The highest feasible level for this bounded enabler was
current external PR/source audit plus executable test discovery; no runtime
structural change was authorized.

Not proved here:

- post-change success/media/role parity -> `GATE-SPINE-001`;
- adverse behavior, ordering, recovery, and classification ->
  `GATE-FAIL-001`/`GATE-ISO-001`;
- deterministic zero-resource cleanup -> `GATE-CLEAN-001`;
- default live skip and zero remote calls -> `GATE-LIVE-001`;
- final package and clean-room validation -> `GATE-PKG-001`/`VAL-001`;
- PR-host package direction and current-head CI -> `GATE-PR-FUNCTIONAL` /
  review-owned `REV-CI-001`;
- real AGY credentials, executable, model, quota, and response -> external
  `provider-release-gate`.

