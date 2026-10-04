# In-process owner observation support

This is the test-only observation enabler authorized by OWNER option b on
2026-10-03T22:45Z. It adds no production API or tracked Go package. The collector
and owner instrumentation exist only in the Go overlay prepared below.

This support establishes owner calibration, report validation and matching
original-pin/candidate admitted Q0/Q1/Q2 scenarios below. The authorized
method amendments are in the governing plan's
[amendment record](../../../docs/internal/development/plans/flat-injection/inprocess-baseline-observer.md).
Do not call these tests a P01,
FI-A6, full RQ-2 or retention pass.

## Prepare and verify

Run from the lane worktree, with cached Go 1.25 and Python 3. No dependency
downloads, debugger, model, LocalAI or paid provider are needed. Leave
GOTOOLCHAIN unset or auto. GOPROXY=off rejects uncached dependencies; leave
GOSUMDB at its normal value because the cached auto toolchain still requires
its checksum verification.

```powershell
$env:GOPROXY = 'off'
python tests/stress/observer/prepare.py --source-workspace . --output .artifacts/observer-candidate --mode candidate
$overlay = '.artifacts/observer-candidate/overlay.json'
$hostPackage = './pkg/services/factory_runtime/internal/services/instance_host/internal/service'
$leasesPackage = './pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service'
go test -overlay $overlay -p 1 -count=1 -run TestInProcessObserver ./pkg/platform/baselineobservation $hostPackage $leasesPackage
go test -race -overlay $overlay -p 1 -count=1 -run TestInProcessObserver ./pkg/platform/baselineobservation $hostPackage $leasesPackage
go test -overlay $overlay -p 1 -count=1 -short=true -run '^TestObserverReport' ./tests/stress/observer
go test -overlay $overlay -p 1 -count=1 -short=false -run '^TestInProcessObserverQ0$' ./tests/stress/observer
make lint pkg-file-count
git diff --check
```

Preparation accepts an existing local source workspace and an output directory
outside source, or its `.artifacts` directory. Pin mode requires HEAD exactly
`95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`; it never fetches or provisions source.
It fails on an unsupported constructor transformation. It writes backing files,
`overlay.json` and `manifest.json`, and creates empty virtual package directories
required by Go vet. It changes no source bytes. Default builds import none of
the virtual packages. `-short=true` skips Q0 and the lifecycle diagnostic;
report unit tests still run.

The manifest separates source commit, tool commit, source status, overlay hash,
backing hashes, fixture and command. Use a committed tool revision for qualified
evidence; development runs with dirty tool sources are provisional. Instrumented
test bytes must never be identified as the unchanged historical binary.

## Original-pin lifecycle diagnostic

Use one isolated local source worktree at the original pin. The following
creates it from already available Git objects; omit `git worktree add` when
that lane-owned workspace already exists. Do not reuse another lane's checkout.
Run preparation from the tool worktree and compilation from the source worktree:

```powershell
$toolWorkspace = (Get-Location).Path
$pinWorkspace = Join-Path (Split-Path $toolWorkspace -Parent) 'fi-t27-observer-pin-source-20261003'
$pinOutput = Join-Path $toolWorkspace '.artifacts/observer-pin'
git worktree add --detach $pinWorkspace 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6
$env:GOPROXY = 'off'
python tests/stress/observer/prepare.py --source-workspace $pinWorkspace --output $pinOutput --mode pin
$overlay = Join-Path $pinOutput 'overlay.json'
$artifact = Join-Path $pinOutput 'observer.test.exe'
Push-Location $pinWorkspace
go test -c -overlay $overlay -p 1 -o $artifact ./tests/stress/observer
git rev-parse HEAD
git status --porcelain
go version
go version -m $artifact
Get-FileHash -Algorithm SHA256 $overlay,$artifact
$env:OBSERVER_MANIFEST = Join-Path $pinOutput 'manifest.json'
$env:OBSERVER_ARTIFACT_SHA256 = (Get-FileHash -Algorithm SHA256 $artifact).Hash.ToLower()
$env:OBSERVER_REPORT = Join-Path $pinOutput 'smoke.json'
& $artifact '-test.run=^TestInProcessOwnerLifecycleSmoke$' '-test.count=1' '-test.short=false' '-test.timeout=5m' '-test.v'
Pop-Location
```

Quote the entire native test flag in PowerShell; unquoted dotted flags can
be split into an invalid `-test` argument. The test verifies its running
executable's hash against the supplied artifact hash. Use a committed clean
tool revision, preserve the manifest/backing files and complete stdout/stderr
outside the PR diff, and record results in a PR comment. The prepared manifest
attests all injected backing files, source commit, tool commit and overlay.
Keep pinned source outside the tool checkout: some repository lint scanners
walk ignored directories and would otherwise count historical source as current.

One BuildProcess serves the entire sequential scenario. Execute starts an idle
continuous host using a test-owned home and HTTP binding. The API starter's
ready channel acknowledges binding; public Factory Sessions Start/Invoke/Control
drive the explicitly allocated session. Work must return COMPLETED both before
and after replacement. Cancellation carries a stable request ID and must return
SUCCEEDED before same-ID Start closes the old activation and creates a new one.
The caller supplies a distinct RuntimeInstanceID for each generation and checks
the current stream generation and actual Q1 handle key against that identity.
Session projections acknowledge lifecycle; they never provide resource counts.

Q0 follows inert construction. Q1 follows replacement's prior-activation cleanup
and terminal Work acknowledgment; the controlled command has returned, no Models
mutation is admitted, and no concurrent session lifecycle operation is issued.
The continuous bootstrap remains live and its real handle is included. Q2 follows
explicit close, cancellation and join of Execute/server, and successful
Process.Close. Session close runs the owned runtime stop, sidecar join, worker
lifecycle cleanup and retirement phases. Empty registry membership alone is
never treated as that join. Every registered owner, including retired owners,
remains observable until after report emission.

Abort reports retain completed checkpoints, the last attempted barrier and
cleanup errors with `complete:false`. Failed joins cannot produce a successful
Q2. Ceilings bound failures; there are no timing samples, sleeps or retention
loops. Reports include actual residuals and live goroutines without requiring
guessed zero or equality to Q0. Retained Models records are never removed by
observation. Controlled zero Models activity leaves RQ-2-COMPOSED-CAPACITY
unproven. Provider progress warnings and goroutine noise belong in the evidence,
with no claim that this scenario proves real provider delivery or leak absence.

At the original pin, leases calibration uses the existing two-argument
constructor and coordinator binder; candidate calibration uses its existing
three-argument constructor. Host calibration passes `t` to the candidate's
lifecycle-control helper so its fixture cleanup remains registered; preparation
removes that argument only for the original pin's one-argument helper.
Preparation adapts only test setup for that exact
pin. The synchronized owner snapshots and lifecycle fixture are identical in
meaning; neither constructor nor binding policy is changed in production.

## Observation semantics and limits

Each real owner constructor registers its pointer identity and source. The
collector retains every constructed owner until explicit observer shutdown;
this includes retired owners. Its references are measurement overhead. Close
and Unregister release callbacks only, without cleaning product state.

Host reads copy registry keys and physical handle pointer identities under
Host.mu. Models reads copy retained lease records, status and claimed state,
every capacityHolders integer and the sum under leases.mu. Sorting and eventual
encoding happen after owner unlock. No getter, expiration, cleanup, coordinator
or nested collaborator runs during capture. Copies can be mutated by a consumer
without changing later observations.

Capture releases its collector lock before reading owners. It is synchronized
per owner, not an atomic multi-owner snapshot. Coherent lifecycle observation
requires a caller-owned quiescent barrier. Missing, duplicate, unreadable or
incompletely attributed owners produce errors, never guessed zero counts.

Calibration demonstrates two holders in one slot, one in another, detached
copies, overdue ACTIVE records preserved until actual owner expiration,
RELEASED/EXPIRED retention, and concurrent normal owner mutation. It does not
prove composed nonzero Models activity. That historical witness stays with
`RQ-2-COMPOSED-CAPACITY` (Lead/T27).

Future original-pin and candidate measurement must use matching instrumentation,
fixture and barriers. Account for callback retention, registration, locking,
sorting and collector allocation; serialize outside lifecycle timing spans.
The unchanged obligations remain 10 inert plus 10 lifecycle valid samples,
full observed ranges, final median/p95 no more than 10% above baseline, and
100-cycle retention. T27-P01/T22-P01 own these measurements; I01 owns delivered
CLI shutdown/flush, and VAL01 owns aggregate Project acceptance.

Independent review repeats focused normal/race/report/Q0 commands on a clean
exact-head checkout and reports the observed property and remaining edges in
a PR comment, and repeats the original-pin procedure above for OBS-PIN.
For OBS-FINAL, repeat the identical lifecycle selector against the candidate:

```powershell
$env:GOPROXY = 'off'
$candidateOutput = Join-Path (Get-Location).Path '.artifacts/observer-candidate'
python tests/stress/observer/prepare.py --source-workspace . --output $candidateOutput --mode candidate
$overlay = Join-Path $candidateOutput 'overlay.json'
$artifact = Join-Path $candidateOutput 'observer.test.exe'
go test -c -overlay $overlay -p 1 -o $artifact ./tests/stress/observer
git rev-parse HEAD
git status --porcelain
go version
go version -m $artifact
Get-FileHash -Algorithm SHA256 $overlay,$artifact
$env:OBSERVER_MANIFEST = Join-Path $candidateOutput 'manifest.json'
$env:OBSERVER_ARTIFACT_SHA256 = (Get-FileHash -Algorithm SHA256 $artifact).Hash.ToLower()
$env:OBSERVER_REPORT = Join-Path $candidateOutput 'smoke.json'
& $artifact '-test.run=^TestInProcessOwnerLifecycleSmoke$' '-test.count=1' '-test.short=false' '-test.timeout=5m' '-test.v'
```

OBS-VAL uses a clean exact-head checkout, the focused normal/race/report checks,
and sequential original-pin/candidate runs of this same scenario. Publish the
factory validation-loopback report in a PR comment, including source/tool SHA,
overlay/backing/artifact hashes, full output, checkpoints, cleanup failures,
goroutine residuals, and PASS/FAIL/BLOCKED for each owned criterion. Do not
repair code during independent validation. Composed zero Models activity
keeps RQ-2-COMPOSED-CAPACITY BLOCKED; P01/I01/VAL01 remain later gates.
Review owns terminal CI, current-main reconciliation and
immediate lint/pkg-file-count before merge.

## Dedicated lifecycle profile preparation and sample capability

The profile tool supports preparation, the short guard, one inert plus one
explicit-session sample, and a joined replacement-cycle diagnostic. Full profiling
remains unsupported until the composed Models capacity story ships. The composed
capability selector retains the replacement diagnostic and fails explicitly with
complete:false because it has no public Models acquisition witness yet. These
diagnostics produce no timing summary, capacity witness, retention verdict or
qualified baseline.

Use clean committed tooling and clean source. Compilation is optional and occurs
only in preparation, using cached Go 1.25 with GOPROXY=off. Preparation retains
build stdout, stderr and exit status and writes an absolute artifact path and
SHA256, source/tree/tool identities, all overlay/backing hashes, toolchain/build
information and owned preparation/runtime environment paths in profile-manifest.json.
Before any Go child runs, Python copies the already cached exact go.mod toolchain,
expanded required modules and .mod/.info/.ziphash metadata into preparation/.
Each copied input has source/path/SHA256 attribution in preparation-inputs.json;
the profile manifest records its hash. Copies use separate bytes, with no shared
cache writes, hardlinks or downloads. Preparation starts with an empty owned
build cache; repeated preparation can reuse that same owned cache. Its preflight
reserves 512MiB for a cold build within the 2GiB output budget. On repeated
preparation, already counted cache/artifact bytes reduce the 384MiB cache/artifact
allowance; a 128MiB temporary-file floor remains. Also account for other lane-owned
output directories before running.
Missing cached inputs or unsupported replace/exclude directives fail actionably.
Use --cached-module-cache and --cached-go-root for explicit read-only input paths
when default GOMODCACHE/GOPATH and exact cached auto-toolchain discovery do not fit.
All preparation children receive owned HOME/USERPROFILE, AppData, XDG config/cache,
Go cache/module/temp and TEMP/TMP paths. GOENV/GOWORK/GOPROXY/GOSUMDB/telemetry are
off, GOFLAGS is empty, and GOTOOLCHAIN remains auto with the copied exact toolchain.
Wrong pins, dirty sources/tooling, unsupported transforms and failed builds
remove any previous profile handoff and exit nonzero. Existing preparation
without --build-profile remains available for provisional development checks.
Preparation copies the checkpoint and profile schemas beside the manifests so
read-only validators can resolve local references without compiling.

```powershell
$env:GOPROXY = 'off'
$pinWorkspace = Join-Path (Split-Path (Get-Location).Path -Parent) 'fi-t27-profiler-pin-source-20261004'
# Allocate this lane-owned source once, from existing local Git objects.
git worktree add --detach $pinWorkspace 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6
$pinOutput = Join-Path (Get-Location).Path '.artifacts/profile-pin'
python tests/stress/observer/prepare.py --source-workspace $pinWorkspace --output $pinOutput --mode pin --build-profile
$env:OBSERVER_PROFILE_MANIFEST = Join-Path $pinOutput 'profile-manifest.json'
$env:OBSERVER_REPORT = Join-Path $pinOutput 'samples.json'
$artifact = Join-Path $pinOutput 'observer.test.exe'
& $artifact '-test.run=^TestLifecycleProfile$' '-test.short=true' '-test.count=1' '-test.timeout=1m' '-test.v'
& $artifact '-test.run=^TestLifecycleProfileSamplesCapability$' '-test.short=false' '-test.count=1' '-test.timeout=5m' '-test.v'
```

Repeat preparation with --mode candidate and the lane worktree as source for
candidate compatibility. Tests do not build artifacts. The running artifact
verifies its own hash and the supplied preparation/overlay/backing hashes before
construction. It applies the manifest's owned home, profile, AppData, config,
cache and temporary directories. Preparation uses detached cached build inputs;
runtime uses fresh owned cache paths and never invokes Go or downloads assets.

Inert timing starts immediately before BuildProcess and ends when Process.Close
returns. Session timing starts immediately before explicit Start and ends after
terminal Work and public Close return. Fixture creation, idle host readiness,
owner snapshots, sorting and serialization occur outside those spans. Go
 time.Now/time.Since uses monotonic elapsed integer nanoseconds; UTC timestamps
only identify the run. Failure stops completeness without retry or replacement.
Every attempted sample, failure, cleanup error and noisy valid duration remains
in the raw report and test output. Preserve full stdout/stderr and exit status.

The statistics helper uses all valid values, sorted ascending, arithmetic mean
of the middle pair for even N and nearest-rank p95 ceil(0.95*N), one-based.
At N=10 p95 equals max. Capability summaries contain counts with null timing
statistics; samples capability cycles and capacityWitnesses are empty arrays. No ordinary test has
a wall-clock performance threshold. Report tests also reject failed barriers,
missing owners, null known-empty attempt arrays and mismatched artifact bytes.

SAMPLE_CLOSED follows public close acknowledgment. HOST_BASELINE follows joined
readiness and bootstrap inventory. Q2 follows canceled/joined Execute and server
and successful Process.Close. Cleanup uses fresh bounded contexts; failures
remain incomplete. Owners, retained lease records and per-slot integer holders
are observed under their existing locks without expiration or state mutation.
The report records retained constructor callbacks, snapshot count and elapsed
capture outside spans. The report's serialization field records the first
encoding pass. The required `<report>.overhead.json` receipt records both
encoding passes, the final report write, and their total, with the exact report
SHA256. Retain both files and require the receipt hash to match the report;
an encoding or write failure fails the selector and leaves the report incomplete.
Receipt encoding/write and raw test logging are additional outside-span costs,
explicitly excluded from the receipt's measured span. They must also use the
same procedure in later baseline/final runs.
Match this method, instrumented bytes, fixture, cache/environment and barriers
between later baseline/final runs; do not subtract guessed instrumentation cost.

Focused checks (from the isolated source workspace, with absolute overlay):

```powershell
python -m unittest discover -s tests/stress/observer -p test_profile_prepare.py
$overlay = Join-Path $pinOutput 'overlay.json'
Push-Location $pinWorkspace
go test -overlay $overlay -p 1 -count=1 -short=true -run '^Test(ObserverReport|LifecycleProfileReport|LifecycleProfileDriver)' ./tests/stress/observer
go test -overlay $overlay -p 1 -count=1 -run TestInProcessObserver ./pkg/platform/baselineobservation ./pkg/services/factory_runtime/internal/services/instance_host/internal/service ./pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service
Pop-Location
```

The replacement diagnostic uses the same prepared artifact and owned environment:

```powershell
$env:OBSERVER_REPORT = Join-Path $pinOutput 'replacement.json'
& $artifact '-test.run=^TestLifecycleProfileCapability$' '-test.short=false' '-test.count=1' '-test.timeout=5m' '-test.v'
# Expected exit 1 until story 003: composed Models capacity unsupported.
```

It opens one explicitly allocated session, completes Work, cancels, replaces that
same session with a fresh runtime, completes Work again, and closes. CYCLE_OPEN
follows Start and terminal Work; CYCLE_REPLACED follows Cancel, replacement Start
(including prior runtime cleanup), and terminal Work; CYCLE_CLOSED follows public
Close and owned stop. Each checkpoint retains both known generation identities
where available, runtime instance IDs, actual handle identities, owner discovery
status, lease records/statuses, per-slot integer holders/totals, and live goroutines.
The current handle must exist exactly once; the replaced handle must be absent;
after close both scenario handles must be absent. Bootstrap handles remain visible.
Read-only snapshots neither expire overdue ACTIVE records nor remove RELEASED or
EXPIRED records. Capture, sorting and serialization occur outside sample spans.

The sequential driver accepts up to 100 cycles on one compatible shared process;
unit callbacks prove exactly 100 contiguous cycles with distinct session/runtime
identities. The real diagnostic runs only one cycle. The full selector remains
disabled pending composition; no 100-real-cycle retention verdict is claimed.
Failed cycles retain their partial checkpoints, attempted/failed barriers and
causes; no automatic retry replaces a failed cycle. Every attempted Start receives
owned Close cleanup on failure, using a fresh bounded context, even if the main
context is canceled. Cleanup errors are joined with the original cause. Execute
and server are canceled/joined before Process.Close; a failed join prevents Q2.
The method hash covers the named sample, cycle and report templates in that order
(UTF-8 name, NUL, raw template bytes, NUL); individual backing hashes remain in the
manifest. Match these bytes and barriers during later baseline/final validation.

Full workload remains 10 inert and 10 explicit-session samples plus 100 joined
open/replace/close cycles and composed nonzero Models capacity. Its eventual
command is the artifact with '-test.run=^TestLifecycleProfile$', '-test.count=1',
'-test.short=false', '-test.timeout=10m' and '-test.v', with an operator-recorded
OBSERVER_HOST_WINDOW reference. Do not execute that workload on the shared host.
OBS-VAL/FI-PREREQ-BASELINE-OBS independently qualifies this exact supplied
artifact; T27-P01 owns the confirmed no-co-tenant measurement window and actual
baseline; T22-P01 owns the matching <=10% median/p95 comparison. I01 and VAL01
remain separate. Author delivery stops at pushed head, non-draft PR, CI started
and addressed blocking feedback; review owns hosted race, terminal CI and merge.

Capacity report validation now requires the composed witness to retain the
Factory Session, model scope, model name, owner, slot, invocation IDs and lease
IDs. Both CAPACITY_ACTIVE and CAPACITY_RELEASED checkpoints must carry joined
barriers and matching artifact/runtime identities. The active slot's integer
count must match the witness, and each named lease must be claimed and ACTIVE.
After joined cleanup the slot has zero holders while every named record remains
RELEASED or EXPIRED. Existing owner policy retains the claimed flag on terminal
records; validation does not rewrite it. Synthetic calibration cannot satisfy
this completeness check. The public acquisition fixture and full selector remain
unfinished; these accounting tests alone do not prove composed Models activity.


The controlled Models gate is supplied in `profile-model-gate.go.txt`. Install
its `invoke` callback at the existing `ModelInvocationBackend` edge after
Models owner acquisition. Declare each Work input before construction; accepted
observations preserve the real request scope and detached inputs/parameters.
Two routes can remain held concurrently, and every route must keep the same
explicit-session model scope. Unexpected models, operations, inputs, duplicate
routes and changed scopes fail closed. Release is idempotent; controlled inference
faults and cancellation retain their original error causes. Join the callback's
return **and** the public terminal Work before taking CAPACITY_RELEASED: a
callback-return barrier alone cannot prove lease release. Gate tests run under
the focused `TestLifecycleProfileDriver` selector and are synthetic component
calibration, never composed public capacity evidence. This gate is not yet wired
into the public capability; story 003 remains incomplete. The method hash also
includes the gate template after the sample, cycle and report templates.

The controlled Models fixture now supplies cached inert model/backend bytes,
loopback health, negotiated protocol, and a joinable host lifetime at the existing
external-effect edges. Its component tests establish stop/join and refusal of
downloads. Public session composition is still unfinished; these fixture tests
do not establish real capacity acquisition or baseline acceptance.

`TestLifecycleProfileModelDiagnostic` is a narrow public acquisition selector
on the prepared artifact (`-test.short=false -test.count=1 -test.timeout=2m`).
Set `OBSERVER_REPORT` to an absolute diagnostic output path. It starts an
explicit session with the published invocation content contract, holds two
EMBED calls at the controlled effect, records CAPACITY_ACTIVE, then joins both
the effect and terminal Work before CAPACITY_RELEASED. The scenario asserts
two attributed ACTIVE claimed leases and two integer holders in one slot, followed by
zero holders and both retained RELEASED/EXPIRED records. It closes the session
and joins the host and process. Public configured capacity is 2; this narrow
selector proves same-slot multiplicity but does not prove fault/cancellation composition
or the complete capability. Its report uses mode `model_diagnostic` and stays
`complete:false`, with empty samples and capacityWitnesses; raw checkpoints and
attribution remain available. It skips before construction under `-short`.

`TestLifecycleProfileModelFaultDiagnostic` uses the same two-holder public
scenario and returns a controlled inference error for the held peer. It requires
FAILED/INVOCATION_RUNTIME_FAILURE while the other Work completes.
`TestLifecycleProfileModelCancelDiagnostic` completes one Work, then cancels the
explicit session while its peer remains held. It requires the peer's public
CANCELED/INVOCATION_CANCELED outcome and joins its callback before observation.
Both selectors require zero holders and retained terminal leases, followed by
session, Execute/server and Process.Close joins. Run each on the prepared
artifact with `-test.short=false -test.count=1 -test.timeout=2m -test.v` and its
own absolute `OBSERVER_REPORT`. Raw output preserves the public result, Work and
session IDs, and error classification. Their diagnostic reports stay incomplete;
these narrow selectors do not establish the full profile or timing acceptance.
