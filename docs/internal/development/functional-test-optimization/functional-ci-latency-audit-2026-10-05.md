# Functional CI latency audit

Measured October 5, 2026. Initial local source revision:
`73d83286167b941e3f2f668bcc7425926481fd99`.

## Current result

The five-minute checkpoint merged in
[PR #2867](https://github.com/portpowered/you-agent-factory/pull/2867), commit
`0dcdbf1fde04e573730e19c4b39fe097e7389d8c`. Its merge-queue functional supervisor
passed in **272.327s**, including **215.102s coverage invocation**. An identical
source tree also took 340.850s; both samples and recovered flakes are retained
below. The three- and two-minute hosted merge checkpoints remain outstanding.

The first hosted next-phase run, at `0878fd26d0`, passes in **316.816s** full
supervisor / **287.224s** coverage invocation. Its trace reports 1,355 compiler
commands and 73 linker commands, and one MCP ACP permission case passes on a
same-head retry. The existing 67 MB dependency cache leaves repository packages
cold; the warm local result below does not predict this cold hosted run. The
next change tests a bounded compiler-archive cache instead of the previously
removed whole build-cache restore, which cost roughly four minutes for 7 GB.

The next candidate's full four-CPU Linux supervisor passes in **111.55s**, with
**103.120s coverage invocation**, all **758 selected tests** accounted for,
756 passes, two skips and no retries. Coverage floors, the selected inventory,
quarantine verification and compile-probe diagnostics remain enabled. Compatible
packages use a build-only overlay: 19 packages share an executable and 36
fixture-sensitive packages keep native binaries. Original source coordinates,
test identities and evidence declarations are preserved. The coverage trace
records **37 linker commands**. This is a warm-cache local result, not a hosted
checkpoint or a cold-build guarantee.

Consolidation has a measured benefit. A controlled build-only comparison reduced
**136 linker invocations to one**, **160.292s linker CPU to 1.807s**, and warm
build wall time from **44.29s to 3.95s**. CPU time is summed across processes;
it is not elapsed time that can be subtracted directly from a CI duration.
The latest private single-binary execution repeat **passes** after removal of
a process-wide working-directory mutation. It combines 63 package groups and
takes **133.03s command wall / 116.653s package execution**, with **one linker
at 1.609s wall / 1.585s CPU**. The repository's coverage evaluator accepts the
recorded profile against unchanged total and package gates: **61.4% total**.
This is an experimental package execution, not the full CI supervisor. Peak RSS
is **6.28 GiB**; fixture memory and hosted behavior still need validation before
shipping the all-in-one layout.

The current source layout converges another **21 packages** into three existing
customer-use-case packages: Factory execution, packaged Factory invocation,
and Work admission. This preserves ordinary and `functionallong` cases and
their session-owned fixtures. Independent parent tests overlap through Go's
test scheduler. Sixteen composition-oriented test names now describe customer
CLI, REST, Work, session control, recording and replay behavior. Coverage floors are unchanged. The rebased manifest preserves all 160 reviewed
scenario decisions, including the new captured-activity surface from main. Six further CLI packages
share one customer-command package. Four workflow fixture suites share one
package, and 28 compatible suites now share Product customer journeys, removing
27 more binaries while retaining their explicit sessions and public observers.
Fifteen further fixture-owning suites now share Product customer lifecycles,
removing another 14 binaries. Their former package setup and shutdown belong to
parallel parent tests; cleanup runs after their parallel children. Repetition
resets fixture state, and resume fixtures resolve repository paths explicitly.

Detailed measurements below include failed runs and their causes. Remaining
work is to consolidate fixture-heavy packages with their own lifecycles, reduce
scenario startup and execution costs, and verify the complete hosted lane at
each requested checkpoint.

## Initial finding

### Latest fixture consolidation measurement

The additional customer-lifecycle package combines 15 fixture-owning suites:
audio artifacts, packaged catalog/classification/CLI–REST parity, review failure
recovery, operator settings, Factory dispatch/eligibility/session lifecycle,
CLI discovery/resume/worker lifecycle/modes, and Work routing. MCP protocol
remains separate after the 16-suite candidate exposed startup contention.
Only their test packaging changes. Each fixture remains owned by one parallel
parent and its customer scenarios; independent parents have independent routes,
sessions, homes and cleanup. Package state resets between repeated runs.

The first full four-CPU Linux coverage run of the 16-suite candidate took **244.70s overall
/ 194.208s tests**, and failed. It observed **68 links**, consuming **95.742
CPU-seconds** with **142.827s of active wall time**, and **1,543 compilations**
consuming **234.302 CPU-seconds**. The previous synchronized-layout sample had
80 links / 110.065 linker CPU-seconds, but only 121 compilations. The new sample
used a fresh source directory and rebuilt substantially more dependencies;
these are different cache states and are not a controlled elapsed-time comparison.
Active compiler and linker intervals overlap each other and running tests.

The run reported 822 top-level passes, two skips and three failures across
two packages. Failures were an incorrect fixed-UUID expectation added during
identity-test cleanup, MCP protocol startup exceeding its existing five-second
guard, and the unchanged injected invocation cancellation case returning an
empty request/session identity. The UUID generator deliberately returns fresh
UUIDs: the corrected customer assertion checks the persisted local UUID and
retains the separate existing-identity reuse proof. MCP now uses the shared
isolated-home helper to exclude operator model caches. The cancellation failure
remains recorded; this failed sample establishes no latency checkpoint.

Pure fixture-ledger checks for packaged cross-surface characterization and the
guard fake's own marker-selection self-test are removed. Operator-settings
construction counters are also removed; CLI output, settings-file persistence,
UUID validity/reuse, session/Work/Event correlation and customer failure
contracts remain. Large migrated scenarios now have named assertion stages
and case helpers, split into focused files. The migrated package's built-in
and repository lint, including the integration/long/conformance tag union,
report zero issues on the migrated package. Three complete native repetitions
of the final customer-lifecycle scenarios pass in 123.771s. Manifest projection
still validates all 159 reviewed decisions. Catalog fixtures now author their
known customer paths directly instead of calling the production path-policy
function to arrange the assertion.

The warm-cache repeat took **174.52s overall / 164.897s tests**, but also
failed: MCP's initial protocol handshake exceeded five seconds, an unchanged
Worker Session missing-transcript case returned recording-unavailable, and an
unchanged mock-worker case failed temporary-directory cleanup. The initial
UUID and cancellation-attribution failures did not recur. This is another
failed sample, not achievement of the three-minute checkpoint.

| Warm-cache phase | Invocations | Aggregate CPU seconds | Active wall seconds |
| --- | ---: | ---: | ---: |
| Compile | 2 | 4.200 | 11.093 |
| Link | 65 | 93.965 | 133.330 |
| Vet | 27 | 0.260 | 1.054 |
| Entire supervisor | — | 627.560 | 174.520 |

Linking remains costly after dependencies are cached, but its active interval
overlaps execution. It accounts for approximately 15% of this sample's total
CPU; eliminating repeated linking alone does not eliminate the remaining
customer-scenario and harness compute. The two-minute target also requires
reducing repeated application construction and execution costs.

| Largest package execution in this warm sample | Seconds | Outcome |
| --- | ---: | --- |
| Product customer journeys | 126.966 | Pass |
| Product customer lifecycles | 94.786 | Fail |
| Packaged Factory invocation | 54.533 | Pass |
| Factory execution | 52.327 | Pass |
| Models inference | 47.026 | Pass |
| Work admission | 39.627 | Pass |
| MCP stdio | 37.629 | Pass |

An MCP experiment completed public first-run home initialization before
starting the protocol response guard. Three focused native repetitions passed
in 14.477s, but full coverage still failed two startup handshakes and the
unchanged mock-worker cleanup. That run took **167.46s overall / 158.046s tests**
with 65 links / 85.919 linker CPU-seconds and two compilations / 4.096 CPU-seconds.
It reported 823 passes, two skips and two top-level failures; it is not a passing
checkpoint. The extra initialization is not shipped because it did not solve
the failure. MCP protocol remains in its separate transport package with
explicit model-cache isolation and its five-second guard unchanged. The other
15 suites remain consolidated; the final layout needs hosted verification.

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

The Provider Sessions root constructor/effect-count tests were subsequently
removed from the functional lane. They never called CLI/MCP/REST or checked
customer files; current transcript discovery, inspection, association and replay
journeys remain in their behavior subsections. Status HTTP journeys now share
the existing HTTP server package, with independent scenarios running in parallel.
The combined server suite passes in 4.274s locally. These changes remove two
more test binaries; their saving is not included in the earlier measurements.

### Passing full supervised run with reduced debug data

The complete four-CPU Linux supervisor for `630fb269c5` **passed**, including
tests, quarantine and existing coverage gates. It took **191.29s (3m11.29s)**
overall; the test invocation took **176.714s (2m56.714s)**. It observed all
141 expected packages and 1,022 top-level tests: 1,020 passed, two skipped,
zero failed. Aggregate coverage was 61.2% against the unchanged 33.1% minimum;
existing staged coverage holds remained in force. The previously deficient
Models HTTP floor passed. Quarantine passed in 19.845s.

The full command consumed 578.62s user CPU and 119.10s system CPU. Recorded
tools included 142 linker processes consuming **171.525s CPU**, 143 compiler
processes consuming **8.931s CPU**, and 27 vet processes consuming 0.242s CPU.
Linker active wall intervals covered 160.770s; summed child wall time was
607.187s. Different warm cache states and concurrent scenario fixes prevent
attributing the entire 45.71s elapsed reduction from the prior failed run to
DWARF removal alone. The passing measurement includes reporting and quarantine,
unlike the earlier 220.85s direct coverage command.

This establishes a passing local result below five minutes; the complete lane
is still **11.29s above three minutes**. Hosted CI of the pushed head remains
required before the first merge checkpoint. The two subsequent binary removals
are not included here. Evidence is under `.artifacts/latency-audit/linux-link-opt/`.

### Follow-up isolation and unsuccessful repeat

The next full four-CPU run, including the two binary removals, took **203.55s**
overall and 189.668s for the test invocation. It failed: 1,014 passed, two skipped
and three failed across 140 packages and 1,019 top-level tests. Coverage gates
were not evaluated. CPU consumption was 618.79s user and 131.39s system.
This result is not a passing latency checkpoint. Evidence is retained under
`.artifacts/latency-audit/linux-next-checkpoint/`.

The failures remain customer-visible promises: restarting an automation session
returned HTTP 504; a deleted packaged-loop session remained readable; and
CC-12 observed session event sequence 6 before 5. These assertions must remain.
Increasing polling timeouts or deleting these journeys would hide unresolved
lifecycle or ordering behavior and would not establish a reliable fast lane.

The preceding passing run identified two avoidable model stalls: the built-in
readiness/unknown-model case took 30.580s and custom CLI model discovery took
18.150s. Their asset HTTP clients were uncontrolled; the reusable CLI process
also used default model-cache resolution. Both now reject unavailable fixture
asset downloads immediately. The CLI process resolves its cache into its own
temporary home and the custom-model invocation uses the shared isolated-home
environment helper. Three Linux repetitions of the focused cases pass in
**1.154s total** for readiness and **0.226s total** for CLI discovery. These
focused, uncovered measurements identify unnecessary network/cache exposure;
they are not a full coverage-lane forecast.

Hosted lint at `630fb269c5` found two new model-test complexity violations.
The built-in route test now retains public audio and readiness responses while
removing internal launch/protocol/network call-count assertions. Cloud REST
discovery uses the existing model lookup helper, and its invocation response
assertions are separated from fixture setup. The retired
`ValidateLocalInvocationRequest` service-root lint allowance is also deleted;
its stale entry failed packaged-factory verification. Hosted validation remains
required after these corrections.

### Hosted result and single-binary consolidation experiment

Hosted run `37292866856` at `630fb269c5` passed functional coverage: 1,020
tests passed, two skipped and none failed. The test invocation took **388.339s
(6m28s)**, and its complete supervisor took **474.070s (7m54s)**. Diagnostics
observed 846 compiler commands and 138 linker commands. The dependency-cache
restore succeeded, but instrumented compilation still occurred. This hosted
result does not meet the five-minute checkpoint. The same workflow failed lint
and packaged-factory verification for the specific issues corrected above.

Following the request to evaluate monolithic test packages, a private build-only
experiment combines 709 Go source files from 133 customer-test package groups
into one package. Typed identifier references, including embedded struct fields,
are namespaced to prevent collisions, and embedded test assets are preserved.
The binary lists all **1,008 customer test functions**. Its original fixture
`TestMain` routines are dormant in the build-only variant; no successful customer
execution or coverage-gate result is claimed from that variant.

The controlled build comparison uses four CPUs, `GOMAXPROCS=4`, jobs 12,
`-vet=off`, `-ldflags=-w`, `-covermode=count`, and the exact canonical backend
coverage package list. The existing-layout command uses `-exec=/bin/true` so
that test bodies and fixture setup do not contaminate build/link measurements.

| Build-only configuration | Command wall | Compiler CPU | Linker CPU | Links |
| --- | ---: | ---: | ---: | ---: |
| Existing layout, warm compiler cache | 44.29s | 0s | 160.292s | 136 |
| Single customer binary, first compilation | 16.67s | 31.675s | 1.745s | 1 |
| Single customer binary, warm compiler cache | 3.95s | 0s | 1.807s | 1 |

The existing-layout measurement includes three fixture-support test binaries
and 12 fixture self-tests. These are already classified as maintenance by
`ForImportPath`, but functional discovery excluded only the support root and
accidentally included its descendants and the REST-client fixture package.
Discovery now consistently excludes `tests/functional/internal/`. The fixture
tests remain available in their maintenance packages. Customer coverage floors
still require a full-lane check after this classification correction.

These measurements strongly favor reducing the number of functional binaries.
They also show why summed linker CPU cannot simply be subtracted from total
elapsed time: the existing layout's 160.292s linker CPU occupies only 42.334s
of active elapsed intervals when test execution is absent. The first consolidated
compile costs about 31.7s CPU for the large test source package. The next step is
an execution experiment retaining every fixture's setup/cleanup and the original
scenario concurrency; a package move alone can accidentally serialize tests.
Raw evidence is under `.artifacts/latency-audit/linux-baseline-build/`,
`linux-monolith-count/`, `linux-monolith-count-warm/`, and `hosted-630/`.

The intervening local full run after model isolation also passed all coverage
gates, with 1,018 tests passed and two skipped, taking **258.10s overall** and
237.974s for tests. It recorded 139 links consuming 234.122s CPU and only 3.828s
compiler CPU. The spread from the preceding passing 191.29s run prevents treating
focused model-case savings as a guaranteed full-lane reduction.

The first private execution variant kept every original fixture's setup and
cleanup, but registered all tests directly in the single package. It failed:
931 passed, 74 failed and three skipped across all 1,008 customer functions.
Its command took 364.32s and its test execution took 348.313s. It is explicitly
excluded from passing latency evidence. Previously separate serial tests ran
in one long serial phase; relative workflow-fixture paths, evidence registry
references and self-executed test helper selectors also needed migration.

A second private variant organizes the original suites as parallel cohorts,
retains their declaration order, and preserves subprocess entrypoint selectors.
It gives the shared binary 48 parallel test slots, corresponding to the previous
12 packages times four slots per package on the four-CPU runner. Repository
fixture lookups and evidence declarations are adapted to the experimental
source paths, without suppressing their checks. After correcting fixture paths,
the complete command took **187.51s**, including **173.374s test execution**.
It consumed 538.82s user CPU and 103.62s system CPU. One linker consumed
**1.394s CPU** and 1.413s wall; compilation consumed 27.999s CPU. It failed
five suites, so this is not a passing latency or coverage checkpoint. Failures
include fixture-only listener probes, response-event visibility, session startup
and controlled ACP peer/version startup. Evidence is retained under
`.artifacts/latency-audit/linux-cohort-path-execution/`.

The platform ListenerStopObserver's synthetic socket/probe test and the HTTP
advanced-save-version matrix's fixture-probe classification subtests are now
removed from functional coverage. They test fixture/platform mechanics and add
OS port-reuse and scheduling sensitivity without another customer journey.
Platform component tests retain listener observation/cancellation/deadline
coverage. HTTP server tests pass in 3.644s, platform HTTP tests in 1.749s, and
the advanced-save-version REST matrix in 1.340s locally.

The aggregate experiment also exposed a failure-propagation bug in the selected
clock fixture: an activation callback using `t.Fatal` closed its completion
channel while returning no value, then the caller dereferenced a nil session.
The driver now distinguishes a returned result from an aborted callback and
fails the owning test immediately. Three focused repetitions pass in 14.361s.
No startup timeout is increased and the public HTTP failure remains visible.

A follow-up private execution variant starts only the owning fixture when a
subprocess helper selector is supplied, avoiding initialization of all 54
unrelated fixture lifecycles in each fake protocol peer. Execution validation
is pending. These experiments are private; the committed suite has not been
replaced with generated `TestSuite` names.

Hosted run `37296571427` at `f5b5dfcfc9` passed every required check, including
lint, unit coverage, Models wire/race, integration, conformance and functional
coverage. Functional tests took **376.528s** and the full supervisor took
**457.177s (7m37s)**; all 1,020 tests were observed, with 1,018 passed and two
skipped. The job including setup/reporting took 8m19s. No merge checkpoint is
claimed from this successful but above-target hosted run.

## Further consolidation and contention findings

The private child-fixture monolith variant took **222.21s** command wall time,
with **527.94s user CPU**, **125.56s system CPU**, and approximately **5.98 GiB**
peak resident memory. It failed five customer cohorts. The failures included
deadlines expiring before HTTP/CLI attachment, recovery startup, runtime
configuration execution and ACP startup. This result demonstrates that removing
linkers alone does not establish the customer execution target.

The committed layout migration moves 76 source files without combining them
into oversized files or introducing generated numeric suite names. Related
packages share a binary while each scenario retains owned inputs and effects.
Typed identifier rewriting changes only colliding helper names. Build-tagged
cases remain available, reviewed scenario evidence points at the new files,
and exact lint debt entries move with their original code rather than expanding
the allowed debt.

Two deadline fixtures now expire their caller context after observing an
outstanding request or attached Work watch stream. This proves handling of
DeadlineExceeded without racing startup against 100ms/250ms wall clocks.
Standard-library expired-deadline cases remain available separately. Three
focused repetitions pass for both the generated HTTP client and Work watch.

Internal effect-count assertions are removed from session-control and Work
submission journeys. The tests retain public pause/resume results, submitted
Work identities, REST listings and completed dispatch events. The dead TTS
`/invoke` protocol fixture and the named-Factory no-signature compatibility
matrix are removed; current signatures, positional/file/stdin inputs and
LocalAI audio/recording/replay scenarios retain behavioral coverage.

| Four-CPU consolidation measurement | First source-layout run | Follow-up run |
| --- | ---: | ---: |
| Supervisor wall | 277.48s | 259.21s |
| Link commands | 115 | 114 |
| Link CPU | 173.957s | 212.998s |
| Link active wall interval union | 192.849s | 222.519s |
| Compile commands | 1,586 | 6 |
| Compile CPU | 208.498s | 7.512s |
| User CPU, whole supervisor | 846.58s | 749.85s |
| System CPU, whole supervisor | 171.30s | 180.28s |
| Outcome | Coverage passed; quarantine invocation failed | One test failed; quarantine passed |

The first run supplied incorrectly named quarantine environment variables,
so its supervisor failure is an audit invocation error and its elapsed time
is coverage-only evidence. The corrected follow-up observed 115 packages and
1,006 top-level tests: 1,003 passed, two skipped and one failed. Its test command
took **233.532s**. The failure was an OS port-rebind probe in
`TestRunScopedServerUsesExactListenAddress`: another concurrent owner can bind
a released port before the probe. The test now retains the public CLI address
and completion checks through a controlled server edge and an owned completion
signal. Real socket policy remains platform/integration behavior. The legacy
ascending fallback compatibility scenario is removed. The complete run-scoped
server package passes three local repetitions in 19.573s.

Focused built-in lint, repository test-shape/boundary/sleep/debt analyzers, and
the 156-scenario manifest check pass. These checks do not replace required
hosted verification or establish any elapsed-time checkpoint.

### Latest complete consolidation run and subsequent cleanup

The next full four-CPU supervisor took **280.40s**, with **251.160s** for the
coverage test invocation. Quarantine validation passed. All 115 packages were
observed: 1,005 top-level tests, 1,001 passed, two skipped and two failed.
The run consumed 791.81s user CPU and 196.46s system CPU. Its 114 linker
commands consumed **214.241s CPU** with **224.601s** active wall interval union;
four compile commands consumed **3.503s CPU**. Failed tests prevent this run
from establishing a coverage or elapsed-time checkpoint.

The two failures were session Response Event history returning zero events
before a 25ms quiet timer, and the Loop fixture-cleanup matrix timing out while
checking its own lifecycle bookkeeping. The first now observes retained public
MESSAGE completion through the existing SSE fixture before asserting disjoint
session, event and dispatch identities. The customer isolation guarantee is
retained. The cleanup matrix, its action counters, release counters, timer-stop
counter wrapper and private stream probes are removed: its assertions concerned
fixture mechanics and duplicated the retained public Loop duration validation,
scheduled Work admission and overlap suppression cases. Ordinary scenario
cleanup still closes sessions, routes and processes.

The retained FSCP-03 live isolation and packaged Loop cases passed three focused
repetitions (8.722s and 9.685s respectively). Focused built-in lint, repository
boundary/shape/sleep/debt analyzers and all 156 reviewed scenarios also pass.
Hosted CI is required on the current pushed source before a merge checkpoint
can be claimed. The audit keeps all complete failed measurements alongside the
passing coverage-only and earlier complete passing measurements.


### CLI fixture packages consolidated into customer command journeys

The six packages for documentation, shell completion, submit, CLI output,
process behavior and invocation help now live under
`tests/functional/transport/cli/customer_commands`. Six named customer journey
parents run independently in parallel. Each former TestMain setup belongs to
its journey parent; parent cleanup runs after all parallel child scenarios
finish. The layout uses ordinary Go test scheduling, without a global lifecycle
shim. Existing scenario names remain visible as named subtests.

| Covered build-only measurement, four CPUs | Six original packages | One customer-command package |
| --- | ---: | ---: |
| Link commands | 6 | 1 |
| Link user + system CPU | 7.803s | 2.403s |
| Summed linker wall | 12.988s | 3.551s |
| Active linker wall interval union | 2.507s | 3.551s |
| Whole build command wall | 4.82s | 7.64s |

These warm dependency-cache runs use `-exec=/bin/true`, full backend coverage
instrumentation and the same four-CPU affinity. They measure binary construction,
not scenario execution. Link CPU falls 69%, but this small batch's elapsed build
sample increases; filesystem and scheduling costs remain material. Summed CPU
savings must not be presented as CI elapsed savings. Raw tool records are in
`linux-cli-fixtures-baseline` and `linux-cli-fixtures-warm` under the audit artifacts.

The initial covered Linux execution of the merged package passed in 5.764s;
its whole command took 20.37s including compilation and linking. The full merged
Windows package passes three consecutive repetitions in 76.185s. Earlier
repetition attempts exposed package-lazy Sync.Once and process references
surviving cleanup: the owned output, help and process fixtures now reset those
references before each journey setup. Those failed attempts are retained as
validation findings rather than counted as passing measurements. The ordinary
and functionallong source variants compile, focused lint and repository custom
analyzers pass, and all 156 reviewed scenario records validate. The complete four-CPU coverage supervisor subsequently passed in **316.66s**,
including **297.001s** for the test invocation: 111 packages, 966 top-level tests,
964 passed, two skipped and no failures. Moving former top-level CLI scenarios
under six journey parents lowers the top-level count without deleting those
scenarios. All coverage gates and quarantine validation passed. Compilation
consumed 183.423s CPU across 781 commands; linking consumed 225.519s CPU across
109 commands, with 248.486s active linker wall. Whole supervisor CPU was 937.12s
user and 230.50s system. This dependency-cache-populating run exceeds five minutes;
a warm rerun is recorded separately. Raw evidence is in linux-cli-cohorts-verified.

The preceding hosted commit 032f9aa889 completed functional coverage successfully
in **448.588s supervisor wall** and **366.218s test invocation wall**, with 115
packages, 1,002 top-level passes, two skips and no failures. Backend Lint failed:
nine existing timer/sleep allowances moved with consolidated source and were
rejected by the deletion-only baseline growth rule; a removed listener-probe
allowance was stale; three new test-only controlled-deadline helper entries were
absent from the deadcode inventory. The stale allowance and helper inventory are
corrected. The relocation gate still needs resolution before required checks
can pass. No elapsed-time or merge checkpoint is established.

A fresh-checkout measurement before the passing local run stopped before coverage
because the audit repository had no Git HEAD, which the diagnostic collector
requires. Its 102.34s invocation is retained under linux-cli-cohorts-full and is
excluded from test-runtime comparisons.


### Warm CLI consolidation follow-up and relocation lint correction

The warm full supervisor took **227.51s**, including **212.589s** for tests,
with 111 packages, 963 top-level passes, two skips and one failure. Its 109 links
consumed **183.089s CPU** and **199.934s active wall**; there were no compiler
commands. Whole supervisor CPU was 683.76s user and 158.51s system. The failure
was `TestAutomationsSelectedTimeControlsWorkAndJoinedShutdown`: public watcher
Factory Session activation returned HTTP 504 while the controlled scheduler
advanced readiness polls. Coverage floors were consequently not evaluated.
The earlier dependency-cache-populating complete run passed; this failed warm
run cannot establish a checkpoint. Raw evidence is in linux-cli-cohorts-warm.

The lint relocation correction now uses Git-detected test-file renames to map
existing testsleep allowances to the destination file and its declared package.
The function identity, finding kind and occurrence number stay exact. Other
rules remain deletion-only. Unit tests verify that added timer occurrences,
changed function identities, copies without rename evidence and unreadable
renamed sources cannot pass through this mapping. One recovery helper retains
its original name; no current symbol collision requires renaming it. Focused
baseline-growth tests, custom analyzers, built-in lint and the affected Work
recovery scenarios pass. Hosted required checks remain necessary.


### Broader customer journey consolidation

Four workflow fixture suites first converged into customer-workflows journeys,
with parent-owned setup and cleanup. Three partial-start fixture-analysis tests
and their failure-injection helpers were removed. The retained workflow suite
passed three repetitions in 29.099s. Automation readiness now acknowledges each
registered readiness timer without advancing the entire scheduler; three focused
repetitions passed in 16.905s. This prevents filesystem initialization under CPU
contention from expiring the session-open deadline through artificial time advance.

A larger consolidation moves 28 suites into
`tests/functional/product/customer_journeys`, removing 27 more binaries. It keeps
original customer test names except 13 names clarified to describe REST, CLI,
ACP and workflow behavior. Typed helper renames avoid package symbol collisions;
scenario-owned sessions, profiles and cleanup stay independent. Two suites remain
separate because they share existing timer-debt identities requiring further
cleanup. The package remains under the approved Product functional domain and
contains public calls, events, Work results and file outputs.

The first measured larger layout ran all 939 top-level passes with two skips:
87 packages, 941 top-level tests, **234.695s test wall**, **286.00s supervisor**.
It consumed 843.37s user CPU and 174.57s system CPU. Its 82 links consumed
**135.550s CPU** with **161.728s active linker wall**; 1,631 compile commands
consumed **233.542s CPU**. The supervisor failed the Live Change package floor
(198/349 statements, 56.7335%, against 57.77%) after the direct internal
coordinator test was removed. Other unmeasured-package notes concern contract-only
packages and are diagnostics, not the blocking regression. Raw evidence is in
linux-meta-full. This failed supervisor is not a checkpoint.

The next warm measurement uses the functional lane budget for in-package
parallel tests as well as package builds. It took **216.264s test wall** and
**229.10s supervisor wall**: 87 packages, 938 top-level passes, two skips and one
failure. Its 80 links consumed **142.683s CPU** with **175.095s active wall**;
four compile commands consumed **10.831s CPU**. Whole supervisor CPU was 661.79s
user and 158.72s system. The remaining failure was the legacy config-alignment
sweep; coverage floors were not evaluated. Raw evidence is in linux-meta-current.

The removed internal coordinator test is replaced by a public REST resource
capacity journey that verifies owned session/resource identities, accepted
changes, stable replay decisions, request-ID conflicts, stale revisions, missing
resources and subsequent valid changes. Three repetitions pass in 2.533s. The
previously missing setFactorySessionResourceCapacity surface now has reviewed
functional evidence; the total inventory still contains 156 reviewed surfaces.

Durability resume now waits for canonical SESSION_COMPLETED through the public
Factory Event history. The response endpoint retains the interrupted attempt;
it cannot provide resumed completion. Private snapshot filesystem notifications
were racing actual successful REST state on Windows. The snapshot-file-only
case and its entire unused fixture are removed, along with cross-project private
snapshot manipulation. Public restart recovery, checkpoint identity, final
results and cross-project REST isolation remain covered, with three focused
repetitions passing in 18.823s.

The legacy config-alignment sweep and its three files are removed. They combine
private flatten/readback analysis, five retired-alias rejection cells, cron
startup, script retry timing, resource accounting and internal topology analysis.
The current customer execution guarantees remain in focused workflow, Work,
worker, resource-concurrency, automation and API suites. The new full measurement
rechecks package floors after this cleanup; floors are not lowered to accommodate
it. The coverage runner unit suite, focused built-in lint, custom analyzers and
ordinary/functionallong compile checks pass.

Recent pushes have no hosted checks because PR #2867 now conflicts with live
main. The next source checkpoint must rebase onto the current main and receive
fresh hosted verification. Local passing or failed measurements before that
rebase do not establish hosted checkpoint latency.


The cleanup follow-up, linux-meta-verified, took **213.292s test wall** and
**225.97s supervisor wall**: 87 packages, 940 top-level tests, 936 passed, two
skipped and two failed. Its 79 links consumed **130.353s CPU** with **163.365s
active linker wall**; four compile commands consumed **8.850s CPU**. Whole
supervisor CPU was 648.73s user and 152.52s system. Failures were a shared review
fixture's session-open request returning HTTP 504 under contention, and a mock
ACP test's temporary directory receiving writes during cleanup. Neither is an
elapsed-time checkpoint, and coverage floors were not evaluated. Live main has
since changed process/session lifecycle code; rebase and current-source
verification precede further fixes rather than patching the superseded lifecycle.

## Rebase, full measurement, and public replay synchronization

The working branch was rebased onto main `c038071b04` after an actual merge
conflict prevented PR CI from starting. Main's new cancellation and gateway
customer tests were retained at their relocated package paths. Wire output was
regenerated from the merged construction source. The complete functional source
inventory compiles, and the scenario projection validates 159 reviewed surfaces.

Hosted run [37317442555](https://github.com/portpowered/you-agent-factory/actions/runs/37317442555)
at `849480ac49` has 88 selected packages, 946 top-level tests, 944 passes,
two skips and zero test failures. Its test invocation took **344.020s**;
the full supervisor took **419.113s** (13:35:30.551–13:42:29.664 UTC).
The subsequent coverage gate failed: Live Change covered 198/349 statements,
56.7335%, below its unchanged 57.77% floor. This is **not a passing checkpoint**.
A historical cleanup error appears in the separate flake evidence even though
the current timing inventory records zero test failures; it remains a cleanup
ownership concern rather than evidence that the complete verification passed.

The four-CPU local measurements on this base were:

| Sample | Full supervisor | Test invocation | Links | Link CPU | Active link intervals | Compile CPU | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Rebased source, changed coverage graph | 276.26s | 209.354s | 82 | 116.168s | 156.748s | 288.360s | Two Response Event replay tests failed |
| First retained-count helper, warm graph with changed tests | 206.93s | 194.221s | 80 | 117.860s | 148.960s | 48.577s | Three customer tests failed |

The first helper implementation incorrectly excluded STREAM_GAP from the
advertised count. The advertised catch-up prefix includes that marker; it now
consumes every advertised frame. The complete Response Event package passes
three repetitions in 34.327s. The failed full sample above remains recorded;
its other failures were the selected-process-time journey and the selected
Work watch reconnect/cancellation journey. They are retained customer behaviors.

The underlying original helper ended history collection after 25ms of silence.
CPU contention can insert a longer gap between retained SSE frames, yielding
an empty or partial history. The replacement observes the public
`X-Factory-Session-Retained-Response-Event-Count` header and reads exactly that
prefix. It removes the idle timer, separate scanner goroutine, and their timer
baseline allowance. It does not lengthen the quiet period or weaken replay
assertions.

Hosted CI also found the docs-smoke Make target still pointing to the retired
Docs package. It now selects the documentation subtest in the consolidated CLI
parent and passes locally. Main intentionally changed baseline policy to permit
count-neutral replacements; the relocation unit now tests actual added debt
while retaining the exact file/function relocation check. Focused baseline units
pass. A TTY output assertion was extracted to reduce the migrated CLI helper's
cyclomatic complexity; focused built-in lint reports zero issues.

A source/reference audit found Live Change's old `New`, `Apply`, and `Recover`
adapters have no production callers. They existed only to construct the
component in unit tests and in the removed internal functional characterization.
Those compatibility adapters are removed; the canonical wire coordinator and
its `ApplyLiveChange`/`RecoverLiveChange` operations remain. Component tests now
construct their own state and exercise the private policy implementation;
they pass. The REST resource-capacity journey no longer requests MockWorkers.
Coverage floors have not been lowered. A new complete run is required to assess
this source and all gates together.

The corrected retained-count follow-up took **196.82s for the full local
supervisor** and **179.743s for the test invocation**. It executed 946 top-level
cases in 88 packages: 943 passed, two skipped, and one failed. The remaining
failure is `TestProcessTimeJourneys/TestSpecializedSourceTimeJourney`: public
session close can race the background CLI's final activation binding, producing
“activated session ... is unavailable” during daemon cleanup. Response Event
replay and selected Work watch passed in this sample. All 80 link commands
consumed **110.065 CPU-seconds** across **137.614s of active linker intervals**;
121 compile commands consumed **62.865 CPU-seconds**. Whole-run user/system
CPU was 579.51s/117.47s. This source-changed warm-build sample remains failed;
coverage floors were not evaluated after the test failure. It proves neither
a three-minute complete lane nor a hosted checkpoint.

The next consolidation candidates are the fixture-owning packages whose
`TestMain` can become a named parallel parent with cleanup after its children.
The existing fixture variables must reset between repetitions, and assets and
subprocess cases require separate handling. The private one-binary experiment's
99% link-CPU reduction remains strong evidence for this direction, but passing
customer execution and complete hosted gates remain the release criteria.


## Immutable startup work and duplicate scenario execution

Two successive hosted samples used the customer-lifecycle consolidation:

| Revision | Full functional supervisor | Coverage invocation | Compile commands | Link commands | Final outcome |
| --- | ---: | ---: | ---: | ---: | --- |
| `e120ac9028` | 398.946s | 326.594s | 694 | 62 | 825 passes, two skips |
| `0958ddb01f` | 307.456s | 248.406s | 696 | 63 | 827 passes, two skips |

These are different hosted samples, not a controlled before/after benchmark.
The second revision restores MCP protocol's separate binary after combined
fixtures exposed initialization failures. Both diagnostics report compilation
work. The first run's Verification Policy failed because Backend Lint was
canceled by the subsequent push. The second run's required jobs all passed,
although its raw diagnostics retain recovered MCP stdio and mock-worker failures.
Neither full supervisor reaches the five-minute merge checkpoint.

At `0958ddb01f`, the slowest packages were:

| Functional package | Elapsed time |
| --- | ---: |
| Product customer journeys | 157.472s |
| Product customer lifecycles | 127.960s |
| Packaged Factory invocation | 85.694s |
| Factory execution | 77.547s |
| Models inference | 71.569s |
| Chat Sessions ACP | 51.193s |
| CLI customer commands | 45.360s |
| Providers ACP | 44.816s |

Package durations overlap and cannot be added to obtain CI elapsed time. The
supervisor also includes coverage tooling and concurrent quarantine validation.

Product customer journeys was profiled with complete production `-coverpkg`,
count coverage, `-ldflags=-w`, four pinned CPUs, `GOMAXPROCS=4` and parallelism
12. Each sample executed fresh tests once and passed. Profile CPU measures the
test process; command wall also includes the package build. These are individual
observations, not repeated statistical estimates.

| Sample | Command wall | Profile duration | Sampled test CPU |
| --- | ---: | ---: | ---: |
| Original startup behavior | 42.967s | 39.930s | 67.880s |
| Cache validated embedded publication | 38.192s | 30.690s | 48.790s |
| Also cache verified managed publication identity | 33.219s | 26.570s | 38.520s |
| Also deduplicate helpers and parallelize parents | 28.588s | 21.630s | 38.430s |

The initial profile attributed 36.46 CPU-seconds to system initialization,
22.44 to process construction and 30.12 to generated Factory JSON decoding.
These are overlapping call-tree totals. Repeated validation and installation
compete with test execution, in addition to the previously measured linker cost.

The embedded publication cannot change during a process. Its validated catalog
now initializes once; accessors still return detached payloads and formats.
Injected filesystems still validate every call. The existing test also verifies
that caller mutations cannot poison a second load and compares publication names
with the exact generated manifest, replacing a stale nineteen-factory list.
Three repetitions of focused catalog checks passed in 0.475s.

Each installer retains only a successfully verified publication fingerprint,
keyed by installation root, Factory name, root filename and payload SHA-256.
Customer files and management stamps remain freshly read on every reconciliation.
Changed input misses the cache; changed customer files retain the repair/backup
path. Prepared layouts stay operation-owned because persistence may mutate them.
A new installer computes its own fingerprints. Three repetitions of existing
reconciliation tests passed in 0.545s, including adoption, malformed evidence,
source refresh, customer modifications, contention and failures.

Eleven helpers had `Test...CaseN` names and ran directly as well as through their
parents' `t.Run`. Renaming them to private `run...CaseN` helpers preserves every
scenario assertion under the six customer-behavior parents. The parents have
separate process/home fixtures and now run in parallel without shared environment
mutation. Three repeated native workflow runs passed in 20.849s. The final
profile has a shorter critical path, with nearly unchanged sampled CPU compared
with the preceding sample; no further CPU reduction is claimed from that sample.

Hosted evidence: [e120 run](https://github.com/portpowered/you-agent-factory/actions/runs/37325476800),
[0958 run](https://github.com/portpowered/you-agent-factory/actions/runs/37327031568).
The cache and duplicate-run changes require complete-lane and hosted validation
before establishing a new checkpoint.


### Complete four-core validation of the cache and duplicate-run changes

The full supervisor passed in **192.74s overall / 143.595s for the coverage
invocation**, with 816 passes, two skips and no failures. All existing coverage
floors and quarantine inventory checks passed. The source snapshot uses the
final fifteen-suite layout with MCP protocol separate. Its 818 top-level cases
are exactly eleven fewer than the hosted 829: only the duplicate standalone
`CaseN` executions disappear; their original parent scenarios remain.

The complete command consumed 581.06 user CPU-seconds and 129.61 system
CPU-seconds. Compiler instrumentation recorded 1,559 compilations consuming
224.163 CPU-seconds and 113.921s of active wall time; 69 links consumed
90.324 CPU-seconds and 109.670s of active wall time. These tool intervals overlap
one another and test execution. There were also 563 vet invocations in the
supervisor/tooling paths (17.720 CPU-seconds), even though the functional
coverage invocation itself uses `-vet=off`. Product customer journeys passed
in 80.669s, customer lifecycles in 71.758s, packaged invocation in 33.986s and
Factory execution in 28.609s under this lane's contention.

This measurement rebuilt dependencies in a fresh source directory. It is not
a controlled warm comparison with previous samples, and it does not establish
a hosted checkpoint. It does provide complete passing verification with the
unchanged gates, rather than extrapolating the isolated profile to CI.


### Further customer-proof cleanup

Two `readWorkAtState` fake-HTTP self-tests are removed because they exercise
test helpers rather than the application. Removal of a legacy metadata
conversion/clone roundtrip and programmatic metadata-snapshot rejection was
also measured, but is not shipped yet: that removal uncovered two existing
functional package floors that require equivalent customer-boundary proof. The real filesystem watcher scenarios and their public Work observers
remain. These tests have no reviewed scenario declarations. They are not removed
because of a runtime failure.

The six retry scenarios previously in `tests/functional/workflow` move into
Product customer journeys, removing one more application-linked binary. Each
has its own process, home, Factory fixture and controlled provider. They run in
parallel. Success observes correlated terminal Work directly rather than an
unconditional quiet window; rejection/exhaustion still inspect terminal Work
and public dispatch events. The long cases retain their `functionallong` tag.
Three repeated focused runs, including the long retry cases, passed in 8.114s.
Three obsolete missing-subsection exemptions for the vacated workflow sources
are removed without adding new debt allowances. Complete coverage validation
of these changes is running separately from the preceding source snapshot.


The first full cleanup follow-up took **153.91s overall / 143.303s tests** and
failed: 811 passes, two skips and one failure. The recording-path reuse case
in Workers inference inherited the operator home, so packaged installation
contended on `/home/andre/.you-agent-factory/factories` with other processes.
This is an isolation failure, not a reason to remove the recording behavior.
Both invocations now use one test-owned home and the existing isolated model
cache environment. The test still reuses the same process and recording path
and proves new Worker/recording identities and unmerged durable history.
Three focused repetitions pass; a full isolated follow-up is required.

The failed run observed 65 links / 104.767 linker CPU-seconds and two compiles /
5.760 compiler CPU-seconds. Its warmer cache differs from the preceding fresh
source rebuild; a shorter failed run establishes no latency checkpoint.


### Hosted validation of immutable caches and duplicate-run removal

Revision `02a491c07b` passes the hosted functional job and unchanged coverage
gates: **333.016s full supervisor / 257.763s coverage invocation**, 816 final
passes, two skips and no final failures. It records 696 compiler commands and
63 linker commands. Providers' timeout-recovery case and JavaScript mock-worker
case failed initially and passed retry; raw diagnostics retain both failures.
Product customer journeys takes 151.687s, customer lifecycles 143.794s, packaged
invocation 81.356s, Factory execution 59.471s and CLI customer commands 54.272s.

This is slower overall than the preceding hosted 307.420s supervisor despite
the isolated CPU improvement. The profiles establish avoided computation, not
a hosted latency checkpoint. Cache state, contention and retries prevent treating
single samples as a controlled comparison. The build-only one-link result remains
a strong reason to continue consolidation, while complete execution and unchanged
gates are required before shipping a monolith.

Exact supervisor timestamps are retained in each run's
`c09-critical-path/critical-path-timing.txt`. The earlier 0958 value of 307.456s
in this document spans the whole workflow step including its shell preamble;
the supervisor itself is 307.420s. The e120 supervisor is 398.914s (398.946s for
the workflow step). Future checkpoint comparisons use supervisor start/end.
[Hosted run 37331029622](https://github.com/portpowered/you-agent-factory/actions/runs/37331029622).

The isolated-home cleanup repeat takes **139.92s overall / 126.732s tests**
and still fails one existing Worker Session observation case (811 passes, two
skips). Its recording fake represents missing history as a generic error, while
the real reader returns typed `os.ErrNotExist`, which the application recognizes
as absent live history. The fake now preserves that error contract. Five focused
repetitions of selected-session observations, recording reuse and opening-gate
scenarios pass in 12.776s. The inference environment helper also adopts the
shared isolated-home helper so an operator's explicit model cache cannot leak in.
The failed run records 65 links / 95.130 linker CPU-seconds and two compiles /
1.373 compiler CPU-seconds. Its shorter failed duration establishes no checkpoint.


The corrected follow-up has **812 passes, two skips and no test failures**, with
**133.49s overall / 122.177s tests**, but fails two unchanged coverage floors:
NameValue metadata validation drops from the required 72.22% to 29.63%, and
Factory snapshot mapping from 75.00% to 68.75%. These gates are not weakened.
The metadata-only tests remain temporarily pending replacement with appropriate
public behavior; the two polling-helper self-tests can be removed independently.
This failed run's 65 links consume 87.740 CPU-seconds, with 103.300s of active
wall time; two compiles consume 0.999 CPU-seconds. A shorter invocation with a
failed gate establishes no checkpoint. Recording reuse and selected-session
observation now pass in this complete run with owned homes and typed fake
missing-history behavior.


### Rebase onto current main

The branch is rebased onto `c54a1263c6` after main added selected Worker captured
activity reads and required the selected packaged-installation logger. The
canonical Wire graph is regenerated from current providers to preserve those
new roles alongside legacy model removal. Eight new Worker HTTP fixture files
move with their existing suite into Product customer journeys, with unchanged
customer assertions and helper ownership. Their package declaration and scenario
references follow the new location. Main's captured-log evidence supersedes the
older read-surface declaration where appropriate. No new binary is introduced
for these additions. Verification and full-lane measurements from before this
rebase remain labeled by their source revisions; they do not verify this new
combined source snapshot.


The first complete four-core rebased snapshot takes **198.81s overall /
151.518s tests** and fails: 819 passes, two skips, one existing Work-watch
startup failure (`SERVER_START_FAILED` before host readiness). The consolidated
customer-journey package, including main's newly moved captured-log cases,
passes, as does Workers inference with the recording corrections. The isolated
captured-log selection also passes natively in 20.896s. The rebased manifest
checks all 160 reviewed scenarios. This full run rebuilds 1,571 compiler actions
(225.939 CPU-seconds, 117.583s active wall) and 68 links (93.624 CPU-seconds,
110.612s active wall). The unrelated readiness failure is retained and prevents
using this sample as a checkpoint. A fresh execution with warm build caches
follows to verify the complete combined source and coverage gates.


### Rebased warm lane and readiness correction

The warm repeat on the current-main rebase took 137.03 seconds for the full
supervisor and 126.450 seconds for the coverage invocation. It recorded 819
passing tests, two skipped tests and one failure: the same Work-watch customer
journey failed when public `session create` returned HTTP 504. There were no
compiler actions; 65 linker actions consumed 88.443 summed CPU seconds over
98.412 seconds of overlapping active intervals. This failed sample is not a
latency checkpoint.

The selected-clock fixture advanced its entire process clock by 10 milliseconds
for each runtime-readiness poll. Under contention, repeated polls could consume
the independent one-second startup deadline before the runtime goroutine made
progress. Readiness now receives a fixture-owned timer wakeup without advancing
that clock. Reconnect timers and the public 99/100 millisecond boundary assertions
remain controlled by explicit clock advancement. Host initialization also uses
the common isolated customer-home/model-cache environment. Five focused native
runs passed in 31.634 seconds. A complete lane measures the correction separately.


The corrected complete sample took 155.12 seconds overall and 144.144 seconds
for the coverage invocation. Work-watch passed in 13.655 seconds under full-lane
load. The sample still failed: 818 tests passed, two skipped and two failed.
Runtime-metrics cancellation reported that closed-session history changed after
clock advancement and peer completion. Mock-worker cleanup reported a nonempty
temporary directory during removal. Both raw failures are retained. The 65 linker
actions consumed 102.978 summed CPU seconds and 109.305 overlapping active
seconds; two compiler actions consumed 0.549 CPU seconds. This sample does not
establish a successful sub-three-minute lane.

The refreshed private monolith experiment includes 63 package groups, 741 Go
files and 823 Test-prefixed declarations. It retains package fixtures, embedded
assets, source mappings and evidence declarations. Unlike the earlier build-only
transform, executable variants retain package initialization functions.


### Current-main one-binary execution experiment

The refreshed four-core experiment built and executed one binary in 145.28
seconds. Its single linker action took 2.186 wall seconds and 2.142 CPU seconds;
two compiler actions consumed 33.707 CPU seconds. Package execution took 128.713
seconds. This establishes the linker reduction, not a passing CI checkpoint:
the mock JavaScript ACP test failed, as did an invocation-scoped cancellation
case. Parent cohort failures are additional JSON events for those same failures,
not independent failed customer scenarios. Peak RSS was 6,325,684 KiB, compared
with 1,362,000 KiB in the preceding native-package lane. A single binary therefore
also concentrates memory and fixture concurrency; its end-to-end benefit is much
smaller than its isolated linker benefit in this sample.

The mock JavaScript ACP fixture unnecessarily changed the process-wide working
directory while running in parallel. Its command now supplies an absolute Factory
path and an explicit command working directory. Removing the global mutation
alone initially exposed the relative Factory-path dependency; making the path
absolute preserved the customer invocation. Five focused native repetitions
passed in 3.729 seconds, and changed watch/mock scopes passed built-in and tagged
repository lint. The private monolith is repeated with this correction before
considering any shipping layout.


The repeat with an absolute mock-worker Factory path passed: 133.03 seconds
command wall and 116.653 seconds package execution, with no failed JSON events.
The one linker consumed 1.609 wall seconds and 1.585 CPU seconds; two compilers
consumed 32.799 CPU seconds. Whole-command CPU was 288.91 user plus 83.21 system
seconds. Peak RSS was 6,584,844 KiB (6.28 GiB). The recorded profile was checked
using the repository's own canonicalization, coverage evaluation and blocking
manifest functions through a private audit-only harness: 61.4% total exceeds the
unchanged 33.1% total threshold, and all active package gates pass with the
existing 0.25-point epsilon and existing main holds. No baseline was edited.
The private harness and monolith are not committed production tests. This
measurement excludes the full functional supervisor and hosted CI environment;
it proves feasibility, not completion of a latency checkpoint.


### Shipping CLI and REST fixture consolidation

Nine fixture-owning packages join `product/cli_rest_journeys`: Provider Session
CLI diagnosis, Models catalog/invocation CLI, Factory YAML parity, Factory
validation/persistence, named Factory lifecycle, Session CLI controls, Work CLI
collections, Factory Events and REST server behavior. Forty-three existing source
files retain their customer assertions beneath named parallel scenario parents.
Each parent constructs and cleans up its own former package fixture; cleanup
runs after parallel children. Repetition resets fixture-owned state. This removes
eight native package links without shipping the private monolith fixture harness.
Two complete native repetitions passed in 58.536 seconds before the additional
cleanup and 62.728 seconds after it. Built-in and tagged repository lint pass.
All 160 reviewed scenarios retain their decisions; evidence declarations move
to executable scenario parents, and existing lint-debt identities follow their
source moves without adding allowances.

The concurrent REST identity proof now performs one overlapping observation
per owned session rather than ten repeated rounds. Distinct identity and
cancellation observations remain. Direct filesystem implementation checks and
durable-appender component failure probes are removed from the functional
webhook case. Their behavior is a component concern; customer webhook delivery,
recorded history, signing, filtering, retries and dead-letter artifacts remain.
Public Work Request observation uses a bounded ticker instead of a fixed 50 ms
sleep. Existing cleanup and failure observation ceilings are unchanged.

Two attempted full measurements in the reused private Linux checkout were
invalid because functional discovery also found the completed experimental
monolith sources. Removing those sources from Git's index did not exclude them
from the repository's filesystem discovery. The experimental directory was moved
outside the functional source tree, and a clean discovery now selects 69
packages. Neither incomplete attempt counts as latency or coverage evidence.


The first platform-complete native consolidation sample passed all 755 selected
tests with two skips: 133.92 seconds full supervisor / 124.764 seconds coverage
invocation. It recorded 57 linker actions, 79.944 CPU seconds and 102.292 seconds
of overlapping link-active intervals; four compiler actions consumed 3.200 CPU
seconds. Whole-command CPU was 380.20 user plus 104.81 system seconds, and peak
RSS 1,359,684 KiB. It failed the unchanged filesystem floor: 31/47 statements
(65.9574%) versus 75%. This is a failed gate sample, not a latency checkpoint.
The largest executing packages were Product customer journeys (91.346s), Product
customer lifecycles (81.527s), packaged invocation (47.874s), the new CLI/REST
journeys (43.430s), Factory execution (39.372s), and Models inference (29.054s).
These package elapsed intervals overlap and cannot be added to supervisor wall.

A Linux compile check also caught a non-Windows memory observation helper that
the Windows typed source load had omitted. Both platform variants now move with
the Provider Session CLI fixture. The early compile-failed sample did not check
coverage; no latency conclusion is drawn from its short termination.

To retain the filesystem floor through customer behavior, the existing webhook
append-failure scenario now uses an actual blocked destination directory through
the exact appender edge. Two Linux cases add an exhausted file and unavailable
durable flush. Each case submits Work through the CLI, observes signed delivery
and retained canonical history through public interfaces, verifies one terminal
append attempt at the external-effect boundary, and proves an independent peer
continues delivering. The dead-letter record is still checked for canonical body,
identity, retry classification and redaction. Device fixtures are confined to
scenario-owned symlinks, and non-Linux execution skips those two device cases.
Three Windows repetitions passed in 33.541s; three four-core Linux repetitions
passed in 7.860s. Built-in and tagged repository lint report zero issues.


The customer file-failure replacement passes the complete four-core canonical
lane: **130.64s full supervisor / 120.533s coverage invocation**, 755 top-level
passes, two skips and no failures. The original total and package coverage gates
and all quarantine checks pass. Total coverage is 61.4%; filesystem coverage is
36/47 statements (76.6%), above its unchanged 75% floor. The recorded tool trace
has 57 linker actions consuming 76.129 summed CPU seconds over 97.962 seconds of
overlapping active intervals. Two compiler actions consume 2.880 CPU seconds.
Whole-command CPU is 366.93 user plus 103.21 system seconds; peak RSS is
1,422,344 KiB (1.36 GiB). This avoids the experimental all-in-one candidate's
6.28 GiB peak while retaining measured link reduction. The failed predecessor
was 133.92s; these are individual samples, not a statistical speedup estimate.

The largest package intervals in the passing sample are Product customer journeys
83.254s, Product customer lifecycles 74.332s, CLI/REST journeys 50.061s, packaged
invocation 45.905s, Factory execution 36.648s and Models inference 28.192s. They
identify the next fixture/compute targets; they overlap and are not additive.
Thirteen resolved sleep/deadline debt records are removed: the fixed sleep no
longer exists, and existing signal-driven failure/cleanup ceilings now explain
their purpose in code. No coverage floor or debt allowance is increased.


### Hosted current-main rebase measurement

The hosted functional job for `79cb69b1e7` succeeds: 332.773 seconds full
supervisor (16:40:14.102Z through 16:45:46.875Z), 258.435 seconds coverage
invocation, 820 final passes, two skips and no failures. It still exceeds five
minutes. Required Backend Lint fails on the now-empty `tests/functional/workflow`
package stub after its customer cases moved to Product customer journeys. The
obsolete `doc.go` stub is removed rather than adding a shape exception. The
new nine-suite consolidation and customer file-failure cases are the next hosted
candidate. No checkpoint merge is claimed from this revision.

### Consolidated API smoke selector correction

Hosted run `37344044590` finds three Make targets still selecting the retired
HTTP server package. The API contract job fails before running the customer
case (`no Go files`), so this is a package-routing failure rather than a failed
REST assertion. `api-smoke`, the focused HTTP baseline, and the related contract
target now select `product/cli_rest_journeys` and the exact nested
`TestRESTServerJourneys/TestGeneratedClientAndServerSchemaStayAligned` case.
The dollar anchor is escaped for Make. Native Go JSON confirms that the moved
case and its fixture-owning parent run and pass (1.550s package elapsed).
Scenario projection defaults now reference the executable CLI/REST parents;
the reviewed 160-decision manifest remains unchanged. The scenario package's
unit tests pass in 0.164s.

### Memory-limited monolith experiments

The retired private single-binary sources are temporarily restored under their
original functional path for measurement, then moved back outside discovery.
An initial attempt to run them under `.artifacts/` fails immediately because Go
correctly forbids importing the functional internal REST client from there.
It supplies no latency evidence. The first correctly located `GOMEMLIMIT=3GiB`
sample takes 130.68s command wall and peaks at 3,463,308 KiB (3.30 GiB), but
fails evidence-declaration checks because the native lane had restored its
current registry while these older generated sources require their original
exact nested aliases. This failed sample cannot establish behavior equivalence
or a checkpoint. A repeat installs the corresponding source revision's registry
and exact mapped aliases only for the private run, restoring the current native
registry in cleanup. The memory limit is a Go GC target, not a hard RSS ceiling.

The corrected registry repeat passes: **139.04s command wall / 134.011s package
execution**, one link at **2.390s wall / 1.973 CPU seconds**, and peak RSS
3,529,376 KiB (3.37 GiB). Whole-command CPU is 343.42 user plus 90.46 system
seconds. No compiler action is recorded, so this is a warm-source-build sample;
it must not be compared as a cold build with the earlier 133.03s experiment.
The repository's actual profile evaluator passes unchanged total and active
package gates at 61.4% total coverage. There are no failed Go JSON events;
2,254 passing events include nested cases and wrappers and are not a count of
distinct original top-level tests. The registry and retired source location
are restored after execution. This still excludes the canonical supervisor and
hosted runner.

Increasing the same memory-limited binary to `-parallel=48` fails and is slower:
**148.62s command wall / 143.450s package execution**, peak RSS 3,318,448 KiB
(3.16 GiB), one link at 2.415s wall / 2.028 CPU seconds, whole-command CPU
369.79 user plus 96.82 system seconds. The selected Worker Session MCP read
exceeds its deadline, and two JavaScript ACP prompt cases receive request
cancellation. Eight failed JSON events include affected parent wrappers.
This concurrency setting is not a passing optimization and is not adopted.

### Hosted nine-suite consolidation result

Hosted run `37344044590` (`d9c1aa259f`) passes the complete functional supervisor
in **320.437s** (17:05:45.416Z through 17:11:05.853Z), with **250.280s** coverage
invocation, 755 final passes, two skips and no failures. Quarantine succeeds in
56.975s while overlapping the coverage child. This sample is below the preceding
332.773s result but still exceeds the five-minute checkpoint by 20.437s. The
separate API package job fails on the retired Make selector described above;
revision `38d312b762` carries its correction. The three-minute and two-minute
checkpoints remain unmet. No merge is claimed.

### Fleet pagination fixture cleanup

The CLI/REST fleet continuation proof now uses six Workers across three owned
Factory Sessions, with a four-row first page and two-row continuation. Each
session still contributes both a COMPLETED and FAILED Worker Session. Ordered
identity, attribution, filtered selection, malformed-token behavior, empty
selection, public session visibility and CLI/REST parity assertions remain.
The case documents its serial execution because the root fleet projection
includes parallel siblings' rows. A duplicate identical terminal fleet read
is removed; the active-to-terminal ordering proof remains.

Process working-set/commit/hostname observations and response-stopwatch logs
were performance diagnostics rather than customer assertions. They are removed,
including the Windows and unsupported-platform memory helper files. Performance
measurements stay in the audit harness. The focused pagination/concurrent-fleet
pair passes three repetitions in 22.420s, and union-tag repository lint reports
zero issues. These changes do not establish a full-lane latency result.

### First hosted five-minute timing observation and merge gate

Run `37346055096` (`38d312b762`) passes the full functional supervisor in
**260.106s** (17:26:37.668Z through 17:30:57.774Z), with a **202.865s** coverage
invocation, 755 final passes and two skips. Quarantine succeeds in 47.869s while
overlapping coverage. The API package check also succeeds with the corrected
Make selector. This is the first passing hosted supervisor below five minutes.
Other required jobs on this revision are cancelled by a subsequent push, so
this observation does not establish a merged checkpoint. Earlier passing
320.437s and 332.773s observations remain part of the measured range.

Run `37347513698` (`91b288cfd4`) fails required Backend Lint on three generated
fixture resets that copy structs containing a Mutex or Once. It also fails the
required functional verdict: the reduced pagination fixture accidentally made
an active-fleet sibling's default-page-size assertion expect four instead of
the public default of twenty. Its recorded supervisor interval is 212.793s and
invocation is 166.920s, but the final summary retains two failed tests, so this
is not a passing latency checkpoint even though the supervisor status files
record zero. The required verdict, not a short elapsed interval, controls merge.

Local corrective commit `676f7a1e05` initializes fresh lock-bearing fixture state
directly from composite literals rather than copying another value, and makes
the empty-page assertion accept the selected expected limit. The continuation
case still checks its explicit four-row limit; the active case retains the
default twenty-row contract. Three repetitions of all three fleet cases pass
in 33.442s, and tagged `go vet` passes. The user subsequently authorized the
corrective push needed to unblock merging, and commit `676f7a1e05` was published.
The merge request is enabled, but GitHub
reports `BLOCKED` on the current published head's required checks. Further
optimization remains separate from the minimal merge correction.

### Package-preserving consolidated build prototype

A private overlay experiment reuses the unit lane's package/bridge construction
to combine 35 compatible functional packages (426 original top-level tests)
into one binary, while explicitly leaving 19 custom-TestMain packages native.
Production exports and original source packages are unchanged. The initial
warm-dependency build succeeds in 9.37s, with 24.28 user plus 3.28 system CPU
seconds and 835,476 KiB peak RSS (0.80 GiB). Subsequent coordinator corrections
relink from cached package archives. This prototype is not shipped and has not
executed its 19 native exceptions or the canonical supervisor.

The first execution panics because parallel coordinator ancestors prohibit
existing process-wide `t.Setenv` calls; 412 expected tests remain unexecuted.
It supplies no complete latency evidence. A repeat preserves serial barriers
for the four active source packages with global environment/directory mutation:
bootstrap portability, coverage observability, JavaScript worker scenarios and
script workers. All 426 expected tests then run, but execution fails in 96.73s
(96.045s package interval), with 43 failed JSON events including wrappers and
peak RSS 3,180,728 KiB (3.03 GiB). Existing subprocess fixtures require their
original test selectors, and the fleet default-limit mistake is also exposed.

A repeat routes unique legacy helper selectors through their exact coordinator
group and includes the fleet assertion correction. All 426 expected tests run,
but four original ACP scenario families still fail (ten failed JSON events
including wrappers): terminal output, permission selection, and new/config RPC
failures. It takes 98.99s command wall / 98.283s package interval, with 228.93
user plus 75.18 system CPU seconds and 3,180,512 KiB peak RSS. These failed,
partial-lane observations demonstrate build feasibility and remaining fixture
coupling; they do not establish equivalent coverage or a CI checkpoint. Source
revision registry declarations receive exact temporary coordinator aliases only
during each private run and are restored in cleanup. No tests are silently
dropped, no guard is increased, and no native exception is reported as executed.

### Corrected-head hosted retry and runner acquisition

Run `37366971390`, attempt 1, ended with numerous cancelled required checks.
The Docs Reference check annotation states: "The job was not acquired by Runner
of type hosted even after multiple attempts". Its job had no runner or steps;
this cancellation does not measure test execution. The failed-job rerun was
requested without another source push, and attempt 2 acquired runners.

On the same corrected head `676f7a1e05`, attempt 2's functional check passed.
The supervisor ran from 22:12:13.417Z to 22:15:53.834Z on 2026-10-05:
**220.417s total wall time**, including concurrent quarantine and coverage work.
Quarantine took 42.440s and overlapped the coverage lane. The functional timing
summary records **169.582s invocation wall time**, 68 expected/observed package
entries, 757 top-level results, 755 passes, two skips, and zero final failures.
The workflow flake ledger records one `TestPackagedLoop` failure that passed on
same-head retry; the failure was an eight-second scheduled-execution readiness
guard. Retain that retry when interpreting this observation and prioritizing
future synchronization cleanup.

This successful full functional interval meets five minutes but exceeds the
three-minute target. At evidence collection, backend lint was still running,
and the PR was open with auto-merge enabled. A timing observation alone does
not establish a merged checkpoint.

Attempt 2 ultimately passed every applicable check, including Backend Lint
(16m8s job duration) and Verification Policy. PR #2867 entered the merge queue
at 22:30:23Z behind PR #2868. A read-only merge-tree comparison against that
queued commit exposed a conflict confined to generated `pkg/wire/wire_gen.go`.
The authored source graph merged cleanly. A separate checkout merged the queued
commit, regenerated Wire, and passed `go test ./pkg/root ./pkg/wire -short
-count=1` (root 0.034s, Wire 25.878s). PR #2868 then merged as
`8284dec71ffaf510c389d3fbb38e16018255c5ae` at 22:39:17Z, and GitHub classified
our PR as conflicting and removed it from the queue.

Merge correction `6759f55ae3a1f6b82e297b59268e3422abc02b22` merges that live main
commit and regenerates Wire. Its generated file matches the tested preview
byte-for-byte; there are no unresolved paths and `git diff --cached --check`
passes. The correction was pushed under the user's authorization to unblock
merging. New-head run `37383931843` started, and auto-merge was enabled again.
The additional optimization prototypes and this unpublished audit were excluded
from the correction.

The corrected-head functional check in run `37383931843` passed, but was slower:
supervisor 22:41:36.403Z to 22:47:17.253Z, **340.850s total wall**, with quarantine
57.969s overlapping coverage. The invocation summary reports **268.630s wall**,
69 expected/observed package entries, 758 top-level results, 756 passes, two
skips, and zero final failures. It exceeds both five- and three-minute targets;
do not carry the previous head's faster interval forward as this head's result.
The slowest package execution intervals are customer journeys 160.694s,
CLI/REST journeys 92.031s, packaged invocation 88.159s, factory execution 69.831s,
Models inference 54.934s, and CLI customer commands 53.550s. These intervals
overlap and are not independent amounts to add to total wall time. At collection,
lint and the packaged-factory candidate build were still active, with no failed
checks. The current priority remains completing the authorized merge before
publishing more optimization changes.

Merge-queue run `37385807288` tests candidate
`0dcdbf1fde04e573730e19c4b39fe097e7389d8c`. Git tree IDs confirm its source tree
is identical to corrected PR head `6759f55ae3`. Its functional check passes in
**272.327s supervisor wall** (23:00:05.530Z to 23:04:37.857Z), with **215.102s
invocation wall**, 69/69 expected package entries, 758 results, 756 passes, two
skips, and zero final failures. Quarantine takes 46.590s and overlaps coverage.
The same source tree thus has successful 340.850s and 272.327s observations;
the 68.523s difference is not evidence of an intervening source optimization.
Both observations and their retry work must remain visible.

The corrected-head run's flake ledger records one review-failure-recovery case
that passes on same-head retry. The queue run records three recovered cases:
resume recovery retaining a scenario TCP port, MCP initialization exceeding its
five-second response guard, and an MCP ACP permission case failing to resolve
`@you/subagent` from a project/global factory catalog. Each passes on same-head
retry. These are concrete remaining synchronization, teardown, and isolation
targets; a green retried verdict does not imply the first attempt was clean.
At collection, all other queue checks had passed and Backend Lint remained
active. The queue observation meets five minutes, but not three minutes, and
the PR had not yet merged.

The queue run ultimately completed successfully with no failed checks. GitHub
merged PR #2867 at **2026-10-05 23:11:00Z**, as commit
`0dcdbf1fde04e573730e19c4b39fe097e7389d8c`. This establishes the first merged
five-minute checkpoint using the queue's 272.327s full supervisor observation,
with the recovered flakes and slower identical-tree observation disclosed above.
The three- and two-minute checkpoints remain outstanding. The checkout now
starts the next optimization branch from that live main commit, retaining this
audit locally; no additional optimization changes were pushed to unblock merge.

### Next checkpoint: compatible build consolidation

The canonical full Linux supervisor on four pinned CPUs passes in **111.55s**
(297.68s user + 83.06s system, 2.01 GiB peak RSS). Coverage invocation is
103.120s: 69/69 original package entries, 758 selected tests, 756 passes, two
skips, no failures and no retries. The unchanged blocking policy retains its
existing staged coverage holds. No floor, hold or quarantine selector is edited.
The build trace records zero compiler commands and 37 linker commands: this
repeat uses a warm compiler cache. Phase times are list 1.706s, plan 0.893s,
test including diagnostics 107.048s, canonicalization 0.168s, evaluation 0.014s
and manifest processing 0.022s. These phases are sequential, while linking and
test execution overlap inside the test phase.

The overlay merges 19 compatible packages containing 171 original tests.
Thirty-six packages retain native binaries for custom TestMain, process-global
state, helper executables or relative/embedded fixtures. Production boundaries
and source files remain unchanged. Native exceptions execute within the same
bounded lane; test-level quarantine selection remains native. Normalized JSON
preserves original test identities, nested names, source locations, failure
capture and evidence declarations. Missing terminal events and registration
mismatches fail the lane. A same-head retry uses the original native package.

Failed development samples are not checkpoints: 130.00s had a provider timeout
recovery observation race and coordinator inventory bookkeeping; 110.14s passed
all customer cases but failed raw-capture completeness. Both failures are fixed
and covered by runner checks. Batch admission now waits for the exact accepted
Work projection; timeout recovery observes successful Work and verifies public
timeout/recovery events rather than assuming retry admission means completion.
Obsolete captured-log migration checks and direct internal publisher privacy
checks are removed; public CLI/REST/replay and live privacy cases remain.

The asynchronous quarantine ratchet previously occupied 30.824s of planning in
the merged-main local baseline. With overlap enabled it now runs beside coverage
and is joined before a passing verdict; current planning takes 0.893s. The
baseline's 208.424s full supervisor failed a batch Work visibility race, so it
is disclosed as a failed reference rather than a passing comparison. The earlier
private 427-test prototype was partial and cannot establish a full checkpoint.
Hosted confirmation and merge remain required for the three-minute milestone.

### Hosted cold-build gap and bounded compiler archives

Run `37390711081` at `0878fd26d0` passes in 316.816s supervisor wall, with
287.224s invocation wall, 1,355 compiler/73 linker trace commands and one
recovered MCP ACP permission case. Run `37391871366` after merging main
(`daf8e9afc6`) passes in 316.715s supervisor / 291.821s invocation, with
679 compiler/37 linker trace commands. Neither meets three minutes. The existing
67 MB dependency-cache hit does not retain compiled repository packages.

The new compiler-archive tier retains recently used Go archives and action
metadata only, with a hard 1 GiB bound. It excludes executable/test-result
payloads and cached executable directories, avoiding the former whole build
cache's 7 GB/four-minute restore. Keys include platform, Go version, module
inputs and source head, with a same-module fallback; Go checks source and tool
identities before accepting each artifact. Functional runs explicitly use
`-count=1`. The transfer reports retained bytes/files and omitted entries, and
the existing build diagnostic reports exact/fallback action-cache identity.

A real coverage-instrumented Go build proof restores only these artifacts into
an empty cache: the repeat performs zero compiler invocations and produces the
original output; changing source recompiles the affected packages and produces
the changed output. Filtering and byte-budget checks also pass, and this proof
now runs in Workflow Lint. Hosted cold-fill, restored-cache timing, transfer
cost and retained size still need measurement; a first cold fill is not claimed
as a three-minute checkpoint.

The private dispatch-concurrency experiment passes at 113.65s versus 111.55s
for the retained candidate, so its increase from two to four is not adopted.
It records 40 links across tests/tools, 61.181s linker CPU, 57.764s overlapping
active linker wall and 186.654s summed linker wall. Largest links are Product
customer journeys (8.829s), Product CLI/REST journeys (8.210s), Providers ACP
(7.311s), Runtime API Factory transformation (7.094s), and Providers (7.030s).
Their intervals overlap and cannot be added to supervisor time. Removing two
Git-file diagnostic helpers privately allows 21 merged groups/320 tests, but
that full sample takes 115.06s and raises peak RSS to 3.86 GiB; it is not adopted
as a latency improvement.

### First hosted archive fill

Run `37393044364`, head `6efdbac547`, has no archive hit. The supervisor takes
296.357s, including 265.724s of test invocation; all 69 package terminals and
759 test results finish (757 pass, two skip, one recovered permission-request
flake). The job fails afterward because the newly wired exact-hit diagnostic
receives an empty GitHub output on a cache miss. The correction explicitly
defaults that output to `false`; test and coverage gates remain unchanged.

Capture retains 12,129 files / 964,300,415 bytes with zero budget omissions.
Capture takes three seconds and cache save takes five seconds. Restored hosted
execution is still required before claiming the three-minute checkpoint.

A separate private archive-restore calibration takes 203.06s with 2,038
compiler tool records and 40 link records. It uses an older private source tree,
the unadopted 21-group experiment, and a capture-time recency window rather
than the workflow's job-start window. It cannot establish current-head latency
or cache effectiveness. Its 96.818s active compiler wall / 318.784s compiler
CPU demonstrates substantial misses, which require investigation if repeated
by the hosted restored run.

The coverage-specific cache proof reproduces one avoidable compile after an
archive-only restore: Go requires its separately cached static coverage
metadata (`\x00cvm`) even when the compiled archive is present. The filter now
retains that metadata too. Repeating fresh `go test -count=1 -coverpkg=./...`
against an empty restored cache performs zero compiles and produces an identical
coverage profile. All four cache proofs pass. Executable, result and counter
payloads remain excluded, and the same total byte limit applies.

Quarantine outcome and package-terminal invocations now use `-vet=off`, as
inventory discovery already does. The required canonical Backend Lint lane
retains vet ownership. This removes duplicate static analysis and regeneration
of vet-only cached source side files without changing runtime selection,
terminal-event evidence, retries, or quarantine expectations. Focused quarantine
and overlap tests pass.

### Corrected metadata fill and local restored execution

Hosted run `37393987887`, source `0e5bd429e7`, passes without retries:
202.617s full supervisor, 193.405s invocation, 467 compiler and 37 linker
trace commands. The earlier archive-only snapshot restores in four seconds
and applies in fifteen seconds (220 MB compressed / 964,300,415 bytes retained).
The corrected capture contains 13,707 files / 969,143,201 bytes, with no budget
omissions; capture and save each take four seconds. This is still above three
minutes and includes rebuilding missing static metadata.

The exact-source four-CPU local restored supervisor passes in 108.938s,
including 102.291s invocation, all 69 package terminals and 759 test results
(757 pass, two skip), without retries. Restoring into an empty cache performs
zero Go compiler invocations. There are 42 test/tool link records: 48.467s
linker CPU, 53.744s active linker wall, and 159.370s summed overlapping link
wall. Peak RSS is 2.22 GiB. The capture reaches the 1 GiB cap and omits 115
older entries; all selected customer cases and unchanged coverage gates pass.
This local result supports the corrected cache mechanism, but hosted reuse
including transfer cost still needs measurement.

The first exact-source local fill sample takes 150.247s and fails the existing
`TestPackagedFullFlowDispatchTerminalFinalization/RoutesNoResultFailureToLead`
case because its immediate public Work list is empty. It remains a failed
sample, not checkpoint evidence. An earlier local harness attempt has no Git
repository and fails before tests while reading the raw-failure artifact head;
that harness setup is corrected before the two full exact-source samples.

### Hosted restored result and complete-job budget

Run `37394846636`, head `e671ed2e38`, passes the full functional supervisor in
140.882s, including 132.624s invocation, zero compiler commands and 37 linker
commands. There are no retries. Archive download takes four seconds and the
second archive copy takes eight seconds. The entire functional job takes
213s (00:45:08–00:48:41 UTC), so this is not yet a three-minute total-job
checkpoint. Checkout takes 15s, Go setup 13s, other preparation and reporting
account for the remainder. These costs are included rather than hiding them
behind a passing supervisor-only measurement.

The next workflow uses a shallow checkout and the restored archive directory
directly as GOCACHE. A cold directory is seeded from the existing dependency
tier. Capture still filters archives, static coverage metadata and action
metadata into a fresh directory capped at 1 GiB before saving; executable and
test-result payloads remain excluded. A real coverage-test proof also executes
against the snapshot in place, with zero compiles and the same coverage profile.
Its external marker confirms all three cold/restored/in-place tests actually
run. Twelve workflow contract checks and actionlint pass.

Backend Lint on the hosted head rejects two monolith complexity violations.
Inventory validation is separated from construction and event identity/boundary
tests are split; the focused tests and scoped built-in lint now pass with no
allowance changes. Diagnostics also distinguish link-only work from compilation:
the earlier JSON correctly reports zero compilers but labels those 37 links
`compile-work-observed`; new diagnostics use `link-work-observed` for that case.

A private cleanup candidate relocates eight internal Make/coverage-runner checks
to their tool package, uses a scenario-owned project Factory for MCP ACP
permissions, observes terminal Work through REST, and copies current captured
journals for recovery fixtures instead of constructing legacy snapshots. Its
first warm control passes in 111.92s with 68 packages/751 results. Raising test
parallelism to 24 passes in 164.19s but needs four retries; a subsequent unchanged
12-way control passes in 162.52s without retries. This sequence does not establish
a parallelism improvement and the increase is not adopted. Linker CPU also
drifts between samples, so none is used to predict a hosted checkpoint.

### In-place archive restore: hosted job still exceeds three minutes

Head `4492b4921d451cf335eff925b5aaeff63deb516a`, Actions run
`37397370508`, functional job `112056602110`, passed with all coverage gates
unchanged. The complete job took **221 seconds** (01:06:30–01:10:11 UTC).
The supervisor took **171.948 seconds** and the coverage invocation reported
**157.896 seconds**. This is not the three-minute checkpoint.

Shallow checkout took six seconds, Go setup nine seconds, archive restore four
seconds, and applying the in-place archive cache took less than one second.
The coverage trace reported zero compiler commands and 73 linker command
observations, including the failed attempt and recovered retry. These trace
counts are not linker CPU seconds. Capture and save each took four seconds.

One recovered failure was `TestMCPSubagentCustomACPHandlesPermissionRequest`:
its named `@you/subagent` Factory was absent from both the project and the
runner's global catalog. The next fixture creates its own project Factory
through the public application process before starting MCP. Three focused
repetitions passed; the previous failure is retained in this measurement.

### Customer-facing test cleanup entering the checkpoint candidate

The next candidate moves eight Make/coverage-rendering checks from functional
`observability/coverage` into `cmd/functionaltestviz`, retaining their assertions
in tooling unit tests. Five Automations files change their misleading
`root_composition` names to `customer_journeys`, without changing registered
customer tests. Worker-session recovery fixtures now copy the current writer's
journal output rather than constructing legacy snapshots. Public CLI/REST
parity, restart, incomplete follow, unreadable recovery, damaged-recording
isolation, and oversized-payload checks remain. The packaged full-flow failure
journey observes terminal public Work records before asserting cardinality and
lineage, replacing the race with an immediately read projection.

Private Linux full-lane verification of these edits passed with 68 packages,
751 test results (749 pass, two skip), and unchanged coverage policy. Twelve-way
execution samples ranged from **111.92 to 162.52 seconds**, with no retries;
a 24-way experiment took 164.19 seconds and recovered four failures, so it is
not adopted. These are local supervisor samples, not hosted whole-job claims.
Focused repetitions passed for MCP permission (three), current-journal recovery
(three), full-flow terminal observation (five), and tooling checks (three).


The archive restore now precedes the smaller dependency tier: a restored archive
cache is used in place, so restoring another directory that the test job never
reads is redundant. Cold misses retain dependency seeding; main pushes retain
the independent dependency-cache maintenance step. This saves the three-second
redundant restore observed in run 37397370508 without skipping fresh tests.

A private cold-cache quarantine compilation experiment is rejected. Native
verification passed in 162.14 seconds; adding the full coverage flags to its
registration/outcome commands also passed but took 247.79 seconds. Compiler
counts fell from 1530 to 1324, while compiler CPU rose from 194.254 to 241.088
seconds, coverage-tool calls rose from 553 to 1157, and linker CPU rose from
48.476 to 87.494 seconds. Aggregate job user/system time rose from 474.77/92.29
to 705.96/175.34 seconds. Host variance and a brief overlapping scoped lint run
limit wall-time attribution, but the experiment does not justify adopting
coverage-instrumented quarantine checks. Both retained the same coverage gates.


### Owned fixtures: changed-source hosted measurement

Head `0e227f2485c866234b2441cbac369e1bb4535c26`, run `37398418294`,
functional job `112060950738`, passed all 68 packages and 751 results
(749 pass, two skip) with no recovered failures. The complete job took
**231 seconds** (01:22:24–01:26:15 UTC); the supervisor took **186.484 seconds**,
and coverage invocation **175.850 seconds**. The restored archive came from
`4492b4921d`; changed test sources required 23 compiler commands and 36 linker
command observations. This is another valid above-target sample, not the
three-minute checkpoint. The isolated MCP Factory fixture resolved the prior
failure in this run.

On exact archive hits, the next workflow skips filtering a snapshot that cannot
be saved again under GitHub's immutable cache key. Fresh tests still execute;
cold or fallback hits still capture and save the bounded archive. This removes
otherwise redundant post-test copying on same-head verification.


### Parallel execution of isolated customer scenarios

Nineteen previously serial top-level scenarios now call `t.Parallel`: ten
initialization scenarios, three JSON/YAML failure journeys, three CLI startup
journeys, and three JavaScript restart/resume journeys. Each owns its temporary
home/project, process, buffers, and external-effect mocks. Steps inside each
restart/recovery scenario remain sequential; no process-wide environment or
working directory is mutated. Registered test names and assertions are retained.

The first sixteen-scenario full-lane sample passed in **117.96 seconds**, with
751 results and a **55.980-second** customer-journeys package. Adding the three
JavaScript scenarios passed in **121.92 seconds**, with **115.960 seconds**
coverage invocation and a **54.948-second** customer-journeys package. Its fresh
tool trace recorded 39 links, **54.524 seconds linker CPU**, **57.593 seconds
active linker wall intervals**, and **159.173 seconds summed linker wall**.
Summed overlapping wall intervals are not elapsed job time. Aggregate job
user/system times were 329.91/77.72 seconds. Three focused JavaScript repetitions
also passed. Existing private twelve-way cleanup samples ranged from 111.92 to
162.52 seconds, so this experiment establishes a lower package serial tail,
not a consistent whole-lane speedup. Hosted whole-job measurement is still needed.

A preliminary private edit accidentally added `t.Parallel` to a test that already
called it; that full run failed and is excluded from successful latency evidence.
An interrupted repeat and a stale overlay lock also produced no valid latency
sample. The duplicate call was removed before the successful full-lane checks.
The final timing directory uses fresh tool records; earlier repeated harness
output directories contained nested stale report copies and are not used for
compiler/link totals.

For timer-only asynchronous components, `testing/synctest` remains appropriate.
Real file, pipe, and network I/O—including loopback HTTP—is not durably blocking
inside a synctest bubble. Accordingly, these public REST/MCP recovery scenarios
use observable completion and controlled clocks/edges rather than shortening
wall-clock sleeps or wrapping real sockets in virtual time. See the Go team's
[Testing Time](https://go.dev/blog/testing-time) explanation of these limits.
