# Flat dependency injection and shared process effects — flat-injection-v1.1

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
| Work/Provider Sessions/Settings composite roots | state access, request/content preparation and invocation-input preparation; Codex/Cursor readers; document and resolution owners | work input, transcript reads, settings documents | T05, T06, T07 |
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

Start the read-only FI-PREREQ-BASELINE-OBS prerequisite before T27 (the pinned baseline harness). Then start T01–T10, T13, T14, T16, T18, T19, T20, T23, T25, T26 and T30 independently, subject to available worker capacity. The retained T12 successor consumes merged AM17 before planning (AM16 is already merged, not a live Work dependency) and continues from #2683/head `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`, with no T20/WSV wait. The remaining lanes wait on prerequisites:

- T11 on T10/T30.
- T31 on T14.
- T15 on T14/T16/T31.
- T17 on T13/T15.
- T28 on T17/T18.
- T21 on T01/T02.
- T24 on the lanes whose callers it retires (T03, T08–T12, T23, T26).
- T29 on every owner lane.
- T22 on T27 and the converged lanes.

A factory DEPENDS_ON gates a whole lane. Work that could start earlier was therefore split into its own task instead of being left as a partial dependency. T25 preserves the Costs query through existing peer contracts. T26 characterization can proceed independently, but replacement cutover remains held on FI-PREREQ-WEBHOOK-READINESS; integrated T18 discovery acceptance waits on FI-PREREQ-MCP-DISCOVERY. There is no all-repository audit/characterization/contract PR prerequisite and no last giant cleanup task.

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


### Authorized amendment: flat-injection-v1.1

Operator authority: 2026-10-03T06:20Z, `docs/temp/projects/flat-injection/rules.md`; correction packet `docs/temp/projects/flat-injection/reconciliation/checkin-e4da1723-20261003/authorized-amendment-corrections.md`. These private Project artifacts are not edited. This revision corrects dispatch documentation only; section 13 and immutable request/acceptance/source-plan bytes remain unchanged. Source locators below refer to the audited candidate base `02c60f888612779f831d3268463ff1f4f7a8b78c`, except AM10 which explicitly uses pinned baseline `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`; prior packet locators are historical witnesses, not final edited line numbers. Follow each row to its named task, native pair in contracts.md, owner in inventory.md, exclusions in lint.md, and remaining gate in validation-review.md. No row certifies production acceptance.

| ID / task | Conflict and canonical file:line evidence | Resolution or precise hold | Unchanged criterion | Accountable owner | Release event |
| --- | --- | --- | --- | --- | --- |
| AM01 / T02 | Invalid quiet+JSON success prerequisite and obsolete OS-stderr logger risk. `docs/internal/development/plans/flat-injection/tasks.md:185,190-198; pkg/services/factory_runtime/transports/cli/invocation_error.go:240; docs/reference/run.md:535; tests/functional/transport/cli/output/json_result_test.go:121; pkg/platform/logging/logger.go:38-52; tests/functional/models/model_invoke/cli_test.go:269-281` | Specify separate simultaneous quiet, JSON, response-stream NDJSON, normal and verbose/debug invocations. Preserve quiet+JSON/explicit-output rejection, empty stdout with typed model-failure stderr, no success artifact and capacity recovery. Describe terminal-muted default logging; retain capture/redaction/isolation characterization before migration. | FI-A1/F14, FI-A3/U02, FI-A8 | T02; T19/T21 consume the output witness | Amendment merge then focused characterization before T02 restructuring |
| AM02 / T05 | Work root injects request preparation but omits invocation preparation and its second reusable layer. `docs/internal/development/plans/flat-injection/tasks.md:478-487; pkg/services/work/internal/service.go:99,177-180; pkg/services/work/invocation_return_policy_contract.go:323-353; pkg/services/work/admission_contract.go:55` | Add direct RequestPreparationService and InvocationInputPreparation to Work root. Construct the private invocation-input policy and its public mapping adapter once in focused owner Wire providers. The adapter stores the completed inner policy. Preserve cancellation/error mapping, admission validation and completed-flush confirmation. | FI-A1/F02, FI-A4/S01 | T05 | Amendment merge then flush characterization before structural cutover |
| AM03 / T07 | Canonical HTTP/MCP settings load/update routes are absent. `docs/internal/development/plans/flat-injection/tasks.md:717-725; tests/functional/operator_settings/root_composition/transport_activation_test.go:127; pkg/wire/profiles.go:316,617-633; api/openapi-main.yaml:1` | Use existing canonical CLI settings unknown-field round trip, precedence and safe-failure proof. Classify dormant HTTP/MCP adapter coverage as component unit evidence, never canonical activation. No new routes. Review complete authored OpenAPI and import/binding reachability before assigning absence. | FI-A1/F12, FI-A4 | T07 | Amendment merge; canonical CLI characterization, then migration |
| AM04 / T08 | Canonical AGY command selection contradicts mandatory PTY launch. `docs/internal/development/plans/flat-injection/tasks.md:828-832; pkg/wire/session_runtime_providers.go:202-214; pkg/wire/runtime_provider_bindings.go:26-39; pkg/wire/agy_pty_test.go:1` | Characterize supplied command runner execution and zero PTY launches through the canonical process. Separately characterize absent-override inert construction/default command composition without launching the real provider. Retain owner-unit legacy PTY compatibility. No runner-selection change. | FI-A1/F01/F05a-d, FI-A4 | T08 | Amendment merge; command-selection characterization before restructuring |
| AM05 / T09 | Attempt cleanup incorrectly implies unconditional release of retained Factory checkout. `pkg/services/workers/internal/service/execute.go:438-441; pkg/services/factory_runtime/internal/services/orchestration/runtime/invoke_worker.go:409-414,479; docs/internal/development/plans/flat-injection/plan.md:260` | Preserve RetainWorktree and reused checkout behavior. Distinguish attempt resource release from checkout lifetime; T09 owns nonretained attempt checkout release, Runtime T15 owns session/generation resource disposition under existing retention semantics. Keep F05a observable completion and cleanup; hold any demand to delete retained checkout for operator policy decision. | FI-A1/F05a-d, FI-A4, unchanged plan §13 | T09 with Runtime T15; operator only if lifetime policy changes | Amendment merge; characterized retain/nonretain/reuse cases; no unconditional deletion claim |
| AM06 / T10/T11/T30 | ProcessDependencies removal assigned to two incompatible stages; T30 host/lease work duplicated. `docs/internal/development/plans/flat-injection/tasks.md:1042-1046; docs/internal/development/plans/flat-injection/inventory.md:41; docs/internal/development/plans/flat-injection/contracts.md:76,117-141; pkg/services/models/internal/service/runtime_factory.go:48,68,109-155` | T10 introduces fixed leaf providers and records ProcessDependencies as temporary compatibility retained, removal owner T11. T11 migrates root/scoped execution callers and removes the declaration/forwarding/consumption. Temporary status is not final S01 satisfaction. Keep T30 exclusive slot/lease/coordinator ownership and current-main terminal rows. | FI-A1/F09a-d, FI-A4/S01 | T10 interim; T11 final; T30 host/leases exclusive | T11 cutover only after T10 and T30 merge; T29 checks final zero unresolved findings |
| AM07 / T13/T17 | Gateway/runtime/invocation cycle lacks replacement contracts; BindProcessDurable overwrites gateway. `docs/internal/development/plans/flat-injection/contracts.md:354-427; pkg/services/factory_sessions/internal/sessionservice/assembly.go:415,457-476; pkg/services/factory_sessions/internal/sessionservice/assembly_durable.go:18; pkg/services/factory_sessions/internal/service/root.go:111-124; pkg/services/factory_sessions/internal/sessionservice/runtime_invocation.go:21-74` | Construct independent session authority first, then keyed scope activation/control, durable owner, invocation and gateway. Invocation timeout calls scope control rather than gateway. Gateway receives durable execution directly; remove BindProcessDurable replacement in T13 with the minimal T17 bridge caller adjustment. T13 owns authority/gateway/invocation leaf cut; T17 removes legacy Complete/opening/service products and durable factories after T13/T15. Keep clock, durable history, response sequencing, logical identity and failed-start rollback. | FI-A1/F03/F04/F06/F07/F08, FI-A4 | T13 leaf contracts and binding removal; T17 final opening migration | Amendment merge; F03/F06 characterization; T17 cutover after T13/T15 |
| AM08 / T14 | Now-only instance-host clock cannot schedule WaitForStart. `docs/internal/development/plans/flat-injection/contracts.md:474; docs/internal/development/plans/flat-injection/inventory.md:213; pkg/services/factory_runtime/clock.go:6-8; pkg/services/factory_runtime/internal/host/lifecycle.go:169-171; pkg/services/factory_runtime/internal/host/service.go:16-35` | Inject process TimerSource separately from replay-sensitive Clock into instance host and lifecycle owner. Propagate scheduler through canonical owner Wire and readiness helpers only; replace readiness ticker/deadline with cancellable NewTimer loops under existing durations. No replay ticks govern OS waits, no hidden real-clock fallback, no broader T15/T31 build rewrite. | FI-A2/U01/F15/F16, FI-A1/F06, FI-A4 | T14 with composition steward | Amendment merge; scheduler unit/readiness proof before control cutover |
| AM09 / T16/T15 | Factory declaration/provider/call-site deletion contradicts dependency order. `docs/internal/development/plans/flat-injection/tasks.md:1626,1689,1732; docs/internal/development/plans/flat-injection/contracts.md:550; pkg/services/factory_runtime/composition_contracts.go:84; pkg/services/factory_runtime/internal/build.go:400; pkg/wire/worker_sessions_providers.go:26` | Assign declaration, provideWorkerSessionsFactoryWithRecorder and call-site removal together to T16 after its separately merged keyed-attempt contract PR and characterization. T15 consumes shared supervisor and remains dependent on T16. Do not write or approve the pending keyed-attempt design in this amendment; preserve its first-step own-PR gate and Runtime/session overrides/clock/retention obligations. | FI-A1/F05/F06, FI-A4 | T16 removal; T15 consumes | T16 keyed-attempt contract PR merge, then structural PR merge, then T15 |
| AM10 / T27 | Pinned baseline cannot count private handles/leases through public session counts. `docs/internal/development/plans/flat-injection/tasks.md:2788,2807,2822; pkg/services/factory_runtime/internal/services/instance_host/internal/service/service.go:22; pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service/service.go:21-22 (pin 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6)` | Require a read-only owner-specific baseline observation prerequisite before P01. Count instance-host handles, Models lease records and capacity-holder values, plus goroutines, at quiescent lifecycle checkpoints. The prerequisite must deliver an external read-only observer runbook for the exact pinned artifact and prove private-map access, synchronization, nonmutation and matching timing methodology before T27 can report. Observation tool/seam feasibility is explicitly unresolved at that owning gate; this docs amendment does not claim it is proven. No production instrumentation, public-count substitute, alternate pinned commit or fabricated P01. Unsupported observation remains BLOCKED at that gate. | FI-A6/P01, FI-A4 | FI engineering baseline-observation prerequisite owner; T27 consumes, T22 compares | Amendment merge; independently reviewed read-only observation runbook/artifact proof on pinned source, then T27 |
| AM11 / T03 | Failing begin writer and successful zero-time finalization are false characterization. `docs/internal/development/plans/flat-injection/tasks.md:306-309; pkg/services/recordings/internal/lifecycle_capability.go:398-426; pkg/services/recordings/internal/services/recording_lifecycle/internal/service/service.go:360-384; pkg/services/recordings/internal/core.go:449-454` | Use cancellation after recording has started, with an injected final-flush failure and fixed UTC clock. Assert joined cancellation/flush causes and terminal metadata; absent clock yields ErrInvalidRecordingTerminalMetadata and unset FinalizedAt, never successful zero time. Preserve retained fi-t03-recordings-history-20261002 head 5725072a5179e7d5e28c869584a2fdb3a8635e29 and F07/F08. | FI-A1/F07/F08, FI-A2, FI-A4 | T03 | Amendment merge; corrected unit characterization before restructuring |
| AM12 / T18 | start_sync completion lacks returned-session ordered-event discovery. `docs/internal/development/plans/flat-injection/tasks.md:1956-1961; pkg/services/factory_sessions/transports/mcp/client.go:168-205; pkg/services/factory_sessions/transports/mcp/inspection.go:82-96; pkg/services/factory_sessions/transports/mcp/errors.go:21` | Name shared FI-owned Sessions/Recordings/MCP discovery prerequisite for FI and worker-session-visibility. Keep completion AND ordered events required (M01/F13). Record retained fi-t18-direct-transport-adapters-20261002 and its uncommitted start_sync_tool_test.go witness. Proposed prerequisite diagnoses same identity/recording lookup and public semantics; if correction needs policy/public-contract change, operator hold precedes implementation. T18 adapter development may continue independently; integrated acceptance waits on prerequisite. | FI-A1/M01/F13, FI-A7 | FI Sessions/Recordings/MCP prerequisite delivery owner; WSV consumer, not prerequisite owner | Amendment merge plus independently reviewed discovery prerequisite or operator decision; no inferred release |
| AM13 / T26 | Replacement activation has no webhook readiness; characterization cannot fix production. `pkg/services/factory_sessions/internal/service/open.go:449,616; pkg/services/factory_sessions/internal/sessionservice/runtime.go:118; pkg/services/factory_runtime/internal/services/instance_host/build/service.go:200; pkg/services/factory_sessions/internal/runtimebinding/binding.go:125; docs/internal/development/plans/flat-injection/plan.md:268-280` | Separate retained characterization, FI-owned Sessions/Runtime/Webhooks production readiness prerequisite, and future cutover. Preserve F18a-f and branches fi-t26-webhook-delivery-20261002 at b82919b6133c4903a6ff30db9d5ad0b11904bd9e and fi-t26-scoped-activation-characterization-20261003 at 720f3d19cbb4fda41b287cf052c5fb3e0a7f94f9. Readiness authority is held: this docs authorization grants no replacement webhook behavior or policy change. | FI-A1/F18a-f, FI-A4, FI-A7 | FI Sessions/Runtime/Webhooks prerequisite owner, production authority pending operator | Amendment merge does not release production hold; explicit authorization and prerequisite merge before T26 cutover |
| AM14 / T25/T06 | Author all-stories-pass gates demand independent validator results. `factory/workstations/process/AGENTS.md:45-52; factory/workstations/review/AGENTS.md:200-214,274-285; docs/internal/development/plans/flat-injection/tasks.md:543,642,2635,3051-3065` | Assign author evidence and push/open/CI-start/blocking-feedback handoff to implementation. Keep independent validation/quality/merge with reviewers and actual validation owners. Preserve T25 #2676 head 26cdf0749d99219b281e56afbb2fd22b2e0cb364, M01/M06/M07/M08/L25-7, exact-head L25-6 and real Linux/artifact obligations. Preserve T06 retained 784df0844baf076e589d386f53ba78570c06c8c2. Neither author report nor docs correction substitutes for validation. | FI-A5, FI-A7, FI-A8 | T25/T06 authors; independent VAL25/review; shared quality owner | Author handoff separately; exact-head independent result/applicable terminal CI/current-main premerge/merge release |
| AM15 / T01/T06 shared gates | Shared lane-audit evidence/release remains missing despite retained work. `factory/workstations/process/AGENTS.md:45-52,96-106; docs/internal/development/plans/flat-injection/tasks.md:124-143,627-642; Makefile:589-590; cmd/testlanecheck/main.go:1` | Keep shared quality prerequisite owned by Factory Reliability/quality-gate maintenance. Retain T01 b8960a415c4e63ce5bd5e5bd001068b5ca4692d7 and T06 784df0844baf076e589d386f53ba78570c06c8c2. Ordinary exact-head CI adjudicates quality; no local broad suite before PR, no exemption, repaired audit claim or invented terminal disposition. Required primary-lane coverage and fixture-isolation proof remain outstanding. | FI-A5/G02, FI-A8 | Factory Reliability/shared quality owner, independent reviewers | Real shared-lane release and terminal applicable PR CI; amendment alone does not satisfy |
| AM16 / T12 | Binding 2026-10-03T10:20Z decision supersedes 09:05Z hold; mailbox `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`; retained #2683/head `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7` | Approved C01–C16: separate lifecycle/recovery roots; runtime controls/scoped reads inline on existing Services; focused direct providers; same-PR stale baseline/obsolete-helper deletion. No allowance, baseline debt, public/persistence-policy change. | FI-A1/F10, FI-A4/S01, FI-A5/G02, FI-A8 | AM16 docs lane; T12 implementation/removal, T20 unchanged checker/measurement | AM16-DOC-VAL/REVIEW current-main merge before retained successor; then exact-head T12-F10/S01/G01/G02 and review/merge. No T20/WSV wait. |
| AM17 / T12 | F10b/g/j incorrectly require composed cursor reads: `tasks.md:1332,1337,1340` at author base `31751c9a52`; real service `automations/contracts.go:215–222,457–474`, durable cursor `script_pollers/internal/service/durable_cursor.go:74–108`, unbound adapter `automations/transports/http/convergence_operations.go:52–97`, canonical `root/process.go:17` / `initializer/application/process.go:185`. Binding mailbox Decisions 12:01Z A and 12:57Z. | Split evidence only: existing GetCursor component/service-contract exact opaque facts and stale conflict/no mutation/no extra command or Work; composed existing public Work, resume environment, error/diagnostic and peer isolation. Both F10b/g proofs required; no endpoint or second functional graph. See the AM17 ledger below. | Complete F10a–m/C01–C16; FI-A1–A8; one-interface/deletion-only/deadcode=0 and original delivery gates | AM17 docs author; retained T12 both runtime proofs; independent ordinary reviewer | AM17 author push/open/CI-start/feedback handoff; independent AM17-DOC-VAL/REVIEW current-main merge before successor planning. AM16 already merged; no historical Work dependency or T20/WSV wait. |

### Stage ownership and held prerequisites

| Gate | Owner and input | Release / remaining proof |
| --- | --- | --- |
| Amendment author | Six companions; AM01–AM15 source-to-task-to-gate rehearsal; DOC-TRACE, DOC-INVARIANTS, DOC-LINT | Push final head, open ordinary PR, start required CI, address blocking feedback. Stop; no runtime PASS. |
| AMD-VAL / AMD-REVIEW | Independent reviewer, isolated exact-head checkout; canonical validation-loopback report | Independent local-criterion PASS/FAIL/BLOCKED; terminal applicable CI, current-origin/main rebase, immediate premerge `make lint pkg-file-count`, then merge. No author report substitutes. |
| FI-PREREQ-BASELINE-OBS | FI engineering baseline-observation owner; pinned artifact and private counted owners in inventory.md | Independently reviewed read-only observer runbook proves map access, synchronization, nonmutation and comparable timing before T27; feasibility unresolved, otherwise BLOCKED. No public-count substitution or baseline production edits. |
| FI-PREREQ-MCP-DISCOVERY | FI Sessions/Recordings/MCP discovery owner; WSV consumes the same prerequisite | Diagnose returned durable session identity/recording lookup; merge behavior-preserving discovery correction before integrated T18 M01/F13. Any public-contract/policy delta returns to operator before implementation. |
| FI-PREREQ-WEBHOOK-READINESS | FI Sessions/Runtime/Webhooks readiness owner; production authority pending operator | Explicit operator authorization plus prerequisite merge before T26 replacement cutover. Amendment merge alone does not release F18a-f. Characterization remains retained. |
| FI-SHARED-QUALITY | Factory Reliability/shared quality-gate maintenance, independent reviewers | Real primary-lane coverage and fixture-isolation/lane-audit release plus applicable terminal exact-head CI. T01/T06 retained work is not exempted or marked terminal here. |
| VAL25 and VAL01 | Independent validation owners; candidate artifacts and contributing lanes | Preserve T25 M01/M06/M07/M08/L25-7, exact-head L25-6 and real Linux/artifact proof, then aggregate VAL01. Author handoff is a separate finish line. |

**AM16 / T12 — approved private behavior-owner decomposition (binding 2026-10-03T10:20Z).** Authority: `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decision 2026-10-03T10:20Z; this supersedes the 09:05Z deferral only. Retain draft [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) at `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; its stories 002/003 and full F10/S01 remain incomplete. The separately authorized `fi-plan-amendment-am16-t12-20261003` docs lane depends on merged v1.1 #2685 and owns only this six-companion packet. AM16 is now merged historical authority; the retained T12 successor consumes merged AM17 before planning, with no historical AM16 Work dependency or T20/WSV wait. T12 owns implementation, same-PR stale script_pollers baseline deletion and obsolete-helper removal. T20 owns unchanged one-interface enforcement and measurement; deadcode allowance remains 0. No public API, configuration, event, persistence-policy or acceptance change is authorized. Native capability/provider/caller/removal pairs C01–C16 are canonical in [contracts.md](contracts.md#t12--automations-runtimesource-isolation). The docs author handoff requires final head pushed, open PR, CI started and blocking feedback addressed; independent AM16-DOC-VAL/REVIEW owns clean-room loopback, terminal CI and current-main merge. The successor still requires exact-head T12-F10 (F10a–m), T12-S01 and T12-G01/G02, independent review and merge. Historical partial evidence never substitutes for those gates.

All T01–T31 and VAL01 obligations remain mapped by sections 10–13 and their packets. Cause-corrected successor planning consumes amended main only after ordinary amendment merge (`DEPENDS_ON` amendment lane, `requiredState: complete`). This docs lane does not admit successors, implement prerequisites, or change Project admission. T12/T19 live owner hunks, T25 #2676 and current-main T30 terminal rows are retained; only overlapping amendment hunks are coordinated.

### AM17 — T12 evidence boundaries

Binding authority: operator mailbox `C:/Users/andre/work/portos/infinite-you/docs/temp/operator-mailbox/responses/flat-injection.md`, Decisions 2026-10-03T12:01Z A and 12:57Z. AM17 corrects evidence classification under flat-injection-v1.1; it changes no endpoint, service signature, configuration, event, persistence, policy or acceptance criterion. AM16 is merged historical authority, not a live Work dependency. The retained T12 successor consumes merged AM17 before planning and continues from [#2683](https://github.com/portpowered/you-agent-factory/pull/2683) / `62cff1c7cfb4d734d462aa1ef08d2d5e0e6c4ba7`; stories 002/003 and full F10/S01 remain unproven. No T20/WSV wait or queue mutation is required.

Follow [the AM17 resolution](plan.md#authorized-amendment-flat-injection-v11) to [the complete F10 requirements ledger](tasks.md#am17--t12-evidence-boundaries). T12 owns both proofs below; independent VAL01 confirms composed behavior. The existing `automations.Root.GetCursor` delegates the service operation (`pkg/services/automations/contracts.go:215–222,457–474`). Durable reads validate `ExpectedCursor` and return detached opaque facts (`pkg/services/automations/internal/services/script_pollers/internal/service/durable_cursor.go:74–108`). The cursor HTTP adapter (`pkg/services/automations/transports/http/convergence_operations.go:52–97`) is unbound in canonical Wire/HTTP; adapter tests are component evidence. Neither that adapter nor `AutomationsRootFromEdges` is a composed functional entry point. Canonical construction/execution remains `pkg/root/process.go:17` / `pkg/initializer/application/process.go:185`.

| Obligation | Component/service-contract evidence — T12-F10-COMPONENT | Composed evidence — T12-F10 / VAL01 | Owner |
| --- | --- | --- | --- |
| F10b | Existing GetCursor returns exact committed opaque cursor/checkpoint facts | Existing customer activation admits public Work; resumed command receives the same exact committed environment facts | T12; both proofs required |
| F10g | After failed replacement, GetCursor returns prior exact committed opaque cursor/checkpoint facts | Admitted public Work remains; existing error/diagnostic is visible; next command resumes prior-commit environment facts; peer progresses | T12; both proofs required |
| F10j | GetCursor with stale ExpectedCursor returns the existing typed conflict; exact committed cursor/checkpoint facts remain unchanged; no mutation, extra command or Work | No composed public cursor endpoint is required or invented; admission/restart/diagnostic/isolation obligations remain in F10b/g and the other F10 rows | T12 component/service-contract proof |

All F10a–m success, failure and recovery obligations remain required on the successor head. C01–C16 native shapes, exactly one named interface per private service root, same-PR obsolete-helper/stale-baseline deletion, deletion-only baselines, deadcode allowance 0, T12-S01/G01/G02 and FI-A1–A8 remain unchanged. This amendment yields documentation/source/static evidence only, never runtime PASS.

AM17-DOC-TRACE/INVARIANTS/LINT belong to the docs author. Author delivery stops at final head pushed, PR open, required CI started and blocking feedback addressed. Independent ordinary review owns AM17-DOC-VAL: a fresh isolated candidate checkout, the [validation loopback report](../../../../../factory/docs/standards/validation-loopback-template.md) with AM17-1..6, AM17-DELIVERY and FI-A1–A8 individually PASS/FAIL/BLOCKED, and a smallest delta request for findings without silent repair. AM17-DOC-REVIEW owns terminal passing own-head CI, conflicts, current-main rebase, immediate premerge `make lint pkg-file-count`, and merge. `make generate-wire` plus clean diff applies only to a separately authorized graph change. Merge releases corrected successor planning; runtime gates and aggregate Project acceptance remain later work.

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

### Complete behavior case matrix

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

F10 uses the AM17 component/service-contract and composed evidence split; the complete F10a–m ledger in tasks.md remains mandatory. No composed cursor endpoint is assumed.

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
| T27 lifecycle performance baseline | FI-PREREQ-BASELINE-OBS | All; measures only pinned 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6 |
| T01 process time; T02 process logging | None | All independent owner extraction lanes |
| T03 Recordings; T04 Definitions (incl. System Initialization installer); T05 Work; T06 Provider Sessions; T07 Settings; T08 Providers; T09 Workers | None | Each other; preserved public peer contracts |
| T10 Models leaf providers | None | Other owners |
| T30 Models slot state / capacity coordinator | None | T10 and other owners |
| T11 Models scoped state | T10, T30 | Runtime/Sessions/Automations |
| T12 Automations | Merged AM17 before successor planning; AM16 already merged; retained #2683/head 62cff1c7 | All independent owners; no T20/WSV wait |
| T13 Sessions directory/streams leaves | None | T14/T16 and other owners |
| T14 Runtime control/host authority | None | T13/T16 and other owners |
| T31 Runtime instance-host build leaves | T14 | T13/T16 and other owners |
| T15 Runtime activation/replacement/replay | T14, T16, T31 | T11/T12/T13 and other owners |
| T16 Worker Sessions keyed supervision | None | T09/T13/T14; existing Workers contract |
| T17 Sessions opening/products cutover | T13, T15 | T18 and other owners |
| T18 HTTP/MCP/visualization direct adapters | Adapter development: None; integrated M01/F13: FI-PREREQ-MCP-DISCOVERY | All owners; whole-lane admission must split development from prerequisite-gated acceptance |
| T28 Sessions getter removal from HTTP/visualization | T17, T18 | Other owners |
| T19 CLI direct roles | None | All owners |
| T20 static enforcement infrastructure and per-set enablement | None | Report legacy first; enforce completed owners progressively |
| T21 cross-surface effect adoption (`pkg/wire` defaults only) | T01, T02 | Other owners; coordinate named provider functions |
| T23 boundary normalization/requiredness | None; coordinates additive seams with T01/T02 | All owner lanes; no all-repository constructor rewrite |
| T24 platform helper/compatibility retirement | T03, T08, T09, T10, T11, T12, T23, T26 | Production user call sites owned by their lane |
| T25 Costs query | None; independent validation/quality remains review-owned | Existing public price/metrics contracts retained |
| T26 Webhooks delivery | Characterization: None; replacement cutover: authorized FI-PREREQ-WEBHOOK-READINESS | Retained characterization only while authority is held |
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
