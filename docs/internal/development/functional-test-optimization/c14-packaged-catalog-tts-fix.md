# C14 packaged Catalog, TTS, and Fix baseline ledger

Status: stories `fto-c14-pkg-factory-packaged-cluster-001` through
`...-004` are complete. This ledger freezes the pre-change denominator and
characterization for the three owned functional packages and records the
bounded Catalog, TTS, and Fix optimizations. Story `...-005` owns final
measurement, loopback, and review handoff.

## Authority, scope, and artifact

- PRD: `prd.json`, project `functional-test-optimization`, branch
  `fto-c14-pkg-factory-packaged-cluster`.
- Source-plan field: `context.sourcePlan` is `null`; no excluded temporary-plan
  source was read or cited.
- Baseline source: `fea2e30a499384182d2fabe7038767e3c2f9c5e5`, equal to
  `origin/main` at capture time.
- Owned implementation surfaces characterized:
  `tests/functional/factory/packaged/catalog/**`,
  `tests/functional/factory/packaged/tts/**`, and
  `tests/functional/factory/packaged/fix/**`.
- Durable handoff artifact: this file. The ignored `prd.json` and
  `progress.txt` are task scaffolding and are not PR changes.

The baseline used the current root-built functional composition. Ordinary
rows execute through `root.BuildProcess`/`Process.Execute`; the Catalog API
row crosses its API-owned loopback; TTS retains the actual delivered `you`
binary row and local Models edge; Fix retains real local Git and worktree
operations. Provider/model effects are controlled at the declared edge and
made zero-cost; no remote or paid call was used.

## Environment and clean starting state

- OS: Microsoft Windows 11 Home, version/build `10.0.26200`, 64-bit.
- Go: `go1.25.0 windows/amd64`, `GOAMD64=v1`, `GOTOOLCHAIN=auto`.
- Host: 24 logical processors and 64 GiB physical memory; unrelated Go jobs
  were active during part of the run.
- Module: repository `go.mod`; `GOFLAGS` empty.
- Git: `HEAD == origin/main == fea2e30a499384182d2fabe7038767e3c2f9c5e5`;
  worktree was clean before the baseline commands.
- Cost/network: controlled local fixtures only, zero remote calls, `$0` paid
  cost. Output containing temporary paths, IDs, and fixture payloads was not
  copied into this ledger.

## GATE-BASE-C14: procedure and result

The discovery and timing commands were run from the unchanged source head.
Each package timing command was run three times alone and sequentially before
the next package. The outer wall sample is the temporary command-wrapper
lifetime; the package sample is the `go test` reported elapsed value.

```text
go test ./tests/functional/factory/packaged/catalog -list .
go test ./tests/functional/factory/packaged/tts -list .
go test ./tests/functional/factory/packaged/fix -list .

go test ./tests/functional/factory/packaged/catalog -count=1  # x3, sequential
go test ./tests/functional/factory/packaged/tts -count=1      # x3, sequential
go test ./tests/functional/factory/packaged/fix -count=1      # x3, sequential

go test -json -count=1 ./tests/functional/factory/packaged/catalog
go test -json -count=1 ./tests/functional/factory/packaged/tts
go test -json -count=1 ./tests/functional/factory/packaged/fix

go test -count=1 -test.trace=<temporary>/catalog.trace ./tests/functional/factory/packaged/catalog
go test -count=1 -test.trace=<temporary>/tts.trace ./tests/functional/factory/packaged/tts
go test -count=1 -test.trace=<temporary>/fix.trace ./tests/functional/factory/packaged/fix
```

All 18 package commands exited `0`; the JSON runs reported 35, 14, and 21 test run
records respectively. The explicit `-test.trace` form produced the three full
trace artifacts used below. The temporary traces and derived pprof streams
were outside the repository and are not handoff artifacts.

### Three isolated samples per package

| Package | Run | Wall seconds | Package seconds | Exit |
| --- | ---: | ---: | ---: | ---: |
| Catalog | 1 | 151.118 | 149.110 | 0 |
| Catalog | 2 | 169.745 | 167.266 | 0 |
| Catalog | 3 | 42.936 | 40.561 | 0 |
| TTS | 1 | 30.048 | 28.076 | 0 |
| TTS | 2 | 25.649 | 23.927 | 0 |
| TTS | 3 | 29.792 | 27.699 | 0 |
| Fix | 1 | 25.539 | 23.569 | 0 |
| Fix | 2 | 23.173 | 21.365 | 0 |
| Fix | 3 | 24.038 | 22.266 | 0 |

| Package | Sorted package samples | Valid median |
| --- | --- | ---: |
| Catalog | `40.561s`, `149.110s`, `167.266s` | `149.110s` |
| TTS | `23.927s`, `27.699s`, `28.076s` | `27.699s` |
| Fix | `21.365s`, `22.266s`, `23.569s` | `22.266s` |

The Catalog first two samples were strongly affected by concurrent host Go
work, while TTS and Fix also show ordinary host variance. The samples remain
the required current-head denominator; they are not a quiet-host threshold or
a final performance verdict. No package failed, so no baseline retry was
needed.

## Executable denominator and assertion inventory

The three package suites contain 61 declared behavior cells from the PRD case
groups (Catalog 32, TTS 12, Fix 17) plus nine aggregate parent records, for
exactly 70 current run records. Aggregate records are retained in the count;
their named children are listed below. The two `ALL-N` edge statements are
non-applicable negative claims and intentionally have no executable test row.

### Catalog: 13 top-level and 22 named records

| Current run record(s) | PRD matrix coverage | Observable assertion family |
| --- | --- | --- |
| `TestEveryPackagedFactoryIsInvocableFromASingleTextPrompt` | `CAT-H01` | Every packaged invocation signature remains reachable from one unstructured text prompt, with the documented Fix exception. |
| `TestPackagedFactoryCatalogListsEveryEmbeddedFactory` | `CAT-H02`, `CAT-B01` | Embedded/runtime membership and stable catalog ordering agree. |
| `TestPackagedFactoryCatalogHasUniqueStableNames` | `CAT-H03` | Name, slug, project, and uniqueness metadata are complete and stable. |
| `TestNewEmbeddedFactoryRequiresFunctionalMatrixEntry` | `CAT-H04` | New embedded definitions have project and invocation-matrix bindings. |
| `TestPackagedFactoriesAPI_ReturnsPublishedCatalog` | `CAT-H05` | API-owned loopback returns complete published JSON/YAML, descriptions, examples, and bindings. |
| `TestFactoryListProjectsEffectiveCatalogWithoutInitialization` | `CAT-U01` | Project/global precedence is projected without initializing the customer home. |
| `TestFactoryListReportsCatalogDiscoveryFailuresAtomically` and `/project`, `/global` | `CAT-U02`, `CAT-X01` | Source failures are atomic, diagnostic, and do not expose partial catalog output. |
| `TestFactoryListHonorsPreCanceledContextAtomically` | `CAT-U03` | Pre-cancellation stops discovery without partial output. |
| `TestLocalFactoryOverridesPackagedFactoryWithSameName` | `CAT-H06` | Local same-name Factory wins and packaged fallback is not silently selected. |
| `TestInvalidLocalOverrideDoesNotFallBackSilently` | `CAT-H07` | Invalid local shadowing remains visible as an error rather than falling back. |
| `TestUnrelatedLocalFactoryDoesNotHidePackagedFactories` | `CAT-H08` | Unrelated local entries coexist with the packaged catalog. |
| `TestPackagedFactoryDefinitionsValidateThroughPublicCLI` and `/@you/fix`, `/@you/ralph` | `CAT-H09` | Public CLI validation accepts the two packaged definitions. |
| `TestPackagedFactoriesRejectMissingRequiredInputs` and the 18 named rows below | `CAT-U04`–`CAT-U21` | Every required-input Factory rejects empty TTY input, names the Factory/input, has no completed result, and makes zero provider calls. |

The 18 required-input child records are, in execution order:

```text
@you/agy-clip-qa  @you/agy-cold-watch  @you/classify  @you/deep-research
@you/fix          @you/full-flow       @you/fusion    @you/goal
@you/loop         @you/plan-execute    @you/plan-parallel  @you/quorum
@you/ralph        @you/review          @you/spawn     @you/subagent
@you/tournament   @you/tts
```

The source assertions are in `acp_invocability_test.go`, `discovery_test.go`,
`publication_test.go`, `override_test.go`, `required_inputs_test.go`, and
`shared_fixture_test.go`. They retain ordering, metadata, precedence,
validation, atomic failure, cancellation, missing-input, and privacy-probe
assertions; no assertion was removed or weakened.

### TTS: 6 top-level and 8 named records

| Current run record(s) | PRD matrix coverage | Observable assertion family |
| --- | --- | --- |
| `TestPackagedTTSNoServerPromptUsesCanonicalInputContract` | `TTS-H01` | Exact text binding, command/model selection, audio bytes, and response identity. |
| `TestPackagedTTSLocalRuntimePayloadPreservesExactBoundText` | `TTS-H02` | Local Models binding, host lifecycle, exact input, and one inference call. |
| `TestDeliveredPackagedTTSFactoryReachesProtocolFixture` | `TTS-R01` | Delivered binary builds/runs the protocol fixture, records live events, and replays without another protocol call. |
| `TestFactoryTTSModelsRootBuildProcessExecuteRecordsAudio` | `TTS-R02` | Root-built Models execution preserves audio Work/Event correlation, digest, metadata, and recording. |
| `TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections` and `/success`, `/failure` | `TTS-R03`, `TTS-U01` | Live/replay success and failure projections are equivalent; replay makes zero second model call and failures create no artifact. |
| `TestPackagedTTSSharedScenarios` and its six children | `TTS-H03`, `TTS-H04`, `TTS-H05`, `TTS-U02`, `TTS-U03`, `TTS-C01` | Required input, Work/Event success, optional voice/format, generic failure, packaged-model failure, and concurrent success/failure isolation retain exact request, session, dispatch, event, artifact, and cleanup identities. |

The six shared children are `required_input`, `success_work_events`,
`optional_voice_format`, `generic_failure`, `packaged_model_failure`, and
`concurrent_success_failure_isolation`. Their source assertions are in
`invocation_test.go`, `local_runtime_invocation_test.go`,
`models_replay_test.go`, `models_replay_helpers_test.go`,
`shared_fixture_test.go`, `shared_command_runner_test.go`, and
`shared_concurrency_test.go`. The delivered row proves the local artifact
protocol only; it does not claim real VibeVoice availability.

### Fix: 5 top-level and 16 named records

| Current run record(s) | PRD matrix coverage | Observable assertion family |
| --- | --- | --- |
| `TestPackagedFixUsesNamedWorktreeAndIndependentReview` | `FIX-H01` | Real local Git repository/worktree, planner/iterator/reviewer role order, result, and cleanup. |
| `TestPackagedFixSharedProcess` and its children | `FIX-H02`–`FIX-H06`, `FIX-U01` | Operator/configured role defaults, rejection feedback, exhaustion, and CLI/session parity. |
| `TestPackagedFixSharedProcess/CleanupPathCensus` and its six children | `FIX-CL01`–`FIX-CL06` | Success, rejection, failure, cancellation, timeout, and assertion-failure cleanup causes. |
| `TestPackagedFixRejectsMissingAndUnsafeWorktreeNamesBeforeProviderExecution` and `/missing`, `/path_traversal` | `FIX-U02`, `FIX-U03` | Missing/traversing names fail before provider execution. |
| `TestPackagedFixWorktreeCreationFailureIsStable` | `FIX-U04` | Non-Git/worktree creation failure is stable and side-effect bounded. |
| `TestPackagedFixWorkerFailureIsStable` | `FIX-U05` | Provider failure remains failed without a false result. |

The shared children are `UsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted`,
`CarriesIndependentRejectionFeedback`, `UsesConfiguredAndExplicitRoleModels`
with `installed_worker_configuration` and `explicit_role_flags`,
`ReviewLoopExhaustionIsStable`, `CLIResponseMatchesExplicitSession`, and
`CleanupPathCensus` with `success`, `rejection`, `failure`, `cancellation`,
`timeout`, and `assertion-failure`. The source assertions are in
`invocation_test.go`, `resource_census_test.go`, `forced_unwind_test.go`, and
`shared_fixture_test.go`.

## Resource and process topology census

Counts below are source/call-site expansion reconciled with the passing JSON
run and trace. They describe the pre-change denominator, not optimization
targets that may be applied without the later story's isolation proof.

| Package | Root/process topology | Host and controlled-edge topology | Setup, identities, and cleanup ownership |
| --- | --- | --- | --- |
| Catalog | One lazy shared `BuildProcessWithContext` root (`shared_fixture_test.go:176-234`); all customer rows use `Process.Execute`; no delivered CLI or remote OS process. | One API-owned `ProcessAPIServer`/listener for the cached HTTP discovery row; one swappable provider runner; all 18 required-input children assert zero provider calls; no model host. | 20 installed packaged Factory copies (18 required-input plus two validation), one API discovery scaffold, and fresh temp home/work directories. `TestMain` closes the shared root and verifies the API listener. |
| TTS | Eight root builds after expanding the direct/replay helpers: one no-server, one local-runtime, one root Models, four Models live/replay, and one shared scenario root. The delivered row separately builds one `you` binary and launches two real child invocations (live/replay). | One shared application API host (`processBuilds=1`, `api.startCount=1`); seven local model protocol servers (one local-runtime, one delivered, five Models-edge calls) and six fake model-host launcher lifecycles. Provider command and Models backends are controlled edges. | Seven packaged installs, six named copies plus two generic scaffolds across direct/shared rows, per-live/replay cache and recording roots, seven explicit shared child sessions including two concurrent sessions, and per-scenario route/session/artifact cleanup. |
| Fix | Five root builds: one shared continuous fixture and four independent CLI failure/validation roots. The shared process runs in-process; real Git subprocesses and real worktree operations remain intentional. | One shared application API host and continuous command; one selector-keyed provider edge with fresh route registrations; no delivered CLI or remote provider. | One shared packaged seed/template plus per-scenario Factory copies; independent rows install their own packaged Factory and initialize real Git repositories. Every scenario keeps unique Factory/workspace/worktree/request/Work/plan/session identities; `resource_census_test.go` owns seven cleanup causes and zero-residue checks. |

Retained isolation reasons are explicit: Catalog source-failure delegates must
reset between sequential rows; TTS replay/model/cache/artifact and delivered
binary rows must retain their distinct lifecycle boundaries; Fix must retain
real Git/worktree behavior and unique selector/session/path identities. No
whole-package parallelism or shared-support change is justified by this
baseline.

### Wait and polling topology

- Catalog has API `WaitForBaseURL` and `WaitForStatus` at the one hosted row,
  plus bounded shared-process close. Cancellation is pre-canceled context
  observation; there is no scenario sleep.
- TTS has one shared API `WaitForURL`/`WaitForStatus` startup, per-session
  terminal observation (two in the concurrent child), model-host channel
  `Wait`/`Stop`, and deterministic route/session cleanup. There is no new sleep
  or timeout-padded synchronization helper.
- Fix has bounded API base/status and terminal-session observation. The only
  `time.After` sites are documented startup/teardown safety ceilings in
  `shared_fixture_test.go:261-270` and `:291-304`; the done channel is the
  normal lifecycle observation. Port closure uses a bounded dial probe, not a
  sleep.

## Profile findings and ranked candidates

The valid full traces were converted with `go tool trace -pprof=sync`,
`sched`, `syscall`, and `net`, then inspected with `go tool pprof -top`.
Trace delay totals are cumulative across goroutines and therefore are not
added to wall time.

| Package | Dominant observed profile cost | Characterization candidate for the next story |
| --- | --- | --- |
| Catalog | Sync delay `671.68s`: `runtime.chanrecv1` `530.94s`/`79.05%`, `runtime.selectgo` `139.56s`/`20.78%`. Syscall delay `135.09s`; `Initializer.InitializeSystem` `118.60s`, `Process.Execute` `122.62s`, with filesystem `ReadFile` `40.04s`, `MkdirAll` `17.49s`, `ReadDir` `15.80s`, `Stat` `17.22s`, `WriteFile` `9.99s`, and `RemoveAll` `8.38s`. JSON's dominant row is the 18-child required-input aggregate at `55.49s`; `/@you/classify` is `15.99s`. | Highest-value bounded candidate is reuse of immutable packaged installation/source setup across the required-input rows while recreating each home/work/input boundary and retaining every zero-call assertion. The cached HTTP catalog and one shared root are already characterized; no final improvement is claimed. |
| TTS | Sync delay `434.93s`: `chanrecv1` `318.23s`/`73.17%`, `selectgo` `116.60s`/`26.81%`. Syscall delay `72.38s`; initialization `50.26s`, `Process.Execute` `56.56s`, `ReadFile` `16.51s`, `MkdirAll` `8.62s`, `Stat` `5.94s`, and `RemoveAll` `5.30s`. Network delay `68.72s`, of which TCP `Accept` is `62.80s`. JSON's largest rows are Models success/failure replay `15.08s`, delivered binary `8.20s`, and shared scenarios `4.22s`. | Reuse only compatible Models root/cache/host setup around the four live/replay root builds and repeated install work, while retaining a separate delivered-binary build/run, zero-call replay, artifact lineage, and concurrent edge. |
| Fix | Sync delay `416.35s`: `chanrecv1` `214.72s`/`51.57%`, `selectgo` `190.62s`/`45.78%`, and `sync.Cond.Wait` `7.72s`. Syscall delay `52.31s`; controlled command-runner execution is visible at `4.04s`, while scheduler monitoring contributes `23.65s` cumulative. Network delay `33.67s`, including `Accept` `9.43s` and connection reads `24.22s`. JSON's largest rows are unsafe-name validation `7.80s`, named worktree success `6.53s`, CLI/session parity `5.76s`, and provider failure `4.81s`. | Reuse the already shared continuous root for compatible journeys and reduce repeated install/Git setup in the four independent CLI rows only where real local Git, provider failure, and cleanup-cause witnesses remain intact. |

The profile ranking is a candidate set for stories `...-002`, `...-003`, and
`...-004`, not permission to broaden this baseline story. In particular, the
delivered binary, replay, concurrency, failure, cancellation, real Git, and
cleanup paths are retained isolation requirements rather than redundant work.

## Gate verdict and remaining edges

| Criterion | Result | Evidence and boundary |
| --- | --- | --- |
| Three isolated samples and medians | PASS | Nine sequential package commands exited `0`; table above records wall/package samples, medians, source head, OS, and Go environment. |
| Complete 70-record denominator | PASS | `-list .` discovery and one JSON run produced 35 Catalog, 14 TTS, and 21 Fix records; every top-level and named record is mapped above. |
| Dependency fidelity | PASS | Root-built Process.Execute paths, Catalog API loopback, delivered TTS binary/local protocol fixture, local Models, and real local Git/worktree boundaries all executed; remote/paid calls were zero. |
| Topology/profile/candidate characterization | PASS | Source census, wait topology, full traces, derived sync/scheduler/syscall/network profiles, and ranked safe candidates are recorded. Cumulative profile delay is not presented as wall time. |
| Final improvement or measured floor | NOT CLAIMED | Post-change optimization, final medians, 40-percent deltas, or a measured floor belong to `GATE-CAT-OPT`, `GATE-TTS-OPT`, `GATE-FIX-OPT`, and `GATE-PERF-C14`. |
| Repeat/race after structural change | NOT CLAIMED | Shared-state repeat and race evidence belongs to the optimization stories after a change. |
| Rebase, final diff, clean-room loopback, PR CI, merge | NOT CLAIMED | Owned by story `...-005` and the review stage. |

GATE-BASE-C14 therefore passes for the story's named denominator,
assertion/dependency map, pre-change resource topology, profile-backed
candidate set, and remaining-edge properties. The next stories must use this
ledger as their before state and must reconcile any base change before
comparing results.

## GATE-CAT-OPT: seed/copy setup reduction and result

The Catalog profile selected the repeated packaged-installation setup in the
18 required-input child rows. The implementation keeps one root-built public
bootstrap as a read-only seed, then copies only the selected packaged Factory
and a valid per-row system-config template into each fresh row home. Each row
still runs its actual `you --json run --named ... --no-record` command through
the shared root process with a fresh working directory, TTY stdin, environment,
and provider recorder. The copied config receives a new local backend scope so
row identity remains isolated.

### Procedure and measured result

The pre-change focused command was run before the fixture edit from the
baseline parent; the after command used the current Catalog source. Both used
the local-real root composition and controlled provider edge on the same
Windows host described above.

```text
go test ./tests/functional/factory/packaged/catalog \
  -run '^TestPackagedFactoriesRejectMissingRequiredInputs$' -count=1 -v
go test ./tests/functional/factory/packaged/catalog/... -count=1 -v
go test ./tests/functional/factory/packaged/catalog/... \
  -run '^TestPackagedFactoriesRejectMissingRequiredInputs$' -count=3
go test -race ./tests/functional/factory/packaged/catalog/... \
  -run '^TestPackagedFactoriesRejectMissingRequiredInputs$' -count=1
```

| Evidence | Before | After | Exit |
| --- | ---: | ---: | ---: |
| Required-input focused test elapsed | `133.76s` | `65.45s` | `0` |
| Complete Catalog package elapsed | `149.110s` baseline median | `78.029s` observed run | `0` |
| Required-input repeat gate | not applicable before edit | `95.998s` for `-count=3` | `0` |
| Required-input race gate | not applicable before edit | `61.411s` for `-race -count=1` | `0` |

The focused required-input slice observed a `51.1%` wall reduction on this
shared host. The complete-package after value is one post-change sample, not
the lane's final three-run Catalog median; `GATE-PERF-C14` remains responsible
for that denominator and verdict.

### Catalog operation and identity handoff

The baseline assertion inventory above remains unchanged: 13 Catalog
top-level records plus 22 named records, including all 18 required-input
identities. The full package run passed all 35 Catalog records. The focused
repeat and race runs passed every required-input identity and retained the
existing assertions for Factory/input diagnostics, non-completed results, and
zero provider calls.

| Operation or identity boundary | Before | After | Ownership/isolation retained |
| --- | ---: | ---: | --- |
| Root builds for Catalog package | `1` shared | `1` shared | `TestMain` closes it once; no production root change |
| Public system-bootstrap probes for required-input rows | `18` | `1` seed | Seed is established through `InstallPackagedFactoryWithProcess` |
| Public packaged-install setup calls for required-input rows | `18` | `1` seed | The selected Factory remains published/validated by the public path |
| Package-local selected Factory copies | `0` | `18` | One fresh home and selected `@you/*` directory per row |
| Package-local config template writes | `0` | `18` | Fresh config path and unique `local-<uuid>` scope per row |
| Required-input CLI behavior invocations | `18` | `18` | Root-built `Process.Execute`, empty TTY stdin, fresh env/workdir |
| Provider calls observed by required-input assertions | `0` | `0` | Per-row recorder remains installed and checked |
| API host / cached discovery response | `1 / 1` | `1 / 1` | Existing API-owned loopback and immutable response cache unchanged |

No Factory definition, API, CLI contract, shared support, production, or
other package file changed. Source-failure delegates continue to reset after
their sequential rows, the forced-unwind report remains owned by `TestMain`,
and no sleep or timeout was added. The retained unproven edges are final
three-run medians or a measured floor, TTS and Fix optimization, touched
integrated loopback, current-head CI, and merge.

## GATE-TTS-OPT: shared Models live/replay fixture and result

The TTS profile selected the four Models live/replay root-built executions and
their repeated packaged-install/model-host setup. This bounded slice reuses
one root-built `Process.Execute` graph, one public packaged-Factory seed, one
ready-model cache seed, and one model protocol/launcher edge. Each success or
failure pair still receives a fresh customer home, copied `@you/tts` Factory,
unique backend scope, cache copy, recording, and artifact boundary; replay
uses its live pair's recording and makes no second model call. The delivered
binary row and shared concurrent scenario were not changed.

### Procedure and measured result

The focused command was run against the current source after the fixture
change. The repeat, race, and full-package commands exercised the same local
root composition and controlled Models edge. Expected failure logs in the
failure witness are part of the assertion and did not affect the exit status.

```text
go test ./tests/functional/factory/packaged/tts \
  -run '^TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections$' \
  -count=1 -v
go test ./tests/functional/factory/packaged/tts \
  -run '^TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections$' \
  -count=3
go test -race ./tests/functional/factory/packaged/tts/... \
  -run '^TestFactoryTTSModelsSuccessAndFailureReplayPreservePublicProjections$' \
  -count=1
go test ./tests/functional/factory/packaged/tts/... -count=1
```

| Evidence | Before | After | Exit |
| --- | ---: | ---: | ---: |
| Models live/replay focused elapsed | `~10.63s` characterization sample | `8.105s` | `0` |
| Focused repeat gate | four independent executions | `27.539s` for `-count=3` | `0` |
| Focused race gate | not applicable before edit | `19.234s` for `-race -count=1` | `0` |
| Complete TTS package | `34.767s` characterization sample | `29.488s` | `0` |

The elapsed values are observed host samples, not the final three-run TTS
median or a quiet-host threshold. The structural reduction and behavior
evidence are the gate result for this story; `GATE-PERF-C14` owns the final
median/floor verdict.

### TTS operation and identity handoff

The TTS assertion inventory remains unchanged at six top-level and eight named
records. The focused test still proves completed audio Work/event correlation,
digest, metadata, lineage, recording order, host lifecycle, replay projection
equivalence, zero replay model calls, failure status, and no failure artifact.
The run log observed one backend call for each live scenario and none during
replay; the fixture cleanup observed balanced model-host starts/stops (`2/2`).

| Operation or identity boundary | Before | After | Ownership/isolation retained |
| --- | ---: | ---: | --- |
| Root builds for the Models live/replay helper | `4` | `1` | One root remains public and is closed by fixture cleanup |
| Public packaged-Factory seed installs | `2` | `1` | Seed is materialized through `InstallPackagedFactoryWithProcess` |
| Model-ready cache initialization | `2` | `1` seed | Two pair-local cache copies remain, one per success/failure pair |
| Model protocol/launcher fixture instances | `4` | `1` | Shared health server/client and launcher edge; live host lifecycle remains `2/2` |
| Models inference calls | `2` live + `0` replay | `2` live + `0` replay | Mutable backend resets per pair; replay cannot call the backend |
| Factory sessions | `4` | `4` | Live/replay success and failure sessions remain independently observed |
| Factory/cache/recording/artifact pair boundaries | `2` pairs | `2` pairs | Fresh homes, Factory copies, caches, recordings, and artifact paths remain |
| Delivered binary build/launch | `1` build / `2` launches | unchanged | Actual delivered protocol witness remains separate |

No assertion was removed or weakened, no production/shared-support file
changed, and no sleep, remote call, paid call, or real VibeVoice claim was
introduced. Remaining unproven edges are the final three-run TTS median or
measured floor, the final Fix median or measured floor, final rebase, clean-room loopback,
current-head CI, PR review, and merge.

## GATE-FIX-OPT: reusable CLI root and real Git metadata seed

The Fix profile selected repeated non-hosted CLI root construction, packaged
Factory materialization, and local Git repository setup. The implementation
adds a package-local serialized CLI process with one public `@you/fix` seed and
fresh per-invocation homes, backend scopes, selected Factory copies, and
provider-selector registrations. The existing continuous API/session fixture
is unchanged. Every customer CLI invocation still crosses `Process.Execute`;
the selector-keyed provider edge, real local Git/worktree operations, provider
failure, and non-Git failure rows remain intact.

The Git optimization creates one real repository with the same user config and
empty initial commit, then copies its `.git` metadata into each scenario. The
copied repositories remain independent local-real Git repositories. The
non-Git failure and path-validation rows do not receive a Git seed, and runtime
worktrees remain per scenario.

### Procedure and measured result

The commands below ran on the current source with local-real Git/filesystem
behavior and controlled provider command edges. Expected provider, validation,
and non-Git failure logs remained part of passing assertions.

```text
go test ./tests/functional/factory/packaged/fix/... -count=1 -v
go test ./tests/functional/factory/packaged/fix/... \
  -run '^TestPackagedFix(UsesNamedWorktreeAndIndependentReview|SharedProcess|RejectsMissingAndUnsafeWorktreeNamesBeforeProviderExecution|WorktreeCreationFailureIsStable|WorkerFailureIsStable)$' \
  -count=3
go test -race ./tests/functional/factory/packaged/fix/... -count=1
```

| Evidence | Before | After | Exit |
| --- | ---: | ---: | ---: |
| Complete Fix package | `22.266s` baseline median | `21.005s` observed run | `0` |
| Complete Fix focused repeat | not applicable before edit | `77.283s` for `-count=3` | `0` |
| Complete Fix race gate | not applicable before edit | `42.164s` for `-race -count=1` | `0` |

The one after-package sample is an observed host value, not the lane's final
three-run median or a 40-percent verdict; `GATE-PERF-C14` owns that final
denominator and any measured-floor disposition.

### Fix operation, identity, and cleanup handoff

The Fix inventory remains five top-level and 16 named records. The full run
retained all 21 records and the existing exact role, prompt, response, Work,
Factory Event, replay, provider-call, local-real Git, validation, failure, and
cleanup assertions.

| Operation or identity boundary | Before | After | Ownership/isolation retained |
| --- | ---: | ---: | --- |
| Fixture root processes | `5` characterized roots | `2` fixture roots (`1` continuous + `1` serialized CLI) | The continuous API/session root remains distinct; customer CLI calls are serialized on the non-hosted root. |
| Public packaged-Factory seed setup | Repeated per independent CLI root | `1` CLI seed install plus the existing `1` continuous seed | Each CLI call receives a fresh selected `@you/fix` copy and config. |
| CLI customer executions | Six retained executions | Six retained executions | Every invocation still uses public `Process.Execute`; no scenario was deleted or merged. |
| CLI Factory copies/config scopes | Repeated materialization | `6` Factory copies and `6` fresh config scopes | Homes, backend scopes, workspaces, and provider selectors remain unique. |
| Real Git repository setup | Nine scenario repositories each ran init/config/commit setup | One real seed init/config/commit plus `9` `.git` metadata copies | Copied repositories remain local-real; non-Git and path-validation failures stay uninitialized. |
| Explicit test-owned Git worktree setup | `1` unrelated worktree add | `1` unrelated worktree add | The named Fix runtime still creates and asserts its requested worktree; unrelated worktree preservation remains covered. |
| Provider edge | Per-scenario controlled runner | Selector-keyed per-scenario runners | Success/rejection role counts, zero-call validation/Git failure, and worker failure bounds remain asserted. |
| Shared resource census | Existing six scenario identities | `6` unique factories/workspaces/worktrees/requests/Works/plans; `255` event IDs | `selectors=0`, all six roots/sessions/definitions/workspaces/worktrees/plans/runtime/replay resources absent. |
| Cleanup causes | Existing seven cause paths | `success=7,rejection=4,failure=1,cancellation=2,timeout=1,assertion-failure=1,package-teardown=1` | Every declared cleanup cause remains observed; original failure causes are not hidden. |

No assertion was removed or weakened, no production/shared-support file
changed, and no new sleep, timeout increase, remote call, paid call, provider
substitute outside `Edges`, or resource leak was introduced. Remaining
unproven edges are the final three-run Fix median or measured floor, final
rebase, clean-room loopback, current-head CI, PR review, and merge.

## Handoff artifacts

- Durable ledger: `docs/internal/development/functional-test-optimization/c14-packaged-catalog-tts-fix.md`.
- Ignored task state: `prd.json` marks stories `...-001` through `...-004`
  `passes:true`; `progress.txt`
  records the concise iteration handoff and reusable patterns.
- No generated, production, API, shared-support, workflow, baseline, CI, or
  sibling-package file changed; the Catalog change is confined to
  `tests/functional/factory/packaged/catalog/required_inputs_test.go`, and the
  TTS change is confined to
  `tests/functional/factory/packaged/tts/{local_runtime_invocation_test.go,models_replay_helpers_test.go,models_replay_test.go}`.
  The Fix change is confined to
  `tests/functional/factory/packaged/fix/{invocation_test.go,shared_fixture_test.go}`.
- Unproven edges handed to later gates: final three-run medians or
  operation-level floors, clean-room loopback,
  current-head CI, and merge.
