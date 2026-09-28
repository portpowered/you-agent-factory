# MCP subagent reliability log — 2026-09-28

This log records observed `you.subagent` behavior during the LocalAI and MCP
cleanup work. Outcomes are verified against the workspace, not inferred from
the agent's text response.

## Environment

- Checkout: `codex/mcp-subagent` at `a16ff10223` when probing began.
- MCP connector: `you.factory_session.list` responded successfully.
- Installed server: `C:\Users\andre\bin\you.exe` (built from source commit
  `4bd24f1142` at the latest probe). Existing MCP connector processes may
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
