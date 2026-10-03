# Installed Windows GDB external-reader qualification

## Decision and scope

RQ-AUDIT rejects installed Windows GDB 16.3 before target access:
`missing_prerequisite`. Matching native implementation, local patches and
binary/source correspondence were not established within the bounded cached
search. This is an unsupported candidate, not a demonstrated unsafe write and
not a claim that every external reader is impossible.

The only candidate is `C:/ProgramData/mingw64/mingw64/bin/gdb.exe`.
No target was built, launched, opened in GDB or attached. No debugger lifecycle
command, target expression, memory/register write, breakpoint, function call,
model service, provider service or daemon operation was attempted. There are
no observed resource counts, timing samples or runtime feasibility results.
Stock Delve is not reconsidered; its earlier rejection remains in
[baseline-observer-runbook.md](baseline-observer-runbook.md).

This runbook implements story 001 of
`fi-t27-no-write-reader-qualification-20261003`. Story 002 must separately
repeat the decisive rejection from a fresh original-pin checkout. The author
does not issue independent RQ-VAL or RQ-REVIEW verdicts. Observer release,
T27/T22, FI-A6 and P01 remain BLOCKED.

## Immutable identities

Product source pin: `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`.
The author scratch is
`C:/Users/andre/work/portos/fi-gdb-audit-pin-20261003-001`, separate from the
delivery worktree. A source checkout is not a compiled target artifact.
The successful creation and HEAD/status commands exited 0: HEAD matched the
pin and status was empty before and after source inspection.

| Input | SHA256 or disposition |
| --- | --- |
| Installed `gdb.exe` | `76A2847EC717563C9CFE94EDEDD42BA354C7EB1B63A765F4143B639278C609BD` |
| Cached Go compiler `C:/Users/andre/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64/bin/go.exe` | `5C08733F77C624DD4B34BC157626656EF2485DE987A8495A4B804CBB91F01726` |
| Installation `C:/ProgramData/mingw64/mingw64/etc/gdbinit` | `B409CB2A6F9834038A0E300CF383780C05A870528969C05A8A7B99C40A589D14` |
| Pinned instance-host `internal/service/service.go` | `EB6146082F8BD136A60A68527475EBC247A21C6EE125ABBEA8A972446CDDA61E` |
| Pinned Models leases `internal/service/service.go` | `2ABBE4F500FBCB40BD02CE4223045ED365BD04BBD29C2F16D9A963F77D335B46` |
| GDB implementation and local patches | Unavailable in completed searches; no source hash or binary correspondence established |
| Loaded DLL/support closure | Not audited after source-provenance rejection; no no-write guarantee |
| Product target / external support executable | Absent; no build flags, executable hash or retained runtime artifact exists |

Hashes identify inspected inputs; they do not certify safety. Version commands
reported `GNU gdb (GDB) 16.3` and `go version go1.25.0 windows/amd64`.

## Target-free reproduction

Run only these identity and filesystem commands. Use a distinct unused owned
scratch path for each later rehearsal. Do not reuse the delivery lane as a
detached evidence checkout. Leave GOTOOLCHAIN unset or `auto`; no downloads or
paid calls are permitted.

```powershell
$scratch = 'C:/Users/andre/work/portos/fi-gdb-audit-pin-20261003-001'
git worktree add --detach $scratch 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6
git -C $scratch rev-parse HEAD
git -C $scratch status --porcelain
$gdb = 'C:/ProgramData/mingw64/mingw64/bin/gdb.exe'
$cachedGo = 'C:/Users/andre/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64/bin/go.exe'
Get-FileHash -Algorithm SHA256 $gdb, $cachedGo
& $gdb -nx -nh -batch --version
& $gdb -nx -nh -batch --configuration
& $cachedGo version
Get-Content 'C:/ProgramData/mingw64/mingw64/etc/gdbinit'
Get-FileHash -Algorithm SHA256 'C:/ProgramData/mingw64/mingw64/etc/gdbinit'
```

GDB version/configuration each exited 0. Configuration includes:

```text
configure --host=x86_64-w64-mingw32 --target=x86_64-w64-mingw32
--enable-targets=x86_64-w64-mingw32
--with-auto-load-dir=$debugdir:$datadir/auto-load
--with-auto-load-safe-path=$debugdir:$datadir/auto-load
--with-python=C:/buildroot/x86_64-1520-posix-seh-ucrt-rt_v13-rev0/mingw64/opt
--without-debuginfod
--with-system-gdbinit=/c/buildroot/x86_64-1520-posix-seh-ucrt-rt_v13-rev0/mingw64/etc/gdbinit (relocatable)
```

These are build-reported strings, not recovered source directories. The
installed init file contains:

```text
python
import sys
sys.path.insert(0, sys.path[0] + '/../../gcc-15.2.0/python')
from libstdcxx.v6.printers import register_libstdcxx_printers
register_libstdcxx_printers (None)
end
```

It was read as text, not executed as a target script. `-nx -nh` on the
target-free identity calls is not proof of nonmutating launch/attachment or
complete autoload/DLL/error-path control.

## Bounded source search and exact rejection

The installation inventory command was:

```powershell
rg --files --hidden C:/ProgramData/mingw64 -g '*gdb*' -g '*windows-nat*' -g '*go-lang*' -g '*infrun*' -g '*breakpoint*' -g '*tar*' -g '*zip*'
```

Exit 0 returned installed binaries (`gdb.exe`, `gdborig.exe`, `gdbserver.exe`),
`etc/gdbinit`, Python/support files and unrelated archives/headers. No native
GDB implementation or matching source archive was returned. Neither other
binary is a candidate in this lane.

For each existing cache root below, the literal command was:

```powershell
rg --files --hidden $root -g 'windows-nat.c' -g 'windows-nat.cc' -g 'go-lang.c' -g 'go-lang.h' -g 'infrun.c' -g 'infrun.cc' -g 'gdb-*.tar*' -g '*gdb*16.3*' -g '*mingw*src*' -g '*mingw*source*' -g '!node_modules/**' -g '!**/.git/**'
```

| `$root` | Observed result |
| --- | --- |
| `C:/Users/andre/.cache` | Exit 1, no matching paths |
| `C:/Users/andre/Downloads` | Exit 1, no matching paths |
| `C:/Users/andre/scoop/cache` | Exit 1, no matching paths |
| `C:/Users/andre/AppData/Local/Temp` | Incomplete; access denied for `apkg-legacy-mcgvz1g4`, `apkg-legacy-bl_siins`, `apkg-real-lg4tk3j4`, `apkg-real-eo9fdlp8`; owned search stopped, exit -1 |
| `C:/Users/andre/source` | Exit 1, no matching paths |

The cache-search script elapsed 108.7941007 seconds. Only its own verified
`rg.exe` PID 90004 (parent 105100, exact Temp search command line) was stopped.
No matches had been emitted by that search. This is not a whole-disk absence
claim: denied/incomplete roots and alternate source naming remain unknown.
Searching forever is not required to reject an unestablished prerequisite.
No remote source retrieval or second reader investigation followed.

Attempt history: installation identity/inventory, bounded cache search,
source-provenance rejection. An overlapping scratch setup command reported
`fatal: 'C:/Users/andre/work/portos/fi-gdb-audit-pin-20261003-001' already exists`
and exit 1; the separately started creation owns that path. Check the
successful creation's pin/clean status before trusting any source inspection.
This setup diagnostic is not candidate runtime evidence.

Admission rule: if exact native implementation, patches and correspondence
cannot be established, stop here before any target operation. No command for
launch, attach, stop, read, resume or detach is admitted by this runbook.

## Required observations and unchecked obligations

The pinned owner/writer, retained-status and checkpoint contract remains in
[the prior runbook](baseline-observer-runbook.md). Rejection leaves every
runtime obligation unproved:

Pinned source/coverage inspection commands (exit 0) were:

```powershell
$hostRoot = "$scratch/pkg/services/factory_runtime/internal/services/instance_host/internal/service"
$leaseRoot = "$scratch/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
rg -n 'handles|leases|capacityHolders' "$hostRoot/service.go" "$leaseRoot/service.go"
Get-FileHash -Algorithm SHA256 "$hostRoot/service.go", "$leaseRoot/service.go"
$hostTests = @(rg -n '^func Test' "$scratch/pkg/services/factory_runtime/internal/services/instance_host" -g '*_test.go')
$leaseTests = @(rg -n '^func Test' "$scratch/pkg/services/models/internal/services/runtime_host/internal/services/leases" -g '*_test.go')
Write-Output "host_test_declarations=$($hostTests.Count) lease_test_declarations=$($leaseTests.Count)"
```

The observed declarations were 56 and 23 for these recursive search roots;
they differ from the planner's 34/22 estimates. No tests were executed and no
coverage sufficiency or external-reader witness is inferred from these counts.
The owner fields were `handles map[string]*factoryhost.Handle` at line 21,
`leases map[string]leaseRecord` at line 21 and `capacityHolders map[string]int`
at line 22. The matching source hashes do not provide runtime values.

| Obligation | Status / missing proof |
| --- | --- |
| Launch/attach/initialization/error/cleanup paths | BLOCKED: matching native source, OS calls/access rights, implicit writes and register effects not traced |
| Autoload, Python, init overrides and loaded support/DLLs | BLOCKED: complete reachable implementation closure not established |
| All-thread suspension, new-thread races, resume/detach | BLOCKED: OS/tool guarantees and cleanup not audited; suspension alone cannot certify writer quiescence |
| Composed private pointer provenance | BLOCKED: no target artifact or recovered composed owners |
| Instance-host `handles` | BLOCKED: no complete external observation; empty registry would not prove cleanup joins |
| Models retained `leases` records/status/claimed | BLOCKED: no complete external observation; RELEASED/EXPIRED retention must not be treated as active capacity |
| Every `capacityHolders` integer and sum | BLOCKED: no complete decoding or nonzero witness; key count is not holder total |
| Live target goroutines | BLOCKED: no Go 1.25 runtime-layout/status decoding; dead `allgs` entries must be excluded |
| Go 1.25 map layout, paging and decoding ceilings | BLOCKED: no proven reader implementation or target DWARF/layout correspondence; missing/truncated values are never zero |
| Q0/Q1/Q2 synchronization and lifecycle joins | BLOCKED: no public-driving composed fixture, writer gates, snapshot or successful join witness |
| Nonzero composed holder, representative value greater than one | BLOCKED: existing external effect ports do not alone prove an admissible readiness fixture |
| Nonmutation and timing comparability | BLOCKED: no runtime witness or diagnostic/optimized artifact correspondence |

Never call target locks, getters or arbitrary functions to fill these gaps.
Assignment, breakpoint insertion, register/memory writes and injected
evaluation remain denied. The absence of user-entered writes is insufficient
to establish that initialization, stop/resume or cleanup avoids writes.

## Quality, remaining gates and handoff

No product, API, generated files, tests or external support change. Conditional
support unit/race checks are not applicable. The highest feasible evidence is
local-real static rejection; it proves lack of established admission, not a
runtime defect or a successful observation.

From the isolated delivery worktree run:

```powershell
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$cachedGo = 'C:/Users/andre/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64/bin/go.exe'
& $cachedGo run ./cmd/markdown-linter docs/internal/development/plans/flat-injection
git diff --check
make lint pkg-file-count GO=C:/Users/andre/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.windows-amd64/bin/go.exe LINT_TARGETS=pkg-file-count LINT_JOBS=1
```

The explicit lint target selects the required package-file-count check through
the lint lane and the direct Make target; it does not claim the entire
repository lint suite.
Record actual results in local progress and the PR conversation. CI-run
evidence never belongs in a commit. Push story 001 as a draft while story 002
remains. RQ-D final handoff is not yet claimed.

Independent RQ-VAL/RQ-REVIEW must use the
[validation loopback template](../../../../../factory/docs/standards/validation-loopback-template.md)
from the final clean delivery head and a fresh pin. Record PASS for established
dossier properties separately from BLOCKED feasibility. A reviewer may accept
the rejection dossier without releasing the observer. T27/T22 retain P01;
I01 retains CLI shutdown/flush; T29/S01, G01/G02 and VAL01 retain their Project
gates. No Project criterion is certified by this slice.

The smallest owner decision is to retain this rejection and the blocked
measurement gate, or authorize a new bounded investigation with a locally
available, demonstrably corresponding implementation. Matching GDB source
provisioning would require separate authority if it entails downloads or
scope growth. This lane cannot waive AM10, switch candidates, instrument the
product or release T27.

Retain the owned clean scratch for story evidence. Before eventual removal,
verify its resolved absolute path, exact pin and empty status, then use
`git worktree remove` on only that path. No target/observer PID or executable
artifact requires cleanup.
