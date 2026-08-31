# Transport protocol-family characterization: ACP stdio

This ledger is the package-local evidence for
`functional-test-optimization-c15-transport-protocol-family-001`.
It freezes ACP-01..08 before fixture topology changes. It is a
characterization record, not post-change parity or performance evidence.

## Artifact and provenance

| Field | Value |
| --- | --- |
| Package | `github.com/portpowered/infinite-you/tests/functional/transport/acp/stdio` |
| Package ID | `F027` |
| Current HEAD | `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Base / merge base | `origin/main` / `42eeee4472656b8290f798c36a5b8c871b24d7d0` |
| Current source SHA-256 | controls `47281af459f1908e0ef715f2cae5d91759a4748ef480be3a72ed9a6a342ad39b`; prompt `8225de68479e84603887bf81e12227545fdbbe2b9f9d5ffec18b2d36f15f788b`; wire `47ac861be2a5cd44ae2f08235d4689c8f9819ce61621b60c49b16d061ccf7499` |
| c01 source SHA-256 | controls `d3a63a6ccc9f5bac03f76ce3f6ca3327000b8b7831b07ac311057ae90efb5516`; prompt and wire match current |
| c01 classification source | `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json`, recorded commit `eef5150f277490384b460a47a0a3bcca51338e67` |
| c01 classification | All eight rows: `isolated-with-reason`, `propertyType=stdio`, `requiredFidelity=local_real`, OS audit `INTENTIONAL-OS`, required property `stream-file-descriptor-behavior`, process observations `PSO-0043`..`PSO-0050` |
| Go / host | `go1.25.0 windows/amd64` |
| Discovery | `go test -list '^Test' -count=0 -timeout=5m ./tests/functional/transport/acp/stdio` exited 0; 8 top-level declarations |

The controls source hash differs from the c01 record because the current file
includes the post-inventory synchronization changes from commits
`1c339cf04a` and `9c963105e1`. The current identities, line-level witnesses,
classification, and assertions remain present. The c01 inventory is excluded
from this lane; its owner may refresh that file's source hash in a separate
delta.

The supplied admission median for this package is **21.128s**. It is an
operator-supplied pre-optimization observation from the PRD and is not a
current comparable median. The one permitted current-head diagnostic was:

```text
go test -v -count=1 -timeout=5m ./tests/functional/transport/acp/stdio -run '^TestServeACP_RootBuildProcessCompletesOneFactoryPrompt$'
exit 0
package-reported: 1.673s (test body 1.59s)
measured command wall: 6.688s
```

The diagnostic passed the real initialize/session/new/prompt exchange, exact
assistant text and end-turn response, provider working-directory and call
count checks, stdout protocol/EOF checks, stderr leak check, and process
completion. The package has no shared topology counter. Source-owned topology
for this selector is one `root.BuildProcess` process, one controlled
`ProviderCommandRunner` call, one ACP `Process.Execute` command, one ACP
Session, one stdin pipe pair, one stdout pipe pair, one disposable HOME, one
working directory with a fixture Factory, and per-test process/pipe cleanup.

## Census and classification rules

The eight matrix rows are eight top-level tests with no named subtests. Every
row uses `testing.Short()` only to preserve the existing explicit integration
skip when the caller requests `-short`; the normal characterization command
did not request short mode and no row was skipped. There is no quarantine,
sleep, or stable-window behavior. Existing five-second `time.After` branches
are documented in source as hang guards around deterministic pipe/process
completion or channel observations; they are not synchronization evidence.

The actual ACP stdin/stdout pipes, JSON-RPC framing, response/update ordering,
Session lifecycle, transcript contents, failed-write behavior, and transcript
permission checks remain local-real. Proposed owners below may share only the
application graph through a case-keyed controlled provider/recorder edge and
must create fresh HOME, cwd, Session, pipe endpoints, context, frame reader,
provider route, and cleanup for each row.

## ACP-01..08 exact witness map

| Case | Current test (source line) | Exact observable assertions | Current resource owner and genuine boundary | c01 record | Proposed owner |
| --- | --- | --- | --- | --- | --- |
| ACP-01 | `TestServeACP_RootBuildProcessProviderFailureTerminalizesPrompt` (`cli_serve_acp_controls_test.go:31`) | Initialize and `session/new` succeed with nonblank Session ID; first prompt returns JSON-RPC error code `-32603`; a second prompt returns `StopReasonEndTurn`; controlled provider call count is exactly `2`; stdin EOF makes `Process.Execute(you server acp)` return successfully. | `newServeACPControlHarness` owns one root process, failure-on-call-1 command-runner edge, fresh HOME/cwd/Factory/profile, ACP stdin/stdout `os.Pipe` pairs, Session, prompt frames, command, stderr, and `finish`/`t.Cleanup` closes. Genuine boundary: provider failure terminalizes the real ACP turn and releases the Session. | `C07-F027-top-level-test-TestServeACP-RootBuildProcessProviderFailureTerminalizesPrompt`; `isolated-with-reason/stdio`; `PSO-0043`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-CONTROL-PROMPT-ROOT`: shared graph candidate with a case-keyed failure/success runner; fresh Session, pipes, frames, and EOF cleanup. |
| ACP-02 | `TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt` (`cli_serve_acp_controls_test.go:67`) | First blocked prompt starts; `session/cancel` yields `StopReasonCancelled`; provider call 1 observes cancellation; next prompt ends turn; a post-completion cancel does not affect the following prompt; final prompt ends turn; provider call count is exactly `3`; command returns after stdin EOF. | Same control harness with blocking call 1, cancellation channel, real ACP notification/response stream, fresh Session and pipe endpoints; `responses` is correlated by ID. Genuine boundary: cancellation reaches only the captured prompt and does not cancel later work. | `C07-F027-top-level-test-TestServeACP-RootBuildProcessCancelTerminalizesOnlyCapturedPrompt`; `isolated-with-reason/stdio`; `PSO-0044`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-CONTROL-PROMPT-ROOT`: shared graph candidate with fresh blocking runner channels and per-case prompt/Session state. |
| ACP-03 | `TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession` (`cli_serve_acp_controls_test.go:102`) | Blocked prompt is canceled; `session/close` succeeds; both prompt and close responses are observed after the public prompt-terminal barrier; provider call 1 observes cancellation; a later prompt returns a closed-session error; provider call count is unchanged after rejection; command returns cleanly on EOF. | Same control harness with blocking runner, response correlation, close request, captured Factory Session, ACP pipes, command, and cleanup. Genuine boundary: close cancels active dispatch, closes the Session, and prevents provider re-execution. | `C07-F027-top-level-test-TestServeACP-RootBuildProcessCloseStopsCapturedFactorySession`; `isolated-with-reason/stdio`; `PSO-0045`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-CONTROL-PROMPT-ROOT`: share graph only; retain per-case close sequence, Session, pipes, and terminal barrier. |
| ACP-04 | `TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities` (`cli_serve_acp_controls_test.go:140`) | First prompt ends turn and emits at least one Worker tool-call ID with no live user-message IDs; second blocked prompt is canceled by close; close succeeds; `session/load` succeeds; loaded Worker IDs begin with the original IDs in order; loaded user-message IDs contain two nonblank IDs; provider call count is exactly `2`; command closes cleanly. | One control harness owns the completed-then-active Session sequence, blocking call 2, ACP update frames, retained identity replay, pipes, context, and cleanup. Genuine boundary: sequenced item identity/order and no provider re-execution across close/load. | `C07-F027-top-level-test-TestServeACP-RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities`; `isolated-with-reason/stdio`; `PSO-0046`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-CONTROL-PROMPT-ROOT`: application root may be shared, but the close/load identity sequence and Session remain isolated per case. |
| ACP-05 | `TestServeACP_RootBuildProcessCompletesOneFactoryPrompt` (`cli_serve_acp_prompt_test.go:66`) | Every stdout line is a JSON-RPC frame; initialize and `session/new` succeed; Session ID is nonblank; exactly one selected config option has current value `fixtureFactoryTargetID`; assistant message text equals `fixtureFinalAnswerText`; prompt has no error and `StopReasonEndTurn`; stdin EOF returns command cleanly; provider call count is `1`; provider `WorkDir` equals the selected Factory directory under the supplied project root; stderr never contains the answer text. | One root process with shaped provider runner; seeded project-local Factory/profile; fresh HOME/cwd; real stdin/stdout pipes and JSON-RPC reader; one Session/prompt; command completion, stdout drain, and process/pipe cleanup. Genuine boundary: caller-supplied `session/new` cwd reaches the one downstream provider execution and clean protocol return. | `C07-F027-top-level-test-TestServeACP-RootBuildProcessCompletesOneFactoryPrompt`; `isolated-with-reason/stdio`; `PSO-0047`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-CONTROL-PROMPT-ROOT`: shared graph candidate with fresh project Factory/profile, provider route, Session, pipes, and cwd. |
| ACP-06 | `TestServeACPWritesAWireTranscriptByDefault` (`cli_serve_acp_wire_log_test.go:42`) | Initialize response is successful and visible in the transcript before return; five 600 KiB malformed frames each return an error; `session/new` succeeds with nonblank Session ID; two prompts each emit exact `fixtureFinalAnswerText` and `StopReasonEndTurn`; each response makes outbound transcript records visible; provider calls exactly `2`; at least active plus rotated backup transcript files exist; every inbound/outbound record has contiguous sequence and current format version, correct client/agent peer, exact bytes or oversized prefix, normalized frame equality, and exactly five malformed inbound records. | Root process with production `wiretranscript.Opener` configured through `ACPWireRecorder` for 1 MiB rotation; four real pipe endpoints; two provider calls; fresh home/cwd/profile/Factory; transcript files and command EOF cleanup. Genuine boundary: actual frame bytes/order, rotation, peer attribution, publication timing, and malformed-frame retention. | `C07-F027-top-level-test-TestServeACPWritesAWireTranscriptByDefault`; `isolated-with-reason/stdio`; `PSO-0048`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-TRANSCRIPT-ROTATION`: retain isolated recorder/pipe cohort; sharing would mix bytes, sequence, files, rotation, and publication ownership. |
| ACP-07 | `TestServeACPDoesNotRecordFailedOutboundFrame` (`cli_serve_acp_wire_log_test.go:466`) | `Process.Execute(you server acp)` fails with the public `connection ended with an error` diagnostic when stdout writer fails; transcript contains exactly one record and it is the inbound initialize frame, with no failed outbound record. | Root process with production rotating recorder edge, fresh HOME/cwd, one real input stream, failing output writer, stderr, transcript file, and process cleanup. Genuine boundary: failed outbound bytes are not falsely recorded. | `C07-F027-top-level-test-TestServeACPDoesNotRecordFailedOutboundFrame`; `isolated-with-reason/stdio`; `PSO-0049`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-OUTBOUND-FAILURE`: isolated writer-failure/recorder cohort; failure ownership and transcript cardinality must remain one invocation. |
| ACP-08 | `TestServeACPWireTranscriptIsOwnerReadableOnly` (`cli_serve_acp_wire_log_test.go:519`) | One initialize connection succeeds; exactly one transcript exists under `<HOME>/.you-agent-factory/acp-wire`; on Windows owner read/write bits are retained, and on non-Windows mode is exactly `0600`. | Root process with default production transcript recorder; fresh HOME/cwd, one initialize input, stdout/stderr buffers, one transcript path, and process/temp cleanup. Genuine boundary: owner-only filesystem permission for full prompt/response content. | `C07-F027-top-level-test-TestServeACPWireTranscriptIsOwnerReadableOnly`; `isolated-with-reason/stdio`; `PSO-0050`; `INTENTIONAL-OS/stream-file-descriptor-behavior`. | `ACP-PERMISSION-ISOLATED`: retain fresh HOME and one-connection transcript; permission ownership cannot be inferred from a shared artifact. |

## Evidence boundary and handoff

The list-only run and source review prove that all eight ACP rows have current
identities, exact protocol/content/lifecycle assertions, resource owners, c01
classification, and proposed owners. The focused selector proves one
local-real ACP pipe/session/provider prompt, clean EOF return, transcript-safe
stdout/stderr behavior, and cleanup. It does **not** prove optimized parity for
all ACP rows, wire-rotation or permission rows after restructuring, package
timing under PR-CI, coverage, repeat/race behavior, ACPX executable behavior,
Unix semantics, terminal CI, or merge. Those edges belong to GATE-ACP,
GATE-RACE-ACP, GATE-PERF, GATE-COVERAGE, GATE-LOOP, the excluded
`transport/acp/realclient` owner, and GATE-PR.

The source-plan artifact named by the PRD is ignored and absent in this
worktree. The PRD/task packet supplies the matrix used here. No source-plan,
shared support, or c01 canonical inventory file was edited; the controls-file
hash refresh is a narrow delta request to the inventory owner.
