# LTV-FUN-01-bootstrap-definitions-004 Factory definition lifecycle evidence

Status: complete

Captured: 2026-08-01T19:13:26Z UTC

Revision: `3383ed3980279ddfaa5b26018e230aed1dec928e`

Branch: `LTV-FUN-01-bootstrap-definitions`

This report covers Story 004. It records the Factory definition lifecycle setup
reuse and the clean independent fresh/repeated evidence. Contended host samples
from earlier on the same revision are retained below as discarded performance
evidence. The combined three-package target remains Story 005.

## Change

Factory definition import/export, validation, and init scenarios reuse one
caller-owned `root.BuildProcess` instance for sequential public
`Process.Execute` setup and readback commands (bootstrap, create, update, list,
flatten, and validation). The hosted API-validation scenario loads its valid
fixture through the server harness `BeforeStart` seam on the same already-built
process. Each scenario still owns its temporary Factory roots, environment
copy, streams, working directory, and cleanup. No production Factory
Definitions or System Initialization cache was introduced; the measured win is
setup reuse of process wiring only.

## Fresh independent benchmark

Machine and toolchain match the Story 001 Windows 11 / `go1.25.0`
`windows/amd64` context. Each sample ran independently from this revision with:

```powershell
go test -short ./tests/functional/factory/definitions -count=1 -timeout=5m
```

All clean samples exited 0. Wall time includes the Go test process and was
measured with a PowerShell stopwatch. The Story 001 Factory definitions wall
median was 16.455s.

| Sample | Wall time | Go test package time | Exit |
| --- | ---: | ---: | ---: |
| 1 | 12.521s | 10.522s | 0 |
| 2 | 12.472s | 10.762s | 0 |
| 3 | 12.214s | 10.396s | 0 |
| **Median** | **12.472s** | — | **0** |

The post-change median is 24.2% lower than the Story 001 median. A repeated
`-short -count=2` run passed in 23.203s wall / 21.617s Go package time with no
cross-test definition or filesystem leakage observed.

## Contended and failed samples retained (not baseline)

Earlier samples on the same Story 004 revision while other Factory lanes and Go
checks contended the shared Windows host:

| Kind | Wall / Go | Exit | Disposition |
| --- | ---: | ---: | --- |
| Nominal contended pass | 56.569s / 52.497s | 0 | Discarded as performance evidence |
| Contended diagnostic | 123.568s / 115.894s | 0 | Discarded as performance evidence |
| Contended API readiness timeout (`TestAPIPreviewFactoryReturnsPublicTopology`) | 67.569s | non-zero | Retained as failed sample; not excluded from history |

Those timings are diagnostic only and are not substituted for the independent
median above.

## Verification

- `go test -short ./tests/functional/factory/definitions -count=1 -timeout=5m` (×3)
- `go test -short ./tests/functional/factory/definitions -count=2 -timeout=10m`
- `make functional-boundary-check`
- `make typecheck`
- `make test`
- `make lint` remains blocked by the pre-existing frontend dead-code baseline
  drift documented in `progress.txt`; this change introduced no UI files
