# LTV-FUN-01-bootstrap-definitions-006 deterministic parallel isolation evidence

Status: complete

Captured: 2026-08-01T20:16:56Z UTC through 2026-08-01T20:17:13Z UTC

Revision: `22cd4d48b0fb65400038fee716c1eb578ff47d2d`

Branch: `LTV-FUN-01-bootstrap-definitions`

This report covers Story 006. It proves the three optimized packages can run
concurrently through isolated `cmd/functionallane` runners without filesystem,
port, environment, or canonical Factory-state leakage. Contended concurrent
timings remain diagnostic only and are not substituted for the Story 005
independent performance baseline (combined median sum **31.823s**).

## Machine and toolchain

- OS: Windows 11 Home, build `26200`, 64-bit
- Machine: MSI `MS-7D91`, 63.7 GB RAM, 24 logical processors
- Go: `go1.25.0 windows/amd64`
- Module: `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\LTV-FUN-01-bootstrap-definitions\go.mod`
- Go build cache: `C:\Users\andre\AppData\Local\go-build`
- Host CPU load before the concurrent run: ~20.3% average
- Other Factory `you` / `you-*` sessions were present on the shared host; this
  run is therefore a contended isolation diagnostic, not an independent
  performance sample
- Measurement timestamp normalization: UTC

## Concurrent functionallane protocol

Because `cmd/functionallane` accepts one `-root` value, three runner processes
were started together. Each used isolated temporary stdout/stderr logs and the
PowerShell-safe root form:

```powershell
go run ./cmd/functionallane -jobs=1 -count=1 `
  ('-root={0}' -f './tests/functional/bootstrap_portability') `
  -short=true -timeout=5m
go run ./cmd/functionallane -jobs=1 -count=1 `
  ('-root={0}' -f './tests/functional/factory/current') `
  -short=true -timeout=5m
go run ./cmd/functionallane -jobs=1 -count=1 `
  ('-root={0}' -f './tests/functional/factory/definitions') `
  -short=true -timeout=5m
```

All three processes were waited on, exit codes and package-reported durations
were recorded, leftover process command lines matching these package roots or
`cmd/functionallane` were scanned, and the temporary log directory was removed
after inspection.

## Concurrent results

| Runner | Exit | Package-reported duration |
| --- | ---: | ---: |
| `bootstrap_portability` | 0 | 8.327s |
| `factory/current` | 0 | 11.480s |
| `factory/definitions` | 0 | 12.533s |

Total concurrent wall time was **16.214s**. All three runners exited 0.

Isolation observations:

- No filesystem or build-cache collision failed any test
- No port-bind / address-in-use / file-lock collision markers appeared in
  stdout or stderr logs
- No environment contamination or canonical Factory-state leakage was observed
  through test failures or nondeterministic outcomes
- Leftover process scan after wait completion: **none** matching
  `cmd/functionallane` or the three package roots
- Temporary runner logs were removed after inspection

These concurrent durations are close to the Story 005 independent package
medians (8.408s / 10.901s / 12.514s). They remain diagnostic only and must not
replace the independent Story 005 combined baseline of **31.823s**.

## Comparison with Story 001 contended diagnostic

Story 001's pre-optimization concurrent diagnostic on the same three roots
passed with package-reported durations 18.656s / 25.752s / 31.888s and
41.836s concurrent wall time. The post-optimization concurrent sample still
passes deterministically and completes faster under current host load, but
that timing difference is not used as the performance claim.

## Verification

- Concurrent three-root `cmd/functionallane` isolation proof above
- Leftover process scan after cleanup: none
- `make functional-boundary-check`
- `make typecheck`
- `make test`
- Contended timings explicitly retained as diagnostic-only evidence
- `make lint` remains blocked by the pre-existing frontend dead-code baseline
  drift documented in `progress.txt`; this story introduced no UI files
