# C15 — transport/acp/realclient characterization ledger

Status: TASK-002 COMPLETE; TASK-003 REMAINS. The pre-edit diagnostic,
focused failure characterization, reusable-session conversion, and final
focused local-real witnesses are recorded below. The earlier diagnostic hit
the existing one-minute phase bound under host load before ACPX acquisition;
the later committed-head witness completed successfully.

This ledger owns the baseline, assertion map, C01 reconciliation, process
profile, focused characterization, and TASK-002 topology for
`functional-test-optimization-c15-transport-acp-realclient`. Final package
and PR validation remain TASK-003 scope.

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
| `TestCharacterizePinnedAcpxFailureClassifications` | `added-after-C01` (no prior row; no conflict) | C15 characterization identity | `isolated-with-reason` | Controlled parser/cleanup cases plus callback-driven ownership cleanup characterization. It deliberately does not launch an OS process; the retained OS process-tree witnesses remain in the existing functional package rows. |

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
| C15 characterization | launch-failure and ownership cases start no process | Parser and cleanup cases use only temp data; ownership callbacks model terminate-and-wait without creating a command | Safe launch/ownership classification, partial-session state transition, and queue-owner failure. This deterministic characterization cost is not part of the pre-edit denominator. |

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
| `RC-OWNERSHIP-FAILURE` | Characterization subtest `process ownership failure cleans up without another process` through the controlled ownership cleanup seam | Controlled terminate-and-wait callbacks; PASS for cleanup ordering and safe classification, with no claim that a naturally failing OS attach was reproduced |
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
| Direct/descendant starts | PASS: nine pre-edit test-owned parent starts plus two helper descendants and named phase ownership recorded; the added characterization now starts no process |
| Prerequisite behavior | PASS: six current-head focused selectors preserve optional skip and required fail-closed facts |
| Applicable failure characterization | PASS: controlled classification/cleanup selectors exited `0`; local-real timeout/non-zero helpers exited `0` |
| Diagnostic timing | PASS as retained diagnostic: required current-head run exited `1` on host build-timeout at the existing phase bound; no success claim made |
| Dependency fidelity | PASS for OS helper and executable lookup characterization; current pinned ACPX success remains unproven and is assigned to later gates |
| Properties not proved | Converted reusable-session parity, current-head ACPX/npm/stdio/session success, frame order, cancellation, repeat/race, final timing, hosted coverage, and opposite-OS behavior |

The smallest safe next step is TASK-002: retain the named real ACPX/OS
witnesses while moving only eligible session behavior through one synchronized
`root.BuildProcess`/`Process.Execute` process. TASK-001 does not claim that
conversion or its performance result.

## TASK-002 partial conversion and lifecycle boundary

The first bounded conversion is implemented in
`tests/functional/transport/acp/realclient/reusable_acp_session_test.go`.
`TestReusableACPServerTurnsThroughOneProcess` builds one canonical
`root.BuildProcess`, starts one ACP server through `Process.Execute`, and
serially drives two prompt turns through the same ACP connection. The only
replaced external effect is `edges.Edges.ProviderCommandRunner`. Each turn has
its own 16-character input, output marker, and provider-call accounting; the
test asserts a non-empty marker-specific assistant result, Worker
`tool_call`, Worker `tool_call_update`, their order, `end_turn`, and exactly
two provider calls. A second `session/new` uses a distinct workspace and
identity on that same connection without activating another Factory runtime.

Focused repetition evidence:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=2 -timeout=15m -v
```

The command exited `0`; both repetitions passed all subtests. Package wall was
`32.461s` on a noisy local host, so this is repeat/isolation evidence only and
not a three-second timing claim.

The package regression selector also exited `0`:

```text
go test ./tests/functional/transport/acp/realclient -count=1 -timeout=15m -v
```

The two local-real process-tree witnesses and the controlled characterization
passed; the pinned ACPX test reported its existing optional prerequisite skip
because this host has Node `v22.12.0`. No real-client success or current-head
ACPX evidence is claimed from that run.

Because the converted witness shares live ACP state across turns, its focused
race procedure also exited `0`:

```text
go test -race ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=15m -v
```

The reusable server/turn assertions passed with no race report; the race run
took `38.738s` locally under instrumentation.

The required two-independent-session probe remains blocked at the existing
production lifecycle boundary. Two sessions that each reach their first
prompt through the same root-built process were tried serially with both the
same target (`factory:@you/factory-builder`) and different named targets. The
first activation completes; the second `StartAsync` fails with the bounded
ACP `dependency_unavailable` response while opening the retained runtime.
`FactorySessionIDGenerator` and `FactorySessionRuntimeInstanceIDGenerator`
edges do not change the runtime's internal fixed `~default` session scope.
Calling `Process.Close` between cases closes process-wide roots and does not
make the process reusable. The public ACP `session/close` path also rejects a
completed terminalized prompt under the current contract, matching the
existing root-composition rationale in
`tests/functional/sessions/chat_sessions/root_composition/acp_controlled_cohort_test.go`.

This is a structured blocker, not a success or skip: solving it requires a
production lifecycle/resolver change or a new test seam, both outside TASK-002
scope (`Production ACP changes` and `Shared-support changes`). The converted
turn slice is proven; RC-SESSION-CREATE/NEGOTIATE is proven for the shared
process, while RC-SHARED-ISOLATION, full two-session prompt parity,
RC-SESSION-CLOSE on the converted path, final package timing, and PR evidence
remain unproven. The retained pinned ACPX/OS witnesses and all existing
failure characterization remain unchanged pending an owner decision on this
boundary.

## TASK-002 bounded close and cancellation evidence

The reusable process slice now also drives an in-flight prompt and sends
`session/close` over the same ACP connection before the provider edge returns.
The command-runner edge waits on its invocation context, so the production
Factory Sessions close control cancels the prompt without a sleep or a second
application process. A single response reader correlates the canceled
`session/prompt` and successful `session/close` responses by JSON-RPC id while
retaining interleaved `session/update` frames. This proves converted close and
cancellation at the controlled provider edge; the pinned ACPX queue-owner
assertion remains owned by the retained local-real witness.

Focused procedure, after the close/cancellation edit:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=15m -v
```

The command exited `0` with a package wall of `10.334s`. The existing two
marker-isolated turns passed their non-empty assistant, Worker
`tool_call`/`tool_call_update` ordering, `end_turn`, and exact-two-provider
assertions; the additional active-turn case returned `cancelled` and its
`session/close` response decoded successfully on the same process and
connection.

Repeat and synchronization evidence also exited `0`:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=2 -timeout=15m -v
```

Both repetitions passed; package wall was `12.785s` on the noisy local host.

```text
go test -race ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=15m -v
```

The race command exited `0` with a package wall of `14.579s` and no race
report. The changed parser, process, and cleanup characterization remained
green:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^(TestCharacterizePinnedAcpxFailureClassifications|TestRunBoundedCommand)' \
  -count=1 -timeout=5m
```

It exited `0` in `2.840s`, including both local-real process-tree witnesses
and the controlled malformed-output, protocol, ownership, partial-session,
and cleanup-failure branches.

These results extend the converted proof to RC-SESSION-CLOSE and RC-CANCEL
for the one already-bound session, but do not prove RC-SHARED-ISOLATION or
full two-independent-session prompt parity. A second `session/new` still
produces a distinct identity and workspace, while its first Factory prompt
cannot be admitted after the first activation because production retains the
fixed `~default` runtime scope. Story 002 therefore remains blocked pending
the previously recorded lifecycle/placement decision; no production or
shared-support change was made.

## TASK-002 final conversion evidence

The final reusable witness keeps one package-scoped `root.BuildProcess`, one
`Process.Execute` ACP server, one ACP connection, and one injected
`ProviderCommandRunner`. It first drives two marker-isolated turns through one
session, then creates a second session with a distinct workspace and identity.
The first session is canceled and closed while its provider call is in flight;
only after that close does the second session receive its first prompt. The
second prompt returns its own marker, Worker updates, and exact provider count.
Every retained prompt update is checked for the requested session identity,
and later turns reject earlier output markers. This proves the supported
serial isolation topology without claiming concurrent retained runtimes.

The public close contract requires an active turn. The reusable test therefore
uses the in-flight prompt/`session/close` pair for cancellation and successful
close; a completed prompt followed by `session/close` is not treated as a
normal-close witness. The pinned ACPX witness retains the real
`session_closed` identity and zero queue-owner assertion.

Focused procedures on committed head `6abbfeb9a1dae0464d76cd30843b0a38b6152621`:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt$' -count=1 -timeout=10m -v
```

With Node `v24.19.0` from the preinstalled Codex runtime prepended to `PATH`
and both real-client gates set to `1`, the exact `acpx@0.13.0` witness exited
`0`; package wall was `117.457s`. It logged the committed revision, negotiated
initialization, created session, target `factory:@you/factory-builder`,
non-empty assistant result, `end_turn`, exactly two `codex` provider
invocations, complete cleanup, and outcome `passed`. The elapsed time is
contaminated by host contention and is retained as local-real OS-client
evidence, not as the package-level performance verdict.

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=2 -timeout=15m -v
```

The changed stateful selector exited `0`; both repetitions passed with package
wall `67.814s`. The command used distinct temporary session workspaces and
per-marker provider observations on each repetition. The package-local race
gate also exited `0` with no race report:

```text
go test -race ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=15m -v
```

Its package wall was `36.326s`. The focused failure and OS gate exited `0` in
`3.154s`:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^(TestCharacterizePinnedAcpxFailureClassifications|TestRunBoundedCommand)' \
  -count=1 -timeout=5m -v
```

That gate passed malformed session/prompt JSON, protocol error, missing
action, missing and duplicate terminal results, launch and ownership
classification, partial-session cleanup, queue-owner cleanup, timeout tree
termination, non-zero crash cleanup, and descendant inactivity. No production
or shared-support file changed. The earlier retained-runtime probe remains a
historical boundary observation; the close-before-next-activation sequence is
the final supported shared-process topology.

### TASK-002 retained-process irreducibility

The converted session witness has no test-owned child process or private root;
its application process is built once and its ACP stream is driven through
`Process.Execute`. The remaining local-real rows are:

The costs below are standalone phase or witness measurements, not another
package aggregate. The Node, Git, and bounded-build values came from the latest
supported pinned selector attempt with Node `v22.13.0`; that host-contaminated
attempt reached admission and revision lookup before the existing one-minute
build bound fired. The npm value came from the same pinned package command
against a fresh disposable cache and exited `0`; its host `npx` launcher emitted
the expected Node `v22.12.0` engine warning while still reporting `0.13.0`.
The ACPX values came from a temporary local-only probe (removed after the run)
that used the current production
binary and the acquired `acpx@0.13.0` CLI, with the same scenario environment
and assertions. The process-tree values came from the focused retained-witness
selector, whose new logs measure the complete command plus descendant-inactive
observation.

| Retained phase/witness | Measured cost | Exact property | Why reuse cannot prove it |
| --- | ---: | --- | --- |
| `node --version` admission | `50.847ms` (Node `v22.13.0`; supported selector reached this phase) | Select a Node executable meeting the pinned ACPX minimum before startup | An in-process root cannot establish the external Node executable/version boundary. |
| `git rev-parse HEAD` | `167.630ms` (supported selector) | Attach sanitized evidence to the current source revision | The evidence contract requires the repository revision from the current checkout. |
| `go build -o <temp>/you[.exe] ./cmd/factory` | `60.027s` (supported selector; existing timeout classification) | Build the current production executable before ACPX-configured startup | `root.BuildProcess` is the controlled application path and cannot prove the shipped executable boundary. The host did not permit a successful bounded build in this measurement; the earlier successful local-real witness remains recorded separately. |
| `npx --yes --package acpx@0.13.0 acpx --version` | `36.856s` (fresh disposable cache; exit `0`, output `0.13.0`) | Acquire and verify the exact third-party ACPX package | A reusable application process cannot prove npm resolution or the pinned external client. |
| ACPX `sessions new` direct Node phase | `2.779s` (temporary phase probe; exit `0`) | Real ACP stdio session creation, persisted client identity, and negotiated setup | The controlled connection proves server behavior but cannot prove the independently packaged ACPX client and its session state. |
| ACPX `prompt` direct Node phase | `4.567s` (temporary phase probe; exit `0`) | Real ACP stdio framing, provider stream, Worker updates, assistant result, and terminal result | A reusable server connection cannot prove the independently packaged ACPX client or its external stream parser. |
| ACPX `sessions close` direct Node phase | `2.860s` (temporary phase probe; exit `0`) | Real close identity and ACPX queue-owner shutdown | The controlled connection cannot prove the independently packaged ACPX queue owner and client shutdown path. |
| `TestRunBoundedCommandTerminatesScenarioDescendants` | `1.732s` (focused selector; exit `0`) | Real timeout, process-tree ownership, and descendant inactivity | Sharing would remove the held OS parent/descendant boundary. |
| `TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit` | `720ms` (focused selector; exit `0`) | Real non-zero crash classification, tree cleanup, and descendant inactivity | Sharing would remove the non-zero exit and process-tree boundary. |

The final package timing and PR functional coverage remain TASK-003 evidence;
this table records why the retained OS/client rows cannot be replaced by the
reusable controlled witness.

### TASK-002 review correction: provider-edge mapping and OS-witness placement

The reusable fixture now retains the `session/new` `cwd` in a
`reusableACPSession` record instead of discarding it. Every session/turn case
passes its expected prompt and the installed Factory directory to the
provider-edge runner. The runner records every invocation's `WorkDir`, input
prompt, and case marker; the test asserts all three for both provider calls in
each turn, including the first prompt of the second session and the
in-flight prompt used by close/cancellation. The evidence distinguishes the
client workspace (`session.workspace`, the distinct `session/new` `cwd`) from
the provider execution workspace (`session.providerWorkDir`, the resolved
Factory directory), so the assertion does not incorrectly equate ACP client
cwd with the provider's production working directory.

Current focused evidence after this correction:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=5m -v
```

Exited `0` with package wall `8.73s`. The two marker-isolated turns, the
in-flight close/cancellation turn, the second session's first turn, and all
provider `WorkDir`/prompt/marker observations passed.

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=2 -timeout=10m -v
```

Exited `0` for both repetitions with package wall `24.142s`. The provider
observation assertions passed on each fresh fixture repetition; no state or
marker crossed repetitions.

```text
go test -race ./tests/functional/transport/acp/realclient -run \
  '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=10m -v
```

Exited `0` with package wall `19.265s` and no race report. The focused
characterization and retained local-real OS selectors also exited `0` in
`2.435s`:

```text
go test ./tests/functional/transport/acp/realclient -run \
  '^(TestCharacterizePinnedAcpxFailureClassifications|TestRunBoundedCommandTerminatesScenarioDescendants|TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit)$' \
  -count=1 -timeout=10m -v
```

The process-ownership characterization was deliberately changed to a
non-OS deterministic callback case. Its placement decision is therefore to
keep the safe classification and terminate-then-wait assertion in this
package's controlled characterization, while retaining the existing C01
local-real timeout/non-zero process-tree witnesses for the executable,
exit, ownership, and descendant-inactivity properties. No additional
`os.Args[0]` process launch or modified OS witness was added, and no
integration mirror or production/shared-support change is needed for this
bounded correction.

## TASK-003 bounded optimization and final validation

The prior PR review measured the authoritative package result at `46.241s`
against the admitted `40.486s`. One bounded optimization pass was applied in
`566aff6e82` by removing the redundant second successful turn from the
controlled reusable witness. The retained cases still execute one successful
turn in the first session, an in-flight cancellation/close, and one successful
turn in a distinct second session with distinct workspace, prompt, provider
marker, session-bound updates, result, and cleanup assertions. The pinned
ACPX witness and the retained timeout/non-zero process-tree witnesses were not
weakened or removed. The second-session marker assertion now rejects the first
session's `alpha1` result; the removed `bravo2` turn was a duplicate controlled
success case rather than an assertion from the original pinned real-client
matrix.

A separate setup experiment copied the generated Factory fixture directly in
an attempt to avoid the normal packaged-installation command. The focused
reusable selector then held in production initialization/reconciliation until
its `15m` test bound. That shortcut was fully reverted; the final fixture uses
the public `InstallPackagedFactoryWithProcess` path and makes no support or
production change.

Current bounded-pass evidence:

```text
go test ./tests/functional/transport/acp/realclient -run '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=15m -v
```

Exited `0`; package wall `2.322s`.

```text
go test ./tests/functional/transport/acp/realclient -run '^TestReusableACPServerTurnsThroughOneProcess$' -count=2 -timeout=15m -v
```

Exited `0`; both repetitions passed; package wall `5.299s`.

```text
go test -race ./tests/functional/transport/acp/realclient -run '^TestReusableACPServerTurnsThroughOneProcess$' -count=1 -timeout=15m -v
```

Exited `0`; package wall `6.388s`; no race was reported.

```text
go test ./tests/functional/transport/acp/realclient -run '^(TestCharacterizePinnedAcpxFailureClassifications|TestRunBoundedCommandTerminatesScenarioDescendants|TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit)$' -count=1 -timeout=10m -v
```

Exited `0`; package wall `1.931s`. Both retained process-tree witnesses and
the controlled characterization branches passed.

```text
go test ./tests/functional/transport/acp/realclient/... -count=1 -timeout=15m -v
```

Exited `0`; package wall `5.586s`. The optional pinned real-client test was
skipped because this host has only Node `v22.12.0`, below the required
`v22.13.0`; no supported Node executable was available for a current-head
local-real rerun. This proves ordinary package coexistence and cleanup, but
does not replace the pinned executable/ACPX edge.

```text
make test
```

Exited `0`. The final Node test stage reported `170` tests, `169` passed,
`1` skipped, `0` failed, and `duration_ms=125724.6233`. The skip is the
existing environment-dependent Windows path-bridge case; no new skip was
introduced by this lane.

The source-plan reference remains an explicit planning conflict. The exact
check `Test-Path docs/temp/functional-test-optimization.md` returned `False`;
the path is absent from `origin/main` and from repository history inspected
with `git ls-tree`/`git log`. No operator amendment authorizes inventing or
reconstructing it. The smallest resolution is for the plan owner to restore
that source plan or amend `context.sourcePlan` before the delivery lane can
be promoted.

# Validation report: BEH-C15-ACP-DELIVERY

## Environment and artifact

- Commit/build identifier: implementation code under validation is
  `566aff6e82`; the report commit is documentation-only.
- Environment and configuration: Windows PowerShell host; Go package tests
  run with the existing deterministic provider edge; only Node `v22.12.0` is
  installed locally, while the pinned ACPX gate requires `v22.13.0+`.
- Customer entry point: `go test ./tests/functional/transport/acp/realclient/...`
  exercising the realclient package's pinned ACPX, process-tree, and reusable
  ACP server witnesses.
- Real and substituted dependencies: current reusable session proof uses one
  production `root.BuildProcess` and one `Process.Execute` server with only
  `ProviderCommandRunner` replaced; the current host could not run the
  pinned external Node/ACPX dependency.
- Cost/call budget used: zero provider API calls; one bounded local
  optimization pass; one full `make test` run; no additional executable or
  support-surface shortcut retained.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| P-01 Complete matrix parity | BLOCKED | Current controlled selectors and the committed ledger preserve the mapped protocol, lifecycle, error, cancellation, framing, and cleanup assertions. | The exact current-head full local-real pinned matrix could not run with Node `v22.12.0`. |
| P-02 One shared production process for eligible session behavior | PASS | Current reusable selector passed with one package-scoped `root.BuildProcess`/`Process.Execute` server, distinct sessions/workspaces, and provider-edge observations. | Arbitrary concurrent load is not proved. |
| P-03 Irreducibility rows for retained boundaries | PASS | Ledger contains measured Node, Git, build, npm, ACPX session/prompt/close, timeout-tree, and non-zero-tree rows with property and reuse rationale. | Future host variance remains unproved. |
| P-04 Local timing target or fallback | PASS | Current package wall was `5.586s`; the complete per-test irreducibility table is published. | The under-three-second target is not met. |
| P-05 PR timing direction and bounded response | BLOCKED | Prior PR result was `46.241s` versus `40.486s`; commit `566aff6e82` is the one bounded optimization pass. | Current-head hosted timing is pending. |
| P-06 PR functional coverage floor | BLOCKED | Prior review evidence recorded `61.6%` at the earlier head. | Current-head coverage has not been reported. |
| P-07 Characterization gate | PASS | Current characterization/process selector exited `0` in `1.931s`; full `make test` also exited `0`. | Unsupported Node prevented repeating the pinned prerequisite on this host. |
| P-08 Focused reusable-session gate | PASS | Single, repeat, and race reusable selectors exited `0`; provider WorkDir/prompt/marker and session-bound frame assertions passed. | No concurrent multi-runtime claim is made. |
| P-09 Focused OS gate | PASS | Retained timeout/non-zero process-tree selectors passed; the unchanged pinned witness and per-phase rows remain in the committed ledger. | A fresh current-head pinned ACPX run is unavailable locally. |
| P-10 Repeat/race gates | PASS | `-count=2` passed both repetitions; package-local `-race` passed with no race report. | Exhaustive race schedules are not proved. |
| P-11 Final package gate | BLOCKED | Ordinary package command exited `0` in `5.586s`; `make test` exited `0`. | The pinned test skipped on the unsupported local Node, so complete local-real final behavior is not current-head proved. |
| P-12 Current-head PR functional gate | BLOCKED | Existing PR #2511 has the prior review result and receives this bounded correction. | Current-head package timing, coverage, and suite coexistence still require CI. |
| P-13 Scope and prohibited-change gate | PASS | The bounded code diff is confined to the owned reusable test; no new skip, sleep, timeout inflation, assertion weakening, credential/data retention, production, or shared-support change was retained. | Repository-wide future drift is outside this report. |
| P-14 VAL-001 clean final-head report | BLOCKED | This report follows `factory/docs/standards/validation-loopback-template.md` and reports every project criterion without repair. | The required source plan is absent, and this report was prepared before the final push/current-head CI observation. |
| P-15 Implementation handoff | BLOCKED | PR #2511 is open and the next step is to push the report head and observe required CI start. | At report creation the bounded-pass head was not yet pushed and current-head CI had not started. |
| S003-01 Clean final package matrix | BLOCKED | The ordinary package passed with no failure, but the pinned test was skipped by the host Node prerequisite. | Current-head local-real ACPX success and zero residue are not freshly proved. |
| S003-02 Local timing disposition | PASS | Actual package wall `5.586s` is recorded and the complete retained-process/client fallback table is present. | Under-three-second timing is not achieved. |
| S003-03 Retained OS rows | PASS | Each retained row names its exact executable, stream, exit, shutdown, crash, or descendant-cleanup property and measured cost. | Measurements are host-contaminated and not a promise of hosted latency. |
| S003-04 PR timing/coverage result | BLOCKED | The prior non-improving timing triggered the bounded pass; no current-head result exists yet. | Hosted package timing direction and `>=61.6%` coverage remain unproven. |
| S003-05 Non-improving result after bounded pass | BLOCKED | The bounded pass is implemented and does not weaken behavior. | The current PR result is needed to determine whether a post-pass blocker remains. |
| S003-06 Missing/stale evidence disposition | BLOCKED | Missing source-plan authority and unavailable current local-real dependency are explicitly recorded; no repair was invented. | Plan-owner resolution and current-head CI evidence are required. |
| S003-07 Final pushed/open/CI-started handoff | BLOCKED | PR #2511 is open; the final push and required check-start observation are the next controlled actions. | The final report head is not yet remotely confirmed. |
| S003-08 Implementation/review ownership | BLOCKED | The report assigns terminal CI, conflicts, and merge to review. | Handoff cannot be complete until the final head is pushed and CI starts. |

## Customer journey

1. A caller enters the realclient package through its declared Go test
   command. The ordinary package command exits `0` in `5.586s`; the pinned
   external test reports an environment skip because Node `v22.12.0` is below
   the required minimum.
2. The reusable ACP path builds one production process, starts one ACP server,
   negotiates ACP v1, creates distinct session identities and workspaces, and
   completes marker-isolated turns through the same connection. Current
   single, repeat, and race selectors pass.
3. The first session's active prompt is canceled and closed; the second
   session then completes with its own provider workspace, prompt, marker,
   session-bound updates, assistant result, and terminal outcome.
4. The retained local-real helpers prove timeout and non-zero process-tree
   termination, while the ledger retains the external Node/npm/ACPX phase
   costs and explains why those boundaries cannot be converted to an
   in-process proof.
5. `make test` exits `0` with no failures. The remaining delivery evidence is
   current-head PR timing/coverage and an approved resolution of the absent
   source-plan reference.

## Cross-task integration and usability

- Documentation discoverability: the package-owned ledger records the
  characterization, conversion, irreducibility, bounded optimization, and
  validation report; the referenced source plan is absent and remains a
  blocker.
- Permission and error behavior: provider-edge failures, cancellation, close,
  malformed/protocol/terminal classifications, and cleanup assertions remain
  covered by focused selectors.
- Persistence/reload behavior: ACP session identity/workspace isolation and
  disposable Factory/session cleanup are asserted; no durable customer data is
  retained.
- Accessibility/keyboard/responsive behavior: not applicable to this backend
  transport test lane.
- Operational signals: phase timing is logged before fail-fast paths;
  package, race, repeat, and full-suite outcomes are recorded without CI
  results being committed.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| F-001 | BLOCKED | `Test-Path docs/temp/functional-test-optimization.md`; inspect `origin/main` and history. | `context.sourcePlan` resolves to an approved source plan. | The path is absent and no operator amendment authorizes reconstruction. | Exact check returned `False`; no matching tree/history entry. |
| F-002 | BLOCKED | Inspect prior PR timing, then evaluate current head. | Package timing improves directionally after one bounded response, or the report gives a measured blocker. | Prior PR timing was `46.241s` vs `40.486s`; the bounded pass is now present, but current-head timing is pending. | Review finding; commit `566aff6e82`; current PR CI required. |
| F-003 | BLOCKED | Run the declared final package command with required Node. | Every final matrix witness passes on the final head. | This host has only Node `v22.12.0`; the pinned test skips rather than claiming real-client evidence. | Current package exit `0`, package wall `5.586s`, pinned prerequisite skip. |
| F-004 | PASS | Run `make test`. | Full repository gate completes without a diff-caused failure. | Exit `0`; final Node stage: 169 passed, 1 skipped, 0 failed. | Local command result. |

## Verdict

BLOCKED

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: `BEH-C15-ACP-DELIVERY`, especially P-01,
  P-05/P-06, P-12, P-14, and S003-01/S003-04/S003-06.
- Root-cause evidence or remaining uncertainty: the referenced
  `docs/temp/functional-test-optimization.md` source plan is absent; the
  local host cannot execute the required Node `v22.13+` pinned ACPX path; and
  the prior hosted timing was non-improving, so the bounded pass needs a
  current-head hosted measurement.
- Smallest recommended correction/prerequisite: the plan owner should restore
  the source plan or amend `context.sourcePlan`; then inspect the current PR
  head's package timing, coverage, pinned evidence, and suite result. If the
  post-pass timing remains non-improving, request a new bounded plan delta
  rather than weakening an assertion or deleting another behavior witness.
- Dependencies and retest scope: approved source-plan resolution; current
  PR Backend Functional Coverage/latency and coverage jobs; one clean
  final-head realclient package run on Node `v22.13+` if available; review
  owns terminal CI, conflicts, and merge.
