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
| OpenCode started a second stale executable | `C:\Users\andre\.config\opencode\opencode.jsonc` pointed its local `you-agent-factory` MCP server at `C:\Users\andre\bin\you-agent-factory-mcp.exe`, dated September 26 at 22:55. The installed definition reverted at 19:26:53 and 19:30:30 during OpenCode child starts, despite the main CLI using a fresh `you.exe`. | This second binary caused the repeat regressions. The config now points to `C:\Users\andre\bin\you.exe`; a subsequent child start kept the installed definition `ENABLED` with `workingRoot`. |
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

After the binary/config repair, two CLI OpenCode audits exited with code 0 but
no primary result and no requested edit. Their stdout contained only an initial
progress sentence and an ACP `peer connection closed` line. A prior CLI
OpenCode dispatch completed a documentation edit and returned a primary result.
The OpenCode log explains the empty audits: they tried to read paths outside
`workingRoot` and reached `external_directory` permission requests with action
`ask`. The harness did not surface a useful pending-approval outcome before
closing. Keep bounded editing tasks inside the workspace until that permission
bridge and terminal classification are fixed.

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
  the OpenCode log showed the actual build model was `big-pickle`, so the
  explicit selection silently fell back. This is being fixed.

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
   `~/.local/share/opencode/log/opencode.log`. The caller currently can report
   success with no primary result when the child waits for this approval.

## Adjacent CLI invocation findings

- Invoking `opencode/muse-spark-1.3-contributor-free` as one value failed with
  `reference identity is invalid`; the execution catalog's reference pattern
  rejects `/`. Passing provider `opencode` and bare model
  `muse-spark-1.3-contributor-free` passed.
- `opencode` was then rejected with `runner is not a supported built-in
  identity`; `knownExecutionRunner` listed only `codex`, `claude`, and
  `antigravity`. `opencode` was added to the runner catalog in
  `pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution/catalog.go`.

## Limitations of this note

- Re-reading the installed definition after the 19:22 build showed
  `policy: READ_ONLY` with no `workingRoot` again because OpenCode's separate
  MCP executable was stale. The config was corrected and one subsequent
  dispatch preserved `ENABLED`; repeat dispatches should confirm durability.
- No trace, transcript, or worker log was captured for the timeouts, so the
  timeout section stays at hypothesis level.
- This note documents a single-operator desktop environment. Results may differ
  on other hosts.
