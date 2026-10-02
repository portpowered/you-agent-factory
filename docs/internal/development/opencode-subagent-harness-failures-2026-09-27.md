# OpenCode subagent harness failures: September 27, 2026

This note records failures seen while driving `@you/subagent` through the MCP
harness to remove the application opener. Schema/definition failures and
timeout failures are recorded separately because they have different
confirmation status: the definition failures are explained end to end, the
timeout failure is not.

## Confirmed causes

| Observation | Evidence | Conclusion |
| --- | --- | --- |
| `BAD_REQUEST unknown named argument "workingRoot"` | `C:\Users\andre\bin\you.exe` was built at 16:36, before commit `580985d1a3` ("Enable editing subagents and converge session open route") at 16:44. The `@you/subagent` package embedded in that binary carried `agentTools.policy: READ_ONLY` and no `workingRoot`, so the managed definition installed from it also lacked both. | Stale binary. The schema mismatch was an artifact of running pre-16:44 code, not of the packaged source. |
| Six stale `you.exe server mcp` processes | Six MCP server processes were running, all from the 16:36 binary, alongside newer CLI invocations. | Stale server set. Mixed binary generations were serving MCP and CLI at the same time. |
| Managed definition regressed after initialization | Reinitializing the package against the then-current binary briefly restored the newer definition; it later reverted to the old shape again. | The managed install tracks the embedded package of whichever binary initialized it, so it reverts whenever a stale binary re-initializes. |
| OpenCode started a second stale executable | `C:\Users\andre\.config\opencode\opencode.jsonc` pointed its local `you-agent-factory` MCP server at `C:\Users\andre\bin\you-agent-factory-mcp.exe`, dated September 26 at 22:55. The installed definition reverted at 19:26:53 and 19:30:30 during OpenCode child starts, despite the main CLI using a fresh `you.exe`. | This second binary caused the repeat regressions. The nested MCP entry was removed at the user's request; a subsequent child start kept the installed definition `ENABLED` with `workingRoot`. |
| Fresh binary resolves the definition gap | A rebuild at 19:22 followed by a fresh install produced a definition with `agentTools.policy: ENABLED` and a `workingRoot` argument, matching `packages/packaged-factories/generated/factories/subagent/factory.json` (`policy: ENABLED`, `workingDirectory: ${workingRoot}`). | Definition/schema failures are closed once the installed binary and the installed definition are both current. |
| `Transport closed` on MCP calls | The transport dropped when the six stale MCP servers were stopped. | Connection lifecycle event at the time, distinct from worker behavior. A reconnect was required after killing stale servers; a fresh outer MCP call later completed `OK`. |

## Timeout failures: root cause unknown

Separate from the above, several calls ended with
`invocation timed out while waiting for primary result` at exactly their
90, 100, or 110 second limits.

| Observation | Detail |
| --- | --- |
| Timeout durations | 90 s, 100 s, and 110 s. Each hit its configured limit exactly, not a nearby value. |
| Side effects | Some timed-out calls left partial edits in the working tree; some left complete edits; some left none. |
| Comparison point | Read-only audits of comparable size sometimes finished in roughly 67-100 s, i.e. near the same limits without timing out. |
| Root cause | Not established. The exact-limit pattern is consistent with a client-side deadline that does not observe completion, but no trace confirms where the time is spent. |

Later bounded OpenCode MCP calls also hit a 90 s deadline with **zero edits**
visible in the working tree afterward. This confirms that a timeout can have
either partial side effects or none; it does not establish whether the worker
started, where it waited, or whether cancellation reached it.

A later direct MCP documentation edit with free Nemotron returned `TIMED_OUT`
at its requested 60 s `you.subagent.timeoutMillis` limit, after writing two
hunks to `docs/architecture/packaged-structure.md`. The probe client's own
response wait was 75 s, so it did not impose this deadline. The MCP tool passes
`timeoutMillis` into `SessionInvokeRequest.Wait`; Factory Sessions converts it
to the invocation wait context deadline and returns `INVOCATION_TIMED_OUT`.
The MCP tool now reports a distinct `factory_session.subagent.timed_out` error
with `partialEffectsPossible: true` and the configured duration. It cannot
assert whether edits occurred for a particular timed-out call; inspect the
workspace before retrying. The documentation edit was reviewed and committed
after the timeout.

After the binary/config repair, two CLI OpenCode audits exited with code 0 but
no primary result and no requested edit. Their stdout contained only an initial
progress sentence and an ACP `peer connection closed` line. A prior CLI
OpenCode dispatch completed a documentation edit and returned a primary result.
The OpenCode log explains the empty audits: they tried to read paths outside
`workingRoot` and reached `external_directory` permission requests with action
`ask`. The harness did not surface a useful pending-approval outcome before
closing. Subsequent changes classify an empty turn after a denied permission
as failure and grant advertised ACP allow choices by default. A live
outside-workspace read with these changes has not yet completed.

## Fresh verified facts (September 28, 2026)

- At the user's request, OpenCode's nested `you-agent-factory` MCP entry was
  removed: `~/.config/opencode/opencode.jsonc` now contains only its schema
  key.
- The five persistent `you.exe server mcp` processes observed afterward had
  parent `codex.exe`, not OpenCode. One additional process started from
  OpenCode disappeared on child exit.
- A fresh outer MCP `you.subagent` call completed `OK` in about 8 seconds.
  The OpenCode log for that call shows build model
  `muse-spark-1.3-contributor-free`. The installed `@you/subagent` definition
  stayed `ENABLED` with `workingRoot`.
- The earlier CLI fallback used bare `muse-spark-1.3-contributor-free`, but
  the OpenCode log showed the actual build model was `big-pickle`. The ACP
  adapter now rejects an explicitly requested model that OpenCode does not
  advertise, and reports a denied permission as a failed turn.
- The execution model catalog now accepts qualified identities such as
  `opencode/muse-spark-1.3-contributor-free`. A fresh CLI dispatch with that
  identity completed `OK` in about 9 seconds, and the OpenCode log confirmed
  the requested Muse Spark build model.
- The ACP client now selects `allow_always` when offered, falling back to
  `allow_once`, without requiring `--skip-permissions`. Reject-only options
  remain rejected, and unknown options are cancelled. Focused provider tests
  cover these outcomes. OpenCode V2 uses ordered `permissions` rules; the
  earlier `OPENCODE_PERMISSION` launch injection used a V1-era setting and was
  removed. The local global `opencode.jsonc` now has a V2 allow-all rule, with
  no nested MCP entry; `opencode debug config` confirms that file is loaded.
  `opencode acp` does not offer `--auto`.
- A fresh CLI binary launched an outside-workspace read using the requested
  Muse Spark model, but OpenCode reported `Rate limit exceeded` before any
  file tool or permission request. A shorter retry also hit the same limit.
  A concurrent `big-pickle` run in the OpenCode log was rate limited too. The
  live read and the effective OpenCode V2 permission behavior therefore remain
  **unverified**; focused tests verify ACP choice handling.

A timeout is therefore not evidence that no edit occurred. Always inspect the
tree before retrying, and never re-issue the same request blind.

## Unconfirmed hypotheses

- Cancellation may not reach the OpenCode child process, leaving the invocation
  running after the caller gives up. Unverified.
- The MCP timeout may cover provider startup, provider execution, and result
  publication as one budget, so a slow provider start consumes the whole limit.
  Unverified.
- The 67-100 s successful audits suggest provider latency near the limit is a
  contributing factor rather than a hard defect. Unverified.
- Do not attribute the timeouts to `workingRoot`, editing policy, or the stale
  binary. Those are confirmed causes of the schema failures only.

## Operational checks

1. **Compare the installed definition with the packaged source.** Diff
   `~/.you-agent-factory/factories/@you/subagent/factory.json` against
   `packages/packaged-factories/generated/factories/subagent/factory.json` and
   check `workingRoot`, `workingDirectory`, and `agentTools.policy`. A
   `READ_ONLY` policy or a missing `workingRoot` means the installed binary is
   stale, regardless of the source tree.
2. **Compare the binary timestamp with the relevant commit.** Check the
   `you.exe` build time against the commit that last changed the packaged
   factory. A binary older than that commit cannot expose the newer schema.
3. **Inventory stale processes before testing.** List path, command line, and
   creation time for every `you.exe server mcp` process. Stop the stale set,
   expect the transport to close, and reconnect before drawing conclusions.
4. **Inspect the diff after every timeout.** Run `git diff` and `git status`
   before retrying, scope the next attempt to one bounded edit, and verify that
   edit independently.
5. **Inspect OpenCode's own MCP configuration.** OpenCode previously started a
   second `you` executable through its nested MCP entry; that entry has since
   been removed at the user's request. If a definition regresses, compare all
   live MCP server executable paths and parent processes.
6. **Treat a closed transport as a connection problem.** Confirm it separately
   by invoking the same packaged factory through the CLI, provided the binary
   and the invocation catalog are current.
7. **Check OpenCode permission logs after an empty result.** Look for
   `permission=external_directory` and `action=ask` in
   `~/.local/share/opencode/log/opencode.log`. This condition previously
   produced a success result with no primary output; the ACP adapter now
   classifies a denied permission as failure and grants advertised allow
   choices. Check for provider rate-limit errors before attributing an empty
   result to permission handling.

## Adjacent CLI invocation findings

- Initially, invoking `opencode/muse-spark-1.3-contributor-free` as one value
  failed with `reference identity is invalid`; the execution catalog's
  reference pattern rejected `/`. The catalog now accepts a qualified model
  identity, and a dispatch confirmed the selected Muse Spark model in the log.
- `opencode` was then rejected with `runner is not a supported built-in
  identity`; `knownExecutionRunner` listed only `codex`, `claude`, and
  `antigravity`. `opencode` was added to the runner catalog in
  `pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution/catalog.go`.
- A later bounded OpenCode edit request failed in about 0.55 s before provider
  launch with `execution catalog resolution failed: reference identity is
  invalid` for the qualified Muse Spark model. A fresh CLI build from the
  subsequent shared tree accepted the same `opencode/muse-spark-1.3-contributor-free`
  identity and reached provider launch. The earlier catalog failure was not
  reproduced; its exact cause remains unconfirmed.
- That fresh run exposed a separate launch mismatch: the embedded provider
  catalog selected `npx -y opencode-ai acp`. It launched OpenCode V1.18.32,
  which rejected the local V2 `permissions` configuration. The installed
  `opencode` command is V2.0.16 and `opencode debug config` loads that
  configuration. The authored harness and generated catalog were subsequently
  changed to launch installed `opencode acp`. A fresh CLI probe at
  2026-09-28T03:43:31Z then logged `version=2.0.16` with `args=["acp"]`,
  passed initialization, and reached `session/prompt`; OpenCode returned
  `Rate limit exceeded` in 5.5 s before any tool call. A live V2 permission
  read remains unverified.
- A later freshly rebuilt CLI edit probe selected another advertised free
  model, `opencode/ling-3.0-flash-fin-free`. OpenCode again returned
  `Rate limit exceeded` at `session/prompt`, this time in 3.4 s, and the
  requested probe file was not created. This did not exercise tool editing,
  ACP permission selection, or form elicitation.
- The ACP client now advertises form elicitation and automatically answers
  schema-backed boolean approvals and choice fields, preferring affirmative
  choices or a declared default. It cancels forms that require free text and
  does not advertise URL elicitation. Focused provider tests cover these
  decisions; no live OpenCode form request was observed.

## Direct MCP stdio probe (September 28, 2026)

- A freshly installed `you` binary launched directly as an MCP stdio server,
  initialized successfully, and listed 11 tools, including `you.subagent`.
  This confirms tool discovery and the direct MCP transport worked for that
  process.
- A bounded OpenCode edit request through `you.subagent` returned a generic
  `BAD_REQUEST` saying work reached a failed state before a primary result.
  The same server's stderr recorded `ACP provider "opencode" session/prompt
  failed: Internal error: Rate limit exceeded`. The provider rate limit is
  confirmed; editing and permission behavior were not exercised by this call.
- The generic MCP error came from invocation primary-result failure
  classification, which omitted the normalized Work failure reason. The MCP
  tool now maps a normalized `throttled` reason to a retryable provider-limit
  error with a fixed message. A first rebuilt live rerun still returned
  `factory_session.subagent.execution_failed`: the ACP provider error was
  classified as `unknown` upstream, despite the OpenCode rate-limit text in
  server stderr. The ACP source now classifies rate-limit RPC errors as
  `throttled` and uses a fixed provider message. A second rerun recorded
  `failure_reason: throttled` on the dispatch but still returned the generic
  MCP error because primary Work was unresolved. The invocation wait now
  carries the sole recorded dispatch failure reason in that case.
- A final freshly rebuilt direct stdio MCP probe with
  `opencode/mimo-v2.6-flash-free` initialized, listed all 11 tools, and
  returned `factory_session.subagent.provider_throttled` from `you.subagent`
  with `retryable: true`, `failureReason: throttled`, and a fixed safe message.
  The dispatch log independently recorded `failure_reason: throttled`. The
  requested file was not edited because the provider rate limit occurred
  before a tool call.
- A subsequent direct stdio MCP edit probe used the advertised free model
  `opencode/nemotron-3.5-lightning-free` with a 60 s invocation limit. It
  initialized, listed 11 tools, and returned a completed `you.subagent`
  result in about 21 s. The requested workspace file
  `.tmp/opencode-subagent-free-model-probe.txt` exists and contains exactly
  `OpenCode MCP edit probe completed`. This verifies editing through the
  rebuilt binary and MCP harness on that model; it does not establish that
  every free model currently has capacity.
- The app's existing `you-agent-factory` MCP connector still returned
  `Transport closed` on a read-only session-list call after this direct
  success. No running server process was stopped. The direct probe used its
  own child stdio process, so app connector reconnection remains unverified.
- A bounded real code rename with the same Nemotron model and a 120 s
  `timeoutMillis` returned `factory_session.subagent.timed_out` with
  `partialEffectsPossible: true`. The only touched file was the requested
  `pkg/services/factory_runtime/internal/build_test.go`, but OpenCode left a
  missing closing brace and mixed old/new identifiers, so the package did not
  compile at that point. The direct MCP server child exited after the
  response; process inventory showed no newly running OpenCode ACP child.
  The rename was completed locally, including two dependent test callsites,
  and the focused Factory Runtime internal tests passed. This confirms that
  a timeout can leave syntactically broken partial code even when the requested
  operation is a mechanical rename.
- The app's MCP connection reported `Transport closed` after its stale server
  was restarted. This is a connection lifecycle observation, not evidence of
  the provider outcome. The direct stdio probe above initialized separately.

## Limitations of this note

### 2026-10-01 concurrent MCP reports outside the workspace

Two concurrent outer MCP dispatches used provider `opencode`, model
`opencode/mimo-v2.6-flash-free`, `timeoutMillis: 120000`, and the managed
worktree as `workingRoot`. Both returned `COMPLETED` with primary results;
the combined elapsed time was 44.5 seconds. No permission or edit-policy
override was supplied.

- Grammar audit session: `8ad25445-a152-42a3-9744-900b861f8d4d`.
  Requested report: `C:/t/dub-multilingual-validation/opencode-concurrency/grammar-audit.txt`.
- Resource audit session: `cea7278e-a97f-4ac8-8cfc-e5f54fda3e82`.
  Requested report: `C:/t/dub-multilingual-validation/opencode-concurrency/resources-audit.txt`.

Independent reads verified both distinct reports existed outside
`workingRoot`. The grammar report described the authored translation and
audit GBNF shapes and `PredictOptions.Grammar` field 29. The resource report
identified GPU capacity 1, the ASR/translation/TTS guards, and unguarded
rendering. Neither report claimed native inference was exercised.

This proves concurrent MCP dispatch, primary-result delivery, and these
requested writes outside the workspace. No permission elicitation trace was
captured, so it does not prove how a permission request was granted or resolve
the causes of earlier timeouts.

### 2026-10-01 dubbing Factory exercise

- A read-only OpenCode research dispatch completed through MCP, session
  `ccff2f32-daca-4e30-b4c6-c6f2a0304337`. Its response was excessively long
  and included incorrect claims that ASR had no implementation. Local source
  inspection and passing ASR tests contradicted that claim; the response was
  not used as implementation evidence.
- A bounded two-file Python editing dispatch using
  `opencode/mimo-v2.6-flash-free` timed out after 900,000 ms, session
  `03ceae93-5157-4e34-91df-a1f32250f756`, request
  `ed926b9c-b929-4287-b58d-7cd848e7d550`. Neither requested file existed
  afterward. The returned diagnostic correctly warned about possible partial
  effects and reported that cleanup closed the live Factory Session. It
  recorded reasoning activity at `2026-10-01T13:18:31.6200605Z`; this is
  activity evidence, not proof of useful editing progress or a timeout cause.
- A smaller single-file retry using `opencode/ling-3.0-flash-fin-free`
  returned `factory_session.subagent.provider_unknown_failure` in about four
  seconds, session `2bd7f3c8-5a5f-4f9b-9ca0-7ae402144c10`. The returned
  `failureReason` was `unknown`; no more specific provider cause was supplied.
  The requested file still did not exist. Implementation continued with an
  available collaborating agent after these two failed editing attempts.
- The MCP Factory Session list tool returned “factory session runtime is not
  available: live session service is required.” The subagent timeout response
  explicitly says its session ID is for log correlation and may no longer be
  available for inspection. Durable inspection remains an operational gap for
  these standalone subagent dispatches.

- Re-reading the installed definition after the 19:22 build showed
  `policy: READ_ONLY` with no `workingRoot` again because OpenCode's separate
  MCP executable was stale. The nested entry has since been removed; a fresh
  outer MCP dispatch and CLI dispatch kept `ENABLED` and `workingRoot`.
- No trace, transcript, or worker log was captured for the timeouts, so the
  timeout section stays at hypothesis level.
- The default-grant policy has focused tests, but the live outside-workspace
  read was blocked by model rate limits before it could exercise permission
  handling.
- This note documents a single-operator desktop environment. Results may differ
  on other hosts.

## October 1 multilingual follow-up

A bounded read-only OpenCode audit (`opencode/mimo-v2.6-flash-free`, 60-second
budget) inspected only the dubbing scripts inside the managed workspace. It
returned `factory_session.subagent.timed_out` without a primary result. Session
`6cc5268e-09ea-44ed-89ee-7e836ffab278`, request
`088c09d0-757c-4aa0-a2c9-66e6b3f1c38c`. The structured outcome recorded one
in-flight dispatch and a provider reasoning delta, and reported that cleanup
closed the live Factory Session. This bounded outcome does not establish the
cause of previous long timeouts. Implementation continued with the existing
collaboration agents rather than repeating the same probe.

## October 1 successful GPU-resource editing audit

A fresh outer MCP `you_subagent` dispatch using
`opencode/mimo-v2.6-flash-free` completed in 21.1 seconds with a primary result,
session `5a50809d-985f-4a58-94d0-41c547fa46bf`. Its 120-second budget was not
exhausted. The managed worktree was supplied as `workingRoot`; no edit-policy
or approval override was passed. It read only the dubbing Factory YAML and
created `docs/internal/development/opencode-gpu-resource-audit-2026-10-01.md`.
Independent inspection confirmed the sole requested file was created and its
claims match the source: one GPU capacity unit guards ASR, translation, and
TTS; rendering does not request it. No Models inference or browser operation
was requested. This demonstrates a successful bounded editing dispatch and
useful primary-result delivery through the MCP infrastructure. It does not
establish the cause of the previous timeout, approval, or connection failures,
or prove general outside-workspace permission handling.

## October 2 two-stage audit review timeout

A subsequent OpenCode MCP review using `opencode/mimo-v2.6-flash-free`
and `timeoutMillis: 120000` returned
`factory_session.subagent.timed_out` without a primary result. Session:
`b3437d1a-ec69-4236-8fef-31fad5cf14d5`; request:
`c79d0302-2370-4c37-8c16-e24703585a7f`. The structured result recorded a
provider reasoning activity delta at `2026-10-02T07:00:20.1545846Z`, reported
that cleanup closed the Factory Session, and warned that partial effects
were possible.

The requested external report,
`C:/t/dub-multilingual-validation/opencode-concurrency/two-stage-audit-review.txt`,
was absent when checked after the response. The activity delta does not prove
useful review progress or explain the timeout. The earlier two concurrent
dispatches remain verified successes; this later failure does not invalidate
their results or establish a common cause.

## OpenCode bounded review retry completed (2026-10-02)

After the 120,000 ms review timeout recorded above, a bounded retry used
`opencode/mimo-v2.6-flash-free` with a 300,000 ms allowance and a requested short
report. Session `54e2230b-b1a4-4d34-8f1a-5e3a2e97af7f` returned `COMPLETED` and a
primary result. The requested external report exists at
`C:/t/dub-multilingual-validation/opencode-concurrency/two-stage-audit-review-retry.txt`.
Root read it and corroborated its findings: comparison has no grammar/output
cap, the indexed final decision treats comparison as untrusted evidence, and
model failures/cancellation propagate. No source edits or local model calls
were requested. This proves this retry completed and wrote the report with
ordinary defaults; it does not prove the original timeout's cause or that a
longer allowance alone explains the different result.

## October 2 native sampler review timeout

A bounded two-file native sampler/recipe review using
`opencode/mimo-v2.6-flash-free` and `timeoutMillis: 300000` returned
`factory_session.subagent.timed_out`. Session
`9370501b-5db6-4a64-9e06-4b50922bc4a8`, request
`6709d5c7-062c-413f-a8ce-4cd44cbb7fd2`. Human-readable MCP text and structured
error both reported the timeout; cleanup closed the live Factory Session and
partial effects were possible. The last recorded provider reasoning delta was
`2026-10-02T08:35:41.0208094Z`. The requested external report
`C:/t/dub-multilingual-validation/opencode-concurrency/native-sampler-review.txt`
was absent afterward. No model inference/build/source edits were requested.
Independent collaboration review and CPU sampler regression passed, but this
dispatch produced no review evidence. The reason for its timeout is unproven;
a five-minute allowance alone does not establish harness stability.
