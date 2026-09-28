# MCP subagent reliability log — 2026-09-28

This log records observed `you.subagent` behavior during the LocalAI and MCP
cleanup work. Outcomes are verified against the workspace, not inferred from
the agent's text response.

## Environment

- Checkout: `codex/mcp-subagent` at `a16ff10223` when probing began.
- MCP connector: `you.factory_session.list` responded successfully.
- Installed server: `C:\Users\andre\bin\you.exe` (built from source commit
  `d04ea6a352` at the latest probe). Existing MCP connector processes may
  retain an earlier binary until they restart.
- Provider/model for the first probes below: `opencode` /
  `opencode/nemotron-3.5-lightning-free`. Later rows name their
  other models.

## Observations

| Probe | Request | MCP outcome | Workspace verification | Follow-up |
| --- | --- | --- | --- | --- |
| LocalAI TTS evidence note | Create one markdown audit inside `workingRoot`; 180-second limit | `COMPLETED`, session `586670ba-b9e1-4987-96cd-9754a6f5585d` | File exists. First draft incorrectly applied generated-output WAV checks to per-request voice input and asserted unproven model requirements. | A correction call completed but left contradictory claims. Model completion is not document correctness; review before committing. |
| Parallel MCP code task | Port typed timeout result to current checkout; 180-second limit | `factory_session.subagent.timed_out`, session `5e0927e0-0afd-431a-91a3-dfa83253326b`, `retryable:false`, `partialEffectsPossible:true` | No MCP source file changed. `you.factory_session.list` returned no live sessions afterward, and no new OpenCode child process remained. | Retry as a smaller edit with another available model. Inspect state before retry because timeouts may leave edits. |
| Live session inspection | List active sessions, then get each returned ID | List showed two live subagent IDs; `get` returned `factory_session.session.not_found` for both. | `get` describes a durable read while list defaults to live scope. | Make the scope difference explicit or route get to live inspection; current discovery is confusing for an operator. |
| Nemotron concise rewrite | Replace the audit with a fact-only note; 120-second limit | `factory_session.subagent.timed_out`, session `39184710-bed0-4095-9d32-413f6591a70e` | The file had partial edits and still contained speculative statements. | Inspected before another attempt; no blind retry. |
| Ling free rewrite | Replace the same audit; 120-second limit | `factory_session.subagent.provider_throttled`, session `1dd53be9-7520-4f99-94b3-e4847b34786f`, `retryable:true` | No new edit from this attempt. | Try a different available model or wait for provider capacity. |
| Longcat free rewrite | Replace the audit with at most 350 words; 120-second limit | `COMPLETED`, session `81e7ea79-8167-4c88-a310-bcb652ceb410` | It produced a concise file at the requested path, but reported a different filename in its final text and mislabeled `TEXT`/`AUDIO`/`JSON` modalities as MIME types. The file was corrected and focused LocalAI tests passed. | Keep the response/file verification step even after successful MCP completion. |
| Mimo default-selection audit | Inspect and fix prompt-only provider/model selection; 180-second limit | `factory_session.subagent.timed_out`, session `b560f791-97dc-49dc-911c-f03b4d28d116` | No source edit or primary result. | Probe a prompt-only invocation directly; keep future audits smaller. |
| Nemotron Ultra LocalAI test task | Add focused reference-audio test; 240-second limit | `factory_session.subagent.timed_out`, session `dd5a0168-893b-4a98-9194-d5e9f737df74` | Four partial tests appeared in two files. All passed, but duplicated existing codec and protocol assertions and froze an unsupported reference-transcript behavior. The partial edits were removed after review. | Scope the next implementation task to one concrete missing behavior. |
| Prompt-only default selection | Read README with no provider or model; 90-second limit | `COMPLETED`, session `21b5bd11-bf33-41db-84d9-1f324512c1ed` | Returned the correct repository summary from the supplied workspace. A second call with no `workingRoot` also completed (`ecb3e0e7-7235-4068-83d3-cac74abfb560`). | The configured operator default works; a clean-install test is still needed to prove out-of-box selection. |
| Longcat empty-result classification | Make a completed invocation with no text an explicit MCP error; 180-second limit | `COMPLETED`, session `e076cfc3-e8a0-47e3-b32d-6ec341cabb5c` | It edited only the requested MCP source and test. Review confirmed `factory_session.subagent.empty_result` includes the session ID, and MCP plus CLI MCP package tests pass. | Rebuild and direct-probe the new binary before relying on the installed copy. |
| Longcat MCP guide correction | Clarify live versus durable session reads; 150-second limit | `factory_session.subagent.timed_out`, session `16eb2140-7735-404b-9968-230584b16518` | No edit. | The guide was corrected after source inspection; packaged docs smoke passed. |
| Longcat complexity fix | Reduce `Subagent` cyclomatic complexity; 150-second limit | `COMPLETED`, session `762a871f-f977-4e5a-9e83-ccae9c89d00f` | One source file changed; focused MCP tests and maintainability check passed. | Keep this smaller edit scope for future lint tasks. |
| Nemotron baseline cleanup | Remove stale ownership rows; 150-second limit | `factory_session.subagent.timed_out`, session `ee7949cb-49f5-4123-97d3-d57a30e4669a` | No edit. The prompt named a non-existent baseline path. | Retry with the root file path. |
| Longcat baseline retry | Remove stale ownership rows from the exact root file; 120-second limit | `factory_session.subagent.timed_out`, session `c4a793a3-a9c4-4db5-853b-f046d90ed992` | Partial edit removed five rows for three file paths; two rows still represented live imports. Restoring those two made `ownershipboundarycheck` pass. | Select rows by both file path and imported target, and review timed-out edits before accepting. |
| Longcat LocalAI `ref_text` | Add the confirmed reference-transcript protobuf mapping; 180-second limit | `COMPLETED`, session `e6417d5d-488b-4b1c-8339-1a137bad9d7d` | Four scoped files changed; codec and TTS protocol tests pass. The broader package exposed a separate Windows clock-resolution test flake. | Verify LocalAI synthesis separately; a mapping test is not GPU evidence. |
| Longcat LocalAI deadline test | Accept equal Health/LoadModel witness timestamps on Windows; 120-second limit | `COMPLETED`, session `13f6c78c-4861-4329-a9f0-eea4ee985c81` | One test assertion changed. Ten repeated runs and the full LocalAI package passed, including runs with equal witness timestamps. | Keep wall-clock ordering checks tolerant of equal adjacent observations. |
| Longcat TTS voice staging | Stage reference bytes as a temporary WAV path; 240-second limit | `factory_session.subagent.timed_out`, session `fa587a34-2a0e-432b-b21d-a56c81770f3a` | No edit appeared. The private path and cleanup were implemented after confirming the worktree was unchanged. | Split multi-package wiring tasks into smaller scopes for MCP agents. |
| Longcat TTS WAV validation | Reject mislabeled or malformed reference audio; 150-second limit | `factory_session.subagent.timed_out`, session `f238fb10-a973-4aef-bbb0-ef888a58d477` | No edit appeared. The codec validation was completed directly and focused tests passed. | Read-only progress without edits should be surfaced before a long timeout. |
| Mimo backend-registry audit | Read-only audit of the backend registry | `factory_session.subagent.provider_throttled`, session `e3813011-68e0-451c-9ddf-47b93d5c050a` | Read-only probe; no edit expected. | Retry when provider capacity allows. |
| Nemotron opener audit | Read-only audit of the opener; 150-second limit | `factory_session.subagent.timed_out`, session `21a72736-e64b-4ab8-9de5-695e8dccac6f` | Read-only probe; no edit expected. | Retry with a smaller read-only probe. |
| Nemotron one-line README probe | Return the repository name in one line; 90-second limit | `factory_session.subagent.timed_out`, session `b676e43f-606e-4135-add9-444c32cb22fe` | No result returned. | The snapshot log repeated `index.lock` failures before the stale lock was cleared; the retry showed no such lock failure, but that did not resolve the timeout. No definitive root cause is asserted. |
| Longcat one-line README probe | Return the repository name in one line; 90-second limit | `COMPLETED`, session `4543371b-715d-4dc9-bebf-9d7cda4dbd1d` | Returned `# you-agent-factory`. | Same probe completed under Longcat after the Nemotron timeout. |
| Longcat read-only permission probe | Read `C:\Users\andre\.config\opencode\opencode.jsonc` outside `workingRoot` | `COMPLETED`, session `06643994-e54c-4d59-ba4a-e87e3880502e` | Returned `permissions[0].effect = allow`. | Confirms the installed permission path grants read access to that external config file on this host. |
| Longcat external-directory edit probe | Edit an agent-owned temp file outside `workingRoot` from `before` to `after` | `COMPLETED`, session `a9d48ccb-eccf-4a5a-863d-fad2286ffe42` | Read-back verified the change; temp file was then removed. | Confirms the installed permission path allows external-directory edits without a special edit flag on this host. |
| Longcat Models architecture audit | Read-only multi-file audit of Models (backend registry, model definitions, artifact resolution); 150-second limit | `factory_session.subagent.timed_out`, session `1ce6cf08-f39a-464e-90fd-bae663bb2f1d` | No primary result; no source edits appeared. | Split audits into one or two file scopes and inspect provider progress before retry; no proven root cause. |
| Longcat artifact-selection audit | Read only `pkg/services/models/internal/artifacts/selection.go`; 90-second limit | `COMPLETED`, session `223c947e-4d3d-446a-9dfb-d622ece87c80` | Correctly identified `FailureUnknownBackend` for unregistered Qwen3 and audio.cpp IDs. | One-file audit completed where the broader audit timed out. |
| Longcat protobuf subset | Add PredictOptions `Tokens=4`, `UseTokenizerTemplate=43`, `Messages=44` and regenerate Go; 180-second limit | `factory_session.subagent.timed_out`, session `910e4885-ad97-4bd0-9ea0-a66f147d1588` | Partial edits to `.proto` and generated `pb.go`; no primary MCP result. Root inspected them and focused LocalAI tests compiled. | Neither timeout produced a primary MCP result despite useful edits; edits require post-timeout inspection. |
| Longcat one-file predictOptions mapping | Apply the requested one-file predictOptions mapping; 150-second limit | `factory_session.subagent.timed_out`, session `73829142-5623-4f41-8dff-9cb54353dfb7` | The requested one-file edit was left in place; no primary MCP result. Root reviewed it; focused tests and a live Gemma GPU gRPC invocation then returned `READY`. | Neither timeout produced a primary MCP result despite useful edits; edits require post-timeout inspection. |
| Backend registry and artifact selection audit | Read-only audit of the backend registry and artifact selection | `COMPLETED`, session `fcd35b90-50f7-4d0b-a4af-c2be633b4ad6` | No edit requested; none expected. Recognition alone lacks published Qwen/Index artifacts. | Recognized backends without published artifacts are not dispatch-ready; artifact publication remains a separate step. |
| Longcat reliability-log edit | Add the preceding audit and lock observations to this file; 90-second limit | `factory_session.subagent.timed_out`, session `1c160486-cace-413f-a135-7bc2c60a649d` | The audit row was added, but the lock note was missing and no primary result returned. | Inspect partial edits after timeouts. |
| Longcat ACP daemon lifecycle audit | Read only the ACP provider service implementation; 90-second limit | `COMPLETED`, session `e2874b9a-715e-4683-b496-f8643ecf3891` | Correctly identified retained daemon reuse and `Service.Close` / `daemon.stopLocked` termination. | A live `opencode acp` child under a live MCP server is expected; its presence alone is not a process leak. |

The two permission probes above verify the installed permission path on this
host for the specific paths exercised; they do not establish that every
external path is writable or readable.

The OpenCode log again showed snapshot `index.lock` warnings through 07:38 UTC.
The zero-byte lock predated the current ACP process, but a subsequent removal
attempt was blocked by local tool policy. The lock's role in model timeouts
remains unproven.

Two `you.subagent` calls ran concurrently and appeared as two live Factory
Sessions. This demonstrates concurrent dispatch, while the code task's timeout
shows that successful dispatch alone is not enough evidence of completion.

The current checkout's MCP code lacked the typed timeout response present in
the installed binary. A focused source change and test now preserve the
session ID, timeout duration, and partial-effect warning; the package test
passes. This source fix still needs a fresh-build MCP probe before it can be
treated as installed behavior.

## Integration status

The isolated `codex/mcp-goal-integration` worktree starts from the opener
cleanup, which already has the typed timeout and provider-throttle outcomes.
It now also validates `you.subagent` JSON arguments at the public MCP boundary
(ported from `ce8fd74079`). The focused MCP package test passes. A full merge
of `codex/mcp-invocation-hardening` would restore deleted runtime-opening
packages, so its customer-facing features are being ported selectively.
A fresh-built integration MCP server rejected an `unknownOption` argument with
`BAD_REQUEST` before dispatch. Its direct stdio `you.subagent` probe also
completed a workspace documentation edit using OpenCode Longcat. An app MCP
invocation using OpenCode Nemotron corrected the authored OpenCode provider
executable from `npx` to `opencode`; the generated catalog was updated and its
check and focused package tests passed. Neither probe required a special edit
flag. A broad `make verify-fast` attempt stopped at dashboard typechecking
because this isolated worktree has no Bun type dependencies installed.
The integrated binary was rebuilt, installed as `C:\Users\andre\bin\you.exe`,
and independently launched over stdio. It exposed all 11 MCP tools and
rejected an unknown argument before dispatch. Two pre-existing MCP server
processes still hold the previous executable image until they restart. A
second direct stdio launch of the installed binary completed a read-only
OpenCode Longcat invocation (`f15a7a77-d43b-49a3-b212-721b6a2aa572`) and
returned the expected primary result, `you-agent-factory`.
After the empty-result change, the binary was rebuilt and installed again.
Direct stdio probes of that installed build passed argument validation and
an OpenCode Longcat read (`945a1003-5170-450e-91d2-bf36720c8ffa`).
`make verify-fast` then passed with locked UI dependencies installed. The
subsequent lint gate exposed one new `Subagent` complexity violation, a stale
model-provider package hash, three stale ownership rows from opener cleanup,
and existing Go formatting drift. Those were corrected; `make lint` passed
with Git's POSIX shell and tools on `PATH` for the format target.
The final committed build was reinstalled and direct stdio probes passed:
unknown input returned `BAD_REQUEST`, and OpenCode Longcat returned the
repository name with a primary result (`289c43e7-d54e-45ce-b05e-94e0a627c79e`).
Concurrent server startup emitted packaged-installation `active-contention`
warnings for existing owner processes but did not prevent completion.
The LocalAI `ref_text` and Windows deadline edits were completed through MCP;
the larger voice-staging and WAV-validation tasks timed out with no edits and
were finished after inspection. The resulting LocalAI source passed focused
tests, `make verify-fast`, and all 24 `make lint` targets. These are protocol
and harness checks, not proof of a running GPU-backed LocalAI model.
A fresh direct stdio probe of the installed binary completed a read-only
OpenCode invocation with session 10d2a44e-00bd-4bdb-a049-1fa15e2572b2 and
returned you-agent-factory. Source commit `36b8d92585` was built and installed
as `C:\Users\andre\bin\you.exe`; a fresh direct stdio MCP launch listed 11
tools and completed a read-only OpenCode subagent with session
`9844094a-1e11-4a17-b391-e0017f30a233`, returning `you-agent-factory`, with
empty stderr. This does not resolve the earlier model-specific timeout
behavior.

A read-only timeout-diagnostic audit completed as session
`208d7d18-0877-4d2f-bde6-b2c54232f656`; its suggestion to expose
`result.Message` was rejected because that field can contain provider-private
output.

A bounded edit of `tool.go` and `subagent_test.go` completed as session
`3cd17741-6f0b-4eff-80d7-b5db3ca0cdf9`; focused MCP Go package tests passed
and timeout errors now include a nonempty `requestId` and `traceId`. This is a
diagnostic-improvement observation only and does not fix the underlying
OpenCode timeout.

A fresh stdio launch of the rebuilt candidate `you-mcp-timeout-ids.exe` listed
11 MCP tools. A one-second bounded OpenCode call timed out as expected in
session `2f9e2fe3-01bb-46e3-94b9-8b5dbfa82593` and returned both
`requestId=3862a36f-529b-452d-807a-f4202f6523cc` and
`traceId=trace-3862a36f-529b-452d-807a-f4202f6523cc`. The same identifiers
appeared in the server's structured timeout log, so they provide a usable
correlation path. This forced timeout does not diagnose sporadic longer calls.
Commit `75ebaca91f` was installed as `C:\Users\andre\bin\you.exe`; a fresh
stdio launch of that installed binary again listed 11 tools and returned the
same correlation fields on a one-second bounded timeout (session
`424cb3b9-6421-4590-805c-e44c2d5f2d2c`). Its request and trace IDs matched
the structured server log.
A separate fresh stdio launch of the installed binary completed a normal
Longcat read-only request in session `55716500-779e-49be-bb36-607a8e8e048c`,
returning `you-agent-factory` with empty stderr.

A provider-only `you.subagent` call (provider `opencode`, model omitted)
completed a read-only README probe in session
`ceaac4f3-f52f-4e8c-ac5f-92ddb76d203e` and returned `you-agent-factory`.
This validates the provider-only customer path on this host.

A provider-only `you.subagent` call (provider `pi`, model omitted) reached the
installed `pi-acp` child but timed out after 90 seconds in session
`824dea61-0ce4-4e45-8257-f41cb073e2a6`, with no workspace edits. Host Pi
settings select local llama-cpp model `qwen-3.8-uncensored` at
`http://localhost:8080/v1`; there was no listener on port 8080 and a direct
endpoint request timed out. The endpoint is a plausible cause, not proven from
the MCP result.
An independent `pi --no-session --no-tools -p 'Reply READY'` run with the same
Pi settings exited after about 18 seconds with `Connection error.` This
supports an unreachable configured model endpoint as the cause on this host;
the MCP timeout still lacks that actionable classification.

A concurrent provider-only `codex` read-only README probe completed in
session `10152e10-a987-4d66-90e3-4b2b82703bbb`, returning
`you-agent-factory`. This confirms the plain `codex` provider path on this host
while the Pi probe was running.

A provider-only `you.subagent` call (provider `opencode`, model omitted)
completed a bounded MCP code edit in session
`b85328b4-c4dc-4eea-8606-d7bd9c8c97e6`, adding `CancelOnTimeout` and private
post-start invoke/cleanup errors; focused MCP tests passed.

A second bounded follow-up request to add request IDs timed out with no edits
in session `2c2147c3-9ef3-46c6-8bb6-a82b8c8c099f`. Root inspected the unchanged
diff, finished those narrow IDs and tests, and the focused package test passed.
This does not resolve the underlying model timeout.
A fresh stdio launch of the rebuilt candidate `you-mcp-safe-close.exe` listed
11 tools and returned the typed timeout with request and trace IDs after a
one-second bounded OpenCode call (session
`88f06ad0-cc45-473e-8fc8-3e99706b47aa`). Its server log had the same IDs;
the explicit cancel-on-timeout path did not turn this ordinary timeout into a
cleanup error. `make pkg-maint` and the focused MCP package test passed.

A read-only Models resolver audit completed in OpenCode session
`6448288b-1ffb-433e-aff5-b421664a1761`. The current host configuration
contains OS and architecture only, and the default resolver hardcodes CPU for
Linux and Windows amd64. The audit suggested adding an accelerator field, but
that alone would not wire host detection or publish CUDA artifacts; the
manifest also lacks a Linux CUDA target today.

Commit `4bd24f1142` was installed as `C:\Users\andre\bin\you.exe`. A fresh
stdio launch returned the typed one-second timeout with request and trace IDs
in session `8dc1465d-eed7-47ba-8665-8f13006428b2`; a separate fresh launch
completed a normal Longcat read-only request in session
`82e6e1ad-4065-4d9b-9dcf-3b1fabbc6faf`, returning `you-agent-factory` with
empty stderr. The new cleanup and invocation failure branches were covered by
focused fakes, not triggered by these live probes.

A broad read-only ACP service audit timed out after 120 seconds with no edit or
primary result (session `8141319d-6b5c-4176-bcf0-705eb8ac0f75`). A narrowed
lines-940-to-1055 audit completed (session
`4615eace-9b53-4c4c-9a56-d6b4754b3b03`): `SessionUpdate` observes text/progress
before `Prompt` returns, while `withPartial` only attaches progress after
`Execute` returns an error; the higher-level Factory Session wait timeout does
not itself use that provider diagnostic path. This is source-level evidence,
not proof of Pi wire behavior.

A read-only OpenCode audit of the durable sync start path completed in session
`5963fe52-2176-4305-9953-35d3ca6fcbcf`, but the two inspected files did not
establish that it supports the packaged subagent. A direct
`you.factory_session.start_sync` request for `@you/subagent` returned
`BAD_REQUEST`: `factory "@you/subagent" is not a JavaScript workflow factory`.
The current durable start path therefore cannot replace the live subagent
invocation without extending its accepted source types or changing the
packaged factory.

An OpenCode Longcat multi-file WorkID change timed out after 180 seconds with no
edits (session `18897392-e782-4687-b963-1c84cc197d05`). The same bounded change
through the plain `codex` provider completed (session
`bc28938a-6747-41de-a935-be2808b251df`). OpenCode Longcat then completed the
MCP `workId` mapping (session `6b3e9499-7d8f-4239-a5c0-76adf1bc8b0a`); focused
invocation and MCP tests plus `make pkg-maint` passed. A fresh candidate binary
one-second timeout returned a nonempty `workId`
`batch-7d4ba850-6230-4b63-8b1f-886e10e8c150-work-1` in session
`01ffe7bd-2606-441a-88c6-1a3b5db40408`, alongside request/trace IDs. This
improves correlation, not the underlying timeout reliability.

Commit `d04ea6a352` was installed as `C:\Users\andre\bin\you.exe`. A fresh
stdio launch returned `workId=batch-f491257c-ad19-447d-91fb-c542fa9be09f-work-1`
on a one-second timeout in session `8a82574e-c722-4c66-a613-8605f7cab85d`.
A separate fresh launch completed a normal Longcat read-only request in
session `3dfa17a8-6f17-4221-b6b1-11fd2d4c167d`, returning
`you-agent-factory` with empty stderr.

The same broad ACP service audit prompt completed through direct OpenCode CLI
in about 69 seconds but timed out through `you.subagent` after 120 seconds
(Factory Session `86f31d6c-b50f-4f02-9fb6-b757a0f866a8`, native OpenCode
session `ses_f18e04808ffeMyvoLMX4uTMW76`). Sanitized native session exports
show that both runs completed their first `read` tool call. The CLI run then
continued through two more assistant turns and finished; the ACP run produced
no second assistant turn. This narrows that timeout to continuation after a
completed tool result. The OpenCode snapshot `index.lock` warning appeared in
both runs, so it does not by itself explain the difference. There is no
evidence in this run of a pending permission request or blocked file read.

A second narrow one-file OpenCode MCP audit timed out after 90 seconds
(Factory Session `fe1993c2-d225-4ff5-bc5f-f02cd17febe8`, native OpenCode
session `ses_f18dbd40effestF2lgkRdfqqen`). Its sanitized export again has
only a user message and an assistant turn ending in a completed `read` tool
call, with no continuation. The equivalent one-file MCP audit through the
plain `codex` provider completed in Factory Session
`3b9316b3-a974-465b-a7ca-1f6d7964f254`. It confirmed that the ACP provider
captures progress in `client.SessionUpdate` and attaches it on a failed
`session/prompt` return through `withPartial`, while the Factory Session wait
can time out before that provider result is available. This provider comparison
supports an OpenCode ACP continuation problem on this host; it does not yet
prove whether the upstream model API, OpenCode's ACP bridge, or our response
handling stops the continuation.

A 120-second `you.subagent` ACP audit with provider `opencode` and model
`opencode/nemotron-3.5-lightning-free` completed in Factory Session
`1681aed7-fd10-458d-a5ab-616dfaad55c1`. Its sanitized native OpenCode session
`ses_f18d9378affeqffM8QDZNIGwZD` shows a completed read tool call followed
by an assistant `finish=stop`. This contrasts two `Longcat` MCP sessions that
stopped after a completed read, but the model comparison does not establish a
root cause. A subsequent `Nemotron` edit correction timed out after 90 seconds
in Factory Session `c1e71a4d-595b-44e0-891b-0a205d66439b` with no edit;
sanitized native session `ses_f18d67165ffeK0gt334J5UvsWK` shows a completed
read but no assistant continuation.

A Codex MCP subagent then implemented timeout diagnostics in the MCP tool:
explicitly selected provider and model plus a `suggestedAction` now accompany
the existing Factory Session, request, trace, and Work IDs. The focused MCP
package test passed. A rebuilt `you.exe` was installed at
`C:\Users\andre\bin\you.exe`; a fresh stdio MCP launch listed 11 tools and
returned all those fields on a forced one-second Longcat timeout (Factory
Session `0547e552-78ef-4a04-a039-bd516f82b646`). The response remains
non-retryable because an edit may have happened before a timeout. This makes
the failure actionable without claiming the intermittent continuation has
been fixed.

An OpenCode Nemotron one-file/multi-file read-only audit of the managed CUDA
resolver timed out after 120 seconds in Factory Session
`aa07fdec-50f8-4bd5-85c4-ed23b69f3950`, with no edit. Sanitized native
session `ses_f18cfd445ffeD9CP0LCImb83wm` had three assistant turns ending in
tool calls, several completed reads/grep/glob calls, and no final answer. The
equivalent Codex MCP read-only audit completed in Factory Session
`a97ed1d5-bcc8-4f98-87f4-dd57d6bfe8c5`, confirming that the private
resolver has no CUDA capability input and the manifest lacks Linux CUDA
artifacts. This comparison does not establish the timeout's root cause.

Native OpenCode exports exposed a packaged prompt defect: the ACP request
contained the submitted task twice, once inside a workstation prompt sent as
system instructions and once in the user turn. It also contained a literal
`{{ (index .Inputs 0).WorkID }}` placeholder. A Codex MCP edit to split the
prompt timed out after 180 seconds (Factory Session
`c2642541-86a2-4a57-8e0d-e0d4a2dbe5e3`) but left partial source and test
edits. The first candidate omitted the task entirely; the new functional
assertion caught that failure. A second Codex MCP task completed (Factory
Session `698936af-ad96-4929-997d-c76826db9844`), placing one-pass
instructions in the worker body and `${input}` in the workstation body. The
packaged subagent functional suite, catalog check, and source check passed.
After rebuilding `you.exe` and replacing the unchanged installed packaged
Factory, a fresh stdio MCP OpenCode Longcat call completed in Factory Session
`898e5cb0-8e97-42be-a634-a932ec501a11`, returning `you-agent-factory` with
empty stderr. Its native session `ses_f18c0118bffeBp4D5E0ZLD5nTE` contains
the task once and no raw WorkID template. This removes prompt duplication;
intermittent model continuation still needs further stress testing.

A subsequent Codex MCP editing task for CUDA resolver plumbing hit the outer
`factory_session.request.timed_out` error after 180 seconds. The response had
no Factory Session or Work ID and labeled the error retryable, even though the
agent had already modified seven source and test files. The partial diff
passed focused Go tests, `make pkg-maint`, and `git diff --check`. This is a
separate terminal-classification defect: callers cannot safely retry a timed
out edit with possible partial effects. The cause of the agent timeout remains
unproven.

An OpenCode Nemotron MCP edit task to detect Linux CUDA capability timed out
after 120 seconds in Factory Session `e77aaa95-7b42-4fd9-8772-5481f2e44237`.
The tool returned `factory_session.subagent.timed_out` with
`partialEffectsPossible: true` and `retryable: false`; no requested source or
test edit was present in the workspace afterward. This adds another timeout
observation without establishing whether model, ACP, or harness continuation
caused it.

A narrower OpenCode Longcat task to update one CUDA target diagnostic and add
one manifest test ended as `factory_session.subagent.execution_failed` in
Factory Session `f70c037e-530d-4e47-b706-b0cbb38d8dac`. The MCP result
contained no primary agent output or root-cause detail, and the worktree had
no edits. This failure differs from the Nemotron timeout but is likewise not
enough to identify the failing layer.

A Codex MCP bridge edit later timed out after 180 seconds in Factory Session
`d2a6a138-8daf-4a9d-9d3c-690528811495`. It left partial edits to the
Models selection contract, service runtime, and wire mapping, but returned no
primary result or focused test outcome. The agent could not safely be retried
blindly. Manual review completed the bridge and its focused tests; the root
cause of this timeout remains unproven.

An OpenCode MCP task to classify `you.subagent` invocation-context deadlines
timed out after 120 seconds in Factory Session
`0041860e-7ca7-43ff-91db-d35ee305bea0`. It returned
`factory_session.subagent.timed_out`, `retryable: false`, and
`partialEffectsPossible: true`. A worktree diff immediately afterward showed
no source or test edits. This is another bounded edit timeout; it does not
identify whether OpenCode, its model, ACP, or the Factory harness stopped
progress.

The OpenCode log for that attempt showed repeated snapshot failures on a
zero-byte `index.lock` created several hours earlier. A second zero-byte lock
from the previous day was also present. With no `git.exe` process running,
both exact stale lock files were removed from the OpenCode snapshot cache.
A subsequent 60-second OpenCode MCP read probe completed in Factory Session
`fa24d9b7-08bb-478c-ac87-0e01c6b752f7`, returning the exact README heading
`# you-agent-factory`. The before/after observation makes the stale locks a
plausible contributor, but does not prove they caused the earlier timeout.

The same deadline-classification edit completed through Codex MCP in Factory
Session `9d19a205-dbf3-46a2-9501-6e0d646c79e3`. It changed only the
requested MCP source and test files; the focused package tests passed on
independent rerun.

After lock cleanup, an OpenCode MCP task to add provider, model, and timeout
facts to the new deadline error timed out after 90 seconds in Factory Session
`8439e615-6c6b-464f-a4cc-4fdd5735379c`. It left the requested source and
test edits. The focused test passed on independent rerun, but `make pkg-maint`
reported `Subagent` cyclomatic complexity 16 against the limit of 15. No
snapshot lock was present after this attempt, and the native log showed
snapshot commands running. Thus stale locks alone do not explain all MCP
OpenCode timeouts; a completed edit without a primary MCP result remains an
observed failure mode.

The sanitized native OpenCode export for that edit (`ses_f18896c52ffeulANUBYsktLseS`)
ended with repeated assistant `tool-calls` turns and no terminal `stop` turn.
The adjacent one-line read probe (`ses_f1889eff6ffebje2VizfN7mGIE`) had a
terminal `stop` turn. This narrows the symptom to continuation after tool
activity for this attempt; it still does not identify the failing component.
A Codex MCP task then extracted the invocation error branch to a helper in
Factory Session `719e7a2d-8720-48f2-8e60-f3b758fb7929`. Full MCP package
tests and `make pkg-maint` passed on independent rerun.

A direct stdio MCP call through a rebuilt `you-next.exe` completed an OpenCode
read-only probe in Factory Session
`99d94e64-cb41-428c-a45a-dabf90caad20`, returning the README heading; its
stderr peer connection closed line appeared after the client closed stdin.
This does not fix the OpenCode edit timeouts observed earlier.

The same rebuilt binary then completed a one-file OpenCode documentation edit
through direct stdio MCP in Factory Session
`33bb2559-3107-40c9-a846-3fb532c79bce`. The requested paragraph appeared
in the worktree. The `peer connection closed` stderr line again arrived only
after the test client closed stdin following the result.

An app-connector OpenCode task to update the LocalAI GPU audit timed out after
120 seconds in Factory Session `c74736c5-7138-499e-ac8e-0244680e289a`.
It added the requested ASR bullet but left the limitation paragraph unchanged;
no snapshot lock was present. The rebuilt binary then completed the remaining
one-file paragraph edit through direct stdio in Factory Session
`8b114fcb-85d2-4e09-99a7-fb53c38a7cb2`. Its first draft contradicted the
verified managed ASR installation, so a second direct stdio OpenCode call
corrected that exact paragraph in Factory Session
`39d3ba02-7cdd-492e-a29e-08a96eef5ac3`. The final diff was reviewed.
These probes show that fresh direct stdio invocations can complete bounded
edits while the current long-lived app connector still times out on some edit
tasks; different task scope and wait length prevent a causal conclusion.

The Codex MCP server configuration was changed from the locked
`C:\Users\andre\bin\you.exe` to the rebuilt
`C:\Users\andre\.local\bin\you.exe` for future connector launches. Existing
`you.exe server mcp` processes remain on the old binary until they exit; they
were not terminated because their live Factory Sessions may belong to other
tasks. The rebuilt binary's direct stdio read and edit probes above verify the
new executable independently of that connector lifecycle.

Two further app-connector OpenCode tasks timed out after 240 seconds without
leaving edits: a CUDA artifact-resolver task in Factory Session
`ca5878cb-e8f4-4f34-985f-219b85bb173b` and a focused LocalAI protobuf
thread-field task in Factory Session `bb718e8d-cc4b-4201-9e0e-496d05939461`.
Both returned `partialEffectsPossible=true`, so the worktree was inspected
before the source changes were completed directly. The underlying OpenCode
timeout cause remains unproven; these outcomes do not establish a resolver
or filesystem defect. A preceding Codex MCP task in Factory Session
`398767f2-b26e-4f65-b62e-f6d9be1fe1cd` completed an opt-in live embedding
diagnostic test. Its follow-on production launcher task in Factory Session
`c098f4a1-7dba-413e-ab61-185dd2bf3bb6` timed out after leaving partial
edits, which were reviewed and completed before validation.

Read-only inspection of OpenCode's local `session_v2` and `session_message`
records for those two timed-out calls found completed read/glob/shell tools
throughout their 240-second windows. The resolver task's last completed tool
turn was at 10:32:52 UTC after starting at 10:29:06; the protobuf task's last
completed turn was at 10:33:58 after starting at 10:33:06. Both ended with an
unfinished assistant message and no final stop. This rules out an initial
workspace-access or permission prompt for these two calls. It does not prove
whether the long gaps between turns came from model latency, tool scheduling,
or another bridge delay.

A subsequent narrow gallery-resolver test edit through the app MCP connector
returned `provider_throttled` immediately with OpenCode `big-pickle` in Factory
Session `d134a6ae-e2a7-4282-945c-827cbfc44786`; the worktree was unchanged.
The same request using OpenCode `nemotron-3.5-lightning-free` completed in
Factory Session `a948dfcf-2f37-42e3-acb8-56e9a486743f`. Its three CUDA
backend test cases passed on independent rerun and `make pkg-maint` passed.
These outcomes show that the provider throttle is now classified distinctly
from a working bounded edit, while the longer OpenCode task deadlines still
need better progress diagnostics.

A concurrent `pi` read-only README probe timed out after 120 seconds in Factory
Session `0b821f09-fefa-4c31-92a8-7dfdff372f02`. The configured Pi default
model is `llama-cpp/qwen-3.8-uncensored` at `http://localhost:8080/v1`, and a
TCP probe found no listener on port 8080. Pi ACP created a session-map entry
but no session transcript file for this invocation. The missing local model
server is a concrete environment blocker; the MCP timeout still did not name
that cause. After the tool closed the live Factory Session, its returned
session ID could not be inspected with `you.factory_session.get` or
`you.factory_session.list_dispatches` (`session.not_found`). Do not interpret
this Pi probe as a working provider result.

An OpenCode `nemotron-3.5-lightning-free` task refined the packaged subagent
worker instructions in Factory Session `37bf7d9a-7265-42dd-ba0a-1b6100d965e5`.
Its initial generic instructions accidentally hard-coded the two files from
that task; this was corrected on review before regenerating the packaged
Factory catalog. The prompt now directs exact-file tasks to read the named
files and direct tests first, then reserve time for edits, verification, and a
single final result. A rebuilt-binary probe is needed to measure whether this
reduces the earlier timeout pattern.

A fresh stdio MCP connection to the rebuilt `you-next.exe` completed an
OpenCode `nemotron-3.5-lightning-free` README-heading probe in 13.95 seconds,
Factory Session `093eca12-5c3a-43c2-9471-b4b393ee453a`. It returned the
single primary text `# you-agent-factory`. The stderr `peer connection closed`
line appeared only after the probe client closed stdin following the result.
This verifies the rebuilt prompt and direct stdio path for a small task; it
does not establish that a broad edit will complete under every model/deadline.

An OpenCode `nemotron-3.5-lightning-free` MCP edit to classify misconfigured
providers and missing executables timed out after 180 seconds in Factory
Session `974520a5-7888-4674-b49c-1048e98e294a`. It left the requested
source and test edits but no primary result; the partial test did not compile
because the `workers` import was missing. OpenCode session
`ses_f183f5011ffeA1tPg26tz0OOAw` recorded completed tools through the
deadline, including unsuccessful attempts to add that import. A 90-second
exact-file repair completed in Factory Session
`0e014f3a-17e3-46b2-a872-8e13ceb55174`, and an independent MCP package
test passed after review. This reinforces that timed-out edits require
worktree inspection; it does not establish the first timeout's cause.

A subsequent 90-second OpenCode log-only edit timed out without changing the
worktree in Factory Session `2e411ab3-4a72-4dec-953a-2d7ae36b4a19`.
OpenCode session `ses_f183ab131ffezgWptqCRANAPAS` recorded one completed
read at the start, then resumed planning about 77 seconds later and reached
the deadline without an edit. The gap's cause remains unproven.

A concurrent pair of read-only README probes tested two other free OpenCode
models. `muse-spark-1.3-contributor-free` returned a classified
`provider_throttled` error in Factory Session
`e750b319-f8cd-4e9f-9e4a-c11865903a7a`. `space-bunny-free` completed in
Factory Session `3d1ed64c-e4e6-4ca2-999a-97003d87af88`, but prefixed the
requested single heading with an extra progress sentence. Transport completion
therefore did not imply exact instruction fidelity for that model.

After rebuilding and copying the binary to the configured MCP path, a fresh
stdio MCP connection negotiated protocol `2024-11-05` and completed a
`nemotron-3.5-lightning-free` README-heading probe in Factory Session
`be3e4371-031e-4218-a4f8-bcf469f8e6e3`. Its primary result was exactly
`# you-agent-factory`. This verifies the installed binary's simple invoke
path; the new misconfiguration mappings are covered by package tests rather
than a live misconfigured provider.

An OpenCode `nemotron-3.5-lightning-free` task to trace the Pi provider's
unavailable-endpoint timeout through the Factory Session wait path timed out
after 180 seconds in Factory Session
`467f5b59-733d-49f1-82a9-cd35fec829f8`. The worktree had no edits afterward.
Its OpenCode session `ses_f1851426bffe7Twaq2JfZhOROj` recorded completed
read/search tools and assistant turns until the deadline, still exploring
provider execution. This was an overbroad audit for that deadline; it does not
show that the endpoint error was available to MCP, nor establish why the Pi
probe itself timed out. A subsequent task should name the exact execution
boundary and test only that error propagation.

A later live `pi` MCP probe timed out after 45 seconds in Factory Session
`93d39508-aaad-461d-8397-d618e97d637c`. The configured local model
endpoint on port 8080 had no listener, and the spawned Pi RPC child remained
alive after the timeout. Native Pi RPC probes with the installed Pi 0.74.2
emitted `message_end` with `Connection error.`, `turn_end`, and `agent_end`,
but never `agent_settled`, even when auto-retry was disabled in an isolated
temporary agent configuration. Installed `pi-acp` 0.0.34 waits for
`agent_settled` to finish `session/prompt`, and its README requires Pi
0.81.0 or newer. Pi 0.74.2 contains no `agent_settled` event implementation.
An isolated Pi 0.87.1 native RPC probe emitted `agent_settled` for the same
connection error. This establishes the version mismatch as the cause of the
observed Pi ACP hang; the missing model endpoint is the underlying provider
failure.

A fresh stdio MCP probe using temporary Pi 0.87.1 and its unreachable model
endpoint exposed a second defect: with Pi startup output enabled, the tool
reported `COMPLETED` and returned only Pi's `## Context` startup text as the
primary result, despite no model answer. With `quietStartup` enabled in the
temporary configuration, the same call returned
`INVOCATION_PRIMARY_RESULT_UNRESOLVED` instead. Startup notifications must
not count as a completed subagent answer.

Further bounded OpenCode probes exercised editing and read-only paths. A Pi
startup-filter edit (`3056ca0c-b01f-42e3-8287-ba4a5e7a817a`) timed out
after writing a new test file and modifying the ACP client. A managed LocalAI
resolver edit (`8852de90-2927-40b2-b68e-62edca5eff6d`) timed out without
writing a file. A read-only CUDA-path audit
(`02ae2913-1b7c-416a-9d61-0d2b01625fda`) completed with a primary result.
A bounded Windows CUDA-detection edit
(`00af9473-03f1-4750-82f4-382f34a5edde`) timed out after changing the
runtime platform selector but before writing its requested test. These results
show that a timed-out editing dispatch can leave a partial change, while a
small read-only task can complete normally; they do not establish the timeout
root cause.

The first Pi version-preflight build incorrectly required an injected
executable locator. Normal composition leaves that optional edge nil, so
every default Pi call failed as misconfigured before launching Pi. A direct
stdio MCP probe exposed the error; the preflight now uses the command factory
when no locator is supplied, and a focused test covers that construction
shape. After rebuilding the installed binary, a global Pi 0.74.2 call failed
in about one second with `provider_misconfigured` and a version-check action.
With temporary Pi 0.87.1, startup output enabled, and an unreachable model
endpoint, the same read request failed with
`INVOCATION_PRIMARY_RESULT_UNRESOLVED` after about eight seconds rather than
returning startup context as a successful primary answer.

After the installed-binary rebuild, an OpenCode read-only README-heading
probe (`46fa3d87-7bff-4a37-b778-a90c2dd951c0`) completed with the requested
primary result `# you-agent-factory`. This verifies that the ACP/Pi repair did
not break the basic OpenCode MCP path; OpenCode editing timeouts remain
unexplained.

A later read-only review of the new LocalAI publication resolver
(`ad57a8e1-e032-49dd-800f-dc79b70ddfac`) timed out after 90 seconds
without a primary result or file edit. This is another bounded read-only
timeout, so the successful README-heading probe does not establish that all
small OpenCode audits complete reliably.
The timed-out review also left an empty file literally named `$paths_file` at
the workspace root despite its read-only prompt. The file was inspected and
removed. This demonstrates that a read-only instruction to an editing-capable
agent is not an enforced read-only boundary.
The file reappeared once after the MCP timeout and was removed again after
the late write; the process listing then showed no matching OpenCode child.

A controlled one-line OpenCode edit using `nemotron-3.5-lightning-free`
completed in Factory Session `f664d29b-5e2d-4b5a-bc99-cb6afe8a132b` after
about 50 seconds. It appended this paragraph's predecessor exactly and
returned the requested path as its primary result. This confirms that the
installed MCP path can complete a real workspace edit, while showing that a
90-second deadline gives little margin for a larger audit with this model.
The OpenCode process log for this probe spans about 50 seconds and records
dozens of Git snapshot subprocesses; it does not expose enough model timing
detail to assign the latency to the model or the snapshot work.

A constrained `space-bunny-free` audit of the Windows LocalAI artifact config
and build script completed in Factory Session
`c2ea0811-3cd4-4d1b-bc98-74db8a678391` in about 10 seconds. It identified
the closed CPU/Metal publication matrix, the Windows llama build's forced
`BUILD_TYPE=cpu`, and the lack of a CUDA build/toolchain path. Those findings
were verified against the source. Resolver support alone therefore cannot
make native Windows CUDA first use work until a CUDA archive is built and
published.

An OpenCode `space-bunny-free` edit to bound the Factory Session
cancel-on-timeout callback timed out at 120 seconds in Factory Session
`d9550533-5f63-478d-aab4-2a0a67f233e9`. It left no workspace diff.
The provider log shows repeated repository searches, including checks for
unrelated lint style and timeout declarations, through the deadline. The
requested two-file change had not begun. This is evidence of task execution
sprawl in that model, not evidence of a transport failure. A smaller edit
prompt or direct implementation is needed for this specific change.

Splitting that edit into two narrow OpenCode `space-bunny-free` requests
worked. The production change completed in Factory Session
`d2af325a-5ee9-45e8-a189-077978c77619` in about 22 seconds, and the
deadline assertion completed in `6eb9383e-0f4d-4004-affe-f7ea9a12782f`
in about 37 seconds. The new 15-second detached context bounds the cancel
control when its downstream operations honor context cancellation. It is not
a process-kill guarantee; a control implementation that ignores context can
still block. Focused package tests and `go vet` passed. This split shows that
the prior 120-second timeout was strongly affected by task breadth and the
model's search behavior, while the exact OpenCode timeout cause remains
unproven.

The committed build `97b4e5b8c4` was rebuilt and copied to the configured
`C:\Users\andre\.local\bin\you.exe`; candidate and installed SHA-256 both
equal `77A9956B3180801FB53767126247705321F4BD7BEDE22A8F094EF925ABC92CC6`.
A fresh OpenCode `space-bunny-free` README-heading MCP probe completed in
Factory Session `4aff8f97-c9a7-4b4c-99f7-ddaece4672e6` in about five
seconds and returned exactly `# you-agent-factory`. Later process inspection
showed that the in-app MCP tool still used an older running
`C:\Users\andre\bin\you.exe`, so this probe did not verify the rebuilt
`.local\bin` executable or exercise the new cancel deadline.

A fresh direct stdio MCP connection to the rebuilt `.local\bin\you.exe`
negotiated protocol `2024-11-05` and invoked OpenCode from a disposable
directory outside the repository. Factory Session
`6059e6da-0263-4c69-bb47-bcf96afb57d2` returned the unique file content
`ROOT_PROBE_7E9C62A4`. A second direct stdio call with a 2-second deadline
returned `factory_session.subagent.timed_out` after about 2.06 seconds in
Factory Session `6beb4969-5938-41c4-8b7a-e77ffa31e934`, with
`retryable: false`, `partialEffectsPossible: true`, and the request, trace,
and Work IDs. No new OpenCode process or workspace edit remained after the
direct probes. This checks the installed binary's success and timeout
envelopes, but does not prove the new 15-second cancel deadline was reached.

The older `C:\Users\andre\bin\you.exe` is still held open by existing MCP
server processes and could not be overwritten on Windows. The current
`config.toml` points to `.local\bin\you.exe`; a fresh process uses the
rebuilt file, while this task's in-app MCP connection may remain on the old
executable until its server restarts.

One direct probe used a nested disposable directory under the repository
with its own `README.md` containing `# timeout probe`. OpenCode was launched
with that directory as its location, according to its process log, but
returned the repository root heading `# you-agent-factory` instead in
Factory Session `3becb2cb-de82-4229-b095-e6897c3d067a`. The same rebuilt
binary returned the exact unique content from an independent temporary
directory in the next probe. The nested result is an answer-fidelity failure;
the available log does not prove whether OpenCode read the parent file or
answered from surrounding context.

An isolated delayed-write stress probe on the rebuilt binary timed out
after about 2.05 seconds in Factory Session
`30c937fe-c37a-43b1-9cf3-c84113972319`. The requested file did not exist
at response time or ten seconds later while the MCP server stayed alive.
This confirms no late write in that run, but a two-second deadline may have
expired before the model attempted any write, so it does not resolve the
earlier observed late-write failure.

Two narrow OpenCode `space-bunny-free` edits then bounded the MCP tool's
detached Factory Session close context to 15 seconds and asserted the
deadline in its test fake. The code edit completed in Factory Session
`51f87e8a-54d7-4ed3-bd85-4ad32a786527` in about 32 seconds; the test edit
completed in `0e3d0d2d-134b-430d-91c0-bad8bdd9572b` in about 38 seconds.
Focused MCP and invocation package tests and `go vet` passed. As with the
cancel-on-timeout deadline, this only bounds downstream close operations
that honor context cancellation.
