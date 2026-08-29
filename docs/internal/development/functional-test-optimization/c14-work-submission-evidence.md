# C14 Work submission optimization evidence

Status: stories `fto-c14-pkg-work-submission-001`,
`fto-c14-pkg-work-submission-002`, and `fto-c14-pkg-work-submission-003` are
**PASS** for implementation-stage local evidence. This ledger records the
unchanged characterization, optimized package evidence, clean-room result, and
remaining PR CI/merge ownership; it does not claim terminal PR CI or merge.

## Scope and authority

- PRD: `prd.json`, project `Reduce intrinsic wall time of Work submission functional tests`.
- Behavior lane: `BEH-SUBMISSION-PERF`.
- Story: `fto-c14-pkg-work-submission-003` (including the retained evidence from
  stories `001` and `002`).
- Source-plan authority: `null` in the PRD; no operator amendment is present.
- Dependency fidelity: local real production wiring with controlled provider and
  inference edges already used by the package.
- Authorized change for this refresh: the evidence ledger; the implementation
  review corrections are limited to the five submission-package test files
  recorded at final head `d6688af10a`. No production, shared-support,
  contract, generated, workflow, or sibling-package file was edited.
- Cost and network: controlled local fixtures only; 0 remote product calls and
  `$0` paid cost.

## Baseline identity

- Source commit: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`
- Branch: `fto-c14-pkg-work-submission`
- Environment: `go version go1.25.0 windows/amd64`; Windows 11 Home build
  `26200`; `amd64`; 24 logical processors.
- Worktree status before the baseline invocations: clean.
- Host observation: the PRD identifies this shared host as compute-saturated.
  The three unchanged wall times ranged from `33.460s` to `122.490s` (3.66x),
  while package-reported time ranged from `31.063s` to `116.815s` (3.76x).
  A separate one-second read-only total-processor sample was `38.6%`; it is
  retained as a point observation, not a quiet-host or repeatability claim.
- No quiet-host, low-variance, or fixed absolute-time prerequisite was imposed.

## GATE-BASELINE — unchanged three-run denominator

Each row below was a separate invocation of the exact named package command:

```text
go test ./tests/functional/work/submission/... -count=1
```

| Run | Commit | Result / exit | Package-reported time | PowerShell wall time |
| ---: | --- | --- | ---: | ---: |
| 1 | `fea2e30a499384182d2fabe7038767e3c2f9c5e5` | PASS / `0` | `45.000s` | `59.2423305s` |
| 2 | `fea2e30a499384182d2fabe7038767e3c2f9c5e5` | PASS / `0` | `31.063s` | `33.4604395s` |
| 3 | `fea2e30a499384182d2fabe7038767e3c2f9c5e5` | PASS / `0` | `116.815s` | `122.4897677s` |

The unchanged wall-time median is **`59.2423305s`**. All three invocations
passed, so there is no baseline behavioral failure to classify. The timing spread
is environmental diagnostic evidence and is not silently discarded.

The default test-list procedure was:

```text
go test ./tests/functional/work/submission/... -list '^Test'
```

It exited `0` and emitted 22 top-level test identities. The tagged inventory
procedure was:

```text
go test -tags=functionallong ./tests/functional/work/submission/... -list '^Test'
```

It exited `0` and emitted the same 22 identities plus
`TestWorkBatchPublicShapeStaysAlignedAcrossWatchedFileAndHTTP` and
`TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests`, for 24
tagged-build identities. The grouped tests expand those identities into the 31
case rows in the assertion ledger below.

## GATE-PROFILE — diagnostic run and source accounting

The diagnostic command was run once against the same unchanged commit:

```text
go test -json ./tests/functional/work/submission/... -count=1
```

Observed result: PASS, exit `0`; package-reported time `197.170s`; measured
PowerShell wall time `200.5781658s`. The JSON output was read transiently for
top-level and named-subtest elapsed values and was not committed.

### Per-case elapsed distribution

For ungrouped default cases, the value is the Go JSON top-level test elapsed
value. For grouped cases, the value is the named subtest elapsed value. These
are distribution diagnostics, not portable performance thresholds.

| Case | Current test or subtest | Elapsed | Build |
| --- | --- | ---: | --- |
| CASE-SUB-001 | `TestAPIPOSTSubmitAndQueryWork` | `7.19s` | default |
| CASE-SUB-002 | `TestAPIBatchUpsertAcceptsWorksContent` | `5.12s` | default |
| CASE-SUB-003 | `TestCLIWorkTypeNameReachesLiveAPIHandler` | `4.91s` | default |
| CASE-SUB-004 | `TestAPISubmitBatchThenListAndGetWork` | `3.18s` | default |
| CASE-SUB-005 | `TestAPIUpsertWorkRequestUsesCanonicalIdentity` | `5.55s` | default |
| CASE-SUB-006 | `TestAPIUnknownWorkReturnsTypedNotFound` | `17.30s` | default |
| CASE-SUB-007 | `TestWorkBatchAcceptsInlineFileAndStdinShapes/inline` | `0.52s` | default |
| CASE-SUB-008 | `TestWorkBatchAcceptsInlineFileAndStdinShapes/file` | `0.45s` | default |
| CASE-SUB-009 | `TestWorkBatchAcceptsInlineFileAndStdinShapes/stdin` | `0.28s` | default |
| CASE-SUB-010 | `TestWorkBatchSelectsDefaultAndExplicitWorkTypes/default` | `0.33s` | default |
| CASE-SUB-011 | `TestWorkBatchSelectsDefaultAndExplicitWorkTypes/explicit` | `0.30s` | default |
| CASE-SUB-012 | `TestWorkBatchRejectsUnknownTypeWithoutPartialMutation/unknown_type` | `0.27s` | default |
| CASE-SUB-013 | `TestWorkBatchRejectsUnknownTypeWithoutPartialMutation/mixed_batch_no_partial_submit` | `0.22s` | default |
| CASE-SUB-014 | `TestBlockedDispatchConcurrentBatchIngressRegression` | `5.05s` | default |
| CASE-SUB-015 | `TestWorkBatchDependencyOrderingNormalizesRuntimeWork` | `5.56s` | default |
| CASE-SUB-016 | `TestAPIBatchUpsertRejectsOversizedWorkAtomically` | `12.77s` | default |
| CASE-SUB-017 | `TestAPIBatchUpsertAcceptsPayloadAtInclusiveLimit` | `14.78s` | default |
| CASE-SUB-018 | `TestAPIStageAndSubmitFileCreatesExpectedWork` | `13.60s` | default |
| CASE-SUB-019 | `TestAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission` | `8.66s` | default |
| CASE-SUB-020 | `TestAPISubmitWorkRejectsEmptyStructuredSubmission` | `1.04s` | default |
| CASE-SUB-021 | `TestAPISubmitWorkAcceptsOrderedTextSubmission` | `1.68s` | default |
| CASE-SUB-022 | `TestAPISubmitWorkAcceptsCanonicalContentParts` | `1.82s` | default |
| CASE-SUB-023 | `TestAPISubmitWorkAcceptsMixedTextAndImageOnSupportedRunner` | `1.54s` | default |
| CASE-SUB-024 | `TestAPISubmitWorkRejectsMixedTextAndImageOnUnsupportedRunner` | `1.98s` | default |
| CASE-SUB-025 | `TestAPISubmitWorkRejectsForgedStructuredFileReference` | `1.40s` | default |
| CASE-SUB-026 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/direct_POST_and_idempotent_PUT` | `17.38s` | default |
| CASE-SUB-027 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/startup_work_file_batch` | `14.57s` | default |
| CASE-SUB-028 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/file_watcher_non_batch_JSON` | `15.16s` | default |
| CASE-SUB-029 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/cron_internal_time_work` | `13.39s` | default |
| CASE-SUB-030 | `TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests` | not run in default profile | `functionallong` |
| CASE-SUB-031 | `TestWorkBatchPublicShapeStaysAlignedAcrossWatchedFileAndHTTP` | not run in default profile | `functionallong` |

The diagnostic top-level elapsed values for grouped parents were
`7.89s`, `9.31s`, `6.26s`, and `60.50s` respectively; named subtest values
above are used for the case rows so group setup is not falsely assigned to one
case. The profile shows the four-case legacy group alone consumed `60.50s` and
its four isolated server lifecycles each consumed `13.39s`–`17.38s`.

### Root, process, fixture, wait, and sleep topology

The read-only source accounting procedure was:

```text
rg -n 'StartFunctionalAPIServer\(|newRootProcessHarness\(|support\.ExecuteProcess\(|CommandContext\(' tests/functional/work/submission
rg -n 'WaitFor|waitFor|time\.Sleep|time\.After|WithTimeout|BlockUntilContext' tests/functional/work/submission
```

The default-build topology is:

| Expensive operation | Static source accounting | Runtime default count | Source basis and witnesses |
| --- | ---: | ---: | --- |
| Functional server/root construction | 24 call sites | **25** | `http_test.go` 6, `structured_submission_test.go` 7, `batch_inputs_test.go` 5, `legacy_unary_test.go` 4, `stage_and_submit_test.go` 1, and `payload_size_http_test.go` 1 helper call site invoked by both CASE-SUB-016 and CASE-SUB-017. Each `StartFunctionalAPIServer` builds one `root.BuildProcess` process. |
| One-shot CLI root construction | 4 call sites | **8** | CASE-SUB-007..009 execute 3 `CommandContext` invocations, CASE-SUB-010..011 execute 2, CASE-SUB-012..013 execute 2, and CASE-SUB-003 calls `support.ExecuteProcess` once. The non-reusable harness builds a root per invocation. |
| Total default root constructions | — | **33** | 25 server roots plus 8 ordinary CLI roots. This is the removable process/setup topology owned by story 002. |
| Sequential grouped fixture cohorts | 4 `t.Run` groups | 4 groups / 7 CLI invocations | Three batch CLI groups serialize 3, 2, and 2 commands over one server each; the legacy group serializes four subtests, each with its own server. No `t.Parallel` call exists in the owned package. |

The wait/synchronization accounting is:

- Every one of the 25 server instances waits for the harness listener readiness
  boundary. Nine of those instances (the payload helper is invoked twice) also
  request service-mode runtime readiness. These waits are inherited from shared
  support and are recorded as setup cost, not changed in story 001.
- The package’s public-projection helpers make 11 actual calls to shared
  `support.WaitForObservation`: five trace/place waits (CASE-SUB-019 and
  CASE-SUB-021..024), four completed-ID waits (CASE-SUB-002, CASE-SUB-015,
  CASE-SUB-026..027), and two completed-type waits (CASE-SUB-003 and
  CASE-SUB-028). The helper’s documented 10ms observation interval is polling
  for asynchronously committed public projections.
- `support.WaitForTerminalStatus` is used by CASE-SUB-001 and CASE-SUB-026 in
  the default run; tagged CASE-SUB-030 uses it twice for live and replay
  processes. CASE-SUB-029 uses a fake-clock waiter and a channel receive rather
  than a sleep. CASE-SUB-015 consumes a Factory Event stream with a bounded
  deadline rather than sleeping.
- There is exactly one package-local `time.Sleep`: the 10ms loop in
  `waitForServiceModeSessionActive` at `batch_inputs_test.go:309-322`, used by
  CASE-SUB-014. This is the direct deterministic-synchronization optimization
  target. The wait observes `RUNNING` and `InFlightCount`; the current source
  has no event signal for that exact readiness condition.
- The package has one package-local `time.After` timeout channel in the cron
  record helper, three 60-second CLI invocation contexts covering seven
  invocations across three grouped tests, and one one-second fake-clock waiter
  context. The ten-second public observation/status arguments, 60-second CLI contexts, and one-second cron
  ceiling are failure bounds; the successful profile values returned before
  those ceilings. They are not evidence that timeout padding is part of the
  success path.

The dominant removable topology is therefore repeated root/process and fixture
startup, with one package-owned polling sleep in the blocked-dispatch case. The
profile does not justify parallelizing cases or sharing mutable Factory state;
the sequential identity and cleanup constraints remain for story 002.

## GATE-ASSERTIONS — unchanged pre-edit witness inventory

Every PRD matrix case has a concrete pre-edit test or subtest and observable
assertions. Helper assertions are included where they supply the public
behavioral proof; no row is a source-inventory placeholder.

| Case | Current test/subtest and source | Observable assertions retained before editing |
| --- | --- | --- |
| CASE-SUB-001 | `TestAPIPOSTSubmitAndQueryWork` (`http_test.go:35-51`, helpers `:445-494`) | POST `/work` returns 201 and a trace; terminal status is observed; GET `/work` returns exactly one Work with type `task`, state `complete`, and terminal state type. |
| CASE-SUB-002 | `TestAPIBatchUpsertAcceptsWorksContent` (`http_test.go:55-129`) | PUT batch returns 201; response request/trace IDs and one accepted Work ID are present; Work completes; exactly two canonical text parts decode and retain their exact order/text; endpoint coverage marker is emitted. |
| CASE-SUB-003 | `TestCLIWorkTypeNameReachesLiveAPIHandler` (`http_test.go:133-176`) | Root-process CLI execution succeeds; trimmed submitted name is `cli-live-api-name`; public Work type is `task`; Work reaches `complete`. |
| CASE-SUB-004 | `TestAPISubmitBatchThenListAndGetWork` (`http_test.go:182-264`) | PUT response preserves request ID, non-empty trace, one fixed Work ID/name; list finds the exact ID/name and type; GET preserves ID/name/type. |
| CASE-SUB-005 | `TestAPIUpsertWorkRequestUsesCanonicalIdentity` (`http_test.go:270-346`) | First PUT preserves fixed request/trace/Work identities; repeated PUT preserves request, trace, and Work ID; GET returns the canonical ID and name. |
| CASE-SUB-006 | `TestAPIUnknownWorkReturnsTypedNotFound` (`http_test.go:352-443`) | GET missing Work returns 404 with JSON; error family is `NOT_FOUND`; code is `NOT_FOUND`; message is non-empty and says not found; body is not a success Work payload. |
| CASE-SUB-007 | `TestWorkBatchAcceptsInlineFileAndStdinShapes/inline` (`batch_inputs_test.go:80-89`) | Inline CLI batch succeeds; JSON acknowledgment contains request ID, trace ID, `workCount:1`, work name/type; listed Work matches returned identity. |
| CASE-SUB-008 | `TestWorkBatchAcceptsInlineFileAndStdinShapes/file` (`batch_inputs_test.go:91-105`) | File-sourced CLI batch succeeds; the same request/trace/count/name/type acknowledgment and public name/ID listing are present. |
| CASE-SUB-009 | `TestWorkBatchAcceptsInlineFileAndStdinShapes/stdin` (`batch_inputs_test.go:107-116`) | Stdin-sourced CLI batch succeeds; the same request/trace/count/name/type acknowledgment and public name/ID listing are present. |
| CASE-SUB-010 | `TestWorkBatchSelectsDefaultAndExplicitWorkTypes/default` (`batch_inputs_test.go:135-153`) | CLI batch without `workTypeName` succeeds; decoded acknowledgment has one Work and identities; public listing contains the Work with default type `task`. |
| CASE-SUB-011 | `TestWorkBatchSelectsDefaultAndExplicitWorkTypes/explicit` (`batch_inputs_test.go:155-174`) | CLI batch with explicit type succeeds; decoded acknowledgment has one Work and identities; public listing contains the Work with explicit type `review`. |
| CASE-SUB-012 | `TestWorkBatchRejectsUnknownTypeWithoutPartialMutation/unknown_type` (`batch_inputs_test.go:196-207`) | CLI returns an error; output has HTTP 400 and `BAD_REQUEST` code/family; no success acknowledgment markers occur; rejected Work is absent; list count stays at baseline. |
| CASE-SUB-013 | `TestWorkBatchRejectsUnknownTypeWithoutPartialMutation/mixed_batch_no_partial_submit` (`batch_inputs_test.go:209-223`) | Mixed valid/unknown batch returns the same typed rejection with no acknowledgment; both sibling names are absent; list count is unchanged, proving atomicity. |
| CASE-SUB-014 | `TestBlockedDispatchConcurrentBatchIngressRegression` (`batch_inputs_test.go:229-307`, wait helper `:309-322`) | POST creates a trace; service mode is RUNNING with in-flight work; PUT returns fixed request/Work identities; one canonical Work Request event exists; list and GET see the Work while blocked; replay preserves request/trace and event identity; release is not observed before assertions; controlled release then permits cleanup. |
| CASE-SUB-015 | `TestWorkBatchDependencyOrderingNormalizesRuntimeWork` (`batch_inputs_test.go:365-441`, event assertions `:443-520`) | PUT response has request/trace and two exact submitted Works; replay response is identical; both Work types complete; each appears exactly once; one batch Work Request and one dependency relation have exact payloads; second dispatch sequence follows first terminal sequence. |
| CASE-SUB-016 | `TestAPIBatchUpsertRejectsOversizedWorkAtomically` (`payload_size_http_test.go:32-79`) | Mixed oversized batch returns 400 `BAD_REQUEST`; message reports request/work and 65,537 versus 65,536 bytes without payload content; list count and both names remain absent; no request/Work/dispatch public observation references the rejected identities. |
| CASE-SUB-017 | `TestAPIBatchUpsertAcceptsPayloadAtInclusiveLimit` (`payload_size_http_test.go:84-120`) | Exactly 65,536-byte payload returns 201; response preserves request and fixed Work ID; list contains the Work; one canonical Work Request event has the request/Work/type. |
| CASE-SUB-018 | `TestAPIStageAndSubmitFileCreatesExpectedWork` (`stage_and_submit_test.go:25-97`, image assertions `:157-205`) | Staging returns 201, non-empty backend reference/URL, exact file name/media type; POST returns trace and Work ID; GET preserves name/type and one image part with image type, staged URL/type, file-name metadata, and `submissionItemType=image`. |
| CASE-SUB-019 | `TestAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission` (`structured_submission_test.go:17-50`) | POST returns trace; Work reaches complete; list has exactly one Work with submitted name/type; content is nil or empty. |
| CASE-SUB-020 | `TestAPISubmitWorkRejectsEmptyStructuredSubmission` (`structured_submission_test.go:54-73`) | A sole whitespace text item is rejected with HTTP 400. |
| CASE-SUB-021 | `TestAPISubmitWorkAcceptsOrderedTextSubmission` (`structured_submission_test.go:77-115`) | Work completes for the returned trace; exactly two projected text parts decode; first retains `Alpha ` including its space and second is `Beta`. |
| CASE-SUB-022 | `TestAPISubmitWorkAcceptsCanonicalContentParts` (`structured_submission_test.go:119-157`) | Canonical POST content completes for the returned trace; exactly two projected text parts decode; order and whitespace are `Alpha ` then `Beta`. |
| CASE-SUB-023 | `TestAPISubmitWorkAcceptsMixedTextAndImageOnSupportedRunner` (`structured_submission_test.go:161-216`) | Staged mixed submission completes through a Codex-capable controlled runner; projected response has exactly one text part equal to `Done. COMPLETE`; it does not echo submitted request text. |
| CASE-SUB-024 | `TestAPISubmitWorkRejectsMixedTextAndImageOnUnsupportedRunner` (`structured_submission_test.go:220-266`) | Claude-capability mixed submission reaches `failed`; provider command runner call count remains zero, proving rejection precedes subprocess launch. |
| CASE-SUB-025 | `TestAPISubmitWorkRejectsForgedStructuredFileReference` (`structured_submission_test.go:270-295`) | Forged staged reference/URL structured submission returns HTTP 400. |
| CASE-SUB-026 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/direct_POST_and_idempotent_PUT` (`legacy_unary_test.go:26-82`) | Direct POST returns trace and reaches terminal status; repeated canonical PUT preserves trace; Work completes; exactly one canonical batch Work Request event has request/Work/type. |
| CASE-SUB-027 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/startup_work_file_batch` (`legacy_unary_test.go:30-32`, helper `:84-113`) | Startup `--work` file is consumed; fixed Work reaches complete; one canonical Work Request event preserves request/Work/type. |
| CASE-SUB-028 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/file_watcher_non_batch_JSON` (`legacy_unary_test.go:34-36`, helper `:115-139`) | Non-batch watcher JSON is accepted; a `task` Work completes; one canonical Work Request event is attributed to the `non-batch` Work name and `task` type. |
| CASE-SUB-029 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/cron_internal_time_work` (`legacy_unary_test.go:38-40`, helper `:141-173`) | Fake clock exposes one waiter; one-minute advance yields a submission with source `external-submit`, system-time Work type, exact cron workstation and nominal-time tags; canonical batch Work Request event includes the Work ID. |
| CASE-SUB-030 | `TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests` (`batch_inputs_long_test.go:27-87`, build tag `functionallong`) | Record-phase PUT preserves request and Work IDs; recorded event has `external-submit`, one Work, zero relations; replay reaches terminal status and reproduces one canonical Work Request event with request/Work/type. |
| CASE-SUB-031 | `TestWorkBatchPublicShapeStaysAlignedAcrossWatchedFileAndHTTP` (`batch_boundary_test.go:43-107`, build tag `functionallong`) | Watched-file and HTTP paths each produce the exact expected request/source, sorted child/parent/prerequisite Work IDs/types/states/traces, and sorted DEPENDS_ON/PARENT_CHILD relations; each equals expected and both paths equal each other. |

The inventory is the same pre-edit contract that story 002 must retain or
strengthen. CASE-SUB-030 and CASE-SUB-031 are intentionally not claimed as
executed by the default baseline; their tagged source ownership and assertions
are explicit above and their execution belongs to GATE-LONG in story 003 when
reachable.

## Story-001 evidence conclusion

| Criterion | Result | Evidence |
| --- | --- | --- |
| Unchanged three-run package baseline | PASS | GATE-BASELINE table: three exit-0 runs on the same commit; wall median `59.2423305s`. |
| Diagnostic topology and timing profile | PASS | GATE-PROFILE JSON run plus source accounting: 25 server roots, 8 one-shot CLI roots, 11 public-projection observation waits, and one package-local polling sleep. |
| Complete CASE-SUB-001..031 assertion inventory | PASS | GATE-ASSERTIONS table maps every case to a current test/subtest and observable assertions; tagged rows are not overclaimed as default execution. |
| Baseline failure classification | PASS | No behavioral failure occurred; timing variance is recorded as environmental diagnostic evidence. |

Remaining edges are owned by later stories: harness optimization and focused
equivalence (`fto-c14-pkg-work-submission-002`), final median/race/tagged/clean
room/PR handoff (`fto-c14-pkg-work-submission-003`), remote PR CI (`GATE-PR-CI`),
and merge (`GATE-DELIVERY`).

## Story-003 final local validation

The final implementation, including the review corrections, was tested from a
clean worktree at head
`d6688af10a5f66e67944fb1600fe13f50aec31de`. The ledger refresh does not alter
the package artifact or its test sources. Environment was
`go version go1.25.0 windows/amd64`, Windows build `26200`, `amd64`, with the
shared compute-saturated host described above. All validation used local
production wiring and controlled provider/command-runner edges; remote product
calls and paid validation remained at `0` and `$0`.

The five review findings addressed at this head were: direct top-level
endpoint evidence placement, request-batch boundary imports, real Codex
provider wiring in the affected tests, removal of the unused completion waiter,
and replacement of stale final-head evidence.

### GATE-FINAL-MEDIAN — optimized three-run denominator

Each row is a separate invocation of the exact named package command:

```text
go test ./tests/functional/work/submission/... -count=1
```

| Run | Commit | Result / exit | Package-reported time | PowerShell wall time |
| ---: | --- | --- | ---: | ---: |
| 1 | `d6688af10a5f66e67944fb1600fe13f50aec31de` | PASS / `0` | `29.501s` | `35.063s` |
| 2 | `d6688af10a5f66e67944fb1600fe13f50aec31de` | PASS / `0` | `36.655s` | `42.981s` |
| 3 | `d6688af10a5f66e67944fb1600fe13f50aec31de` | PASS / `0` | `40.470s` | `46.494s` |

The final clean-worktree wall-time median is **`42.981s`**, versus the unchanged
baseline median **`59.2423305s`**, for a **`27.45%` reduction**. The
package-reported median is `36.655s` versus the unchanged `45.000s` median, a
`18.54%` reduction. All three runs passed. The final review correction changed
the provider fixture path and therefore raised the measured package time, but
the current package still materially follows the proven root-reuse and fixture
grouping optimization. The wall spread is retained as host-contention
diagnostic evidence; package-level behavior remains the primary verdict.

The final fidelity-adjusted median is below the PRD's 40% target. The permitted
profile-backed-floor alternative is satisfied after the one bounded optimization
pass: current-head source accounting retains 12 default server/root call sites
instead of the baseline 25 server plus 8 ordinary CLI roots, zero one-shot CLI
root patterns, zero package-local sleeps, and the deterministic controlled-edge
release for CASE-SUB-014. The current three-run measurements are the
fidelity-adjusted floor for the full provider-wired package; no assertion,
scope, or timeout weakening was used to obtain it.

### GATE-PROFILE — post-pass distribution and topology

The diagnostic command was run once against the optimized implementation
before the final rebase:

```text
go test -json ./tests/functional/work/submission/... -count=1
```

It passed with exit `0`, package-reported time `22.777s`, and measured
PowerShell wall time `26.6141s`. The transient JSON artifact was not committed.
The grouped package profile reported these principal serialized cohorts:

| Cohort | Package profile elapsed |
| --- | ---: |
| `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly` | `6.82s` |
| `TestStructuredSubmissionSimplePipeline` | `1.98s` |
| `TestWorkBatchDependencyOrderingNormalizesRuntimeWork` | `2.00s` |
| `TestPayloadSizeHTTPSubmission` | `2.05s` |
| `TestBlockedDispatchConcurrentBatchIngressRegression` | `1.58s` |
| `TestWorkBatchCLIIngress` | `1.50s` |
| `TestWorkBatchHTTPSubmission` | `1.17s` |

Read-only owned-source accounting at the final code/test head found 12
default-build `StartFunctionalAPIServer` call sites, 16 including the two tagged
source files, zero `newRootProcessHarness`/`support.ExecuteProcess`/
`CommandContext` CLI-root patterns, and zero package-local `time.Sleep` calls.
The ordinary CLI cases therefore execute through the live server root. Shared
public projection observation helpers remain because they prove asynchronously
committed Work projections; the package adds no polling or timeout padding.

### GATE-PACKAGE, GATE-RACE, and GATE-LONG

The named default package command passed in the final clean-worktree three-run
set above. The supported race command also passed at the final head:

```text
go test -race ./tests/functional/work/submission/... -count=1
```

Result: PASS / exit `0`, package-reported time `60.887s`; no race report was
emitted. The tagged command passed as well at the same clean head:

```text
go test -tags=functionallong ./tests/functional/work/submission/... -count=1
```

Result: PASS / exit `0`, package-reported time `35.175s`. This executed the two
tagged witnesses and the shared optimized helpers. These gates prove local
functional behavior and detected race safety for the changed package paths;
they do not prove terminal PR CI or merge.

The repository validators also passed at this head:

```text
go run ./cmd/functionalscenarioproject -check contracts/functional-scenarios.json
go run ./cmd/functionalboundarycheck
```

The scenario validator emitted `154 reviewed scenarios are current`; the
boundary validator exited `0` without diagnostics.

### GATE-ASSERTIONS — post-edit same-or-stronger audit

The pre-edit inventory above was audited against the optimized test and
subtest identities. Every CASE-SUB row remains present with its prior
observable assertions; rows moved into serialized fixture cohorts are marked
as merged witnesses below. No assertion, failure marker, privacy check,
atomicity check, event check, or cleanup check was removed or weakened.

| Case | Optimized test/subtest witness | Audit result |
| --- | --- | --- |
| CASE-SUB-001 | `TestStructuredSubmissionSimplePipeline/TestAPIPOSTSubmitAndQueryWork` | Same assertions; merged into simple-pipeline cohort |
| CASE-SUB-002 | `TestAPIBatchUpsertAcceptsWorksContent` | Same assertions |
| CASE-SUB-003 | `TestStructuredSubmissionSimplePipeline/TestCLIWorkTypeNameReachesLiveAPIHandler` | Same assertions; moved into live-root cohort |
| CASE-SUB-004 | `TestWorkBatchHTTPSubmission/TestAPISubmitBatchThenListAndGetWork` | Same assertions; merged into HTTP cohort |
| CASE-SUB-005 | `TestWorkBatchHTTPSubmission/TestAPIUpsertWorkRequestUsesCanonicalIdentity` | Same assertions; merged into HTTP cohort |
| CASE-SUB-006 | `TestWorkBatchHTTPSubmission/TestAPIUnknownWorkReturnsTypedNotFound` | Same assertions; merged into HTTP cohort |
| CASE-SUB-007 | `TestWorkBatchCLIIngress/TestWorkBatchAcceptsInlineFileAndStdinShapes/inline` | Same assertions; merged into CLI cohort |
| CASE-SUB-008 | `TestWorkBatchCLIIngress/TestWorkBatchAcceptsInlineFileAndStdinShapes/file` | Same assertions; merged into CLI cohort |
| CASE-SUB-009 | `TestWorkBatchCLIIngress/TestWorkBatchAcceptsInlineFileAndStdinShapes/stdin` | Same assertions; merged into CLI cohort |
| CASE-SUB-010 | `TestWorkBatchCLIIngress/TestWorkBatchSelectsDefaultAndExplicitWorkTypes/default` | Same assertions; merged into CLI cohort |
| CASE-SUB-011 | `TestWorkBatchCLIIngress/TestWorkBatchSelectsDefaultAndExplicitWorkTypes/explicit` | Same assertions; merged into CLI cohort |
| CASE-SUB-012 | `TestWorkBatchCLIIngress/TestWorkBatchRejectsUnknownTypeWithoutPartialMutation/unknown_type` | Same rejection, typed diagnostic, and no-mutation assertions |
| CASE-SUB-013 | `TestWorkBatchCLIIngress/TestWorkBatchRejectsUnknownTypeWithoutPartialMutation/mixed_batch_no_partial_submit` | Same atomicity and no-mutation assertions |
| CASE-SUB-014 | `TestBlockedDispatchConcurrentBatchIngressRegression` | Same public RUNNING/in-flight, idempotency, event, and deterministic-release assertions |
| CASE-SUB-015 | `TestWorkBatchDependencyOrderingNormalizesRuntimeWork` | Same ordering, relation, event, identity, and completion assertions |
| CASE-SUB-016 | `TestPayloadSizeHTTPSubmission/TestAPIBatchUpsertRejectsOversizedWorkAtomically` | Same typed error, privacy, atomicity, and no-event assertions; merged into boundary cohort |
| CASE-SUB-017 | `TestPayloadSizeHTTPSubmission/TestAPIBatchUpsertAcceptsPayloadAtInclusiveLimit` | Same inclusive-boundary, identity, list, and event assertions |
| CASE-SUB-018 | `TestWorkBatchHTTPSubmission/TestAPIStageAndSubmitFileCreatesExpectedWork` | Same staged-file and projected-image assertions; moved into HTTP cohort |
| CASE-SUB-019 | `TestStructuredSubmissionSimplePipeline/TestAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission` | Same empty-content and completion assertions |
| CASE-SUB-020 | `TestStructuredSubmissionSimplePipeline/TestAPISubmitWorkRejectsEmptyStructuredSubmission` | Same HTTP 400 assertion |
| CASE-SUB-021 | `TestStructuredSubmissionSimplePipeline/TestAPISubmitWorkAcceptsOrderedTextSubmission` | Same ordered text and whitespace assertions |
| CASE-SUB-022 | `TestStructuredSubmissionSimplePipeline/TestAPISubmitWorkAcceptsCanonicalContentParts` | Same canonical content ordering and whitespace assertions |
| CASE-SUB-023 | `TestAPISubmitWorkAcceptsMixedTextAndImageOnSupportedRunner` | Same controlled response and no-echo assertions |
| CASE-SUB-024 | `TestAPISubmitWorkAcceptsMixedTextAndImageOnUnsupportedRunner` | Same failed status and zero-provider-call assertions |
| CASE-SUB-025 | `TestStructuredSubmissionSimplePipeline/TestAPISubmitWorkRejectsForgedStructuredFileReference` | Same HTTP 400 security assertion; merged into simple-pipeline cohort |
| CASE-SUB-026 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/direct_POST_and_idempotent_PUT` | Same trace, idempotency, completion, and canonical-event assertions |
| CASE-SUB-027 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/startup_work_file_batch` | Same startup persistence and canonical-event assertions |
| CASE-SUB-028 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/file_watcher_non_batch_JSON` | Same watcher completion and canonical-event assertions |
| CASE-SUB-029 | `TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly/cron_internal_time_work` | Same fake-clock, tags, source, and canonical-event assertions |
| CASE-SUB-030 | `TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests` (`functionallong`) | Same record/replay identity and canonical-event assertions |
| CASE-SUB-031 | `TestWorkBatchPublicShapeStaysAlignedAcrossWatchedFileAndHTTP` (`functionallong`) | Same exact sorted public-shape and relation assertions |

The optimized inventory is therefore explicitly **same or stronger** for
CASE-SUB-001 through CASE-SUB-031. The only identity changes are parent
fixture grouping and named subtest paths; the two tagged test identities and
their assertions remain unchanged.

## GATE-LOOPBACK — clean-room validation report

# Validation report: BEH-SUBMISSION-PERF

## Environment and artifact

- Commit/build identifier: clean worktree at
  `d6688af10a` (`d6688af10a5f66e67944fb1600fe13f50aec31de`); the tested
  implementation is the optimized package plus the review corrections.
- Environment and configuration: `go version go1.25.0 windows/amd64`,
  Windows build `26200`, `amd64`, shared compute-saturated host; no test
  flags beyond the commands listed below.
- Customer entry point: root-built functional process exercised through the
  public Work HTTP and CLI contracts.
- Real and substituted dependencies: local production wiring with controlled
  provider/command-runner edges; no remote product dependency.
- Cost/call budget used: `$0`, zero paid or real-remote calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| GATE-PACKAGE: default Work submission package | PASS | `go test ./tests/functional/work/submission/... -count=1` x3; all exit `0`, package median `36.655s`, wall median `42.981s` | Terminal PR CI and merge |
| GATE-RACE: changed package race safety | PASS | `go test -race ./tests/functional/work/submission/... -count=1`; exit `0`, package time `60.887s`, no race report | Schedules outside this run |
| GATE-LONG: tagged replay and compatibility witnesses | PASS | `go test -tags=functionallong ./tests/functional/work/submission/... -count=1`; exit `0`, package time `35.175s` | Remote tagged workflow behavior |
| GATE-ASSERTIONS: CASE-SUB-001..031 | PASS | Post-edit same-or-stronger table above; all 31 rows mapped to retained observable assertions | Untested real-remote systems |
| Scope and cleanup | PASS | Final source accounting remains package-only, with no package-local sleeps and deterministic cleanup witnesses | Future changes after this head |

## Customer journey

1. From the clean detached final artifact, run the exact default package
   command. All 29 default Work-submission cases pass through assembled local
   production wiring, including HTTP, live-root CLI, Work projection, event,
   persistence, failure, and controlled-edge assertions.
2. Run the race and `functionallong` commands from the same clean artifact.
   Both pass, preserving the two tagged witnesses and detecting no race in the
   changed package paths.

## Cross-task integration and usability

- Documentation discoverability: final measurements, topology, assertion map,
  and loopback results are in the authorized C14 evidence ledger.
- Permission and error behavior: existing typed HTTP/CLI rejection,
  atomicity, privacy, and unsupported-capability assertions pass.
- Persistence/reload behavior: startup-file, watcher, cron, record/replay,
  Factory Event, and Work projection witnesses pass in the package/tagged
  gates.
- Accessibility/keyboard/responsive behavior: not applicable; no UI or
  customer-copy changes.
- Operational signals: test failures remain actionable through exact package,
  race, tagged, and controlled-edge assertions; no sensitive payloads or
  credentials are recorded.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| — | — | — | No loopback defect | No findings | All clean-room gates passed |

## Verdict

PASS
