# In-process owner observation support

This is the test-only observation enabler authorized by OWNER option b on
2026-10-03T22:45Z. It adds no production API or tracked Go package. The collector
and owner instrumentation exist only in the Go overlay prepared below.

Current delivery establishes owner calibration, report validation and inert
Q0 registration. The admitted lifecycle scenario, original-pin run and matching
candidate Q1/Q2 observations remain unfinished. Do not call these tests a P01,
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
go test -overlay $overlay -p 1 -count=1 -short=true -run '^TestObserverReportValidation$' ./tests/stress/observer
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
the virtual packages. `-short=true` skips Q0; report unit tests still run.

The manifest separates source commit, tool commit, source status, overlay hash,
backing hashes, fixture and command. Use a committed tool revision for qualified
evidence; development runs with dirty tool sources are provisional. Instrumented
test bytes must never be identified as the unchanged historical binary.

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
a PR comment. Full OBS-PIN/OBS-FINAL/OBS-VAL procedures will be completed with
the next stories. Review owns terminal CI, current-main reconciliation and
immediate lint/pkg-file-count before merge.
