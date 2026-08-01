# Functional performance benchmark process

Use this process when measuring a latency-sensitive functional package on a
developer workstation. The result must distinguish a fresh test invocation,
compiler-cache effects, package execution, and a deliberately contended run.

## Independent samples

Run each package separately from the same revision with:

```powershell
go test -short ./tests/functional/<package> -count=1 -timeout=5m
```

Wrap the command with a wall-clock stopwatch and retain every sample's exit
code. `-count=1` disables the Go test-result cache; it does not flush the Go
build cache. If the first sample pays compilation or other one-time setup,
keep it in the evidence and label the effect instead of silently dropping it.
Report at least three samples, each package median, and the combined sum of
the package medians. Do not use a contended run as the performance baseline.

## Phase attribution

Use `go test -json` to identify the slowest observable test cases. For
owner-level attribution, collect a CPU profile with an explicitly expanded
workspace path and inspect cumulative callers:

```powershell
$profilePath = Join-Path $PWD 'functional-package-cpu.prof'
go test -short ./tests/functional/<package> -count=1 -timeout=5m `
  ('-cpuprofile={0}' -f $profilePath)
go tool pprof -top -cum -nodecount=25 $profilePath
```

On Windows PowerShell, format the `-cpuprofile` argument as shown so the
absolute path is expanded before it reaches the Go test binary. Profile runs
are diagnostic runs, not samples for the timing median, because profiling
changes runtime cost.

## Contention diagnostic

`cmd/functionallane` accepts one `-root` package pattern. To target several
specific packages concurrently, start one functionallane process per package
with isolated stdout/stderr log files, wait for all processes, record each
exit code and package-reported duration, and clean the temporary log directory
in a `try/finally` block. Verify that no package test or runner subprocesses
remain. Record filesystem, build-cache, port, environment, or canonical-state
collisions, but keep this result separate from the independent baseline.

Always record the UTC timestamp, revision, operating system, architecture,
logical processor count, Go version, `GOMOD`, `GOCACHE`, exact commands, and
whether the run was independent, diagnostic, or contended.

## Scenario setup reuse

When one functional scenario needs setup commands followed by a hosted server,
construct one process through `root.BuildProcess` and reuse it for the sequential
public `Process.Execute` calls. Share only immutable fixture payloads, returning
copies to callers. Keep each scenario's Factory roots, HOME/USERPROFILE,
working directory, streams, external edges, and process cleanup isolated; do
not share a process or mutable canonical state across scenarios. A server
harness may expose a setup callback that runs after its invocation-local
environment is prepared and before the daemon command starts.

When a reused setup command reads a fixture, pass the server invocation's
environment explicitly through the process-input helper. The process wiring is
reusable; environment values, working directories, captured streams, and
durable Factory state are not.
