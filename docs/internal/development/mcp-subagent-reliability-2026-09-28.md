# MCP subagent reliability log — 2026-09-28

This log records observed `you.subagent` behavior during the LocalAI and MCP
cleanup work. Outcomes are verified against the workspace, not inferred from
the agent's text response.

## Environment

- Checkout: `codex/mcp-subagent` at `a16ff10223` when probing began.
- MCP connector: `you.factory_session.list` responded successfully.
- Installed server: `C:\Users\andre\bin\you.exe` (built from commit
  `cc4492754b`). The integrated branch has since rebuilt and installed the
  binary from this commit; source and installed binary are in sync.
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

The two permission probes above verify the installed permission path on this
host for the specific paths exercised; they do not establish that every
external path is writable or readable.

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
returned you-agent-factory.
