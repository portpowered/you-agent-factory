# C15 packaged TTS, plan-execute, and Full Flow characterization ledger

Status: Story 001's pre-change record is complete and retains its historical
U02 blocker; Story 002 supplied the explicitly scoped smallest delta and now
passes the complete TTS gate; Story 003 preserves the plan-execute witness and
adds explicit runner-workspace and shared-host cleanup evidence; Story 004 now
passes after the operator-authorized Full Flow OS-spawn bookkeeping
reconciliation. Story 005 remains the integrated promotion story. The
unchanged-head table below must not be read as claiming that U02 existed before
the implementation change.

## Authority and boundary

- PRD: `prd.json`, project `functional-test-optimization`, branch
  `functional-test-optimization-c15-packaged-tts-plan-full-flow`.
- Characterized head: `42eeee4472`, equal to `origin/main` at capture time.
- The PRD's `context.sourcePlan` points to `docs/temp/functional-test-optimization.md`,
  which is absent from this checkout. The explicit PRD/customer payload was
  used for the missing addendum requirements; no source-plan claim is made.
- No implementation, baseline, inventory, or generated file was changed for
  this characterization. `prd.json` and `progress.txt` remain ignored task
  scaffolding.
- Dependency fidelity: local-real `root.BuildProcess`/`Process.Execute` and
  local Factory/session/event/recording paths with controlled provider, model,
  protocol, filesystem, and repository effects. No remote or paid calls.

## Environment and procedures

Environment: Windows `windows/amd64`, Go `go1.25.0`, `GOFLAGS` empty. The
following list-only commands returned exit status 0 and discovered the three
top-level parents:

```text
go test ./tests/functional/factory/packaged/tts -list '^Test' -count=0
go test ./tests/functional/factory/packaged/plan_execute -list '^Test' -count=0
go test ./tests/functional/factory/packaged/full_flow -list '^Test' -count=0
```

The required unchanged-head functional run was:

```text
go test -v -count=1 -timeout 10m \
  ./tests/functional/factory/packaged/tts \
  ./tests/functional/factory/packaged/plan_execute \
  ./tests/functional/factory/packaged/full_flow
```

It exercised all declared parents and nested behavior rows and exited 0. A
separate bounded package timing capture also exited 0 for each package. Its
package-reported timings were TTS `72.033s`, plan-execute `2.888s`, and Full
Flow `11.511s`; outer Windows wall samples were `115.611s`, `6.468s`, and
`15.258s`, respectively. These are characterization samples, not the PR
timing verdict. The customer-supplied baselines are TTS `35.224s` and
plan-execute `5.036s`; no customer Full Flow baseline was supplied.

## Behavior and assertion inventory

The table maps every PRD case to its current source selector and exact
observable assertion family. `BLOCKED` means the required source witness is
absent; it is not a pass by routing or naming convention.

| Case | Current selector/source | Result and assertion inventory |
| --- | --- | --- |
| TTS-H01 | `^TestPackagedTTSNoServerPromptUsesCanonicalInputContract$` (`tts/invocation_test.go:25`) | PASS. `COMPLETED`, non-empty request/trace identity, exactly one `codex exec --json --model tts -` call, consumed Work ID/authored prompt, audio metadata, and exact fixture bytes. |
| TTS-H02 | `^TestPackagedTTSLocalRuntimePayloadPreservesExactBoundText$` (`tts/local_runtime_invocation_test.go:28`) | PASS. `COMPLETED`, one `TTS/tts` Models request with exact text, one host start and balanced stop, and exact audio bytes. |
| TTS-R01 | `^TestDeliveredPackagedTTSFactoryReachesProtocolFixture$` (`tts/models_replay_test.go:34`) | PASS. Delivered binary reaches the controlled protocol once with exact TTS/model/text input, records ordered live events and audio, and replay makes no second protocol call. |
| TTS-R02 | `^TestFactoryTTSModelsRootBuildProcessExecuteRecordsAudio$` (`tts/models_replay_test.go:129`) | PASS. Root `Process.Execute` returns audio, makes one Models call, records ordered Work/dispatch/model events, preserves request/trace/dispatch correlation and worker-session association, and writes the recording. |
| TTS-R03 | `^TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections$/success` (`tts/models_replay_test.go:369`) | PASS. Live/replay Work and event projections are equivalent; audio digest, metadata, artifact lineage, event order, and zero replay backend calls are asserted. |
| TTS-U01 | `^TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections$/failure` (`tts/models_replay_test.go:418`) | PASS. Live/replay are `FAILED` with correlated model/dispatch/Work projections, exact failure detail, no artifact, and one live backend call only. |
| TTS-H03 | `^TestPackagedTTSSharedScenarios/success_work_events$` (`tts/shared_fixture_test.go:402`) | PASS. Explicit session Work request, `COMPLETED` Work, one command call, exact audio artifact, Work/dispatch/model events, and response correlation. |
| TTS-H04 | `^TestPackagedTTSSharedScenarios/optional_voice_format$` (`tts/shared_fixture_test.go:405`) | PASS. Exact `voice=alloy` and `format=mp3` bindings reach one command call; audio, metadata, events, and response correlation remain valid. |
| TTS-U02 | **No current selector** | **BLOCKED.** The PRD requires absent/empty text to fail before provider/model execution. Source inspection found no empty or omitted text invocation; `required_input` instead sends `functional shared packaged tts required input` and expects `COMPLETED`. The positive row cannot prove this failure property. |
| TTS-U03 | `^TestPackagedTTSSharedScenarios/generic_failure$` (`tts/shared_fixture_test.go:408`) | PASS. Exact `tts backend failed` failure reaches failed Work/model/dispatch projections, with no audio path, artifact event, or false success output. |
| TTS-U04 | `^TestPackagedTTSSharedScenarios/packaged_model_failure$` (`tts/shared_fixture_test.go:411`) | PASS. `FAILED`, exact `INVOCATION_TTS_GENERATION_FAILED` code and `omnivoice invoke failed: exit status 1` detail, correlated failure events, and no artifact. |
| TTS-C01 | `^TestPackagedTTSSharedScenarios/concurrent_success_failure_isolation$` (`tts/shared_fixture_test.go:414`) | PASS. Concurrent success/failure retain distinct sessions, requests, Work IDs, model/trace/dispatch IDs, event IDs, routes, and artifact ownership; each makes one call and only success emits audio. |
| PLAN-H01 | `^TestPackagedPlanExecute/TestPackagedPlanExecutePlansThenExecutesWithOperatorDefaults$` (`plan_execute/invocation_test.go:31`) | PASS. Calls are exactly `planner,executor`; each uses `codex` and `operator-default-model`; `implemented.txt` bytes and passed PRD story/notes are exact. |
| FLOW-H01 | `^TestPackagedFullFlow/TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion$` (`full_flow/invocation_test.go:25`) | PASS. Exact `task-a.txt`/`task-b.txt`, `core.longpaths=true`, two planner calls, overlapping implementations, accepted merge order, two replay waves, two merge dispatches, and exact cursor replay. |
| FLOW-U01 | `^TestPackagedFullFlow/TestPackagedFullFlowBoundsImplementationContinueLoopAndFailsProject$` (`full_flow/invocation_test.go:28`) | PASS. Project is failed after finite guarded implementation attempts (`2..42`) and zero merges. |
| FLOW-U02 | `^TestPackagedFullFlow/TestPackagedFullFlowEnforcesCallerSelectedTaskBound$` (`full_flow/invocation_test.go:31`) | PASS. One planner call rejects the over-bound wave before implementation or merge (`0/0`). |
| FLOW-U03 | `^TestPackagedFullFlow/TestPackagedFullFlowEnforcesCallerSelectedCycleBound$` (`full_flow/invocation_test.go:34`) | PASS. One planner call, exactly two merges, no second plan, and failed bounded result. |

The executed but non-substitutive current row is
`^TestPackagedTTSSharedScenarios/required_input$` (`tts/shared_fixture_test.go:399`):
it is a valid-input success witness, not TTS-U02. Therefore 16 PRD rows have
matching passing evidence and one required PRD row remains unproven.

## Resource and topology census

| Package | Immutable/reusable setup observed | Scenario-private setup and cleanup evidence |
| --- | --- | --- |
| TTS | Five `BuildProcess` construction sites: no-server, local runtime, root Models, managed live/replay, and shared scenarios. Managed fixture asserts `roots=1`, `installs=1`, `cacheSeeds=1`; shared fixture asserts one process and one API host. | Managed live/replay uses two Factory copies, two cache copies, two recordings, and balanced model-host starts/stops (`2/2`). Shared scenarios use seven explicit sessions (five sequential plus the concurrent pair), unique route/Work/event/artifact identities, and zero active sessions/artifact roots/routes after cleanup. |
| plan-execute | One shared root/API host, one packaged seed install, and one routed provider edge; the fixture ledger expects one process start and one explicit session. | One scenario Factory/workspace/session is copied and registered; close verifies session `404`, unique roots, removed scenario root, zero provider routes, listener shutdown, and zero runtime artifacts. |
| Full Flow | One shared root/API host, one packaged seed install, and one routed provider edge; the fixture ledger expects one process start. | Four fresh explicit sessions, Factory copies, repositories/worktrees, and provider routes are created across the four cases; close verifies session `404`, unique roots, removed roots, listener shutdown, and zero runtime artifacts. The current repository initializer still uses the direct Git helper recorded below. |

Static source counts at this head are TTS `2`, plan-execute `0`, and Full Flow
`1` OS-spawn sites. The TTS sites are the delivered-row `go build` and binary
execution witnesses. Full Flow's site is currently:

```text
tests/functional/factory/packaged/full_flow/invocation_test.go:389
fullFlowGit -> exec.Command("git", args...)
OSSPAWN-tests-functional-factory-packaged-full-flow-invocation-test-fullFlowGit-01
```

## OS, enablement, and coverage gates

The mandated command was:

```text
make functional-os-boundary-check
```

It exited 0 and reported:

```text
static OS-spawn baseline holds: observed=70 baseline=70 packages=23 intentional=62 accidental=8 decreased=0
reconciled 70 inventory OS-spawn records
```

The checked-in baseline has 70 total sites, Full Flow count 1, and the exact
Full Flow site ID above. No baseline or verdict reclassification was changed.

`tests/functional/functional-quarantine.json` contains nine entries, all for
other packages; TTS, plan-execute, and Full Flow each have zero quarantine
entries. Source inspection found no `t.Skip` in the three owned directories,
and the required run exited 0. The functional coverage manifest records
`lane=functional`, 357 package rows, default floor `15.0`, and 29 explicit
holds. The lane's customer acceptance reference is aggregate coverage at
least `61.6%`; no aggregate coverage measurement was claimed by this
characterization-only story and PR coverage remains a later gate.

## Story 001 historical result

At the characterized head (`42eeee4472`), Story 001 recorded a source/PRD
contradiction rather than silently counting the positive `required_input` row
as U02. The unchanged three-package run, OS check, selector/assertion mapping,
topology census, quarantine inspection, and coverage-reference inventory above
remain the pre-change evidence. Story 002 is the recorded smallest delta that
resolved the missing executable witness.

## Story 002 — reusable TTS matrix result

The implementation stays within `tests/functional/factory/packaged/tts/**`.
The command-edge cases now use one package-scoped immutable `root.BuildProcess`
and one packaged Factory seed. The no-server H01 row routes its unique working
directory to the shared command runner; the HTTP matrix starts one persistent
server command and gives every scenario a fresh home, Factory copy, explicit
Factory Session, route, outcome/artifact root, and cleanup. Repeating the
shared parent keeps that server alive until `TestMain`, because the application
process intentionally cannot restart its API server after shutdown. Managed
Models setup remains one process/installation/cache seed with fresh live/replay
copies and backend/recording state; the local-runtime, root-Models, and
delivered-binary rows remain isolated because their host, edge, or artifact
properties are incompatible.

The missing U02 witness is now
`^TestPackagedTTSSharedScenarios/missing_required_input$` at
`tts/shared_fixture_test.go:526`, with two exact admission cases:

| Variant | HTTP result | Execution proof |
| --- | --- | --- |
| omitted required text | `400`, `INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT` | zero command calls, no dispatch/model execution events, no artifact |
| explicit empty text | `400`, `INVOCATION_INPUT_EMPTY` | zero command calls, no dispatch/model execution events, no artifact |

The positive `required_input` row remains enabled and unchanged as a success
witness. Together with H01, H02, R01, R02, R03/U01, H03, H04, U03, U04, and
C01, the package executes all twelve PRD TTS cases. No TTS source row was
skipped or quarantined.

### Story 002 verification evidence

All procedures used local-real production composition with controlled provider,
model, protocol, and artifact edges. Each command exited 0:

```text
go test -count=1 -timeout 10m ./tests/functional/factory/packaged/tts -run '^TestPackagedTTSNoServerPromptUsesCanonicalInputContract$'
go test -count=1 -timeout 10m ./tests/functional/factory/packaged/tts -run '^TestPackagedTTSSharedScenarios$'
go test -count=3 -timeout 10m ./tests/functional/factory/packaged/tts -run '^TestPackagedTTSSharedScenarios$'
go test -race -count=1 -timeout 10m ./tests/functional/factory/packaged/tts -run '^TestPackagedTTSSharedScenarios$'
go test -json -count=1 -timeout 10m ./tests/functional/factory/packaged/tts
```

The focused observations were H01 `4.576s`, shared matrix `13.408s`, shared
repeat three-count `29.847s`, and race-enabled shared matrix `14.422s`. The
final full-package run passed all twelve cases at `112.411s` package-reported
on a contaminated Windows host. An earlier bounded run before that host
slowdown reported `60.265s`, versus the unchanged diagnostic `72.033s`; the
customer-supplied TTS baseline is `35.224s`. The optimization is therefore
structurally material (one shared command-edge root/build/install) but does not
claim a portable local wall-time threshold.

The final full-package per-row measurements provide the required residual
irreducibility record:

| Residual row/group | Measured cost | Exact property that prevents further compatible collapse |
| --- | ---: | --- |
| TTS-H01 no-server prompt | `2.58s` | Exact CLI prompt, consumed Work identity, command arguments, metadata, and bytes; shares only the command-compatible edge. |
| TTS-H02 local runtime | `4.93s` | Real local model-host readiness/lifecycle and exact joined Models payload; its host edge cannot share the managed Models or command fixture. |
| TTS-R01 delivered protocol/replay | `34.86s` | Delivered `you` build plus binary live/replay protocol boundary and one-call replay proof; the artifact boundary is intentionally isolated. |
| TTS-R02 root Models recording | `13.69s` | Root `Process.Execute` recording, Work/Event lineage, digest, metadata, and artifact property on the Models edge. |
| TTS-R03 success/failure replay pair | `41.56s` | Two live/replay projections with independent backend/cache/recording state and distinct success/failure semantics; sharing those mutable resources would erase the isolation proof. |
| TTS-H03/H04/U02/U03/U04/C01 shared matrix | `13.93s` | One API/server lifecycle is shared, but each row retains fresh sessions/copies/routes/outcomes and the concurrent pair retains distinct overlap and cleanup assertions. |

The shared fixture asserts immutable setup counts of one process, one packaged
install, and one API start per package process; route count and active
session/artifact counts are zero after the matrix. The managed Models fixture
logs `roots=1 installs=1 factoryCopies=2 cacheSeeds=1 cacheCopies=2
liveRecordings=2 modelHostStarts=2 modelHostStops=2`. TestMain stops the
persistent command, closes the process, verifies the listener is unreachable,
and removes the package-owned root. Remote model availability, audio quality,
aggregate coverage/package floors, three-package coexistence, PR timing, clean
room loopback, terminal CI, and merge remain later gates.

## Story 003 — reusable plan-execute result

The plan-execute journey retains one package-scoped `root.BuildProcess`, one
packaged Factory installation, one API host, and one routed
`ProviderCommandRunner` edge. The scenario creates a fresh copied Factory,
working directory, non-default Factory Session, and runner workspace. The
runner workspace is nested under the scenario root, so cleanup removes it
explicitly instead of relying only on Go's automatic `t.TempDir` cleanup.

The selector
`^TestPackagedPlanExecute/TestPackagedPlanExecutePlansThenExecutesWithOperatorDefaults$`
passed with the exact `planner,executor` call order. Both requests were
`codex` calls carrying `--model operator-default-model`; the exact
`implemented.txt` bytes were `implemented from prd\n`; and the generated fixture
PRD contained one passed story with non-empty notes. The scenario then verified
the session was absent with HTTP 404, the runner workspace and scenario root
were removed, the provider route registry was empty, and the shared host still
served a valid `/status` response before process teardown. The lifecycle log
reported `process_starts=1 explicit_sessions_opened=1
explicit_sessions_closed=1 unique_session_ids=1 scenario_roots_removed=1
runner_workspaces_removed=1 runtime_artifacts=0 isolated_rows=0`.

### Story 003 verification evidence

Procedure, run on the delivered working tree after the bounded cleanup/reuse
pass, used local-real production composition with a controlled provider
command runner. It exited 0:

```text
go test -v -count=1 -timeout 10m ./tests/functional/factory/packaged/plan_execute
```

The package reported `ok ... 5.256s` on Windows; the nested behavior row
reported `0.96s`. The retained characterization sample was `2.888s`, and the
customer-supplied diagnostic baseline is `5.036s`. This local run is noisy and
does not claim the PR timing verdict. The completed bounded pass found no
compatible setup to remove beyond the existing one-root/one-install/one-host
topology: packaged initialization protects the packaged seed contract, the
continuous host is required by the Factory Session invocation boundary, and
the runner must remain a real controlled command edge. The residual PLAN-H01
row therefore retains exact two-stage output, prompt/default selection,
artifact/PRD mutation, explicit-session cleanup, route teardown, and host
reuse as its real properties; PR timing remains authoritative at the PR gate.

The run emitted existing structured discovery diagnostics while probing copied
non-root child directories (`.you-agent-factory`, `inputs`, `workers`, and
`workstations`); these are expected probe logs, not test failures. Remote
provider behavior, aggregate coverage/package floors, three-package
coexistence, clean-room loopback, terminal CI, and merge remain later gates.

## Story 004 — Full Flow repository-edge result

The Full Flow fixture now supplies one shared `WorkersWorktreeGit` edge and
one script command edge to the root-built process. The edge performs the
fixture's observable repository contract in memory while materializing real
`.git` metadata, worktree files, committed task snapshots, and merged task
files for the production Worktree preparer and public assertions. Setup's
packaged script behavior is preserved through the controlled script edge, so
the test no longer launches Git from `invocation_test.go`.

### Story 004 verification evidence

Local-real production composition with controlled provider, repository, and
script edges was used. These commands exited 0:

```text
go test -count=1 -timeout 10m ./tests/functional/factory/packaged/full_flow
go test -race -count=1 -timeout 15m ./tests/functional/factory/packaged/full_flow
go test -count=3 -timeout 20m ./tests/functional/factory/packaged/full_flow
```

The package timings were `14.936s`, `27.387s`, and `60.878s` on the Windows
host. All four Full Flow cases ran in each invocation, including exact success,
review/verification, merge, replay, bounded implementation failure, caller
task bound, caller cycle bound, and shared four-session cleanup. The happy
case's edge ledger observed initialization, identity config, add/commit,
repository and branch probes, two worktree adds, two long-path config writes,
two task commits, two `diff --check` calls, two merges, and the persisted
`core.longpaths` readback. The race run reported no data race.

An independent source cross-check found no `exec.Command` or
`exec.CommandContext` match under `tests/functional/factory/packaged/full_flow`
and counted 69 remaining functional matches repository-wide, down from the
70-site characterization. The required boundary command was run before the
operator-authorized bookkeeping reconciliation and failed only because the
removed site remained in the frozen records:

```text
make functional-os-boundary-check
```

It exited 1 before reporting the decreased count because the frozen inventory
still contained the removed site
`OSSPAWN-tests-functional-factory-packaged-full-flow-invocation-test-fullFlowGit-01`:
`inventory site ... is not present in the AST census` and
`LINT_VIOLATION_COUNT: 1`. The baseline and inventory files remain unchanged,
as required by this story's current scope. Real Git executable semantics,
combined clean-room coverage, terminal CI, and merge remain unproven.

### Operator-authorized OS bookkeeping reconciliation

The `operatorAmendment` dated 2026-08-31 authorizes reconciliation of exactly
`OSSPAWN-tests-functional-factory-packaged-full-flow-invocation-test-fullFlowGit-01`.
No checker code, tolerance, test, assertion, fixture, or other OS-spawn site
was changed.

| Authorized location | Before | After |
| --- | --- | --- |
| `docs/internal/baselines/functional-os-spawn-baseline.json`, Full Flow package row | `count=1`, `siteIds=[OSSPAWN-tests-functional-factory-packaged-full-flow-invocation-test-fullFlowGit-01]` | `count=0`, `siteIds=[]` |
| `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json`, `osSpawnSites` | One `fullFlowGit` `ACCIDENTAL-OS` object | Object removed; 69 inventory records remain |
| `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.md`, complete static-site projection | One `fullFlowGit` row | Row removed |
| `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.md`, retained isolated-row verdict projection | One `fullFlowGit` row | Row removed |

The derived human package summary in the same Markdown inventory changed from
Full Flow `1` to `0` so it agrees with the four reconciled locations. No other
site was reconciled.

The required source and count evidence on the reconciled head was:

```text
rg -n 'os/exec' tests/functional/factory/packaged/full_flow
exit 1; no matches

rg -n 'exec\.Command(?:Context)?\s*\(' tests/functional/factory/packaged/full_flow
exit 1; no direct process launch matches
```

The total-site ratchet is explicit: the checked-in baseline decreased from
`70` to `69`, and the inventory decreased from `70` to `69` OS-spawn records.
The post-source-migration AST census was already `69` before reconciliation,
with zero Full Flow sites; reconciliation made all three records agree.

The final boundary command was:

```text
make functional-os-boundary-check
```

It exited `0` and reported:

```text
[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=69 baseline=69 packages=22 intentional=62 accidental=7 decreased=0
[agent-factory:functional-os-boundary] reconciled 69 inventory OS-spawn records
```

The focused Full Flow package, repeat, and race evidence above remains valid
because this reconciliation changed only governance data; the final integrated
clean-room, coverage, PR timing, terminal CI, real Git executable semantics,
and merge remain Story 005 or review-stage evidence.

## Source hash register

SHA-256 hashes tie this ledger to the delivered source:

| File | SHA-256 |
| --- | --- |
| `tests/functional/factory/packaged/tts/characterization_assertions_test.go` | `3caf03d8171212709655e21deb5ee596fca6b8ba8b0dc00b4c06fbd6b15fa0b4` |
| `tests/functional/factory/packaged/tts/factory_scaffolds_test.go` | `60472c68dc4be28fa568c15d7f630344839021964607bd9fff19902ab3301b02` |
| `tests/functional/factory/packaged/tts/fixtures_test.go` | `3ae578684a6926256b52b30563cdc2e721dda8443cf20d2ec16f5f458a41bf4b` |
| `tests/functional/factory/packaged/tts/invocation_test.go` | `874b3ca268d469b7fc7750c25911ef4c2649060d3c976f7011d474da174216e` |
| `tests/functional/factory/packaged/tts/local_runtime_invocation_test.go` | `ded1cd867f329bad5cf7e48815e6e55be01cbd958b716d2c5e7a7a697329b96a` |
| `tests/functional/factory/packaged/tts/models_replay_helpers_test.go` | `5503dc8c3ece2cdaadae04bdc7faaaa0f2c2be1443173610bd8386a338e1f9f5` |
| `tests/functional/factory/packaged/tts/models_replay_test.go` | `5fd863efde65b6fd7a61b29cd2d4c8dd6e214662bfd0d6c81fa4ac1792c14b9c` |
| `tests/functional/factory/packaged/tts/shared_command_runner_test.go` | `253016f4dff475794cf81924c39bfbe9acb5f7f64120192c9d4c46584c1c0f47` |
| `tests/functional/factory/packaged/tts/shared_concurrency_test.go` | `d9beae4d8d67c462a83fd2299f46e0a4c16186c4a454364a06121f82344a7155` |
| `tests/functional/factory/packaged/tts/shared_fixture_test.go` | `280e65ac62b81601faf157603e21141784d69085c284747ec2bf8ef145b7907b` |
| `tests/functional/factory/packaged/plan_execute/invocation_test.go` | `E841BE6D898F9CF4435D6C34E84BBAC2818160C19FD583574AEEC34410A5BE28` |
| `tests/functional/factory/packaged/plan_execute/shared_fixture_test.go` | `A936845335DDA31E563E4F1FDE70D31CED837141D34825D03453ECC6EBE3FE03` |
| `tests/functional/factory/packaged/full_flow/invocation_test.go` | `B2CC50CF94EE9FC52D81555AEDB05B4546E971092655721D56C6558DF0F9C770` |
| `tests/functional/factory/packaged/full_flow/shared_fixture_test.go` | `4CBAC9DB44632A955EEE438FC1275EA852044EF27A4E282E273472299FC292D2` |
| `tests/functional/factory/packaged/full_flow/repository_edge_test.go` | `66967689FDBB679C93BEB5C034FF7CD4E03C8C6027B93160E4429996F7A0CBB1` |
