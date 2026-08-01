# LTV-FUN-01-bootstrap-definitions-005 combined wall-time reduction evidence

Status: complete

Captured: 2026-08-01T19:50:43Z UTC through 2026-08-01T19:54:22Z UTC

Revision: `a604e66018bcc3b4c106fa62a7178c979b316a12`

Branch: `LTV-FUN-01-bootstrap-definitions`

This report covers Story 005. It re-measures all three optimized packages on one
idle-host pass and compares the combined sum of package wall-time medians with
the Story 001 isolated baseline. Contended parallel isolation remains Story 006.

## Machine and toolchain

- OS: Windows 11 Home, build `26200`, 64-bit
- Machine: MSI `MS-7D91`, 63.7 GB RAM, 24 logical processors
- Go: `go1.25.0 windows/amd64`
- Module: `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\LTV-FUN-01-bootstrap-definitions\go.mod`
- Go build cache: `C:\Users\andre\AppData\Local\go-build`
- Host CPU load at sampling start: ~24% average; sampling end: ~33%
- Measurement timestamp normalization: UTC

## Independent post-change benchmark

Each package was sampled three times independently from this revision with the
documented protocol:

```powershell
go test -short ./tests/functional/<package> -count=1 -timeout=5m
```

Wall time was measured with a PowerShell stopwatch around the full `go test`
process. All nine samples exited 0. No failed sample occurred during this
idle-host pass.

| Package | Sample 1 wall / Go | Sample 2 wall / Go | Sample 3 wall / Go | Wall median |
| --- | ---: | ---: | ---: | ---: |
| `bootstrap_portability` | 8.408s / 6.085s | 7.593s / 6.048s | 8.440s / 6.833s | **8.408s** |
| `factory/current` | 10.982s / 9.346s | 10.630s / 9.116s | 10.901s / 9.388s | **10.901s** |
| `factory/definitions` | 12.514s / 10.901s | 12.839s / 11.057s | 12.127s / 10.402s | **12.514s** |

Story 001 package medians were 41.780s, 14.808s, and 16.455s. Per-package
reductions on this idle pass:

| Package | Story 001 median | Story 005 median | Delta |
| --- | ---: | ---: | ---: |
| `bootstrap_portability` | 41.780s | 8.408s | **−79.9%** |
| `factory/current` | 14.808s | 10.901s | **−26.4%** |
| `factory/definitions` | 16.455s | 12.514s | **−24.0%** |

The combined sum of post-change package wall-time medians is
**31.823s** versus the Story 001 combined baseline of **73.043s**, a
**56.4%** reduction.

No production Factory Definitions or System Initialization cache was added on
this branch. The measured wins come from isolation-preserving process-wiring
reuse in the leased functional suites and shared support helpers.

## Fresh-run and functionallane validation

Repeated fresh runs and lane validation on the same revision:

| Check | Result |
| --- | --- |
| `go test -short ./tests/functional/bootstrap_portability -count=2 -timeout=10m` | pass, 16.982s wall / 15.288s Go |
| `go test -short ./tests/functional/factory/current -count=2 -timeout=10m` | pass, 21.809s wall / 20.096s Go |
| `go test -short ./tests/functional/factory/definitions -count=2 -timeout=10m` | pass, 23.713s wall / 22.150s Go |
| `go run ./cmd/functionallane -jobs=1 -count=1 -root=./tests/functional/bootstrap_portability -short=true -timeout=5m` | pass, 9.925s wall / 7.172s Go |
| `go run ./cmd/functionallane -jobs=1 -count=1 -root=./tests/functional/factory/current -short=true -timeout=5m` | pass, 14.416s wall / 11.728s Go |
| `go run ./cmd/functionallane -jobs=1 -count=1 -root=./tests/functional/factory/definitions -short=true -timeout=5m` | pass, 14.343s wall / 11.257s Go |

PowerShell callers should format `-root` as `('-root={0}' -f $path)` so the
package path reaches `cmd/functionallane`.

## Verification

- Nine independent `-count=1` package samples (table above)
- Repeated `-count=2` runs for all three optimized packages
- Sequential `cmd/functionallane` validation for all three roots
- `make functional-boundary-check`
- `make typecheck`
- `make test`
- No production service packages under Factory Definitions or System
  Initialization changed on this branch, so no additional owner-service unit
  suite was required beyond the short Go suite already covered by `make test`
- `make lint` remains blocked by the pre-existing frontend dead-code baseline
  drift documented in `progress.txt`; this story introduced no UI files
