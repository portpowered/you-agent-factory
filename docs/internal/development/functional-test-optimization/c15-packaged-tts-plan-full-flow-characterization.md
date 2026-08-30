# C15 packaged TTS, plan-execute, and Full Flow characterization ledger

Status: Story 001 is **BLOCKED**, not passed. The unchanged head passes the
three owned packages, but the PRD's TTS-U02 missing-input case has no
executable source row. The current `required_input` row is a valid-input
success case and is not counted as evidence for the failure contract.

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

## Story result, blocker, and next step

Story 001 cannot be marked `passes:true` because the required TTS-U02
behavioral witness is absent. This is a plan/source contradiction, not a
test failure caused by this work. Safe work completed: the unchanged
three-package run, static OS check, selector/assertion mapping, topology
census, quarantine inspection, and coverage-reference inventory are recorded
above. The smallest delta is for Story 002, which owns
`tests/functional/factory/packaged/tts/**`, to add the missing empty/omitted
text scenario with the PRD's exact pre-execution failure and zero-call
assertions, then rerun the TTS characterization before claiming GATE-TTS.

Unproven later edges: TTS-U02, post-optimization TTS/plan/Full Flow behavior,
the Full Flow direct-Git removal, PR timing, aggregate/package-floor
measurement on the delivered PR head, clean-room loopback, terminal CI, and
merge.

## Source hash register

SHA-256 hashes tie this read-only ledger to the characterized source:

| File | SHA-256 |
| --- | --- |
| `tests/functional/factory/packaged/tts/characterization_assertions_test.go` | `3caf03d8171212709655e21deb5ee596fca6b8ba8b0dc00b4c06fbd6b15fa0b4` |
| `tests/functional/factory/packaged/tts/factory_scaffolds_test.go` | `60472c68dc4be28fa568c15d7f630344839021964607bd9fff19902ab3301b02` |
| `tests/functional/factory/packaged/tts/fixtures_test.go` | `3ae578684a6926256b52b30563cdc2e721dda8443cf20d2ec16f5f458a41bf4b` |
| `tests/functional/factory/packaged/tts/invocation_test.go` | `e78c5084588882fd885c804d1d07ba12d084cb9d915862d4437cf46a6ed22672` |
| `tests/functional/factory/packaged/tts/local_runtime_invocation_test.go` | `ded1cd867f329bad5cf7e48815e6e55be01cbd958b716d2c5e7a7a697329b96a` |
| `tests/functional/factory/packaged/tts/models_replay_helpers_test.go` | `5503dc8c3ece2cdaadae04bdc7faaaa0f2c2be1443173610bd8386a338e1f9f5` |
| `tests/functional/factory/packaged/tts/models_replay_test.go` | `5fd863efde65b6fd7a61b29cd2d4c8dd6e214662bfd0d6c81fa4ac1792c14b9c` |
| `tests/functional/factory/packaged/tts/shared_command_runner_test.go` | `253016f4dff475794cf81924c39bfbe9acb5f7f64120192c9d4c46584c1c0f47` |
| `tests/functional/factory/packaged/tts/shared_concurrency_test.go` | `d9beae4d8d67c462a83fd2299f46e0a4c16186c4a454364a06121f82344a7155` |
| `tests/functional/factory/packaged/tts/shared_fixture_test.go` | `1e6f39f73840981f09e36b2e2ef4853d09c48bee41436c5461a21262f11c5729` |
| `tests/functional/factory/packaged/plan_execute/invocation_test.go` | `a1a8d4d7b9afe0814f99d3e41941c661e66284d909ec0bafe2fc6282cf8d65cd` |
| `tests/functional/factory/packaged/plan_execute/shared_fixture_test.go` | `f35f5e52ab0b26e3e7a05a377582cbb31e73fa14a9e858c7fe93dbd7ad1ed921` |
| `tests/functional/factory/packaged/full_flow/invocation_test.go` | `34cd8ae792108eae46a98114460080f8000a3a61d4513bc370081cb62d62143f` |
| `tests/functional/factory/packaged/full_flow/shared_fixture_test.go` | `1c6864ef4c89fda99b9d2387612d29419b898d92460f7c1509cbc7cf2cb2fdf3` |
