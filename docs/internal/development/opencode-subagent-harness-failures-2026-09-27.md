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
- The app's MCP connection reported `Transport closed` after its stale server
  was restarted. This is a connection lifecycle observation, not evidence of
  the provider outcome. The direct stdio probe above initialized separately.

## Limitations of this note

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
