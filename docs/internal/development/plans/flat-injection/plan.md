# Flat dependency injection and shared process effects

This plan removes secondary service construction and dependency lookup from the Go application while preserving Factory Session isolation, replay, execution, transport behavior, and resource cleanup. It defines independent behavior lanes so owners can work concurrently. The plan changes documentation only; implementation and verification remain future work.

## 1. Problem and desired outcome

### Problem statement

Customers need concurrent Factory Sessions, execution, replay, and diagnostics to use consistent injected dependencies without sharing or losing session-owned state.

### Current behavior and gap

`root.BuildProcess` calls one `wire.InjectBundle`, but its providers return factories and composite roots that build further service graphs. Sessions opening passes peer services through eleven owner-port bags; Runtime assembly constructs a builder, instance host, Worker Sessions, engine, and bundle; Models lazily caches scoped service graphs; Automations recreates its service tree per runtime. HTTP construction obtains peer services through Sessions root getters. These are construction problems even when their argument lists are flat.

Active timestamp and scheduling paths also bypass injected clocks. Models receives two no-op Zap loggers instead of the constructed process logger. A distinct adapter, named logger, replay clock, or runtime resource is not automatically redundant: consolidation follows authority, time domain, and lifetime, not type or variable name.

### Desired outcome and success measures

1. Every reusable production behavior owner has one focused constructor provider in its owning `wire` package, is selected by canonical `pkg/wire`, and is supplied directly to consumers. A provider constructs one implementation; it does not recursively assemble a service tree.
2. No runtime operation invokes a reusable service constructor/factory, acquires services through a container, or installs behavioral collaborators through setters. Operations may allocate explicit session state, engine buffers, attempt state, resources, subprocess instances, and lifecycle handles through already-injected owners.
3. One default process time source supplies process timestamps and scheduling capabilities. One base process logging backend supplies process diagnostics and narrow logging adapters. Explicit legacy override precedence and separate replay time remain documented and tested.
4. Parallel sessions retain independent events, response cursors, Work, model capacity leases, runtime generations, cancellation, and artifact destinations. Failed startup and replacement retain rollback and retry guarantees.
5. Every finding in [inventory.md](inventory.md) has an owning task and a terminal disposition: removed, direct injection, retained scoped state/resource, or retired compatibility code. There is no unowned final sweep.
6. Required behavioral dependencies are normalized once at the caller-facing construction boundary. Internal constructors presume those parameters are real, store them directly, and do not substitute defaults, validate nil repeatedly, or silently omit behavior. Explicit disabled implementations are selected by Wire. Optional domain data and resource lifecycle state retain their existing semantics.
7. Static construction and effect-selection checks report zero prohibited occurrences in migrated lanes, and zero unresolved inventory findings at project completion. Behavior evidence, rather than constructor-count assertions, proves customer compatibility.

## 2. Scope and constraints

### In scope

Go service construction, owner-private collaborators, Sessions/Runtime activation, Models scopes, Automations activation, Workers/Providers execution, Recordings, Definitions, Work, Provider Sessions, Settings, visualization, Costs queries, Webhooks delivery, and CLI/HTTP/MCP composition; shared clocks/loggers; required constructor dependencies, nil fallback removal, and existing lint enforcement; relevant unit, functional, static, and limited prebuilt integration evidence. The impact inventory and concrete current/proposed contracts are companion documents to this plan.

### Non-goals

No UI redesign, API/configuration/event/persistence format change, runtime-policy correction (including Automations durable-cursor failure explicitness, recorded as a separate follow-up), microservice deployment, new DI framework, dependency service locator, global service registry, blanket conversion of state to singletons, or paid provider validation. Do not move private implementations out of Go `internal` solely so canonical Wire can import them.

### Assumptions and constraints

- Public service roots remain the peer boundary. Owner `wire` packages expose focused provider outputs/aliases for private collaborator interfaces; `pkg/wire` can assemble them without importing forbidden private packages.
- Flat injection means direct dependency edges, not one enormous constructor. Split behavior by its actual authority before removing a wide bag.
- `edges.Edges` remains the approved caller-facing external-effect aggregation exception. Never substitute a new internal bag for it.
- Preserve user changes. Do not hand-edit `pkg/wire/wire_gen.go` or generated public clients.
- Existing suites have been identified by source inspection, not executed for this planning change. Coverage sufficiency is an implementation prerequisite within each lane.
- An existing `Now`-only clock does not imply controllable timers. No adapter may invent deterministic timer semantics or silently replace a selected scheduler with real time.

### Open questions

These are bounded design questions with assigned resolution, not permission blockers: T01 determines timer-capability compatibility for existing `Edges.Clock` callers (decided: `After` joins the shared `TimerSource` seam); T11 confirms which cached Models values are resources versus behavior; T14/T15 confirm the single instance-host handle authority; T16 confirms Worker Sessions attempt keys and retention when replacing per-runtime services. If evidence contradicts the proposed keyed-state design, the owner submits the smallest delta plan before structural implementation. Operator decision (2026-10-02): T16 and T31 each make their contracts.md current/proposed pair their first required step, before any structural change (T16 in its own PR); a pair that cannot be written stops the lane with a delta plan.

### Replanning triggers

An unavoidable dependency cycle; a T16/T31 first-step contract pair that cannot be written (including Worker Session attempts not keyable by runtime ID and dispatch ID); externally consumed constructor removal; a discovered format/API behavior change; replay clock leakage into OS deadlines; scoped state that cannot be safely keyed; missing characterization for a customer invariant; or a task requiring multiple independently observable behaviors. Split affected work without blocking unrelated lanes. T09, T12, T13 and T17 keep "split if review requires"; the project lead MAY pre-split them at admission along the lines named in their packets (operator decision, 2026-10-02). Estimate after validation review: 31 implementation tasks (T27–T31 split out of T10, T15, T18, T20 and T22) plus one independent validation task; missing characterization may add a small prerequisite task inside the affected lane only.

## 3. Recommended approach

Construct reusable behavior once through focused owner providers and canonical Wire, then activate explicitly keyed session state and resources through operations. Consolidate default process clocks and logging first through additive seams while independent service decomposition proceeds against preserved peer contracts. Deliver 31 bounded tasks across concurrent lanes, with one composition steward owning small Wire registration/regeneration patches and lane owners retaining their own migration and removal work.

### Decision record

| Option | Decision | Evidence and tradeoff |
| --- | --- | --- |
| Direct leaf injection plus keyed state | Selected | Preserves event-first ownership and supports concurrent sessions without secondary graphs. |
| Replace eleven bags with one aggregate or huge constructor | Rejected | Preserves hidden dependencies or concentrates unrelated opening responsibilities. |
| Merge every clock/logger/host by type | Rejected | Replay ticks, OS deadlines, correlated logging, file sinks, and runtime handles have distinct semantic lifetimes. |
| Rewrite all roots in one PR | Rejected | Broad rollback, weak failure localization, and unnecessary serialization across unchanged peer contracts. |

## 4. Customer behavior

Actors are CLI operators, dashboard/API clients, and ACP/MCP callers with existing permissions. Their journeys remain: load/save a Factory; start an explicit Factory Session; submit Work or invoke a Worker/model; observe progress; pause/resume/cancel; replace the Current Factory; record/export/replay; inspect historical state; close resources. A standalone model or Worker operation continues to work without inventing a live Factory Session where its public contract does not require one.

Default/empty behavior, startup readiness, terminal success, typed errors, permission handling, quiet/normal/verbose/debug modes, and stdout/stderr/NDJSON/SSE framing remain unchanged. Construction must remain inert. Concurrent explicit sessions must not collide with `~default` or Current Factory selection. UI accessibility, focus, keyboard, responsive layout, localization, and visual references are not applicable: no UI or customer copy changes are planned; existing behavior is preserved.

## 5. Contracts and data

### Contract inventory and compatibility classification

| Contract | Authored source | Classification | Consumers |
| --- | --- | --- | --- |
| Process external effects | `pkg/services/edges/definition.go` | Additive process scheduler/logger seams; existing clock overrides retained during compatibility interval | `pkg/root`, functional fixtures, host callers |
| Service constructors/private collaborator aliases | Owners' `wire` and `internal` files in inventory | Breaking internal signatures, additive focused providers, deprecated composite providers | Canonical Wire and owner unit fixtures |
| Session activation/runtime view | `factory_runtime` and `factory_sessions` Go construction contracts | Internal decomposition; existing public Start/control/read contracts unchanged | Sessions, Runtime, HTTP, visualization |
| HTTP/OpenAPI | `api/openapi-main.yaml`, `api/components/` | Unchanged | HTTP, generated Go/TS clients, dashboard |
| CLI grammar | `pkg/transports/cli`, `docs/reference/` | Unchanged | CLI/functional scenarios |
| Configuration | Authored factory/operator schemas | Unchanged | Definitions, Settings, packaged factories |
| Events/messages/persisted recordings/checkpoints | Existing owner contracts | Unchanged | Replay, projections, responses, ACP/MCP |

### Concrete construction changes

[contracts.md](contracts.md) contains exact current source excerpts and proposed Go shapes. Task packets reference the applicable pairs; they must not substitute prose for changed interfaces. Deletion pairs explicitly show removed containers. Function parameter names may change during implementation, but dependency types, ownership, method inputs/results, and compatibility obligations require a plan delta if they change. Additional discovered signature changes must receive current/proposed blocks before implementation; this is a scope-control rule, not authorization to defer the design.

### Persisted data and generated artifacts

No data migration or retention change. Preserve read compatibility for existing recordings, cursors, logical session identities, and checkpoints. Regenerate `pkg/wire/wire_gen.go` using `make generate-wire` for provider changes. Public API generation is unnecessary unless a separately reviewed contract delta is approved; such a delta must name authored fragments and all generated Go/TS/package consumers. Do not regenerate unrelated artifacts to hide drift.

## 6. Architecture and state

### Current and target flow

Current: `BuildProcess -> InjectBundle -> composite providers/factories -> Sessions opening -> Runtime Assembly -> RuntimeBuild -> RuntimeFactory -> Bundle -> post-construction binding`. Models and Automations additionally reconstruct behavior underneath operations. HTTP obtains services from Sessions and constructs adapters after opening.

Target: `BuildProcess -> InjectBundle -> individual inert behavior owners and transport adapters -> Process.Execute -> injected activation operations -> session state/resource handles -> event-first runtime`. Initializer starts, stops, cancels, and joins constructed roles. It never selects or constructs product collaborators.

### Concrete decomposition map

| Current container or graph | Injected behavior owners | Scoped data/resources that remain | Removal owner |
| --- | --- | --- | --- |
| Sessions port bags and opening Root | request normalization; definition/replay selection; durable execution; runtime activation; observation/presentation | normalized request, session/runtime/generation IDs, model scope, replay facts, cleanup lease | T13, T17 |
| Sessions Assembly and late binding | session directory/state; response streams; invocation; live-change coordinator | registry entries, response sequence/cursors, start singleflight, admission projections | T13 |
| Runtime Assembly/Build/Factory | definition compilation; engine behavior; dispatch planning; instance host; artifact/log/metrics owners; sidecar activation | marking/net, buffers, attempts, generation view, owned sinks/recording resource | T14, T31 (build leaves), T15 |
| Runtime Bundle/products/presentation | direct consumer injection and narrow runtime queries | identity, diagnostics, scoped state and lifecycle handles | T15, T17, T28 |
| Models component bag and scoped service cache | scopes; assets; catalog; host; leases; inference; execution/limiting behavior | scoped config, supervised processes, capacity leases, verified assets, cache facts | T10, T30 (slot state/coordinator), T11 |
| Automations per-runtime owner | reconciler; source lifecycle driver; cron; script pollers; watchers; hosted sources; cursor service | runtime/source identities, cursors, scheduled instances, cancellation/join state | T12 |
| Recordings combined root | ledger; projection; lifecycle; export; replay; historical query | recording scopes, ledger entries, flush tasks, durable artifacts | T03 |
| Definitions composite root | catalog; validation; compilation; authoring; portability; distribution; runtime snapshot queries | authored definition/version/layout values | T04 |
| Workers registry/conductor graph | constructed agent/script/inference strategies; registry; Execute; harness; prompting/worktree effects | per-attempt request, observation routing, provider override selection, worktree resource | T09 |
| Providers options/built-in bags | catalog; ACP; constructed provider effects; normalized execution; registrations | provider configuration, selected attempt, protocol/session handles | T08 |
| Work/Provider Sessions/Settings composite roots | state access and request preparation; Codex/Cursor readers; document and resolution owners | work input, transcript reads, settings documents | T05, T06, T07 |
| HTTP/MCP/visualization factories | directly injected owner adapters, mappers, projection behavior, tool dispatch | host/session selection, cancellation authority, connection/sink resource | T18 (adapters), T28 (Sessions getters), T24 (dead MCP wrappers) |

### Dependency and mutation rules

Break Sessions/Definitions/Work cycles using narrow independently constructed session-state/resolver capabilities, not setter injection. Break Models host/leases cycles by extracting slot facts and capacity/idle coordination into independently injected state/query/coordinator owners; remove both adapter.host assignment and BindCoordinator. Break Automations reconciler/parent callbacks using an injected source-lifecycle driver backed by source state. Preserve public owner roots and do not publish private stores.

Recordings remains canonical for Factory Events; Events remains process-local source-native delivery. Workers/Providers outputs re-enter Runtime as events. Sessions owns selection/publication, response sequencing, durable lifecycle, and generation routing. Models owns scopes/host/capacity state; Automations owns source/cursor state. Activation publishes only complete initialized generations; failed cleanup remains owned and retryable. Replacement must reject stale projections, route new work to the current generation, and preserve ordered replay history. A shared instance host owns keyed handles; sharing must not merge their cancellation, lifecycle state, or cleanup.

### Constructor requiredness

Required service/effect parameters are fixed, direct arguments. No variadic dependency defaults, dependency-bearing functional options, `EnsureLogger`/`Ensure(clock)` calls, nil-to-real/no-op substitution, or nil-gated collaborator invocation belong inside strict internal constructors. Validate supported caller effects and capabilities once before invoking Wire, including typed-nil interfaces and nil function effects; resolve absent optional overrides there. Retain validation of domain input, resource-opening errors, optional request selections, and existing configuration. No new panic contract is needed. Select explicit disabled publishers/observers/diagnostics where disabling is valid. A nil Providers override means inherit the default under the existing request contract; it is not an unavailable-provider implementation.

T23 owns boundary normalization/requiredness and T24 owns cross-cutting platform helpers plus retirement of unused compatibility constructors. Every behavior lane removes its own fallback and redundant requiredness guards as it migrates. [lint.md](lint.md) defines the exact enforcement rules, exclusions, known limits, fixture matrix, and progressive adoption using existing Make/CI targets. Automations durable-cursor construction currently falls back to memory on error (unreachable today): T12 preserves that behavior. Making durable-cursor failure explicit is a separate follow-up outside this program (operator decision, 2026-10-02), not a task in this plan.

### Shared process effects and legitimate specialization

- Default process timestamps, scheduling, artifacts, Recordings, Chat Sessions, provider command timing, staging, and transport timing project one normalized source. Explicit specialized overrides retain precedence until their callers migrate.
- Replay/logical `SetTick` time remains scoped and governs replay-sensitive facts. Real OS/network safety deadlines remain an explicit process scheduler role. They must not inherit replay tick progression accidentally.
- `Now`, `NewTimer`, and `After` form the one shared process `TimerSource` seam (operator decision: T01 adds `After`, with a deterministic implementation; no per-owner `After` adapters). Where a consumer needs `AfterFunc` or clockwork ticker/sleep behavior, replace its requirement with the smallest owner-local scheduling interface over that seam; do not fake missing methods.
- A process Zap backend and one `logging.Logger` projection are injected. `.Named`/`.With` correlation and per-session sinks remain legitimate derivations. Invocation-selected terminal formatting, verbosity, and quiet policy remain local; never mutate a shared logger's level/output on each concurrent invocation.
- Metrics, retry scheduling, host clocks, and diagnostic adapters may have different interfaces while sharing the same underlying origin. Pointer equality of wrappers is not a success measure.

## 7. Failure modes and quality attributes

| Case | Detection | Customer outcome | State/recovery | Telemetry | Evidence |
| --- | --- | --- | --- | --- | --- |
| Invalid Factory/Work/model input | Existing owner validation | Existing typed error, no dispatch | No admitted mutation | Safe operation outcome | F02, F09, F13 |
| Permission denied | Existing provider/API policy | Same denial; no hidden retry/bypass | Attempt terminal; capacity released | Redacted failure classification | F05, owner unit denial cases |
| Provider timeout/outage | Controlled runner failure and selected scheduler | Existing failure/timeout | Release attempt/worktree/leases; retain canonical outcome | Attempt/session IDs, duration | F05, F09 |
| Partial startup/resource failure | Injected failing resource boundary | Start fails; no live incomplete generation | Close acquired resources once; joined errors; retry possible | Start/cleanup failure | F03 |
| Replacement failure | Existing activation commit boundary | Prior generation remains usable before commit | Restore prior sidecars; retire only committed generation | Generation/replacement outcome | F04 |
| Concurrent sessions | Explicit identity and route observation | Independent Work and ordered response streams | No cross-session cancellation/state | Session/generation correlation | F06 |
| Cancellation and process shutdown | Public controls and readiness/terminal observation | Terminal response and graceful stop | Owned resources stop/join/flush; other sessions survive scoped cancellation | Cancellation/cleanup cause | F05, F07, I01 |
| Capacity exhaustion | Model/worker lease owner | Existing capacity error/queue behavior | No leaked lease; retry after release | Capacity/rejection metrics | F09 |
| Recording persistence failure | Injected write/flush error | Existing surfaced recording error | Preserve durable facts; explicit retry/recovery | Artifact identifiers, safe path diagnostics | F07, F08 |
| Automation cursor repository construction failure | Controlled repository setup error | Existing startup continues using memory; do not change this policy in this program (explicit failure is a separate follow-up) | Preserve characterized memory fallback and restart behavior | Capture actual existing diagnostics/visibility before migration | F10, T12 unit characterization |
| Automation cursor write failure | Injected durable cursor commit error | Preserve source-specific existing failure observation/status | Do not claim durability for an unsuccessful commit; preserve current retry/cursor outcome | Existing source/cursor diagnostics | F10 and owner unit cases |
| Replay/restart/legacy compatibility | Public replay/resume/read | Same history and logical identity | New live generation; no history rewrite | Recovery outcome | F08 |
| Incompatible clock capability | Composition validation | Construction fails actionably; never half-controls time | No started service | Required/missing capability | U01 |
| Logger/scheduler cross-session mutation | Captured logs/timing and race checks | Correct stream framing and attribution | Shared backend, isolated contexts | No secret payloads | U02, F06 |

### Performance and scale

T27 captures the baseline before structural work, using a dedicated `tests/stress` lifecycle profile at the pinned project base commit (the main commit that adds this plan). The baseline therefore stays valid whatever order the lanes merge in. T22 re-measures that commit on the final host if the environment differs. Capture 10 valid samples of inert construction plus 10 explicit-session start/stop samples, reporting median, p95, and full range. Final median/p95 must not regress by more than 10 percent, and repeated 100 sequential start/replace/stop cycles in the dedicated stress lane must return owned handles/leases to baseline; document timing noise rather than discard slow valid samples. Use four concurrent sessions for isolation proof, not a load claim. These are proposed project thresholds, not existing measurements.

### Reliability, security, privacy, and cost

No new retry policy, permissions, network targets, secrets, persistence ownership, or telemetry payloads. Preserve redaction and CLI output framing. New operation logs use session/runtime/generation/attempt identifiers and safe causes; do not log prompts, tokens, secrets, or raw provider output. Validation is free and controlled except bounded local CPU/disk integration; paid calls budget is zero. No new production alert infrastructure: existing failures and invalid composition are stop signals for rollout.

## 8. Rollout and rollback

Each lane first characterizes missing invariants, introduces focused providers/operations alongside the old interface, makes the new path canonical, migrates its callers, and removes its old path. A compatibility constructor may temporarily delegate to the new implementation in the owning `wire` package; it must not introduce a second graph or execute alongside the new path. No public feature flag or split-brain runtime selection is required.

Compatibility interval: existing external clock override fields remain accepted for this program; explicit override beats process default and is documented as intentional specialization. Removing an externally used field/provider requires a separate compatibility decision and native before/after shape. Internal adapters expire in their owning lane's terminal PR. Every PR leaves main releasable and is independently revertible; rollback is its code revert without data migration. Stop on event/response ordering drift, lost correlation, unexpected real-time waits under a selected controllable scheduler, leaked resources, startup effects during construction, or altered quiet/output behavior.

## 9. Implementation strategy and parallel work

### Coverage assessment and executable spine

Existing source anchors: Runtime root composition and mixed dispatch scenarios; Events concurrent response isolation and replacement; Factory replay contract scenarios; Models invocation/capacity/repeated-scope scenarios; Automations cron/hosted/watchers; Recordings replay and flush; owner unit control/cleanup/recovery tests. Reading them is not proof they currently pass. Each lane runs its named focused suite on the base commit, maps criteria to existing cases, and lands any missing characterization before its restructuring PR. New cases are limited to the complete matrix in section 10 and task-local unit failures; report an extra discovered public behavior as a delta.

The spine is `root.BuildProcess -> Process.Execute -> explicit Factory Session -> admitted Work -> controlled provider command edge -> Factory Events/public result -> close/replay`. Every lane preserves it. T01/T02 are justified bounded enablers because process source selection is shared infrastructure independent of any single customer command; neither blocks unrelated constructor/state extraction.

### Maximum parallelism

Start T27 (the baseline harness) first. Then start T01–T10, T12, T13, T14, T16, T18, T19, T20, T23, T25, T26 and T30 independently, subject to available worker capacity. The remaining lanes wait on prerequisites:

- T11 on T10/T30.
- T31 on T14.
- T15 on T14/T16/T31.
- T17 on T13/T15.
- T28 on T17/T18.
- T21 on T01/T02.
- T24 on the lanes whose callers it retires (T03, T08–T12, T23, T26).
- T29 on every owner lane.
- T22 on T27 and the converged lanes.

A factory DEPENDS_ON gates a whole lane. Work that could start earlier was therefore split into its own task instead of being left as a partial dependency. T25/T26 preserve the Costs query and Webhooks delivery through the existing peer contracts, so they run independently of the other owner lanes. There is no all-repository audit/characterization/contract PR prerequisite and no last giant cleanup task.

T01/T02 publish additive effect/provider interfaces early; owner lanes may implement against unchanged narrow clock/logger contracts and adopt the normalized defaults after those seams land. Such adoption is a small integration milestone inside that lane, not a dependency that stops its other development. Model scope, Automations, Definitions, Recordings, Providers, Workers, Settings, and reader lanes retain their public peer contracts and therefore do not depend on each other's implementation order.

### Shared surfaces

| Surface | Owner | Coordination rule |
| --- | --- | --- |
| `pkg/wire/wire.go`, `wire_gen.go`, canonical sets | Composition steward, implemented as part of each lane PR | Owners supply focused provider signatures/registration delta; steward integrates/regenerates against current head. Never queue all registration until a final task. |
| `edges/definition.go`, merge behavior, Platform clock | T01; T02 supplies only logger delta to T01 owner | Additive seams; small separately reviewable patches; no conflicting global rewrite. |
| Broad `pkg/wire/profiles.go`, `session_runtime_providers.go`, `models_runtime.go` | Owning lane for named functions | Move only the lane's functions. `make pkg-file-count` is exact and deletion-only: `pkg/wire` is pinned at 50 files in `docs/internal/baselines/backend-package-file-count.json`, and packages not listed there may hold at most 15. New providers go into existing files or owner `wire` packages. A file deletion lowers its baseline entry in the same PR. Concurrent PRs edit the same JSON, so every lane follows the §13 merge-safety rule (rebase onto origin/main, re-run `make lint pkg-file-count`, plus `make generate-wire` + diff when Wire changed). Resolve shared-file edits mechanically; contention is not a semantic edge. |
| Sessions `factory.go`, `open.go`, root binding/state | T13 registry/leaf construction; T17 opening cutover | T13 supplies stable leaf/resolver roles. T17 owns final opening/port deletion. |
| Runtime root/build/host | T14 fixed root/handle authority; T15 scoped activation | Preserve Root's maps/locks in T14. T15 removes builders after handle authority is available. |
| Existing tests and support | Behavior lane owner | Edit only owned cases/helpers; shared support changes have one named author and behavioral review. |
| Worker Sessions construction, Worker Sessions HTTP handler wiring, `pkg/wire` (overlap with the concurrent worker-session visibility/controls project, `docs/internal/development/plans/backlog/worker-session-visibility-and-controls.md`) | This plan: T16 (`worker_sessions` construction), T18/T28 (HTTP handler wiring), T03 (Recordings wiring), composition steward (`pkg/wire`) | Operator decision (2026-10-02): flat-injection owns construction shape for `worker_sessions`, HTTP worker-session handler wiring and `pkg/wire`. Visibility tasks that add constructor dependencies or handler wiring wait for T16, T18/T28 (and T03 for Recordings wiring) to merge. Flat-injection does not wait for visibility. |
| Static checks | T20 (infrastructure, per-set enablement); T29 (final repository scope) | Install/update rules per migrated owner; lane owners remove their violations. Never enable a set while an open lane PR still edits it; ratchet/baseline merges follow the §13 merge-safety rule. |

Parallel branch development is allowed, but integrated customer behavior must use the canonical graph. Private Go implementation access remains constrained; focused aliases belong in owner `wire`, not new public peer service bags.

## 10. Verification strategy

| Gate | Scope | Fidelity | Cadence | Cost | Proves | Does not prove |
| --- | --- | --- | --- | --- | --- | --- |
| U01 time projection/precedence/capability | Unit | Controlled | Each time change | Free | Selected clock methods/timers and override validation | Whole journey or OS deadline behavior |
| U02 logging routing/redaction | Unit | Controlled capture | Each logging change | Free | Shared backend routing, correlation, isolation, no secret output | Production collector availability |
| U03 owner behavior suites | Unit | Controlled | Each lane | Free | Validation, state transitions, cleanup, leases, replacement | Full process composition |
| F01-F18 below | Functional | Controlled command/filesystem/network edges | Each owning lane PR | Bounded local | Public behavior through composed application | Real paid provider/network availability |
| S01 existing and extended construction checks | Lint/static | Import-aware AST analysis | Each PR | Free | Constructor placement, direct dependency edges, no bags/default bypass in migrated owners | Runtime success |
| G01 `make wire-smoke` | Generation + component checks | Controlled | Each graph PR | Bounded local | Reproducible Wire output and focused composition behavior | Public journey alone |
| G02 `make verify-fast`, `make lint`, `make verify-pr` | Repository gates | Mixed controlled/local | PR | Bounded local | Compile/lint/boundaries and their actual tested properties | Unmeasured project criteria |
| I01 prebuilt shutdown smoke | Integration | Local real OS | Final and lifecycle-risk PR | Bounded local | Compiled CLI readiness, graceful signal/stop, recording flush | Exhaustive customer matrix |
| P01 dedicated lifecycle performance/stress | Load/stress | Controlled/local | Base (T27, pinned base commit) and final (T22); risk-triggered | 10 min maximum, 100 sequential cycles | Proposed thresholds and lifecycle resource retention | Fleet-scale throughput |
| VAL01 clean environment journey | End-to-end/loopback | Controlled providers, real local delivered entry | Final | No paid calls | Cross-lane integration and criteria | Billable provider availability |

### Complete functional case matrix

These are selected distinct behaviors; input-validation permutations stay in unit tests. Reuse existing cases where they prove the condition.

| ID | Kind | Given | When | Then | Owner |
| --- | --- | --- | --- | --- | --- |
| F01 | Happy | Controlled effect observers and valid process input | BuildProcess constructs the process | No runtime/sidecar/provider execution occurs before selected lifecycle | Every graph lane |
| F02 | Happy/unhappy | Valid and invalid Factory/Work inputs | Operator loads/saves/submits | Same public result or typed validation failure; no invalid dispatch | T04/T05 |
| F03 | Unhappy/boundary | Resource opening fails once | Start then retry explicit session | Failed start is not live; retry succeeds; owned resources released | T13/T15/T17 |
| F04 | Happy/unhappy | Active session and retained response cursor | Replace successfully, then fault a later replacement | Cursor/event history survive; failed precommit replacement keeps prior generation usable | T15/T17 |
| F05 | Happy/unhappy | Controlled provider success, denial, timeout, and cancellation | Invoke Worker/direct child/Factory Work and cancel one | Same terminal classification; released worktree/attempt; peer session survives | T08/T09/T16 |
| F06 | Happy/boundary | Four explicit sessions with owned inputs/routes/streams | Overlap Work, reads, and one cancellation | Outputs/events/log correlation and cursors remain attributable to each session | T13/T15/T16/T17 |
| F07 | Happy/unhappy | Selected recording path and injected flush failure | Stop after Work completion or cancellation | Durable history readable on successful flush; flush failure is surfaced safely | T03/T17 |
| F08 | Happy/boundary | Recorded history and representative legacy fixture | Export/read/replay/resume through public surface | Same admitted facts, history, logical identity and typed legacy failure inspection | T03/T15/T17 |
| F09 | Happy/unhappy | Two model scopes and controlled inference success/failure/capacity | Invoke/repeat/cancel in either scope | Same result/error, no cross-scope config leak; capacity released for next invocation | T10/T11 |
| F10 | Happy/unhappy | Controlled source scheduler and cursor store | Trigger cron/script/hosted/watch event; stop/restart; fault cursor write | Expected Work admission/cursor behavior; no post-stop admission or peer cancellation | T12 |
| F11 | Happy/unhappy | Fixture Codex/Cursor transcripts and unavailable storage | CLI inspects Provider Sessions | Same discovery/transcript result or safe error | T06 |
| F12 | Happy/unhappy | Operator settings with unknown fields and unavailable prerequisite | Read/write/resolve settings | Preserve unknown fields/default precedence; same safe failure | T07 |
| F13 | Happy/unhappy/boundary | Explicit live session or expired/missing target, retained/live SSE events and gap | HTTP reads/actions/SSE, MCP/ACP invocation | Existing request/result/error, typed gone/gap, ordering and close boundaries | T18 |
| F14 | Happy/boundary | Concurrent quiet, normal, verbose/debug invocations | Run/read through CLI with controlled logs | Correct stdout/stderr/NDJSON framing; one invocation cannot change peer output policy | T02/T19/T21 |
| F15 | Happy | Selected controllable process source; no specialized override | Drive admission/artifact/recording/chat timestamps and scheduling | Public/edge-observed facts follow selected source and scheduling progression | T01/T21 and affected lanes |
| F16 | Happy/boundary | Explicit specialized legacy clock and replay clock | Execute their owning operations | Specialized override wins; replay facts follow ticks; OS cleanup remains governed by scheduler | T01/T15/T21 |
| F17 | Happy/unhappy | Controlled prices, Settings and canonical usage; unavailable price/metrics edge | Query Costs through CLI/HTTP | Same deterministic scoped report or typed failure; redacted attributed diagnostics | T25 |
| F18 | Happy/unhappy | Two explicit recorded sessions and controlled webhook success/retry/outage/secret/store failure | Activate subscriptions, admit Work, stop one | Correct event signatures/delivery/retry/dead letters; peer delivery survives; no pre-activation history | T26 |

### Distinct selected failure witnesses

The parent rows route ownership; the following subcases are separate Given/When/Then witnesses, not one parameterized functional case claiming several outcomes. Keep lower-level input permutations in owner unit tests.

| ID | Given | When | Then | Owner |
| --- | --- | --- | --- | --- |
| F05a | Controlled successful provider command and admitted Work | Worker/direct/Factory invocation completes | Successful terminal output and canonical outcome; attempt/worktree released | T08/T09/T16 |
| F05b | Policy-denied attempt fixture with preserved characterized public denial | Invoke without authorized capability/permission | Existing denial, no unauthorized command effect or bypass/retry; no leaked attempt/resource | T08/T09 |
| F05c | Controlled command exceeds selected deadline | Advance scheduler through timeout | Provider-normalized timeout (ErrExecuteTimeout at owning boundary), existing public timeout mapping, released resources | T08/T09/T16 |
| F05d | Two running explicit sessions | Cancel one invocation through public control | Provider-normalized cancellation (ErrExecuteCancelled at owning boundary), existing terminal event, only selected resources stopped; peer completes | T08/T09/T16 |
| F09a | Two model scopes with distinct valid config | Invoke each successfully and repeat | Correct scoped result/config/asset selection and capacity reusable | T10/T11 |
| F09b | Controlled host/inference failure after lease acquisition | Invoke model | Existing staged model failure result; one release attempt/disposition, no false success or cross-scope change | T10/T11 |
| F09c | Model slot capacity occupied | Acquire/invoke same slot, then release existing holder and retry | Existing capacity-exhausted/contended classification (ErrHostCapacityExhausted/ErrHostCapacityContended where applicable), no second allocation, next eligible attempt succeeds | T10/T11 |
| F09d | Two model scopes, one active invocation | Cancel one and advance its scheduler/cleanup | Its lease/process disposition follows existing policy, other scope survives, subsequent eligible invocation can acquire capacity | T10/T11 |
| F18a | Two explicit event scopes, valid secret and successful endpoint | Activate at known cursor and admit later Work | Correct signed canonical body/event ID/timestamp; no pre-activation history or peer event delivery | T26 |
| F18b | Endpoint returns retryable failure once then success | Advance selected retry scheduler | Same event identity/body follows configured backoff, delivery succeeds, no terminal dead letter | T26 |
| F18c | Endpoint remains retryable through configured maximum attempts | Exhaust retry policy | Exactly the configured attempt outcome followed by redacted terminal dead letter with retry_exhausted; peer subscription survives | T26 |
| F18d | Selected signing-secret resolver returns error | Activate endpoint and observe its worker | No HTTP delivery, existing redacted secret-resolution diagnostic; subscription close joins resources and peers survive | T26 |
| F18e | Terminal delivery failure and injected dead-letter append error | Exhaust delivery and attempt persistence | Existing append-failed diagnostic; no claim of persisted dead letter; no recursive retry storm or peer cancellation | T26 |
| F18f | Two endpoint subscriptions are live | Stop one session subscription | No further admissions/delivery from closed scope after join; peer still delivers, handles released | T26 |

Source semantics for these classifications were inspected in Providers execute_contract.go, Models host_contract.go/leases, and Webhooks service.go/delivery.go/dead_letter.go. Functional cases observe public outcome or supported command/HTTP/log/storage effect; sentinel assertions belong at the owning unit boundary when transports do not expose them.

### Test layer design and execution

Functional cases enter through CLI `Process.Execute` by default. HTTP/MCP/ACP cases own explicit protocol/parity behavior. Share one `root.BuildProcess` per package when safe; use explicit scenario-owned Factory Sessions allocated before opening, profiles, directories, routes, streams and fake state. First-run profile initialization completes before sharing a profile; independent concurrent commands otherwise own distinct profiles. No real user environment mutation, built CLI, sleeps, global locks spanning invocations, or internal constructor-count assertions. Observe public results/events/replay or supported external effects.

Focused commands are listed per task in [tasks.md](tasks.md). Broad functional evidence uses `make test-functional`/the existing bounded `cmd/functionallane`, not raw wildcard fan-out. Focused runs use `go run ./cmd/functionallane -root ./tests/functional/<dir>/...`. The lane has no `-run` flag and defaults to `-short=true`: `SkipLongFunctional` cases need `-short=false`, and `functionallong`-tagged replay cases need `make test-functional-long`. Race runs target changed owner packages with bounded package concurrency. Source-topology tests currently in `pkg/wire` move to S01; their passing is not customer evidence. I01 consumes an artifact built by the invoking build/release lane; no test compiles its own executable. P01 is separate from functional/unit/integration suites. Paid validation is not applicable: maximum calls/cost are zero; real provider availability is outside this refactor's claim.

Remaining unproven edges: OS signal/pipe behavior -> I01; aggregate retention/performance -> P01; integrated ordering/recovery/output policy -> VAL01; remote endpoint availability -> existing authorized release gates, not this plan. CI evidence must come from the change's own PR and reside in its PR comment.

## 11. Task dependency graph

| Task | Semantic prerequisites | Concurrent work |
| --- | --- | --- |
| T27 lifecycle performance baseline | None; schedule first | All; measures the pinned base commit |
| T01 process time; T02 process logging | None | All independent owner extraction lanes |
| T03 Recordings; T04 Definitions (incl. System Initialization installer); T05 Work; T06 Provider Sessions; T07 Settings; T08 Providers; T09 Workers | None | Each other; preserved public peer contracts |
| T10 Models leaf providers | None | Other owners |
| T30 Models slot state / capacity coordinator | None | T10 and other owners |
| T11 Models scoped state | T10, T30 | Runtime/Sessions/Automations |
| T12 Automations | None | All independent owners |
| T13 Sessions directory/streams leaves | None | T14/T16 and other owners |
| T14 Runtime control/host authority | None | T13/T16 and other owners |
| T31 Runtime instance-host build leaves | T14 | T13/T16 and other owners |
| T15 Runtime activation/replacement/replay | T14, T16, T31 | T11/T12/T13 and other owners |
| T16 Worker Sessions keyed supervision | None | T09/T13/T14; existing Workers contract |
| T17 Sessions opening/products cutover | T13, T15 | T18 and other owners |
| T18 HTTP/MCP/visualization direct adapters | None | All owners |
| T28 Sessions getter removal from HTTP/visualization | T17, T18 | Other owners |
| T19 CLI direct roles | None | All owners |
| T20 static enforcement infrastructure and per-set enablement | None | Report legacy first; enforce completed owners progressively |
| T21 cross-surface effect adoption (`pkg/wire` defaults only) | T01, T02 | Other owners; coordinate named provider functions |
| T23 boundary normalization/requiredness | None; coordinates additive seams with T01/T02 | All owner lanes; no all-repository constructor rewrite |
| T24 platform helper/compatibility retirement | T03, T08, T09, T10, T11, T12, T23, T26 | Production user call sites owned by their lane |
| T25 Costs query; T26 Webhooks delivery | None | Existing public price/metrics and recording contracts retained |
| T29 final repository-scope static enforcement | T01–T19, T21, T23–T26, T28, T30, T31 | None |
| T22 integrated performance/release evidence | T27 and T01–T19, T21, T23–T26, T28, T30, T31 | T29 |
| VAL01 independent read-only loopback | T01–T31 terminal criteria | Final reviewer; no implementation work |

Development work on a dependent task may begin using its proposed contracts; only canonical cutover waits for its semantic prerequisites. T21 owns artifact/chat/CLI protocol/recording planner default adoption; each owner lane owns adoption inside its service provider. This prevents a cross-repository effect sweep from conflicting with every lane.

## 12. Tasks

The self-contained canonical task packets are in [tasks.md](tasks.md). Each names the parent behavior, source-plan reference, exact scope, dependency graph, evidence, rollback, removal owner, and applicable [contracts.md](contracts.md) pairs. Any missing local characterization lands before that packet's restructuring. The composition steward integrates each lane's small Wire delta as part of its PR, not as a terminal integration project.

## 13. Project acceptance criteria

- [ ] F01-F18 preserve named public successes, failures, ordering, persistence, isolation, and output behavior; record case-level evidence against the changed head.
- [ ] U01/F15/F16 prove process source propagation, override precedence, controllable scheduling, and intentional replay/OS separation.
- [ ] U02/F14 prove canonical Models/host diagnostics, shared backend origin, scoped correlation, redaction, and concurrent output-policy isolation.
- [ ] S01 reports no service constructor execution during runtime operations, dependency bags/service getters used as locators, recursive service-tree providers, or hidden clock/logger defaults, constructor dependency substitution, or redundant requiredness guards in completed lanes. Every inventory item has a verified terminal disposition.
- [ ] G01 confirms reproducible generated Wire; boundary/cycle/constructor/logging checks and relevant G02 gates measure their properties on each change's PR.
- [ ] P01 meets the declared median/p95 and retention thresholds with honest baseline/final ranges; I01 proves delivered CLI shutdown/flush.
- [ ] VAL01 independently validates the composed journey from a clean environment and emits the canonical structured report without silently fixing defects.
- [ ] Implementation-stage delivery criterion: The implementation stage marks this criterion satisfied and stops after its final head is pushed, the PR is open, CI has started, and all blocking review feedback is addressed. It does not poll or re-check CI after this finish line. The review stage owns driving CI to terminal-and-passing, resolving merge conflicts, and merging the PR; merge remains the lane-wide delivery boundary. Immediately before merge, the review stage rebases onto current origin/main and re-runs `make lint pkg-file-count` (plus `make generate-wire` and a clean diff check when Wire changed) on the rebased head; merging a stale-green head is forbidden (operator decision, 2026-10-02). Enabling GitHub "require branches to be up to date" is a repository-settings decision outside this plan. CI-run evidence goes in a PR comment and never in a commit.

## 14. References

- [Planning standard](../../../../../factory/docs/standards/planning-standards.md), [plan template](../../../../../factory/docs/standards/plan-template.md), [task template](../../../../../factory/docs/standards/task-template.md), [testing standard](../../../../../factory/docs/standards/testing-standards.md), [loopback template](../../../../../factory/docs/standards/validation-loopback-template.md): normative planning/delivery format.
- [Backend standard](../../../standards/code/general-backend-standards.md): direct single injection, constructor ownership, explicit state and logging.
- [Package architecture](../../../../architecture/packaged-structure.md), [structures](../../../../architecture/structures.md), [ownership rationale](../../../../architecture/service-ownership-rationale.md), [data model](../../../../architecture/data-model.md): durable ownership, private implementation and public vocabulary.
- [Inventory](inventory.md), [contracts](contracts.md), [tasks](tasks.md), [lint policy](lint.md): concrete source findings, target shapes and execution packets.
