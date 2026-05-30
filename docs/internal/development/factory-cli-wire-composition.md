# Factory CLI Wire Composition

The `you` factory binary (`cmd/factory`) uses [Google Wire](https://github.com/google/wire) for optional compile-time dependency injection at the CLI entrypoint. Wire is confined to `cmd/factory/composition`; production packages under `pkg/service`, `pkg/api`, and `pkg/cli` stay free of `wireinject` build tags and Wire struct tags.

## Layout

| Path | Role |
| --- | --- |
| `cmd/factory/main.go` | Registers the generated builder with `pkg/cli/run` before `cli.Execute()`. |
| `cmd/factory/composition/wire.go` | Authored injector (`//go:build wireinject`) and `//go:generate` directive. |
| `cmd/factory/composition/wire_gen.go` | Generated injector checked into git; builds without the `wireinject` tag. |
| `pkg/service/factory_build.go` | Plain Go provider functions Wire calls (`ProvideFactorySessionsRegistry`, `ProvideRuntimeBuildService`, `BuildFactoryServiceFromCollaborators`, and related S6 collaborators). |
| `pkg/cli/run/run.go` | Package-level `buildFactoryService` seam; tests override it directly without calling registration. |

`pkg` cannot import `cmd`, so `main` registers `composition.BuildFactoryService` through `run.SetBuildFactoryService(run.FactoryServiceBuilderFromService(...))`. `service.BuildFactoryService` delegates to the same assembly path the generated injector uses.

## Install Wire CLI

The repository pins Wire through `go.mod`:

```bash
go install github.com/google/wire/cmd/wire@v0.6.0
```

Contributors may also rely on the module tool entry (no separate install) when regenerating from the composition package:

```bash
go run -mod=mod github.com/google/wire/cmd/wire
```

The `tool` block in `go.mod` includes `github.com/google/wire/cmd/wire` beside `oapi-codegen` so `go generate` resolves the same Wire version the module depends on.

## Regenerate `wire_gen.go`

From the repository root (the directory containing `go.mod`):

```bash
go generate ./cmd/factory/composition/...
```

Wire reads `cmd/factory/composition/wire.go` and rewrites `cmd/factory/composition/wire_gen.go`. Commit both files when provider sets change.

Verify the factory binary still builds:

```bash
go build -o /tmp/you-factory ./cmd/factory
```

Use an explicit output path when a sibling `factory/` directory exists in the working directory; `go build ./cmd/factory` alone can fail in that layout.

Focused tests:

```bash
go test ./cmd/factory/composition/...
go test ./pkg/cli/run/...
go test ./pkg/service/...
```

CI and local build-contract verification run `make factory-composition-smoke`, which regenerates `wire_gen.go` twice and fails when `git diff` shows drift. Use that target before pushing provider changes.

## Rules

1. **Never hand-edit `wire_gen.go`.** Change providers in `wire.go` or exported helpers in `pkg/service/factory_build.go`, then regenerate.
2. **Keep Wire only under `cmd/factory/composition`.** Do not add `wireinject` tags or `wire:"..."` struct tags to `pkg/service`, `pkg/api`, `pkg/cli`, or other production packages.
3. **Preserve the `pkg/cli/run` seam.** Production registration lives in `cmd/factory/main.go`; unit tests continue to assign `buildFactoryService` directly without calling `SetBuildFactoryService`.
4. **Keep business logic in `pkg/service`.** Wire providers should delegate to plain constructors and assembly helpers; do not duplicate factory build logic in `cmd/`.

## When to regenerate

Regenerate and commit `wire_gen.go` when you:

- Add, remove, or reorder providers in `cmd/factory/composition/wire.go`.
- Change signatures of functions referenced from the Wire provider set in `pkg/service/factory_build.go`.
- Upgrade `github.com/google/wire` in `go.mod`.

If `wire_gen.go` is missing on a fresh checkout, run `go generate ./cmd/factory/composition/...` once before `go build ./cmd/factory`.

## Related maintainer surfaces

- [Development Guide](development.md) — root verification commands (`make typecheck`, `make test`).
- [Development Guide Relevant Files](../processes/development-guide-relevant-files.md) — inventory row for `cmd/factory/composition/`.

Stale-generation checks for `wire_gen.go` run in CI via `make factory-composition-smoke` (also included in `make verify-build-contracts` and `make verify-pr`).
