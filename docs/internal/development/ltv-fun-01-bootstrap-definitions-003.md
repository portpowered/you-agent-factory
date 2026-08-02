# LTV-FUN-01-bootstrap-definitions-003 Current Factory evidence

Status: complete

Captured: 2026-08-01T10:25:12Z UTC

Branch: `LTV-FUN-01-bootstrap-definitions`

This report covers Story 003. It records the Current Factory setup change and
the fresh/repeated functional evidence; the combined three-package target is
reserved for Story 005.

## Change

Current Factory scenarios now prepare named Factory roots through the same
`root.BuildProcess` instance that hosts the public server. The setup callback
runs after the server harness has prepared the scenario's isolated
HOME/USERPROFILE and streams, then invokes public Factory create, flatten, and
read commands through `Process.Execute`. Each test still owns its temporary
named-Factories root, source files, environment copy, server, and cleanup.

The public fixture-read helper also accepts an explicit environment so setup
cannot fall back to the host process environment. No Current Factory state or
durable Factory directory is shared between scenarios.

## Fresh benchmark

Machine and toolchain are the Story 001 Windows 11 / `go1.25.0`
`windows/amd64` context. Each sample ran independently from this story's
working tree with:

```powershell
go test -short ./tests/functional/factory/current -count=1 -timeout=5m
```

All samples exited 0. Wall time includes the Go test process and was measured
with a PowerShell stopwatch. The Story 001 Current Factory wall median was
14.808s.

| Sample | Wall time | Go test package time | Exit |
| --- | ---: | ---: | ---: |
| 1 | 11.949s | 9.956s | 0 |
| 2 | 11.981s | 9.871s | 0 |
| 3 | 13.180s | 11.074s | 0 |
| **Median** | **11.981s** | — | **0** |

The post-change median is 19.1% lower than the Story 001 median. The long
Current Factory coverage also passed with `-tags functionallong -count=1` in
15.241s wall time. A repeated `-short -count=2` run passed in 21.234s wall
time, with no cross-test Current Factory or filesystem leakage observed.

## Verification

- `go test -short ./tests/functional/factory/current -count=1 -timeout=5m`
- `go test -short ./tests/functional/factory/current -count=2 -timeout=10m`
- `go test -tags functionallong ./tests/functional/factory/current -count=1 -timeout=10m`
- `go test ./tests/functional/internal/support -count=1 -timeout=5m`
- `go test ./pkg/services/system_initialization/... ./pkg/services/factory_definitions/internal/services/distribution/packagedinstallation/... -short -count=1 -timeout=5m`
- `make functional-boundary-check`
- `make typecheck`
- `make test`
- `make lint` (blocked by the pre-existing frontend dead-code baseline drift
  documented in `progress.txt`; this change introduced no UI files)
