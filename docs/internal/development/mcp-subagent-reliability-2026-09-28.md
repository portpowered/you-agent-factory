# MCP subagent reliability log — 2026-09-28

This log records observed `you.subagent` behavior during the LocalAI and MCP
cleanup work. Outcomes are verified against the workspace, not inferred from
the agent's text response.

## Environment

- Checkout: `codex/mcp-subagent` at `a16ff10223` when probing began.
- MCP connector: `you.factory_session.list` responded successfully.
- Installed server: `C:\Users\andre\bin\you.exe` (built from the separate
  `opencode/application-opener-convergence` branch). The installed binary and
  this checkout's source are currently out of sync; validation of a source fix
  requires a fresh build and direct MCP probe before installation.
- Provider/model for the probes below: `opencode` /
  `opencode/nemotron-3.5-lightning-free`.

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
