# Pinned baseline observer: owner and tool admissibility

This is the story 001 input to FI-PREREQ-BASELINE-OBS. It establishes source
identity and rejects the cached stock Delve path before target attachment.
It does not establish runtime feasibility, complete the story 002 dossier,
release T27, or report P01 measurements. OBS-V/OBS-REVIEW remains independent.

## Authority and limits

The governing amendment is AM10 in [contracts.md](contracts.md), with counted
owners in [inventory.md](inventory.md#p01-counted-resources-at-pinned-baseline).
The inspected source-plan snapshot SHA256 is
`30be9a8fd73e76876def17e319af9bf7ff5356d1f81053454df46446e6f1e4e8`.
These identities identify evidence; they are not admission allowlists.

Product source must remain at `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`.
Only this new runbook and separately admitted external test support are in
scope. No production hooks, new public API, companion changes, daemon restart,
real provider/model service, downloads, paid calls, or performance workload
are authorized. Total owned scratch/artifacts must remain below 2 GiB and the
author lane below 90 minutes (story 001 investigation: at most 25 minutes).
Use cached Go 1.25.0, GOTOOLCHAIN unset/auto, offline module resolution, one
owned target and one observer at most. No target was launched in story 001.

## Reproduce the static investigation

Run from the isolated delivery worktree in PowerShell. Choose a new owned
scratch path outside other worktrees; the recorded path below was absent
before creation. Do not operate on the repository root checkout.

```powershell
$scratch = 'C:/Users/andre/work/portos/fi-obs-pin-20261003-001'
$pin = '95e213cfb35b50236fd7a34ad66c797d2ee7b5b6'
git worktree add --detach $scratch $pin
git -C $scratch rev-parse HEAD
git -C $scratch status --porcelain
go version
go env GOMODCACHE GOTOOLCHAIN
dlv version
gdb --version
go version -m (Get-Command dlv).Source
Get-FileHash -Algorithm SHA256 (Get-Command go).Source
Get-FileHash -Algorithm SHA256 (Get-Command dlv).Source
Get-FileHash -Algorithm SHA256 (Get-Command gdb).Source
$delveSource = 'C:/Users/andre/go/pkg/mod/github.com/go-delve/delve@v1.27.0'
```

Observed 2026-10-03: worktree creation and both Git queries exited 0; HEAD
was the exact pin and status was empty. Version commands exited 0: Go
`go1.25.0 windows/amd64`, GOMODCACHE `C:\Users\andre\go\pkg\mod`,
GOTOOLCHAIN `auto`, Delve `1.27.0`, build
`0782d3511ee64ac561a207d35b3403f49d3744a6`, GDB `16.3`.
Delve build metadata reports Go `1.25.12` and module
`github.com/go-delve/delve v1.27.0` with checksum
`h1:i66Einw/sQhm0hlbjLNUNxrwCmKdTcIqpyHVqSWAbd0=`.
That debugger build version is distinct from the permitted target compiler.

| Binary | Resolved path | SHA256 |
| --- | --- | --- |
| Go bootstrap (installed 1.24.2; auto selects 1.25.0 here) | `C:/Program Files/Go/bin/go.exe` | `01CDA0BA94EFA133F57E4794FC950927176A52338EB9B384EED29C5A1683DBE7` |
| Cached Go 1.25.0 compiler | `C:/Users/andre/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64/bin/go.exe` | `5C08733F77C624DD4B34BC157626656EF2485DE987A8495A4B804CBB91F01726` |
| Delve | `C:/Users/andre/go/bin/dlv.exe` | `0D2BD70C9FE426169AC06A86E5E574615F166075F8CD2BA4D2CEB2EB6775161E` |
| GDB | `C:/ProgramData/mingw64/mingw64/bin/gdb.exe` | `76A2847EC717563C9CFE94EDEDD42BA354C7EB1B63A765F4143B639278C609BD` |

Tool presence proves neither owner access nor synchronization. The cached
Delve source was available at the path above. GDB received only a version and
binary identity check; its initialization, Go decoding and read-only access
are unproved. Do not attach it as a substitute.

## Exact owner and writer provenance at the pin

All paths and line numbers in this section refer to the detached pin, not the
delivery head. Inspect their complete files, including error paths:

```powershell
$hostDir = "$scratch/pkg/services/factory_runtime/internal/services/instance_host/internal/service"
$leaseFile = "$scratch/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service/service.go"
Get-Content "$hostDir/service.go"
Get-Content "$hostDir/execute.go"
Get-Content "$hostDir/replace.go"
Get-Content "$hostDir/terminate.go"
Get-Content "$scratch/pkg/services/factory_runtime/internal/host/lifecycle.go"
Get-Content $leaseFile
rg -n 'handles|Lock|Unlock' $hostDir -g '*.go' -g '!**/*test.go'
rg -n 'leases|capacityHolders|Lock|expire|releaseCapacity' $leaseFile
```

| Observation | Exact storage owner | Writers and protection | Access status |
| --- | --- | --- | --- |
| Runtime handles | `pkg/services/factory_runtime/internal/services/instance_host/internal/service/service.go:21`, `(*Host).handles`, `map[string]*factoryhost.Handle` | `execute.go:56` registers; `execute.go:89-103` clears matching identity; `replace.go:82-97` commits replacement. Host `mu` protects registry reads/writes. WaitForStart failure removes through `removeHandle`; completion alone is not registry removal. | BLOCKED: no admitted external owner-pointer or complete map read. Session projections cannot replace it. |
| Models lease records | `pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service/service.go:21`, `(*service).leases`, `map[string]leaseRecord` | Acquire inserts; Claim updates `claimed`; release/revoke/expiry update status under `mu`. Each record contains `lease` and `claimed`. | BLOCKED: no admitted external read of all retained records or composition provenance. |
| Capacity holders | Same leases `service.go:22`, `(*service).capacityHolders`, `map[string]int` | Acquire increments; release/revoke/expiry decrement or delete via `releaseCapacityCountLocked`, under the same `mu`. | BLOCKED: complete per-slot integer values and their sum unproved. Number of keys is not total holders. |
| Target goroutines | Go runtime `runtime.allgs` and each `g` status in the target artifact; cached Go 1.25.0 `src/runtime/proc.go:661-683` documents `allgs`, `allglen`, `allgptr` | Scheduler and goroutine creation/exit change the population; `allgs` includes dead goroutines. Enumeration must classify statuses and use the target runtime layout. | BLOCKED: no artifact, all-thread snapshot, complete live enumeration or owned/peer attribution. Shell process counts and observer goroutines are not evidence. |

The host `Stop` clears registration before `LifecycleService.Stop` joins the
run. An empty handles map alone therefore cannot prove joined cleanup.
`host/lifecycle.go:211-226` stops/joins sidecars, cancels the run, calls
`handle.Wait`, then finalizes artifacts. Observe successful return and any
cleanup errors separately from registry emptiness. Replacement publication
and registry commit are distinct steps; no snapshot may straddle them.

Lease release retains the record with RELEASED status. Expiry retains EXPIRED
records. Do not reinterpret their continued presence as active capacity or
silently demand zero records. `GetModelLease` calls
`expireStaleLeasesLocked`; even a getter can mutate lease state and notify the
coordinator. Never invoke getters, arbitrary functions, assignments, or
injected calls through an observer.

Pinned source SHA256 values (relative paths below use the same prefixes as the
owner table):

| Source | SHA256 |
| --- | --- |
| Instance host `internal/service/service.go` | `EB6146082F8BD136A60A68527475EBC247A21C6EE125ABBEA8A972446CDDA61E` |
| Instance host `internal/service/execute.go` | `5A26563D5F554DC086837EFC7F4E84D2FE4ABE97D34E2C4BA01AC82AAFE58C67` |
| Instance host `internal/service/replace.go` | `D64172196E6C3BE02F9178344FAAF260DAC890BE4D5412B2304F3391A8F86FDA` |
| Instance host `internal/service/terminate.go` | `7B3134E73D5B8BBE772C64BFE6AFED55ABCA48814AF40D8C7D1A08D699959048` |
| Runtime `internal/host/lifecycle.go` | `B0F78CC22E6E4DAB224DBC8B47A8B0A5F0309AC0F672DF4647D1A5267EF8168F` |
| Models leases `internal/service/service.go` | `2ABBE4F500FBCB40BD02CE4223045ED365BD04BBD29C2F16D9A963F77D335B46` |
| Cached Go 1.25.0 `src/runtime/proc.go` | `6080C3885B996C1BF10C215E8E84633E13BE1C00466D42A12DC182C024318DE4` |

## Checkpoint contract for a future admitted fixture

These are required checkpoints, not executed observations:

- Q0: after public `root.BuildProcess` construction and owned initialization,
  before the owned lifecycle sequence; record all relevant parent/scoped owner
  identities and the exact target process.
- Q1: after an owned explicit Factory Session opens and replacement completes,
  after controlled work/lease activity has settled at explicit signal gates.
  Record current/replaced generation attribution and every remaining writer.
- Q2: after owned close returns and sidecar/run/attempt cleanup joins succeed;
  record retained lease statuses and residual scoped entries without altering
  retention policy.

At each checkpoint, block new fixture work, prove no mutation is in flight,
then obtain an all-thread suspended coherent snapshot through an admitted
nonmutating OS/tool path. A stopped thread may be halfway through a protected
map update; suspension alone is insufficient. Do not acquire target locks by
calling target functions. Read all four owners in that same snapshot, including
all Models owner instances associated with the fixture's scopes. Prove pointer
provenance through pinned composition; an isolated component fake cannot
certify the composed observer. Resume only through the admitted procedure.

Require complete map paging and decoding, readable-empty versus unavailable
distinction, every holder integer and total, and a feasible representative slot
value greater than one. A missing owner, partial map, ambiguous pointer or
unjoined writer is BLOCKED, never zero. The composed nonzero holder witness
without a real model service remains unproved and belongs to story 002.

## Cached Delve command-path audit: rejected before attachment

The following source inspection commands exited 0 with the matches described
below. These commands read local files; they never initialize a target:

```powershell
rg -n 'initialize|DisableAsyncPreempt|writeSoftwareBreakpoint' "$delveSource/pkg/proc/native/proc.go"
rg -n 'initialize' "$delveSource/pkg/proc/native/proc_windows.go"
rg -n 'createUnrecovered|createFatal|createPlugin|setAsyncPreemptOff|setValue' "$delveSource/pkg/proc/target.go"
rg -n 'SetBreakpoint|WriteBreakpoint' "$delveSource/pkg/proc/breakpoints.go"
rg -n 'WriteMemory|WriteProcessMemory' "$delveSource/pkg/proc/native/threads_windows.go"
rg -n 'setValue|writeUint' "$delveSource/pkg/proc/eval.go"
rg -n 'MaxArrayValues|MaxVariableRecurse|loadMap' "$delveSource/pkg/proc/variables.go"
```

Windows launch (`native/proc_windows.go:66`) and attach (`:181`) call
`nativeProcess.initialize`. In `native/proc.go:351-380` it creates the target
group with `DisableAsyncPreempt` true on Windows. `newTarget` in
`target.go:225-227` automatically creates panic, fatal-throw and plugin-open
breakpoints. Panic/fatal helpers call `SetBreakpoint` (`target.go:400,413`),
which reaches `t.proc.WriteBreakpoint` (`breakpoints.go:879`), native
`writeSoftwareBreakpoint` (`native/proc.go:433-434`), thread `WriteMemory`
and Windows `_WriteProcessMemory` (`native/threads_windows.go:106-114`).

`target.go:233-234` also calls `setAsyncPreemptOff(t, 1)`;
`:374-387` finds `runtime.debug.asyncpreemptoff`, loads its value and invokes
`scope.setValue`. Integer assignment reaches `writeUint` in `eval.go:645`.
Thus avoiding typed user assignments or user breakpoints does not avoid
automatic writes. Clearing breakpoints or detaching later cannot retroactively
make this a nonmutating observation. Stock Windows native launch/attach is
REJECTED; no product target was attached to test the prediction.

Function injection and target assignment paths are forbidden. A debugger core
reader or another backend is not admitted by this audit: snapshot creation,
complete owner recovery and nonmutation would require their own proof. The
default variable load uses a 64-element limit and recursion depth one
(`variables.go:192`); map loading stops at `MaxArrayValues` (`:2041`). Merely
printing a map cannot establish complete values. A future reader must prove
loaded children exhaust the map length, keys are complete, and values are
readable, rather than silently truncating at its load configuration.

| Cached source relative to Delve module | SHA256 |
| --- | --- |
| `pkg/proc/target.go` | `F433D4847FD9D0B4488950A0834B34FED3AFEFEBC1FC2F2F8CBEE16DCBAE24AD` |
| `pkg/proc/target_group.go` | `A98C6DDCA41558CA08C22451DA1113A9AA2E2D5AAE10692F1EB4A788A8530A21` |
| `pkg/proc/native/proc.go` | `5C384EFA34F3E7687BEF31388E868A638B758D5DEE9E26C13EB2753DE6BACD4F` |
| `pkg/proc/native/proc_windows.go` | `1C680435498A7F2967795D11C37A2F7252C17BB1747D9B1C78F7B7B0816E2276` |
| `pkg/proc/native/threads_windows.go` | `43D8EA5E65ADAB149B536CE8407B910B3C9707778C5CD5E831FD7366D275D57A` |
| `pkg/proc/variables.go` | `02FCF4AEE8C27805ADE06E25BA95723082546F8CF3D900DB917E9A94F65CF514` |
| `service/debugger/debugger.go` | `B286DBBF691D32F8D0ACF362A35A91305C3A65B4CC9CF233599ECE70789B8387` |

Recompute source hashes with `Get-FileHash -Algorithm SHA256` before trusting
line references. A different source/binary pair is a new audit, not permission
to run the cached command path. No diagnostic or optimized target artifact,
overlay, fixture, runtime output or timing sample exists in story 001.

## Timing comparability remains held

After admission, build the target once outside the consuming smoke, offline;
record the compiler, module cache, product pin, overlay hashes, complete flags
and executable hash. Diagnostic `-gcflags=all=-N -l` artifacts differ from
optimized artifacts. Never use their elapsed time as optimized P01 evidence.
Prove owner correspondence across artifacts or leave that edge BLOCKED.

Future baseline/final measurements need matching lifecycle boundaries, build
flags, environment and controlled effects, with observer suspension outside
the timed interval. Retain all valid ranges and contaminated attempts; count
peer/observer overhead explicitly. No ten-sample cohort, 100-cycle workload,
variance requirement, quiet-host prerequisite, or performance threshold is
introduced here. At most one unchanged-base contention retry with isolated
`-p 1` is permitted; there was no build timeout or retry in story 001.

## Remaining work and smallest decision

OBS-1 static identity and unsafe-path rejection are established. OBS-2 runtime
access, coherent synchronization, complete values, nonmutation, nonzero fixture
and cross-artifact correspondence remain BLOCKED. Story 002 must reproduce
this boundary from a fresh pin and complete the deterministic dossier, or
demonstrate a separately admissible path before any smoke. It must use the
operator authority procedure if proceeding requires a scope/tool decision.

The smallest missing capability is an independently auditable external reader
that suspends and reads the exact target without automatic target writes and
recovers complete composed owner state. The owning decision is whether to
authorize investigation of that specific capability or retain P01 BLOCKED and
accept an infeasibility dossier. No broader production instrumentation or
changed pin is implied. This first-story increment does not request or assume
that decision. Independent feasibility release belongs to OBS-V/OBS-REVIEW;
T27/T22 own P01, I01 owns shutdown/flush, VAL01 owns composed acceptance and
T29/S01 owns final construction policy.

## Quality and handoff

From the delivery worktree, run these bounded static checks with downloads
disabled; record exact exit statuses in the local progress log and PR comment:

```powershell
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$cachedGo = 'C:/Users/andre/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64/bin/go.exe'
& $cachedGo run ./cmd/markdown-linter docs/internal/development/plans/flat-injection
git diff --check
```

The first attempt through plain `go run` exited 1 before linting:
`go: golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64: verifying module: checksum database disabled by GOSUMDB=off`.
Invoking the already-cached compiler directly, with GOTOOLCHAIN still `auto`,
GOPROXY/GOSUMDB `off`, exited 0 for the complete plan directory. No dependency
download, checksum-network access or toolchain setting override was used.
This is an explicit tool-path correction, not a contention retry or runtime
test. The source pin remained clean throughout; no overlay was installed.

These checks prove Markdown/whitespace quality only. No test support or tests change;
no source-scanning test substitutes for runtime proof. Commit only the new
runbook, push the verified increment and open a draft PR while story 002
remains. Independent readers use the
[canonical loopback template](../../../../../factory/docs/standards/validation-loopback-template.md).
CI evidence belongs in a PR comment. Review owns terminal CI, current-main
premerge checks and merge. Never claim an author-produced independent verdict.

To clean up, first verify `git -C $scratch rev-parse HEAD` equals the pin,
`git -C $scratch status --porcelain` is empty, and the resolved scratch path
is the exact owned path above. Then `git worktree remove $scratch` removes
only that clean scratch; do not force-remove, delete peer paths or kill peer
processes. No target/observer PID needs cleanup for this static investigation.
