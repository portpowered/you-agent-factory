# Factory CLI Wire Composition

Production `you run` and the factory HTTP server share one composition root under
`cmd/factory/compose/`. [google/wire](https://github.com/google/wire) is used
only under `cmd/**`; `pkg/service`, `pkg/api`, and `pkg/cli` must not add
`wire:` struct tags or `wireinject` build files.

## Package layout

| Path | Role |
| --- | --- |
| `cmd/factory/compose/wire.go` | `//go:build wireinject` injector definition (`InjectFactoryService`) |
| `cmd/factory/compose/providers.go` | Provider functions wired into `service.ComposeFactoryService` |
| `cmd/factory/compose/wire_gen.go` | Generated injector (checked in; normal builds) |
| `cmd/factory/compose/generate.go` | `//go:generate` hook that runs the Wire CLI |
| `cmd/factory/compose/api_server.go` | `ServeAPIServer` (`api.NewServer` on the wired runtime) |
| `pkg/service/factory_compose.go` | Composition seams (`ComposeFactoryService`, collaborator snapshot); no Wire tags |

`pkg/cli/run` calls `compose.InjectFactoryService` through overrideable
`buildFactoryService` / `wireBuildFactoryService`. Tests replace
`buildFactoryService` without running Wire. Service-layer tests may still call
`service.BuildFactoryService` directly.

## Install the Wire CLI

From the repository root, either use the module `tool` entry (preferred for CI
and reproducible versions):

```bash
go run -mod=mod github.com/google/wire/cmd/wire -help
```

Or install a global binary (optional for local iteration):

```bash
go install github.com/google/wire/cmd/wire@latest
```

The dependency is declared in `go.mod` as `github.com/google/wire` with
`tool github.com/google/wire/cmd/wire`.

## Regenerate `wire_gen.go`

After changing `wire.go`, `providers.go`, or provider signatures that affect the
graph:

```bash
go generate ./cmd/factory/compose/...
```

This runs the directive in `cmd/factory/compose/generate.go`:

```go
//go:generate go run -mod=mod github.com/google/wire/cmd/wire
```

Then review the diff, commit `wire_gen.go` together with the provider changes,
and run backend checks:

```bash
go build ./cmd/factory/...
go test ./cmd/factory/compose/... ./pkg/cli/run/... -count=1
```

## Rules

- **Never hand-edit** `wire_gen.go`. Change providers or `wire.Build` in
  `wire.go`, regenerate, and commit the generated file.
- **Do not add Wire** under `pkg/**`. Keep composition logic in
  `pkg/service/factory_compose.go` and providers in `cmd/factory/compose/`.
- **Preserve behavior**: invalid `FactoryServiceConfig` must still fail with the
  same semantics as `service.BuildFactoryService` (see
  `cmd/factory/compose/compose_test.go` and `pkg/cli/run/run_compose_test.go`).

## When to regenerate

Regenerate after any of the following:

- New or removed provider in `providers.go` or `wire.go`
- Changed constructor signatures in the wired graph
- Moved composition seam in `pkg/service/factory_compose.go` that alters what
  providers must supply

No regeneration is required for changes that only touch
`pkg/cli/run` override seams, HTTP handlers, or tests that mock
`buildFactoryService` without altering the Wire graph.
