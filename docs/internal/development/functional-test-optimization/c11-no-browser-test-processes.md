# C11 no-browser test-processes implementation ledger

## Scope and source authority

- Behavior: `BEH-NB-01` — test-spawned built CLI processes suppress the real
  browser launcher before construction while production defaults and explicit
  injection remain unchanged.
- Implementation story: `functional-test-optimization-c11-no-browser-test-processes-001`.
- Source-plan reference: `Scope 9 — No real browser launch from any test run;
  Acceptance Criterion 8; Non-goals`.
- The PRD references `docs/temp/functional-test-optimization.md`, which is not
  present in this checkout. That absence is recorded as an authority gap; no
  source-plan content was invented or silently repaired.
- Authored production source: `pkg/wire/profiles.go`.
- Authored harness sources: `internal/builtcliacceptance/env.go` and
  `internal/builtcliacceptance/harness.go`.
- Generated Wire output was regenerated with `make wire-smoke`; no generated
  diff remained.

## Configuration and precedence

`YOU_NO_BROWSER_OPEN=1` is an exact-value, process-environment opt-out. Wire
selection order is:

1. an explicit `edges.BrowserOpener`;
2. a no-op opener when the value is exactly `1`;
3. the existing `platformbrowser.NewHost(runtime.GOOS).Open` fallback.

Missing, empty, whitespace, `0`, `true`, and other values retain the real
fallback. The host factory is not evaluated for the exact opt-out.

Every canonical built-child environment removes inherited or extra
`YOU_NO_BROWSER_OPEN` entries and appends exactly one
`YOU_NO_BROWSER_OPEN=1`, while retaining isolated home variables and unrelated
entries.

## Local implementation evidence

Environment: Windows `go1.25.0`, `windows/amd64`, local controlled launcher
sentinel, no remote or paid dependency.

| Evidence | Result | Property proved |
| --- | --- | --- |
| `go test ./pkg/wire -run 'Browser|Opener' -count=1` | PASS | Exact-value selection, injection precedence, lazy non-construction. |
| `go test ./internal/builtcliacceptance/... -count=1` | PASS | Canonical environment propagation and child-runner delivery. |
| `make wire-smoke` | PASS | Wire regeneration, drift check, and package-wide Wire tests. |
| `go test ./tests/integration/transport/cli/process -run 'TestBuiltCLINoBrowserOpen' -count=1 -timeout 5m` | PASS | A real built server reaches readiness, fails on an occupied `--listen` port, recovers, overlaps on isolated ports, and stops without the controlled `rundll32.cmd` marker. |
| `go test ./tests/integration/transport/cli/process -count=1 -timeout 10m` | PASS (implementation-stage, pre-rebase) | Existing built-process signal, pipe, startup, recovery, and cleanup behavior passed on the implementation-stage head; the final rebased-head rerun and its cleanup failure are recorded below. |

The integration test uses a temporary fail-closed `rundll32.cmd` at the front
of the child PATH. Its marker was absent after readiness, failure, recovery,
concurrent execution, and stop. The child stdout scanner was joined after
each started process. Temporary homes and the reusable package binary directory
remain owned by the existing Go test cleanup and `TestMain` paths.

## Case coverage at this stage

| Cases | Evidence | Status | Remaining edge |
| --- | --- | --- | --- |
| `CASE-NB-001`–`CASE-NB-006` | Wire selector tests | PASS locally | Linux same-head evidence belongs to `GATE-LINUX`. |
| `CASE-NB-007`–`CASE-NB-009`, `CASE-NB-020` | Harness environment and child-runner tests | PASS locally | Final clean-room audit belongs to `GATE-LOOPBACK`. |
| `CASE-NB-010`–`CASE-NB-017` | Built CLI integration matrix on Windows | PASS locally | Linux sentinel and current-head CI evidence belong to `GATE-LINUX`. |
| `CASE-NB-018` | No authorization contract exists | Not applicable | None. |
| `CASE-NB-019` | Exact opt-out disables launcher construction | Not applicable under opt-out | Server deadline behavior remains owned by existing process tests. |

## Security, privacy, and exclusions

No browser, network, remote provider, paid dependency, persisted secret, or
inherited-environment dump was introduced. No public CLI, OpenAPI, event,
persisted contract, production auto-open default, functional shared support,
C01 inventory, baseline, or stability-cleanup surface was changed. UI,
accessibility, keyboard, responsive, and localization checks are not
applicable.

## Final validation loopback — story 002

This section follows `factory/docs/standards/validation-loopback-template.md`.
It is read-only validation evidence; no implementation defect was silently
repaired.

### Environment and artifact

- Commit/build identifier: rebased validation head
  `a42201245e`; the implementation commits are `1662c27ff1` and
  `18315bea3f`, rebased onto `origin/main` `962b2e0eea`. This report amendment
  is documentation-only.
- Environment and configuration: Windows `windows/amd64`, Go `go1.25.0`,
  local controlled `rundll32.cmd` sentinel, isolated harness homes and random
  loopback ports. The source-plan path named by the PRD remains absent.
- Customer entry point: the compiled `you` binary through `server --listen`.
- Real and substituted dependencies: production Wire and the real built CLI;
  only the operating-system launcher was substituted with a fail-closed
  sentinel. No remote product or paid dependency was used.
- Cost/call budget used: zero paid or remote calls.

### Final-head evidence

| Procedure | Result | Property proved |
| --- | --- | --- |
| `go test ./pkg/wire -run 'Browser|Opener' -count=1` | PASS, exit 0 | Exact-value selector precedence, explicit injection, unchanged fallback, and lazy non-construction. |
| `go test ./internal/builtcliacceptance/... -count=1` | PASS, exit 0 | Exact-one environment normalization through the canonical helpers and child runner. |
| `go test ./tests/integration/transport/cli/process -run '^TestBuiltCLINoBrowserOpenSuppressesLauncherAcrossLifecycleCases$' -count=1 -timeout 5m` | PASS, exit 0, 6.891s | C11 Windows readiness, occupied-port failure, recovery, cancellation, concurrency, port reuse, scanner join, and launcher-marker absence with clean focused teardown. |
| `go test ./tests/integration/transport/cli/process -count=1 -timeout 10m` | FAIL, exit 1, 55.201s on the final attempt | All process tests printed `PASS`, but existing `TestMain` cleanup could not unlink `you.exe` under `you-cli-process-package-285162567` (`Access is denied`). |
| `make test-functional` | FAIL, exit 1 | The C11-owned `tests/functional/transport/cli/process` cell passed; the complete local lane failed in untouched model and ACP areas. |
| `go test ./tests/functional/providers/acp -count=1 -timeout 5m` | PASS, exit 0, 65.575s | The ACP failures observed only under full-lane contention are not reproducible in isolation on the rebased head. |

The final full functional run reported
`TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess` in
`tests/functional/models/root_composition` observing `READY/INSTALLED` instead
of its `MISSING/NOT_INSTALLED` baseline. The same run also reported ACP
process-start/context-cancellation failures, including
`TestFactoryRunRetriesACPProviderByResumingExactSession`,
`TestUnknownExecutorProviderFailsBeforeACPProcessStart`,
`TestProvidersACPRetiresDisconnectedConnectionBeforeReuse`,
`TestACPCommandStartFailureMapsToDependencyFailure`,
`TestPackagedACPProfilesUseSharedConformanceBehavior`,
`TestACPFailureRedactsConfiguredSecretsFromStderr`,
`TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass`,
`TestYouRunReturnsUnsupportedFilesystemAndTerminalRPCsAtTheACPBoundary`,
`TestPackagedSpawnRunsPlannerChildrenAndMergerThroughPersistentACPStdio`, and
`TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio`.
The model and ACP packages are outside this diff; no repair was made. The
model-cache/root-build failure was previously reproduced on parent
`d4ce490f7c`, and the ACP package passed in the isolated diagnostic above.

The available Linux run `33249826008` tested the pre-rebase implementation
head `9f7968dc7015859527f4e63f174031a70e5be841` on Go `1.25.0`/Linux and
passed the complete functional coverage job, including the existing
`tests/functional/transport/cli/process` cell in 4.606s. The repository CI
workflow does not invoke `tests/integration/transport/cli/process`, and no
same-head Linux run exists for rebased head `a42201245e`; this run is not
evidence for the Linux built-executable sentinel edge.

### Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Exact Wire opt-out and precedence | PASS | Focused Wire tests on the final implementation head; no-op selected before the controlled host factory for exact `YOU_NO_BROWSER_OPEN=1`. | A real production browser launch is intentionally not performed. |
| Exact-one canonical child environment | PASS | Final-head builtcliacceptance suite passed. | A Linux built child using this helper. |
| Built-server success, failure, cancellation, concurrency, recovery, and no launcher | BLOCKED | Windows C11 selector matrix passed; Linux integration package is not run by the current CI workflow. | Linux sentinel construction/invocation and Linux cleanup. |
| CASE-NB-001 through CASE-NB-020 | BLOCKED | Windows and controlled selector cases are enumerated below; Linux cases remain unexercised. | Complete cross-platform matrix on the final pushed head. |
| Named Wire, harness, and process commands | FAIL | Wire and harness passed; the focused C11 process selector passed, but the full named process package exited 1 in existing `TestMain` executable cleanup. | A clean full package teardown and Linux operating-system path. |
| One final `make test-functional` pass | FAIL | The sole final rebased-head local invocation exited 1 in untouched model and ACP packages; C11's functional process cell passed. | A clean full-lane pass under an uncontended/baseline-clean environment. |
| Windows/Linux cleanup and CI evidence | BLOCKED | Focused C11 cleanup, port reuse, scanner joins, and marker absence passed; full package teardown hit an existing Windows access-denied cleanup; Linux functional CI is stale/pre-rebase and has no integration-process invocation. | Same-head Linux process, marker, port, home, scanner, and artifact evidence plus full package teardown. |
| Contract and exclusion audit | PASS | `git diff --check` is clean; diff is limited to the intended Wire, harness, integration test, and ledger files. | None in the inspected diff. |
| Security/privacy and non-UI scope | PASS | No real browser marker, remote product call, paid call, secret persistence, or inherited-environment dump was produced; UI checks are N/A. | Linux marker observation. |
| Validation loopback report | PASS (with BLOCKED verdict) | This report follows the required template and contains the delta plan below. | Delta-plan execution. |
| Implementation delivery handoff | BLOCKED | The prior PR feedback code finding is fixed and the PR remains open, but this report still has Linux/full-functional blockers and the next head has not yet started CI. | Final pushed report head and review-owned terminal checks. |

### CASE-NB matrix

| Case | Outcome | Evidence or explicit disposition |
| --- | --- | --- |
| CASE-NB-001 | PASS | Missing opt-out selects the lazy host factory exactly once in Wire tests. |
| CASE-NB-002 | PASS | Empty value retains the lazy real fallback in Wire tests. |
| CASE-NB-003 | PASS | Whitespace, `0`, `true`, and other non-`1` values retain the fallback; no permissive parsing. |
| CASE-NB-004 | PASS | Explicit opener is invoked and host factory is not evaluated. |
| CASE-NB-005 | PASS | Explicit opener retains first precedence with exact opt-out present. |
| CASE-NB-006 | PASS | Exact opt-out returns a no-op and evaluates the controlled host factory zero times. |
| CASE-NB-007 | PASS | Isolated-home helper removes inherited variants and emits one canonical `YOU_NO_BROWSER_OPEN=1`, retaining unrelated entries and home isolation. |
| CASE-NB-008 | PASS | `Session.ProcessEnv` exposes the same invariant to a direct child. |
| CASE-NB-009 | PASS | `ProcessEnvWith` and child runners preserve one canonical entry with extra variables. |
| CASE-NB-010 | PASS | Windows built server reaches dashboard readiness with the launcher marker absent. |
| CASE-NB-011 | PASS | Windows occupied `--listen` startup fails without claiming readiness or creating a marker. |
| CASE-NB-012 | PASS | Windows stop/cancellation joins the child scanner and leaves no launcher marker. |
| CASE-NB-013 | PASS | Windows overlapping children use distinct homes/ports and both suppress the launcher. |
| CASE-NB-014 | PASS | Controlled Windows `rundll32.cmd` sentinel is neither constructed nor invoked. |
| CASE-NB-015 | BLOCKED | Linux `xdg-open` sentinel is implemented but the integration package has no current-head Linux CI invocation. |
| CASE-NB-016 | BLOCKED (Windows package teardown) | The focused C11 selector rebinds ports, joins scanners, and removes its per-run resources; the full package's existing `TestMain` could not remove its reusable binary because Windows returned `Access is denied`. |
| CASE-NB-017 | PASS (Windows) | Windows failure/recovery starts a fresh child and preserves the primary failure; Linux recovery remains unproven. |
| CASE-NB-018 | N/A | Local browser opening has no authorization contract. |
| CASE-NB-019 | N/A under opt-out | The launcher is disabled before construction; no launcher timeout/outage behavior is applicable. Existing server deadlines remain covered by existing process tests. |
| CASE-NB-020 | PASS | Repeated helper construction independently yields one canonical entry; no state accumulates across children. |

### Customer journey

1. Built the customer CLI through the production command path and ran the
   focused Wire and harness gates; both exited 0.
2. Spawned the real Windows binary with `server --listen`, an isolated home,
   a controlled front-of-PATH launcher, and no external server. The dashboard
   readiness signal arrived, occupied-port startup failed safely, recovery and
   concurrent children succeeded, cancellation completed, ports rebound, and
   no launcher marker appeared. The focused C11 selector teardown passed;
   the full package later failed only while removing its reusable binary.
3. Ran the sole final rebased-head `make test-functional` invocation. The C11
   functional process cell passed, but unrelated model and ACP packages caused
   exit 1; the ACP package passed when subsequently run alone.
4. Reviewed the available Linux CI. Its functional suite passed on the
   pre-rebase head, but the required real built-process package is not part of
   that workflow and no rebased-head Linux run exists, so the Linux journey
   cannot be claimed complete.

### Cross-task integration and usability

- Documentation discoverability: the report is in the existing canonical C11
  implementation ledger; no customer-facing documentation or copy changed.
- Permission and error behavior: no authorization contract exists; Windows
  occupied-port diagnostics and non-readiness behavior remain intact.
- Persistence/reload behavior: no persisted contract or migration changed.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: dashboard readiness, child stdout scanner completion,
  exit status, port rebind, controlled marker, and temp/process audits were
  observed without persisting secrets or a full inherited environment.

### Cleanup and scope audit

- The focused C11 Windows process run left no process command line referencing
  the worktree, no current C11 launcher marker, no current owned listener, and
  no current per-run process-package binary directory. Scanner joins and port
  rebind assertions passed in that selector.
- The full process-package attempts left generated directories
  `you-cli-process-package-2837636637` and
  `you-cli-process-package-285162567` because existing `TestMain` teardown
  received Windows `Access is denied` for their `you.exe` files. No process
  owned those paths during the read-only audit; they are recorded as a
  baseline cleanup blocker and were not deleted by this lane.
- A pre-existing
  `C:\Users\andre\AppData\Local\Temp\you-cli-process-package-1092338325\you.exe`
  dated 2026-08-28 was observed without an owning process. It predates this
  validation and was left untouched to avoid deleting another checkout's
  generated artifact; it is not attributed to the current run.
- No public CLI, OpenAPI, event, persisted contract, production auto-open
  default, functional shared support, C01 inventory, Project validation,
  baseline, stability-cleanup file, or unrelated source surface changed.

### Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C11-F001 | BLOCKING | Inspect the current CI workflow and the available run `33249826008`. | Linux runs `go test ./tests/integration/transport/cli/process -count=1` with the controlled `xdg-open` sentinel. | The Linux job runs the functional tree and its existing functional CLI-process cell, but not the integration process package. | Workflow log and pre-rebase run `33249826008`; Linux process-package evidence is absent. |
| C11-F002 | BLOCKING baseline/environment | Run the required final rebased-head `make test-functional`. | Complete local functional lane exits 0. | Exit 1: model root-composition baseline mismatch and ten ACP process-start/context-cancellation failures; the C11 functional process cell passes, and ACP passes in isolation. | Final local command output; package paths and test names recorded above. |
| C11-F003 | BLOCKING review-owned CI | Review required checks for prior implementation head `9f7968dc70`. | Required checks are terminal and green. | `Backend Unit Latency` failed on unrelated `pkg/transports/mcp/server/TestServeStdioValidatesRuntimeInputsAndCancellation`; `Verification Policy` consequently failed. Backend Lint and functional coverage were green. | Existing PR #2439 review evidence and Actions run `33249826008`; a new run is required for the rebased head. |
| C11-F004 | BLOCKING baseline/environment | Run `go test ./tests/integration/transport/cli/process -count=1 -timeout 10m` on the rebased head. | Full named process package exits 0 and removes its reusable binary. | All tests printed `PASS`, but existing `TestMain` failed `RemoveAll` with Windows `Access is denied` for `you-cli-process-package-285162567`; the focused C11 selector cleaned up successfully. | Final process-package command output and read-only process/temp audit above. |
| C11-F005 | NON-BLOCKING host hygiene | Inspect temp/process state after the current run. | A clean-room host has no unrelated generated residue. | One pre-existing process-package binary dated 2026-08-28 remains; no current process owns it and it was not deleted. | Read-only Windows temp/process audit above. |

### Verdict

BLOCKED

### Delta-plan request [Required for BLOCKED]

- Affected behavior and criteria: `BEH-NB-01`, GATE-LINUX,
  GATE-FUNCTIONAL/GATE-PR-CI, and the process cleanup criterion; specifically
  C11-F001 through C11-F004 and criteria for Linux built-child behavior, full
  package teardown, and a green final functional pass.
- Root-cause evidence or remaining uncertainty: the existing CI workflow has
  no Linux invocation of the integration process package; the available Linux
  functional run is pre-rebase. The final local lane is contaminated by
  unrelated model/ACP behavior, and the full process package has an existing
  Windows executable-cleanup failure. The hosted required-check failure is in
  an untouched MCP package. No C11 regression was observed in the focused
  selector or its Windows real-binary behavior.
- Smallest recommended correction/prerequisite: provide one current rebased-
  head Linux run of `go test ./tests/integration/transport/cli/process
  -count=1 -timeout 10m` with the fail-closed `xdg-open` sentinel and cleanup
  artifact; obtain a baseline-clean or owner-dispositioned full functional
  run; and route the existing process-package teardown to its owning cleanup
  lane. Do not change C11 implementation, model/ACP/MCP code, or unrelated CI
  policy in this lane without an operator-authorized scope delta.
- Dependencies and retest scope: a Linux CI/integration execution owner, the
  process-package cleanup owner, and the owners of the unrelated model, ACP,
  and MCP baseline failures; then rerun the process package, one final
  `make test-functional`, and the current-head required checks. Review owns
  terminal CI, conflict resolution, and merge.

## Handoff and remaining gates

The implementation review finding about the unreachable environment helper was
fixed in the pre-rebase head `9f7968dc70` and is present in rebased commit
`18315bea3f`. This loopback has supplied the requested Windows validation and
explicit delta plan, but it does not claim story 002 or lane completion while
C11-F001 through C11-F004 remain. The next action is to push this report in
the open PR and post the evidence; the review workstation owns terminal CI,
Linux evidence execution if provisioned, conflict resolution, and merge.
