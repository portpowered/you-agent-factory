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

## October 2 concurrent ten-minute dispatch timeouts

Two concurrent OpenCode MCP dispatches using `opencode/mimo-v2.6-flash-free`
each received a `timeoutMillis: 600000` allowance. One requested a CPU-only
code/startup audit report; the other requested bounded dubbing prompt edits.
Both timed out. Root checked afterward: the requested audit report was absent,
and the requested dubbing prompt edits had no Git diff. The human-readable MCP
responses reported that cleanup closed the sessions and that partial effects
were possible. These observations do not identify the timeout cause or prove
that no other partial effects occurred.

The root wrapper did not retain the structured request/session IDs, so no IDs
are assigned to these dispatches here. A live-inspection identifier beginning
`07f22431` disappeared after cleanup; it does not establish the identity of the
first dispatch. Collaboration-agent fallback implemented the prompt edits for
independent review. No local Models inference was requested in either task.

## October 2 bounded startup audit retry completed

A narrower read-only OpenCode MCP retry using `opencode/big-pickle`, session
`7a3300f5-1a6d-4183-990d-07a85b61a713`, completed within its 300,000 ms allowance
and returned a primary result. It reviewed two files inside `workingRoot`,
without requesting report files, edits, or local Models inference. Root
corroborated the reported full-file hash calls in `findGenericArtifact`
(`generic_cache.go`) and `verifyGenericCachedArtifact` (`generic_source.go`)
before known-size rejection, plus conditional additional hashing during legacy
repair. This identifies source-level opportunities for repeated reads; it does
not prove how many passes one Models invocation performs or explain the
observed 143 GB read volume.

This demonstrates useful primary-result delivery for this bounded review. The
model, prompt scope, and requested effects differed from the two ten-minute
timeouts, so the successful retry does not identify their cause or demonstrate
editing stability.

## October 2 bounded dubbing context design audit completed

OpenCode MCP with `opencode/big-pickle`, session
`3451c58a-67dd-45fe-93ff-e18807887569`, completed a two-file read-only audit
inside the workspace within its 300,000 ms allowance and returned a primary
result. It identified the ASR segment projection and proposed a separate frame
evidence artifact. Independent review confirmed that source evidence must also
reach both audit passes and fit repair, not only initial translation.

The result is design input, not accepted implementation: its proposed total
evidence caps conflict with the large-input requirement; its suggested frame
input slot was unverified; supplied ASS currently supplements rendered media
and does not correct ASR. A conflict-only gate cannot recover the disputed
source meaning. No edits or GPU inference were requested. This is another
bounded primary-result success, with no new editing or concurrency guarantee.

A subsequent bounded editing dispatch with the same model, session
`31e8d34b-3ca2-4fe3-b06a-1249e624df62`, completed and appended the supplied
Qwen ASR size-comparison facts to the plan. Root inspected the actual diff,
checked it against the saved comparison, and corrected one phrase from
identical requests to identical recognition results. No external reads or GPU
calls were requested. This establishes one completed documentation edit and
primary result; concurrent editing stability remains unproven. Both exact MCP
results were retained externally for independent inspection.

## October 2 engine review timeout

The OpenCode MCP engine review using `opencode/big-pickle` and a 300,000 ms
allowance returned `factory_session.subagent.timed_out`, with no primary
result. Session `4d14464f-f822-449c-8a0a-38ba485bdae6`, request
`dc8610e3-8079-4073-a3d5-11cf16fc6739`. Human-readable text and structured
error both reported cleanup closing the live Factory Session and possible
partial effects. The last retained provider observation was a reasoning delta
at `2026-10-02T11:48:59.5075866Z`; one dispatch remained in flight.

Root inspected Git status afterward and found only expected files. This does
not prove that no partial effects occurred. The cause remains unknown, and the
dispatch supplies no independent engine review. The exact MCP response is
saved at `C:/t/dub-multilingual-validation/opencode-engine-review-timeout-result.json`.

A read-only retry with `opencode/big-pickle`, session
`881987fb-f1cf-48f6-a3fe-19ba325490dd`, completed within a 1,200,000 ms
allowance and returned a primary review. The exact response is saved at
`C:/t/dub-multilingual-validation/opencode-engine-review-completed-result.json`.
It found no blocking correctness defect in publishing the complete reserved
dispatch batch before external execution. It raised the fatal submission-error
path as nonblocking and suggested deleting unsubmitted tail entries.

Independent source review confirmed that all batch resource mutations already
precede submission, and a submission error already aborts the runtime loop.
Root rejected deleting entries alone: it would discard held-mutation and
recovery lineage without restoring consumed resources or resolving recorded
dispatch events. No partial rollback was implemented. This review does not
show actual execution overlap. It proves one completed read-only retry;
timeout allowance and prompt scope both changed, so timeout causation and
general harness stability remain unproven.

## October 2 engine regression complexity edit

The first bounded editing request used provider `opencode`, unqualified model
`big-pickle`, and a 1,200,000 ms allowance. It returned
`factory_session.subagent.provider_request_rejected`, with
`failureReason: permanent_bad_request`, session
`c22a773a-18d3-4a8f-9ad0-e58d6fc37214`, and cleanup closing the live session.
The requested test file had no diff afterward. The unqualified model identifier
may have affected the request; the saved error does not establish that cause.
Exact result: `C:/t/dub-multilingual-validation/opencode-engine-complexity-bad-request-result.json`.

After that terminal result, the next editing dispatch requested
`opencode/space-bunny-free` with the same allowance. Session
`71f62be8-f48d-4b3b-9f2d-275c4fad1730` returned `COMPLETED` and a primary
result. The actual diff extracted the held-resource reservation assertion into
a private test helper in `engine_runtime_snapshot_test.go`, preserving every
assertion. Independent review confirmed the change; final repository
maintainability and engine race checks passed. No production/native files,
baselines, model defaults, or GPU inference changed in this task. Exact result:
`C:/t/dub-multilingual-validation/opencode-engine-complexity-space-bunny-result.json`.
This establishes one successful bounded edit with the requested model. Both
model and identifier form changed, so it does not identify the first rejection
cause or prove general concurrent editing stability.

## October 2 native independent-image implementation dispatches

The implementation request using `opencode/mimo-v2.6-flash-free` timed out
with a 1,200,000 ms allowance, without a primary result or the requested edits
in the inspected scope. Session `57888254-fb89-45af-b963-e2c3d3dd673c`, request
`ae266748-da6d-42df-8143-b00019494cec`. Its retained provider activity had
`kind: ERROR`, `phase: FAILED`, and `providerSessionObserved: true` at
`2026-10-02T12:02:42.3171664Z`, but omitted the underlying error payload.
Cleanup closed the live session; the terminal response reported possible
partial effects. The cause remains unknown. A later session lookup returning
404 does not prove a lookup bug: the response explicitly documents its session
ID as correlation-only and warns that lookup may return `session.not_found`.

A BigPickle retry completed with a primary result, session
`7b2f5175-1a52-48e0-8ee0-6a4d3ea65393`, and actual native packing source edits
plus CPU evidence. Independent review corrected the author's inaccurate
per-text-part trim assumption, comments, test/recipe execution, and incomplete
provenance. The actual template trims the whole rendered content; interior
whitespace remains intact. Nonwhitespace ordinal media boundaries were retained.
The reviewed source was committed as `6d1aede588`; native compilation is active,
with no public inference or correspondence acceptance from these CPU proofs.
This is author delivery followed by independent repair and review, not proof
that the initial implementation needed no correction or that timeout causes
are understood.

Exact terminal results and the reviewed handoff are preserved under
`C:/t/dub-llama-independent-images-source-proof/` in
`opencode-dispatch-terminal.json`, `opencode-bigpickle-terminal.json`, and
`handoff.md`. Workspace observations cover the inspected owned scope; they do
not rule out all other partial effects.

A subsequent read-only native review requested `opencode/space-bunny-free`
with a 1,200,000 ms allowance. Session
`1b0daa67-8099-48c2-aaaa-c6fbdca0a625` completed with a primary result. It
traced the pinned LocalAI/llama.cpp rendering and media-tokenization chain,
found no blocking Windows defect, and ran the applicator helper's five tests
successfully. Its observation of pre-existing Git modifications does not alone
prove absence of other effects; root independently checked that workspace
changes were limited to expected notes. The result is saved as
`C:/t/dub-llama-independent-images-source-proof/opencode-space-bunny-native-review.json`.
This establishes one completed bounded review, without native inference
acceptance or a claim that non-Windows recipes received the same repair.

A separate read-only `opencode/space-bunny-free` dispatch with a 1,200,000 ms
allowance completed as session `2095f3d8-747e-44d8-97e8-d0dbdf9d2c60`. Its
primary result corroborated that Worker Session terminal publication precedes
canonical Factory result acceptance and projection publication. Independent
review accepts that ordering, while retaining uncertainty about the precise CI
interleaving and requiring a public Factory completion barrier before asserting
the Work projection. This is a completed source audit, not a repaired test or
proof of the cleanup failure's cause. Exact result:
`C:/t/dub-multilingual-validation/opencode-submit-observer-space-bunny-result.json`.

The follow-up bounded editing task on `opencode/space-bunny-free` completed as
session `659e972f-1558-4d52-82a9-66c0670f972c`, with a primary blocker report
and no final test diff. It verified that the public open mapping selects SERVICE
mode, which deliberately does not terminate when Work finishes. A natural
`RUN_RESPONSE` therefore cannot provide this test's requested completion
barrier. Independent source review confirmed that contract. The author removed
its temporary probe; the owned submit package had no remaining diff. This is
useful completed analysis, not a delivered test repair. Exact result:
`C:/t/dub-multilingual-validation/opencode-submit-barrier-space-bunny-result.json`.

### Rebuilt stdio server and actionable failure classification

Space Bunny completed the timeout-advisory implementation as session
`5f884551-b1ab-4cef-af98-f21f2a08f6a4`. Commit `ca27b505d5` now reports a last
observed provider ERROR/FAILED even when a provider-session reference was
observed. It preserves timeout classification, cleanup and partial-effects
metadata, and excludes raw provider payloads. Root independently reviewed the
diff and ran the owning MCP package race suite successfully in 18.264 seconds.
This corrects diagnostic evidence; it does not establish the cause of the
earlier Mimo stall. Exact author result:
`C:/t/dub-multilingual-validation/opencode-mcp-diagnostic-space-bunny-edit.json`.

The rebuilt executable was installed as `C:/Users/andre/bin/you.exe`, SHA-256
`6e91728f298ea7115ee48ec70a49d1ae0ea02325c1b25150935b33f7aa011a85`.
Its committed Go production inputs include `ca27b505d5`; unrelated live native
draft/test artifacts were present in the worktree during compilation, so this
is not a clean-whole-tree build claim. A fresh `you server mcp` child performed
initialize, tools/list and an actual default-editing call on
`opencode/space-bunny-free`. Session
`1fe8d108-4798-492c-bd33-e4d7a429cb43` returned COMPLETED with a primary result;
root confirmed the requested file bytes and child exit 0 after closing stdin.
The whole fresh-server probe took 68.093 seconds. Exact protocol and executable
identity: `C:/t/dub-multilingual-validation/repaired-mcp-proof/result.json`.

A second fresh-server probe deliberately supplied an unregistered provider.
It returned human-readable text, `isError=true`, and matching structured error
metadata, and exited 0 in 1.328 seconds. However, it classified that known bad
input as `factory_session.subagent.provider_unknown_failure` with message
"provider failed for an unknown reason". That ambiguity is an observed remaining
defect; provider validation/classification repair is delegated. Exact result:
`C:/t/dub-multilingual-validation/repaired-mcp-error-proof/result.json`.

Space Bunny subsequently delivered the submit projection barrier as session
`4e6b840f-f650-4e8f-bcd8-9d857802f782`. It uses the existing public status
terminality observer before reading Work and preserves every failed/done
assertion. Independent full submit race suite, three runs, passed in 41.633
seconds; maintainability and diff checks passed. This addresses the source
ordering risk, not a proven cause of the earlier CI failure. The previously
pushed `0a83cd7816` already passed required CI, run `37056730593`; newer local
changes still require their own CI. Exact author result:
`C:/t/dub-multilingual-validation/opencode-submit-status-barrier-space-bunny-result.json`.

The separate native final-answer grammar editing call reached its 1200000 ms
timeout without a primary result but left patch/test drafts. Those partial
edits were inspected, the fixture renamed `.cpp.in`, and an actual MSVC syntax
check passed. They have not yet passed linked sampler execution or live
thinking-enabled inference and are not accepted as production-ready. Thinking
remains enabled; reasoning is never promoted into the public structured answer.

The first linked execution of that native draft subsequently failed: the
Qwen2 vocab-only fixture did not enter COUNTING from its generation prefix,
and the eager grammar consumed the reasoning decoy. The original failure is
preserved at `C:/t/dub-llamacpp-final-grammar-cpu-review/native-test-result.json`.
Context-sensitive tokenization of ordinary thinking delimiters is under
investigation. A syntax pass therefore did not establish correct behavior;
the draft remains unaccepted and separate from the independently verified
Windows CUDA image-boundary build.

### Subsequent structured-output and ACP evidence

A fresh stdio Space Bunny editing call completed as session
`bc7c7ebe-5271-4733-b0e6-8b6281a5b24a` in 549.594 seconds. It added an actual
protobuf decoder regression with mixed private reasoning and split final JSON
content. The focused tests passed, but the first draft failed maintainability
(complexity 21 versus limit 15). The author reported a temporary production
decoder mutation and revert despite an only-test-file prompt; root confirmed
the production diff was empty. This is a scoped-writing lapse, even though no
production change remained. Exact result:
`C:/t/dub-multilingual-validation/repaired-mcp-privacy-edit-proof/result.json`.

A bounded follow-up completed as session
`5f6745b1-457b-4abd-bbda-9ff8bd58efda` in 124.906 seconds and removed redundant
assertions. Root independently ran the decoder tests and full maintainability
gate successfully, then committed the final regression as `0d195960e5`.
It proves that private reasoning, including a misleading JSON draft, does not
enter final text. It does not prove live native structured generation. Thinking
remains enabled. Exact result:
`C:/t/dub-multilingual-validation/repaired-mcp-privacy-refine-proof/result.json`.

The next pushed head, `e4dd9b69c4`, failed CI run `37059462705`. Unit verification
failed the ACP version-classification case with a peer-disconnected error;
integration verification failed to recognize startup cancellation. The captured
successor actually exited 130 and emitted a canonical Zap console record whose
context says `operation=run.service` and `outcome=cancelled`. The test previously
accepted whole-line JSON or a plain cancellation line, so it missed that console
representation. The exact captured record is preserved in
`C:/t/dub-ci-e4dd-restart-diagnostics/cancellation-scenario.json`.

Space Bunny delivered a narrowly scoped cancellation parser/test edit as session
`7714ee5a-8daa-4087-abd2-f0d3447f8c21` in 229.844 seconds. Root independently
reviewed the parser and ran all `TestBoardPersistenceReports` tests successfully.
The compiled cancellation scenario has not yet been rerun with a current
prebuilt artifact. A bounded Big Pickle refinement is pending to reduce the
test function below the repository's 80-line review limit without losing cases.
Exact author result:
`C:/t/dub-multilingual-validation/repaired-mcp-restart-log-edit-proof/result.json`.

Controlled in-memory experiments independently reproduced two ACP SDK v0.13.5
ordering failures. With a buffered Initialize response and EOF both ready,
16 draws produced six valid responses and ten peer-disconnected failures.
With a held pre-response notification callback, a buffered final Prompt response,
and EOF, 16 draws produced zero successes: ten failed waiting for notifications
and six failed before reading the response. These are deterministic readiness
arrangements with nondeterministic select outcomes, not timeout-cause guesses.
Exact harnesses and logs:
`C:/t/dub-acp-version-ordering-proof/sdk-order/`.
The reviewed upstream release and main still point to the affected version.
Repair of the canonical SDK dependency and parent-owned subprocess pipes is in
progress; fixture keepalive alone cannot establish that the customer race is fixed.

Two concurrent 20-minute Space Bunny schema-authoring calls timed out without
primary results. Native session `aaf311fc...` left useful partial patch/fixture
bytes, while Go session `c461eef0...` left no owned Go changes. Both reported
closed-session cleanup, with partial effects possible. The native partial was
archived before review; it required correction to reject explicitly empty
schemas rather than silently permit unconstrained output. The initial ACP
service-author call also reached its 20-minute bound without a primary result.
The common duration does not establish a shared timeout cause. Narrow repairs
continue using reviewed partials and bounded follow-up MCP tasks.

Big Pickle completed the cancellation-test refinement as session
`1689fee3-8c8b-46a2-b5fb-2aa2e60db565` through a fresh stdio server in 186.563
seconds, with a primary result and child exit 0. Root independently reviewed
the final diff and reran all fourteen console cases plus the existing helpers
successfully (0.037 seconds). Commit `1378e88d48` retains the exact captured
189-byte record, typed-field classification, and negative cases. This remains
a parser-level proof; the compiled restart scenario still needs its current
artifact run. Exact result:
`C:/t/dub-multilingual-validation/repaired-mcp-restart-refine-proof/result.json`.

The provider-validation edit first timed out after twenty minutes with partial
edits and no primary result. A completed Big Pickle review found a real privacy
defect in the non-not-found catalog error fallback. The subsequent ten-minute
Big Pickle edit also timed out with partial changes and no primary result.
Independent review finished and narrowed those changes, preserving previous
failure assertions. Commit `8d0225132e` rejects unknown provider names before
session creation, resolves configured aliases through the authoritative
Providers catalog, keeps defaults, bounds catalog lookup by the invocation
deadline, and uses fixed safe catalog-unavailable errors. Root's independent
race run passed MCP (18.307 seconds), CLI MCP (1.299 seconds), and Wire (29.512
seconds). The author also passed full maintainability and file-count gates.
Fresh rebuilt-binary verification remains pending. Exact timeout evidence:
`C:/t/dub-multilingual-validation/opencode-provider-validation-space-bunny-timeout-result.json`
and `C:/t/dub-multilingual-validation/opencode-provider-catalog-privacy-bigpickle-timeout-result.json`.

Root then built committed `8d0225132e` in an isolated clean source copy, excluding
all concurrent ACP/native/schema drafts. A fresh server rejected the same bad
provider in 1.500 seconds with `provider_not_found`, readable content text,
matching structured error, and no session identity. A valid Big Pickle editing
probe failed in 15.641 seconds with `provider_throttled`, retryable true and
confirmed cleanup. Its existing suggested action still omitted the direct
recovery of waiting/retrying or selecting another available model; that
guidance improvement is delegated. Exact results:
`C:/t/dub-multilingual-validation/repaired-mcp-provider-validation-proof/result.json`
and `C:/t/dub-multilingual-validation/repaired-mcp-provider-valid-edit-proof/result.json`.

The separate Space Bunny edit succeeded in 10.375 seconds as session
`2f8afe30-1711-480e-b872-39a14ad6039f`. Root verified the exact requested file
change, COMPLETED primary result, and child exit 0. Big Pickle's observed limit
therefore does not establish unavailability of Space Bunny or failure of the
new provider admission path. Exact result:
`C:/t/dub-multilingual-validation/repaired-mcp-provider-valid-space-proof/result.json`.

The same clean committed source was rebuilt with matching embedded VCS metadata
and `vcs.modified=false`. The compiled Windows
`TestRestartRecoveryCancellationBeforeReadinessDoesNotClaimSuccess` actually
ran and passed (scenario 1.56 seconds, package 1.779 seconds), using that prebuilt
artifact. Source revision, binary hash and captured evidence are recorded in
`C:/t/dub-mcp-provider-validation-build/restart-proof/`. This supersedes the
earlier skipped-artifact limitation for that Windows scenario; Linux CI and
the broader integration suite still require their own verification.

That clean binary is now installed at `C:/Users/andre/bin/you.exe`, SHA-256
`48a2b10e63fdf7d28d30912f6031a97d05f44de60cfba2ad6ad3da3d4d314c52`.
A new stdio child using the installed executable repeated unknown-provider
rejection successfully in 0.735 seconds. Exact protocol and binary identity:
`C:/t/dub-multilingual-validation/repaired-mcp-provider-installed-proof/result.json`.

Separately, the normal public custom-model CLI discovered the published
independent-image Windows CUDA package without a backend source override.
The downloaded 443,193,250-byte archive hash and loaded executable/CUDA DLL
hashes matched the reviewed release. It returned all three literal Chinese
subtitles in the correct order, exited 0 after 405.406 seconds including asset
preparation, and left no owned processes. Its cue-25 overlay hallucination
(`EPISODE 01`) remains a semantic defect; this is not acceptance of a full dub
or a general vision-quality claim. Exact evidence:
`C:/t/dub-qwen-independent-images-managed-proof/`.

Two later bounded real editing tasks completed successfully on Space Bunny.
Session `ad458be5-2931-4bc4-b289-a67753459b74` removed the MCP binding dependency
bag and forwarding wrapper in 94.625 seconds. Root reviewed every converted
injection argument and ran the full owning MCP suite successfully (17.114
seconds), then committed the two-file flattening as `44310080b4`. Exact result:
`C:/t/dub-multilingual-validation/repaired-mcp-flatten-bind-proof/result.json`.

Session `8646ba5f-f544-477d-aabe-f72b8365abbb` delivered the throttled recovery
guidance in 218 seconds. Root reviewed the final two-file change and independently
ran the classification race test three times successfully (1.056 seconds).
The remedy now explicitly recommends waiting/retrying or another available
configured model/provider, while preserving partial-edit inspection, log
correlation, typed failure and private-payload protection. Exact author result:
`C:/t/dub-multilingual-validation/opencode-throttle-action-space-bunny-result.json`.

The IndexTTS feasibility audit first received a typed Big Pickle throttle after
117 seconds, then completed on Space Bunny after 281 seconds as session
`b859380c-79b3-49cd-8450-271538b943cd`. Independent source review confirmed the
current pinned LocalAI source has no audio-cpp backend. A newer immutable
LocalAI/audio.cpp pair documents Windows MSVC/CUDA and reference-audio IndexTTS
support, but it needs a distinct source/protobuf build and actual invocation
proof. The audit is useful planning evidence, not verified runtime support.
Exact results and source review:
`C:/t/dub-multilingual-validation/opencode-indextts-feasibility-result.json`,
`C:/t/dub-multilingual-validation/opencode-indextts-space-bunny-result.json`, and
`C:/t/dub-multilingual-validation/indextts-windows-feasibility-review.md`.

The initial SDK edit completed with a primary result, but its claim of no false
success was contradicted by an independent already-canceled-caller witness:
fourteen of sixteen buffered-response/EOF draws falsely succeeded. A bounded
five-minute Space Bunny corrective call then timed out with partial guards and
an incomplete decoder regression. Independent review finished the correction,
and root ran the full SDK race suite successfully in 7.388 seconds, including
the new EOF, notification, cancellation, deadline and decode cases. This does
not establish the compiled provider boundary until its separate test runs.

Root review also found that the proposed local `go.mod replace` would break the
published `go install ...@version` path. The replacement arrangement is therefore
unaccepted and is being changed to one preserved SDK package within the root
module, with mechanical namespace changes and truthful provenance. The twelve
unreachable-code vet findings in upstream generated SDK code were reproduced
on pristine upstream bytes; authored application checks must remain intact.
No SDK draft has been included in the installed clean `8d0225132e` executable.

The IndexTTS build-author task reached its five-minute timeout without a
primary result or authored wrapper. The reviewer then executed the unmodified
official immutable Windows MSVC/CUDA build script. That standalone native build
passed in 419.740 seconds, and its help/version commands succeeded with CPU and
CUDA reported. Its identified executable is recorded with build commands and
toolchain evidence in `C:/t/dub-indextts-windows-native-proof/`. Reference-audio
GPU inference, gRPC packaging, and public Models invocation are still pending;
a compiled executable alone does not establish those requirements.

### SDK repair, final structured schema, and native reference-audio proof

The corrected SDK repair is committed as `71dffd4e3f`. It keeps the preserved
SDK package under the root-owned `third_party` namespace and introduces no
local `go.mod replace` and no nested module, so the published
`go install ...@version` path remains intact. Root independently ran the full
SDK race suite successfully in 7.432 seconds. The compiled EOF proof at the CLI
provider boundary is still pending, so this remains a source and suite result
rather than a compiled boundary acceptance.

The final structured-schema change is committed as
`a335e9cfb9943f354b2a0c58e3d6cbd28baa7ee1`. It retains thinking and the default
uncapped generation path and excludes `ReasoningContent` from the final JSON
answer. Root's independent localai and platform race runs passed in 1.217 and
1.057 seconds. An isolated native CUDA build of that change is still pending,
so this is not live native structured-generation acceptance.

The standalone native IndexTTS Windows CUDA executable then produced actual
reference-audio output for the original video cue 13.44-16.32 s: English in
9.625 s and Chinese in 10.031 s, both exiting 0 and generating valid WAV files.
Exact evidence:
`C:/t/dub-indextts-windows-native-proof/native-reference-results.json`.

The normal installed public Models ASR path also agreed exactly with the target
text in both languages: English in 31.203 s and Chinese in 29.454 s. Exact
evidence: `public-asr/acceptance-summary.json` in the same proof root.

Taken together, these establish native conditioning invocation and intelligibility
only. They do not establish perceptual voice similarity to the reference, the
public `models IndexTTS` surface, gRPC packaging, or full English dub
acceptance.

One external QA script first failed on an erroneous nonexistent input path
before any backend call; after the input was corrected, the Chinese run
executed. That was a harness input error, not an MCP failure.

### Compiled ACP boundary acceptance and installed clean a335 verification

A clean build of committed `a335e9cfb9943f354b2a0c58e3d6cbd28baa7ee1` is now
installed at `C:/Users/andre/bin/you.exe`, SHA-256
`38964fcd5f8f7ca908523e8801b3bf3d65d1a654958a7ce201b94cec54f50704`. This
replaces the previously installed `8d0225132e` binary and is the first
installed executable that contains the corrected SDK repair, so the earlier
"no SDK draft has been included in the installed clean executable" statement no
longer describes the installed artifact.

The compiled CLI provider boundary case was never skipped: its earlier compiled
trials FAILED before the peer was invoked. The first attempt failed with
`execution catalog resolution failed: runner is not a supported built-in
identity`, and the next attempt failed with `ACP session does not advertise
requested model "fixture"`. Both are custom-runner rejections ahead of any
peer response, not delivery evidence. After the fixture correction in
`72c209d9ff` advertised its capabilities, both advertised-fixture cases passed
against the current prebuilt artifact: `Initialize` negotiated against protocol
version `999` under immediate EOF, and the Prompt primary-result case also
passed under immediate EOF. Source and compiled passes are accepted now; root's
independent suite run passed in 5.721 seconds and the preserved compiled run
passed in 4.977 seconds, with one compiled trial per mode. This closes the
pending compiled-boundary item recorded above for the EOF arrangement only; it
is not evidence about the broader timeout cause.

A real MCP editing call on the clean candidate
`C:/t/dub-acp-version-ordering-proof/you-a335-eof.exe`, SHA-256
`38964fcd5f8f7ca908523e8801b3bf3d65d1a654958a7ce201b94cec54f50704`, returned
`COMPLETED` in 38.750 seconds, session
`fdc04954-0606-4560-98a2-9c19a76f91df`. That same candidate SHA is what was
installed at `C:/Users/andre/bin/you.exe`; the edit itself ran on the candidate
path, not on the installed path. This is a bounded editing success with the
requested model, not a general editing or concurrency guarantee.

The installed path itself validated unknown-provider rejection: a readable
content text plus matching structured error metadata and `isError=true` were
returned in 1.110 seconds, rejected before dispatch. This confirms the provider
admission path is present in the installed artifact, and narrows the previously
delegated classification ambiguity for this input.

The installed path also ran an intentional 5000 ms timeout probe, which
returned in 6.360 seconds with the typed `timed_out` outcome. Cleanup closed the
live Factory Session, and the structured result carried private reasoning
activity metadata without the reasoning text. Session
`406a90a4-8826-475d-9961-51bcaadb43b9`, request
`9d3c2c04-050d-409b-ab3b-191927ac1308`. The suggested action named
partial-edit inspection, log correlation, a longer timeout, or another model,
with no raw provider payload. This verifies typed timeout reporting and
redaction on the installed binary; it deliberately proves no successful
long-running edit.

Exact evidence directories:
`C:/t/dub-multilingual-validation/repaired-mcp-a335-edit-proof/`,
`C:/t/dub-multilingual-validation/repaired-mcp-a335-installed-invalid-proof/`,
and
`C:/t/dub-multilingual-validation/repaired-mcp-a335-timeout-proof/`.

### Recordings MCP flatten and deadcode gate

Two separate bounded tasks both completed successfully, and they are not a
before/after or candidate comparison.

The first task ran against the previously installed clean `8d0225132e`
executable and took 163.266 seconds, session
`d77b24c9-bef3-407e-901e-934f47a79a50`. It implemented the Recordings
injection flatten across seven owned files: `client.go` lost the
`RootDependencies` struct and the `Bind` wrapper while `BindToolOperation` began
binding directly by closure, and 29 call sites across six owning MCP test files
were converted.

The second task was different work, not a faster rerun of the first: it ran on
the `a335` candidate and took 107.187 seconds, session
`858301d8-90a5-44d0-bab9-0b33d8e17389`. It updated one functional caller,
`tests/functional/recordings/root_composition/portable_transport_activation_test.go`,
and deleted one stale deadcode baseline line for the retired recordings `Bind`.

Root ran the full owning tests and the functional race suite, which passed in
15.552 seconds, and the combined change is committed as `87e55a1fb9`. The two
durations bound two distinct tasks and identify no cost attribution.

The deadcode gate started from a baseline of 3154 and reported 3171 current
findings, so the additions exceeded the baseline by seventeen. Eighteen of the
added findings were upstream ACP SDK helpers in the preserved dependency tree
that the root module compiles but this repository does not author; the one
removal was the repository's own
`pkg/services/recordings/transports/mcp/client.go: unreachable func: Bind`,
unrelated to the SDK. Exact preserved-dependency handling drops those eighteen
dependency findings, and the separately removed stale baseline line drops the
baseline from 3154 to 3153, so the remaining 3153 owned findings match the 3153
baseline and root's authoritative gate run passes. This makes the gate count
comparable instead of including dependency code; it does not reduce the review
burden on owned paths.

### Open items

A fresh native structured-schema build failed after 5.7 seconds, before any
dependency download. The cause was a patch written with CRLF line endings
applied to LF-prepared source. A real `git apply`-based fix and its test are
underway. This is a build-plumbing failure, not native endpoint acceptance, and
the pending native structured-generation claim above remains pending.

The unsupported-runner bug for a custom named ACP provider is newly reproduced
and fails before dispatch. Repair is underway and no success is claimed for that
configuration.

### Independent evidence correction and bounded writer timeout

The evidence entry above was authored through MCP session
`d9070497-837d-4651-bdb2-72a728d70dfc` in 68.500 seconds. Independent review
found reversed baseline/current counts, an invalid comparison between two
separate editing tasks, and confusion between the candidate and installed
executable paths. A bounded corrective Space Bunny task applied those
corrections but timed out after 121.688 seconds without a primary result:
session `9c06e220-aa1d-4602-bd0b-93faec41f200`, request
`c7045e88-8fd6-4b58-9e66-f0167364bc80`. The MCP response reported the timeout,
closed session, last activity, partial effects and concrete retry guidance.
Root independently reviewed the corrected partial edit before retaining it;
a completed file edit does not convert the terminal timeout into success.
Exact terminal: `C:/t/dub-multilingual-validation/repaired-mcp-evidence-log-correction-proof/result.json`.

The line-ending repair is now committed as `cb59c212d3`. Actual Git tests cover
LF and CRLF prepared sources with opposite patch endings, repeat application,
unknown/partial source rejection and mixed-source rejection without mutation.
Root's combined CI/applicator run passed all eleven tests. A new isolated
canonical CUDA build is live from the frozen committed inputs; runtime native
structured-answer acceptance remains pending.

### Versioned install, compiled EOF suite, and concurrency evidence

The normal network versioned install path now works with the preserved
`third_party` namespace arrangement:
`go install github.com/portpowered/infinite-you/cmd/factory@14a917cd88a164b045a65211256b747a511ec778`
succeeded using an isolated GOBIN with GOWORK off, and Go auto-selected toolchain
1.26.8. The resulting binary has SHA-256
`16b6284525348cfbf40e118dacb956c9aab838f31968eb014003b31dd6f5900a` and is installed at
`C:/Users/andre/bin/you.exe`. This is the first published `go install ...@version`
verification recorded for the repaired dependency layout.

The compiled three-row EOF suite passes against that artifact: root's independent
run completed in 10.022 seconds and verifies the immediate-EOF `Initialize` case
returning the unsupported protocol version `999` error, the Prompt primary-result
case, and the registered custom `eof-peer` provider on the actual ACP route. The
provider repair is `c09d7254f4`; the previously recorded unsupported-runner repair
is no longer pending.

An unregistered provider `unregistered-acp-proof` supplied to the installed binary was rejected
at canonical provider-selection activation, before any session, workstation, or
external execution, in 1.082 seconds. Exact evidence:
`C:/t/dub-acp-version-ordering-proof/versioned-14a917`.

Two queued tool calls on the same server both returned COMPLETED, sessions
`fd7261b6-acc1-414b-8255-95a47151390e` and `651a89b8-f30d-48e3-bdbe-f951d667b923`,
replying at 17.359 s and 27.343 s with a 27.484 s total, exit 0, and both requested
exact edits passed independent inspection. Exact evidence:
`C:/t/dub-multilingual-validation/concurrent-installed-14-proof`.

A stronger live probe captured 16 snapshots while both calls were pending. The
final-correlated session IDs `4c200484-7e20-488e-a887-4a53af8fc50d` and
`e7e75f73-dfda-4d3d-8de2-e80c772e49c3` were simultaneously live from 1.844 s to
17.469 s. Replies returned COMPLETED at 18.078 s and 37.360 s, total 37.422 s, exit
0. Task (a) passed exact edit inspection; task (b) omitted the requested period and
failed exact inspection, which the task itself admitted was model judgment rather
than a harness requirement. Evidence is under `concurrent-installed-14-live-proof`
in the same proof root. This proves two live sessions and a responsive session list
during pending calls; it does not prove precise provider-worker execution overlap
or semantic correctness.

An invalid-model probe against the installed path returned the typed
`provider_request_rejected` outcome with `isError` true in 2.485 seconds, and
cleanup closed session `bd546097-8d1d-48e4-aafd-195dd2fb9f70`. Root record:
`repaired-mcp-public-invalid-model-proof`. The correct recovery for this input is to
verify the advertised model list before retrying.

The settings injection flatten completed through actual MCP in 143.875 seconds,
session `d3c61847-73cf-4dbc-a45a-b9b184390e0d`. Root independently reviewed the
change and the owning and functional race suites passed; it is committed as
`879601d1a8`. It removed one owned deadcode baseline line (3153 to 3152) with no
baseline increase.

CI run `37070592713` at head `14a917cd88a1` failed at the BackendLint fmt-check step because a new catalog test file
lacked a final newline. The selector step was skipped after that failure, and the
inventory step, configured with `if: always()`, received an empty `LINT_JOBS`, which the strict parser
rejected with exit 2 and no inventory output. Commit
`9aa9fea9a82fe120f7ee671cfc387392c9f630ba` adds the gofmt final newline, makes the
selector always run with a positive fallback output, and gives the Make target a
blank-only fallback to the existing budget; nonempty invalid values keep strict
validation. Root's 20 Node checks passed in 32.216 seconds. CI run `37071707444` was
still in progress as recorded; no green claim is made for it.

A bounded Make-author task timed out at 300000 ms, session
`3b7646bc-f23c-4e33-830e-356059a52bb5`, request
`69f04bed-f1e9-4c00-821a-81b1026b640d`, with last FILE_CHANGE
`2026-10-02T22:13:23.5849185Z`; the session was closed and partial effects are
possible. The partial sources were independently reviewed and finalized. A completed
local edit does not convert that timeout into a COMPLETED result.

The native structured-schema build is live at this checkpoint; there is still no final native
structured-answer acceptance. The IndexTTS gRPC build and two reference RPCs
succeeded, which is not yet public Models acceptance, and no new ASR exactness claim
is made.

### Correction to the compiled ACP boundary paragraph

The earlier phrase "Both are custom-runner rejections ahead of any peer response"
overstates the model-advertisement failure. Only the unsupported-runner trial failed
before the peer was invoked. The advertisement error for the requested model
`fixture` occurred after the `Initialize` and `session/new` replies and before
`session/prompt`. The earlier paragraph is left unchanged; this section corrects its
scope.

The latest appendix was authored through bounded OpenCode MCP session
`4d31efe1-adff-4fe5-86d0-030d24cfbcd6`, which returned a COMPLETED primary
result. Independent review corrected the unknown-provider name, the second
concurrency session ID and first reply duration, and the full CI run identity
before retaining the appendix. The original log bytes were preserved.

### 2026-10-02 CI run `37071707444` outcome and goal-resume evidence

CI run `37071707444` at commit
`9aa9fea9a82fe120f7ee671cfc387392c9f630ba` completed `FAILURE`. This
supersedes the earlier "still in progress" record above. No final green, merge,
live schema inference, or proven duplicate cause is claimed here.

- **Backend lint.** It identifies two new direct `os.File` usages in the
  Providers ACP `command_parse.go`. Proper Platform process ownership plus
  explicit canonical Wire injection is being authored via MCP and is **not
  accepted yet**.
- **Functional.** The raw artifact shows one underlying failing scenario,
  `TestPackagedGoalSharedScenarios/PausedSubmissionResumes`, plus its failed
  parent suite; these are not two independent defects.
  `shared_scenarios_test.go:177` observes two `execute-goal` dispatch
  observations for the same Work where exactly one is expected after paused
  submission and resume. The duplicate-dispatch cause remains unproven.
- **Targeted repetition.** Root ran
  `go test ./tests/functional/factory/packaged/goal -run
  '^TestPackagedGoalSharedScenarios/PausedSubmissionResumes$' -count=10` in the
  Windows live worktree, and it passed in 12.774s, so isolated repetition did
  not reproduce the CI sibling-parallel context.
- **In flight.** Root dispatched an actual installed `you.subagent` read-only
  Space Bunny audit with a 600000 ms allowance. No conclusion yet.

Exact external evidence:
`C:/t/dub-multilingual-validation/ci-37071707444-functional-artifact/raw-failures/index.json`
and `C:/t/dub-multilingual-validation/repaired-mcp-goal-resume-audit-proof`.

### Canonical Windows CUDA schema build from `cb59c212d3`

The canonical Windows CUDA schema build from commit
`cb59c212d351f1a63235330b10ac5ae4a8dfe300` passed `exit 0` in 40m39.75s. The
linked CPU schema, startup, and payload gates passed, with 23 authored inputs
and 18 package files verified. Live inference and public release are still
pending, so this is not native structured-answer acceptance. Exact evidence:
`C:/t/dub-llamacpp-schema-build-proof/build-result.json` and
`verified-package-provenance.json` in the same proof root.

### 2026-10-02 parallel Goal suite repetition, log-edit terminal result, and author timeouts

Root ran the full parallel Goal suite, `go test ./tests/functional/factory/packaged/goal -run '^TestPackagedGoalSharedScenarios$' -count=10`, and it also passed in 17.699s. Together with the earlier isolated scenario repetition, this does not reproduce `PausedSubmissionResumes` in the CI sibling-parallel context, so the duplicate-dispatch cause remains unproven and no duplicate defect is claimed fixed by repetition alone.

The latest actual MCP attempt to edit this log wrote the requested 39-line appendix, but its terminal result is a failure: `isError` true, code `factory_session.subagent.provider_unknown_failure`, provider activity `FAILED`, `sessionClosed` true, elapsed 103.781s, session `2f046143-8fc4-4d70-bf6f-f1434730e439`. Server stderr contains only a peer connection closed line at 22:52:05, and provider log run `9828d7af` records no proven root cause. The partial edit is not counted as success, and the EOF peer-connection line is not evidence of a timeout or of a permission outcome. Exact evidence: `C:/t/dub-multilingual-validation/repaired-mcp-ci-resume-log-proof/result.json`.

The IPC author timed out at 1200000 ms with partial edits and no primary result, session `769c0b53-a28f-4b36-9e0e-be7a5b5cc62a`, request `2f57f4fd-d338-425a-9fde-bc2b24e50ff1`, last `FILE_CHANGE` `22:51:37.6139021Z`; the session was closed. Independent review is finishing the proper pipe-ownership change.

The six-file Index author also timed out at 600000 ms with partial edits and no primary result, session `3a91afb6-4a8d-475a-a570-82402e33085d`, request `10a4a143-e790-49a3-ad89-aed4cc301bc1`, closed, with no proven cause. A completed local edit does not convert either terminal timeout into a COMPLETED result.

Root independently rehashed all 18 schema candidate payload files, and every size and hash matched. Public live schema proof is still pending, so no merge, green CI, or public structured-answer acceptance is claimed here.

### 2026-10-02 Index follow-up and Goal audit terminal outcomes

The narrowed Index repair completed with a primary result in 99s, session
`05a99439-5955-4780-944b-5e7a1c22ec64`. It corrected two fixtures, removed a
needless exported constant, and corrected a discovery comment. It also adjusted
two characterization-test references outside its three named paths but inside
the approved owner scope. Independent Models, backend-registry, and wire tests
passed in 0.030s, 0.026s, and 0.036s. Exact result:
`C:/t/dub-indextts-localai-grpc-proof/opencode-public-integration-repair-result.json`.

Two launcher edit requests failed immediately with
`factory_session.subagent.provider_unknown_failure`, `INVOCATION_RUNTIME_FAILURE`,
and cleanup closed: Space Bunny session
`c86c01d9-3d8f-4e9e-b413-395c1a3b8d47`, then BigPickle session
`d9cfaec9-8924-40dd-b211-107e14d71398`. No launcher edits were observed. A later
Space Bunny request to append these notes also failed immediately with the same
classification, session `06c38510-f2c0-4914-8a7b-e42ea674b9e5`; no note edit was
observed, so this appendix was written manually. The causes remain unknown.
These three calls used the exposed `you.subagent` connector from `functions.exec`,
with the managed worktree as `workingRoot`; its server binary identity was not
exposed. They did not use root's installed-binary stdio proof client, so the
results do not establish failure of that separately successful invocation path.
Exact results are `opencode-launch-writer-space-result.json`,
`opencode-launch-writer-bigpickle-result.json`, and
`opencode-deferred-notes-result.json` in the same external proof root.

The user prioritized Qwen full dubbing. With root approval, the six owned
optional Index drafts were preserved externally and removed from the release
tree; unrelated changes were preserved. The exact patch is
`C:/t/dub-indextts-localai-grpc-proof/deferred-index-public-integration.patch`,
SHA256 `9a66d7b3f76fa3339c34f6fbff3f05c2445a6af438acad566ef9aac4f7ca249a`.
`deferred-index-public-integration-provenance.json` records the per-file hashes
and Git identities. Private Index CUDA reference-audio and actual production
codec interoperability proofs remain retained. Normal public Index discovery
and invocation remain incomplete.

The root Goal read-only audit timed out at its 600000ms allowance, measured
elapsed 602.406s, session `129dc0b4-66ed-4d6f-bf48-371362cc370b`, request
`71b71e61-4341-4817-8613-2efd72786cff`. Its last activity was `REASONING` at
`22:57:36.8893634Z`; cleanup closed the session. It returned no conclusion and
made no observed edits. The timeout cause remains unknown. A narrower Goal
diagnostic writer is in progress; that is not acceptance evidence.

### 2026-10-02 versioned `b050` install, native release download, and CI outcome

- The normal network versioned Go install of
  `b050291bfd91b847410e2d0c08810b851b5185ba` succeeded and produced
  `C:/Users/andre/bin/you.exe`, SHA256
  `fe20cf4fb9a2b38cdb421b1ab758c45854d27e8c0501b3adfe699c7b261b9384`, from
  module `v0.0.8-0.20261002232031-b050291bfd91`.
- The compiled ACP EOF three-row proof passed in 6.746s against that install.
- A fresh installed stdio MCP edit completed in 33.954s, session
  `96929d18-8a1e-46c3-90ed-8ce92a00b1f5`. The exact edit was independently
  verified and the server exited 0. Proof:
  `C:/t/dub-multilingual-validation/installed-versioned-b050-proof`.
- Codex config was pointing at the old `you-model-infer-20260929.exe`. The
  command was updated to `you.exe` with `tool_timeout_sec` 3700 preserved, but
  already loaded connector processes remain the old ones. An exposed connector
  edit nevertheless completed in 7.816s, session
  `03b79549-318b-4091-9b28-a78cae00c155`; that success is not a new binary
  identity proof.
- The native schema Windows CUDA archive was published at release
  `localai-backends-v1-90eb06d7fb9a54cb8ae9ca83172ca599cbd1252d500446dedbf8400a9405add5`,
  archive SHA256
  `272233b21ce0d353b10bfdd31dffb5d37bfc21a55a2efee5dd57490be34a2c19`.
  A full normal installed factory downloaded that exact archive and uses native
  executable SHA256
  `8ece891bfd64a1526a58a22cd4e82bf8bab572aaaea4f8372a03fb33c2e7c475`.
- The public prerelease Models schema proof retained 855 private reasoning bytes
  while the public JSON omitted the reasoning/thinking keys. The Korean prompt
  fixture returned English, so this is representation acceptance, not Korean
  translation acceptance.
- The full actual original 240-second Chinese video is still running toward
  en-US. ASR produced 59 cues and the initial exact ordered 59-cue translation
  completed; the semantic audit is in progress and no final dubbed output is
  accepted yet.
- CI run `37077160500` on `b050` completed with Backend Functional Coverage and
  Verification Policy failure. Diagnosis is in progress; no merge or green CI
  claim is made.

The bounded installed-`b050` MCP author for this section returned `COMPLETED`
with a primary result in 53.782s, session
`38acf72d-5647-4284-b81d-9e540eb79fc0`, and the server exited 0. Root
independently verified the exact append-only 37-line edit; no other file was
touched and no Git mutation was run. This records authoring provenance for the
section above only and adds no new install, native, CI, or dubbing evidence.
Exact raw result:
`C:/t/dub-multilingual-validation/b050-release-progress-log-proof/result.json`.

### 2026-10-02 controlled-clock CI repair and bounded MCP outcomes

The preceding `b050` CI run `37077160500` failed
`TestModelsASRControlledHealthTimeoutStopsReadiness` and
`TestModelsGenericCLIProcessTimeoutStopsReadinessAndPublishesNothing` because
the shared fake clock still advanced only 31 seconds, below the new five-minute
production deadline those tests now assert. The Verification Policy failure was
cascading from those two tests and was not a separate defect.

The installed-`b050` MCP OpenCode Space Bunny author returned `COMPLETED` in
54.047s, session
`fbad23a4-9ce7-4d0f-9d7d-12650d28a124`, and changed one test helper only, running
against the same actual installed server SHA256
`fe20cf4fb9a2b38cdb421b1ab758c45854d27e8c0501b3adfe699c7b261b9384`. Root
independently verified the focused race pass in 2.651s and the full Models
`root_composition` pass in 25.907s. Public error and cleanup assertions and
wall-clock waits were unchanged, and no production code changed. The fix was
committed and pushed as
`8e1daf1b861f8341c83512a22c1d1d0d5ac611bc`. CI run `37078759573` Backend
Integration passed, including the real five-minute blocked-load witness. The
functional lane was still active with no failures observed, and no merge is
claimed here. Exact evidence: `C:/t/dub-ci-b050-review/proof.json`.

Separately, a documentation provenance paragraph author returned `COMPLETED`
with a primary result in 397.578s and server exit 0, session
`08c0fd90-4ed3-42dd-8d9d-64a4f296c05b`, with a verified additive 9-line edit
only. That call was not a timeout, but the cause of the long latency remains
unproven, and the different prompts mean it does not establish before/after
performance. It overlapped the 54.047s one-test writer; both completed
independently in separate source paths with no conflicting edits. Raw result:
`C:/t/dub-multilingual-validation/b050-log-terminal-proof/result.json`.

A subsequent plan author returned `COMPLETED` in 46.469s, session
`b1cbd733-8e70-49c6-994f-494083b937e9`, with an independently verified
append-only 48-line edit and no other file changed. Raw result:
`C:/t/dub-multilingual-validation/b050-current-delivery-plan-proof/result.json`.

### 2026-10-02 failure-correlation diagnostics, broad-author timeout, and green CI

An installed-`b050` OpenCode Space Bunny bounded diagnostics review completed in
42.500s, session `d06715cf-d272-417f-abd9-f178fe1285e2`, with a primary result
and server exit 0 against the same actual server SHA256
`fe20cf4fb9a2b38cdb421b1ab758c45854d27e8c0501b3adfe699c7b261b9384`; it made no
source edits. It independently found that ordinary `FAILED` results drop the
runtime RequestID/TraceID/WorkID because those fields are attached only in the
timeout branch. A broad author for that correlation work was already dispatched
with snapshot expansion when root narrowed final acceptance to IDs-only; it
reached `TIMED_OUT` at 301.125s against the requested 300000ms, session
`257eeb51-2839-47c6-871d-4e5d6a799b48`, request
`ced92381-1f26-4249-8e24-749dafea954e`, last `FILE_CHANGE` `UPDATED`
`23:58:28.9122711Z`, with cleanup closed and `partialEffectsPossible`. It left
two-file partial changes and no primary result, so it is not a completed author.
The exact broad patch, protocol, and result are retained at
`C:/t/dub-multilingual-validation/mcp-failure-correlation-edit-proof`, patch
SHA256
`de8f824b0b73720681bb1e33172c42290773c4fe910b84fd2372b8e3f804475d`. A separate
narrow author is now active after the broad call reached its true terminal, with
no overlapping writer. Final acceptance is IDs-only, with no extra snapshot calls
or helpers and private-payload protection preserved; it is not yet verified or
committed. A caller-selected five-minute budget exhaustion is not evidence of a
harness root defect, and the underlying long-task cause remains unproven.
Separately, CI run `37078759573` completed `SUCCESS` on
`8e1daf1b861f8341c83512a22c1d1d0d5ac611bc`, superseding the earlier in-progress
record above; raw artifact `C:/t/dub-ci-8e-green-proof/run.json`. The newer
pending diagnostics are not CI-proven, and no merge is claimed.

The separate narrow author that followed the broad call also failed: it reached
`TIMED_OUT` in 181.422s against the requested 180000 ms with no primary result,
session `274185d3-73d7-45dd-8ead-fc4b6e08036d`, request
`9a7536eb-edd7-4b82-b28c-f3354d6c80f5`, last `FILE_CHANGE` `UPDATED`
`00:01:51.9520886Z`, cleanup closed with `partialEffectsPossible`, exact evidence
`C:/t/dub-multilingual-validation/mcp-failure-correlation-narrow-proof`. Its
partial IDs-only patch passed the tests independently, but the new test
scaffolding breached the file budget at 1051 > 1000 lines and the helper budget
at CC 16 > 15, so it was not accepted at that time and no baseline was increased.
A subsequent installed-`b050` OpenCode Space Bunny test-maintenance author with a
1200000 ms allowance `COMPLETED` in 324.297s with a primary result and server exit
0, session `dfa5953e-0dec-40b5-9030-9f56b8ec1029`, exact evidence
`C:/t/dub-multilingual-validation/mcp-failure-correlation-maint-proof`. The final
scoped source keeps the original timeout and snapshot behavior and only hoists
the guarded runtime IDs before terminal classification, and the tests enhance
the existing seven-row classification/private-secret/cleanup table without
production helpers, state, or service reads. Independent verification passed the
full MCP race in 18.217s and package maintainability at 997 test lines and helper
CC 15, and the scoped diff check passed; the exact final reviewed patch is
SHA256 `7b5d4b3ee0b95481937f9e84fc94acddba6413b43a0d75ead502b6d083049af8`.
Root independently reviewed the source, while commit, push, and new-artifact
concurrency or failure proof remain pending. The earlier broad and narrow
timeouts stay recorded failures with partial effects and are not retroactive
completions; the larger task allowance is what allowed this task to finish and
does not establish historical root causes or a controlled performance comparison,
and no new CI, merge, or final-video success is claimed.