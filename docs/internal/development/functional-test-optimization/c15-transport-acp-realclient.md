# C15 — transport/acp/realclient characterization ledger

Status: CHARACTERIZED FOR TASK-001. The current-head diagnostic and focused
failure characterization are recorded below. The diagnostic reached the real
production binary build but hit the existing one-minute phase bound under host
load before ACPX acquisition; it is retained as diagnostic evidence, not as a
real-client success claim.

This ledger owns the baseline, assertion map, C01 reconciliation, process
profile, and focused characterization for
`functional-test-optimization-c15-transport-acp-realclient-001`. It does not
authorize the reusable-session conversion owned by TASK-002 or final package
and PR validation owned by TASK-003.

## Authority and scope

- Baseline head before the characterization edit:
  `42eeee4472656b8290f798c36a5b8c871b24d7d0`.
- Package: `github.com/portpowered/infinite-you/tests/functional/transport/acp/realclient`.
- Owned implementation surface: the package's realclient tests and this
  package-specific ledger. No production ACP, shared functional support, or
  baseline inventory was changed.
- `prd.json` and `progress.txt` are ignored factory scaffolding and are not PR
  files.
- The supplied `40.486s` observation and prior C14 `34.118s` median are
  retained as prioritization context only; neither is treated as a portable
  local threshold or as current-head evidence.

## Environment and diagnostic authority

| Fact | Observation |
| --- | --- |
| OS/architecture | Windows `windows/amd64` |
| Go | `go1.25.0` |
| Host Node/npm | Node `v22.12.0`, npm `10.9.0` |
| Diagnostic Node/npm | Isolated official Node `v22.13.0`, npm/npx `10.9.2` in the system temp directory |
| Pinned dependency | `acpx@0.13.0` |
| Enable/required gates | `INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1`; `INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT=1` |
| Short mode | Not used for the required diagnostic |
| Provider/cost | Zero provider API calls; the diagnostic stopped before npm/ACPX/provider startup |
| State policy | ACPX home/cache/temp, binary, config, provider marker, and evidence were disposable; no payload or credential was retained |

### Required current-head diagnostic

Procedure, run once before the characterization edit:

```text
go test ./tests/functional/transport/acp/realclient/... -count=1 -timeout=10m -v
```

The command ran with the diagnostic Node `v22.13.0` first on `PATH`, both
real-client environment gates set to `1`, no `-short`, and a temp-only
`INFINITE_YOU_ACPX_EVIDENCE_OUTPUT` path. It exited `1` after an outer wall of
`69.838s`; the Go package reported `64.347s`.

Observed result:

- `TestRunBoundedCommandTerminatesScenarioDescendants`: PASS, `2.30s`.
- `TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit`: PASS, `0.97s`.
- `TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt`: reached
  `buildCurrentYouBinary`, then failed closed with
  `real ACP evidence failed during build-current-you: timeout` after its
  existing `60s` bounded phase; no ACPX version, session, prompt, close, or
  provider phase ran.
- Final package result: exit `1`, no skip, no real-client success evidence.
- Counting actual starts reached by this run: five test-owned parent starts
  (two helper parents, Node probe, Git revision, and Go build) plus two helper
  descendants, seven `Start`/`Output` process starts total; no `npx`, ACPX,
  production-server, or provider start was reached.

The host had substantial concurrent Go/Node/cmd activity when the timeout was
observed. This is retained as environmental timing noise; it is not attributed
to the characterization change and was not repaired by widening the test
timeout. The C14 supported local runs remain historical context and are not
substituted for a current-head pass.

### Discovery and prerequisite matrix

Before editing, this procedure exited `0` in `0.034s` and listed the three
existing top-level identities:

```text
go test ./tests/functional/transport/acp/realclient -list .
```

After adding the focused characterization, the same procedure exited `0` in
`0.323s` and listed the original three identities plus
`TestCharacterizePinnedAcpxFailureClassifications`. The added identity is
explicitly `added-after-C01`; it is not a replacement for an existing row.

The admission selectors were run on the host Node `v22.12.0` without starting
the real client. Each used
`go test ./tests/functional/transport/acp/realclient -run
^TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt$ -count=1 -timeout=5m -v`.

| Case | Gates | Result | Evidence disposition |
| --- | --- | --- | --- |
| `RC-PREREQ-001` | optional, `-short`, gates omitted | exit `0`; explicit skip: `real acpx client evidence builds the CLI and installs a pinned npm package` | PASS; no real-client claim |
| `RC-PREREQ-002` | optional, enable omitted | exit `0`; explicit skip: `set INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1 to run pinned real-client ACP evidence` | PASS; no real-client claim |
| `RC-PREREQ-003` | optional enable, host Node below minimum | exit `0`; explicit skip: `pinned acpx@0.13.0 requires Node.js 22.13.0 or later` | PASS; no startup claim |
| `RC-PREREQ-004` | required enable, `-short` | exit `1`; `required lane must run without -short` | PASS; fail closed before startup |
| `RC-PREREQ-005` | required, enable omitted | exit `1`; `required lane did not enable the pinned client` | PASS; fail closed before startup |
| `RC-PREREQ-006` | required enable, host Node below minimum | exit `1`; `Node.js 22.13.0 or later is required` | PASS; fail closed before startup |

## C01 classification reconciliation

The canonical C01 inventory identifies package `F026`, three top-level tests,
zero shareable rows, zero shareable-with-mock rows, and three
`isolated-with-reason` rows. The exact inherited rows and their C01 lineage are:

| Current identity | Prior C01 row | Current inventory row | Classification | Required property and reason |
| --- | --- | --- | --- | --- |
| `TestRunBoundedCommandTerminatesScenarioDescendants` | `C01-P038-TestRunBoundedCommandTerminatesScenarioDescendants` | `C07-F026-top-level-test-TestRunBoundedCommandTerminatesScenarioDescendants` | `isolated-with-reason` | Real `os.Args[0]` helper parent/descendant, OS process-tree ownership, timeout, and inactive-PID cleanup; sharing would remove the process boundary. |
| `TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit` | `C01-P038-TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit` | `C07-F026-top-level-test-TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit` | `isolated-with-reason` | Real helper parent/descendant, non-zero exit, tree termination, and inactive-PID cleanup; sharing would remove the exit/process boundary. |
| `TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt` | `C01-P038-TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt` | `C07-F026-top-level-test-TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt` | `isolated-with-reason` | Pinned ACPX executable, real stdio, current production executable, provider process, session lifecycle, and cleanup; sharing would remove the external-client boundary. |
| `TestCharacterizePinnedAcpxFailureClassifications` | `added-after-C01` (no prior row; no conflict) | C15 characterization identity | `isolated-with-reason` | Controlled parser/cleanup cases plus one real started command for launch-ownership cleanup characterization. It is not eligible for the reusable session process. |

The three inherited package rows retain their C01 source hashes and process
observation IDs (`PSO-0040`, `PSO-0041`, and `PSO-0042`) in the canonical
inventory. The new characterization is intentionally recorded here rather
than editing that historical baseline.

## Direct and descendant process profile

The pre-edit cost denominator is nine test-owned parent starts: two bounded
helper parents and seven normal pinned-client phases. The two helper parents
each start one held descendant, so the process-helper witnesses add two
descendant starts. Starts internal to Go, `npx`, ACPX, the configured `you`
executable, provider commands, Windows command hosts, and process inspection
are descendants or platform-control effects, not additional test-owned parent
phase counts.

| Owner | Test-owned parent starts | Descendant/control effects | Irreducible property and cleanup |
| --- | ---: | --- | --- |
| Timeout helper | 1 bounded parent | 1 `os.Args[0]` held descendant; Unix process group or Windows Job Object | One-second deadline, safe `timeout` classification, tree termination, and recorded PID inactive check. |
| Non-zero helper | 1 bounded parent | 1 `os.Args[0]` held descendant; Unix process group or Windows Job Object | Parent exits non-zero, safe `non-zero exit` classification, tree termination, and recorded PID inactive check. |
| Pinned client normal path | 7: Node version probe, Git revision, Go build, one `npx` pinned-version phase, ACPX session creation, prompt, and close | `npx`/Node/ACPX, configured current `you server acp`, and two deterministic provider invocations | Exact executable/config/version/stdio/session/frame/provider/close/queue-owner assertions. Current diagnostic reached the first three phases only. |
| C15 characterization | launch-failure case starts no process; ownership case starts one `os.Args[0]` command | Windows cleanup may invoke `taskkill`; parser and queue cases use only temp data | Safe launch/ownership classification, started-command wait, partial-session state transition, and queue-owner failure. This cost is not part of the pre-edit denominator. |

The five canonical C01 spawn-site records remain intentional OS boundaries:
`runProcessTreeParent` for stream/child framing, Windows process inspection and
control for descendant cleanup, `nodeSupportsPinnedAcpx` for executable
selection, and `runBoundedCommandWithTimeout` for executable/stdio/exit
behavior. No spawn was removed or replaced in TASK-001.

## Assertion and matrix inventory

| Case | Runtime witness or focused owner | Fidelity/status |
| --- | --- | --- |
| `RC-PREREQ-001`–`RC-PREREQ-006` | `requirePinnedAcpxPrerequisites` at `pinned_acpx_test.go:71`, with the six focused admission selectors above | Local host runtime; PASS for exact skip/fail-closed diagnostics |
| `RC-OS-TIMEOUT` | `TestRunBoundedCommandTerminatesScenarioDescendants` and `runTimedOutProcessTreeScenario` in `pinned_acpx_process_test.go` | Local-real OS process tree; PASS in diagnostic and focused run |
| `RC-OS-CRASH` | `TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit` | Local-real OS process tree; PASS in diagnostic and focused run |
| `RC-LAUNCH-FAILURE` | Characterization subtest `launch failure` through `runBoundedCommandWithTimeout` | Local-real executable lookup; PASS, no owned process is created |
| `RC-OWNERSHIP-FAILURE` | Characterization subtest `process ownership failure cleans up the started command` through the controlled ownership seam | Local-real started command plus controlled attach fault; PASS for terminate-and-wait classification; not a claim that a naturally failing OS attach was reproduced |
| `RC-ACPX-PIN` | `preparePinnedAcpx` and `resolvePinnedAcpxPath` | Current-head real ACPX edge UNPROVEN because the diagnostic timed out before acquisition; owner `GATE-FOCUSED-OS` |
| `RC-ACPX-STREAM` | `scenario.run` and `assertPromptEvidence` | Current-head real stdio edge UNPROVEN; owner `GATE-FOCUSED-OS` |
| `RC-SESSION-CREATE` | `newSession`, `requireAcpxSessionResult`, and `assertCreatedSession` | Current-head real session UNPROVEN; owner `GATE-FOCUSED-SESSION` |
| `RC-SESSION-NEGOTIATE` | `loadSessionRecord` and `assertNegotiatedDefaultTarget` | Current-head real persisted negotiation UNPROVEN; owner `GATE-FOCUSED-SESSION` |
| `RC-PROMPT-STREAM` | `prompt` and `assertPromptEvidence` | Current-head real stream/provider result UNPROVEN; owner `GATE-FOCUSED-SESSION`/`GATE-FOCUSED-OS` |
| `RC-PROVIDER-COUNT` | `assertDeterministicProviderInvocations` | Exact two-marker assertion remains in source; current real execution UNPROVEN; owner `GATE-FOCUSED-SESSION` |
| `RC-SESSION-CLOSE` | `closeSession`, `queueOwnerStopped`, and registered cleanup | Current-head real close UNPROVEN; owner `GATE-FOCUSED-SESSION` |
| `RC-BAD-JSON` | Characterization subtests `malformed session JSON` and `malformed prompt JSON` through the existing validators | Controlled output seam; PASS for machine-readable classification; real ACPX wire injection remains unproven |
| `RC-PROTOCOL-ERROR` | Characterization subtest `protocol error` through `validatePromptEvidence` | Controlled frame seam; PASS for protocol-error classification; real peer injection remains unproven |
| `RC-MISSING-ACTION` | Characterization subtest `missing session action` through `parseAcpxSessionResult` | Controlled output seam; PASS for missing-action classification |
| `RC-DUPLICATE-TERMINAL` | Characterization subtest `duplicate terminal result` through `validatePromptEvidence` | Controlled frame seam; PASS for exact-one-terminal rejection |
| `RC-FRAME-ORDER` | `assertPromptEvidence` observes tool-call, update, assistant, and terminal facts; it does not currently assert cross-frame order | Not claimed here; owner `GATE-FOCUSED-SESSION` |
| `RC-PARTIAL-SESSION` | `sessionMayExist` is set before `newSession` runs; `registerSessionCleanup` calls `closeSessionForCleanup`; focused subtests cover success and close-failure state | Controlled cleanup seam; PASS for attempt/state/fail-closed behavior; real ACPX close after malformed output remains unproven |
| `RC-CANCEL` | Existing bounded timer covers timeout; reusable-session cancellation is not in the current package | Not claimed here; owner `GATE-FOCUSED-SESSION`/`GATE-FOCUSED-OS` |
| `RC-SHARED-ISOLATION` | Current three witnesses deliberately allocate independent temp state; no shared session exists before TASK-002 | Not claimed here; owner `GATE-FOCUSED-SESSION` |
| `RC-REPEAT` | No stateful selector was changed before this ledger; repeat is a TASK-002 gate | Not claimed here; owner `GATE-REPEAT` |
| `RC-CLEANUP-FAILURE` | Characterization subtests `partial session close failure stays fail-closed` and `queue owner remains a cleanup failure` | Controlled cleanup seam; PASS for no false successful cleanup evidence |
| `RC-AUTH-NA` | Local credential-free fixture and allowlisted child environment | Not applicable; no authorization contract or credential added |
| `RC-CAPACITY-NA` | No load/capacity claim in this lane | Not applicable; no artificial threshold added |
| `RC-MAXIMUM-NA` | No maximum prompt/frame-size contract in this fixture | Not applicable; no synthetic maximum added |
| `RC-IDEMPOTENCY-NA` | Session creation/prompt/close are not declared idempotent | Not applicable; repetition will prove isolation only, not duplicate-request idempotency |

Every C15 matrix row now has a named current witness, a focused
characterization owner, an explicitly named later gate, or a not-applicable
rationale. Controlled characterization is not substituted for the real ACPX
executable, stream, provider, or persisted-session claims.

## Focused characterization evidence

The following procedure ran after the characterization edit and exited `0`:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^(TestCharacterizePinnedAcpxFailureClassifications|TestRunBoundedCommand)' \
  -count=1 -timeout=5m -v
```

Package wall: `2.838s`. It passed all controlled subtests for malformed
session/prompt JSON, missing action, protocol error, zero or duplicate terminal
results, valid evidence acceptance, launch failure, process ownership failure,
partial-session close success/failure, and queue-owner cleanup failure. It also
passed both real Windows process-tree witnesses. The focused run proves safe
classification and state transitions at the controlled seam; it does not prove
the external ACPX wire or current production executable success.

The refactor preserves the real witness's existing fatal messages through
`requireAcpxSessionResult` and `assertPromptEvidence`. The ownership seam
defaults to the unchanged platform implementation, and the cleanup closer is
only set by controlled characterization. No production behavior, public
contract, skip, timeout bound, sleep, or assertion was weakened.

## GATE-CHARACTERIZATION disposition

| Property | Result |
| --- | --- |
| Current identities | PASS: three pre-edit top-level tests recorded; one explicitly added focused characterization identity recorded after C01 |
| Assertion inventory | PASS: all C15 rows map to a source witness, focused owner, later gate, or N/A rationale |
| C01 reconciliation | PASS: exact inherited `C01-P038`/`C07-F026` rows and added-after-C01 characterization are recorded |
| Direct/descendant starts | PASS: nine pre-edit test-owned parent starts plus two helper descendants and named phase ownership recorded |
| Prerequisite behavior | PASS: six current-head focused selectors preserve optional skip and required fail-closed facts |
| Applicable failure characterization | PASS: controlled classification/cleanup selectors exited `0`; local-real timeout/non-zero helpers exited `0` |
| Diagnostic timing | PASS as retained diagnostic: required current-head run exited `1` on host build-timeout at the existing phase bound; no success claim made |
| Dependency fidelity | PASS for OS helper and executable lookup characterization; current pinned ACPX success remains unproven and is assigned to later gates |
| Properties not proved | Converted reusable-session parity, current-head ACPX/npm/stdio/session success, frame order, cancellation, repeat/race, final timing, hosted coverage, and opposite-OS behavior |

The smallest safe next step is TASK-002: retain the named real ACPX/OS
witnesses while moving only eligible session behavior through one synchronized
`root.BuildProcess`/`Process.Execute` process. TASK-001 does not claim that
conversion or its performance result.
