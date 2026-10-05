# Consolidated Go unit execution evidence

The prepared Go unit suite meets the requested subminute aggregate CPU target
on this Windows/amd64 host. The final compact run passed all 393 selected unit
packages in **33.219 user-plus-kernel CPU seconds**, including the coordinator
and every helper descendant. Wall time was **56.029 seconds**. Preparation is
separate and is not included in that result. This is one observed sample, not
a portable performance guarantee or a cold-build result.

The runner preserves 389 package test groups in one shared executable and four
native executables. Production package ownership, Go internal visibility,
package working directories, parallel tests, helper selection, cleanup, and
default `TestMain` leak verification remain enforced. Unsupported setup and
incompatible test imports retain native execution. Failure, deliberate leaks,
and an early successful process exit with an incomplete inventory all fail
the lane; normal helper-process working-directory checks pass.

## Commands and source freshness

```text
make test-unit-monolith-prepare UNIT_DEFAULT_JOBS=2
make test-unit-monolith-prebuilt
```

Preparation builds the coordinator, discovers the current corpus, creates a
build overlay, and uses native `go test -c` to produce all five executables.
The execution target runs this explicit prepared source snapshot fresh with
`-test.count=1`. It does not rediscover sources, regenerate overlays, invoke
Go build tools, or check source freshness. Re-run preparation after changing
code, tests, assets, dependencies, toolchain, target platform, build tags, or
instrumentation. The runner records the preparation identity rather than
describing an old binary as current source.

For complete subtest reporting, use the prepared coordinator with
`-monolith-prebuilt -monolith-details -count=1`. Detailed reporting retains
`go tool test2json`; compact reporting emits merged top-level test completion
and native package completion. Both execute the same registered cases.
`UNIT_TIMING_OUTPUT` saves the compact timing document from the Make target.
Set `UNIT_MONOLITH_GROUP_CPU=1` for package process-CPU diagnostics on Windows.

`make test-unit-monolith` still performs preparation/validation and execution
together. Ordinary `make test` retains native Go source validation and test
caching. Full instrumented monolith coverage has now been compared; the
consolidated path remains opt-in while cold builds and representative
changed-source rebuilds are evaluated. Prepared execution is available now;
those rebuild gates have not been declared complete.

## Passing execution measurements

All samples below execute fresh tests, including required subprocess helpers.
They use Windows Job Object accounting of the coordinator and descendants;
the external measurement script is excluded. Other Go work was active on the
host, so wall-time and CPU variation should be retained in comparisons.

| Prepared scope | Reporting | Packages | CPU seconds | Wall seconds | Processes |
| --- | --- | ---: | ---: | ---: | ---: |
| Before service-local wiring exclusion | Compact | 445 | 40.141 | 80.055 | 47 |
| Before service-local wiring exclusion | Detailed | 445 | 50.453 | 68.158 | 77 |
| Unit lane before Goal component isolation | Compact | 393 | 33.219 | 56.029 | 46 |
| Unit lane before Goal component isolation | Detailed | 393 | 47.359 | 64.059 | 76 |
| Goal component isolation | Compact | 393 | 38.406 | 65.184 | 46 |
| Goal component isolation | Detailed | 393 | 50.156 | 79.147 | 76 |
| Lint-corrected snapshot | Compact, CPU attribution enabled | 393 | 39.984 | 66.153 | 46 |
| Final snapshot with deterministic shutdown/cancellation checks | Detailed | 393 | 42.234 | 62.111 | 76 |

Earlier passing complete prepare-and-execute measurements were approximately
74–95 CPU seconds on the preceding 447-package scope. They are separate from
execution-only results and do not establish a subminute build pipeline.
The original warm native pipeline was approximately 312 CPU seconds, but it
included discovery/build work and tests now assigned to other lanes; it is
not an identical-scope comparison against the final prepared execution.

The first prepared attempt failed after five minutes because an existing
Claude deadline test started a 50 ms timeout before its mock effect entered,
then waited indefinitely for startup. Its failed 12.547 CPU-second sample is
excluded from passing results. Claude and Codex now use explicitly signaled
deadline termination after effect startup and bounded observation waits.
Both packages passed 20 consecutive native runs. A separate earlier failed
profiling attempt hit a loaded-host gRPC readiness/deadline race and is also
excluded from passing measurements.

Machine-readable sample accounting and all 393 package CPU observations are
in [monolith-cpu-evidence.json](monolith-cpu-evidence.json). Raw timing,
coverage, and diagnostic files from this experiment remain in the local
temporary `you-unit-audit-20261004` directory.

## Coverage and reductions

The capture before Goal component isolation contains **18,890 unit test/subtest entries**.
No retained entry disappeared when switching to prepared execution or
excluding wiring. All **1,169 wiring entries in 54 packages** remain runnable
through `make test-wiring-integration`, which `make test-integration` includes.
Those are legacy private behavior/composition witnesses pending owner
migration; reclassification does not make them valid public functional tests
or make Wire's remaining behavioral production code inert. New application
functional tests use `root.BuildProcess`.
Coverage tooling separates production measurement from unit-test selection:
functional profiles still instrument Wire, while unit invocations and unit
floor enforcement exclude its entire tree. The coverage-checker maintenance
suite verifies that distinction and passes with the updated lane classifier.

Native production coverage comparisons across ten changed fixture owners cover
**11,481 statements** and lose **zero previously covered production blocks**:
installer, Factory Sessions execution, ASR codecs, definition catalog,
visualization metrics service and CLI, response presentation, Claude, and
Codex, and Packaged Goal's combined unit/contract tests. Installer coverage remains 509/616 statements (82.6%), with all 469
production coverage blocks identical. Three metrics attribution branches
formerly covered only by scale tests now have tiny component assertions.
The full instrumented comparison spans the same **117,368 production blocks**
in native and shared-binary profiles. Native execution covers 116,646 statements;
the consolidated suite plus five fresh focused repetitions covers 116,658.
One native-only statement remains: the early return after an interrupted terminal
subscription drain (`event_history_dispatch_lifecycle.go:107`). This is a
pre-existing select-order-dependent branch; its tests remain registered and
passing. This is roughly equivalent coverage, not an assertion of identical
single-run counters. The five focused repetitions cover process monitoring,
engine cancellation, TTS parameter mapping, event subscriptions, and Worker
Session replay completion races. Tiny controlled cancellation and subscription
handoff tests remove four other ordering-dependent gaps. No production code
was changed.

Instrumentation used a temporary physical copy of the overlay sources because
Go 1.25's coverage tool could not read virtual-only overlay files. Both paths
instrumented `./pkg/...`; comparison excludes generated registration, copied
test files, and every file absent from the original production source tree.
Passing helper-process counters and the four native exceptions are included.
Instrumentation/build time is excluded from prepared execution measurements.
An initial instrumented helper run failed because missing `GOCOVERDIR` warnings
changed protocol stderr; the passing run explicitly sets it. One native package
was recompiled after an import edit during compilation. An unsupported in-process
`-count=10` probe failed leak checks; its profiles are excluded. All five fresh
focused repetitions passed. Leak assertions and the runner's fresh-only rule
remain intact.

Changes retain the component behavior with smaller inputs:

- Replay/config and related projection/resolution test directories were
  consolidated, retaining their case inventories and scoped implementation
  coverage.
- Installer units use opaque tiny payloads and a controlled persistence stub.
  Published-factory matrices and redundant Wire installation/catalog checks
  were removed; public install, validation, preservation, and replacement
  behavior is covered by the named-lifecycle functional scenario.
- Retry snapshot units use the 32/33 retention boundary; large 10/100/1000
  histories remain in the stress lane.
- Warning units use 4/8 KiB limits plus pure production-threshold checks;
  original 64/128 MiB cases remain stress tests.
- Terminal compaction units use one/two short records; 500/1000 large terminal
  lifecycles remain stress tests.
- ASR units use short transcripts and three ordered segments; the 16 MiB and
  million-segment cases remain stress tests.
- Metrics units retain small selection/attribution inputs; 500/2000-artifact,
  streaming reachability, and partitioned CLI scale cases remain stress tests.
- Response-presentation producers use bounded attempts and an observed backlog
  barrier instead of spinning for elapsed time. Late enqueue rejection,
  ordering, drops, drain, and finalization assertions remain. Twenty native
  repetitions and identical production coverage passed.

`make test-stress-fixtures` runs the retained snapshot, ASR, and metrics
workloads. All three stress packages and the replacement packaged-installation
functional package passed. Focused layering, test-lane, test-boundary, and
test-sleep analyzers passed; stale debt entries for removed checks were deleted.

## Remaining package costs and next reductions

The final snapshot reduces Packaged Goal's attributed CPU from 2.281 seconds
to below Windows process-time resolution in this run. Its units no longer
construct Factory Definitions or materialize published factories. Tiny prompt
selection/path/error tests replace that fixture. Published prompt conformance
runs under `distribution/internal/contracttests/goal`; the public CLI functional
scenario proves editable prompt layout and authored prompt bytes after fresh
installation and replacement in all three supported formats. Combined Goal
coverage increases from 37/55 to 45/55 statements with no covered blocks lost.
The unit-only package floor changes deliberately from 67.27% to 38.18% because
catalog conformance now belongs to the contract lane; combined coverage is
81.82%. Goal component isolation reduces the detailed unit inventory to 18,887
entries; that three-entry reduction is confined to its materialization/drift
cohort. Six deterministic cancellation/shutdown entries then bring the final
capture to 18,893. The untouched package case inventories remain identical.

Final parent CPU observations are CLI 5.031 seconds, model assets 3.516,
Providers ACP 2.484, and Recordings internal 2.391. These remain bounded
component/effect costs. Assets' existing artifact bodies are already only a few
dozen bytes; recovery cases deliberately corrupt or remove independent mutable
caches. Sharing those caches would weaken isolation. ACP subprocess cases retain
pipe, termination, and working-directory proof. Further conversion of composed
CLI cases and durable-storage scenarios is separate work; the complete prepared
suite already meets the aggregate CPU target. The table below retains the
earlier attribution sample rather than claiming measurements are identical.

These values attribute parent-process CPU, including user and kernel time.
Merged groups use process-time deltas; native exceptions use direct process
CPU. Helper descendants and initialization outside group boundaries are not
attributed to individual packages. Aggregate process-tree CPU, not the sum of
this table, remains the budget metric. Windows accounting quantization limits
precision for very small groups.

| Package area | Attributed CPU seconds | Best next action |
| --- | ---: | --- |
| `pkg/transports/cli` | 4.844 | Separate actual command/component behavior from composed application and inventory scenarios; preserve its isolated home/setup until a verified merge can replace the native binary. |
| Models assets `internal/service` | 3.375 | Reduce repeated filesystem/cache reconstruction setup; share immutable tiny source inputs while keeping mutable caches isolated. Move cross-instance durability scenarios to integration/functional coverage. Its existing model bytes are already tiny. |
| Packaged Goal distribution | 2.281 | Replace published-factory materialization unit fixtures with synthetic component inputs; keep editable layout/materialization behavior in public functional tests and prompt/catalog conformance in contract checks. |
| Providers ACP `internal/service` | 2.094 | Consolidate repeated protocol scenarios around a controlled in-memory effect where process behavior is incidental. Preserve subprocess cases proving pipe, process, teardown, and working-directory behavior. |
| Recordings `internal` | 1.828 | Reuse immutable replay inputs and reduce repeated temporary storage setup; retain ordering, cancellation, replay isolation, and durability boundaries. |
| Factory Definitions `definition` | 1.219 | Consolidate redundant representation fixtures around one minimal valid definition with focused mutations. |
| Factory-config OpenAPI tests | 1.156 | Keep schema/contract inventories in the contract lane and isolate normalization behavior with minimal payloads. |

Repeated package linking is absent from the prepared hot path. Further gains
come primarily from fixture/effect work and reporting, rather than additional
production package merging. The runner still coordinates all selected groups,
native exceptions, failures, and cleanup; removing it would not preserve those
guarantees.

## Hosted coverage lane reconciliation

Hosted run `37293075863` passed all 391 selected unit packages and all
functional and integration scenarios. The unit coverage gate then rejected
two obsolete Wire requirements. Removing those requirements exposed 23
incidental service-floor drops from the removed composition/materialization
fixtures. No implementation or retained assertion was removed to repair the
gate.

The same-head unit/functional production-block union meets the prior unit
floor for 16 affected packages. Their unit floors now reflect the component
lane; their broader guarantees remain in the passing functional lane. This
includes two assembly/forwarding owners with zero component coverage and
100% functional coverage. Existing remediation holds and unrelated floors
remain unchanged. The package measurements and exact prior/reconciled floors
are recorded in `coverage-lane-reconciliation.json`; this is a deliberate
scope reconciliation, not a claim that functional coverage counts as unit
coverage or that all production statements are covered.

Seven areas keep their prior floors and receive tiny component tests:
mock dispatch selection, rejection, script effects and input selectors;
detached CLI parse/capture observations; invalid empty artifact summaries;
inference materialization failure/cancellation cleanup and projection
fallbacks; and pure default-work-type, work-propagation, and Quorum-lineage
policies with tiny synthetic definitions. Five fresh focused coverage processes
passed. Combined with the
captured hosted unit profile, coverage is 157/211 statements for artifact
export, 376/404 for inference (within the existing 0.25-point tolerance),
75/93 for the mock runner, and 79/88 for CLI observation. No application graph,
published factory, actual script process, or elapsed-time wait is needed.

The final prepared snapshot passes all 397 packages and 18,926 test/subtest
entries, using 33.265625 aggregate user-plus-kernel CPU seconds and 59.778445
elapsed seconds with full detailed reporting. The 33 additional entries belong
to the seven component owners above. Preparation/build costs remain separate;
the earlier samples are historical measurements of their stated snapshots.
