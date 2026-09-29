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

Build `6d0c7e60a4` was rebuilt and copied to the configured
`.local\bin\you.exe`; the candidate and installed SHA-256 both equal
`84DC7F5DECDF97475E4C302681D2624D5989F07F2B5EF359B6AD9455837B38A1`.
A fresh direct stdio MCP connection negotiated protocol `2024-11-05` and
completed an OpenCode read of the unique temporary file in Factory Session
`c47446d0-4d1f-4da6-bdc8-2a46482b5c49`. Its result included the expected
`ROOT_PROBE_7E9C62A4` content. This verifies the rebuilt basic invoke path,
not the 15-second cleanup deadline under a stalled close.

Two narrow OpenCode `space-bunny-free` tasks made a cleanup deadline
actionable. Factory Session `dc12f955-8a73-4cc9-825a-805408f298a1`
changed the MCP response classification in about 29 seconds, and
`ce84bd01-7245-4d3d-9a53-93c540ae8fa0` added a focused test in about
35 seconds. A close error wrapping `context.DeadlineExceeded` now yields
`factory_session.subagent.cleanup_timed_out`, nonretryable with the session
ID, partial-effects warning, and an explicit inspection action. Other close
errors retain `cleanup_failed`. The MCP package test passed and confirmed
that sensitive provider and close-error text does not enter the envelope.

Build `93a99f120b` was rebuilt and copied to the configured
`.local\bin\you.exe`; candidate and installed SHA-256 both equal
`951B359BA69881429A7842359ED7BBE46A73B7A90870162CAD3E8B69B67AE5D5`.
A fresh direct stdio MCP connection negotiated protocol `2024-11-05` and
completed an OpenCode read from the independent temporary directory in
Factory Session `5650abde-d907-45ba-bc21-a02506075816`. Its result
contained the unique expected file content. The live probe covers the
rebuilt success path; the cleanup deadline classification is covered by the
focused test.

A stronger isolated cancel probe used the rebuilt `.local\bin\you.exe` and
asked OpenCode to run a PowerShell command that wrote `started.txt`, slept
20 seconds, then wrote `late.txt`. The first marker existed when the
12-second MCP deadline returned `factory_session.subagent.timed_out` in
Factory Session `2a4c2e69-0a57-4a67-970d-babc7ceee1bb` after about
12.05 seconds. The second marker was absent at response time and 22 seconds
later while the MCP server was still alive. Process inspection after server
shutdown showed no new OpenCode or matching PowerShell child; only the
older in-app OpenCode process tree remained. This proves cancellation
stopped that in-flight shell command before its delayed write, while the
earlier late-write observation remains a separate unresolved case.

Two read-only OpenCode audits of a managed Qwen3-TTS/IndexTTS path returned
primary results but incorrect implementation advice. A `space-bunny-free`
audit (`8505372a-3335-41e3-9b47-27ce56ae5814`) suggested using the
VibeVoice backend for both models, contrary to the live evidence for
`cuda12-qwen3-tts-cpp` and `cuda12-audio-cpp`. A corrected
`nemotron-3.5-lightning-free` audit
(`613ff044-dcd0-4dbe-97b6-fe59daa711c2`) fabricated source paths,
proposed two definitions with the same built-in `tts` name, and mapped
IndexTTS to `cuda12-audio-cpp-indextts`, which is not the installed gallery
backend name. Neither audit changed files. MCP completion and a plausible
primary result therefore cannot be treated as implementation correctness;
the managed model design requires source and live runtime verification before
editing.

A full `make lint` attempt through Windows `bash.exe` actually ran in WSL,
where Node was unavailable and the Windows worktree Git path was invalid.
The lane reported six failures; native Windows reruns confirmed
`pkg-boundary` passes and `pkg-file-count` had five real package count
findings. The WSL UI, model-provider-package, and formatting results are
environment failures, not verified code defects. `go vet`, maintainability,
deadcode, and most other backend lint targets passed in that lane.

Narrow OpenCode `space-bunny-free` mechanical edits consolidated related
files without changing their test bodies: permission tests into elicitation
tests (`5fb47e23-62aa-4f0a-bbfb-6a11e030355a`), CUDA selection tests
into gRPC projector tests (`dfbf4667-80d2-4baa-9660-0fb08272ae1c`),
VibeVoice layout tests into TTS protocol tests
(`388d61af-92ed-47de-aac2-2217348840db`), two small Models wire runtime
files into `invocation_runtime.go` (`f67447ce-6a5d-48ad-9e0e-39986094913c`,
`8f304d65-88de-4a96-a568-b1c13a28eaca`), and gallery/published resolver
tests into default resolver tests (`94c949ac-1a78-4c34-9e56-15d9f4fda971`,
`dc8d233b-4ead-4a20-bf61-e742ce84725f`). All seven calls returned
primary results. Focused Go tests and `make pkg-maint` passed. Native
`pkg-file-count` improved from five findings to two; the remaining packages
are `pkg/services/models/internal/service` and `pkg/wire`.

Two further narrow `space-bunny-free` edits consolidated Models service
tests: gallery backend tests into runtime configuration tests
(`4b705c37-b936-4ec0-a081-b69731336120`) and external-package host-lease
tests into constructor tests (`915c7b1f-3aa9-44ba-9a0d-2937764910c1`).
Both calls completed and the package tests passed. Native
`pkg-file-count` now reports only `pkg/wire` (57 files against its recorded
50-file baseline).

Seven `space-bunny-free` OpenCode edits then consolidated cohesive `pkg/wire`
files. Three disjoint pairs were dispatched concurrently: Chat Sessions
composition (`fbb788f5-00a6-456f-abe5-05ea2b50668f`) with packaged
Factory CLI composition (`4a433431-cc80-4905-b50f-79a0f8b518ad`), run
session selection (`a98029e8-7aee-495c-8528-6dc5ca6c4cf6`) with Factory
Definitions service composition (`103da496-5190-4dd1-a869-4525dc140213`),
and recordings test support (`58ecfffa-06f6-404d-b97d-2faa37263ae9`)
with llama launcher environment tests
(`4c3b198d-e5ad-4a62-b1be-b9fba0ed8f0b`). A final bounded call moved
CUDA platform tests (`82115e37-7796-4f4c-bf1f-4cfd5615414a`). All seven
returned primary results and the concurrent pairs made disjoint changes.
`go test ./pkg/wire`, native `pkg-file-count`, `pkg-maint`, UI lint/deadcode,
model-provider package check, and native formatting check passed. The full
Git Bash `make lint` lane had only `fmt-check` failing while deleted tracked
files were still uncommitted; the formatter attempted to stat their old
paths. The lane should be rerun after committing the deletions.

A read-only `space-bunny-free` OpenCode audit of the Linux CUDA capability
caller path completed in Factory Session
`01a1431c-9835-4a6e-a7cc-1e4a585f397e` with a primary result and no
source edits. Source review confirmed that `cmd/factory/main.go` passes the
environment edges through `root.BuildProcess`, while
`pkg/wire/models_runtime.go` derives CUDA availability from the Linux device
and `nvidia-smi` probe before passing the platform into Models. The gallery
resolver then selects the accelerator from that platform. The concurrent
change to `pkg/services/models/wire/default_backend_resolver.go` addresses
Linux CUDA selection; this audit found no additional missing caller input.
The untracked, zero-byte root file named `$paths_file` was present before and
after this call. Its creator is unconfirmed, so it is not attributed to this
session.

After the wire consolidations were committed, the full native Windows Git
Bash `make lint` lane passed all 24 targets. The zero-byte `$paths_file` was
removed after its size was checked; its creator remains unknown.

An OpenCode `space-bunny-free` request to fix published Windows CUDA release
selection and add a test timed out after 120 seconds with
`factory_session.subagent.timed_out` (Factory Session
`9612fb6f-641e-47f5-9d72-9d5a6aa3bf7d`). The tool marked it
nonretryable because partial workspace effects were possible. Immediate Git
inspection found a clean tree and no requested edit. OpenCode's log showed
repository searches and snapshot commands during the call; it did not prove
why no edit was produced before the deadline. Both exposed Factory Session
inspection methods returned `factory_session.session.not_found` for the
returned ID, so the timeout's session identifier was not independently
inspectable through this MCP server instance. The next probe should use a
single-file edit and compare its completion with this broader request.

A narrower `space-bunny-free` request to add only that regression test in
`default_backend_resolver_test.go` also timed out at 90 seconds (Factory
Session `234620f4-cc8e-4d44-be00-09fe0a83406c`). Git inspection showed
only this reliability-log edit; OpenCode made no test edit. The log shows it
searching the manifest decoder and fixture near the deadline. Session lookup
again returned `factory_session.session.not_found`. Two different task sizes
on the same model timed out without primary results, so the next comparison
should use a different available OpenCode model on a small edit.

Three alternative OpenCode models (`mimo-v2.6-flash-free`,
`ling-3.0-flash-fin-free`, and `big-pickle`) each failed in about 21 seconds
with the actionable `factory_session.subagent.provider_throttled` envelope,
`retryable: true`, and `failureReason: throttled` (sessions
`8b18a778-001b-4826-8fa7-e42c45b2c929`,
`d3016dd3-f170-46d7-94b5-1a04d09ae782`, and
`644d0c99-22ea-4610-97d3-80549fc6407a`). These were capacity failures,
not evidence that the requested edit or repository code failed. No requested
test edit appeared in the worktree.

The same `you.subagent` MCP operation with provider `codex` completed the
bounded regression-test edit in Factory Session
`f4a0e6db-ad43-40fb-aca6-1b67e3da67ed`. Independent execution of the
exact new test failed as expected: the resolver chose the newer CPU archive
instead of the older published CUDA archive. A second Codex subagent
(`d017aba2-7cac-48fc-8347-eb4136d4612d`) implemented per-backend search
through validated release manifests; the Models wire suite passed. Review
found that a later malformed release could discard earlier valid candidates,
so a third bounded Codex subagent
(`9ea9054a-a0ae-400a-9618-b290dd10c014`) retained usable candidates and
added a malformed-later-release regression case. The Models wire, artifacts,
and application wire Go suites passed after the change. This comparison
shows the MCP route can complete a real test-to-fix cycle with another
provider, while the two Space Bunny timeout causes remain unproven.

A fourth bounded Codex MCP subagent
(`45959ba4-49ee-4869-b772-fa8dab897bdb`) added a same-resolver test for
llama.cpp and Whisper CUDA archives from different validated releases.
Independent execution of the Models wire, artifacts, and application wire
Go suites passed. This covers release selection across two backend requests
without a new publication fetch for the second request.

The first full `make lint` pass exposed size and complexity violations in
the new tests and resolver, plus one test-only helper left in production
code. Codex MCP subagent `fd9d2df1-4888-4412-83d5-d9600882c2ec`
refactored those same files; focused Go suites passed independently, and
the full native Windows Git Bash `make lint` then passed all 24 targets.
The updated `you.exe` was rebuilt and installed at
`C:\Users\andre\.local\bin\you.exe` (SHA-256
`7698C21E39C80E7515D4DF26824655D017BC38B9AA1580D667E0BFA1572283D6`).
A fresh stdio MCP initialization and `tools/list` on that installed binary
returned 11 tools including `you.subagent`.

The failed timeout-session lookups were traced to `you.subagent` closing its
live Factory Session before returning. Codex MCP subagent
`ec5123a1-4b90-45c1-8ccc-7efab168eaa9` updated post-close failure
envelopes with `sessionClosed: true`, log-correlation guidance, and a
workspace/provider-log inspection action; cleanup failures do not claim a
closed session. Follow-up subagent `ccf05932-304e-44d0-9354-c4f276ad27a4`
documented the same lifecycle for successful results in tool discovery.
The first full package test found stale MCP inventory output, so subagents
`7800cbe0-954e-483d-93eb-65b4b2bdee69` and
`820a84f2-e1dc-4e49-b21e-4ace56c55ab5` regenerated the canonical tool
inventory and published API contract artifacts. The full Factory Sessions
MCP package test, contract check, and native Windows `make lint` (24 targets)
then passed. The underlying OpenCode timeout cause remains unproven.
The rebuilt installed binary (`C:\Users\andre\.local\bin\you.exe`, SHA-256
`7C17AD7B50196279214FAF553F3584740C090089B9B7FEFCB6B2417296387A24`)
was launched as a fresh stdio MCP server. A forced one-second Codex subagent
timeout returned `sessionClosed: true`, request/trace/work IDs, and the new
inspection guidance. In that same server process,
`you.factory_session.get` for the returned ID correctly returned
`factory_session.session.not_found`. The worktree remained clean after the
cancelled probe. This verifies the customer diagnostic and explains the
earlier failed lookups; it does not identify the OpenCode timeout cause.

A subsequent read-only Codex MCP audit of timeout progress sources timed out
at the caller's 120-second deadline (Factory Session
`19fe1190-ef92-45f9-b004-f1d8467315cb`) without a primary result.
Immediate Git inspection showed no workspace edits. This demonstrates that
the short bounded deadline can also expire on a Codex research task; it is
not evidence specific to OpenCode. The next probe should compare a realistic
longer deadline or inspect progress through a service read before assigning
the timeout to a provider or transport fault.
The same narrow question, capped at 300 seconds and limited to the MCP tool
and Factory Sessions service contract, completed via Codex MCP subagent
`1499003f-877a-446c-b41d-14bc3efb9848` in about 49 seconds. Source
review confirmed `Service.GetFactorySession` can read a live projection
before close, but its `Runtime.Progress` only offers Factory state/counts;
`Runtime.Lifecycle.UpdatedAt` is not a precise last-worker-activity time.
This narrows the next diagnostic design: a coarse progress snapshot is
available now, while identifying a stalled provider/tool step requires a
source-native Worker observation read or a new owner-level contract.

An OpenCode Space Bunny MCP subagent (`7deb1c76-977d-408f-9ca0-df44e8b73897`)
completed a bounded Models gallery edit with a 300-second deadline in about
195 seconds. It added Qwen3 TTS and audio-cpp gallery backend names and CUDA/CPU
selection tests, and returned a primary result. Independent Models wire tests,
package maintenance checks, and `git diff --check` passed. Earlier 90- and
120-second Space Bunny research tasks expired without edits. The longer
deadline allowed this edit to complete; it does not by itself prove why the
shorter calls stalled or timed out.

Another OpenCode Space Bunny MCP subagent
(`151253f0-63ca-4eba-b622-c82290dac7cf`) reached its 300-second timeout
while auditing and editing Windows published backend selection. It returned
no primary result and left no source changes in the worktree. The cause
remains unproven; a narrower resolver edit was delegated through the same MCP
infrastructure to Codex for comparison.

The narrower Codex MCP subagent (`6dd14a41-4166-422c-b3a0-ed2dc1de0f3a`)
completed the Windows published-resolver edit and returned a primary result.
It changed online selection to fetch validated publications for CPU and both
implicit and explicit CUDA requests, with offline cache reuse and a checked-in
fallback. Focused and race Models wire tests passed in the subagent; independent
Models wire, artifact, and application wire tests passed afterward. Review
identified a separate compatibility gate that still rejects explicit Windows
CUDA before the resolver; that follow-up is in progress.

Codex MCP subagent `7b057768-6dd4-4741-99c7-53a8ec6f5c21` completed that
compatibility gate follow-up. It permits a supported explicit Windows CUDA
request to reach publication selection while still checking backend, protocol,
platform, and accelerator. Independent Models wire, artifacts, and application
wire Go suites passed after both edits. The current public release index still
has no Windows CUDA archive, so this proves selection behavior with validated
fixtures rather than native Windows GPU first use.

The first full `make lint` failed only at `pkg-file-count`: two new wire test
files raised the Models wire package from its 15-file limit to 17. Codex MCP
subagent `bab1cc86-b5f0-49b4-b9c4-1131888061b0` consolidated the published
resolver with the default resolver and folded host compatibility tests into
the existing resolver test file. The package returned to 15 files. Independent
`pkg-file-count`, `backend-size`, focused Go tests, and `git diff --check`
passed; a full lint rerun follows.

The first full lint rerun after consolidation failed only because the deleted
resolver file was still in Git's index and `fmt-check` enumerated that path.
After staging the intended deletion and edits, full native Windows Git Bash
`make lint` passed all 24 targets.

The change was committed as `4222cb05ab`. A fresh Windows CLI build was
installed at `C:\Users\andre\.local\bin\you.exe` (SHA-256
`A8F05B57AEB957708BA4E6F94BDC030CA8D17433CAA49C2ED35BA6B6DF2E3069`).
A fresh stdio MCP initialization and `tools/list` returned 11 tools including
`you.subagent`.

An OpenCode Space Bunny MCP subagent
(`f6be8da4-9497-463d-ac29-fa91d7a6c74c`) was assigned a bounded timeout
progress-snapshot edit with a 300-second deadline. The OpenCode log showed
ongoing repository reads, and the worktree gained a partial `tool.go` edit
before the call returned `factory_session.subagent.timed_out` without a primary
result. The edit had no tests yet. This is a concrete partial-effect timeout:
the workspace must be inspected and the patch reviewed before any retry. The
provider-side reason for the long research and timeout remains unproven.

Codex MCP subagent `384e73da-e226-433a-bdab-e7fc3bfd9c28` completed the
partial patch. The timeout envelope now captures numeric live progress before
session close and omits arbitrary projection strings; tests cover both timeout
forms, snapshot failure, cleanup failure, and sensitive-text exclusion.
Independent MCP transport Go tests passed. `pkg-maint` then found two small
complexity violations in the new code/test, which are being refactored through
another MCP subagent.

Codex MCP subagent `ba2df20d-ca21-4045-9c25-25e02cce3272` refactored both
violations. Independent Factory Sessions MCP, MCP transport, and sessionservice
Go suites passed, and full native Windows Git Bash `make lint` passed all 24
targets. The timeout snapshot still needs validation against a fresh installed
stdio MCP binary; the in-process MCP connector can retain an older server
process after binary replacement.

The timeout-snapshot change was committed as `d90a2cdbc6`, then the Windows
binary was rebuilt and installed at `C:\Users\andre\.local\bin\you.exe`
(SHA-256 `9F57E38A7CD08E74B3636B5C1F3768AE515A72AA69375FB306C77AF44280AD68`).
A fresh stdio MCP process received a forced one-second Codex `you.subagent`
timeout. Its response contained `sessionClosed: true`, request/trace/work IDs,
and `progress: {available: true, inFlightDispatches: 1}`. The workspace remained
clean. The OpenCode log for the earlier five-minute partial-edit timeout shows
continued repository tool activity near the deadline; this argues against a
silent peer disconnect in that attempt, but does not establish why all
OpenCode tasks take as long as they do or explain other timeout cases.

An OpenCode MCP subagent (`27b27be4-d462-4e73-9240-16542951c011`) was
assigned a bounded Linux CUDA publication-resolver edit with a 300-second
deadline. It timed out without a primary result and left no resolver edits.
The timeout's underlying cause remains unproven. Codex MCP subagent
`e095dcf7-cf36-459e-bb6a-7d698420cb5b` completed the same narrow
resolver task with synthetic Linux CUDA publication tests. Codex MCP subagent
`0fec9c41-28b3-4c45-b11e-8c53e023bbbd` then wired Linux production
selection to prefer a published CUDA archive and retain the gallery CUDA
fallback. A live release-index check found no published Linux CUDA archive;
the new archive path is fixture-tested, while the gallery remains the
available GPU path.

A broader Codex MCP audit of the custom IndexTTS pull failure
(`c2beeeae-5f70-4d4b-b7bd-578417bc7294`) reached its 300-second timeout
without a primary result or source edit. It left an isolated probe directory
in the workspace. An independent WSL file trace showed the CLI trying to
stat the relative name `index-tts2.5` while never opening the configured
GGUF path. A narrower follow-up targets the scoped pull routing directly;
the broad audit's timeout cause remains unproven.

The narrower Codex MCP follow-up (`854559d8-baf4-4ba7-bc46-79719add6a3c`)
also timed out at 300 seconds, leaving a partial scoped-pull edit and test.
Independent testing found that the partial test failed: the canonical resolver
interpreted dotted operator name `index-tts2.5` as a local path before it
checked configured names. The implementation was completed locally by giving
configured names precedence, retaining the scoped-pull routing for operator
overlays, and correcting the test to assert private source handling. The
focused and broader Models Go tests passed, as did all 24 lint targets. A
second relative-path classification in Models Assets was then fixed with a
focused regression. A rebuilt WSL CLI installed `cuda12-audio-cpp` and pulled
the 7,885,093,568-byte IndexTTS GGUF to managed cache with
`INSTALLED_SUCCESSFULLY` and `READY`. The next offline reference-audio
invocation failed at a distinct boundary: `local model worker not found for
"index-tts2.5"`. Managed inference remains unverified.

Codex MCP subagent `c28264d0-7c82-4ac5-9760-95db71b3b972` completed a
narrow Runtime Host edit and test for the standalone operator worker lookup.
The next WSL offline IndexTTS invocation started the `cuda12-audio-cpp` gRPC
server, then failed at `INVOKE (INVOCATION_FAILED)` with no WAV output. This
is a separate inference-stage failure; the exact backend response is not yet
proven by the CLI error.

The WSL failure was reproduced with private runtime evidence. `PROTOCOL_LOAD`
completed, but the request spent CPU time with no corresponding GPU memory
rise and eventually failed at `INVOKE`. The installed LocalAI IndexTTS model
configuration declares `options: [backend:best]`; the managed `LoadModel`
request omitted it. After adding that audio.cpp option, the same offline
managed invocation completed in about 34 seconds and produced a non-silent
56,364-byte, 22,050 Hz WAV. Runtime evidence recorded `INVOKE` and terminal
`COMPLETED`, and GPU memory rose during the request. An intermediate test
raised the Factory Sessions default one-shot timeout from 10 seconds to five
minutes, but the failure persisted; that speculative change was reverted.

Three further OpenCode MCP outcomes were recorded today. Session
`bb4555ef-1839-4384-8b7b-d6e6c143c1c8` completed in about 35 seconds, editing
`callSubagentJSON` to reject `workingRoot:null` and adding a focused test;
package Go tests passed. Session `3712dcc1-e8d9-463d-8529-dbcee7730ac2`
completed a read-only bounded tail of
`C:\Users\andre\.local\share\opencode\log\opencode.log` outside `workingRoot`
with no permission denial, and found run `32c535d1` stopped gracefully with no
actual error. Session `dbc2852f-bd98-48ce-a5b6-70ed86aba68c` completed creation
and read-back of
`C:\Users\andre\AppData\Local\Temp\you-mcp-external-write-probe-20260928.txt`
outside `workingRoot` without prompt or denial. These successes validate this
permission path on the current host but do not prove the cause of the earlier
timeouts. Automatic approval review rejected the caller's cleanup of the
temporary probe file, which remains in place; no further attempt was made to
delete or edit that external file.

Subsequent OpenCode MCP subagent calls were recorded. Session
`50d8efa4-faca-4841-9dae-ff9eb5fe0f9d` completed in about 42 seconds,
changing `.gitignore` to ignore generated root cache/probe artifacts. Session
`942f690c-d217-4b92-b0fe-c4831132a666` completed a WSL readiness read in
about 49 seconds but incorrectly claimed the Qwen backend was absent and
supplied invalid `models invoke` flags; caller source and CLI checks found
the Linux gallery mapping and the correct `--input`/`--parameter` flags.
Session `d52e84d5-0b19-4ec3-9535-2385c848b43f` timed out at 120 seconds with
no WSL config edit because the log shows attempts to read `/root` config as
the default WSL user instead of `-u root`; the exact-root retry
`b0b34e3e-05de-4837-99fa-e3cfe52bc942` completed in about 67 seconds.
Session `95555b6f-ed25-4b01-8143-54053522c723` timed out at 300 seconds after
the real pull populated both GGUFs and the CUDA backend but before a primary
result; `54013b56-603b-45cf-af63-fa20125cf4f0` used an unsupported
`--offline` flag on `models pull` and received the exact CLI error; the
corrected call `92a7da69-1876-4102-bf55-6d8153687ba5` completed with
`ALREADY_READY`/`READY` in about 22 seconds. The timeout cause for that
300-second call is not fully proven. Session
`024fcf86-ba3d-4005-876c-eeb6f0a62af8` completed Qwen synthesis but made an
unsupported CPU-only inference from post-exit `nvidia-smi` output and left
two untracked helper scripts despite a no-code-edits instruction; the
repeated invocation `04630134-939a-4f53-bc7d-a7beb6a8f8aa` completed in
about 45 seconds while the caller sampled GPU, proving memory rose from
1518 to 6458 MiB and utilization reached 97%. Automatic approval review
rejected cleanup of the helper scripts and no deletion was retried.

After commit `1714a1ca56`, the Windows `you.exe` was rebuilt and copied to
`C:\Users\andre\.local\bin\you.exe` (SHA-256
`40956595C05713F1D41FC2CD123E80C91C68C57614A70DD21ACC8E2872C8908C`) after
stopping the idle MCP process that held the file. A fresh stdio MCP process
listed `you.subagent` and returned `BAD_REQUEST` `workingRoot must not be null`
for an explicit null. OpenCode MCP session
`43552c31-04d2-4ad1-816c-8d62ab745bb4` completed an exact README-heading probe
in about 9 seconds with `# you-agent-factory`.

Two concurrent OpenCode read-only architecture audits in the same workingRoot
(Factory Sessions `c7912a0b-fcba-4e9d-accc-5063b97a718a` and
`a602a91c-48c0-44f7-b3b8-e6ad7e6e4056`) both timed out at 150 seconds with no
source edits. OpenCode logs showed repeated shared snapshot Git `index.lock`
collisions; no `git.exe` was active and a zero-byte lock remained. Automatic
approval review rejected removal of that exact lock. Commit `70bd219a32`
defaulted OpenCode ACP v2 `OPENCODE_CONFIG_CONTENT={"snapshots":false}` unless
the operator already supplies the variable; full lint passed. The Windows
installed MCP binary SHA-256 is
`0F1BF5B56813B4B9508B17CE32A6B653A9EDFDF7C1CB5F7F56463BED286DB1BB`. Two
simultaneous OpenCode heading probes then both completed in 22 seconds
(sessions `8aca7beb-7883-4c37-a187-575ac0bb42dc` and
`d9de89e3-8447-4252-af3e-4fd24166ab7d`) with exact expected headings despite
the stale lock. A later concurrent backend/docs editing batch lost its tool
stdout at the host; worktree inspection found scoped partial edits to the replay
opener and harness docs, and independent tests found six replay fixture
failures. Codex MCP follow-up `bf98d715-9f7e-4b78-8783-4a7e85e4e059` repaired
the fixture without weakening assertions; the full Factory Sessions Go suite,
docs-reference smoke, and all 24 lint targets passed. The observed snapshot
collision is distinct from the unproven separate model/tool latency.

After commit `ae089306df`, the Windows binary was rebuilt and copied to the
configured MCP command path. Replacing it required stopping the idle server
process holding the executable open. The connector in this already-running
Codex chat did not reconnect: two concurrent OpenCode MCP calls and a later
single call each returned `Transport closed` immediately. This is a separate
host-connector lifecycle failure, so the new binary still needs a fresh-chat
MCP smoke test. Rebuild/install procedures should account for active MCP
connections and avoid claiming tool readiness solely from a successful copy.

A fresh standalone stdio MCP client then initialized the installed Windows
binary in two separate processes, listed all 11 tools including `you.subagent`,
and exited cleanly on EOF. One process accepted overlapping OpenCode calls and
returned two primary `COMPLETED` results in about 9 and 18 seconds (sessions
`86d4c19b-9513-4516-91b1-34a27063d54d` and
`9f73242c-4f23-4be9-8818-7fffd5e81236`). A third fresh process ran a real
OpenCode editing task: session `6a765726-39b7-486c-acdf-e02431afb3f1`
completed in about 130 seconds, removed the duplicate Factory Session sidecar
shutdown fallback, and added focused tests. Independent Factory Sessions Go
tests and `git diff --check` passed; commit `15364a6b62` contains the edit.
The installed MCP server therefore works through new stdio connections; the
earlier immediate `Transport closed` failures are specific to this Codex host
connector's old connection after its process was stopped for replacement.

A follow-up standalone MCP OpenCode edit for ACP peer-disconnect classification
timed out at 240 seconds (session `79a144dc-a54d-483e-8c35-a74ecaff3fd5`).
The typed result was `factory_session.subagent.timed_out` with
`INVOCATION_TIMED_OUT`, `partialEffectsPossible=true`, one in-flight dispatch,
and a closed Factory Session. There were no edits to its requested files.
OpenCode logs showed exploration of the ACP SDK module cache as late as 26
seconds before the deadline; the observed failure is task/model latency, not
an MCP connector close, permission denial, or snapshot lock. A narrower retry
was prepared with the exact SDK error shape and a single-file edit.

The narrower single-file OpenCode retry also timed out (session
`b2133aff-aa23-4a30-bad1-2b32db833ec7`, `timeoutMillis=180000`, response
after about 190 seconds) without an edit. Its log still showed `go doc`
exploration of the supplied `RequestError` shape. This repeated limit was too
short for the observed model/tool pace. A subsequent real editing retry uses a
15-minute limit; short heading probes remain bounded separately.

An `OpenCode` MCP edit for making typed MCP errors visible in text content
timed out at 300 seconds (session `ce494547-cdf8-4a21-865e-2a4ea72b9657`).
It had already made scoped partial edits in the server and result-policy
inventory, so the caller retained them and dispatched a 15-minute continuation
instead of restarting or discarding its work. This is another task-duration
timeout, with the server returning a typed timeout envelope rather than an
ambiguous transport closure. The model comparison does not establish a root cause.

The 20-minute OpenCode MCP Qwen3 reference-audio content probe completed, but
the standalone test client's Windows console encoding raised
`UnicodeEncodeError` while printing the MCP response. The OpenCode session
export recovered its primary result. The CLI TTS and managed ASR invocations
both exited successfully. The output WAV was valid mono 24 kHz audio, 7.44
seconds and non-silent. The generated speech transcript was
`0, 0, 0, 0, 0, 0, 0, 0, if I was a bitch.` for target text
`The quick brown fox jumps over the lazy dog.` The recorded TTS command passed
the requested text separately from reference WAV and `ref_text` (`Zero.`).
GPU samples from this particular run did not establish utilization. This is
a content-validation failure, not a proven reference-conditioning root cause;
ASR and plain Qwen synthesis need comparison. The temporary standalone client
was changed to UTF-8-safe output for subsequent calls.

The 15-minute OpenCode ACP peer-disconnect edit also timed out after writing a
partial `service.go` change. A second copy of its helper appeared near the
deadline, leaving the package temporarily uncompilable. Its server and ACP
child process tree were confirmed gone before retry. A fresh MCP call through
the `codex` provider (session `1f2e9dbd-85f9-4754-baec-f04aff392376`)
completed the scoped repair in about 181 seconds, kept one exact SDK-shape
classifier, and added positive/negative protocol tests. Commit `eb92eca62e`
contains it. A clean checkout at commit `3d9e9b0f08` passed ACP service,
workers wire, MCP server, Factory Sessions MCP transport, and every MCP
functional package test, independently of unrelated live changes in the
shared worktree.

The MCP error presentation OpenCode continuation completed (session
`6fe9c9f0-5eb8-456b-8b5b-d6ddda802523`): typed errors now put their safe
message in first text content with `isError=true`, retaining the typed envelope
in `structuredContent`. A protocol test follow-up
`c1020a44-26df-491b-a39a-20e46a8d6159` and resume decoder follow-ups
`6f0af534-32f6-46f5-84a8-a1a95ec7f35f` and
`5fd7df36-4777-4771-b593-ae15de98c867` covered the public stdio response
and exact message equality. Commits `b54051be6c`, `eb78f8297a`, and
`0e97ac3cc7` contain these changes; `3d9e9b0f08` added customer timeout/error
guidance. The installed binary has not yet been replaced because standalone
MCP sessions are still active.

A second 20-minute OpenCode MCP Qwen content comparison timed out with typed
`factory_session.subagent.timed_out` (session
`2d668037-cdb0-4e53-a0f9-a5df3a3206bf`, one in-flight dispatch). Its
managed ASR control transcribed a known fixture exactly as `Zero.`. The same
WSL root binary/config reported Qwen `models pull` as `ALREADY_READY`, while
the immediately following `models inspect` still reported `MISSING` and
`NOT_INSTALLED`; an online plain Qwen TTS invocation nevertheless succeeded
and wrote `/tmp/qwen-setup-online.wav` before the deadline. Thus inspect
readiness disagrees with runnable state. Plain output transcription remains
outstanding: `/tmp/qwen-setup-online.wav` was absent on the next independent
read, so the narrower follow-up must write to a persistent probe directory.

The narrowed plain Qwen3 OpenCode MCP call completed (Factory Session
`403d68d2-0830-4e3f-bf14-9040dea8cf63`) and returned a primary result.
Managed offline TTS without voice/reference produced persistent
`/home/andre/you-localai-probe/qwen-plain-probe.wav` (mono 16-bit PCM 24 kHz,
2.72 seconds, non-silent). Managed ASR transcribed it exactly as the target:
`The quick brown fox jumps over the lazy dog.` The known `Zero.` fixture also
transcribed exactly. The prior reference-conditioned run with the same target
spoke repeated zeroes instead. This comparison localizes the content failure
to the reference-conditioned request/backend path; it does not yet establish
which layer is wrong. Sixteen GPU samples during the plain run showed memory
increasing from 3692 to 6200 MiB and utilization of 7–44%, evidence of GPU
activity during invocation without direct process attribution. The task made
no repository edits.

Two 15-minute OpenCode MCP lint edits exposed different terminal behavior.
The MCP inventory refactor timed out with a typed error after partial edits;
the worker repeatedly reread the same small verifier before its deadline.
A fresh Codex-provider MCP continuation completed the baseline repair and
focused tests. The ACP test refactor made partial edits but returned no MCP
response even 100 seconds after `timeoutMillis=900000`; its client then closed
stdin. A fresh Codex-provider MCP continuation repaired the affected test.
The exact hang location is unproven because no Go goroutine dump was captured.
The invocation wait, cancel-on-timeout control, progress snapshot, and close
path each contain synchronous calls whose context deadline does not force a
noncooperative callee to return.

The OpenCode sidecar test consolidation completed with a primary result and
removed the extra Factory Sessions package file. At commit `239c55bc7f`, a
clean checkout passed all 24 `make lint` targets, including backend size,
maintainability, package file count, and deadcode. A separately built binary
from the same commit returned a readable first MCP text message,
`isError=true`, and the typed `structuredContent` for an invalid subagent
request over a real stdio MCP connection. The installed user binary has not
yet been replaced while other standalone MCP sessions remain live.

The 20-minute OpenCode matched-reference Qwen probe returned a typed
`factory_session.subagent.timed_out` envelope (session
`769ae091-71f0-4d00-8871-40c7e8702fc7`, one in-flight dispatch). Managed
ASR did transcribe the 6.435-second reference as 15 spoken "zero" words,
showing that the prior `ref_text="Zero."` was incomplete. The worker then
repeatedly used `--parameter "parameters.ref_text=..."`, which the CLI rejected
with `parse --parameter 1: invalid JSON`. No matched synthesis ran or output
WAV was created. A tighter retry must supply the known working
`--input 'parameters=json:{...}'` form explicitly; this is a prompt/tool-use failure,
not evidence of a TTS backend failure.

A fresh 10-minute OpenCode MCP retry included the exact working
`--input 'parameters=json:{...}'` form. It returned a primary `COMPLETED`
result (Factory Session `deb3fdf4-dd11-4e52-876c-4fbb6c90bcf1`). Managed
Qwen3 TTS and ASR exited successfully. The persistent matched-reference WAV
was PCM mono 16-bit 24 kHz, 11.6 seconds, and non-silent. ASR returned
`The quick brown fox jumps over the lady dog.` for requested `... lazy dog.`;
one word differs in the transcript. GPU utilization peaked at 77% while VRAM
rose from 3854 to 7346 MiB during the run, without direct process
attribution. The improvement over the earlier repeated-zero output strongly
supports reference-text mismatch as the earlier content failure, but does not
prove that as the sole cause. The OpenCode primary result overclaimed exact
content and miscounted words; independent artifact checks supplied the figures
above. The task made no repository edits.

Commit `e642113f48` changed omitted `you.subagent.timeoutMillis` to a named
20-minute default while preserving explicit 10- and 20-minute values. A clean
checkout passed all 24 lint targets, docs-reference smoke, and the focused
ACP/Factory Sessions/MCP functional suites. The Windows CLI binary was rebuilt
and installed at `C:\Users\andre\bin\you.exe` (SHA-256
`948649B9D0D52408FDE85EF9F9E3921A3FACEFE522F4FE88E1FFBE2057DBF7F2`).
A fresh stdio MCP process from that binary returned first-content readable
error text, `isError=true`, and typed `structuredContent` for an invalid
subagent request. The Codex MCP configuration now points at this binary for
new connections; existing live Codex MCP processes still use their original
executable until they reconnect.

Further MCP subagent runs fixed two concrete defects. A Codex-provider MCP
edit (session `3d7b1d28-4e07-4fa8-89d2-704a2854a3a8`) removed the
non-built-in cache-inspection gate for operator-defined models. A clean Linux
binary from commit `cb36e47a43` reported Qwen pull `ALREADY_READY/READY`,
then immediate inspect `READY/INSTALLED` with the same revision, cache path,
valid manifest, and two installed files. The preceding binary had returned
`MISSING/NOT_INSTALLED` on inspect despite that manifest. A later Codex MCP
edit (session `483d5ba0-def0-4ef9-a823-6b37baf6a5cf`) reduced test
complexity without changing the production fix. The Windows binary still
needs rebuilding from those later commits.

A Codex-provider MCP edit (session `3475ef21-2a7c-4a76-846b-8563becd7ad1`)
added bounded subagent Start, Invoke, snapshot, cancellation, and close
response paths; a second MCP edit (session
`09104b30-68df-4946-adda-c3b806485758`) refactored the implementation to
pass size and complexity gates. A late Start returns a typed timeout and
retains one admission slot until it can close any session created afterward.
Sixteen process-wide admission slots cap exposure if dependencies never
return, and capacity exhaustion rejects new starts. Confirmed close is the
only path that reports `sessionClosed=true`. The canonical invocation owner
now checks expired context before submitting Work. A clean checkout at commit
`942d93ada3` passed all 24 lint targets and the focused Factory Sessions,
Models, and MCP functional tests. The final Windows binary was installed at
`C:\Users\andre\bin\you.exe` (SHA-256
`83616D0C2F2193E02B3908E1FA935A1178A1A280D1D1A2F2354B07C4464953A6`).
A fresh stdio MCP call with omitted timeout completed in 27 seconds and
returned `you-agent-factory`. A separate 1 ms request returned a readable
typed `factory_session.subagent.timed_out` with `phase=start`, request ID,
and no false session-closed claim. These probes validate fast success and
early timeout; they do not reproduce a 20-minute non-cooperative dependency.

A final Codex-provider MCP edit (session
`2d1ae5af-2d59-428d-9471-c5affdf1c015`) added recovery guidance to the
typed capacity-exhausted error. At commit `70490643ec`, the clean checkout
again passed all 24 lint targets. The installed Windows binary was rebuilt
from that commit (SHA-256
`F6C8D15E7583406995B7AFBF4988B7EA09F4CEF6FD5C3F4AA42FB19F18E8CC09`).
Fresh stdio MCP processes from the installed binary returned readable
first-content errors with `isError=true` and typed `structuredContent` for
both invalid input and a 1 ms start-phase timeout. Existing Codex app MCP
processes using the old `.local\bin\you.exe` remain live; the configured
command for new connections points at the rebuilt `C:\Users\andre\bin\you.exe`.

During the LLM media audit, one OpenCode MCP read-only audit closed before
returning a primary result (session `8b72385a-e675-448b-8e73-2b1d42e282ca`).
A separate OpenCode MCP audio probe first hit a 120-second cold-start bound;
its 300-second retry completed and transcribed the known WAV exactly as
`The quick brown fox jumps over the lazy dog.`. Independent CLI probes from a
current-source Windows binary returned `Green` for a green PNG, `Blue` for a
blue MP4, and the same exact WAV transcript. The installed binary also
distinguished red and blue MP4 clips under the same prompt. These semantic
controls verify media understanding on this host; protocol field acceptance
alone did not establish it.

The first source-built Windows CLI failed before inference because automatic
NVIDIA detection populated an explicit CUDA accelerator while no compatible
published Windows CUDA archive was available. Commit `797801406e` leaves
Windows accelerator selection automatic, allowing the published resolver to
try CUDA and fall back to CPU. A fresh source build then pulled the built-in
LLM with both model and projector verified and completed the media probes.
The rebuilt binary was installed at `C:\Users\andre\bin\you.exe` (SHA-256
`97BF73C5CEA0E229D8AC0722597FCDB752D3202CCA0A54ADCFB30F52A1B81E0E`).
Its `models inspect llm` reported `READY`, two installed assets, and
`videoReadiness=verified-projector`. Already-running MCP processes retain
their executable until restarted.

An OpenCode-provider `you.subagent` edit (Factory Session
`1233753a-4609-45bd-89fb-f95980cf1ddd`) renamed the projector diagnostic
from `videoReadiness` to `mediaReadiness` across Models, the authored OpenAPI
description, generated clients, tests, and the model guide. The MCP call
returned a primary `COMPLETED` result after the multi-file edit. The Codex
MCP connection serving it still ran the older `.local\bin\you.exe` process,
despite the configured command pointing at `C:\Users\andre\bin\you.exe` for
new connections. A concurrent `factory_session_list` showed the live session,
while `factory_session_get` and `read_events` returned `session.not_found` for
its ID. This may be a tool-surface mismatch; the original invocation handle
remained live and completed successfully.

The subagent's final report described `make api-smoke` as passing while also
noting generated drift. Independently rerunning the gate before commit returned
exit 1 because its drift check compares generated artifacts with `HEAD`; the
diff was the expected authored description change. After commit `5c73b77925`,
`make api-smoke` returned exit 0. Models Go tests, the focused projector
functional test, and docs-reference smoke also passed. The lesson is to keep
the exact command exit status in the primary result and distinguish a gate
that will pass after committing from one that already passed.

The next concurrent MCP exercise submitted three bounded jobs in one
workspace: two OpenCode lint repairs and one Pi formatting repair. Pi returned
primary `COMPLETED` for Factory Session
`f07fee01-800c-4ad5-a1a2-caed554d127b`, but its entire answer was retry
progress and `Retry finished, resuming.`; `make fmt-check` still failed on the
same three files. Pi's session JSONL recorded four assistant messages with
`stopReason=error` and `errorMessage=Connection error.`. Its selected local
model was `llama-cpp/qwen-3.8-uncensored` at `http://localhost:8080/v1`, and
TCP port 8080 was closed. This is a confirmed false-success terminal outcome:
the provider failed to connect, while the Factory invocation treated retry
notices as the answer. An OpenCode task is tracing the missing ACP/provider
failure classification.

The two OpenCode jobs both reached their explicit 20-minute bounds and
returned typed `factory_session.subagent.timed_out`, with request IDs,
`partialEffectsPossible=true`, `inFlightDispatches=1`, and confirmed cleanup.
The Models lint task (session `db7eb48f-600a-4aa7-9768-4d7c396e2a0e`)
left a compiling partial refactor: `backend-size` and `pkg-maint` passed, but
`pkg-file-count` still failed in two packages. The package-boundary task
(session `29262435-2572-4150-a377-52fd3b7c1055`) made no file edits.
The ACP service currently retains one daemon per provider and gates its
execution, so these accepted sessions do not prove simultaneous OpenCode
turns. The OpenCode provider-failure-classification task (session
`e2fd1cf3-ad14-4453-8656-da4953cfedcf`) also reached its 20-minute bound
without editing. The Codex-provider continuation (session
`0f9966cb-d637-4895-8419-b6157b9bc664`) completed the Models file-size,
complexity, and package-count repair; independently rerun focused Go tests
and all three gates passed. Commit `0d47d528d4` records the repair.

A Codex-provider MCP boundary/fmt repair (session
`b07865a9-6cc8-4d48-9ae3-af9c085e5f08`) returned a primary result and
removed five test-only peer wire imports. Its claim that `make fmt-check`
passed required qualification: with three deleted `TestMain` files still
unstaged, the gate enumerated them through `git ls-files --cached` and failed
to open them. Staging the deletion made `make pkg-boundary fmt-check` pass.
Focused runtime, system-initialization, and worker-session Go tests passed;
commit `f49004df3b` records the boundary edit. Full lint then exposed one
new runtime test file beyond the package-count limit. A narrow Codex-provider
MCP follow-up (session `617fdba6-a9f9-4a64-9029-52fc970410b2`) merged that
helper into its existing test file. Independent `make pkg-file-count` and the
two focused runtime tests passed; commit `6fe9728a77` records the follow-up.
Full `make lint` then passed all 24 targets.

An OpenCode-provider MCP media audit (session
`7a0695ea-517a-46c2-bc57-b01ab5b5edc0`) returned a primary report, but
its text repeated one progress sentence several times and called the OMNI
path sound without live semantic proof. Independent offline probes established
audio understanding (`zero` for a known spoken-digit WAV) and one-frame
video understanding (correct red background and `PHASE 1` label). A four-second
100-frame MP4 returned empty text or a generic invoke failure. Removing its
audio track and reducing it to four frames did not resolve the failure. A
Codex-provider MCP experiment (session `6a6bc658-e9e3-405e-84c0-3b805a3cde29`)
raised only the video generation-token budget from 256 to 1024; the live
four-second probe still failed, so the subagent reverted the experiment.
This distinguishes protocol acceptance and projector readiness from the
longer-video behavior actually observed on this Windows backend.

An OpenCode-provider MCP implementation task for VIDEO Work content (session
`87657a4c-3081-42af-9ed5-3bca5155e2a9`) reached its 20-minute bound and
returned typed timeout with partial edits. Its focused tests passed, but two
maintainability checks still failed. A Codex-provider MCP continuation
(session `28b2f3d2-d5e5-4b83-89e8-36a63efc65d7`) completed the refactor;
commit `6f71ac44f5` records VIDEO Work content and generated contracts.

The next Codex-provider MCP task (session
`51070abe-c959-49f5-8eda-821e4da1e0fb`) completed Workers inference
media-byte materialization and tests. Independent review extended its URL
handling to `http`, `https`, and `data` Work content, then focused Go tests
and full lint passed. A concurrent, read-only OpenCode MCP audit (session
`8b031ff9-9041-4c69-8b5c-8b7482cfe596`) returned a primary result but
asserted that the multi-frame video failure was inside the pinned backend
without backend logs. That boundary remains unproven. An independent
two-frame, 160x120, no-audio MP4 invoke also failed with a generic internal
server error. The live observations therefore show one-frame success and
failure at two or more frames in these fixtures, not a confirmed root cause.
Repeating the two-frame probe with `--debug` reported only
`model runtime stage failed: INVOKE (INVOCATION_FAILED)` beneath the generic
CLI error; it did not expose the backend's underlying error or response.
Commit `ce336bbbc3` records the Work media-byte repair and Factory VIDEO
capability mapping. Focused Go tests, dashboard typecheck, docs-reference
smoke, all 24 lint targets, and post-commit API smoke passed.

The user clarified that the next file-upload scope was the direct Models
endpoint. A narrow OpenCode-provider MCP task to add direct JSON media tests
and docs (session `4b34a453-a3c3-44f1-99ae-f69934dc5ee3`) timed out after
10 minutes without a primary result or requested edit; the tool returned a
typed `factory_session.subagent.timed_out` with `partialEffectsPossible=true`
and confirmed session cleanup. A Codex subagent completed direct HTTP
multipart upload handling, and the root agent added the JSON media proof and
docs. Commit `cd2c464910` records this direct endpoint change. The timeout
is another bounded OpenCode harness failure, not evidence that the requested
edit was performed. Focused HTTP and contract tests, dashboard typecheck,
docs-reference smoke, all 24 lint targets, and post-commit API smoke passed.

An OpenCode MCP probe with explicit model `big-pickle` (session
`b8d91515-091a-49fc-bd6f-579aef127d41`) closed quickly with generic
`factory_session.subagent.execution_failed`, `INVOCATION_RUNTIME_FAILURE`, no
`failureReason`, and no edit. The available OpenCode log showed startup but did
not establish whether the model was rejected or another provider failure
occurred. A Codex MCP audit (session
`4c0f9cef-84f4-4158-96a1-e5f80c6240ce`) traced a concrete classification
gap: Workers maps provider invalid requests to `permanent_bad_request`, while
the MCP terminal handler omitted that reason; Factory Sessions can also leave
the reason empty if no Work ID matches and more than one failure detail exists.
A Codex MCP edit (session `80fa86c5-8fe1-4247-bb9b-db8eaf6d4310`) added
safe terminal classifications for permanent bad request, internal server error,
and unknown failure, plus guidance for an empty reason. Focused MCP tests pass
and assert that sensitive provider text is never copied to the response.

A bounded OpenCode MCP read-only audit of direct Models multipart upload
(session `d6523b15-ea35-4046-88c4-97764bcaf07b`) returned a primary result,
but falsely claimed that the multipart handler had zero direct test coverage.
Its four-file limit omitted `handler_test.go`, which already tests audio/video
upload, matching, and invalid parts. This is an audit accuracy failure, not an
execution failure. A follow-up audit was sent to that exact test file to check
whether OpenCode corrects its conclusion from source evidence.

The follow-up OpenCode audit (session `99e8b2b4-1546-4e03-a184-bc9f5bb54109`)
correctly listed multipart audio/video and invalid-input coverage after
reading `handler_test.go`, but its proposed replacement gap was also false:
`invoke_operations_test.go` already contains direct `InvokeGenericModel` JSON
handler tests, including media bytes and invalid input. Narrow file limits
made the audits fast and completed, but conclusions about absent coverage need
an explicit repository-wide search before they can be trusted.

The first classification edit passed focused Go tests but failed the package
complexity and file-count lint gates. A second Codex MCP edit (session
`9ccbed93-e36d-4cfd-a9b6-60b5d4197f76`) moved the tests into an existing
file and split the terminal classification helper. Focused Go tests, the two
previously failing lint targets, and `git diff --check` then passed.

Commit `6cc66b3254` records the safe classification repair. A rebuilt Windows
binary (SHA-256 `7C517C387A9A158E6F167173B02B800338933C15317F2D011B8B466C8E254886`)
initialized a fresh stdio MCP process and listed all 11 tools. Several active
MCP processes held the old `C:\Users\andre\bin\you.exe` open, so copying over
that file failed with Windows sharing violation. To avoid interrupting those
connections, the rebuilt binary was installed at
`C:\Users\andre\bin\you-6cc66b3254.exe`, and the configured command for new
Codex MCP connections now points to that versioned executable. Existing
connections keep their prior process image until they close.

A further Codex MCP edit (session `e318cac9-72ce-4cf1-b85d-7aea3923f410`)
addressed one source of missing terminal classifications: when primary-result
selection lacks a matching Work ID but provides a dispatch ID, Factory
Sessions now carries a failure reason only if exactly one non-nil failure
detail matches that dispatch. Exact Work ID still takes precedence, and
ambiguous matches remain unclassified. Focused invocation tests cover both
cases and confirm that sensitive provider messages remain absent; the package
tests and maintainability gate passed. This does not prove the cause of the
`big-pickle` failure, whose retained result did not include the underlying
failure detail.

Commit `df7163d36c` records that dispatch correlation change. The final
Windows binary was rebuilt at `C:\Users\andre\bin\you-df7163d36c.exe`
(SHA-256 `EFE62DCE70DEE55C64376E6D67DCD496C0513AC4E8DC69D6FCB9DF9D41EE272E`),
and a fresh stdio MCP initialize request completed successfully. The
configured MCP command now points to this binary; old active MCP processes
continue to run their prior image until closed.

The direct Models HTTP file path received live semantic probes through a
fresh `you-df7163d36c.exe` local server on port 7548. Multipart
`POST /models/invocations` with the known spoken-digit WAV returned HTTP 200
and `zero`; a one-frame solid-red MP4 returned HTTP 200 and `Red`; a two-frame
solid-red MP4 also returned HTTP 200 and `Red`. A two-frame red-then-blue MP4
returned HTTP 502 `MODEL_BACKEND_FAILURE` with `OMNI response did not contain
text output`. These are file-upload and inference results, not merely request
mapping tests. The solid-red two-frame success disproves a universal
two-frame limit suggested by earlier fixtures. A bounded OpenCode MCP audit
(session `2d945925-9727-491e-a7d2-f0311e78463f`) traced the 502 to
`OmniCodec.Invoke`: the gRPC `Reply` decoded without a transport error, but
neither `Message` nor `ChatDeltas` yielded text. The adapter currently drops
reply shape and token metadata before the codec checks for empty text, so the
root cause of this content-sensitive failure remains unproven. The local HTTP
server was stopped gracefully after the probes.

A fresh standalone MCP process from `you-df7163d36c.exe` then received a
real `tools/call` for `you.subagent` with provider `opencode` and deliberately
unadvertised model `does-not-exist`. The provider/Worker path recorded
`permanent_bad_request`; MCP returned `isError=true`, first content text
`provider rejected the subagent request`, structured code
`factory_session.subagent.provider_request_rejected`,
`failureReason=permanent_bad_request`, `retryable=false`, confirmed cleanup,
and guidance to verify the selected model. This end-to-end check confirms the
new typed classification works for an actual invalid-model failure. It does
not explain the separate `big-pickle` generic failure.

An OpenCode MCP edit (session `d7859fe6-9d31-4cd4-86df-decae60566ec`)
added a typed LocalAI Predict failure for an empty gRPC response payload.
This separates a zero-byte response from a decoded reply with no text, which
the prior protobuf decoder conflated. Focused tests initially passed, but
`pkg-maint` found a new complexity violation and an oversized test file. The
bounded OpenCode lint repair (session `8b336e18-f3eb-4cf8-96b7-bc8dd6a5f050`)
timed out after partial edits, leaving one test uncompilable and the file two
lines over the limit. A Codex MCP continuation (session
`8080888d-1cd8-427b-b37a-c1e85b30d08b`) repaired the declaration, moved
the focused tests into an existing smaller test file, and retained the
response-decoding helper. Independent LocalAI package tests, `pkg-maint`,
`pkg-file-count`, `fmt-check`, and `git diff --check` passed. The observed
red-then-blue video only proved that no text reached the codec; a repeat with
this guard is needed to distinguish a zero-byte payload from a decoded reply
with empty text. The guard does not claim to fix that content failure.

A Codex MCP read-only audit (session `d98cce9c-1f8f-43c8-88c7-6bcc44d1b792`)
traced the earlier Pi false success through the ACP adapter: a non-error
`session/prompt` response and nonempty retry text become a successful worker
result. Inspection of installed `pi-acp` 0.0.34 source then found that its
`PiAcpAgent.prompt` maps an internal `result === "error"` to ACP
`stopReason: "end_turn"`; its turn settlement path also resolves `end_turn`.
The Pi session JSONL retained `stopReason="error"` for the earlier failure,
but that typed status was not represented in the ACP facts consumed by this
adapter. The next safe fix must preserve a typed Pi failure signal across the
provider boundary (or use a direct Pi protocol) rather than classify the
English retry-status text as an error. The installed package is the current
published 0.0.34 version at the time of this audit.

Commit `4e8ac01594` records the LocalAI Predict guard. A new Windows binary
was built at `C:\Users\andre\bin\you-4e8ac01594.exe` (SHA-256
`98C8620BE9956A2CD1467A8FDDAF4302F05206BD9D36B53DB227E8BA419D1521`),
and the configured MCP command for new connections points to it. A fresh
HTTP server from that binary repeated the exact red-then-blue two-frame
multipart request and again returned HTTP 502 `OMNI response did not contain
text output`. Because the new zero-byte guard did not fire, this attempt
returned a nonempty, decodable protobuf payload with no usable text in its
legacy message or chat deltas. The payload's other field values are still
unknown. The server was stopped gracefully after the probe.

A Codex MCP edit (session `b826620c-6ad2-4c65-9a51-99bbf40be631`)
added safe numeric reply-shape counts to the LocalAI empty-text failure.
Commit `61dfb43650` records that change; LocalAI package tests and the
package-size, file-count, and formatting gates passed. A fresh Windows build
repeated the same red-then-blue multipart clip and returned HTTP 502 with
`reply_bytes=984 message_bytes=0 chat_delta_count=1 generated_tokens=256
prompt_tokens=451 audio_bytes=0`. The generated token count exactly matched
the source's 256-token Predict cap. A temporary build with only that cap
raised to 1024 returned HTTP 500 `model invocation failed` after a longer
run; its server log recorded `model runtime stage failed: INVOKE
(INVOCATION_FAILED)` without a lower-level cause. The experimental source
change was reverted and was not committed. This is evidence that the 256 cap
may contribute to the empty-text result, but 1024 did not produce a usable
answer in this controlled clip.

A fresh standalone MCP Pi probe from the rebuilt binary first returned typed
`provider_misconfigured` because the Providers executable locator was nil,
even though `pi --version` reported 0.87.0. A Codex MCP edit (session
`a85e9418-7579-44c5-abf1-6251e3db1ca4`) selected the host locator by
default while preserving injected overrides; commit `f107bace18` records
the fix. Focused `pkg/wire` tests and maintainability, file-count, and
formatting checks passed. The next standalone `you.subagent(provider=pi)`
advanced past preflight, but returned `COMPLETED` with only three retry
notices and `Retry finished, resuming.`; the newly written Pi session JSONL
contained four assistant records with `stopReason=error` and `Connection
error.`. This independently reproduces the typed-failure loss after fixing
the OOTB locator. A Codex MCP design audit (session
`84ce972b-d75c-4d23-b833-c338d360c5e1`) recommended a controlled Pi
bridge that emits a typed prompt failure, or a direct Pi RPC provider, rather
than interpreting retry prose. The current configured MCP command points to
`C:\Users\andre\bin\you-f107bace18.exe` (SHA-256
`F7A72BA4BF1083DB866EC89B71DD107C38D95077EBB07324345E7291968F4B66`).

A further Codex MCP edit (session `36a3a0c9-e461-42a7-91ef-1e62d536f46c`)
counted `ChatDelta.ReasoningContent` bytes in the safe empty-text diagnostic;
commit `e993bfa890` records it. Focused LocalAI tests and maintainability,
file-count, and formatting checks passed. A fresh Windows binary repeated the
same red-then-blue two-frame direct Models multipart request. The HTTP 502
reported `reply_bytes=984 message_bytes=0 chat_delta_count=1
generated_tokens=256 prompt_tokens=451 audio_bytes=0 reasoning_bytes=954`.
All 256 generated tokens were spent on reasoning content with no visible answer
before the request cap. The server was stopped gracefully. A targeted fix should
control thinking for this managed model or request and then repeat the live
video probe; merely raising the global cap to 1024 had already failed.

A bounded Codex MCP edit (session `722ebce3-b9e9-4d58-856c-82073e007221`)
added `reasoning_budget:0` only to the managed built-in Gemma 4 LLM's
`LoadModel` options. The pinned LocalAI llama.cpp backend accepts this option
and treats zero as disabling the reasoning budget. Exact wire and scope tests,
the LocalAI package suite, `pkg-maint`, `pkg-file-count`, `fmt-check`, and
`git diff --check` passed. A fresh Windows build sent the same red-then-blue
two-frame MP4 through direct `POST /models/invocations` multipart and received
HTTP 200 with text `Red blue`. The known WAV fixture returned HTTP 200 with
`zero `, and the single red frame returned HTTP 200 with `Red`. The test server
was stopped gracefully. These three live probes confirm the focused managed
model setting fixes the observed video failure while retaining the previously
working audio and single-frame paths.

A concurrent bounded OpenCode MCP read-only audit of the Pi ACP false success
(session `28a859c9-3865-4ca2-8c40-ca305efbfbd3`, request
`43b23c8f-f8f2-480d-821c-258c08aacc63`) timed out at 240 seconds with no
primary result. MCP returned `factory_session.subagent.timed_out`,
`partialEffectsPossible=true`, `inFlightDispatches=1`, and
`sessionClosed=true`. The audit was instructed not to edit and no new
Pi-related tracked diffs appeared; the working tree still has unrelated
preexisting dirty files. This is another OpenCode harness timeout to investigate.

Commit `04fdf3a21d` records the managed Gemma load option, tests, and these
observations. The installed Windows binary was rebuilt as
`C:\Users\andre\bin\you-04fdf3a21d.exe` (SHA-256
`7A2E19ECEB41599EF5F6B0C2DF19AE6C406942AEC6011C202F530B20948A7B41`),
and the Codex MCP command for new connections points to it.
A fresh standalone MCP stdio handshake from this binary returned the expected
protocol initialization and advertised `you.subagent` in `tools/list`.

Follow-up inspection of OpenCode's local log for timed-out session
`28a859c9-3865-4ca2-8c40-ca305efbfbd3` found run `58a4d58e` actively
searching the workspace and issuing `go doc` commands through
`2026-09-28T22:43:59Z`, shortly before the explicit 240-second MCP deadline.
The request used a broad ACP audit prompt and an explicit four-minute timeout,
while the MCP default is twenty minutes. This evidence supports a task overrun
under the caller's short deadline, not an idle or crashed OpenCode process. It
does not yet prove whether cleanup preserved any partial answer or whether
OpenCode would have finished under the default deadline.

A bounded Codex MCP Pi audit (session `58655a37-175a-49a4-a8a4-6bd4aa3b485a`)
confirmed no reliable typed final Pi failure reaches the current ACP adapter.
The installed `pi-acp` 0.0.34 turns Pi's final error into ACP `end_turn` and
emits retry notices as ordinary message chunks. A controlled bridge must retain
the *settled* Pi outcome, since an earlier failed attempt can be followed by a
successful retry. Inspecting Pi session JSONL after the fact cannot reliably
associate the exact prompt outcome, and parsing English retry text remains
unsound. No edit was made. Focused ACP failure-classification tests passed;
the full ACP service package had an existing dirty Pi preflight failure in
`TestPiPreflight_CanceledContext` (`Canceled` expected, `misconfigured` seen).

A deliberately narrow OpenCode MCP read-only audit (session
`1fc6ea28-03db-41d9-a646-7eea7ee74ea7`) completed with a primary result
in under a minute under a ten-minute explicit deadline. Independent source
inspection confirmed its core finding: direct Models TTS already accepts a
PCM WAV `voice` input and a `ref_text` parameter, carries them through the
codec, stages the voice file, and sends the reference transcript in the LocalAI
TTS protobuf parameters. This was already validated with managed WSL Qwen3
and IndexTTS GPU runs in `localai-tts-reference-audio-audit-2026-09-28.md`;
the new audit is useful harness success evidence, not new inference proof.

A Codex MCP edit (session `69eb55bc-6ddd-4571-9019-087b0868ce81`) added
`progress.lastObservedProviderActivity` to timeout errors by reading the live
Factory Session's retained response events before cleanup. It reports only
validated event kind/phase, age in milliseconds, and whether a provider
session reference was observed; provider text, payloads, and metadata values
are excluded. A failed read does not change timeout cleanup. The field is
named *observed* because filtered ACP fragments can be newer than the last
retained Factory Session event. The full MCP transport package and
maintainability/file-count/format checks passed; an independent package rerun
also passed.

The user then directed removal of arbitrary default reasoning, output-token,
and context-input caps. A Codex MCP edit (session
`a697c91d-cb01-43ed-9fca-989c935fc61c`) removed the fixed 256-token
Predict default and the Gemma `reasoning_budget:0` load option, preserving
reasoning. The pinned llama.cpp backend maps zero Predict tokens to
`n_predict=-1`. A scoped OpenCode MCP edit (session
`312d2a05-92e0-40d0-ada8-f09401a4008d`) then removed the built-in
embedding model's 512-token context override, so the backend/model chooses
context. Focused tests passed. Byte bounds for uploads and returned media
remain because they protect process memory; these are not context-token caps.

Live Windows direct Models probes with the no-cap build returned HTTP 200 for
plain text (`READY`) and a single red video frame (`Red`), but the same
red-then-blue multipart MP4 returned HTTP 500. A bounded Codex MCP raw-error
audit (session `38e13935-5958-4724-94a5-bd0c97d739cb`) timed out at ten
minutes without a primary result or tracked edits. Independent raw gRPC probes
against the loaded backend then completed in about 67 seconds with a nonempty
reply (376 generated tokens, 451 prompt tokens, visible text `Red, Red`, and
reasoning content); the visual answer itself is inaccurate for the desired
two-color sequence, so this is protocol evidence rather than semantic success.
Temporary diagnostic binaries found that the HTTP path's LocalAI Predict RPC
also succeeds with a 1405-byte reply. The later Models invocation fails after
the one-minute host lease expires: the lease owner releases capacity at TTL,
and inference completion reports a failed cleanup. The elapsed wall time also
includes model startup. A correct fix needs to pin an active invocation's
lease until it finishes while retaining expiration for abandoned leases; a
Codex MCP implementation task is in progress. Temporary diagnostic source
changes were reverted and were not committed. Test servers were stopped.
The user asked that probes stop opening browser tabs. Subsequent server probes
used `YOU_NO_BROWSER_OPEN=1`, whose wired opener is a no-op; no further
automatic browser opens are intended. The server startup line still says
`Opening dashboard` despite the no-op and should be corrected separately.

A Codex MCP edit (session `337b4a83-ac7b-4080-bbae-e197adba9a6e`)
implemented invocation-owned managed leases. Inference atomically claims a
detached lease before backend execution; claimed capacity stays reserved past
the one-minute detached-lease TTL and returns only when invocation cleanup
finishes. Unclaimed leases still expire. Cancellation retains capacity until
the backend exits, and an external release cannot free an in-flight claim.
Fake-clock and cancellation tests cover expiration, admission, and cleanup.
The full Models Go suite, repeated targeted race tests, file-count check, and
diff check passed. An independent focused package rerun passed.

A fresh Windows binary with the no-cap and lease changes completed the same
direct Models two-frame video request in 111 seconds with HTTP 200. Its text
was `Red, Red`, whereas the fixture is red then blue. This proves the lease
failure is fixed but leaves a video-understanding accuracy issue. The server
was launched hidden with `YOU_NO_BROWSER_OPEN=1` and stopped gracefully; no
dashboard tab was opened.

Commit `4823783075` records the no-cap model settings and the lease fix. The
Windows MCP binary was installed as `C:\Users\andre\bin\you-4823783075.exe`
(SHA-256 `7EBF5DCA35A103B572FF0BBEB7ED2F21A138CE6C5339FD68736746F1A96FA117`),
and the Codex MCP command for new connections points to it.

A local decoder validation of the fixture `.tmp/video-red-blue.mp4` with
ffprobe/ffmpeg 7.0 (essentials build) found it well-formed, not malformed: a
1644-byte H.264 High-profile MP4, 160x120 yuv420p, 1 fps, 2 frames, 2.000 s
duration, probe score 100, decoding with no errors or warnings. Frame 0
(pts 0.000000, I-frame) decodes to a single uniform color RGB (253,0,0) red
and frame 1 (pts 1.000000, P-frame) to RGB (0,0,254) blue, 19200/19200 pixels
each with one distinct color per frame. The repeated `Red, Red` answers are
therefore LocalAI frame-sampling/model behavior, not fixture corruption. This
recorded a prior completed OpenCode audit (session
`f5a6a649-8724-41cc-bf68-9a16c0a6a8a5`) whose finding was that the Models
mapping treats the MP4 as opaque bytes through `grpc_protocol.go` `Videos[0]`
(`predictInputValue` base64-encodes the content and `predictOptions` appends it
to the repeated `options.Videos` field), and that a direct raw gRPC probe
against the loaded backend also answered `Red, Red`. Remaining inference, not
proven by the decoder: which frame(s) the backend sampled and why the model
reported red twice; the decoder establishes fixture content and the mapping
path only.

A concurrent Codex MCP implementation attempt for Pi's false-success terminal
outcome (session `4f45b364-3578-400d-b772-13019ff1fa21`) completed with no
edits. Inspection confirmed installed `pi-acp` 0.0.34 receives Pi's typed
assistant outcome but resolves its `agent_settled` handler as ACP `end_turn`
without forwarding that outcome. Retry events are plain message chunks. The
Go ACP boundary cannot distinguish a final failure from a successful retry
reliably without replacing or patching the upstream bridge to carry the final
settled outcome as typed metadata or a prompt error. Parsing text or session
JSONL remains unsafe. Focused ACP tests passed; the broader Pi preflight test
still fails in the preexisting dirty `TestPiPreflight_CanceledContext` case.

A second bounded Codex MCP task (session
`abc05780-461a-4740-875f-d9e8e8a995c4`) checked distribution feasibility.
The authored Pi harness launches external `npx pi-acp`; this repository's
model-provider package publishes catalog data, not an ACP executable. A
cache-local patch would not reach customers. A truthful OOTB fix needs a
corrected and distributed `pi-acp` bridge that preserves the final assistant
`message_end` stop reason at `agent_settled`, then a pinned harness update and
tests for final error versus failed-attempt-then-successful-retry. The task
made no edits; this remains an open product dependency rather than a fixed
Go-side classification.

A concurrent OpenCode MCP read-only audit of pinned LocalAI video sampling
(session `538a9b7c-5794-4bc5-935f-349be71e5608`) returned a primary result,
but it asserted that `Red, Red` is expected without identifying the exact C++
video-frame sampling implementation. That assertion is unsupported. A scoped
correction request (session `0baa565e-0e64-4b42-b031-378dd885b69a`) timed
out after 300 seconds with no result and no tracked edits. MCP returned
`factory_session.subagent.timed_out`, `partialEffectsPossible=true`, and
`inFlightDispatches=1`. The decoder's frame/timestamp selection remains
unverified; the opaque MP4 mapping and local fixture frames are established.
An independent command-line read of pinned LocalAI
`backend/cpp/llama-cpp/grpc-server.cpp` found that the C++ gRPC server
base64-decodes each `video_data` entry into `files` (around lines 2537-2545)
and passes those files to `process_mtmd_prompt` (around line 2556). This
confirms the raw bytes reach llama.cpp's multimodal path, but still does not
establish which frames/timestamps that path selected or why the model replied
`Red, Red`.

The full `make lint` run passed vet, deadcode, formatting, contracts, and most
other targets, but failed four targets: `backend-size` and `pkg-maint` from the
lease change's 1006-line `runtime_factory_scoped_pull_test.go` and
`InvokeModelWithLease` complexity 17 (limit 15); `pkg-boundary` from a
`time.Now` call in the MCP timeout diagnostic and an existing local
`pkg/services/models/.artifacts/direct_video_probe.go`; and `pkg-structure`
from that same `.artifacts` directory. Focused MCP repair work is in progress
for the tracked regressions. The local diagnostic directory remains outside
the staged change set.

A Codex MCP lint repair (session `394b9a67-722f-44a2-bb5e-c2b37363dfb0`)
extracted runtime-outcome handling from `InvokeModelWithLease` and moved the
complete runtime-scope close race test into its own file. Focused tests,
`pkg-maint`, `backend-size`, and diff checks passed; the original test file is
now 879 lines. An OpenCode MCP repair (session
`c5e46881-b92a-4655-abcf-75bcedd76da9`) replaced the MCP timeout
diagnostic's computed age with the retained event's `RecordedAt` timestamp,
removing its production `time.Now` dependency. The full MCP transport suite
passed. `pkg-boundary` no longer reports that clock call; its three remaining
findings all come from the untracked local `.artifacts/direct_video_probe.go`.

Commit `b04fb8704d` records both lint repairs and this audit. The rebuilt
Windows MCP binary is `C:\Users\andre\bin\you-b04fb8704d.exe` (SHA-256
`4AEDB087F5542C230C6D167D1430CB3AC3B57A24E9638820F003B418A27A76FA`);
the Codex MCP command for new connections points to it. A fresh headless
stdio initialize and `tools/list` returned 11 tools including `you.subagent`.

The ignored `.artifacts/direct_video_probe.go` was identified as this
session's earlier temporary raw-gRPC diagnostic and preserved at
`C:\Users\andre\.codex\worktrees\mcp-goal-integration\diagnostics\direct_video_probe.go`,
outside the package tree. A full lint rerun then passed every target except
`pkg-file-count`: the new race-test file raised `pkg/services/models/internal/service`
from its recorded 16 files to 17. A Codex MCP follow-up (session
`3a287f02-8921-4a91-b097-c6e43281686f`) moved that test into the existing
`runtime_factory_inference_test.go` and removed the extra file. The destination
is 942 lines; focused package tests, `pkg-maint`, `pkg-file-count`,
`backend-size`, and diff checks passed.
After staging the deletion of the temporary split test file, `fmt-check`
passed, and the final `make lint` run passed all 24 targets. The first full
rerun after moving the diagnostic source had failed only `pkg-file-count`;
the intermediate rerun after the test relocation failed only because the
removed tracked test file was not yet staged and the formatter tried to read
it. Neither was a remaining code violation after staging the intended edit.

A Codex MCP architecture audit (session
`667f57f0-0509-4c26-9209-594c06039407`) found a remaining renamed
application-opening seam: `RunSelection.Open` returned `RunApplication`, and
`Initializer.Run` called `Open` then `Run`. A Codex MCP edit (session
`3b69fa98-8b0a-4b81-a190-e67bbce8e81f`) removed that staged handoff:
`RunSelection.Run` now executes the selected CLI path directly, and the
initializer no longer stages a `RunApplication`. Focused initializer, CLI,
run, and wire tests, targeted vet and architecture checks, and all 24 `make
lint` targets passed. Commit `2105d425fe` records this slice. The CLI run
operation's own `Open`/`Run` stage remains and is being flattened next.

A 12-minute OpenCode MCP GPU embedding task timed out with no primary result
or document edit (session `4e0f89d9-d3b0-4afa-b32a-89a9cf29ee90`). MCP
reported `factory_session.subagent.timed_out`, `partialEffectsPossible=true`,
and `inFlightDispatches=1`; `you.factory_session.get` then returned
`session.not_found`. The OpenCode log showed an invocation and later commands
that embedded a large vector literal for counting, but no reliable final
validation from the agent. A separate concise direct offline CLI probe with
the installed binary completed and returned an `EMBED` JSON output with
1,024 values. The cached Windows llama.cpp backend archive's own
`build-metadata.json` declares `buildType: cpu` and `accelerators: [cpu]`, so
GPU embedding is still unverified. The current default artifact manifest has
only CPU/Metal targets despite CUDA-variant resolver tests. A scoped CUDA
artifact/resolver implementation task is in progress. An older diagnostic
server on port 7557 was found and stopped gracefully; no browser was opened.

A Codex MCP follow-up (session `2d1d8857-ccde-448c-b0de-672983e2d36c`)
removed the CLI run `OperationFactory`, `Operation`, `Open`, and
`OpenWithVisualizationOwnerAndDiagnostics` staged runtime path. CLI selection
now calls `RunSelected` with injected Factory Sessions dependencies. Wire was
regenerated. Focused CLI/run/wire tests and targeted architecture checks
passed. The full CLI subtree test attempted by the agent failed in separate
contract/fixture packages; this needs independent triage before claiming the
broader subtree is green. An initial `pkg-maint` check found the new
`runHostedRuntime` at complexity 24 and 101 lines. A bounded Codex MCP repair
(session `7120361b-b270-4c42-a45e-e68a68db2027`) extracted local helpers;
focused CLI/run/wire tests, `pkg-maint`, `pkg-file-count`, `backend-size`,
`fmt-check`, and diff checks then passed. Commit `b24531d9b0` records this
slice. The DirectJavaScript `Open`/application-plan handoff remains and is
being flattened separately.

A Codex MCP follow-up (session `da3402bc-b29e-4051-ada8-d0339a080346`)
removed the remaining DirectJavaScript `Open`/application-plan handoff.
`DirectJavaScriptRunOperation.Run` now starts the canonical Factory Session
and owns request-scoped optional HTTP host lifecycle; CLI selection no longer
receives an application or plan. The unused application type was removed and
wire regenerated. Focused CLI run/session execution/wire tests, wire smoke,
maintainability/file-count/size, and architecture checks passed. Commit
`0b40aafe1f` records it. The implementation still uses an internal lifecycle
plan builder and request-scoped HTTP host component; those are separate
remaining flattening candidates, not an application-opening handoff.

A Codex MCP artifact task (session `075853f2-d3a5-48fd-90b7-f7f3aef41ae3`)
added a Linux llama.cpp CUDA archive leg to the authored build matrix, with
CUDA toolkit checks and package verification. A concurrent Windows follow-up
(session `b6cc047b-dfac-4620-aaba-61056fc56c86`) added a Visual Studio/MSVC
CUDA build leg while retaining MinGW for the CPU archives. A scoped review
(session `433c3bd6-72c6-4b6e-bc5b-de5d1c03a0c4`) broadened the Windows
binary's supported CUDA architectures and guarded recursive generated-path
cleanup against unexpected targets and links. The 11-leg workflow config,
Node tests (14 pass, 1 local MSYS2-path skip), actionlint, path guard test,
and independent local MSVC/CUDA 13.3 compile probe passed. No full LocalAI
CUDA archive or GPU inference has been produced; no release URL was added to
the embedded manifest and nothing was published. Linux and Windows CUDA
runners, full archive builds/startup smoke, manifest publication, and live GPU
model invocations remain necessary.

A read-only OpenCode MCP audit of the broader CLI subtree (session
`094be105-7729-44ab-af73-2e6b9e0ac5b6`, request
`5755a776-e362-47a4-97eb-8f94afe13de8`) timed out after 360 seconds
without a primary result. The MCP error correctly appeared in structured
content as `factory_session.subagent.timed_out` and reported one in-flight
dispatch with recent reasoning activity. The audit made no observed edit;
the CLI subtree failures still need classification.

Codex MCP Pi bridge edits (sessions `97cd0fac-c947-41ad-b4cb-17be558bfab7`
and `028818f8-f5aa-4c88-8998-e776e06c72fa`) vendored the MIT `pi-acp`
bridge into the `you pi-acp` executable and made the Pi provider launch the
current `you` binary by absolute path. The bridge now waits for Pi's
`agent_settled` event and converts a final typed assistant error into an ACP
RPC failure; a failed attempt followed by a successful retry completes normally.
The bundled script rebuild reproduced the staged bundle. Node tests and
typecheck, focused Go tests, all 24 lint targets, and the packaged docs smoke
passed. A live Pi model invocation through the newly built binary remains to
be checked; these tests establish the bridge behavior but not live Pi inference.

The first built-binary ACP initialize probe exposed a bundle defect: the ESM
bundle's CommonJS `cross-spawn` dependency needed a Node `require` binding.
The build now supplies `createRequire`, and a new test starts the checked-in
bundle over ACP stdio. All three Node tests and typecheck pass; the rebuilt
`you pi-acp` binary returned a valid ACP initialize response headlessly.
Live Pi inference and end-to-end failure classification remain unverified.

Headless live checks with commit `464a786236` then established the boundary.
The configured Pi model (`llama-cpp/qwen-3.8-uncensored`) currently returns
`Connection error` even in direct `pi -p`. Raw Pi RPC emitted final assistant
`message_end.stopReason=error` and `agent_settled`. The rebuilt `you pi-acp`
returned ACP JSON-RPC error `-32603` with `{provider:pi,outcome:error}`. A
fresh `you server mcp` from the same binary returned `you.subagent`
`isError=true` and `factory_session.subagent.provider_unknown_failure`, with
`INVOCATION_RUNTIME_FAILURE` and a failed Work state. This fixes false
success in the new binary, but the final MCP message still loses Pi's
`Connection error` detail and labels it unknown; that diagnostic propagation
remains work. An in-app MCP call during this check returned only Pi retry
progress as `COMPLETED`, consistent with a previously started old connection;
new connections use the updated binary path. The fresh server advertised all
11 MCP tools, including `you.subagent`.
The Codex MCP configuration for new connections points at
`C:\Users\andre\bin\you-464a786236.exe` (SHA-256
`F643CE36D7CFC4D19447289751E9F9A6937EF443179C1868C93E2BBBCB06BA53`).

After the run and CUDA edits, a full `make lint` rerun passed all targets
except `deadcode`: removal of `run.Open` leaves one stale baseline line
(`pkg/transports/cli/run/run.go: unreachable func: Open`). A focused
OpenCode MCP edit (session `a5b073bc-c658-4cb1-8ad2-ba20be331149`)
removed only that stale line; `make deadcode` and diff checks passed. Commit
`fba0b94729` records the CUDA artifact build preparation, still without a
published archive. The final `make lint` rerun passed all 24 targets.

Commit `a6504a902b` records the deadcode baseline and audit. The Windows
binary was rebuilt as `C:\Users\andre\bin\you-a6504a902b.exe` (SHA-256
`782AA47E51B9993BE557C135F451A32B4EDD978BE6EE1546F8438C5C078E10DC`),
and the Codex MCP command for new connections points to it. A fresh headless
stdio initialize and `tools/list` advertised all 11 tools including
`you.subagent`.

A bounded OpenCode MCP WSL managed GPU embedding probe (session
`5a44e598-8e89-4fc9-a0c1-de31c4817a88`) inspected the binary at
`/home/andre/fr035-20260921023755Z-bf8ba28e/you` (version
`v0.0.8-0.20260911144023-9bec7aae0697+dirty`, built Sep 20). The RTX 4090 was
visible and idle under `nvidia-smi` (driver 595.71, CUDA 13.2, 1963 MiB / 24564
MiB used). `you models inspect embed --json` reported `NOT_INSTALLED` /
`FAILED`: the managed cache at
`/home/andre/.agent-factory/models/EMBED/370f27d7550e0def9b39c1f16d3fbaa13aa67728/`
contains `Qwen3-Embedding-0.6B-Q8_0.gguf` (639 MB) while the configured source
expects `Qwen3-Embedding-0.6B-f16.gguf`
(`hf://Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-f16.gguf@370f27d7550e0def9b39c1f16d3fbaa13aa67728`).
The `localai-backends` directory was empty (no backend build metadata). No
embedding invocation was run. Precise next step: `you models pull embed` from
the directory whose factory definition specifies the f16 source, then
re-inspect and re-run the offline EMBED probe if that f16 definition is still
intended. Because this WSL binary/config is older than the current Windows
build and a Q8 asset already exists, a cheaper first check is to rebuild the
current Linux binary and inspect the built-in Q8 definition from a neutral
directory before downloading another quantization. A published CUDA backend
archive is still required for managed GPU inference.

The next headless, concurrent MCP reliability pass ran three `you.subagent`
sessions against the same worktree. A broad OpenCode edit to propagate Pi's
connection diagnostic (session `e9f01676-f6e2-412d-baa1-9af12cf2c777`,
request `4a68ac06-a32f-41df-9692-e4a226a5a8fa`) timed out after 360
seconds with recent reasoning activity and no observed edits. Its local log
shows serial repository searches spread over several minutes; the task was
too broad for this timeout. A follow-up OpenCode task was narrowed to the Pi
bridge files and given a longer bound.

A read-only Codex MCP audit (session `309008c8-475a-4b38-84d5-5abf1bd32434`)
reproduced the CLI subtree failures and identified stale MCP command
expectations and a fixture smoke decoder that parsed a human-readable error
message as JSON. Two simultaneous bounded Codex MCP edits completed:
`dd658878-f3d8-4bcf-8f21-9f55aea78170` fixed the smoke decoder (commit
`87fcab18d3`), and `39db5b42-558c-4165-abf4-b19b151c5e73` reconciled
the command/input baselines and tests (commit `b69d5adbf1`). An independent
`go test ./pkg/transports/cli/... -count=1` passed in full. The agents
preserved unrelated dirty files and returned primary results; overlapping
MCP dispatches completed without a session collision.

A further bounded Codex MCP task (session `45a0878b-8a3a-4759-af0b-c9ea9778b8cd`)
reconciled the session-list default test with the authored CLI default `all`
(commit `effe740ff7`); the full `contracts` package passed. OpenCode then
completed a narrow Pi bridge task (session `d069cd71-1a69-4c4f-8115-2ff5ea8ecc37`)
that marks only Pi's exact `Connection error.` outcome as `model_connection`
in the ACP error data. A second OpenCode follow-up (session
`2272a937-83c1-4266-b8ab-b0cfe875f88b`) removed a stale-marker edge case
across retries. The eight bridge Node tests and typecheck passed. Codex MCP
session `36268364-ca34-4b3d-b487-1a2ebe0d04cc` mapped the marker at the
Go ACP boundary to a safe, actionable misconfiguration diagnostic; focused
provider tests and the full Factory Session MCP package passed. A package
file-count check found that the new test files should be merged into existing
test files before final lint and live verification. The selected Pi model
endpoint itself remains unreachable, so success-path live Pi inference is
still unverified.

Codex MCP follow-ups `a935ec4e-2a4d-49e0-b553-b37f964de80d` and
`f68ac872-09be-4994-b688-c9787063450b` merged the new tests into existing
package files and reduced two complexity findings; file-count and
maintainability checks passed. A freshly compiled headless MCP binary ran
`you.subagent` against the currently unreachable Pi model endpoint and
returned `isError=true` with `factory_session.subagent.provider_misconfigured`
in both MCP text and structured content. It no longer reports success or an
unknown failure. The initial message still buried the endpoint check, so a
bounded Codex MCP copy edit (session `c25e0280-5de7-4c0b-bab6-cf0a840389e3`)
was attempted. That call timed out after 150 seconds with
`providerSessionObserved=false` despite leaving reviewable edits. Independent
focused tests, the full Factory Session MCP package, and maintainability
checks passed on those edits. This is a partial-edit timeout to retain in
the reliability record. Independent `make lint` then passed all 24 targets.
A rebuilt headless MCP binary returned `isError=true`, code
`factory_session.subagent.provider_misconfigured`, and the message
"Pi subagent provider is misconfigured; check that Pi's selected model
endpoint is running and reachable". Its suggested action begins with the
same concrete endpoint check, followed by Pi setup/version and workspace
inspection. The live Pi success path is still unavailable because the
selected local model endpoint reports `Connection error.` even via direct Pi.
Commit `ffce01616e` records the bridge, classification, tests, and this log.
The installed MCP command for new connections now points to
`C:\Users\andre\bin\you-ffce01616e.exe` (SHA-256
`7CC1E5F3094562806353641BFAE116999BC3D53EBFC178A631631109CDE9C658`).
A fresh headless MCP initialize and tools/list from that exact binary returned
11 tools including `you.subagent`.
