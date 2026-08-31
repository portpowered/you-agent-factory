# C15 provider implementations characterization ledger

## Scope and status

This ledger is the fixed pre-change denominator for
`functional-test-optimization-c15-provider-implementations-001` and GATE-CHAR-001.
It reconciles every AGY, Claude, permission, and Codex row in `prd.md` with the
current functional-test witness, records the current source-derived topology,
and preserves the one required noisy diagnostic run. No provider test,
production package, support fixture, inventory, baseline, API, or UI source was
changed for this story.

The task packet names `docs/temp/functional-test-optimization.md` as its source
plan, but that file is not present in this checkout. The complete matrix and
scope used here are the checked-in `prd.md` packet and `prd.json`. This is a
planning-input discrepancy for the operator/review stage, not a reason to
invent a replacement plan in this story.

The packet cites latest hosted functional coverage of 61.6%. The checked-in
C13 evidence document currently says 61.3%. Both values are retained as
provenance; current-PR coverage is explicitly deferred to GATE-PR-FUNC-006.

## Evidence procedure

The required commands were run once before structural changes:

```text
go test -list '^Test' ./tests/functional/providers/agy
go test -list '^Test' ./tests/functional/providers/claude
go test -list '^Test' ./tests/functional/providers/permission
go test -list '^Test' ./tests/functional/providers/codex
go test ./tests/functional/providers/agy ./tests/functional/providers/claude ./tests/functional/providers/permission ./tests/functional/providers/codex -count=1 -timeout=10m
make functional-os-boundary-check
```

The four list commands exited zero. Their top-level identity inventories are
in the next section. The combined functional command exited zero with these Go
test package results:

```text
ok  github.com/mostlygeek/you-agent-factory/tests/functional/providers/agy         136.736s
ok  github.com/mostlygeek/you-agent-factory/tests/functional/providers/claude      15.392s
ok  github.com/mostlygeek/you-agent-factory/tests/functional/providers/permission   8.246s
ok  github.com/mostlygeek/you-agent-factory/tests/functional/providers/codex        9.181s
```

Those elapsed values are a single local diagnostic on a saturated Windows
host, not a portable performance threshold. Other Go test jobs were running on
the host during the probe. The supplied hosted observations remain the
customer reference: AGY 19.558s, Claude 11.518s, permission 5.402s. The
supplied noisy Windows diagnostic remains diagnostic only: AGY 166.332s,
Claude 30.490s, permission 12.815s, Codex 16.138s, combined wall 192.985s.

Environment captured with the probe: Go 1.25.0, Windows 10 build 26200,
PowerShell 7.6.5, 13th Gen Intel Core i7-13700K, 24 logical processors, base
HEAD `42eeee4472656b8290f798c36a5b8c871b24d7d0`, `GOAMD64=v1`,
`CGO_ENABLED=1`, and no configured `GOMAXPROCS` or `GOFLAGS` value.

The OS-boundary checker exited zero:

```text
[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0
[agent-factory:functional-os-boundary] reconciled 70 inventory OS-spawn records
```

The Codex package has exactly one current functional accidental record:
`OSSPAWN-tests-functional-providers-codex-codex-untrusted-working-directory-test-initTrustedGitRepository-01`,
`tests/functional/providers/codex/codex_untrusted_working_directory_test.go:188`,
`initTrustedGitRepository`, `fixture-git-command`. Its call to `git -C <dir>
init` prepares a fixture and is not itself asserted as an OS-process behavior.
The unchanged aggregate baseline/inventory is 70 records; story 005 owns
removing this one site.

## Current test identity inventory

The list probes found 13 AGY, 5 Claude, 1 permission, and 2 Codex top-level
test identities. Table-driven subcases are reconciled below rather than
treated as new top-level identities.

### AGY top-level identities

```text
TestAgyMultimodalGoldenThroughRootBuildProcess
TestAgyClipQAGoldenPassThroughRootBuildProcess
TestAgyStructuredJSONGoldenThroughRootBuildProcess
TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess
TestAgySharedProcessFailureThenSuccessRecovers
TestAgySharedProcessEarlyHostedExitReleasesResources
TestAgySharedProcessConcurrentRoutesRemainIsolated
TestAgyLiveSmoke
TestAgyConductorSuccessThroughRootBuildProcess
TestAgyNativeFailureThroughRootBuildProcessIsSafe
TestAgyTimeoutFailureThroughRootBuildProcess
TestAgyCommandCancellationThroughRootBuildProcessIsCanonical
TestAgyProductionReviewRolesThroughRootBuildProcess
```

### Claude top-level identities

```text
TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting
TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks
TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources
TestClaudeHaikuStreamJSONGoldens
TestClaudeStreamJSONCommandThroughRootBuildProcess
```

### Permission and Codex top-level identities

```text
TestProviderPermissionBypassFunctionalContract
TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess
TestCodexSharedTrustedWorkAndHistory
```

## Matrix reconciliation

Every row below names the existing public-behavior witness. `planned addition`
means the row is intentionally not present in the base source and is owned by
the later story named in the note; it is not claimed as current evidence.

### AGY-001..029

| ID | Existing witness and preserved observable |
|---|---|
| AGY-001 | `TestAgyConductorSuccessThroughRootBuildProcess`: completed Work, AGY/session metadata, and skip-permissions command contract. |
| AGY-002 | `TestAgyNativeFailureThroughRootBuildProcessIsSafe`: one safe normalized failure with secret-like text hidden. |
| AGY-003 | `TestAgyTimeoutFailureThroughRootBuildProcess`: canonical timeout/provider-session failure and cleanup. |
| AGY-004 | `TestAgyCommandCancellationThroughRootBuildProcessIsCanonical`: canonical cancellation without invented success. |
| AGY-005 | `TestAgyMultimodalGoldenThroughRootBuildProcess/video-watch`: exact sanitized response, request, session, Work, Event, and dispatch assertions. |
| AGY-006 | `TestAgyMultimodalGoldenThroughRootBuildProcess/groundtruth-video`: exact visual/audio evidence and provider metadata. |
| AGY-007 | `TestAgyClipQAGoldenPassThroughRootBuildProcess`: schema-valid Clip-QA pass with audio evidence. |
| AGY-008 | `TestAgyStructuredJSONGoldenThroughRootBuildProcess`: exact structured JSON shape, values, and provider metadata. |
| AGY-009 | `TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess`: missing-file refusal, failed Work, and failed dispatch. |
| AGY-010 | `TestAgyProductionReviewRolesThroughRootBuildProcess/cold-watch-complete-report-contract`: complete report fields and pass recommendation. |
| AGY-011 | `TestAgyProductionReviewRolesThroughRootBuildProcess/cold-watch-incomplete-real-traces-fail/video-watch` and `/groundtruth-video`: incomplete traces fail. |
| AGY-012 | `TestAgyProductionReviewRolesThroughRootBuildProcess/missing-file-fails-work-after-provider-success`: missing-file evidence and failed Work. |
| AGY-013 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-structured-pass-with-audio-evidence`: structured pass and audio evidence. |
| AGY-014 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-structured-reroll-is-accepted`: accepted reroll terminal outcome. |
| AGY-015 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/confidence-below-zero`: semantic rejection and failed Work. |
| AGY-016 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/confidence-above-one`: semantic rejection and failed Work. |
| AGY-017 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/pass-with-incomplete-action`: semantic rejection and failed Work. |
| AGY-018 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/pass-with-specification-deviation`: semantic rejection and failed Work. |
| AGY-019 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/pass-with-temporal-artifact`: semantic rejection and failed Work. |
| AGY-020 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/pass-with-unexpected-speech`: semantic rejection and failed Work. |
| AGY-021 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-semantic-invalid-results-fail/reroll-with-provider-failure-status`: semantic rejection and failed Work. |
| AGY-022 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-schema-invalid-result-fails-work`: schema failure and no successful output. |
| AGY-023 | `TestAgyProductionReviewRolesThroughRootBuildProcess/clip-qa-provider-failure-fails-work`: provider failure maps to failed Work with safe detail. |
| AGY-024 | `TestAgySharedProcessFailureThenSuccessRecovers`: sequential failed/successful outcomes, distinct identities, event order, and two removed recordings. |
| AGY-025 | `TestAgySharedProcessEarlyHostedExitReleasesResources`: blocked call release, closed HTTP run, and zero active calls. |
| AGY-026 | `TestAgySharedProcessConcurrentRoutesRemainIsolated`: overlapping immutable routes with isolated Work/Event/response/listener/session state. |
| AGY-027 | `assertAgySharedRouteRejections`: duplicate, unknown, late, mismatched, closed, and secret-bearing requests fail closed without census mutation. |
| AGY-028 | `agySharedProcessFixture.close` assertions exercised by the shared lifecycle tests: process, listener runs, active calls, recordings, routes, and temporary root cleanup. |
| AGY-029 | `TestAgyLiveSmoke`: existing opt-in paid smoke gate; skipped unless `YOU_AGY_LIVE_SMOKE=1`, so the remote edge remains unproven in this diagnostic. |

The AGY shared fixture freezes 29 routes: 4 direct + 5 golden + 16 role + 1
recovery + 1 early-exit + 2 concurrency routes. It builds one application
process and closes it once. Its source-derived invocation count is 30 default/
explicit session bindings: 29 route execution boundaries plus one additional
explicit concurrent session. It starts 11 listener-backed hosted runs: 4
direct + 5 golden + 1 early-exit + 1 concurrency. The role cases and both
recovery outcomes use the existing process without a new listener. At close,
the fixture asserts one process close, zero active calls, route count 29 before
clear and zero after clear, 16 role recordings plus 2 recovery recordings
absent, and the temporary root absent. Per-route command assertions require
one call; recovery requires two calls. The timeout witness intentionally
asserts `at least one` attempt, so the source does not expose a stronger
aggregate command-call total; that bound is retained rather than relabeled as
an exact count.

### CLAUDE-001..018

| ID | Existing witness and preserved observable |
|---|---|
| CLAUDE-001 | `TestClaudeHaikuStreamJSONGoldens/alias`: alias selector, pinned reported model/session, exact stream command, Work/Event, and output. |
| CLAUDE-002 | `TestClaudeHaikuStreamJSONGoldens/family`: family selector with pinned reported identity and exact output. |
| CLAUDE-003 | `TestClaudeHaikuStreamJSONGoldens/pinned`: fully pinned selector, session, command, and output. |
| CLAUDE-004 | `TestClaudeStreamJSONCommandThroughRootBuildProcess`: one Sonnet stream-json command with exact flags. |
| CLAUDE-005 | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting/empty-metadata`: validation fails before a provider route call. |
| CLAUDE-006 | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting/empty-stream`: line-one validation fails before a provider route call. |
| CLAUDE-007 | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting/malformed-stream`: JSON parse validation fails before a provider route call. |
| CLAUDE-008 | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting/partial-result`: terminal-result validation fails before a provider route call. |
| CLAUDE-009 | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting/unsanitized-stream`: sanitization validation fails before a provider route call. |
| CLAUDE-010 | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting/checksum-mismatch`: integrity validation fails before a provider route call. |
| CLAUDE-011 | `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks/duplicate-directory` and `/duplicate-selector`: duplicate routes reject without partial registration. |
| CLAUDE-012 | `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks/unknown-workdir`: fails closed without a new call or secret leak. |
| CLAUDE-013 | `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks/selector-mismatch`: fails before replay and leaves call count unchanged. |
| CLAUDE-014 | `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks/unexpected-command`: fails closed with bounded diagnostics. |
| CLAUDE-015 | `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks/closed-router`: rejects after close and leaves route count zero. |
| CLAUDE-016 | `TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources/partial-result`: failed Work, rejected success assertion, one route call, and cleanup. |
| CLAUDE-017 | `TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources/cancellation`: deterministic start, termination, one route call, and cleanup. |
| CLAUDE-018 | `TestClaudeHaikuStreamJSONGoldens`: three pre-registered manifest routes run in order, once each, with unique explicit non-default sessions and repeat-safe cleanup. |

The Claude package has four application/root builds and four listener starts:
one three-case golden process, two adverse process-path cases, and one
standalone stream-json process. It binds six sessions: three explicit golden
sessions, two explicit adverse sessions, and one default standalone session.
The route registries contain three golden routes plus one route in each adverse
process case (five registrations total); the six functional calls are three
goldens, one standalone, one partial-result, and one cancellation. Each
process closes its server/session/router resources; the invalid-router
validation identity is a direct router witness and intentionally adds no root,
listener, session, or provider-call count.

### PERM-001..003

| ID | Existing witness or planned addition |
|---|---|
| PERM-001 | `TestProviderPermissionBypassFunctionalContract/capable`: completed Work and exactly one Codex command containing `--dangerously-bypass-approvals-and-sandbox`. |
| PERM-002 | `TestProviderPermissionBypassFunctionalContract/incapable`: permanent bad request before the command edge, bounded capability diagnostic, failed Work, and zero runner calls. |
| PERM-003 | **Planned addition owned by story 004**: the base source deliberately keeps capable and incapable immutable capability graphs in separate root-built subtests; no current witness claims parallel overlap or race cleanup. |

Permission currently builds two roots, starts two listener-backed processes, and
uses two default session execution boundaries. It observes one command call in
the capable case and zero in the incapable case. There is no route registry in
this fixture. The separate roots are an immutable construction-time capability
boundary; story 004 owns the bounded concurrency witness and its cleanup/race
evidence. No new skip or quarantine was added.

### CODEX-001..012

| ID | Existing witness and preserved observable |
|---|---|
| CODEX-001 | `TestCodexSharedTrustedWorkAndHistory/trusted_work`: trusted fixture, completed Work, current event/history assertions, and one request. |
| CODEX-002 | `TestCodexSharedTrustedWorkAndHistory/actionable_refusal`: permanent actionable refusal names the trusted-repository remedy without retry. |
| CODEX-003 | `TestCodexSharedTrustedWorkAndHistory/neutral_refusal`: neutral refusal hides forbidden path, raw credential-like text, and exit detail. |
| CODEX-004 | `TestCodexSharedTrustedWorkAndHistory/successful_history`: exact provider, source, transcript, and session metadata. |
| CODEX-005 | `TestCodexSharedTrustedWorkAndHistory/detached_repeated_history`: repeated reads are stable and mutation-isolated. |
| CODEX-006 | `TestCodexSharedTrustedWorkAndHistory/missing_history`: stable not-found/empty behavior. |
| CODEX-007 | `TestCodexSharedTrustedWorkAndHistory/malformed_history`: safe parse failure without panic or leak. |
| CODEX-008 | `TestCodexSharedTrustedWorkAndHistory/oversized_history`: existing size-bound failure. |
| CODEX-009 | `TestCodexSharedTrustedWorkAndHistory/bounded_history`: bounded walk selects only allowed history in order. |
| CODEX-010 | `TestCodexSharedTrustedWorkAndHistory/containment_history`: containment escape cannot expose outside-session data. |
| CODEX-011 | `TestCodexSharedTrustedWorkAndHistory` fixture topology/finalize assertions: three unique sessions delete, routes/process/listener close, and paths disappear. |
| CODEX-012 | `make functional-os-boundary-check` plus the inventory record at `initTrustedGitRepository:188`: current accidental functional spawn is one; story 005 owns its removal. |

The shared Codex fixture builds one application process, starts one listener,
registers three routes, and binds three unique explicit sessions for trusted,
actionable-refusal, and neutral-refusal calls. It observes three provider
command calls. Finalization asserts three sessions opened/deleted, route count
3 before and 0 after clear, zero active calls, closed process/listener, and
removed temporary paths. The separate historical-cancellation witness builds
one additional root/listener and exercises provider-session discovery without
adding a Factory Session to the shared topology. Thus the package total is two
root builds and two listeners, with three shared Factory Sessions. The package's
one accidental OS spawn is the unchanged Git fixture initializer above; the
aggregate checker baseline remains 70/70 until story 005.

## Cleanup, skips, and remaining proof edges

The current witnesses retain public Work, Factory Session, Factory Event,
provider command, refusal, history, cancellation, concurrency, and cleanup
assertions. Existing capability-dependent behavior is unchanged: AGY live
smoke is opt-in and the Codex containment row may skip when the host cannot
create the required symlink. This story introduced no skip, quarantine,
sleep, timeout padding, or assertion weakening.

This characterization proves the base identity and diagnostic denominator only.
It does not prove shared-server/fresh-session parity, permission overlap/race
behavior, Codex zero-spawn behavior, optimized latency, current-PR coverage,
paid AGY behavior, or the final blind validation loopback. Those edges remain
owned by stories 002–006 and their named gates.
