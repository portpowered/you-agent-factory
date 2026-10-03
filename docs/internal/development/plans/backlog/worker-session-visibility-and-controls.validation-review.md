# Validation review: Worker Session visibility and controls

Plan reviewed: `docs/internal/development/plans/backlog/worker-session-visibility-and-controls.md`. Source baseline: origin/main 933b8c691d. This was a blind review: the plan was checked against source, contracts, Make targets, CI and planning standards. The plan was edited in place; this file records why.

## Verdict

**READY-WITH-CHANGES.** The edits below make every packet dispatchable. T1 must go first and its plan amendment must merge before any later lane is dispatched. That amendment carries the Go Current/Proposed excerpts that T4, T5 and T6 still lack, and the "T1 amendment slot" paragraphs in those packets block their dispatch until it lands. The decisions in section 2 are defaults that the operator can overturn (see Residual decisions).

## Findings

| # | Severity | Finding | Evidence | Change made |
| --- | --- | --- | --- | --- |
| 1 | High | MCP has no Worker Session exposure for any capability, and the plan did not mention MCP. | `pkg/transports/mcp/generated/discovery.json` (11 tools: 10 `you.factory_session.*`, `you.subagent`); `pkg/wire/profiles.go:617-630` routes every tool to `factorysessionmcp`. | Added T9 (MCP spine with seven tools against the `--server` host), plus MCP additions in T3 (`history`, `read_logs`), T5 (`kill`), T6 (`resumeMode`) and T7 (`start`). Section 5 now carries exact tool schemas and an error envelope; F11 parity cases and an M1 static gate were added. Tool count superseded by finding 16. |
| 2 | High | MCP runs in-process and cannot see Workers owned by a running host. | `docs/reference/mcp.md:22-24`; `local-only` placement; `MCPIntent` at `pkg/initializer/process/contracts.go:76` has no server URL. | Decision: Worker Session tools call the selected host's HTTP API through the generated client, with no in-process fallback. Added the `MCPIntent.ServerURL` Current/Proposed excerpt and the routing and contract-check generalization to T9. |
| 3 | High | T4, T5 and T6 had no Go contracts, only "after T1 identifies" notes, and T1 had no defined output. | Original §5 kill note and T4 ownership text. | T1 is now a characterization lane whose PR amends the plan; the T1 loopback lists five required outputs. Each of T4, T5 and T6 has a "T1 amendment slot" that blocks dispatch. A §11 gate prevents any lane from dispatching before the amendment merges. |
| 4 | High | T2 and T8 contradicted each other: T2 text migrated `read`/transcript and removed the readers, which T8 also owns. | The "Captured logs page" note appeared in both §5 and T2. | The note now says T2 delivers logs only and T8 owns the migration and removal. T8's scope says this explicitly. |
| 5 | High | T8's blast radius was not enumerated, including the dashboard consumer of `/provider-sessions/detail`. | `ui/src/api/provider-session-details/api.ts`; deletion-only baselines in `docs/internal/baselines/`; `cmd/pkgboundarycheck/*`; `internal/ownershipinventory/provider_sessions*`; functional and stress rollout tests. | Added a "Known consumers" inventory to §6. T8 must keep the dashboard panel working through captured associations (`make ui-test`), ratchet the baselines down, and migrate or delete `TestLargeRolloutStress`. |
| 6 | High | Analysis fields were missing for an experiment observer: no per-record time, lineage dropped from the public schema, no provider before a Provider Session ref, and no terminal cause. `tokenUsage`/`turnUsage`/`parse` come from the native readers that T8 deletes. | `WorkerSessionEventRecord.yaml` has no time; the Go `Observation` has predecessor/successor fields; `observations.go:637-657`. | Added `capturedAt` (T2), durable `tokenUsage` from capture (T2), lineage and `provider` (T3), and `terminalCause` (T4; T5 writes `OPERATOR_KILL`). T8 must retain `tokenUsage` and document any loss of `turnUsage`/`parse`. |
| 7 | Medium | The section 2 open questions were unresolved, so packets depended on undecided scope. | §2 "Open questions". | Replaced with a Decisions table covering Factory interrupt, kill scope, platforms, continuation, history backfill, retention, MCP host and follow, and history default. Each decision has a revisit trigger. |
| 8 | Medium | L1 cited `tests/stress/worker_sessions`, which does not exist and had no owner. `make test-stress` passes `-short`, which skips stress cases. | `tests/stress` is one flat package; `Makefile:139`. | T3 creates the package and owns L1, run with an explicit `go test ... -count=1 -timeout 15m`. |
| 9 | Medium | The I1 and V1 artifacts had no owner, and I1 would skip locally and read as green. | CI `backend-integration` (`ci.yml:573-653`, ubuntu only) builds `.artifacts/integration/bin/you`; `Makefile:592` uses a hard-coded package list. | Section 10 now names the CI job and the required env vars, and says to extend `tests/integration/workers/cancel` or append a package to the Makefile list. V1 builds once with `go build ./cmd/factory`. Windows is UNSUPPORTED unless a Windows I1 run is recorded. |
| 10 | Medium | The generated Go client is filtered, so new operations would silently not generate. | `api/codegen_config/client.yaml` `include-operation-ids` has only three Worker Session operations. | Added to §5 generated artifacts and to T2, T5, T7 and T9. |
| 11 | Medium | The coverage manifest marks existing REST routes and all MCP tools `missing`. | `contracts/functional-scenarios.json`: `listWorkerSessions`, `terminateWorkerSession`, all 11 MCP tools. | T1 covers the two REST routes. Every new surface adds a `covered` entry, checked by `make contracts-check`. |
| 12 | Low | The CLI "Current" grammar was inaccurate: `stream` lacks `--follow` and provider selectors, `list` lacks `--session`, and `--remote` is shown as a per-command flag. | `contracts/cli/commands.json`. | Corrected both copies. Finding 16 later replaced the new `logs` and `kill` commands with flags on `read` and `terminate`. |
| 13 | Low | The plan used absolute local worktree paths. | 0 hits remaining. | Converted to repo-relative paths. |
| 14 | Low | "Portos" would leak into customer reference docs. | `docs/reference/` has no occurrences. | T2 and T8 require the wording "captured by `you`". |
| 15 | Info | ACP is out of scope for operator controls. | Only `chat_sessions/internal/responsebridge/worker_children.go` uses worker sessions. | Recorded as N/A in the matrix. |
| 16 | High | MCP/API surface compressed per operator: "too many tools; interfaces as small as possible to start bootstrap." The plan added 10 MCP tools (T9 seven, plus `read_logs`, `kill` and `start`), 2 HTTP routes with 2 response schemas (`/logs`, `/kill`), and 2 CLI commands (`logs`, `kill`). | `you.factory_session.control` already uses one tool with an uppercase `operation` enum across six POST routes; `get_result` uses a `mode` selector. `/events` is SSE and `/transcript` is terminal-only (409) with a required `providerSession`, so neither can carry a JSON log page. `streamWorkerSessionEventsByWorkerSessionId` in `client.yaml` is the Factory-scoped route, not the top-level one the old T9 text assumed. | MCP: 10 -> 3 tools: `list`, `read` (`view` summary/transcript/events; T3 adds `logs` + `nextToken`) and `control` (`operation` CANCEL/TERMINATE/INTERRUPT; T5 adds KILL + `expectedAttemptId`; T6 adds `resumeMode`). Later tasks extend these tools; they never add one. HTTP: 2 -> 1 new route (`/logs`) and 2 -> 1 new schema. Kill is an optional `{force, requestId, expectedAttemptId}` body on the existing `/terminate`, with an optional `forced` on WorkerSessionControlResponse. CLI: 2 -> 0 new commands, using `read --view logs` and `terminate --force`. Coverage-manifest entries: 14 -> 4 new. Go client include additions: 10 -> 6. Deferred to a follow-up: MCP continue, pause/resume and start (T7 has no MCP change; start would become a `control` operation). Updated §1, §2 Decisions, §4, §5, §9, F11, M1, §11, T2/T3/T5/T6/T7/T8/T9, V1 and §13. |

All Make targets cited were verified in `Makefile`: test-functional, api-smoke, cli-contract-smoke, cli-manifest-generate, docs-reference-smoke, generate-api, interfaces-all, contracts-generate, contracts-check, mcp-discovery-generate, mcp-contract-smoke, test-integration, test-stress, ui-test, lint and verify-pr. Every cited path exists except files the plan proposes to create.

## Parity matrix

Key: **exists** means present at 933b8c691d; **T*n*** means added by that task; **N/A** means not exposed by design. ACP is N/A for every row because operator control is not a chat surface. The "Generated clients" column covers Go and TS: TS is generated from the full contract, while Go needs the `client.yaml` include list.

| Capability | Service | HTTP/OpenAPI | Generated clients | CLI | MCP | docs/reference |
| --- | --- | --- | --- | --- | --- | --- |
| list | exists (process-local) | exists | TS exists; Go T9 | exists | T9 `list` | exists |
| history (archived/all) | T3 | T3 | T3 | T3 `--history` | T3 `history` | T3 |
| show | exists | exists | exists | exists | T9 `read` view summary | exists |
| transcript read | exists (native readers) | exists | exists | exists | T9 `read` view transcript; captured content T8 | exists; T8 |
| events replay | exists | exists | Go T9 (top-level route) | exists `stream --replay-only` | T9 `read` view events | exists |
| logs page | T2 | T2 `/logs` (only new route) | T2 | T2 `read --view logs` | T3 `read` view logs | T2 |
| follow | exists (stream); logs follow T3 | exists (SSE) | exists | exists `stream --follow`; T3 `read --view logs --follow` | T3 `read` view logs polling (no streaming, by decision) | T3 |
| capture health | T2 | T2 | T2 | T2 | T3 via `read` view logs | T2 |
| cancel | exists | exists | TS exists; Go T9 | exists | T9 `control` | exists |
| terminate | exists | exists (REST coverage T1) | TS exists; Go T9 | exists | T9 `control` | exists |
| kill | T5 | T5 force body on `/terminate` | T5 | T5 `terminate --force` | T5 `control` KILL | T5 |
| interrupt | exists | exists | TS exists; Go T9 | exists | T9 `control` INTERRUPT | exists |
| interrupt resumeMode | T6 | T6 | T6 | T6 `--resume-mode` | T6 `control` `resumeMode` | T6 |
| continue | exists | exists | TS exists | exists | Deferred (future `control` operation) | exists |
| start/invoke | exists | exists | TS exists | exists | Deferred (future `control` START); `you.subagent` exists | exists |
| analysis fields | partial (Go has lineage) | T2/T3/T4 | T2/T3/T4 | via `--output json` | via `read` | T3/T4 |

There are no unplanned GAP cells. MCP continue, pause/resume and start are deliberately deferred (finding 16). MCP streaming is a deliberate non-goal, and follow over MCP is cursor polling.

## Residual decisions for the operator

1. **Windows kill support.** The default is UNSUPPORTED, because no CI job runs on Windows. Supporting it requires either a recorded Windows I1 run or a new Windows integration job.
2. **MCP host routing.** The default routes `you.worker_session.*` tools through the `--server` HTTP API with no in-process fallback, and amends `docs/reference/mcp.md`. The alternative is to add an MCP surface to the host itself, which is larger and is not planned.
3. **Losing `turnUsage`/`parse` after T8** for sessions without captured equivalents. The default accepts and documents the loss; `tokenUsage` must survive.
4. **Dropping the native-only `/provider-sessions/detail` path.** The default makes unrecorded native-only sessions NOT_FOUND, while the dashboard keeps working for recorded sessions.
5. **Factory-originated interrupt** stays UNSUPPORTED until Runtime owns replacement.
6. **History default.** The CLI keeps its compatibility behavior when the flag is omitted, while MCP `list` defaults to `all`. Confirm that this asymmetry is acceptable or align both to the same default.
7. **Paid I2 validation** is still unauthorized. Real-provider resume and remote kill claims stay unproven until it is authorized.
8. **Dependency on the flat-injection project.** T4, T5, T6 and T9 constructors must rebase onto its merged `wire` shape. Sequence the two projects if they collide.
9. **Deferred MCP capabilities.** Continue, pause/resume and start are not reachable through MCP in this project. The default is to add them later as `control` operations, not tools. Confirm that the operator agent can bootstrap with CLI/HTTP for these.
