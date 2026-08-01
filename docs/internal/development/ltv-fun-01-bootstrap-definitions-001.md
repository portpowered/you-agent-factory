# LTV-FUN-01-bootstrap-definitions-001 baseline and contention evidence

Status: baseline and profile evidence captured

Captured: 2026-08-01 UTC

Revision: `08ae95c0e141a71a913c73fe64aab13476146ad4`

Branch: `LTV-FUN-01-bootstrap-definitions`

This report covers Story 001 only. It establishes the pre-change evidence for
the three owner-coherent functional packages; it does not claim a post-change
improvement.

## Machine and toolchain

- OS: Windows 11 Home, build `26200`, 64-bit
- Machine: MSI `MS-7D91`, 63.7 GB RAM, 24 logical processors
- Go: `go1.25.0 windows/amd64`
- Go toolchain: `auto`
- Module: `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\LTV-FUN-01-bootstrap-definitions\go.mod`
- Go build cache: `C:\Users\andre\AppData\Local\go-build`
- Measurement timestamp normalization: UTC

## Independent benchmark protocol

Each sample ran sequentially from the unchanged revision with this command,
using a PowerShell stopwatch around the process:

```powershell
go test -short ./tests/functional/<package> -count=1 -timeout=5m
```

The `-count=1` setting forces a fresh test invocation and bypasses the Go test
result cache. The Go build cache was left intact so the samples represent the
normal developer feedback loop and expose the first-invocation build effect.
All samples passed with exit code 0. `go`'s package duration is included to
show test execution separately from the wrapper's wall time.

| Package | Sample 1 wall / Go | Sample 2 wall / Go | Sample 3 wall / Go | Wall median |
| --- | ---: | ---: | ---: | ---: |
| `bootstrap_portability` | 61.424s / 42.571s | 41.780s / 33.549s | 11.517s / 9.363s | **41.780s** |
| `factory/current` | 14.808s / 11.939s | 14.730s / 11.778s | 34.744s / 31.956s | **14.808s** |
| `factory/definitions` | 16.455s / 12.417s | 15.845s / 13.852s | 17.995s / 15.158s | **16.455s** |

The combined independent baseline, calculated as the sum of the three package
medians, is **73.043s**. The first bootstrap sample includes a visible
one-time build/setup effect; the outlying current sample remains in the raw
evidence and was not excluded from the median calculation.

## Phase and owner attribution

The JSON timing runs passed and identified these largest observable test
executions:

| Package | Largest test-level observations |
| --- | --- |
| `bootstrap_portability` | `TestExportImportSmoke_PublicShareImportSurfaceCarriesDetachedStarterWork` 1.900s; `TestExportImportSmoke_PreservesNestedFactoryDocsThroughExportImport` 1.680s; `TestAutomatPortabilityFixture_ExpandedLayoutIsDispatchReadyForBoundedSmoke` 1.520s |
| `factory/current` | `TestProcessFactoryConfigVersionChangesObservableRouting` 7.900s; `TestProcessIndependentFactoryRootsRemainIsolated` 7.870s; `TestProcessRejectionLoopCompletesAfterRetry` 2.560s |
| `factory/definitions` | `TestExportedFactoryCanBeImportedAndRun` 2.170s; `TestFactoryInitFailureRoutingProducesFailedWork` 1.730s; `TestFactoryInitCreatesRunnablePortableScaffold` 1.730s |

The CPU profiles were diagnostic-only runs. Cumulative attribution from
`go tool pprof -top -cum` was:

- `bootstrap_portability`: 30.81s of samples; `Process.Execute` 12.69s;
  `SystemInitialization` 8.91s; Factory Definitions
  `EnsurePackagedFactories`/`InstallPackagedFactory` 8.77s; authored-layout
  persistence 8.76s.
- `factory/current`: 35.68s of samples; `Process.Execute` 16.58s;
  `SystemInitialization` 13.97s; Factory Definitions
  `EnsurePackagedFactories` 13.70s and named-factory persistence 12.67s;
  `os.RemoveAll` 6.28s.
- `factory/definitions`: 38.00s of samples; `Process.Execute` 14.25s;
  `SystemInitialization` 11.33s; Factory Definitions
  `EnsurePackagedFactories`/`InstallPackagedFactory` 11.05s; named-factory
  persistence 9.76s; the `fsnotify` Windows directory watcher 10.04s.

Across the packages, sampled CPU was dominated by Windows syscall/file-system
work (`runtime.cgocall`). The repeated call chain is observable application
execution through `Process.Execute`, followed by System Initialization and
Factory Definitions packaged-factory installation and authored-layout writes.
This is a measured owner-local optimization hypothesis for later stories; no
production behavior was changed in Story 001.

## Concurrent diagnostic

Because `cmd/functionallane` accepts one `-root` value, the targeted diagnostic
started three separate runner processes concurrently. Each used:

```powershell
go run ./cmd/functionallane -jobs=1 -count=1 `
  -root=./tests/functional/bootstrap_portability -short=true -timeout=5m
go run ./cmd/functionallane -jobs=1 -count=1 `
  -root=./tests/functional/factory/current -short=true -timeout=5m
go run ./cmd/functionallane -jobs=1 -count=1 `
  -root=./tests/functional/factory/definitions -short=true -timeout=5m
```

The three commands were launched together with isolated temporary stdout and
stderr logs and cleaned up in a `try/finally` block. Results:

| Runner | Exit | Package-reported duration |
| --- | ---: | ---: |
| `bootstrap_portability` | 0 | 18.656s |
| `factory/current` | 0 | 25.752s |
| `factory/definitions` | 0 | 31.888s |

Total concurrent wall time was **41.836s**. No filesystem/build-cache
collision, port conflict, environment contamination, test failure, or
nondeterministic result was observed, and no targeted runner or test process
remained after cleanup. The current and definitions package durations were
substantially above their independent medians, so the green contended run
rules out a reproducible failure in this sample but confirms resource-contention
variance that must not be used as the performance baseline.

## Reproduction and handoff

The reusable protocol is documented in
[`docs/internal/processes/functional-performance-benchmark.md`](../processes/functional-performance-benchmark.md).
The next story should retain the independent command boundary and the same
machine/toolchain context, then compare at least three post-change samples
against the medians above. It should focus first on repeated packaged-factory
installation and authored-layout persistence during System Initialization,
while preserving the public `root.BuildProcess`/`Process.Execute` functional
proofs.
