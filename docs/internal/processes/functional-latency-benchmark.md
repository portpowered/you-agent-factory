# Functional latency benchmark protocol

This protocol measures the functional hotspots owned by
`LTV-FUN-02-runtime-cli-events`. It is intentionally package-scoped so a
future before/after run cannot hide a regression in one named group behind a
faster unrelated lane.

## Scope

Run these seven package commands in this order for every sequential sample:

| Group | Package |
| --- | --- |
| Automations | `./tests/functional/automations` |
| CLI factory-run output | `./tests/functional/cli/factory_run/output` |
| CLI MCP resume | `./tests/functional/cli/mcp_resume` |
| CLI root discovery | `./tests/functional/cli/root_discovery` |
| CLI session resume | `./tests/functional/cli/session_resume` |
| Factory Events | `./tests/functional/events/factory_events` |
| Response Events | `./tests/functional/events/response_events` |

The package paths and test selection must stay unchanged between the before
and after measurements.

## Sequential protocol

Run from the repository root on a clean worktree. Record the UTC timestamp,
commit, OS, Go version, processor count, and any other concurrent build or
test workload. Do not run another repository-wide Go command during a sample.

`-count=1` disables the Go test result cache. `go clean -testcache` is run at
the start of each sample as an explicit guard against an accidental cached
result; it does not clear the build cache. The repository functional lane
uses `-short`, so the benchmark uses the same mode and the repository's
standard five-minute timeout:

```powershell
$packages = [ordered]@{
  "automations" = "./tests/functional/automations"
  "cli-factory-run-output" = "./tests/functional/cli/factory_run/output"
  "cli-mcp-resume" = "./tests/functional/cli/mcp_resume"
  "cli-root-discovery" = "./tests/functional/cli/root_discovery"
  "cli-session-resume" = "./tests/functional/cli/session_resume"
  "factory-events" = "./tests/functional/events/factory_events"
  "response-events" = "./tests/functional/events/response_events"
}

for ($sample = 1; $sample -le 3; $sample++) {
  go clean -testcache
  $rows = foreach ($entry in $packages.GetEnumerator()) {
    $timer = [Diagnostics.Stopwatch]::StartNew()
    & go test -short $entry.Value -count=1 -timeout=5m
    $exitCode = $LASTEXITCODE
    $timer.Stop()
    [pscustomobject]@{
      Sample = $sample
      Group = $entry.Key
      ExitCode = $exitCode
      WallSeconds = [math]::Round($timer.Elapsed.TotalSeconds, 3)
    }
    if ($exitCode -ne 0) { throw "failed: $($entry.Value)" }
  }
  $rows | Format-Table -AutoSize
  "CombinedWallSeconds=$([math]::Round((($rows | Measure-Object WallSeconds -Sum).Sum), 3))"
}
```

Record both the outer wall time and the `ok` duration printed by `go test`.
The difference is command/build/process overhead and is useful evidence when
the test body itself is not the dominant phase. Report the median for each
group and the median of the seven per-sample combined sums. Do not substitute
the fastest sample for a median, and do not compare a sequential sum with a
concurrent maximum as if they were the same metric.

## Concurrent isolation diagnostic

After the sequential samples, launch the same seven commands concurrently as
independent child processes. Keep each process's stdout/stderr in a unique
temporary file, wait for every process, collect every exit code and wall time,
then remove only that diagnostic directory after all children have exited.
The diagnostic is a pass only when all seven exit codes are zero and the
parent has no benchmark-owned child process left. Inspect the output for file
or executable locks, port collisions, cross-test directories, session/event
identity leaks, and cleanup failures. A successful concurrent run is
isolation evidence; it is not a replacement for the sequential median.

Before declaring cleanup clean, inspect process state for commands launched by
the benchmark and distinguish them from unrelated processes already running
on the machine. Never terminate a process that the benchmark did not launch.

## Baseline captured for LTV-FUN-02-runtime-cli-events

The initial Windows/amd64 measurement used Go 1.25.0 on Windows 11, with
24 logical processors. All seven packages passed in each sequential sample
and in three concurrent diagnostics. The sequential sample commands used the
same `-short -count=1` package selection; the first measurement pass used a
ten-minute safety timeout, with no test approaching that limit. The five-minute
form above is the canonical protocol for future evidence.

Captured on 2026-08-01 (UTC) at commit `08ae95c0e`.

### Sequential wall seconds

| Group | Sample 1 | Sample 2 | Sample 3 | Median |
| --- | ---: | ---: | ---: | ---: |
| Automations | 38.520 | 5.640 | 7.990 | 7.990 |
| CLI factory-run output | 15.790 | 6.160 | 24.460 | 15.790 |
| CLI MCP resume | 19.910 | 5.460 | 6.200 | 6.200 |
| CLI root discovery | 23.040 | 7.330 | 9.220 | 9.220 |
| CLI session resume | 5.690 | 6.610 | 5.940 | 5.940 |
| Factory Events | 8.520 | 8.680 | 9.500 | 8.680 |
| Response Events | 5.760 | 6.320 | 5.530 | 5.760 |
| **Combined sequential wall time** | **117.218** | **46.191** | **68.850** | **68.850** |

The package-median sum is 68.120 seconds; the reported combined median is the
median of the three per-sample sums (68.850 seconds), as required by the
protocol. The first sample also showed the largest gap between outer wall time
and the `go test` `ok` duration: Automations was 38.520 seconds wall versus
15.082 seconds of test execution, and root discovery was 23.040 seconds wall
versus 17.363 seconds of test execution. This attributes a material part of
the variance to command/build/process setup and host load rather than to a
single shared production lifecycle phase.

### Concurrent wall seconds

The three concurrent diagnostics passed all seven packages. Their maximum
per-package wall times were 16.210, 20.930, and 37.200 seconds respectively;
no file-lock, port, environment, session, subprocess, recording, cursor, or
event-state collision was observed in these runs. The benchmark-owned Go
processes exited after each diagnostic. A separate repository-wide Go
verification workload was active during the later measurements, so its load
is recorded as a caveat rather than treated as product latency.

These numbers are a baseline for later stories, not a performance claim. Any
after measurement must repeat this protocol on the same machine with the same
package order and report the same per-group and combined statistics.

## Automations after story 002

The automation lifecycle proofs now run as independent Go parallel test cells.
Each cell still constructs its own root process and owns its Factory directory,
environment, HTTP server, clock, watcher or poller, command edge, and
observation channel. The two reconciliation proofs no longer construct and
discard a second process before creating their explicitly invoked Automations
Root; their dedicated inert proof remains separate.

Three uncached samples were captured on 2026-08-01 (UTC) on the same Windows
11/amd64 machine, with Go 1.25.0 and the same `-short -count=1` command used by
the baseline. `go clean -testcache` ran before each sample. The outer wall time
includes command/build setup; the `ok` duration is the Go test execution time.

| Sample | Started (UTC) | Exit code | Outer wall seconds | Go `ok` seconds |
| ---: | --- | ---: | ---: | ---: |
| 1 | 2026-08-01T09:50:37.692Z | 0 | 3.240 | 0.915 |
| 2 | 2026-08-01T09:50:41.115Z | 0 | 3.560 | 0.950 |
| 3 | 2026-08-01T09:50:44.979Z | 0 | 5.250 | 1.125 |
| **Median** |  |  | **3.560** | **0.950** |

Against the baseline Automations median of 7.990 outer wall seconds, the
post-change median is 55.4% lower. The prior serialized local run was 24.666
seconds; it is retained as a host-local diagnostic only because it was not a
three-sample protocol run. All three post-change samples passed every
automation test.

## CLI output and root discovery after story 003

The factory-run output and root-discovery cells now schedule independent
top-level and table-driven subtests in parallel. Each cell still constructs a
fresh root-built Process, owns its temporary HOME/USERPROFILE, working
directory, captured streams, Factory files, injected server/provider edges,
and cancellation lifecycle. No process, writable root, environment map,
stream, listener, or runtime/session state is shared between cells.

Three uncached samples were captured on 2026-08-01 (UTC) on the same Windows
11/amd64 machine, with Go 1.25.0 and the same `-short -count=1` mode. The
sample selected the two unchanged package paths from the protocol in the same
order, and `go clean -testcache` ran before each sample. The outer wall time
includes command/build setup; the `ok` duration is Go test execution time.

| Sample | CLI factory-run output wall | CLI factory-run output `ok` | CLI root discovery wall | CLI root discovery `ok` |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 10.70 | 6.05 | 12.41 | 4.42 |
| 2 | 7.81 | 2.49 | 4.37 | 1.34 |
| 3 | 4.02 | 1.32 | 8.16 | 3.67 |
| **Median** | **7.807** | **2.494** | **8.162** | **3.674** |

The two-package outer-wall combined sums were 23.103, 12.180, and 12.181
seconds, with a 12.181-second median. Against the same two package groups'
baseline sample sums of 38.830, 13.490, and 33.680 seconds (33.680-second
median), the combined median is 63.8% lower. The package-median sum is 15.969
seconds versus the 25.010-second baseline package-median sum, a 36.1% lower
measurement. Factory-run output's median outer wall time is 50.5% lower;
root discovery's median outer wall time is 11.5% lower, while its test-body
median is 78.8% lower than the only baseline body-duration observation. The
remaining outer-time variance is command/build setup rather than a shared
runtime cache.

Three concurrent two-package diagnostics also passed. Each wave launched the
same two uncached package commands as separate child processes with unique
stdout/stderr files; all six exit codes were zero and every launched process
was joined before the diagnostic directory was removed. Maximum per-wave wall
times were 7.786, 9.498, and 7.436 seconds. No executable or file-lock,
working-directory, environment, port, child-process, or assertion collision
was observed.
