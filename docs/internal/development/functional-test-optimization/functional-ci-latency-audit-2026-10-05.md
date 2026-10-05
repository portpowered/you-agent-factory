# Functional CI latency audit

Measured October 5, 2026. Local source revision:
`73d83286167b941e3f2f668bcc7425926481fd99`.

## Finding

The initial measurements below are followed by an [implementation and validation
update](#implementation-and-validation-update) for the authorized cleanup.

The three-minute goal is not met. Two successful Linux functional jobs took
**476 seconds and 622 seconds**. Their authoritative coverage invocations took
**344.601 seconds and 421.149 seconds**. The work is split between repeated
compilation/linking, expensive customer scenarios, and work outside the measured
coverage invocation. Reducing timeout constants or enabling more parallelism
alone will not solve it.

The highest priorities are:

1. Isolate model caches and customer profiles, then remove repeated bootstrap
   and application construction where immutable process reuse is safe.
2. Profile the slowest customer journeys on the actual four-CPU Linux runner.
3. Reduce the approximately 146 full-application test binaries, while retaining
   scenario concurrency and customer guarantees.
4. Fix synchronization failures and move fixture, source-shape, and executable
   tests into their correct lanes.
5. Cache the exact coverage build graph and account for the entire harness,
   not just `go test` package durations.

One optimization was demonstrated without modifying committed tests: a private
empty model cache reduced an isolated Models test's median package execution
from **15.929 seconds to 1.220 seconds**, with all assertions passing. Package
consolidation reduced link work but did **not** demonstrate an overall latency
improvement in the tested group.

## Evidence and measurement boundaries

### Hosted CI

These are two observed samples, not a statistical baseline or a controlled
before/after comparison. They ran different revisions:

| Measurement | Earlier run | Later run |
| --- | ---: | ---: |
| Run | [37262603762](https://github.com/portpowered/infinite-you/actions/runs/37262603762) | [37270620737](https://github.com/portpowered/infinite-you/actions/runs/37270620737) |
| Revision | `11e0ad42e37699ebe28f56cc496a3fa6f76bd0d0` | `8079404795e9799312144eb52a0805aad456e205` |
| Whole Functional Coverage job | 622s | 476s |
| Supervisor/coverage child | 572.221s | 425.438s |
| Measured coverage invocation | 421.149s | 344.601s |
| Difference: coverage child minus measured invocation | 151.072s | 80.837s |
| Concurrent quarantine validator | 125.515s | 56.502s |
| Selected packages | 149 | 149 |
| Top-level tests in timing summary | 1,064 | 1,064 |
| Summed package execution time | 2,782.136s | 2,248.576s |
| Compiler commands in build diagnostics | 892 | 864 |
| Linker commands in build diagnostics | 146 | 146 |

The later job log reports **four logical CPUs and 12 package jobs**. Its
dependency build cache restored successfully in about two seconds. The earlier
job did not have that restore step. This is evidence of cache behavior, not
proof that the cache caused the entire difference between runs.

The earlier run retained an initial failure of
`TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` in
`workers/mock` and completed successfully with retry machinery. Keep the
slower successful sample in the range; a green job is not evidence that its
first attempt was clean.

The quarantine validator overlaps coverage. Its duration must **not** be
added to the coverage child's duration. Likewise, the 80.837–151.072-second
difference is unattributed harness time, not a measured compiler duration.
It includes work before/after the timed invocation and can include retry work
and reporting. The invocation timing excludes the enclosing `go run`
commands, discovery, and other checker/report phases.

### Local experiments

Host: Windows/amd64, Go 1.25.0, 24 logical processors, approximately 64 GiB RAM.
The build cache was inherited; it was not globally cleared. Other unrelated
Go workloads were observed on this machine. Local numbers explain mechanisms
and are not substitutes for measurements on the Linux CI runner.

All executions used `-count=1`, so test-result caching cannot explain their
speed. Broad diagnostic commands explicitly used `-p=12` to reproduce CI's
package budget. This is a diagnostic configuration, not a proposed change to
the canonical Windows lane.

Measurement methods:

- Outer subprocess stopwatch: complete command wall time and exit status.
- `go test -json`: package and individual test execution durations/outcomes.
- Go 1.25's internal `-debug-trace`: package-loading and build-action spans.
- A temporary `-toolexec` wrapper: actual tool subprocess wall time, user CPU,
  system CPU, package attribution, and exit status.
- CPU and blocking profiles for the slow Models named-route scenario.

Summed tool wall time and summed package time represent concurrent work, not
elapsed job time. CPU seconds can exceed wall seconds because tools use
multiple threads. Trace unions show time with at least one action of a class
active; different classes overlap and their unions are not additive.

The first two tool-timer attempts had filename collisions. Their overall
command timings and Go traces remain valid, but their tool counts/CPU sums
are incomplete and are excluded from the authoritative tool totals below.
The corrected forced-rebuild capture has no malformed records and accounts
for all 145 linked packages.

## What CI actually runs

The required lane is:

```text
ci.yml: Backend Functional Coverage
  run-functional-coverage-with-quarantine.sh
    concurrent: verify-functional-quarantine-packages.sh
    concurrent: make functional-test-viz
      go run ./cmd/functionaltestviz
        go run ./cmd/gocoveragecheck
          discover current packages/tests and subtract quarantine
          go test -coverpkg=<backend packages> -p=12
                  -covermode=count -timeout=10m -json -x ...
          coverage checks, failure capture/retry, reports
        render diagnostics and verdict
```

This is full, non-short coverage; it is not equivalent to the default short
`make test-functional` command. The captured invocation instruments **556
backend packages**. There are 157 discovered functional packages, eight
package exclusions, and one quarantined live-test selector. The selected
inventory is 149 packages and 1,064 top-level tests.

The current functional dependency-cache workflow builds ordinary standard and
third-party dependencies, but does not preserve the complete project coverage
graph or the functional test binaries. Cache hits therefore do not mean that
the required coverage compilation/linking work disappeared. Both observed
jobs still compiled hundreds of packages and linked 146 executables.

The quarantine script also invokes the coverage checker for validation and
runs exact-package `go test -list` commands for default and `functionallong`
tags. This is additional compilation/discovery pressure even though it is
off the critical path once coverage takes longer. Preserve its validation
guarantees while avoiding duplicate discovery or unnecessary test linking.

## Highest package execution latencies

These durations come from hosted `go test` package outcomes and exclude
package compilation/linking. Paths are relative to `tests/functional/`.

| Package | Later run | Earlier run | Audit focus |
| --- | ---: | ---: | --- |
| `models/root_composition` | 73.678s | 89.668s | Model cache/readiness IO; repeated application/server fixtures |
| `factory/review_failure_routing` | 58.564s | 73.670s | Shared host bootstrap/profile ownership; scenario IO and polling |
| `sessions/chat_sessions/root_composition` | 49.552s | 60.637s | ACP journeys, profile bootstrap, public completion/cleanup |
| `factory/visualization/runtime_metrics` | 49.413s | 56.701s | Multiple immutable fixtures, selected-clock/readiness phases |
| `providers/acp` | 46.277s | 64.855s | Provider protocol lifecycle, cancellation, daemon/process boundaries |
| `sessions/execution` | 45.516s | 56.247s | Repeated process/server setup; result and response observation |
| `transport/mcp/stdio` | 35.309s | 42.964s | Protocol startup/shutdown and stream completion |
| `sessions/root_composition` | 35.133s | 43.630s | Recording/lifecycle matrices and repeated setup |
| `events/factory_events` | 34.960s | 43.396s | Webhook/fault journeys, repeated server setup, polling |
| `factory/replay_contracts` | 33.121s | 42.277s | Persisted/replay guarantees and fixture cost |
| `provider_sessions/details` | 32.468s | 43.680s | Transcript/session inspection fixtures |
| `factory/process_time` | 32.330s | 39.139s | Existing controlled-clock journeys and fixture overhead |
| `factory/packaged/named_invocation` | 30.198s | 40.488s | Multi-stage packaged customer journeys |

Notable individual hosted tests:

| Test | Later run | Earlier run |
| --- | ---: | ---: |
| `TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker` | 56.33s | 44.17s |
| `TestAPIResultAndResultsExposeTerminalInvocationData` | 25.27s | 25.93s |
| `TestCLISettingsUnknownFieldsPrecedenceAndSafeFailure` | 24.91s | 27.97s |
| `TestFactoryWebhooksRecordedFaultsAndPeerContinuation` | 23.11s | 18.14s |
| `TestPackagedDubVideoRunsFourStagesAndRejectsFailures` | 17.84s | 23.45s |

Do not sum individual test times to infer package wall time: parallel tests,
subtests, fixture setup, and cleanup make those different metrics.

At the observed package durations, ideal scheduling into 12 slots has a lower
bound of **187.381–231.845 seconds**, before build and harness work. This bound
holds for those measured durations; reducing contention or changing fixtures
can change the durations themselves. The current average occupied package
slots over the coverage invocation is only about 6.5–6.6, illustrating that
the lane also pays build barriers and uneven scheduling.

## Build, linking, instrumentation, and vetting

Three local covered build-only commands used `-run=^$`. They select no test
bodies, but still include package loading, compilation, coverage generation,
linking, vetting, test-process startup, and any `TestMain` work.

| Experiment | Wall | Outcome |
| --- | ---: | --- |
| Initial covered build, inherited cache | 77.820s | pass |
| Repeat, warm compiler cache | 50.731s | pass |
| Corrected timer, warm compiler cache | 49.197s | pass |
| Forced rebuild, `-a`, including standard/third-party dependencies | 111.587s | pass |

The local pattern `-coverpkg=github.com/portpowered/infinite-you/pkg/...`
instruments a broader set than CI's explicit 556-package list. It also
includes Windows-selected functional packages and support packages: 158
package outcomes and 145 link actions. Consequently, these are diagnostic
build experiments, not exact CI lane reproductions.

The forced rebuild's actual child-tool costs were:

| Tool | Calls | Summed tool wall | User CPU | System CPU |
| --- | ---: | ---: | ---: | ---: |
| Compiler | 1,251 | 243.901s | 144.000s | 85.531s |
| Linker | 145 | 266.389s | 192.141s | 106.672s |
| Vet | 1,111 | 143.242s | 37.063s | 35.703s |
| Coverage instrumentation | 644 | 61.902s | 9.406s | 13.500s |
| Assembler | 93 | 9.657s | 0.859s | 0.938s |

Linking consumed **298.813 child CPU seconds**, more than compilation's
**229.531**, and approximately **48% of the captured tool CPU total**. These
totals exclude `cmd/go`, the timer wrappers, test processes, and OS activity
outside the recorded children.

In the warm build, no compiler or coverage-tool calls were recorded, yet
145 link actions remained. At least one link action was active for **44.995
of 49.197 seconds**. A better compiler cache helps, but cannot by itself remove
this warm-build linking floor. The corresponding build-action union was
2.687 seconds. Build and link unions overlap with vet and test startup.

### Highest compiler latencies

Measured actual compiler subprocesses in the forced-rebuild sample:

| Package | Wall | User + system CPU |
| --- | ---: | ---: |
| `modernc.org/sqlite/lib` | 5.840s | 12.406s |
| `github.com/dop251/goja` | 3.766s | 9.016s |
| `runtime` | 1.238s | 3.172s |
| `pkg/transports/http/generated` | 1.218s | 2.000s |
| `packages/packaged-factories` | 1.111s | 0.063s |
| `tests/functional/models/root_composition_test` | 1.104s | 2.422s |
| `pkg/services/factory_sessions/internal/execution` | 1.091s | 2.234s |
| `net/http` | 1.045s | 2.703s |

These identify where expensive cold compilation is concentrated. Do not
rewrite SQLite, Goja, or generated contracts just to shave this sample. First
verify that CI's dependency cache hits for the actual build flags and retain
project coverage archives where useful.

### Highest warm linker latencies

| Test binary | Wall | User + system CPU |
| --- | ---: | ---: |
| `factory/packaged/named_invocation.test` | 4.190s | 2.906s |
| `factory/packaged/quorum.test` | 4.059s | 2.719s |
| `factory/packaged/plan_execute.test` | 3.990s | 2.656s |
| `factory/packaged/plan_parallel.test` | 3.940s | 2.672s |
| `factory/packaged/javascript_families.test` | 3.761s | 2.500s |

Individual links are only a few seconds, but almost every functional package
links the broad application dependency graph. The aggregate is the problem.

## Experiments on the proposed optimization approaches

### 1. Session/process reuse and cache isolation

The Models named-route test already calls `t.Parallel`, but creates a separate
API host using `StartFunctionalAPIServer`. Its configured edge only replaces
provider execution. It then lists/inspects effective models and checks two
named-route failures. Its missing-model/readiness contract does not need a
populated developer model cache.

The targeted CPU profile attributed **13.22 cumulative sample seconds** to
`InspectRuntimeCache -> inspectGenericRuntimeFiles -> verifyCachedFile ->
fileSHA256`, including **11.32 seconds** through model catalog listing. Windows
fsnotify/syscall frames also appear prominently. These profile samples are
diagnostic attribution, not additive phase durations or proof of the same
cause in Linux CI.

Three ordinary isolated invocations and three identical invocations with
`INFINITE_YOU_OMNIVOICE_CACHE_DIR` pointed at fresh audit-owned directories:

| Metric | Ambient cache | Private empty cache |
| --- | --- | --- |
| Command wall samples | 24.202, 19.420, 16.518s | 4.640, 3.811, 5.753s |
| Command wall median | 19.420s | 4.640s |
| Package execution samples | 19.855, 15.929, 13.658s | 1.220, 1.173, 1.438s |
| Package execution median | 15.929s | 1.220s |
| Verdicts | 3/3 pass | 3/3 pass |

That is approximately **76% less command latency and 92% less package
execution latency** in this local experiment. The scope of the evidence is
this one test and this host. Verify the gain on Linux before extrapolating it
to the 73–90-second package.

Static source review also found:

- `models/root_composition` has approximately 50 application-construction
  call sites across helpers and scenarios, plus repeated API-host setup.
  Different immutable injected edges may legitimately require separate
  processes; reuse only within compatible cohorts.
- `sessions/execution` has approximately 23 construction call sites and
  repeated server fixtures. Its 25-second result/results journey is a good
  next profile target.
- Review-failure routing already owns one shared process and opens explicit
  Factory Sessions. Recommending a fresh session conversion for the whole
  package would duplicate completed work. Its shared host uses `FakeInputs`,
  whose default environment is `os.Environ()`, without an invocation-local
  profile override in that construction path. Fix this remaining bootstrap
  ownership issue and measure it.
- Chat Session composition already has a controlled cohort with distinct
  Chat/Factory Sessions and working roots. Diagnose remaining setup and
  completion phases rather than replacing that fixture wholesale.
- The invalid generic CLI Models fixture holds `executeMu` across each
  invocation and observes aggregate effect counters. Its subtests can be
  marked parallel while the customer commands remain serialized. Attribute
  effects to the scenario before removing that lock.

Across all source files under `tests/functional`, a static scan found 280
explicit `root`/`support` construction sites, 56 `time.Sleep` sites, and 120
files mentioning `~default` or `DefaultSessionID`. These counts include support
code and tagged files and are triage signals, not runtime invocation counts
or evidence that every default-session assertion is invalid.

### 2. Timeouts, deterministic observation, and synctest

Go 1.25 already supports `testing/synctest`. It advances fake time when the
bubble's goroutines are durably blocked and provides `synctest.Wait` for
quiescence. Real network and filesystem IO are not automatically converted
into controlled fake effects. Use it for suitable isolated scheduling,
deadline, retry, and cancellation components; use injected clock/runner edges
and public events for whole-application journeys. See
[Testing Time](https://go.dev/blog/testing-time) and the
[synctest contract](https://pkg.go.dev/testing/synctest).

No functional source file currently imports `testing/synctest`. However,
`factory/process_time`, scheduling selected-time tests, and runtime-metrics
selected-time tests already implement controlled clock/timer edges. Their
32–39-second package times are not proof of real-time sleeping: profile their
application/bootstrap/IO work before replacing the clock implementation.

Good observation-conversion targets include the repeated pollers in
`sessions/root_composition/p3_p7_behavior_matrix_test.go`,
`events/factory_events/order_and_cursor_test.go`,
`recordings/root_composition/record_replay_lifecycle_activation_test.go`, and
`work/recovery/helpers_test.go`. Review-failure routing's Work-state polling
documents a real ordering distinction: a dispatch response can precede the
public Work projection. Replace it only with a signal that proves the
projection is visible, not merely with the earlier response event.

Keep safety timeouts as ceilings that return as soon as completion arrives.
Shortening a five-minute package timeout to three minutes would make the
observed blocked test fail earlier; it would not make its customer behavior
correct or its synchronization deterministic.

### 3. Joining test packages

Temporary source copies combined `plan_parallel`, `fusion`, and
`deep_research` into one package. Helpers were namespaced, assertions and
fixtures were retained, and the temporary package was removed afterward.
The initial copy had a generic-helper name collision and failed compilation;
those failed samples are retained but excluded from performance conclusions.

The corrected experiment showed:

| Arrangement | Command wall samples | Median | Verdicts |
| --- | --- | ---: | --- |
| Three original packages | 10.152, 10.471, 11.890s | 10.471s | 3/3 pass |
| One merged package, original serial top-level tests | 18.309, 21.425, 21.907s | 21.425s | 3/3 pass |
| Three original packages, second paired series | 11.093, 11.458, 17.056s | 11.458s | 3/3 pass |
| One merged package, parallel top-level tests | 12.292, 15.613, 11.267s | 12.292s | 3/3 pass |

The build-only comparison reduced **three linker calls to one**. In the
first corrected comparison, actual linker CPU dropped from **4.844 seconds
to 1.266 seconds**, and build-only wall time from 4.449 to 2.714 seconds.
The second comparison included compilation of the changed merged test source
and had build-only wall times of 3.192 versus 3.305 seconds.

The original top-level tests do not call `t.Parallel`; separate packages
provided their overlap. Naive joining removed that overlap and doubled
latency. Adding top-level parallelism helped, but the complete paired ranges
overlap and the merged median remained slightly slower. **This experiment
proves lower link compute, not a consistent end-to-end speedup.**

Consolidation should therefore pair compatible behavior cohorts with preserved
parallelism and, where appropriate, one reusable immutable process. Retain
scenario-owned profiles/routes/sessions and independent cleanup. Use a
bounded number of binaries that can still keep the runner busy; one giant
serial test package is not the goal.

### 4. Removing non-customer test work

Concrete reclassification candidates:

| Current test/work | Appropriate destination |
| --- | --- |
| `TestFunctionalTestVizLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog`: reads workflow/supervisor source and searches strings | Static CI/lint check |
| `TestFunctionalTestVizLaneScriptSmoke_PreservesFailureExitAndLog`: reads Makefile and Go source to check implementation strings | Runner-owned unit/behavior tests; static checks for routing |
| `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks`: duplicate fake routes, fake route counts, fake close behavior | Unit tests for the support router |
| `TestJourneySchedulerSeparatesReadinessAndSourceWaits`: custom fixture clock behavior | Support-boundary unit test |
| Helper-executable/Go-test-binary cases in provider ACP/mock-worker coverage | Small compiled-artifact integration proofs, where OS behavior is the actual contract |

`observability/verification` costs **16.945–20.528 seconds** as a package, but
only part of that package is static checking. Do not claim that moving the
two identified source checks saves the entire package duration.

The customer promises for persisted identity, cache reuse, recovery,
redaction, CLI/API parity, and replay remain valuable. Retain one focused
public-boundary proof for each distinct promise. Filesystem assertions that
verify a customer-visible saved/exported artifact are not automatically
source-shape tests. Reconcile coverage floors after justified reclassification
rather than keeping internal inventory tests solely for incidental coverage.

## Failures in the local full diagnostic

The full non-short, non-covered local command completed in **634.675 seconds**
with 158 package outcomes and exit status 1. Failed packages were:

- `automations/scheduling`
- `orchestration/javascript/durability`
- `sessions/root_composition`
- `transport/mcp/protocol`
- `workers/inference`
- `work/watch`
- `transport/submit`

The submit package consumed **300.389 seconds** and hit the five-minute package
timeout. `SUB-015 deadline reaches handler and joins` was blocked in
`waitForSignal` at `matrix_test.go:592`, called from line 285: the test was
waiting for its handler-observation signal before joining the deadline-bound
command. That receive has no local ceiling. The intended 250ms cancellation
contract can therefore turn into a whole-package timeout if the handler is
never reached. Investigate the fixture/request path and await either the
observed handler or the command's terminal error. Do not silently accept
deadline expiry before the scenario's required observation.

The same submit package passed in the later Linux sample in 7.530 seconds.
This local failure is not proof that it caused the sampled Linux slowdown.
The failed local full run, concurrent ambient workload, broader inventory,
and tracing prevent it from serving as a clean Linux baseline. Focused cache,
build, and corrected consolidation experiments all passed. No full passing
suite under three minutes was demonstrated.

## Plan to reach a three-minute whole-job budget

Define the target as **start of Functional Coverage job to completed verdict
and diagnostic uploads**, excluding GitHub queue time and unrelated jobs.
Do not relabel only package execution or a warm cached replay as the total.

Proposed budgets are acceptance targets, not measured forecasts:

| Phase | Target |
| --- | ---: |
| Checkout, tool setup, restore, dependency availability | 35s |
| Harness/discovery, build/instrument/link critical path | 45s |
| Customer-scenario execution critical path | 80s |
| Coverage gate, verdict, diagnostic upload | 15s |
| Margin | 5s |
| Whole job | **180s** |

Build and execution overlap in today's runner. Phase budgets must be checked
against the actual dependency graph, not enforced by simply summing overlapping
timers. The table allocates a practical total for the optimized lane.

### Recommended implementation order

1. **Fix isolation and blocking first.** Give every invocation explicit
   profile/cache roots, allocate session identity before runtime opening,
   retain profile bootstrap as a prerequisite outside readiness clocks unless
   first-run behavior is the assertion, and fix unbounded observations.
   Start with the demonstrated Models cache issue and the local submit failure.
2. **Measure the missing hosted phases.** Publish package load/discovery,
   checker/tool build, compile, instrumentation, vet, link, execution,
   retries, coverage processing, and rendering/upload spans. Capture actual
   per-tool package timings and CPU, not just `-x` command counts. Explain the
   81–151 seconds outside the timed coverage invocation before claiming a
   three-minute total.
3. **Reduce the slowest runtime cohorts.** Profile Models, result/results,
   review-failure routing, ACP, Chat Session composition, and runtime metrics
   on the four-CPU runner. Group compatible edges into reusable processes;
   remove unnecessary servers/streams and repeated installation, and observe
   the specific public session/projection each assertion owns.
4. **Reduce link count deliberately.** Trial compatible packaged-factory and
   transport behavior groups. Verify retained scenario counts/results and
   runtime overlap. Set an evidence-based binary-count target after a real
   covered Linux comparison; the local merge trial does not justify a
   guaranteed wall-time savings estimate.
5. **Tune cache and concurrency together.** Trial `-p=2,4,6,12` on the same
   four-CPU runner with fixed source and cache conditions. Consider per-binary
   parallelism as well as package count. Restore coverage-compatible project
   archives with scoped keys/size limits, and prebuild the runner tools once
   where useful. Do not return to the previous approximately 7GB cache without
   measuring restore/save cost.
6. **Reclassify static/support/OS work.** Keep customer behavior in functional
   tests, source-policy checks in lint, support-fake tests near their boundary,
   and small executable proofs in integration. Reconcile the inventory and
   coverage policies to those retained guarantees.

For an 80-second execution budget at 12 package slots, summed package time
must be below about 960 seconds even with ideal scheduling. Today's samples
need approximately **57–65% less aggregate package duration** to reach that
bound, plus better scheduling and a substantially cheaper build/harness path.

Accept completion only after at least five unchanged-head Linux samples of
the complete required non-short covered inventory, including project-cache
miss and representative restored-cache conditions. Record every sample and
failure/retry; require every complete job sample to stay below
180 seconds, no new quarantine, retained customer guarantees, and separate
race validation. Hardware changes, sharding, or moving coverage to another
required job must be stated explicitly rather than counted as reduced work.

## Reproduction and retained artifacts

All raw local evidence is under `.artifacts/latency-audit/` (ignored diagnostic
output), including CI downloads/job metadata, JSON events, Go traces,
per-tool records, profiles, and `measurement-summary.json`. Temporary merge
source was removed; production code and committed tests were not changed.

Representative commands, from the repository root:

```text
go test -p=12 -json -count=1 -timeout=5m ./tests/functional/...

go test -p=12 -json -count=1 -timeout=5m -run=^$ \
  -covermode=count \
  -coverpkg=github.com/portpowered/infinite-you/pkg/... \
  ./tests/functional/...

# Add -a to deliberately rebuild standard and third-party packages too.
# Add -debug-trace=<absolute-path> for Go 1.25 build-action diagnostics.
# The audit's tooltimer.go/measurement scripts retain exact tool arguments.

go test -json -count=1 -timeout=5m \
  -run=^TestModelsNamedBuiltinRouteUsesEffectiveDefinitionWithoutWorker$ \
  ./tests/functional/models/root_composition
```

For the cache comparison, set `INFINITE_YOU_OMNIVOICE_CACHE_DIR` only in each
child process's environment to a fresh owned directory; preserve normal
customer assertions. Python subprocess argument arrays were used to avoid
PowerShell's handling of certain `-flag=value` arguments. For an authoritative
Linux comparison, use the required CI supervisor and quarantine selection,
not the broader raw Windows wildcard above.

## Implementation and validation update

### Changes applied

Managed worker execution now always uses the joined Models operation contract.
The separate OmniVoice command runner, supervised `/invoke` adapter, local handle
cache, `InvokeLocal` Models operation and its request/result contracts, and their
wiring have been removed. Non-LocalAI supervised servers require an explicit
command; there is no implicit `omnivoice-llamacpp` fallback. Current LocalAI
operation, host, asset, output, cancellation, and scope coverage remains.
The old opt-in real OmniVoice functional sweep and legacy invocation and unary
retirement tests have been removed. Current model-reference resolution and
scope resource isolation unit tests were retained in `model_reference_test.go`.

Default functional API hosts now override an inherited model cache with a cache
inside their own temporary profile. Explicit fixture environments remain
authoritative. The model suite's profile environment selects its own
`.agent-factory/models` directory, matching the small assets seeded by its
fixtures. This prevents tests from traversing or hashing an operator's installed
models while retaining cache-selection and offline customer scenarios.

Three packaged invocation suites (`plan_parallel`, `fusion`, and `deep_research`)
now share `factory/packaged/invocation`. Their top-level journeys run in parallel,
preserving overlap that previously came from separate package binaries.
Model list/inspect/pull and invocation CLI tests now share `models/cli`.

The functional package names now describe behavior:

| Previous package | Current package |
| --- | --- |
| `models/root_composition` | `models/local_inference` |
| `factory_runtime/root_composition` | `factory_runtime/execution` |
| `operator_settings/root_composition` | `operator_settings/configuration` |
| `recordings/root_composition` | `recordings/lifecycle` |
| `work/root_composition` | `work/admission` |
| `sessions/root_composition` | `sessions/isolation_and_recovery` |
| `sessions/chat_sessions/root_composition` | `sessions/chat_sessions/acp` |

Active imports, CI selectors, Make targets, and reviewed scenario evidence paths
were updated. Generated Wire composition was regenerated. Constructor effect
count checks, Models contract-value checks, fixture self-tests, and direct
recording-service scope checks were removed from functional coverage.
Make/CI verification checks moved to `tests/tooling/verification`, outside the
customer functional lane. The functional README now states the customer-surface
rule explicitly. This cleanup is not a claim that every historical test in the
repository has been reviewed or that every use of a service observer has been
eliminated.

Two submit timeout cases no longer wait for an HTTP request that may never start
when initialization exhausts the caller's deadline. They still assert deadline
failure, join handlers that started, and prove subsequent submission works.
Shared submission boundary waits are bounded. The configured-server
withheld-header scenario now cancels upon an observed backend-start signal,
replacing a fixed 500ms delay. Its real HTTP interaction makes an explicit event
signal appropriate; virtual time is better reserved for isolated in-memory timer
behavior.

### Measurements after the changes

These are Windows diagnostics, not a new authoritative Linux CI baseline.

| Measurement | Result |
| --- | --- |
| Earlier warm covered build-only command | 49.197s; 145 link actions |
| First covered build after cleanup | 111.473s; pass; 636 compiler subprocesses |
| Warm covered build after cleanup | 39.014s; pass; 142 link actions |
| Final model local-inference suite | 27.038s; pass |
| Final submit suite | 23.229s; pass |
| Final merged model CLI suite | 20.181s; pass |
| Final merged packaged invocation suite | 27.241s; pass |

The warm build command used `-run=^$`, `-count=1`, `-p=12`, `-covermode=count`,
and the same whole-backend `-coverpkg` setting as the earlier local build audit.
The package cleanup removes **three link actions**, about 2.1% of the previous
145-action Windows graph. The warm command was about 20.7% shorter in this sample;
that difference is not a controlled CI improvement estimate.

The warm run recorded 140 actual linker subprocesses. Their summed child wall
time was 283.972s, user CPU 185.500s, and system CPU 122.453s. Two compiler
subprocesses consumed 1.560s summed wall time, and vet consumed 20.534s summed wall
time across 164 subprocesses. These sums overlap because work runs concurrently;
they must not be added to estimate the command's 39.014s wall time. Tool records
were filtered to this command's start timestamp because the timer directory also
contains records from the preceding build.

A broader non-covered functional run on an intermediate cleanup snapshot took
**635.099s and failed 24 of 143 package outcomes**. Seven packages reached the
two-minute package timeout. Other Go builds, lint jobs, and package discovery
were active on the same host. The run also included legacy model-invocation
tests and deadline assumptions removed or fixed in the final cleanup. It is
neither a passing final-suite result nor evidence of a speedup. The previous raw
Windows baseline also failed, so Linux CI remains necessary to distinguish host
contention, existing failures, and regressions.

Highest durations in that diagnostic run were `factory/process_time` (122.322s,
failed), `work/transports/cli/submit/batch_contract` (120.462s, failed),
`provider_sessions/details` (120.358s, failed), `factory/replay_contracts`
(120.182s, failed), `factory/packaged/named_invocation` (120.163s, failed),
`sessions/execution` (120.161s, failed), and `transport/mcp/stdio` (120.111s,
failed). Passing packages worth profiling next include `transport/cli/process`
(114.706s), `events/factory_events` (114.487s),
`factory/visualization/runtime_metrics` (109.432s), and `sessions/lifecycle`
(108.872s). The successful hosted CI rankings earlier in this audit remain the
better basis for prioritizing actual CI latency.

### Verification and remaining target

Models unit suites, current worker runner/interface tests, and canonical wiring
tests pass. Model-reference and resource-scope tests pass after separating them
from the retired executor suite. The affected CLI, local inference, submit,
packaged invocation, factory execution, work admission, settings, and recording
functional packages pass in focused fresh runs. The full covered functional
tree builds successfully. The union of `functionallong`, `backendconformance`,
`factoryartifact`, and `managed_process_integration` tags compiles for wiring,
model functional tests, and renamed session recovery tests. All 156 reviewed
functional scenario records validate. `git diff --check` passes.

The **under-three-minute total CI target remains unverified and unmet by the
available full-run evidence**. No passing hosted CI run of these changes has
been produced. Removing three links is a useful cleanup, not enough on its own
to remove the 165–241 seconds still needed from the earlier authoritative
344.601–421.149s coverage invocations. The next measurements should use the
four-CPU Linux runner, preserve the canonical coverage and quarantine population,
profile the highest customer journeys, and separately time supervisor and
quarantine overhead. Remaining default-session bootstrap, repeated graph
construction, and broad whole-backend instrumentation are the largest targets
identified by the original audit.

Raw update evidence is retained locally under `.artifacts/latency-audit/`,
including `after-summary.json`, build traces and tool records, full-run JSON
events, focused validation logs, and tagged compile results. The raw full run
is explicitly an intermediate snapshot; focused final validation logs supersede
its model and submit results.

## Hosted checkpoint and Linux follow-up

PR [#2867](https://github.com/portpowered/you-agent-factory/pull/2867), based on
main `9ca420195a1d2f3135334b293193fa92df29cf31`, first measured cleanup commit
`8a09e574de32b5ebd3792869e227d4e28cbbddc9` in hosted
[CI run 37283549677](https://github.com/portpowered/you-agent-factory/actions/runs/37283549677).
All **1,022 selected tests passed or intentionally skipped**: 1,020 passed,
two skipped, zero failed. The coverage test invocation took **385.076s**.
The supervised coverage child took **468.477s**, with concurrent quarantine
verification taking **59.263s**. The 83.401s difference between the coverage
child and test invocation remains material overhead. These measurements sit
inside the earlier observed 344.601–421.149s test-invocation range; they do not
establish a controlled improvement or a five-minute checkpoint.

Hosted coverage failed its package floor for `models/transports/http`:
440/798 statements (55.1378%) against 58.27%. Backend Lint also found stale
compiler-owner baseline entries after moves and seven newly unreachable
production functions left behind by the executor removal. Models wiring and
race verification passed. The five-, three-, and two-minute merge checkpoints
remain unachieved; this PR has not been merged.

For additional diagnostics, the same tracked source was archived into an
isolated Linux filesystem snapshot under WSL, running Go 1.25 with four-CPU
affinity, `GOMAXPROCS=4`, and the canonical 12-package budget. The populated
Linux module/build caches were retained; this is not a clean hosted runner.
The first instrumented run took **436.49s overall**, including **368.666s**
inside its coverage test invocation. Two top-level tests failed: a LocalAI
fixture asserted internal CPU-only platform facts on this CUDA-capable host,
and a packaged loop scenario failed but passed its focused rerun. Partial
coverage from a failed run is diagnostic only.

| Linux tool measurement | Invocations | User CPU | System CPU | Sum of child wall time | Union of active wall intervals |
| --- | ---: | ---: | ---: | ---: | ---: |
| Compiler | 1,637 | 240.364s | 37.980s | 918.352s | 200.731s |
| Linker | 147 | 266.927s | 40.231s | 1,084.364s | 314.304s |
| Vet | 1,247 | 49.254s | 13.853s | 249.964s | 88.565s |
| Coverage instrumentation | 658 | 2.604s | 1.808s | 22.850s | 16.201s |

These tool totals include discovery/helper compilation and overlap both one
another and execution. They cannot be summed to estimate elapsed time. The
whole command consumed 1,275.33s user CPU and 332.40s system CPU. The residual
includes customer commands, Go orchestration, profile processing, and reporting;
it must not all be attributed to tests without process-level profiling.

The highest package durations were `runtime_api` 73.083s,
`models/local_inference` 58.977s, `factory/review_failure_routing` 52.148s,
`transport/mcp/stdio` 48.697s, `factory/visualization/runtime_metrics` 43.190s,
`providers/acp` 41.383s, and `sessions/chat_sessions/acp` 40.036s. Linker child
wall times reached 15.376s for `transport/run_scoped_server`, 14.997s for
`transport/submit`, and 14.458s for `workers/transports/http`, while consuming
about 3.3–3.7 CPU seconds each. Much of that wall time is competition for the
four CPUs rather than unique package code size.

A follow-up snapshot, with warmer compilation caches, fresh test execution
(`-count=1`), one further CLI-package merge, cache isolation, and functional
vet disabled, took **253.92s overall / 231.050s test invocation**. It still
failed three top-level tests, so it is not a passing latency result. Its 145
linker invocations consumed 215.223s CPU; 304 compiler invocations consumed
55.729s CPU. Nine vet invocations consumed 0.138s CPU, showing that the
instrumented functional graph no longer repeated repository vet. The warmer
cache and several simultaneous changes prevent attributing the full elapsed
difference to any single optimization.

### Further changes being validated

- Removed the unused legacy runtime HTTP edge, six legacy catalog-host
  functions and their compatibility unit tests, and the retired worker
  working-directory helper. Renamed dead-code baseline paths without adding
  findings; removed vanished compiler-owner allowances.
- Combined CLI command and parameter journeys under `transport/cli/invocation`.
  Independent top-level tests overlap, and parameter runs use unique explicit
  sessions. Negative parameter cases assert their own public errors instead
  of counting another concurrent invocation's provider calls. The merged
  package passed Windows (5.952s), Linux (3.868s), and three fresh Windows
  repetitions (25.297s total).
- Moved Factory execution under `factory/execution` and visualization journeys
  under `factory/visualization/presentation`, matching the durable customer
  ownership required by the functional lane.
- Extended owned home/model-cache environments to shared review routing and
  common packaged-Factory/daemon helpers. Seeded assets retain the normal
  fixture-owned `.agent-factory/models` layout.
- Removed the functional PID-parser self-test and two packaged-loop fixture
  self-tests. Removed legacy cache-migration and provider-backed OmniVoice
  smoke cases; current LocalAI CLI, REST, recorded-event, and audio/file
  journeys remain. Added one shared-server table for actionable errors on
  malformed current model REST requests; its nine customer cases pass in
  1.592s locally.
- Removed internal model configuration assertions from the Factory inference
  scenario while retaining public results, errors, and canonical recordings.
  It passes on the CUDA-capable Linux host.
- Made recording cleanup use the selected process fact clock when an owned
  session supplies no clock. Linux session deletion had exposed a nil-clock
  panic; the existing fact-clock lifecycle test now checks this missing-clock
  case. The unit test passes.
- Corrected the unified event-log smoke's assumption that a continuous server
  must publish a completed live Factory state before closing. It retains the
  completed terminal run result, completed Work, canonical ordering, and
  identical live/recorded event payloads.
- Removed a fake-timer channel-release assertion after session close and
  post-close TCP probes against LocalAI fixture ports. A stopped timer need
  not fire, and another test may own a reused port. The public no-late-delivery,
  peer-isolation, model-result, and joined-close checks remain. Fresh Windows
  runs of process-time and local-inference packages pass in 35.151s and
  20.392s respectively; these are focused checks, not a full-lane result.

Required Backend Lint continues to own repository-wide vet. Bounded event and
cleanup waits have inline reasons; they are not replaced with shorter sleeps.
Full hosted verification of this follow-up, restored Models HTTP coverage, and
a passing supervised latency measurement are still required before merging.
Raw local evidence is under `.artifacts/latency-audit/linux-8a09/`,
`linux-next/`, `ci-functional/`, and the focused validation logs.

### Fresh execution after the follow-up fixes

The next four-CPU Linux run passed all **1,018 selected top-level tests**:
1,016 passed, two skipped, zero failed, across 145 packages. Its covered test
invocation took **202.455s (3m22.455s)**; the complete `gocoveragecheck` command
took **220.85s (3m40.85s)**. This diagnostic command excludes the outer
`functionaltestviz` supervisor and its separate quarantine validation. The
populated compilation caches were reused, while `-count=1` forced execution.
It is neither a cold CI measurement nor a verified hosted checkpoint.

The command consumed 655.76s user CPU and 139.92s system CPU. Instrumentation
recorded 144 linker invocations consuming **204.122s CPU**, 40 compiler
invocations consuming **15.308s CPU**, and nine vet invocations consuming
0.227s CPU. Linker active wall intervals covered 185.106s; summed child wall
time was 704.149s. Those overlapping intervals must not be added to elapsed
test time. The largest linker wall times were HTTP status (9.180s), CLI
session resume (8.898s), and docs (8.711s), each using about 1.5–1.6s CPU.
Reducing repeated application links remains material even with warm caches.

This run still failed the Models HTTP coverage floor: 405/798 statements,
50.7519%, against 58.27%. Current cloud-model REST discovery, invocation and
unsupported local-pull behavior now share one owned server. The malformed
request table also covers invalid text, media location, metadata and generic
JSON envelopes through actual REST calls. Together these focused tests pass
in 1.170s locally. A diagnostic union with the Linux profile reaches
465/798 statements (58.2707%); only a fresh complete covered run can confirm
the package floor. No coverage threshold was lowered.

The customer inference package is now `tests/functional/models/inference`,
because it covers both local and configured cloud models. Earlier measurements
retain the historical `models/local_inference` path. Workflow and scenario
manifest references follow the new path; all 156 reviewed scenario records
validate. Evidence for this run is under `.artifacts/latency-audit/linux-current/`.

The Linux snapshot contained the initial measured runtime plus subsequent
patches. It did not incorporate the intervening main-branch dead-code tooling
change; its snapshot commit identity is not the PR head identity. Hosted
verification of the exact rebased PR head remains the merge authority.

### Complete supervisor measurement and subsequent cleanup

An exact tracked-source archive of `ac9dc75bef` was measured with the same
four-CPU affinity, warm build/module caches, fresh execution and package budget.
The complete CI supervisor ran coverage/reporting concurrently with the real
quarantine validator. It took **255.84s (4m15.84s)** overall. Quarantine passed
in **22.294s**; the coverage child took **255.828s**, including **234.580s**
for its test invocation. It observed 146 packages and 1,021 top-level tests:
1,018 passed, two skipped and one failed. The Models HTTP floor was not evaluated
because execution failed. This is not a passing checkpoint. Setup-only attempts
that lacked required validator metadata or `jq` were terminated and excluded
from these results.

The failure was the specialized webhook-time journey's submission returning
HTTP 500. Three focused Linux repetitions of the complete two-cohort journey
passed in 15.283s total. The fixture observed secret resolution before submitting
Work; that effect precedes completed runtime startup. It now additionally awaits
the exact session's public `RUNNING` status and includes command/session identity
in failures. Verification of the updated fixture under full covered load is
still required. Evidence is under `.artifacts/latency-audit/linux-ac9dc/`.

Five further packaged-Factory suites—Quorum, Subagent, Ralph, Plan Execute and
Factory Builder—now share `factory/packaged/invocation`, saving five additional
test binaries. Each independently owned top-level cohort runs in parallel;
stateful sequences within a cohort retain their ordering. Their owned-home
environments also discard inherited model-cache overrides. The combined suite
passes locally in 14.491s and compiles with `functionallong`; a full-lane latency
result is needed to establish the saving.

Hosted lint on `b80e2127e6` identified remaining internal adapter construction,
operator-path policy calls, stale timing/construction baseline entries and the
missing maintenance-lane assignment for moved tooling tests. The recordings
journey now retains actual CLI recording files and REST artifacts without its
internal adapter/decoder checks. Operator settings retains CLI output, dispatched
model arguments and the persisted customer config file without constructor or
filesystem call-count assertions. Tooling tests have an explicit maintenance
lane. The dead-code baseline records the new test-support-only home-environment
helper, consistent with its existing helpers excluded from production reachability;
no new unreachable production code is accepted.

Model test names now describe customer outcomes: built-in CLI validation,
REST audio output, readiness errors, CLI output modes and unsupported pinned
backends. The TTS named/generic REST journey runs as its own test rather than
inside the embedding suite. The complete model package passes in 12.548s
locally; its scenario evidence references were updated and all 156 reviewed
records validate. Fixture source identities use `customer-inference` rather
than the retired `root-composition` label.

After the five-package merge, the complete Linux supervisor at `d087be567f`
took **237.00s (3m57s)**, including a **222.064s** test invocation across
141 packages. Quarantine passed in 21.766s. Of 1,021 top-level tests, 1,018
passed, two skipped and one failed. The webhook journey passed. The remaining
failure was admitted-Work cancellation/recovery: the fixture received the exact
dispatch-response event, then immediately read the Work projection while it
still held its initial state. The event is not a public read barrier. It now
awaits the exact Work's terminal state and output through REST before checking
correlation/isolation. Three fresh local repetitions pass in 11.214s total.
The package declaration is also corrected from `root_composition_test` to
`concurrency_test`. Full coverage gates were not evaluated on this failed run;
evidence is under `.artifacts/latency-audit/linux-d087/`.

A sequential four-CPU linker experiment compiled the fully covered HTTP status
test binary with ordinary debug data and with `-ldflags=-w`. The first ordinary
build warmed additional compiler entries and is excluded from comparison.
The subsequent ordinary build took 1.815s wall / 2.425s child CPU and produced
84,485,923 bytes. Two builds without DWARF took 1.435s/1.388s wall and
1.623s/1.553s child CPU, producing 68,593,982 bytes. These are complete warm
compile/link command measurements for one package, not a suite-wide forecast.
Functional coverage now omits debugger-only DWARF while retaining Go runtime
stack symbols/source coordinates and coverage counters. Other coverage lanes
retain their existing flags. A fresh full-lane measurement is required to
establish its actual elapsed saving.
